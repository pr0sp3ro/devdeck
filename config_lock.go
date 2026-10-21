package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Resolve existing ancestors too, keeping the lock identity stable before creation.
func configLockPath(path string) (string, error) {
	path, err := filepath.Abs(path)

	if err != nil {
		return "", err
	}

	var missing []string

	for {
		resolved, err := filepath.EvalSymlinks(path)

		if err == nil {
			path = resolved
			break
		}

		if !os.IsNotExist(err) {
			return "", err
		}

		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			return "", fmt.Errorf("resolve configuration path %s: %w", path, err)
		}

		missing = append(missing, filepath.Base(path))
		path = filepath.Dir(path)
	}

	for i := len(missing) - 1; i >= 0; i-- {
		path = filepath.Join(path, missing[i])
	}

	state := os.Getenv("XDG_STATE_HOME")

	if state == "" {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", err
		}

		state = filepath.Join(home, ".local", "state")
	}

	if !filepath.IsAbs(state) {
		return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
	}

	hash := sha256.Sum256([]byte(path))
	return filepath.Join(state, "dirhaven", "locks", fmt.Sprintf("%x.lock", hash)), nil
}

// Keep lock files in place so atomic config replacements share one lock.
func lockConfig(path string, mode int) (*os.File, error) {
	lockPath, err := configLockPath(path)

	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)

	if err != nil {
		return nil, err
	}

	for {
		err = syscall.Flock(int(file.Fd()), mode)

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

func readConfig(path string) (Config, error) {
	lock, err := lockConfig(path, syscall.LOCK_SH)

	if err != nil {
		return Config{}, err
	}

	defer lock.Close()

	return loadConfig(path)
}
