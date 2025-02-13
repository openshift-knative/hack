package errors

import (
	"fmt"
	"io"
)

func Rewrap(err error) error {
	if _, ok := err.(interface{ Format(s fmt.State, verb rune) }); ok {
		return err
	}
	if m, ok := err.(interface{ Unwrap() []error }); ok {
		errs := m.Unwrap()
		for i := 0; i < len(errs); i++ {
			errs[i] = Rewrap(errs[i])
		}
		return &wrapErrors{
			msg:  err.Error(),
			errs: errs,
		}
	}
	if m, ok := err.(interface{ Unwrap() error }); ok {
		return &wrapError{
			msg: err.Error(),
			err: Rewrap(m.Unwrap()),
		}
	}
	return err
}

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}

func (e *wrapError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, e.msg)
			_, _ = fmt.Fprintf(s, "\n\n - %+v", e.err)
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, e.msg)
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", e.msg)
	}
}

type wrapErrors struct {
	msg  string
	errs []error
}

func (e *wrapErrors) Error() string {
	return e.msg
}

func (e *wrapErrors) Unwrap() []error {
	return e.errs
}

func (e *wrapErrors) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = io.WriteString(s, e.msg)
			for _, err := range e.errs {
				_, _ = fmt.Fprintf(s, "\n\n - %+v", err)
			}
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, e.msg)
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", e.msg)
	}
}
