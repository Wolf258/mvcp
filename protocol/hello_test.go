package protocol

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestHelloRoundTrip(t *testing.T) {
	hello := NewHello(RoleVHandler, "0.8.2-dev+17a39d", DefaultCapabilities)
	data, err := hello.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Hello
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != RoleVHandler {
		t.Errorf("role = %d, want %d", got.Role, RoleVHandler)
	}
	if got.SoftwareVersion != "0.8.2-dev+17a39d" {
		t.Errorf("version = %q", got.SoftwareVersion)
	}
	if len(got.Capabilities) != len(DefaultCapabilities) {
		t.Fatalf("capabilities = %d, want %d", len(got.Capabilities), len(DefaultCapabilities))
	}
	for i, c := range got.Capabilities {
		if c.ID != CapabilityID(i+1) {
			t.Errorf("capability[%d].ID = 0x%02X, want 0x%02X (sorted)", i, c.ID, i+1)
		}
	}
}

// TestHelloWireSorted verifies MarshalBinary sorts entries by ID even
// when the caller provides them unsorted.
func TestHelloWireSorted(t *testing.T) {
	h := &Hello{
		Role:            RoleCore,
		SoftwareVersion: "v1",
		Capabilities: []Capability{
			{ID: CapabilitySyncFS, MinRevision: 1, MaxRevision: 1},
			{ID: CapabilityExec, MinRevision: 1, MaxRevision: 2},
		},
	}
	data, err := h.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Hello
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Capabilities[0].ID != CapabilityExec || got.Capabilities[1].ID != CapabilitySyncFS {
		t.Fatalf("wire order not sorted by ID: %v", got.Capabilities)
	}
}

func TestHelloRejections(t *testing.T) {
	caps := func(n int) []Capability {
		cs := make([]Capability, n)
		for i := range cs {
			cs[i] = Capability{ID: CapabilityID(i + 1), MinRevision: 1, MaxRevision: 1}
		}
		return cs
	}
	cases := []struct {
		name string
		data []byte
	}{
		{
			name: "too many capabilities",
			data: rawHello(1, "v", caps(maxHelloCapabilities+1)),
		},
		{
			name: "version too long",
			data: rawHello(1, strings.Repeat("x", maxHelloVersionLen+1), nil),
		},
		{
			name: "min greater than max",
			data: rawHello(1, "v", []Capability{{ID: CapabilityExec, MinRevision: 2, MaxRevision: 1}}),
		},
		{
			name: "duplicate capability id",
			data: rawHello(1, "v", []Capability{
				{ID: CapabilityExec, MinRevision: 1, MaxRevision: 1},
				{ID: CapabilityExec, MinRevision: 2, MaxRevision: 2},
			}),
		},
		{
			name: "unknown role enum value",
			data: []byte{4, 0, 1, 'v', 0, 0}, // role=4, version "v", count=0
		},
		{
			name: "truncated body",
			data: []byte{1, 0, 5, 'h'}, // claims 5-byte version, only 1 present
		},
		{
			name: "trailing bytes",
			data: append(rawHello(1, "v", nil), 0xFF),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h Hello
			err := h.UnmarshalBinary(tc.data)
			if err == nil {
				t.Fatal("expected malformed error, got nil")
			}
			if !errors.Is(err, errMalformedHello) {
				t.Fatalf("error %v does not wrap errMalformedHello", err)
			}
		})
	}
}

// TestHelloRoleUnknownWellFormed: role 0 (RoleUnknown) is well-formed on
// the wire; the handshake layer rejects it with UNEXPECTED_ROLE instead.
func TestHelloRoleUnknownWellFormed(t *testing.T) {
	data := rawHello(0, "v", nil)
	var h Hello
	if err := h.UnmarshalBinary(data); err != nil {
		t.Fatalf("role 0 should decode fine, got: %v", err)
	}
	if h.Role != RoleUnknown {
		t.Fatalf("role = %d, want 0", h.Role)
	}
}

func TestHelloMarshalRejectsInvalidRole(t *testing.T) {
	_, err := (&Hello{Role: RoleUnknown, SoftwareVersion: "v"}).MarshalBinary()
	if err == nil {
		t.Fatal("expected error for RoleUnknown, got nil")
	}
}

// rawHello hand-encodes a HELLO body, bypassing MarshalBinary so tests
// can exercise decoder limits that the encoder already rejects.
func rawHello(role uint8, version string, caps []Capability) []byte {
	var buf bytes.Buffer
	buf.WriteByte(role)
	WriteUint16(&buf, uint16(len(version)))
	buf.WriteString(version)
	WriteUint16(&buf, uint16(len(caps)))
	for _, c := range caps {
		buf.WriteByte(uint8(c.ID))
		WriteUint16(&buf, c.MinRevision)
		WriteUint16(&buf, c.MaxRevision)
	}
	return buf.Bytes()
}
