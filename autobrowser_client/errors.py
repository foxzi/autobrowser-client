from __future__ import annotations

from enum import StrEnum


class ErrorCode(StrEnum):
    INVALID_ARGUMENT = "INVALID_ARGUMENT"
    CONFIG_ERROR = "CONFIG_ERROR"
    SCRIPT_VALIDATION_ERROR = "SCRIPT_VALIDATION_ERROR"
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


class NotImplementedAppError(AppError):
    def __init__(self, message: str) -> None:
        super().__init__(ErrorCode.NOT_IMPLEMENTED, message)
