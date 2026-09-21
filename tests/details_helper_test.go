package tests

import (
	"context"
	"testing"

	"github.com/axsh/timeout"
)

func runWithChanDetails(t *testing.T) string {
	t.Helper()
	res := timeout.Run(context.Background(), timeout.New(), func(exec timeout.Execution) error {
		exec.Progress(timeout.Progress{
			Stage:   "x",
			Message: "m",
			Details: make(chan int),
		})
		raw, ok := timeout.MarshalDetailsJSON(make(chan int))
		if ok || raw != nil {
			return errString("marshal should fail")
		}
		return nil
	})
	if res.Status != timeout.StatusSucceeded {
		return "status=" + string(res.Status)
	}
	return "ok"
}

type errString string

func (e errString) Error() string { return string(e) }
