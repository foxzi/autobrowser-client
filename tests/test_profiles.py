"""Unit tests for profile locking and existence check."""

from __future__ import annotations

import os
import tempfile
from pathlib import Path

import pytest

from autobrowser_client.errors import AppError, ErrorCode
from autobrowser_client.profiles.manager import (
    LOCK_FILENAME,
    ProfileLock,
    ProfileLockedError,
    check_profile_exists,
)


@pytest.fixture()
def profile_dir(tmp_path: Path) -> Path:
    d = tmp_path / "test_profile"
    d.mkdir()
    return d


def test_acquire_and_release(profile_dir: Path):
    lock = ProfileLock(profile_dir)
    lock.acquire()
    assert lock.is_acquired
    assert (profile_dir / LOCK_FILENAME).exists()
    assert (profile_dir / LOCK_FILENAME).read_text() == str(os.getpid())

    lock.release()
    assert not lock.is_acquired
    assert not (profile_dir / LOCK_FILENAME).exists()


def test_double_release_is_safe(profile_dir: Path):
    lock = ProfileLock(profile_dir)
    lock.acquire()
    lock.release()
    lock.release()  # Should not raise


def test_concurrent_lock_raises(profile_dir: Path):
    lock1 = ProfileLock(profile_dir)
    lock1.acquire()

    lock2 = ProfileLock(profile_dir)
    with pytest.raises(ProfileLockedError) as exc_info:
        lock2.acquire()
    assert str(os.getpid()) in str(exc_info.value)

    lock1.release()


def test_stale_lock_cleaned_up(profile_dir: Path):
    # Write a lock with a dead PID
    (profile_dir / LOCK_FILENAME).write_text("999999999")

    lock = ProfileLock(profile_dir)
    lock.acquire()  # Should succeed after cleaning stale lock
    assert lock.is_acquired
    lock.release()


def test_stale_lock_with_invalid_content(profile_dir: Path):
    # Write garbage content in lock file
    (profile_dir / LOCK_FILENAME).write_text("not-a-pid")

    lock = ProfileLock(profile_dir)
    lock.acquire()  # Should succeed (can't parse PID -> treated as stale)
    assert lock.is_acquired
    lock.release()


def test_lock_after_release(profile_dir: Path):
    lock1 = ProfileLock(profile_dir)
    lock1.acquire()
    lock1.release()

    lock2 = ProfileLock(profile_dir)
    lock2.acquire()
    assert lock2.is_acquired
    lock2.release()


def test_profile_locked_error_is_app_error(profile_dir: Path):
    err = ProfileLockedError("myprofile", 12345)
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.BROWSER_START_FAILED
    assert "myprofile" in err.message
    assert "12345" in err.message


def test_check_profile_exists(tmp_path: Path):
    profile = tmp_path / "existing"
    profile.mkdir()
    result = check_profile_exists(str(tmp_path), "existing")
    assert result == profile


def test_check_profile_not_exists(tmp_path: Path):
    with pytest.raises(AppError) as exc_info:
        check_profile_exists(str(tmp_path), "missing")
    assert exc_info.value.code == ErrorCode.PROFILE_NOT_FOUND
