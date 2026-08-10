// Package messages defines the MVCP wire message types and their
// binary (de)serialization.
package messages

import (
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type PingMsg struct{}

func (m *PingMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *PingMsg) UnmarshalBinary([]byte) error { return nil }

type PongMsg struct{}

func (m *PongMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *PongMsg) UnmarshalBinary([]byte) error { return nil }

type ShutdownMsg struct{}

func (m *ShutdownMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *ShutdownMsg) UnmarshalBinary([]byte) error { return nil }

type ShutdownAckMsg struct{}

func (m *ShutdownAckMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *ShutdownAckMsg) UnmarshalBinary([]byte) error { return nil }

// SyncFilesystemsMsg requests that the guest flush all mounted
// filesystems. It deliberately carries no caller-controlled command or path:
// this is a lifecycle primitive, not a shell-execution escape hatch.
type SyncFilesystemsMsg struct{}

func (m *SyncFilesystemsMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *SyncFilesystemsMsg) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("sync_filesystems: unexpected payload")
	}
	return nil
}

// SyncFilesystemsAckMsg confirms the guest has completed the sync request.
// Like the request, it has no payload so future versions can introduce a new
// message type instead of silently changing the acknowledgement contract.
type SyncFilesystemsAckMsg struct{}

func (m *SyncFilesystemsAckMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *SyncFilesystemsAckMsg) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("sync_filesystems_ack: unexpected payload")
	}
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypePING, func(r io.Reader) (protocol.Message, error) {
		return &PingMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypePONG, func(r io.Reader) (protocol.Message, error) {
		return &PongMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypeSHUTDOWN, func(r io.Reader) (protocol.Message, error) {
		return &ShutdownMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypeSHUTDOWNACK, func(r io.Reader) (protocol.Message, error) {
		return &ShutdownAckMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypeSYNCFILESYSTEMS, func(r io.Reader) (protocol.Message, error) {
		return decodeEmptyControlMessage(r, &SyncFilesystemsMsg{})
	})
	protocol.RegisterMessage(protocol.TypeSYNCFILESYSTEMSACK, func(r io.Reader) (protocol.Message, error) {
		return decodeEmptyControlMessage(r, &SyncFilesystemsAckMsg{})
	})

	protocol.RegisterMessage(protocol.TypeSTARTED, func(r io.Reader) (protocol.Message, error) {
		stream, err := protocol.ReadBool(r)
		if err != nil {
			return nil, fmt.Errorf("started: read stream: %w", err)
		}
		return &StartedMsg{Stream: stream}, nil
	})
	protocol.RegisterMessage(protocol.TypeERROR, func(r io.Reader) (protocol.Message, error) {
		code, err := protocol.ReadUint16(r)
		if err != nil {
			return nil, fmt.Errorf("error: read code: %w", err)
		}
		msg, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("error: read message: %w", err)
		}
		return &ErrorMsg{Code: code, Message: msg}, nil
	})
}

func decodeEmptyControlMessage(r io.Reader, msg protocol.Message) (protocol.Message, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := msg.UnmarshalBinary(body); err != nil {
		return nil, err
	}
	return msg, nil
}
