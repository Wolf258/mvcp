package messages

import (
	"bytes"
	"fmt"

	"github.com/Wolf258/mvcp/protocol"
)

type ErrorMsg struct {
	Code    uint16
	Message string
}

func (m *ErrorMsg) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, m.Code)
	protocol.WriteString(&buf, m.Message)
	return buf.Bytes(), nil
}

func (m *ErrorMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	code, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("error: read code: %w", err)
	}
	msg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("error: read message: %w", err)
	}
	m.Code = code
	m.Message = msg
	return nil
}
