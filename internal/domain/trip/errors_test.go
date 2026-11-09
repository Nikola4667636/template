package trip

import (
	"errors"
	"fmt"
	"testing"
)

func TestValidationError(t *testing.T) {
	validationErr := &ValidationError{
		Field:  testPriceField,
		Reason: testPriceReason,
	}
	if got := validationErr.Error(); got != "price: must not be negative" {
		t.Errorf("Error() = %q, want price: must not be negative", got)
	}

	wrapped := fmt.Errorf("validate create trip: %w", validationErr)
	if !errors.Is(wrapped, ErrInvalidArguments) {
		t.Fatal("wrapped validation error does not match ErrInvalidArguments")
	}

	var extracted *ValidationError
	if !errors.As(wrapped, &extracted) {
		t.Fatal("cannot extract *ValidationError from wrapped error")
	}
	if extracted != validationErr {
		t.Fatal("errors.As did not preserve the original validation error")
	}
	if extracted.Field != testPriceField || extracted.Reason != testPriceReason {
		t.Errorf("extracted error = %+v, want price validation details", extracted)
	}
}
