package protocol

import (
	"bytes"
	"testing"
)

func TestMaxLineBytesDefault(t *testing.T) {
	t.Setenv(envMaxLineBytes, "")
	n, err := EffectiveMaxLineBytes()
	if err != nil || n != DefaultMaxLineBytes {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestMaxLineBytesFromEnv(t *testing.T) {
	t.Setenv(envMaxLineBytes, "8192")
	n, err := EffectiveMaxLineBytes()
	if err != nil || n != 8192 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestMaxLineBytesEnvTooSmall(t *testing.T) {
	t.Setenv(envMaxLineBytes, "4095")
	_, err := EffectiveMaxLineBytes()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMaxLineBytesEnvInvalid(t *testing.T) {
	t.Setenv(envMaxLineBytes, "abc")
	_, err := EffectiveMaxLineBytes()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeLineExceedsLimit(t *testing.T) {
	payload := append([]byte(`{"v":1,"type":"status","message":"`), bytes.Repeat([]byte("x"), 100)...)
	payload = append(payload, '"', '}')
	_, err := DecodeLineWithLimit(payload, 50)
	if err == nil {
		t.Fatal("expected error")
	}
}
