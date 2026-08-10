package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type GetStatusMsg struct{}

func (m *GetStatusMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *GetStatusMsg) UnmarshalBinary([]byte) error { return nil }

type StatusMsg struct {
	Version      string
	PID          uint32
	ShuttingDown bool
}

func (m *StatusMsg) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := protocol.WriteString(&buf, m.Version); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint32(&buf, m.PID); err != nil {
		return nil, err
	}
	if err := protocol.WriteBool(&buf, m.ShuttingDown); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *StatusMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	ver, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("status: read version: %w", err)
	}
	pid, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("status: read pid: %w", err)
	}
	down, err := protocol.ReadBool(r)
	if err != nil {
		return fmt.Errorf("status: read shutting_down: %w", err)
	}
	m.Version = ver
	m.PID = pid
	m.ShuttingDown = down
	return nil
}

const heartbeatHeaderSize = 20

// HeartbeatFailureReason is a short machine-readable reason the guest
// emits in the ExtFailureReason TLV when it transitions to State=Failed.
// Empty when the heartbeat is not a failure report. Kept as a string
// (rather than a typed enum) so new reasons can be added without a
// protocol bump — the host treats unknown reasons as opaque context.
type HeartbeatFailureReason string

const (
	FailureReasonInitTimeout HeartbeatFailureReason = "init_timeout"
	FailureReasonInitPanic   HeartbeatFailureReason = "init_panic"
	FailureReasonMountFailed HeartbeatFailureReason = "mount_failed"
	FailureReasonExecFailed  HeartbeatFailureReason = "exec_failed"
	FailureReasonInternal    HeartbeatFailureReason = "internal"
)

type HeartbeatExtension struct {
	Type  uint16
	Value []byte
}

type HeartbeatMsg struct {
	BootID uint64
	Seq    uint64
	State  uint8
	Flags  uint8
	// Health is decoded from the ExtHealth TLV in Extensions.
	// Default HealthHealthy when the TLV is absent.
	Health uint8
	// FailureReason is decoded from the ExtFailureReason TLV in
	// Extensions when State=HeartbeatStateFailed. Empty otherwise.
	FailureReason HeartbeatFailureReason
	Extensions    []HeartbeatExtension
}

func (m *HeartbeatMsg) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(heartbeatHeaderSize)

	if err := protocol.WriteUint64(&buf, m.BootID); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint64(&buf, m.Seq); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint8(&buf, m.State); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint8(&buf, m.Flags); err != nil {
		return nil, err
	}

	exts := m.materializedExtensions()

	var extBuf bytes.Buffer
	if err := writeHeartbeatExtensions(&extBuf, exts); err != nil {
		return nil, err
	}
	payloadLen := uint16(extBuf.Len())
	if err := protocol.WriteUint16(&buf, payloadLen); err != nil {
		return nil, err
	}
	buf.Write(extBuf.Bytes())

	return buf.Bytes(), nil
}

// materializedExtensions returns the explicit Extensions slice with
// the Health and (when State=Failed) FailureReason TLVs projected
// in. Callers that want to keep Extensions pristine can pre-populate
// the fields instead of using the synthetic TLV path; the projection
// here is what makes Marshal/Unmarshal round-trip without forcing
// every emitter to construct TLVs by hand.
//
// Health is *always* emitted, even when 0 (Healthy). The wire
// stays self-describing: every heartbeat carries an explicit
// Health byte so the host never has to default on absence.
// FailureReason is only emitted on a Failed heartbeat and only
// when non-empty.
func (m *HeartbeatMsg) materializedExtensions() []HeartbeatExtension {
	out := make([]HeartbeatExtension, 0, len(m.Extensions)+2)
	seenHealth := false
	seenReason := false
	for _, e := range m.Extensions {
		switch e.Type {
		case protocol.ExtHealth:
			seenHealth = true
			// Replace the input TLV's value with the current
			// Health field so callers can pre-populate the TLV
			// in Extensions and the projection still wins.
			e.Value = []byte{m.Health}
		case protocol.ExtFailureReason:
			seenReason = true
			if m.FailureReason != "" {
				e.Value = []byte(m.FailureReason)
			}
		}
		out = append(out, e)
	}
	if !seenHealth {
		out = append(out, HeartbeatExtension{
			Type:  protocol.ExtHealth,
			Value: []byte{m.Health},
		})
	}
	if m.State == protocol.HeartbeatStateFailed && m.FailureReason != "" && !seenReason {
		out = append(out, HeartbeatExtension{
			Type:  protocol.ExtFailureReason,
			Value: []byte(m.FailureReason),
		})
	}
	return out
}

