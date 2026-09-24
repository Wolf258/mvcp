package messages

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Wolf258/mvcp/protocol"
)

// ExecCmd is the body of EXEC (0x10): control fields as canonical JSON.
// Bulk data keeps its binary channels: EXECSTREAM (0x11) and EXECRESULT
// (0x12) are unchanged.
type ExecCmd struct {
	Command   string            `json:"command"`
	Workdir   string            `json:"workdir"`
	Env       map[string]string `json:"env,omitempty"`
	TimeoutMs *int64            `json:"timeout_ms,omitempty"`
	// Spec is reserved for a future ExecSpec. v1 senders omit it and v1
	// receivers MUST reject any non-empty payload (fail-closed).
	Spec json.RawMessage `json:"spec,omitempty"`
}

func (m *ExecCmd) MarshalBinary() ([]byte, error) {
	return json.Marshal(m)
}

func (m *ExecCmd) UnmarshalBinary(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(m); err != nil {
		if errors.Is(err, io.EOF) {
			// Empty body decodes to the zero value; Validate rejects it
			// ("command is required"), so the wire stays fail-closed.
			return nil
		}
		return fmt.Errorf("exec: decode body: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("exec: trailing data after body")
	}
	return nil
}

// Validate checks the wire invariants shared by both endpoints. Workdir
// is always absolute: core resolves it before sending and the guest never
// resolves relative paths.
func (m *ExecCmd) Validate() error {
	if m.Command == "" {
		return errors.New("command is required")
	}
	if m.Workdir == "" {
		return errors.New("workdir is required")
	}
	if !path.IsAbs(m.Workdir) {
		return errors.New("workdir must be absolute")
	}
	if path.Clean(m.Workdir) != m.Workdir {
		return errors.New("workdir must be clean (no . or ..)")
	}
	if strings.ContainsRune(m.Workdir, 0) {
		return errors.New("workdir contains NUL")
	}
	if m.TimeoutMs != nil && *m.TimeoutMs <= 0 {
		return errors.New("timeout_ms must be positive")
	}
	return nil
}

// HasSpec reports whether the body carries a non-empty ExecSpec payload.
func (m *ExecCmd) HasSpec() bool {
	s := bytes.TrimSpace(m.Spec)
	return len(s) > 0 && !bytes.Equal(s, []byte("null"))
}

type ExecResult struct {
	ExitCode   int32
	Stdout     []byte
	Stderr     []byte
	DurationMs uint32
}

func (m *ExecResult) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := protocol.WriteInt32(&buf, m.ExitCode); err != nil {
		return nil, err
	}
	if err := protocol.WriteBytes(&buf, m.Stdout); err != nil {
		return nil, err
	}
	if err := protocol.WriteBytes(&buf, m.Stderr); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint32(&buf, m.DurationMs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ExecResult) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	exit, err := protocol.ReadInt32(r)
	if err != nil {
		return fmt.Errorf("exec_result: read exit_code: %w", err)
	}
	stdout, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("exec_result: read stdout: %w", err)
	}
	stderr, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("exec_result: read stderr: %w", err)
	}
	dur, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("exec_result: read duration: %w", err)
	}
	m.ExitCode = exit
	m.Stdout = stdout
	m.Stderr = stderr
	m.DurationMs = dur
	return nil
}

type ExecStream struct {
	Channel  uint8
	Sequence uint32
	Data     []byte
}

func (m *ExecStream) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := protocol.WriteUint8(&buf, m.Channel); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint32(&buf, m.Sequence); err != nil {
		return nil, err
	}
	if err := protocol.WriteBytes(&buf, m.Data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ExecStream) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	ch, err := protocol.ReadUint8(r)
	if err != nil {
		return fmt.Errorf("exec_stream: read channel: %w", err)
	}
	seq, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("exec_stream: read sequence: %w", err)
	}
	d, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("exec_stream: read data: %w", err)
	}
	m.Channel = ch
	m.Sequence = seq
	m.Data = d
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeEXEC, func(r io.Reader) (protocol.Message, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("exec: read body: %w", err)
		}
		m := &ExecCmd{}
		if err := m.UnmarshalBinary(data); err != nil {
			return nil, err
		}
		return m, nil
	})
	protocol.RegisterMessage(protocol.TypeEXECRESULT, func(r io.Reader) (protocol.Message, error) {
		exit, err := protocol.ReadInt32(r)
		if err != nil {
			return nil, fmt.Errorf("exec_result: read exit_code: %w", err)
		}
		stdout, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("exec_result: read stdout: %w", err)
		}
		stderr, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("exec_result: read stderr: %w", err)
		}
		dur, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("exec_result: read duration: %w", err)
		}
		return &ExecResult{ExitCode: exit, Stdout: stdout, Stderr: stderr, DurationMs: dur}, nil
	})
	protocol.RegisterMessage(protocol.TypeEXECSTREAM, func(r io.Reader) (protocol.Message, error) {
		ch, err := protocol.ReadUint8(r)
		if err != nil {
			return nil, fmt.Errorf("exec_stream: read channel: %w", err)
		}
		seq, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("exec_stream: read sequence: %w", err)
		}
		data, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("exec_stream: read data: %w", err)
		}
		return &ExecStream{Channel: ch, Sequence: seq, Data: data}, nil
	})
}
