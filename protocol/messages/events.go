package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type EventReady struct {
	Version string
}

func (m *EventReady) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.Version)
	return buf.Bytes(), nil
}

func (m *EventReady) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	ver, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_ready: read version: %w", err)
	}
	m.Version = ver
	return nil
}

type EventFileReceived struct {
	Path string
	Size uint64
}

func (m *EventFileReceived) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.Path)
	protocol.WriteUint64(&buf, m.Size)
	return buf.Bytes(), nil
}

func (m *EventFileReceived) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	path, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_file_received: read path: %w", err)
	}
	size, err := protocol.ReadUint64(r)
	if err != nil {
		return fmt.Errorf("event_file_received: read size: %w", err)
	}
	m.Path = path
	m.Size = size
	return nil
}

type EventMount struct {
	Path   string
	FsType string
}

func (m *EventMount) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.Path)
	protocol.WriteString(&buf, m.FsType)
	return buf.Bytes(), nil
}

func (m *EventMount) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	path, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_mount: read path: %w", err)
	}
	fstype, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_mount: read fstype: %w", err)
	}
	m.Path = path
	m.FsType = fstype
	return nil
}

type EventErrorMsg struct {
	Code    uint16
	Message string
}

func (m *EventErrorMsg) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, m.Code)
	protocol.WriteString(&buf, m.Message)
	return buf.Bytes(), nil
}

func (m *EventErrorMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	code, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("event_error: read code: %w", err)
	}
	msg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_error: read message: %w", err)
	}
	m.Code = code
	m.Message = msg
	return nil
}

type EventLog struct {
	Level   uint8
	Message string
	TsNs    uint64
}

func (m *EventLog) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint8(&buf, m.Level)
	protocol.WriteString(&buf, m.Message)
	protocol.WriteUint64(&buf, m.TsNs)
	return buf.Bytes(), nil
}

func (m *EventLog) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	lvl, err := protocol.ReadUint8(r)
	if err != nil {
		return fmt.Errorf("event_log: read level: %w", err)
	}
	msg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("event_log: read message: %w", err)
	}
	ts, err := protocol.ReadUint64(r)
	if err != nil {
		return fmt.Errorf("event_log: read ts_ns: %w", err)
	}
	m.Level = lvl
	m.Message = msg
	m.TsNs = ts
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeEVENTREADY, func(r io.Reader) (protocol.Message, error) {
		ver, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_ready: %w", err)
		}
		return &EventReady{Version: ver}, nil
	})
	protocol.RegisterMessage(protocol.TypeEVENTFILERECEIVED, func(r io.Reader) (protocol.Message, error) {
		path, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_file_received: %w", err)
		}
		size, err := protocol.ReadUint64(r)
		if err != nil {
			return nil, fmt.Errorf("event_file_received: %w", err)
		}
		return &EventFileReceived{Path: path, Size: size}, nil
	})
	protocol.RegisterMessage(protocol.TypeEVENTMOUNT, func(r io.Reader) (protocol.Message, error) {
		path, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_mount: %w", err)
		}
		fs, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_mount: %w", err)
		}
		return &EventMount{Path: path, FsType: fs}, nil
	})
	protocol.RegisterMessage(protocol.TypeEVENTERROR, func(r io.Reader) (protocol.Message, error) {
		code, err := protocol.ReadUint16(r)
		if err != nil {
			return nil, fmt.Errorf("event_error: %w", err)
		}
		msg, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_error: %w", err)
		}
		return &EventErrorMsg{Code: code, Message: msg}, nil
	})
	protocol.RegisterMessage(protocol.TypeEVENTLOG, func(r io.Reader) (protocol.Message, error) {
		lvl, err := protocol.ReadUint8(r)
		if err != nil {
			return nil, fmt.Errorf("event_log: %w", err)
		}
		msg, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("event_log: %w", err)
		}
		ts, err := protocol.ReadUint64(r)
		if err != nil {
			return nil, fmt.Errorf("event_log: %w", err)
		}
		return &EventLog{Level: lvl, Message: msg, TsNs: ts}, nil
	})
}
