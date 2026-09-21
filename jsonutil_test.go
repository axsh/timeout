package timeout

import "testing"

func TestMarshalDetailsJSON_OK(t *testing.T) {
	raw, ok := MarshalDetailsJSON(map[string]any{"file": "a.csv"})
	if !ok || len(raw) == 0 {
		t.Fatalf("ok=%v raw=%s", ok, raw)
	}
}

func TestMarshalDetailsJSON_Nil(t *testing.T) {
	raw, ok := MarshalDetailsJSON(nil)
	if !ok || raw != nil {
		t.Fatalf("ok=%v raw=%v", ok, raw)
	}
}

func TestMarshalDetailsJSON_ChanFails(t *testing.T) {
	_, ok := MarshalDetailsJSON(make(chan int))
	if ok {
		t.Fatal("expected failure")
	}
}

func TestMarshalDetailsJSON_FuncFails(t *testing.T) {
	_, ok := MarshalDetailsJSON(func() {})
	if ok {
		t.Fatal("expected failure")
	}
}
