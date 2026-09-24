package messages

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

func decodeExecFrame(t *testing.T, body []byte) (interface{}, error) {
	t.Helper()
	frame := &protocol.Frame{Type: protocol.TypeEXEC, Flags: 0, MsgID: 1, Body: body}
	var buf bytes.Buffer
	if err := protocol.WriteMVCPFrame(&buf, frame); err != nil {
		t.Fatalf("write mvcp frame: %v", err)
	}
	decoded, err := protocol.ReadMVCPFrame(&buf)
	if err != nil {
		return nil, err
	}
	return protocol.DecodeMessage(decoded.Type, bytes.NewReader(decoded.Body))
}

func TestExecCmdJSONRoundTrip(t *testing.T) {
	timeout := int64(120000)
	want := &ExecCmd{
		Command:   "pytest -q",
		Workdir:   "/work/pkg/foo",
		Env:       map[string]string{"FOO": "1"},
		TimeoutMs: &timeout,
	}
	body, err := want.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("MarshalBinary body is not JSON: %q", body)
	}
	gotAny, err := decodeExecFrame(t, body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got, ok := gotAny.(*ExecCmd)
	if !ok {
		t.Fatalf("decoded %T, want *ExecCmd", gotAny)
	}
	if got.Command != want.Command || got.Workdir != want.Workdir || got.Env["FOO"] != "1" {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	if got.TimeoutMs == nil || *got.TimeoutMs != 120000 {
		t.Fatalf("timeout = %v, want 120000", got.TimeoutMs)
	}
	if got.HasSpec() {
		t.Fatal("HasSpec = true, want false for absent spec")
	}
}

func TestExecCmdAbsentTimeoutIsNil(t *testing.T) {
	body := []byte(`{"command":"ls","workdir":"/work"}`)
	gotAny, err := decodeExecFrame(t, body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := gotAny.(*ExecCmd)
	if got.TimeoutMs != nil {
		t.Fatalf("timeout = %v, want nil (absent)", *got.TimeoutMs)
	}
}

func TestExecCmdRejectsUnknownFields(t *testing.T) {
	body := []byte(`{"command":"ls","workdir":"/work","cwd":"/tmp"}`)
	if _, err := decodeExecFrame(t, body); err == nil {
		t.Fatal("decode accepted unknown field cwd, want error")
	}
}

func TestExecCmdHasSpec(t *testing.T) {
	cases := map[string]bool{
		``:                                   false,
		`{"command":"ls","workdir":"/work"}`: false,
		`{"spec":null}`:                      false,
		`{"spec":{}}`:                        true,
		`{"spec":{"profile":"x"}}`:           true,
	}
	for body, want := range cases {
		var m ExecCmd
		if err := m.UnmarshalBinary([]byte(body)); err != nil {
			t.Fatalf("UnmarshalBinary(%s): %v", body, err)
		}
		if got := m.HasSpec(); got != want {
			t.Fatalf("HasSpec(%s) = %v, want %v", body, got, want)
		}
	}
}

func TestExecCmdValidate(t *testing.T) {
	timeout := int64(1000)
	zero := int64(0)
	negative := int64(-1)
	cases := []struct {
		name string
		cmd  ExecCmd
		want string
	}{
		{"ok", ExecCmd{Command: "ls", Workdir: "/work", TimeoutMs: &timeout}, ""},
		{"missing command", ExecCmd{Workdir: "/work"}, "command is required"},
		{"missing workdir", ExecCmd{Command: "ls"}, "workdir is required"},
		{"relative workdir", ExecCmd{Command: "ls", Workdir: "pkg/foo"}, "workdir must be absolute"},
		{"unclean workdir", ExecCmd{Command: "ls", Workdir: "/work/../etc"}, "workdir must be clean"},
		{"zero timeout", ExecCmd{Command: "ls", Workdir: "/work", TimeoutMs: &zero}, "timeout_ms must be positive"},
		{"negative timeout", ExecCmd{Command: "ls", Workdir: "/work", TimeoutMs: &negative}, "timeout_ms must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate: %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate = %v, want containing %q", err, tc.want)
			}
		})
	}
}
