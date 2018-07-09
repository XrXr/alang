package errors

import "fmt"

type UserError struct {
	Line        int
	StartColumn int
	EndColumn   int
	Message     string
}

func (e *UserError) Error() string {
	return fmt.Sprintf("%d:%d %s", e.Line, e.StartColumn, e.Message)
}

func MakeError(start, end int, message string) error {
	return &UserError{Line: -1, StartColumn: start, EndColumn: end, Message: message}
}
