package errors

import (
	"fmt"
)

type ResolvingError struct {
	Err          error
	HowToResolve string
}

func NewResolvingError(err error, howToResolve string) *ResolvingError {
	return &ResolvingError{
		Err:          err,
		HowToResolve: howToResolve,
	}
}

func Wrap(parent *ResolvingError, err error, howToResolve string) *ResolvingError {
	return &ResolvingError{
		Err:          fmt.Errorf("%w: %w", err, parent.Err),
		HowToResolve: fmt.Sprintf("%s: Resolve via %s", howToResolve, parent.HowToResolve),
	}
}

func (e *ResolvingError) Error() string {
	return fmt.Sprintf("%s. To resolve: %s", e.Err.Error(), e.HowToResolve)
}

func (e *ResolvingError) AsZapLogKV() []string {
	return []string{
		"error", e.Err.Error(),
		"how_to_fix", e.HowToResolve,
	}
}
