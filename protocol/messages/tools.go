package messages

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type ToolCall struct {
	ToolName string
	Params   []byte
}

func (m *ToolCall) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteString(&buf, m.ToolName)
	protocol.WriteBytes(&buf, m.Params)
	return buf.Bytes(), nil
}

func (m *ToolCall) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	name, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("tool_call: read tool_name: %w", err)
	}
	params, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("tool_call: read params: %w", err)
	}
	m.ToolName = name
	m.Params = params
	return nil
}

type ToolResult struct {
	Result   []byte
	Ok       bool
	ErrorMsg string
}

func (m *ToolResult) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteBytes(&buf, m.Result)
	protocol.WriteBool(&buf, m.Ok)
	protocol.WriteString(&buf, m.ErrorMsg)
	return buf.Bytes(), nil
}

func (m *ToolResult) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	result, err := protocol.ReadBytes(r)
	if err != nil {
		return fmt.Errorf("tool_result: read result: %w", err)
	}
	ok, err := protocol.ReadBool(r)
	if err != nil {
		return fmt.Errorf("tool_result: read ok: %w", err)
	}
	errMsg, err := protocol.ReadString(r)
	if err != nil {
		return fmt.Errorf("tool_result: read error_msg: %w", err)
	}
	m.Result = result
	m.Ok = ok
	m.ErrorMsg = errMsg
	return nil
}

type ListToolsMsg struct{}

func (m *ListToolsMsg) MarshalBinary() ([]byte, error) { return nil, nil }

func (m *ListToolsMsg) UnmarshalBinary([]byte) error { return nil }

type ToolEntry struct {
	Name         string
	Description  string
	Version      string
	Capabilities []string
	Permissions  []string
	Schema       []byte
}

type ListToolsResult struct {
	Entries []ToolEntry
}

func (m *ListToolsResult) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, uint16(len(m.Entries)))
	for _, e := range m.Entries {
		protocol.WriteString(&buf, e.Name)
		protocol.WriteString(&buf, e.Description)
		protocol.WriteString(&buf, e.Version)
		protocol.WriteStringSlice(&buf, e.Capabilities)
		protocol.WriteStringSlice(&buf, e.Permissions)
		protocol.WriteBytes(&buf, e.Schema)
	}
	return buf.Bytes(), nil
}

func (m *ListToolsResult) UnmarshalBinary(data []byte) error {
	r := bytes.NewReader(data)
	count, err := protocol.ReadUint16(r)
	if err != nil {
		return fmt.Errorf("list_tools_result: read count: %w", err)
	}
	m.Entries = make([]ToolEntry, count)
	for i := uint16(0); i < count; i++ {
		name, err := protocol.ReadString(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read name: %w", err)
		}
		desc, err := protocol.ReadString(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read description: %w", err)
		}
		version, err := protocol.ReadString(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read version: %w", err)
		}
		caps, err := protocol.ReadStringSlice(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read capabilities: %w", err)
		}
		perms, err := protocol.ReadStringSlice(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read permissions: %w", err)
		}
		schema, err := protocol.ReadBytes(r)
		if err != nil {
			return fmt.Errorf("list_tools_result: read schema: %w", err)
		}
		m.Entries[i] = ToolEntry{Name: name, Description: desc, Version: version, Capabilities: caps, Permissions: perms, Schema: schema}
	}
	return nil
}

func init() {
	protocol.RegisterMessage(protocol.TypeTOOLCALL, func(r io.Reader) (protocol.Message, error) {
		name, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("tool_call: %w", err)
		}
		params, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("tool_call: %w", err)
		}
		return &ToolCall{ToolName: name, Params: params}, nil
	})
	protocol.RegisterMessage(protocol.TypeTOOLRESULT, func(r io.Reader) (protocol.Message, error) {
		result, err := protocol.ReadBytes(r)
		if err != nil {
			return nil, fmt.Errorf("tool_result: %w", err)
		}
		ok, err := protocol.ReadBool(r)
		if err != nil {
			return nil, fmt.Errorf("tool_result: %w", err)
		}
		errMsg, err := protocol.ReadString(r)
		if err != nil {
			return nil, fmt.Errorf("tool_result: %w", err)
		}
		return &ToolResult{Result: result, Ok: ok, ErrorMsg: errMsg}, nil
	})
	protocol.RegisterMessage(protocol.TypeLISTTOOLS, func(r io.Reader) (protocol.Message, error) {
		return &ListToolsMsg{}, nil
	})
	protocol.RegisterMessage(protocol.TypeLISTTOOLSRESULT, func(r io.Reader) (protocol.Message, error) {
		count, err := protocol.ReadUint16(r)
		if err != nil {
			return nil, fmt.Errorf("list_tools_result: %w", err)
		}
		entries := make([]ToolEntry, count)
		for i := uint16(0); i < count; i++ {
			name, err := protocol.ReadString(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			desc, err := protocol.ReadString(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			version, err := protocol.ReadString(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			caps, err := protocol.ReadStringSlice(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			perms, err := protocol.ReadStringSlice(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			schema, err := protocol.ReadBytes(r)
			if err != nil {
				return nil, fmt.Errorf("list_tools_result: %w", err)
			}
			entries[i] = ToolEntry{Name: name, Description: desc, Version: version, Capabilities: caps, Permissions: perms, Schema: schema}
		}
		return &ListToolsResult{Entries: entries}, nil
	})
}
