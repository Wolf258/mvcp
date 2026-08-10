// Package vpp implements the MVCP vsock proxy protocol (VPP) used for
// console attach between the host and the guest vhandler.
package vpp

import (
	"bytes"
	"fmt"

	"github.com/Wolf258/mvcp/protocol"
)

type AttachMsg struct {
	Term string
	Cols uint16
	Rows uint16
}

func (m *AttachMsg) Encode() []byte {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.Term)
	protocol.WriteUint16(&buf, m.Cols)
	protocol.WriteUint16(&buf, m.Rows)
	return buf.Bytes()
}

func (m *AttachMsg) Decode(body []byte) error {
	r := bytes.NewReader(body)
	term, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("vpp attach: read term: %w", err)
	}
	cols, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp attach: read cols: %w", err)
	}
	rows, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp attach: read rows: %w", err)
	}
	m.Term = term
	m.Cols = cols
	m.Rows = rows
	return nil
}

type SessionMsg struct {
	SessionID uint32
	PID       uint32
	Cols      uint16
	Rows      uint16
}

func (m *SessionMsg) Encode() []byte {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.SessionID)
	protocol.WriteUint32(&buf, m.PID)
	protocol.WriteUint16(&buf, m.Cols)
	protocol.WriteUint16(&buf, m.Rows)
	return buf.Bytes()
}

func (m *SessionMsg) Decode(body []byte) error {
	r := bytes.NewReader(body)
	sid, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("vpp session: read session_id: %w", err)
	}
	pid, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("vpp session: read pid: %w", err)
	}
	cols, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp session: read cols: %w", err)
	}
	rows, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp session: read rows: %w", err)
	}
	m.SessionID = sid
	m.PID = pid
	m.Cols = cols
	m.Rows = rows
	return nil
}

type WinchMsg struct {
	Cols uint16
	Rows uint16
}

func (m *WinchMsg) Encode() []byte {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, m.Cols)
	protocol.WriteUint16(&buf, m.Rows)
	return buf.Bytes()
}

func (m *WinchMsg) Decode(body []byte) error {
	r := bytes.NewReader(body)
	cols, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp winch: read cols: %w", err)
	}
	rows, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("vpp winch: read rows: %w", err)
	}
	m.Cols = cols
	m.Rows = rows
	return nil
}

type DetachMsg struct {
	ExitCode uint32
}

func (m *DetachMsg) Encode() []byte {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.ExitCode)
	return buf.Bytes()
}

func (m *DetachMsg) Decode(body []byte) error {
	r := bytes.NewReader(body)
	code, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("vpp detach: read exit_code: %w", err)
	}
	m.ExitCode = code
	return nil
}
