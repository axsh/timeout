package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const Version = 1

// MaxLineBytes is the provisional maximum size of one NDJSON control message.
const MaxLineBytes = 1 << 20

// Envelope is one control-protocol message.
type Envelope struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	Message string          `json:"message,omitempty"`
	Stage   string          `json:"stage,omitempty"`
	Current *int64          `json:"current,omitempty"`
	Total   *int64          `json:"total,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
	ID      uint64          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
}

// DecodeLine parses one NDJSON line.
func DecodeLine(line []byte) (Envelope, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Envelope{}, fmt.Errorf("protocol: empty line")
	}
	if len(line) > MaxLineBytes {
		return Envelope{}, fmt.Errorf("protocol: line exceeds %d bytes", MaxLineBytes)
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("protocol: invalid json: %w", err)
	}
	if env.V != Version {
		return Envelope{}, fmt.Errorf("protocol: unsupported version %d", env.V)
	}
	if env.Type == "" {
		return Envelope{}, fmt.Errorf("protocol: missing type")
	}
	return env, nil
}

// EncodeHeartbeat returns a heartbeat message line (without trailing newline).
func EncodeHeartbeat() []byte {
	return []byte(`{"v":1,"type":"heartbeat"}`)
}

// EncodeStatus returns a status message line.
func EncodeStatus(message string) ([]byte, error) {
	return json.Marshal(Envelope{V: Version, Type: "status", Message: message})
}

// EncodeProgress returns a progress message line.
func EncodeProgress(stage, message string, current, total *int64, details any) ([]byte, error) {
	env := Envelope{V: Version, Type: "progress", Stage: stage, Message: message, Current: current, Total: total}
	if details != nil {
		raw, err := json.Marshal(details)
		if err != nil {
			return nil, err
		}
		env.Details = raw
	}
	return json.Marshal(env)
}

// EncodeUnitBegin returns a unit_begin message line.
func EncodeUnitBegin(id uint64, name string) ([]byte, error) {
	return json.Marshal(Envelope{V: Version, Type: "unit_begin", ID: id, Name: name})
}

// EncodeUnitEnd returns a unit_end message line.
func EncodeUnitEnd(id uint64) ([]byte, error) {
	return json.Marshal(Envelope{V: Version, Type: "unit_end", ID: id})
}

// Handler consumes decoded envelopes.
type Handler func(Envelope) error

// ReadLoop reads NDJSON lines from r and invokes handler.
// Malformed lines invoke onError and continue when onError returns nil.
func ReadLoop(r io.Reader, handler Handler, onError func(error)) error {
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, MaxLineBytes+1)
	for sc.Scan() {
		env, err := DecodeLine(sc.Bytes())
		if err != nil {
			if onError != nil {
				onError(err)
			}
			continue
		}
		if err := handler(env); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}
