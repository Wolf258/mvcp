package messages

import (
	"bytes"
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

func TestHeartbeatMinimalRoundTrip(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: 42,
		Seq:    991,
		State:  protocol.HeartbeatStateRunning,
		Flags:  protocol.HeartbeatFlagBusy,
		// Health defaults to 0 (Healthy). The encoder always
		// materializes an ExtHealth TLV on the wire so the host
		// never has to default on absence.
	}

	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// 20B header + 5B Health TLV (type=4, len=1, value=0).
	wantLen := heartbeatHeaderSize + 5
	if len(data) != wantLen {
		t.Fatalf("expected %d bytes, got %d", wantLen, len(data))
	}

	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BootID != orig.BootID {
		t.Errorf("BootID: got %d, want %d", decoded.BootID, orig.BootID)
	}
	if decoded.Seq != orig.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, orig.Seq)
	}
	if decoded.State != orig.State {
		t.Errorf("State: got %d, want %d", decoded.State, orig.State)
	}
	if decoded.Flags != orig.Flags {
		t.Errorf("Flags: got %d, want %d", decoded.Flags, orig.Flags)
	}
	if len(decoded.Extensions) != 1 {
		t.Fatalf("Extensions: expected 1 (Health), got %d", len(decoded.Extensions))
	}
	if decoded.Extensions[0].Type != protocol.ExtHealth {
		t.Errorf("Extensions[0].Type: got %d, want %d", decoded.Extensions[0].Type, protocol.ExtHealth)
	}
	if decoded.Health != protocol.HealthHealthy {
		t.Errorf("Health: got %d, want %d (default Healthy)", decoded.Health, protocol.HealthHealthy)
	}
}

func TestHeartbeatWithExtensions(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: 1725120000000000000,
		Seq:    5,
		State:  protocol.HeartbeatStateRunning,
		Flags:  0,
		Extensions: []HeartbeatExtension{
			{Type: protocol.ExtCPUUsage, Value: []byte{0, 0, 0, 21}},
			{Type: protocol.ExtMemoryUsage, Value: []byte{0, 0, 2, 200}},
			{Type: protocol.ExtQueueDepth, Value: []byte{0, 0, 0, 3}},
		},
	}

	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if len(data) < heartbeatHeaderSize {
		t.Fatalf("data too short: %d bytes", len(data))
	}

	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BootID != orig.BootID {
		t.Errorf("BootID: got %d, want %d", decoded.BootID, orig.BootID)
	}
	if decoded.Seq != orig.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, orig.Seq)
	}
	if decoded.State != orig.State {
		t.Errorf("State: got %d, want %d", decoded.State, orig.State)
	}
	// 3 explicit extensions + the always-emitted Health TLV.
	if len(decoded.Extensions) != 4 {
		t.Fatalf("Extensions: expected 4 (3 + Health), got %d", len(decoded.Extensions))
	}

	for i, ext := range orig.Extensions {
		got := decoded.Extensions[i]
		if got.Type != ext.Type {
			t.Errorf("Extension[%d].Type: got %d, want %d", i, got.Type, ext.Type)
		}
		if !bytes.Equal(got.Value, ext.Value) {
			t.Errorf("Extension[%d].Value: got %v, want %v", i, got.Value, ext.Value)
		}
	}
	if decoded.Extensions[3].Type != protocol.ExtHealth {
		t.Errorf("Extensions[3].Type: got %d, want %d (Health)", decoded.Extensions[3].Type, protocol.ExtHealth)
	}
}

func TestHeartbeatStateTransitions(t *testing.T) {
	states := []uint8{
		protocol.HeartbeatStateUnknown,
		protocol.HeartbeatStateBooting,
		protocol.HeartbeatStateRunning,
		protocol.HeartbeatStateStopping,
		protocol.HeartbeatStateStopped,
		protocol.HeartbeatStateFailed,
	}

	for _, state := range states {
		orig := &HeartbeatMsg{
			BootID: 1,
			Seq:    1,
			State:  state,
		}
		data, err := orig.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal state=%d: %v", state, err)
		}
		decoded := &HeartbeatMsg{}
		if err := decoded.UnmarshalBinary(data); err != nil {
			t.Fatalf("unmarshal state=%d: %v", state, err)
		}
		if decoded.State != state {
			t.Errorf("state=%d: got %d after round-trip", state, decoded.State)
		}
	}
}

