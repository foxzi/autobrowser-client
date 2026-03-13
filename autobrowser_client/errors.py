from __future__ import annotations

from enum import StrEnum


class ErrorCode(StrEnum):
    INVALID_ARGUMENT = "INVALID_ARGUMENT"
    CONFIG_ERROR = "CONFIG_ERROR"
    SCRIPT_VALIDATION_ERROR = "SCRIPT_VALIDATION_ERROR"
    SESSION_NOT_FOUND = "SESSION_NOT_FOUND"
    BROWSER_START_FAILED = "BROWSER_START_FAILED"
    PROFILE_NOT_FOUND = "PROFILE_NOT_FOUND"
    TIMEOUT = "TIMEOUT"
    NOT_IMPLEMENTED = "NOT_IMPLEMENTED"
    INTERNAL_ERROR = "INTERNAL_ERROR"


class AppError(Exception):
    def __init__(self, code: ErrorCode, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message


class ConfigError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.CONFIG_ERROR, message)


class ScriptValidationError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.SCRIPT_VALIDATION_ERROR, message)


class SessionNotFoundError(AppError):
    def __init__(self, session_id: str) -> None:
        super().__init__(ErrorCode.SESSION_NOT_FOUND, f"Session not found: {session_id}")


class BrowserStartError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.BROWSER_START_FAILED, message)


class NotImplementedAppError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.NOT_IMPLEMENTED, message)