func (m *HeartbeatMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)

	bootID, err := protocol.ReadUint64(r)
	if err != nil {
		return fmt.Errorf("heartbeat: read boot_id: %w", err)
	}
	seq, err := protocol.ReadUint64(r)
	if err != nil {
		return fmt.Errorf("heartbeat: read seq: %w", err)
	}
	state, err := protocol.ReadUint8(r)
	if err != nil {
		return fmt.Errorf("heartbeat: read state: %w", err)
	}
	flags, err := protocol.ReadUint8(r)
	if err != nil {
		return fmt.Errorf("heartbeat: read flags: %w", err)
	}
	payloadLen, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("heartbeat: read payload_length: %w", err)
	}

	exts, err := readHeartbeatExtensions(r, payloadLen)
	if err != nil {
		return fmt.Errorf("heartbeat: read extensions: %w", err)
	}

	m.BootID = bootID
	m.Seq = seq
	m.State = state
	m.Flags = flags
	m.Health = protocol.HealthHealthy
	m.FailureReason = ""
	m.Extensions = exts

	for _, e := range exts {
		switch e.Type {
		case protocol.ExtHealth:
			if len(e.Value) >= 1 {
				m.Health = e.Value[0]
			}
		case protocol.ExtFailureReason:
			m.FailureReason = HeartbeatFailureReason(e.Value)
		}
	}
	return nil
}

func writeHeartbeatExtensions(w io.Writer, exts []HeartbeatExtension) error {
	for _, e := range exts {
		if err := protocol.WriteUint16(w, e.Type); err != nil {
			return fmt.Errorf("ext type: %w", err)
		}
		if err := protocol.WriteUint16(w, uint16(len(e.Value))); err != nil {
			return fmt.Errorf("ext length: %w", err)
		}
		if _, err := w.Write(e.Value); err != nil {
			return fmt.Errorf("ext value: %w", err)
		}
	}
	return nil
}

func readHeartbeatExtensions(r io.Reader, payloadLen uint16) ([]HeartbeatExtension, error) {
	if payloadLen == 0 {
		return nil, nil
	}
	raw := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, fmt.Errorf("read extension data: %w", err)
	}
	extReader := bytes.NewReader(raw)
	var exts []HeartbeatExtension
	for extReader.Len() >= 4 {
		extType, err := protocol.ReadUint16(extReader)
		if err != nil {
			return nil, err
		}
		extLen, err := protocol.ReadUint16(extReader)
		if err != nil {
			return nil, err
		}
		if extReader.Len() < int(extLen) {
			return nil, fmt.Errorf("heartbeat: extension type=%d declared len=%d but only %d bytes remain", extType, extLen, extReader.Len())
		}
		value := make([]byte, extLen)
		if _, err := io.ReadFull(extReader, value); err != nil {
			return nil, fmt.Errorf("heartbeat: read extension type=%d value: %w", extType, err)
		}
		exts = append(exts, HeartbeatExtension{Type: extType, Value: value})
	}
	return exts, nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeGETSTATUS, func(r io.Reader) (protocol.Message, error) {
		return &GetStatusMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypeSTATUS, func(r io.Reader) (protocol.Message, error) {
		ver, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("status: read version: %w", err)
		}
		pid, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("status: read pid: %w", err)
		}
		down, err := protocol.ReadBool(r)
		if err != nil {
			return nil, fmt.Errorf("status: read shutting_down: %w", err)
		}
		return &StatusMsg{Version: ver, PID: pid, ShuttingDown: down}, nil
	})
	protocol.RegisterMessage(protocol.TypeHEARTBEAT, func(r io.Reader) (protocol.Message, error) {
		bootID, err := protocol.ReadUint64(r)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read boot_id: %w", err)
		}
		seq, err := protocol.ReadUint64(r)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read seq: %w", err)
		}
		state, err := protocol.ReadUint8(r)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read state: %w", err)
		}
		flags, err := protocol.ReadUint8(r)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read flags: %w", err)
		}
		payloadLen, err := protocol.ReadUint16(r)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read payload_length: %w", err)
		}
		exts, err := readHeartbeatExtensions(r, payloadLen)
		if err != nil {
			return nil, fmt.Errorf("heartbeat: read extensions: %w", err)
		}
		return &HeartbeatMsg{
			BootID:     bootID,
			Seq:        seq,
			State:      state,
			Flags:      flags,
			Extensions: exts,
		}, nil
	})
}
