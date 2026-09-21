package protocol

import "testing"

func TestDecodeHeartbeat(t *testing.T) {
	env, err := DecodeLine([]byte(`{"v":1,"type":"heartbeat"}`))
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "heartbeat" {
		t.Fatalf("type=%s", env.Type)
	}
}

func TestDecodeRejectsBadVersion(t *testing.T) {
	_, err := DecodeLine([]byte(`{"v":99,"type":"heartbeat"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEncodeRoundTripProgress(t *testing.T) {
	cur, tot := int64(3), int64(10)
	b, err := EncodeProgress("import", "msg", &cur, &tot, map[string]any{"file": "a.csv"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := DecodeLine(b)
	if err != nil {
		t.Fatal(err)
	}
	if env.Stage != "import" || env.Current == nil || *env.Current != 3 {
		t.Fatalf("%+v", env)
	}
}
