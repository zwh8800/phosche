package errors

import "fmt"

// AppError is the standard application error type for phosche.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Details    any    `json:"details,omitempty"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewNotFoundError creates a 404 AppError.
func NewNotFoundError(msg string) *AppError {
	return &AppError{
		Code:       "NOT_FOUND",
		Message:    msg,
		HTTPStatus: 404,
	}
}

// NewValidationError creates a 400 AppError with optional details.
func NewValidationError(msg string, details any) *AppError {
	return &AppError{
		Code:       "VALIDATION_ERROR",
		Message:    msg,
		HTTPStatus: 400,
		Details:    details,
	}
}

// NewInternalError creates a 500 AppError wrapping the given error.
func NewInternalError(err error) *AppError {
	return &AppError{
		Code:       "INTERNAL_ERROR",
		Message:    "an internal error occurred",
		HTTPStatus: 500,
		Err:        err,
	}
}

// NewServiceUnavailableError creates a 503 AppError.
func NewServiceUnavailableError(msg string) *AppError {
	return &AppError{
		Code:       "SERVICE_UNAVAILABLE",
		Message:    msg,
		HTTPStatus: 503,
	}
}
