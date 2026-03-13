"""Unit tests for error codes and AppError hierarchy."""

from __future__ import annotations

import pytest

from autobrowser_client.errors import (
    AppError,
    BrowserStartError,
    ConfigError,
    ErrorCode,
    NotImplementedAppError,
    ScriptValidationError,
    SessionNotFoundError,
)


def test_error_codes_are_strings():
    assert ErrorCode.INVALID_ARGUMENT == "INVALID_ARGUMENT"
    assert ErrorCode.BROWSER_START_FAILED == "BROWSER_START_FAILED"


def test_app_error_base():
    err = AppError(ErrorCode.INTERNAL_ERROR, "something broke")
    assert err.code == ErrorCode.INTERNAL_ERROR
    assert err.message == "something broke"
    assert str(err) == "something broke"


def test_config_error():
    err = ConfigError("bad config")
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.CONFIG_ERROR
    assert err.message == "bad config"


def test_script_validation_error():
    err = ScriptValidationError("invalid step")
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.SCRIPT_VALIDATION_ERROR


def test_session_not_found_error():
    err = SessionNotFoundError("sess_abc123")
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.SESSION_NOT_FOUND
    assert "sess_abc123" in err.message


def test_browser_start_error():
    err = BrowserStartError("chrome not found")
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.BROWSER_START_FAILED


def test_not_implemented_error():
    err = NotImplementedAppError("future feature")
    assert isinstance(err, AppError)
    assert err.code == ErrorCode.NOT_IMPLEMENTED


def test_all_error_codes_defined():
    expected = {
        "INVALID_ARGUMENT",
        "CONFIG_ERROR",
        "SCRIPT_VALIDATION_ERROR",
        "SESSION_NOT_FOUND",
        "BROWSER_START_FAILED",
        "PROFILE_NOT_FOUND",
        "TIMEOUT",
        "NOT_IMPLEMENTED",
        "INTERNAL_ERROR",
    }
    actual = {e.value for e in ErrorCode}
    assert actual == expected


def test_error_is_catchable_as_exception():
    with pytest.raises(Exception):
        raise BrowserStartError("test")

    with pytest.raises(AppError):
        raise ConfigError("test")
