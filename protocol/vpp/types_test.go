package vpp

import (
	"bytes"
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

func TestAttachMsgRoundTripWithSessionID(t *testing.T) {
	in := &AttachMsg{Term: "tmux-256color", Cols: 132, Rows: 43, SessionID: 7}
	var out AttachMsg
	if err := out.Decode(in.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != *in {
		t.Fatalf("round-trip = %+v, want %+v", out, *in)
	}
}

func TestAttachMsgRoundTripZeroSessionID(t *testing.T) {
	in := &AttachMsg{Term: "xterm-256color", Cols: 120, Rows: 40}
	var out AttachMsg
	if err := out.Decode(in.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.SessionID != 0 {
		t.Fatalf("session_id = %d, want 0 (join-or-create)", out.SessionID)
	}
	if out.Term != in.Term || out.Cols != in.Cols || out.Rows != in.Rows {
		t.Fatalf("round-trip = %+v, want %+v", out, *in)
	}
}

func TestAttachMsgDecodeOldHostBody(t *testing.T) {
	// A host predating the SessionID field sends term/cols/rows only.
	// The new decoder must read it as session_id=0 (join-or-create).
	var buf bytes.Buffer
	protocol.WriteString(&buf, "linux")
	protocol.WriteUint16(&buf, 80)
	protocol.WriteUint16(&buf, 24)

	var out AttachMsg
	if err := out.Decode(buf.Bytes()); err != nil {
		t.Fatalf("decode old body: %v", err)
	}
	if out.Term != "linux" || out.Cols != 80 || out.Rows != 24 || out.SessionID != 0 {
		t.Fatalf("decoded = %+v, want linux 80x24 session_id=0", out)
	}
}

func TestAttachMsgDecodeTrailingTolerance(t *testing.T) {
	// New decoder must ignore trailing bytes beyond session_id so it
	// stays forward-compatible with additive tail fields.
	in := &AttachMsg{Term: "screen", Cols: 100, Rows: 30, SessionID: 3}
	body := append(in.Encode(), 0xde, 0xad, 0xbe, 0xef)
	var out AttachMsg
	if err := out.Decode(body); err != nil {
		t.Fatalf("decode with trailing bytes: %v", err)
	}
	if out != *in {
		t.Fatalf("decoded = %+v, want %+v", out, *in)
	}
}

func TestAttachMsgDecodeTruncated(t *testing.T) {
	var out AttachMsg
	if err := out.Decode([]byte{0x00, 0x03}); err == nil {
		t.Fatal("truncated body must error")
	}
}

func TestSessionMsgRoundTrip(t *testing.T) {
	in := &SessionMsg{SessionID: 42, PID: 1337, Cols: 200, Rows: 60}
	var out SessionMsg
	if err := out.Decode(in.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != *in {
		t.Fatalf("round-trip = %+v, want %+v", out, *in)
	}
}

func TestWinchMsgRoundTrip(t *testing.T) {
	in := &WinchMsg{Cols: 180, Rows: 55}
	var out WinchMsg
	if err := out.Decode(in.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != *in {
		t.Fatalf("round-trip = %+v, want %+v", out, *in)
	}
}

func TestDetachMsgRoundTrip(t *testing.T) {
	in := &DetachMsg{ExitCode: 137}
	var out DetachMsg
	if err := out.Decode(in.Encode()); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out != *in {
		t.Fatalf("round-trip = %+v, want %+v", out, *in)
	}
}
