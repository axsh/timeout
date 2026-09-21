package process

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/axsh/timeout"
)

// EventWriter writes engine events as NDJSON (never to the child stdout).
type EventWriter struct {
	w io.Writer
}

// OpenEventWriter opens an event sink.
// fd >= 0 wins over path. path "-" means stderr. empty path and fd < 0 disables.
func OpenEventWriter(path string, fd int) (*EventWriter, error) {
	if fd >= 0 {
		return &EventWriter{w: os.NewFile(uintptr(fd), "events")}, nil
	}
	if path == "" {
		return nil, nil
	}
	if path == "-" {
		return &EventWriter{w: os.Stderr}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &EventWriter{w: f}, nil
}

// WriteEvent writes one NDJSON event line.
func (e *EventWriter) WriteEvent(typ string, fields map[string]any) error {
	if e == nil || e.w == nil {
		return nil
	}
	m := map[string]any{
		"v":    1,
		"type": typ,
		"ts":   time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range fields {
		if k == "details" {
			raw, ok := timeout.MarshalDetailsJSON(v)
			if !ok {
				fmt.Fprintln(os.Stderr, "timeoutx: details_marshal_error")
				continue
			}
			if raw != nil {
				m[k] = json.RawMessage(raw)
			}
			continue
		}
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = e.w.Write(append(b, '\n'))
	return err
}

type eventObserver struct {
	w *EventWriter
}

func (o eventObserver) OnEvent(ev timeout.Event) {
	if o.w == nil {
		return
	}
	fields := map[string]any{}
	switch ev.Type {
	case timeout.EventMeaningfulProgress, timeout.EventProgress:
		fields["stage"] = ev.Snapshot.Progress.Stage
		fields["current"] = ev.Snapshot.Progress.Current
		fields["total"] = ev.Snapshot.Progress.Total
		fields["percent"] = ev.Snapshot.Progress.Percent
		fields["message"] = ev.Snapshot.Progress.Message
		_ = o.w.WriteEvent("progress", fields)
	case timeout.EventTimeout:
		fields["kind"] = string(ev.Snapshot.State) // will override below
		_ = o.w.WriteEvent("timeout", map[string]any{
			"elapsed": ev.Snapshot.Elapsed.String(),
		})
	case timeout.EventFinished:
		_ = o.w.WriteEvent("finished", map[string]any{
			"elapsed": ev.Snapshot.Elapsed.String(),
		})
	default:
		_ = o.w.WriteEvent(string(ev.Type), fields)
	}
}
