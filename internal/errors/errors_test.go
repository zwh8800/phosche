package errors

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	err := NewNotFoundError("photo not found")
	got := err.Error()
	want := "NOT_FOUND: photo not found"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAppError_ErrorWithWrapped(t *testing.T) {
	inner := errors.New("disk full")
	err := NewInternalError(inner)
	got := err.Error()
	want := "INTERNAL_ERROR: an internal error occurred: disk full"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAppError_JSONMarshal(t *testing.T) {
	err := NewNotFoundError("photo not found")
	data, err2 := json.Marshal(err)
	if err2 != nil {
		t.Fatalf("json.Marshal failed: %v", err2)
	}

	var decoded map[string]any
	if err3 := json.Unmarshal(data, &decoded); err3 != nil {
		t.Fatalf("json.Unmarshal failed: %v", err3)
	}

	if decoded["code"] != "NOT_FOUND" {
		t.Errorf("code: got %v, want NOT_FOUND", decoded["code"])
	}
	if decoded["message"] != "photo not found" {
		t.Errorf("message: got %v, want photo not found", decoded["message"])
	}
	// HTTPStatus and Err should be excluded from JSON
	if _, ok := decoded["http_status"]; ok {
		t.Error("http_status should not appear in JSON output")
	}
	if _, ok := decoded["err"]; ok {
		t.Error("err should not appear in JSON output")
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := errors.New("root cause")
	err := NewInternalError(inner)

	unwrapped := err.Unwrap()
	if unwrapped == nil {
		t.Fatal("Unwrap() returned nil")
	}
	if unwrapped.Error() != "root cause" {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, inner)
	}

	// Verify errors.Is works
	if !errors.Is(err, inner) {
		t.Error("errors.Is(err, inner) should be true")
	}
}

func TestAppError_UnwrapNil(t *testing.T) {
	err := NewNotFoundError("nope")
	if unwrapped := err.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() should be nil for errors without inner error, got %v", unwrapped)
	}
}

func TestConstructors(t *testing.T) {
	tests := []struct {
		name       string
		construct  func() *AppError
		wantCode   string
		wantStatus int
	}{
		{
			name:       "NotFoundError",
			construct:  func() *AppError { return NewNotFoundError("not found") },
			wantCode:   "NOT_FOUND",
			wantStatus: 404,
		},
		{
			name:       "ValidationError",
			construct:  func() *AppError { return NewValidationError("bad input", nil) },
			wantCode:   "VALIDATION_ERROR",
			wantStatus: 400,
		},
		{
			name:       "InternalError",
			construct:  func() *AppError { return NewInternalError(errors.New("oops")) },
			wantCode:   "INTERNAL_ERROR",
			wantStatus: 500,
		},
		{
			name:       "ServiceUnavailableError",
			construct:  func() *AppError { return NewServiceUnavailableError("down") },
			wantCode:   "SERVICE_UNAVAILABLE",
			wantStatus: 503,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.construct()
			if err.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", err.Code, tc.wantCode)
			}
			if err.HTTPStatus != tc.wantStatus {
				t.Errorf("HTTPStatus = %d, want %d", err.HTTPStatus, tc.wantStatus)
			}
			if err.Message == "" {
				t.Error("Message should not be empty")
			}
		})
	}
}

func TestValidationError_Details(t *testing.T) {
	details := map[string]string{"field": "title", "reason": "required"}
	err := NewValidationError("invalid request", details)

	data, err2 := json.Marshal(err)
	if err2 != nil {
		t.Fatalf("json.Marshal failed: %v", err2)
	}

	var decoded map[string]any
	if err3 := json.Unmarshal(data, &decoded); err3 != nil {
		t.Fatalf("json.Unmarshal failed: %v", err3)
	}

	if decoded["code"] != "VALIDATION_ERROR" {
		t.Errorf("code = %v, want VALIDATION_ERROR", decoded["code"])
	}
	if decoded["message"] != "invalid request" {
		t.Errorf("message = %v, want invalid request", decoded["message"])
	}
	det, ok := decoded["details"].(map[string]any)
	if !ok {
		t.Fatal("details should be present in JSON output")
	}
	if det["field"] != "title" {
		t.Errorf("details.field = %v, want title", det["field"])
	}
}
