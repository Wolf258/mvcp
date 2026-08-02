package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type ExecCmd struct {
	Command   string
	Cwd       string
	Env       map[string]string
	TimeoutMs uint32
}

func (m *ExecCmd) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	if err := protocol.WriteString(&buf, m.Command); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(&buf, m.Cwd); err != nil {
		return nil, err
	}
	if err := protocol.WriteStringMap(&buf, m.Env); err != nil {
		return nil, err
	}
	if err := protocol.WriteUint32(&buf, m.TimeoutMs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (m *ExecCmd) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	cmd, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("exec: read command: %w", err)
	}
	cwd, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("exec: read cwd: %w", err)
	}
	env, err := protocol.ReadStringMap(r)
	if err != nil {
		return fmt.Errorf("exec: read env: %w", err)
	}
	timeout, err := protocol.ReadUint32(r)
	if err != nil {
		return fmt.Errorf("exec: read timeout: %w", err)
	}
	m.Command = cmd
	m.Cwd = cwd
	m.Env = env
	m.TimeoutMs = timeout
	return nil
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
		cmd, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("exec: read command: %w", err)
		}
		cwd, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("exec: read cwd: %w", err)
		}
		env, err := protocol.ReadStringMap(r)
		if err != nil {
			return nil, fmt.Errorf("exec: read env: %w", err)
		}
		timeout, err := protocol.ReadUint32(r)
		if err != nil {
			return nil, fmt.Errorf("exec: read timeout: %w", err)
		}
		return &ExecCmd{Command: cmd, Cwd: cwd, Env: env, TimeoutMs: timeout}, nil
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
