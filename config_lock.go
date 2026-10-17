package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Keep the sidecar file in place so atomic config replacements share one lock.
func lockConfig(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	if _, err := os.Lstat(path); err == nil {
		resolved, err := filepath.EvalSymlinks(path)

		if err != nil {
			return nil, fmt.Errorf("resolve configuration path %s: %w", path, err)
		}

		path = resolved
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	dir, err := filepath.EvalSymlinks(filepath.Dir(path))

	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filepath.Join(dir, filepath.Base(path))+".lock", os.O_CREATE|os.O_RDWR, 0600)

	if err != nil {
		return nil, err
	}

	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)

		if err != syscall.EINTR {
			break
		}
	}

	if err != nil {
		file.Close()
		return nil, fmt.Errorf("lock configuration: %w", err)
	}

	return file, nil
}
