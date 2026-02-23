//go:build linux

package ingestor

import (
	"os"
	"syscall"
)

// fileInode returns the inode number for an open file.
func fileInode(f *os.File) (uint64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, nil
	}
	return stat.Ino, nil
}

// fileInodeByPath returns the inode number for a file path.
func fileInodeByPath(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, nil
	}
	return stat.Ino, nil
}
