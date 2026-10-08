package activity

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type RotatingWriter struct {
	mu      sync.Mutex
	path    string
	max     int64
	backups int
	file    *os.File
	size    int64
}

func NewRotatingWriter(path string, max int64, backups int) (*RotatingWriter, error) {
	r := &RotatingWriter{path: path, max: max, backups: backups}
	err := r.open()
	return r, err
}
func (r *RotatingWriter) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.file = f
	r.size = info.Size()
	return nil
}
func (r *RotatingWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.file.Close(); err != nil {
			return 0, err
		}
		for i := r.backups; i > 1; i-- {
			if err := os.Rename(fmt.Sprintf("%s.%d", r.path, i-1), fmt.Sprintf("%s.%d", r.path, i)); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return 0, err
		}
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}
func (r *RotatingWriter) Close() error { r.mu.Lock(); defer r.mu.Unlock(); return r.file.Close() }

var _ io.WriteCloser = (*RotatingWriter)(nil)
