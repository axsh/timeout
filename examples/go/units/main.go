package main

import (
	"context"
	"fmt"
	"time"

	"github.com/axsh/timeout"
)

func main() {
	cfg := timeout.New(timeout.UnitLimit(500 * time.Millisecond))
	result := timeout.Run(context.Background(), cfg, func(exec timeout.Execution) error {
		return exec.Unit("download", func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
	})
	name := ""
	if result.TimeoutUnit != nil {
		name = result.TimeoutUnit.Name
	}
	fmt.Println(result.Kind, name)
}
