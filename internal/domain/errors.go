package domain

import "errors"

var (
	ErrNotFound  = errors.New("resource not found")
	ErrConflict  = errors.New("state conflict")
	ErrInvalid   = errors.New("invalid input")
	ErrForbidden = errors.New("operation not allowed")
)
