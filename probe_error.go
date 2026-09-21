package timeout

import "errors"

// ErrProbeFailure marks a failed command probe that should stop the execution.
var ErrProbeFailure = errors.New("timeout: probe failure")

func isProbeFailure(err error) bool {
	return errors.Is(err, ErrProbeFailure)
}

// ProbeFailure wraps err as a probe failure for Result.Kind=probe.
func ProbeFailure(err error) error {
	if err == nil {
		return ErrProbeFailure
	}
	return errors.Join(ErrProbeFailure, err)
}
