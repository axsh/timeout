package main

import (
	"context"
	"fmt"
	"time"

	"github.com/axsh/timeout"
)

func main() {
	cfg := timeout.New(
		timeout.Hard(30*time.Second),
		timeout.Idle(10*time.Second),
		timeout.Stall(10*time.Second),
	)
	result := timeout.Run(context.Background(), cfg, func(exec timeout.Execution) error {
		for current := int64(1); current <= 3; current++ {
			exec.Progress(timeout.Progress{
				Stage:   "import-users",
				Current: current,
				Total:   3,
				Message: "importing users",
			})
		}
		return nil
	})
	fmt.Println(result.Status)
}