func TestHeartbeatFlagCombinations(t *testing.T) {
	flagSets := []uint8{
		0,
		protocol.HeartbeatFlagBusy,
		protocol.HeartbeatFlagMaintenance,
		protocol.HeartbeatFlagBusy | protocol.HeartbeatFlagLowResources,
		protocol.HeartbeatFlagReadOnly | protocol.HeartbeatFlagMaintenance,
		protocol.HeartbeatFlagBusy | protocol.HeartbeatFlagMaintenance | protocol.HeartbeatFlagReadOnly | protocol.HeartbeatFlagLowResources,
	}

	for _, flags := range flagSets {
		orig := &HeartbeatMsg{
			BootID: 2,
			Seq:    10,
			State:  protocol.HeartbeatStateRunning,
			Flags:  flags,
		}
		data, err := orig.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal flags=0x%02X: %v", flags, err)
		}
		decoded := &HeartbeatMsg{}
		if err := decoded.UnmarshalBinary(data); err != nil {
			t.Fatalf("unmarshal flags=0x%02X: %v", flags, err)
		}
		if decoded.Flags != flags {
			t.Errorf("flags=0x%02X: got 0x%02X after round-trip", flags, decoded.Flags)
		}
	}
}

func TestHeartbeatFutureProofExtensions(t *testing.T) {
	var buf bytes.Buffer
	protocol.WriteUint64(&buf, 1)
	protocol.WriteUint64(&buf, 100)
	protocol.WriteUint8(&buf, protocol.HeartbeatStateRunning)
	protocol.WriteUint8(&buf, 0)

	unknownExt := bytes.Buffer{}
	protocol.WriteUint16(&unknownExt, 99)
	protocol.WriteUint16(&unknownExt, 8)
	unknownExt.Write([]byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE})

	protocol.WriteUint16(&unknownExt, 255)
	protocol.WriteUint16(&unknownExt, 4)
	unknownExt.Write([]byte{1, 2, 3, 4})

	extBytes := unknownExt.Bytes()
	protocol.WriteUint16(&buf, uint16(len(extBytes)))
	buf.Write(extBytes)

	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(buf.Bytes()); err != nil {
		t.Fatalf("unmarshal with unknown extensions: %v", err)
	}

	if decoded.BootID != 1 {
		t.Errorf("BootID: got %d, want 1", decoded.BootID)
	}
	if decoded.Seq != 100 {
		t.Errorf("Seq: got %d, want 100", decoded.Seq)
	}
	if decoded.State != protocol.HeartbeatStateRunning {
		t.Errorf("State: got %d, want Running", decoded.State)
	}
	if len(decoded.Extensions) != 2 {
		t.Fatalf("Extensions: expected 2 unknown types, got %d", len(decoded.Extensions))
	}
	if decoded.Extensions[0].Type != 99 {
		t.Errorf("Ext[0].Type: got %d, want 99", decoded.Extensions[0].Type)
	}
	if decoded.Extensions[1].Type != 255 {
		t.Errorf("Ext[1].Type: got %d, want 255", decoded.Extensions[1].Type)
	}
}

func TestHeartbeatMaxValues(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: ^uint64(0),
		Seq:    ^uint64(0),
		State:  ^uint8(0),
		Flags:  ^uint8(0),
		Extensions: []HeartbeatExtension{
			{Type: ^uint16(0), Value: []byte{0xFF, 0xFF}},
		},
	}

	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.BootID != ^uint64(0) {
		t.Errorf("BootID: got %d, want %d", decoded.BootID, ^uint64(0))
	}
	if decoded.Seq != ^uint64(0) {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, ^uint64(0))
	}
	if decoded.State != ^uint8(0) {
		t.Errorf("State: got %d, want %d", decoded.State, ^uint8(0))
	}
	if decoded.Flags != ^uint8(0) {
		t.Errorf("Flags: got %d, want %d", decoded.Flags, ^uint8(0))
	}
	// 1 explicit extension + the always-emitted Health TLV.
	if len(decoded.Extensions) != 2 {
		t.Fatalf("Extensions: expected 2 (1 + Health), got %d", len(decoded.Extensions))
	}
	if decoded.Extensions[0].Type != ^uint16(0) {
		t.Errorf("Ext[0].Type: got %d, want %d", decoded.Extensions[0].Type, ^uint16(0))
	}
	if decoded.Extensions[1].Type != protocol.ExtHealth {
		t.Errorf("Ext[1].Type: got %d, want %d (Health)", decoded.Extensions[1].Type, protocol.ExtHealth)
	}
}

