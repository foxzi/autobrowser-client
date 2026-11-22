// Package profiles ports autobrowser_client/profiles/manager.py: the
// profile.lock file of a profile directory and the profile existence check.
package profiles

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
)

// LockFilename is the name of the lock file inside a profile directory.
const LockFilename = "profile.lock"

// LockedError is ProfileLockedError: BROWSER_START_FAILED with the holder PID.
func LockedError(profile string, holderPID int) *apperr.Error {
	return apperr.New(apperr.BrowserStartFailed, fmt.Sprintf("Profile '%s' is locked by PID %d", profile, holderPID))
}

// Lock is a file lock: profile.lock holds the PID of the owner.
type Lock struct {
	dir      string
	path     string
	acquired bool
}

// NewLock makes a lock for the profile directory (nothing is created yet).
func NewLock(profileDir string) *Lock {
	return &Lock{dir: profileDir, path: filepath.Join(profileDir, LockFilename)}
}

// Acquired reports whether Acquire succeeded and Release was not called.
func (l *Lock) Acquired() bool { return l.acquired }

// Acquire takes the lock. A lock of a live process is an error; a lock of a
// dead (or unreadable) one is stale and gets replaced.
func (l *Lock) Acquire() error {
	name := filepath.Base(l.dir)
	if _, err := os.Stat(l.path); err == nil {
		holder, ok := l.readHolderPID()
		if ok && pidAlive(holder) {
			return LockedError(name, holder)
		}
		if ok {
			slog.Warn(fmt.Sprintf("Removing stale lock for profile '%s' (PID %d)", name, holder))
		} else {
			slog.Warn(fmt.Sprintf("Removing stale lock for profile '%s' (PID None)", name))
		}
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.WriteFile(l.path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return err
	}
	l.acquired = true
	slog.Debug(fmt.Sprintf("Acquired lock for profile '%s'", name))
	return nil
}

// Release removes the lock file if the lock is held.
func (l *Lock) Release() {
	if !l.acquired {
		return
	}
	_ = os.Remove(l.path)
	l.acquired = false
	slog.Debug(fmt.Sprintf("Released lock for profile '%s'", filepath.Base(l.dir)))
}

func (l *Lock) readHolderPID() (int, bool) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid, err == nil
}

// CheckExists returns the profile directory path or a PROFILE_NOT_FOUND error.
func CheckExists(profilesDir, name string) (string, error) {
	dir := filepath.Join(profilesDir, name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", apperr.New(apperr.ProfileNotFound, fmt.Sprintf("Profile '%s' not found at %s", name, dir))
	}
	return dir, nil
}
