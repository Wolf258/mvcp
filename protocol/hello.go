// Package protocol implements the MVCP binary wire protocol: framing,
// message encoding/decoding, and message registration.
//
// hello.go implements the HELLO handshake message and the capability
// vocabulary it carries. The wire contract is specified in
// mvcp/docs/06-negotiation.md.
package protocol

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	// maxHelloCapabilities bounds the capability list a peer may send,
	// checked before any allocation (see docs/06-negotiation.md §4.4).
	maxHelloCapabilities = 64
	// maxHelloVersionLen bounds the informational software version,
	// checked before any allocation.
	maxHelloVersionLen = 128
)

// errMalformedHello wraps any decode/validation failure of a HELLO
// body. Handshake helpers map it to an ERROR(BAD_PAYLOAD) frame once
// the wire prefix has been accepted.
var errMalformedHello = errors.New("mvcp: malformed hello")

// PeerRole identifies the component on the other end of a connection.
// It identifies the software, not a network direction.
type PeerRole uint8

const (
	// RoleUnknown is reserved and never valid on the wire.
	RoleUnknown PeerRole = 0
	// RoleCore is the shifty-core host daemon.
	RoleCore PeerRole = 1
	// RoleVHandler is the guest agent (vhandler).
	RoleVHandler PeerRole = 2
	// RoleCLI is the in-guest diagnostic CLI (shiftyctl).
	RoleCLI PeerRole = 3
)

// CapabilityID identifies a negotiable message family. The baseline
// control/status messages are NOT capabilities — every connection
// supports them unconditionally (see docs/06-negotiation.md §5).
type CapabilityID uint8

const (
	// CapabilityExec covers EXEC/EXECSTREAM/EXECRESULT. Rev 2 adds
	// streaming (FlagExecStreaming + STARTED(stream=true) + chunks).
	CapabilityExec CapabilityID = 0x01
	// CapabilityTools covers TOOLCALL/TOOLRESULT/LISTTOOLS/LISTTOOLSRESULT.
	CapabilityTools CapabilityID = 0x02
	// CapabilityEvents covers the EVENT* family on port 9002.
	CapabilityEvents CapabilityID = 0x03
	// CapabilityFileTransfer covers XFERINIT/XFERCHUNK/XFERDONE on port 9004.
	CapabilityFileTransfer CapabilityID = 0x04
	// CapabilitySyncFS covers SYNCFILESYSTEMS/SYNCFILESYSTEMSACK.
	CapabilitySyncFS CapabilityID = 0x05
)

// CapabilitySupport is the revision range a peer supports for a
// capability. Revisions are monotonic: supporting N implies supporting
// every revision in [MinRevision, N].
type CapabilitySupport struct {
	MinRevision uint16
	MaxRevision uint16
}

// AdvertisedCapabilities is a peer's full capability table.
type AdvertisedCapabilities map[CapabilityID]CapabilitySupport

// Capability is one entry in the wire HELLO list.
type Capability struct {
	ID          CapabilityID
	MinRevision uint16
	MaxRevision uint16
}

// Hello is the first MVCP frame after the wire prefix (type=0x00,
// flags=0x00, msg_id=0). SoftwareVersion is informational only and is
// never used to decide compatibility.
type Hello struct {
	Role            PeerRole
	SoftwareVersion string
	Capabilities    []Capability
}

// DefaultCapabilities is the table advertised by both core and vhandler
// in the common case (same build). A side may advertise a narrower or
// wider table when software versions diverge.
var DefaultCapabilities = AdvertisedCapabilities{
	CapabilityExec:         {MinRevision: 1, MaxRevision: 2}, // rev 2 = streaming
	CapabilityTools:        {MinRevision: 1, MaxRevision: 1},
	CapabilityEvents:       {MinRevision: 1, MaxRevision: 1},
	CapabilityFileTransfer: {MinRevision: 1, MaxRevision: 1},
	CapabilitySyncFS:       {MinRevision: 1, MaxRevision: 1},
}

