package profiles

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
)

func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=NONE")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

func TestAcquireRelease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	l := NewLock(dir)
	if err := l.Acquire(); err != nil {
		t.Fatal(err)
	}
	if !l.Acquired() {
		t.Fatal("not acquired")
	}
	data, _ := os.ReadFile(filepath.Join(dir, LockFilename))
	if string(data) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock content %q", data)
	}
	l.Release()
	if _, err := os.Stat(filepath.Join(dir, LockFilename)); !os.IsNotExist(err) {
		t.Fatal("lock file left")
	}
	l.Release() // second release is a no-op
}

func TestLockedByLivePID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "work")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(dir, LockFilename), []byte(strconv.Itoa(os.Getpid())), 0o644)
	err := NewLock(dir).Acquire()
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.BrowserStartFailed {
		t.Fatalf("got %v", err)
	}
	if want := "Profile 'work' is locked by PID " + strconv.Itoa(os.Getpid()); ae.Message != want {
		t.Fatalf("message %q, want %q", ae.Message, want)
	}
	if _, err := os.Stat(filepath.Join(dir, LockFilename)); err != nil {
		t.Fatal("foreign lock must stay")
	}
}

func TestStaleLockReplaced(t *testing.T) {
	for name, content := range map[string]string{
		"dead pid": strconv.Itoa(deadPID(t)),
		"garbage":  "not a pid",
		"empty":    "",
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "p")
			os.Mkdir(dir, 0o755)
			os.WriteFile(filepath.Join(dir, LockFilename), []byte(content), 0o644)
			l := NewLock(dir)
			if err := l.Acquire(); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(dir, LockFilename))
			if string(data) != strconv.Itoa(os.Getpid()) {
				t.Fatalf("lock not replaced: %q", data)
			}
		})
	}
}

func TestCheckExists(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "ok"), 0o755)
	os.WriteFile(filepath.Join(root, "file"), nil, 0o644)
	if got, err := CheckExists(root, "ok"); err != nil || got != filepath.Join(root, "ok") {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, name := range []string{"missing", "file"} {
		_, err := CheckExists(root, name)
		var ae *apperr.Error
		if !errors.As(err, &ae) || ae.Code != apperr.ProfileNotFound {
			t.Fatalf("%s: got %v", name, err)
		}
		if want := "Profile '" + name + "' not found at " + filepath.Join(root, name); ae.Message != want {
			t.Fatalf("message %q", ae.Message)
		}
	}
}
