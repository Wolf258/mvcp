// events_test.go - mvcp
//
// Round-trip tests for event message types. The wire format is
// the contract between the guest (vhandler) and the host
// (shifty-core + shifty-cli): every event must survive
// Marshal/Unmarshal without losing fields, and the registered
// type-byte → decoder must resolve to the right struct.

package messages

import (
	"bytes"
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

// TestEventReadyRoundTrip covers the canonical event the guest
// emits once after /init.sh (or defaultInit) returns nil.
// String-only payload (version) — same shape as the other
// event types so a single round-trip pattern covers them all.
func TestEventReadyRoundTrip(t *testing.T) {
	orig := &EventReady{Version: "0.3.0"}

	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded, err := decodeEventFrame(t, protocol.TypeEVENTREADY, data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := decoded.(*EventReady)
	if !ok {
		t.Fatalf("decoded type: got %T, want *EventReady", decoded)
	}
	if got.Version != orig.Version {
		t.Errorf("Version: got %q, want %q", got.Version, orig.Version)
	}
}

// TestEventInitFailedRoundTrip covers the new event the guest
// emits once when /init.sh (or defaultInit) fails, panics, or
// hits the init timeout. Two-string payload (version + reason)
// — the reason is the same machine-readable vocabulary the
// heartbeat's ExtFailureReason TLV uses
// (FailureReasonInitTimeout / FailureReasonInitPanic /
// FailureReasonInternal / etc.), so a host can reuse its
// reason→error mapping.
func TestEventInitFailedRoundTrip(t *testing.T) {
	cases := []EventInitFailed{
		{Version: "0.3.0", Reason: "init_timeout"},
		{Version: "0.3.0", Reason: "init_panic"},
		{Version: "0.3.0", Reason: "internal"},
		{Version: "0.3.0", Reason: ""}, // empty reason still round-trips
	}
	for _, orig := range cases {
		data, err := orig.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal reason=%q: %v", orig.Reason, err)
		}
		decoded, err := decodeEventFrame(t, protocol.TypeEVENTINITFAILED, data)
		if err != nil {
			t.Fatalf("decode reason=%q: %v", orig.Reason, err)
		}
		got, ok := decoded.(*EventInitFailed)
		if !ok {
			t.Fatalf("decoded type reason=%q: got %T, want *EventInitFailed", orig.Reason, decoded)
		}
		if got.Version != orig.Version {
			t.Errorf("Version reason=%q: got %q, want %q", orig.Reason, got.Version, orig.Version)
		}
		if got.Reason != orig.Reason {
			t.Errorf("Reason: got %q, want %q", got.Reason, orig.Reason)
		}
	}
}

// TestEventInitFailedRegistrationType guards against accidental
// drift between protocol.TypeEVENTINITFAILED and the message
// registry: if the byte changes, this test fires so the
// decoder in init() is updated in lockstep.
func TestEventInitFailedRegistrationType(t *testing.T) {
	if protocol.TypeEVENTINITFAILED != 0x86 {
		t.Fatalf("TypeEVENTINITFAILED: got 0x%02X, want 0x86 (reserved in events.md range 0x85-0x8F)", protocol.TypeEVENTINITFAILED)
	}
}

// decodeEventFrame routes a raw body through the protocol
// registry by the given type byte. Mirrors the production
// dispatch path so a wrong decoder wiring would surface here
// rather than in the vhandler.
func decodeEventFrame(t *testing.T, typeByte uint8, body []byte) (interface{}, error) {
	t.Helper()
	frame := &protocol.Frame{Type: typeByte, Flags: 0, MsgID: 0, Body: body}
	var buf bytes.Buffer
	if err := protocol.WriteMVCPFrame(&buf, frame); err != nil {
		t.Fatalf("write mvcp frame: %v", err)
	}
	decoded, err := protocol.ReadMVCPFrame(&buf)
	if err != nil {
		return nil, err
	}
	return protocol.DecodeMessage(decoded.Type, bytes.NewReader(decoded.Body))
}
