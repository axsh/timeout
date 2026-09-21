package main

import (
	"context"
	"fmt"
	"time"

	"github.com/axsh/timeout"
)

func main() {
	cfg := timeout.New(timeout.Stall(time.Second))
	result := timeout.Run(context.Background(), cfg, func(exec timeout.Execution) error {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-exec.Context().Done():
				return nil
			case <-ticker.C:
				exec.Heartbeat()
			}
		}
	})
	fmt.Println(result.Status, result.Kind)
}
