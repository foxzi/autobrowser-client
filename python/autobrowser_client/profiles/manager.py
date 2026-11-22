"""Profile existence check and file-based locking."""

from __future__ import annotations

import logging
import os
from pathlib import Path

from autobrowser_client.errors import AppError, ErrorCode

logger = logging.getLogger(__name__)

LOCK_FILENAME = "profile.lock"


class ProfileLockedError(AppError):
    def __init__(self, profile: str, holder_pid: int | None = None) -> None:
        detail = f"Profile '{profile}' is locked"
        if holder_pid is not None:
            detail += f" by PID {holder_pid}"
        super().__init__(ErrorCode.BROWSER_START_FAILED, detail)


class ProfileLock:
    """File-based lock for a browser profile directory.

    Writes current PID to ``profile.lock`` inside the profile dir.
    On release the lock file is removed.
    """

    def __init__(self, profile_dir: Path) -> None:
        self._profile_dir = profile_dir
        self._lock_path = profile_dir / LOCK_FILENAME
        self._acquired = False

    @property
    def is_acquired(self) -> bool:
        return self._acquired

    def acquire(self) -> None:
        """Acquire lock. Raises ProfileLockedError if already held."""
        if self._lock_path.exists():
            holder_pid = self._read_holder_pid()
            if holder_pid is not None and self._is_pid_alive(holder_pid):
                raise ProfileLockedError(self._profile_dir.name, holder_pid)
            # Stale lock — previous process died without cleanup
            logger.warning(
                "Removing stale lock for profile '%s' (PID %s)",
                self._profile_dir.name,
                holder_pid,
            )
            self._lock_path.unlink(missing_ok=True)

        self._lock_path.write_text(str(os.getpid()), encoding="utf-8")
        self._acquired = True
        logger.debug("Acquired lock for profile '%s'", self._profile_dir.name)

    def release(self) -> None:
        """Release the lock if held."""
        if not self._acquired:
            return
        self._lock_path.unlink(missing_ok=True)
        self._acquired = False
        logger.debug("Released lock for profile '%s'", self._profile_dir.name)

    def _read_holder_pid(self) -> int | None:
        try:
            text = self._lock_path.read_text(encoding="utf-8").strip()
            return int(text)
        except (ValueError, OSError):
            return None

    @staticmethod
    def _is_pid_alive(pid: int) -> bool:
        try:
            os.kill(pid, 0)
            return True
        except OSError:
            return False


def check_profile_exists(profiles_dir: str, profile_name: str) -> Path:
    """Return profile path if it exists, raise otherwise."""
    profile_dir = Path(profiles_dir) / profile_name
    if not profile_dir.is_dir():
        raise AppError(
            ErrorCode.PROFILE_NOT_FOUND,
            f"Profile '{profile_name}' not found at {profile_dir}",
        )
    return profile_dir
