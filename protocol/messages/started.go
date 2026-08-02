package messages

import (
	"bytes"
	"fmt"

	"github.com/Wolf258/mvcp/protocol"
)

type StartedMsg struct {
	Stream bool
}

func (m *StartedMsg) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteBool(&buf, m.Stream)
	return buf.Bytes(), nil
}

func (m *StartedMsg) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	stream, err := protocol.ReadBool(r)
	if err != nil {
		return fmt.Errorf("started: read stream: %w", err)
	}
	m.Stream = stream
	return nil
}
