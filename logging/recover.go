package logging

import "fmt"

// RecoverError recovers from a panic and returns an error in that case
func RecoverError() error {
	if r := recover(); r != nil {
		if e, isError := r.(error); isError {
			return e
		}
		return fmt.Errorf("panic: %#v", r)
	}
	return nil
}

// CatchPanic calls a function, returning any panic as error
func CatchPanic(f func()) (err error) {
	defer func() { err = RecoverError() }()
	f()
	return
}
