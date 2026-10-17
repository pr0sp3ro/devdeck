package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestConcurrentConfigUpdatesPreserveSettings(t *testing.T) {
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
	root := t.TempDir()
	target := filepath.Join(root, "target.toml")
	link := filepath.Join(root, "config.toml")

	if err := saveConfig(target, Config{}); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("target.toml", link); err != nil {
		t.Fatal(err)
	}

	lock, err := lockConfig(link)

	if err != nil {
		t.Fatal(err)
	}

	defer lock.Close()

	if err := saveConfig(link, Config{Sources: []Source{{Path: root}}}); err != nil {
		t.Fatal(err)
	}

	other, err := os.OpenFile(target+".lock", os.O_RDWR, 0600)

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
