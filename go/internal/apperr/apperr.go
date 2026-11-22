// Package apperr ports autobrowser_client/errors.py.
package apperr

// Code is the machine-readable error code printed in JSON errors.
type Code string

const (
	InvalidArgument     Code = "INVALID_ARGUMENT"
	ConfigErr           Code = "CONFIG_ERROR"
	ScriptValidationErr Code = "SCRIPT_VALIDATION_ERROR"
	SessionNotFound     Code = "SESSION_NOT_FOUND"
	BrowserStartFailed  Code = "BROWSER_START_FAILED"
	ProfileNotFound     Code = "PROFILE_NOT_FOUND"
	PageNotFound        Code = "PAGE_NOT_FOUND"
	ActionFailed        Code = "ACTION_FAILED"
	Timeout             Code = "TIMEOUT"
	NotImplemented      Code = "NOT_IMPLEMENTED"
	InternalErr         Code = "INTERNAL_ERROR"
)

// Error is the Go counterpart of AppError.
type Error struct {
	Code    Code
	Message string
}

func (e *Error) Error() string { return e.Message }

// New builds an Error with the given code.
func New(code Code, message string) *Error { return &Error{Code: code, Message: message} }

// Config is ConfigError.
func Config(message string) *Error { return New(ConfigErr, message) }

// ScriptValidation is ScriptValidationError.
func ScriptValidation(message string) *Error { return New(ScriptValidationErr, message) }

// Invalid is AppError(INVALID_ARGUMENT, ...).
func Invalid(message string) *Error { return New(InvalidArgument, message) }