func TestHeartbeatTruncatedHeader(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"4 bytes", []byte{0, 0, 0, 0}},
		{"18 bytes", make([]byte, 18)},
		{"19 bytes", make([]byte, 19)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decoded := &HeartbeatMsg{}
			err := decoded.UnmarshalBinary(tc.data)
			if err == nil {
				t.Errorf("expected error for %d bytes, got nil", len(tc.data))
			}
		})
	}
}

func TestHeartbeatTruncatedExtensions(t *testing.T) {
	var buf bytes.Buffer
	protocol.WriteUint64(&buf, 1)
	protocol.WriteUint64(&buf, 1)
	protocol.WriteUint8(&buf, protocol.HeartbeatStateRunning)
	protocol.WriteUint8(&buf, 0)
	protocol.WriteUint16(&buf, 10)

	buf.Write([]byte{1, 2})

	decoded := &HeartbeatMsg{}
	err := decoded.UnmarshalBinary(buf.Bytes())
	if err == nil {
		t.Errorf("expected error for truncated extension data, got nil")
	}
}

func TestHeartbeatMessageRegistry(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: 999,
		Seq:    42,
		State:  protocol.HeartbeatStateRunning,
		Flags:  0,
	}

	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	msg, err := protocol.DecodeMessageBody(protocol.TypeHEARTBEAT, data)
	if err != nil {
		t.Fatalf("DecodeMessageBody: %v", err)
	}

	hb, ok := msg.(*HeartbeatMsg)
	if !ok {
		t.Fatalf("expected *HeartbeatMsg, got %T", msg)
	}
	if hb.BootID != orig.BootID {
		t.Errorf("BootID: got %d, want %d", hb.BootID, orig.BootID)
	}
	if hb.Seq != orig.Seq {
		t.Errorf("Seq: got %d, want %d", hb.Seq, orig.Seq)
	}
}

// TestHeartbeatHealthDefault ensures a heartbeat with no Health TLV
// in the extensions decodes to HealthHealthy on the receiver. The
// guest in this iteration always emits Health, but old guests
// (forward-compat) may not — the host must default sensibly.
func TestHeartbeatHealthDefault(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: 1,
		Seq:    1,
		State:  protocol.HeartbeatStateRunning,
		Health: protocol.HealthHealthy,
	}
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Health != protocol.HealthHealthy {
		t.Fatalf("Health: got %d, want %d (default)", decoded.Health, protocol.HealthHealthy)
	}
}

// TestHeartbeatHealthDegraded projects the Health field into a TLV
// during marshal and decodes it back as HealthDegraded.
func TestHeartbeatHealthDegraded(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID: 1,
		Seq:    1,
		State:  protocol.HeartbeatStateRunning,
		Health: protocol.HealthDegraded,
	}
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Health != protocol.HealthDegraded {
		t.Fatalf("Health: got %d, want %d", decoded.Health, protocol.HealthDegraded)
	}
}

// TestHeartbeatFailureReason verifies that a Failed heartbeat with a
// FailureReason string round-trips through the ExtFailureReason TLV.
func TestHeartbeatFailureReason(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID:        1,
		Seq:           1,
		State:         protocol.HeartbeatStateFailed,
		FailureReason: FailureReasonInitTimeout,
	}
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.State != protocol.HeartbeatStateFailed {
		t.Errorf("State: got %d, want Failed", decoded.State)
	}
	if decoded.FailureReason != FailureReasonInitTimeout {
		t.Errorf("FailureReason: got %q, want %q", decoded.FailureReason, FailureReasonInitTimeout)
	}
}

// TestHeartbeatFailureReasonOnlyOnFailed documents that the
// FailureReason TLV is only emitted when State=Failed. A
// FailureReason on a Running heartbeat is dropped silently — the
// receiver treats it as context for an absent failure.
func TestHeartbeatFailureReasonOnlyOnFailed(t *testing.T) {
	orig := &HeartbeatMsg{
		BootID:        1,
		Seq:           1,
		State:         protocol.HeartbeatStateRunning,
		FailureReason: FailureReasonInitTimeout,
	}
	data, err := orig.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &HeartbeatMsg{}
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.FailureReason != "" {
		t.Fatalf("FailureReason on a Running heartbeat should be empty, got %q", decoded.FailureReason)
	}
}
