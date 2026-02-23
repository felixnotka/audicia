package ingestor

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var fileLog = ctrl.Log.WithName("ingestor").WithName("file")

// FileIngestor tails a Kubernetes audit log file and emits events.
type FileIngestor struct {
	// Path is the filesystem path to the audit log.
	Path string

	// StartPosition is the position to resume from.
	StartPosition Position

	// BatchSize is the number of events to read per batch.
	BatchSize int

	mu       sync.Mutex
	position Position
}

// NewFileIngestor creates a new file-based ingestor.
func NewFileIngestor(path string, startPos Position, batchSize int) *FileIngestor {
	if batchSize <= 0 {
		batchSize = 500
	}
	return &FileIngestor{
		Path:          path,
		StartPosition: startPos,
		BatchSize:     batchSize,
		position:      startPos,
	}
}

// Start begins tailing the audit log file.
func (f *FileIngestor) Start(ctx context.Context) (<-chan auditv1.Event, error) {
	ch := make(chan auditv1.Event, f.BatchSize)

	go func() {
		defer close(ch)
		f.tail(ctx, ch)
	}()

	return ch, nil
}

// Checkpoint returns the current file position.
func (f *FileIngestor) Checkpoint() Position {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.position
}

func (f *FileIngestor) setPosition(pos Position) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.position = pos
}

// tail is the main loop that opens, reads, and watches the audit log file.
func (f *FileIngestor) tail(ctx context.Context, ch chan<- auditv1.Event) {
	for {
		if err := f.readFile(ctx, ch); err != nil {
			fileLog.Error(err, "error reading audit log", "path", f.Path)
		}

		// Wait before retrying (file may not exist yet, or rotation happened).
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// readFile opens the file, seeks to the checkpoint offset, and reads events.
func (f *FileIngestor) readFile(ctx context.Context, ch chan<- auditv1.Event) error {
	file, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			fileLog.V(1).Info("error closing audit log file", "error", cerr)
		}
	}()

	// Check inode to detect log rotation.
	currentInode, err := fileInode(file)
	if err != nil {
		fileLog.V(1).Info("could not get inode, skipping inode check", "error", err)
	}

	startPos := f.Checkpoint()

	// If inode changed (rotation), read from beginning.
	if startPos.Inode != 0 && currentInode != 0 && startPos.Inode != currentInode {
		fileLog.Info("detected log rotation (inode changed)", "oldInode", startPos.Inode, "newInode", currentInode)
		startPos.FileOffset = 0
	}

	// Seek to the checkpoint offset.
	if startPos.FileOffset > 0 {
		if _, err := file.Seek(startPos.FileOffset, io.SeekStart); err != nil {
			return err
		}
	}

	scanner := newAuditScanner(file)

	if _, err := scanAndEmit(ctx, scanner, ch); err != nil {
		return err
	}

	// Update checkpoint with current file position and inode.
	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	f.setPosition(Position{
		FileOffset:    offset,
		Inode:         currentInode,
		LastTimestamp: time.Now().UTC().Format(time.RFC3339),
	})

	// After exhausting current data, poll for new data.
	return f.pollForData(ctx, file, ch, currentInode)
}

// newAuditScanner creates a bufio.Scanner configured for audit log lines (up to 1MB).
func newAuditScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return s
}

// scanAndEmit reads all available lines from the scanner, parses them as audit
// events, and sends them on ch. Returns whether any events were emitted.
func scanAndEmit(ctx context.Context, scanner *bufio.Scanner, ch chan<- auditv1.Event) (bool, error) {
	readAny := false
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return readAny, ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event auditv1.Event
		if err := json.Unmarshal(line, &event); err != nil {
			fileLog.V(1).Info("skipping malformed audit event line", "error", err)
			continue
		}

		select {
		case ch <- event:
			readAny = true
		case <-ctx.Done():
			return readAny, ctx.Err()
		}
	}
	return readAny, scanner.Err()
}

// pollForData waits for the file to grow (new audit events appended).
func (f *FileIngestor) pollForData(ctx context.Context, file *os.File, ch chan<- auditv1.Event, originalInode uint64) error {
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		// Check if file was rotated by comparing inodes.
		currentInode, err := fileInodeByPath(f.Path)
		if err != nil {
			// File may have been removed during rotation; return to reopen.
			return nil
		}
		if originalInode != 0 && currentInode != 0 && originalInode != currentInode {
			// File rotated. Save position and return so tail() reopens.
			fileLog.Info("file rotated during polling, reopening")
			pos := f.Checkpoint()
			pos.Inode = currentInode
			pos.FileOffset = 0
			f.setPosition(pos)
			return nil
		}

		// Try to read more lines.
		readAny, err := scanAndEmit(ctx, scanner, ch)
		if err != nil {
			return err
		}

		// Update checkpoint after reading new data.
		if readAny {
			offset, err := file.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}
			f.setPosition(Position{
				FileOffset:    offset,
				Inode:         originalInode,
				LastTimestamp: time.Now().UTC().Format(time.RFC3339),
			})
			// Reset scanner for next poll cycle.
			scanner = newAuditScanner(file)
		}
	}
}
