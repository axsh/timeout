package protocol

import (
	"fmt"
	"os"
	"strconv"
)

const (
	// DefaultMaxLineBytes is the default NDJSON line size limit (1 MiB).
	DefaultMaxLineBytes = 1 << 20
	// MinMaxLineBytes is the minimum accepted override.
	MinMaxLineBytes = 4096
	envMaxLineBytes = "TIMEOUTX_MAX_LINE_BYTES"
)

// MaxLineBytes is the default line limit (kept for compatibility).
const MaxLineBytes = DefaultMaxLineBytes

// EffectiveMaxLineBytes returns the active line size limit.
// TIMEOUTX_MAX_LINE_BYTES overrides the default when set to a valid integer >= 4096.
func EffectiveMaxLineBytes() (int, error) {
	v := os.Getenv(envMaxLineBytes)
	if v == "" {
		return DefaultMaxLineBytes, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("protocol: invalid %s: %w", envMaxLineBytes, err)
	}
	if n < MinMaxLineBytes {
		return 0, fmt.Errorf("protocol: %s must be >= %d", envMaxLineBytes, MinMaxLineBytes)
	}
	return n, nil
}
