package trip

import "errors"

var (
	ErrTripNotFound     = errors.New("trip not found")
	ErrDriverBusy       = errors.New("driver already has active trip")
	ErrTripCompleted    = errors.New("trip is already completed")
	ErrInvalidArguments = errors.New("invalid input arguments")
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Reason
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalidArguments
}
