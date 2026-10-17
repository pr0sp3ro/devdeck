package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestConcurrentConfigUpdatesPreserveSettings(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const count = 8
	paths := make([]string, count)
	start := make(chan struct{})
	errors := make(chan error, count)

	for i := range paths {
		paths[i] = t.TempDir()

		go func(i int) {
			<-start
			errors <- run([]string{"project", "add", fmt.Sprintf("project_%d", i), paths[i]})
		}(i)
	}

	close(start)

	for range paths {
		if err := <-errors; err != nil {
			t.Error(err)
		}
	}

	path, err := configPath()

	if err != nil {
		t.Fatal(err)
	}

	c, err := loadConfig(path)

	if err != nil || len(c.Projects) != count {
		t.Fatalf("concurrent writes lost projects: %v, %v", c, err)
	}

	// An even number of concurrent toggles must leave the favorite disabled.
	start = make(chan struct{})

	for range paths {
		go func() {
			<-start
			_, err := toggleFavorite(path, paths[0])
			errors <- err
		}()
	}

	close(start)

	for range paths {
		if err := <-errors; err != nil {
			t.Error(err)
		}
	}

	c, err = loadConfig(path)

	if err != nil || len(c.Projects) != count || c.Projects["project_0"].Favorite {
		t.Fatalf("concurrent toggles lost settings: %v, %v", c, err)
	}
}

func TestConfigLockUsesSymlinkTargetAndSurvivesReplacement(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	target := filepath.Join(root, "target.toml")
	link := filepath.Join(root, "config.toml")

	if err := saveConfig(target, Config{}); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("target.toml", link); err != nil {
		t.Fatal(err)
	}

	lock, err := lockConfig(link, syscall.LOCK_EX)

	if err != nil {
		t.Fatal(err)
	}

	defer lock.Close()

	if err := saveConfig(link, Config{Sources: []Source{{Path: root}}}); err != nil {
		t.Fatal(err)
	}

	lockPath, err := configLockPath(target)

	if err != nil {
		t.Fatal(err)
	}

	other, err := os.OpenFile(lockPath, os.O_RDWR, 0600)

	if err != nil {
		t.Fatal(err)
	}

	defer other.Close()

	if err := syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != syscall.EWOULDBLOCK {
		t.Fatalf("replacement bypassed the shared lock: %v", err)
	}

	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}

	if err := syscall.Flock(int(other.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("closing the file did not release the lock: %v", err)
	}
}

func TestConfigReadWaitsForEditorLock(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.toml")

	if err := saveConfig(path, Config{}); err != nil {
		t.Fatal(err)
	}

	lock, err := lockConfig(path, syscall.LOCK_EX)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { lock.Close() })

	if err := os.WriteFile(path, []byte("invalid = ["), 0600); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		close(started)
		c, err := readConfig(path)

		if err == nil && len(c.Sources) != 1 {
			err = fmt.Errorf("reader did not receive the completed configuration: %v", c)
		}

		done <- err
	}()

	<-started

	select {
	case err := <-done:
		t.Fatalf("reader bypassed the editor lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := saveConfig(path, Config{Sources: []Source{{Path: t.TempDir()}}}); err != nil {
		t.Fatal(err)
	}

	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not resume after the editor released its lock")
	}
}

func TestConfigReadDoesNotCreateMissingConfiguration(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "devdeck", "config.toml")
	c, err := readConfig(path)

	if err != nil || len(c.Projects) != 0 {
		t.Fatalf("missing config: %v, %v", c, err)
	}

	if _, err := os.Lstat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("reading missing configuration created its directory: %v", err)
	}
}

func TestConfigReadFromReadOnlyDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	target := filepath.Join(root, "config.toml")
	c := Config{Sources: []Source{{Path: root}}}

	if err := saveConfig(target, c); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(t.TempDir(), "config.toml")

	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := os.Chmod(root, 0700); err != nil {
			t.Error(err)
		}
	})

	for _, path := range []string{target, alias} {
		loaded, err := readConfig(path)

		if err != nil || len(loaded.Sources) != 1 || loaded.Sources[0].Path != root {
			t.Fatalf("could not read configuration from a read-only directory: %v, %v", loaded, err)
		}
	}

	entries, err := os.ReadDir(root)

	if err != nil || len(entries) != 1 || entries[0].Name() != "config.toml" {
		t.Fatalf("reading created a file beside configuration: %v, %v", entries, err)
	}
}

func TestConfigLockIdentityBeforeAndAfterCreation(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")

	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "devdeck", "config.toml")
	before, err := configLockPath(filepath.Join(alias, "devdeck", "config.toml"))

	if err != nil {
		t.Fatal(err)
	}

	if err := saveConfig(path, Config{}); err != nil {
		t.Fatal(err)
	}

	after, err := configLockPath(path)

	if err != nil || before != after {
		t.Fatalf("configuration creation changed its lock identity: %q, %q, %v", before, after, err)
	}
}
