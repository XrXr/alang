package errors

import "fmt"

type UserError struct {
	Line        int
	StartColumn int
	EndColumn   int
	Message     string
}

func (e *UserError) Error() string {
	return fmt.Sprintf("%d:%d %s", e.Line+1, e.StartColumn, e.Message)
}

func MakeError(start, end int, message string) *UserError {
	return &UserError{Line: -1, StartColumn: start, EndColumn: end, Message: message}
}
