//go:build !linux

package ingestor

import "os"

// fileInode returns 0 on non-Linux platforms (inode detection not supported).
func fileInode(_ *os.File) (uint64, error) {
	return 0, nil
}

// fileInodeByPath returns 0 on non-Linux platforms (inode detection not supported).
func fileInodeByPath(_ string) (uint64, error) {
	return 0, nil
}