// NewHello builds a Hello from a role, an informational software
// version, and a capability table. Capabilities are sorted by ID for
// deterministic wire output.
func NewHello(role PeerRole, softwareVersion string, caps AdvertisedCapabilities) *Hello {
	cs := make([]Capability, 0, len(caps))
	for id, s := range caps {
		cs = append(cs, Capability{ID: id, MinRevision: s.MinRevision, MaxRevision: s.MaxRevision})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	return &Hello{Role: role, SoftwareVersion: softwareVersion, Capabilities: cs}
}

// advertised converts the wire list back into a table for negotiation.
func (h *Hello) advertised() AdvertisedCapabilities {
	out := make(AdvertisedCapabilities, len(h.Capabilities))
	for _, c := range h.Capabilities {
		out[c.ID] = CapabilitySupport{MinRevision: c.MinRevision, MaxRevision: c.MaxRevision}
	}
	return out
}

// MarshalBinary encodes the HELLO body (big-endian, existing
// primitives):
//
//	uint8  role
//	string software_version     (uint16 len + bytes, ≤ 128)
//	uint16 capability_count     (≤ 64)
//	repeat count ×: { uint8 id; uint16 min_revision; uint16 max_revision }
//
// Capability entries are sorted by ID ascending.
func (h *Hello) MarshalBinary() ([]byte, error) {
	if h.Role == RoleUnknown || h.Role > RoleCLI {
		return nil, fmt.Errorf("mvcp: hello: invalid role %d", h.Role)
	}
	if len(h.SoftwareVersion) > maxHelloVersionLen {
		return nil, fmt.Errorf("mvcp: hello: software version too long (%d bytes, max %d)", len(h.SoftwareVersion), maxHelloVersionLen)
	}
	if len(h.Capabilities) > maxHelloCapabilities {
		return nil, fmt.Errorf("mvcp: hello: too many capabilities (%d, max %d)", len(h.Capabilities), maxHelloCapabilities)
	}

	cs := make([]Capability, len(h.Capabilities))
	copy(cs, h.Capabilities)
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })

	var buf bytes.Buffer
	if err := WriteUint8(&buf, uint8(h.Role)); err != nil {
		return nil, err
	}
	if err := WriteString(&buf, h.SoftwareVersion); err != nil {
		return nil, err
	}
	if err := WriteUint16(&buf, uint16(len(cs))); err != nil {
		return nil, err
	}
	for _, c := range cs {
		if c.MinRevision > c.MaxRevision {
			return nil, fmt.Errorf("mvcp: hello: capability 0x%02X min > max", uint8(c.ID))
		}
		if err := WriteUint8(&buf, uint8(c.ID)); err != nil {
			return nil, err
		}
		if err := WriteUint16(&buf, c.MinRevision); err != nil {
			return nil, err
		}
		if err := WriteUint16(&buf, c.MaxRevision); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary decodes and strictly validates a HELLO body:
// version length ≤ 128 and capability count ≤ 64 are enforced before
// allocating, IDs must be unique, min ≤ max, and the role must be a
// known enum value (RoleUnknown 0 is well-formed and rejected later by
// the handshake's role check). Trailing bytes are rejected.
func (h *Hello) UnmarshalBinary(data []byte) error {
	malformed := func(err error) error {
		return fmt.Errorf("%w: %v", errMalformedHello, err)
	}

	r := bytes.NewReader(data)

	role, err := ReadUint8(r)
	if err != nil {
		return malformed(err)
	}
	if role > uint8(RoleCLI) {
		return fmt.Errorf("%w: invalid role %d", errMalformedHello, role)
	}

	verLen, err := ReadUint16(r)
	if err != nil {
		return malformed(err)
	}
	if verLen > maxHelloVersionLen {
		return fmt.Errorf("%w: software version too long (%d bytes, max %d)", errMalformedHello, verLen, maxHelloVersionLen)
	}
	verBuf := make([]byte, verLen)
	if _, err := io.ReadFull(r, verBuf); err != nil {
		return malformed(err)
	}

	count, err := ReadUint16(r)
	if err != nil {
		return malformed(err)
	}
	if count > maxHelloCapabilities {
		return fmt.Errorf("%w: too many capabilities (%d, max %d)", errMalformedHello, count, maxHelloCapabilities)
	}
	seen := make(map[CapabilityID]struct{}, count)
	cs := make([]Capability, 0, count)
	for i := 0; i < int(count); i++ {
		id, err := ReadUint8(r)
		if err != nil {
			return malformed(err)
		}
		minRev, err := ReadUint16(r)
		if err != nil {
			return malformed(err)
		}
		maxRev, err := ReadUint16(r)
		if err != nil {
			return malformed(err)
		}
		cid := CapabilityID(id)
		if _, dup := seen[cid]; dup {
			return fmt.Errorf("%w: duplicate capability 0x%02X", errMalformedHello, cid)
		}
		seen[cid] = struct{}{}
		if minRev > maxRev {
			return fmt.Errorf("%w: capability 0x%02X min > max", errMalformedHello, cid)
		}
		cs = append(cs, Capability{ID: cid, MinRevision: minRev, MaxRevision: maxRev})
	}

	if r.Len() != 0 {
		return fmt.Errorf("%w: %d trailing bytes", errMalformedHello, r.Len())
	}

	h.Role = PeerRole(role)
	h.SoftwareVersion = string(verBuf)
	h.Capabilities = cs
	return nil
}

func init() {
	RegisterMessage(TypeHELLO, func(r io.Reader) (Message, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		h := &Hello{}
		if err := h.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return h, nil
	})
}
