package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type AppOpen struct {
	StreamID uint32
	Service  string
	Meta     []byte
}

func (m *AppOpen) MarshalBinary() ([]byte, error) {
	if len(m.Meta) > protocol.AppMaxMetaBytes {
		return nil, fmt.Errorf("app_open: meta too large (%d bytes)", len(m.Meta))
	}
	var buf bytes.Buffer
	if err := protocol.WriteUint32(&buf, m.StreamID); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(&buf, m.Service); err != nil {
		return nil, err
	}
	if err := protocol.WriteBytes(&buf, m.Meta); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *AppOpen) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_open: read stream_id: %w", err)
	}
	service, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("app_open: read service: %w", err)
	}
	meta, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("app_open: read meta: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_open: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Service, m.Meta = id, service, meta
	return nil
}

type AppAccept struct {
	StreamID uint32
	Meta     []byte
	Grant    uint32
}

func (m *AppAccept) MarshalBinary() ([]byte, error) {
	if len(m.Meta) > protocol.AppMaxMetaBytes {
		return nil, fmt.Errorf("app_accept: meta too large (%d bytes)", len(m.Meta))
	}
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	protocol.WriteBytes(&buf, m.Meta)
	protocol.WriteUint32(&buf, m.Grant)
	return buf.Bytes(), nil
}

func (m *AppAccept) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_accept: read stream_id: %w", err)
	}
	meta, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("app_accept: read meta: %w", err)
	}
	grant, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_accept: read grant: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_accept: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Meta, m.Grant = id, meta, grant
	return nil
}

type AppReject struct {
	StreamID uint32
	Code     uint16
	Message  string
}

func (m *AppReject) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	protocol.WriteUint16(&buf, m.Code)
	protocol.WriteString(&buf, m.Message)
	return buf.Bytes(), nil
}

func (m *AppReject) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_reject: read stream_id: %w", err)
	}
	code, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("app_reject: read code: %w", err)
	}
	msg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("app_reject: read message: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_reject: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Code, m.Message = id, code, msg
	return nil
}

type AppData struct {
	StreamID uint32
	Data     []byte
}

func (m *AppData) MarshalBinary() ([]byte, error) {
	if len(m.Data) > protocol.AppMaxDataBytes {
		return nil, fmt.Errorf("app_data: data too large (%d bytes)", len(m.Data))
	}
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	protocol.WriteBytes(&buf, m.Data)
	return buf.Bytes(), nil
}

func (m *AppData) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_data: read stream_id: %w", err)
	}
	d, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("app_data: read data: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_data: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Data = id, d
	return nil
}

type AppCredit struct {
	StreamID uint32
	Bytes    uint32
}

func (m *AppCredit) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	protocol.WriteUint32(&buf, m.Bytes)
	return buf.Bytes(), nil
}

func (m *AppCredit) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_credit: read stream_id: %w", err)
	}
	n, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_credit: read bytes: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_credit: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Bytes = id, n
	return nil
}

type AppClose struct {
	StreamID uint32
}

func (m *AppClose) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	return buf.Bytes(), nil
}

func (m *AppClose) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_close: read stream_id: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_close: %d trailing bytes", r.Len())
	}
	m.StreamID = id
	return nil
}

type AppReset struct {
	StreamID uint32
	Code     uint16
	Message  string
}

func (m *AppReset) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint32(&buf, m.StreamID)
	protocol.WriteUint16(&buf, m.Code)
	protocol.WriteString(&buf, m.Message)
	return buf.Bytes(), nil
}

func (m *AppReset) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	id, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("app_reset: read stream_id: %w", err)
	}
	code, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("app_reset: read code: %w", err)
	}
	msg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("app_reset: read message: %w", err)
	}
	if r.Len() != 0 {
		return fmt.Errorf("app_reset: %d trailing bytes", r.Len())
	}
	m.StreamID, m.Code, m.Message = id, code, msg
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeAPPOPEN, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppOpen{})
	})
	protocol.RegisterMessage(protocol.TypeAPPACCEPT, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppAccept{})
	})
	protocol.RegisterMessage(protocol.TypeAPPREJECT, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppReject{})
	})
	protocol.RegisterMessage(protocol.TypeAPPDATA, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppData{})
	})
	protocol.RegisterMessage(protocol.TypeAPPCREDIT, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppCredit{})
	})
	protocol.RegisterMessage(protocol.TypeAPPCLOSE, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppClose{})
	})
	protocol.RegisterMessage(protocol.TypeAPPRESET, func(r io.Reader) (protocol.Message, error) {
		return decodeApp(r, &AppReset{})
	})
}

func decodeApp(r io.Reader, m protocol.Message) (protocol.Message, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := m.(interface{ UnmarshalBinary([]byte) error }).UnmarshalBinary(data); err != nil {
		return nil, err
	}
	return m, nil
}
