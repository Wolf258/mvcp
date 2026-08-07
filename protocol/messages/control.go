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
