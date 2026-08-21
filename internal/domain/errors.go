package domain

import "errors"

// Sentinel errors mapped by the controller layer to HTTP status codes.
var (
	// ErrNotFound signals a lookup miss. Controllers map to 404.
	ErrNotFound = errors.New("not found")

	// ErrConflict signals a uniqueness violation. Controllers map to 409.
	ErrConflict = errors.New("conflict")

	// ErrInvalidDefinition signals an invalid cohort definition or input.
	// Controllers map to 400.
	ErrInvalidDefinition = errors.New("invalid definition")
)
