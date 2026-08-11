// Package atomicfile writes a file via a temp file + rename, so a reader
// never observes a partially-written file and a crash mid-write never
// truncates the previous contents.
package atomicfile

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces path's contents with data, creating path's parent
// directory if it's missing, and chmods the result to mode.
//
// The temp file is created in path's own directory (rename is only atomic
// within a filesystem) with a unique per-call name: a fixed ".tmp" name
// races when several moomux processes write the same file at once, where
// one process's rename can steal or delete another's in-flight temp. It's
// also dot-prefixed so a leftover temp from a crash stays out of the way of
// anything listing the directory.
func Write(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
