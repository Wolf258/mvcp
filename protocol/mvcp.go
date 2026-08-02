package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
)

const frameHeaderSize = 6

// --- Message types ---

const (
	TypePING        uint8 = 0x01
	TypePONG        uint8 = 0x02
	TypeSHUTDOWN    uint8 = 0x03
	TypeSHUTDOWNACK uint8 = 0x04

	TypeGETSTATUS uint8 = 0x05
	TypeSTATUS    uint8 = 0x06
	TypeHEARTBEAT uint8 = 0x07

	TypeEXEC       uint8 = 0x10
	TypeEXECSTREAM uint8 = 0x11
	TypeEXECRESULT uint8 = 0x12

	TypeXFERINIT  uint8 = 0x20
	TypeXFERCHUNK uint8 = 0x21
	TypeXFERDONE  uint8 = 0x22

	TypeTOOLCALL        uint8 = 0x30
	TypeTOOLRESULT      uint8 = 0x31
	TypeLISTTOOLS       uint8 = 0x32
	TypeLISTTOOLSRESULT uint8 = 0x33

	TypeEVENTREADY        uint8 = 0x80
	TypeEVENTFILERECEIVED uint8 = 0x81
	TypeEVENTMOUNT        uint8 = 0x82
	TypeEVENTERROR        uint8 = 0x83
	TypeEVENTLOG          uint8 = 0x84

	TypeSTARTED uint8 = 0xFA
	TypeERROR   uint8 = 0xFE
)

// --- Flags ---

const (
	FlagResponse      uint8 = 0x01
	FlagStreamMore    uint8 = 0x02
	FlagExecStreaming uint8 = 0x04
)

// --- Error codes ---

const (
	ErrorCodeUnknownType      uint16 = 0x0001
	ErrorCodeBadPayload       uint16 = 0x0002
	ErrorCodeFileNotFound     uint16 = 0x0003
	ErrorCodePermissionDenied uint16 = 0x0004
	ErrorCodeExecFailed       uint16 = 0x0005
	ErrorCodeTimeout          uint16 = 0x0006
	ErrorCodeNotADirectory    uint16 = 0x0007
	ErrorCodeBadVersion       uint16 = 0x0008
	ErrorCodeUnknownTool      uint16 = 0x0009
	ErrorCodeToolFailed       uint16 = 0x000A
)

// --- Heartbeat states (lifecycle) ---
//
// The State byte in a heartbeat is the VM's *lifecycle* state — a
// mutually exclusive set (Booting/Running/Stopping/Stopped/Failed).
// The VM's orthogonal *health* state (Healthy/Degraded) is carried
// in a TLV extension (see ExtHealth below) so the two dimensions
// never share a byte.

const (
	HeartbeatStateUnknown  uint8 = 0
	HeartbeatStateBooting  uint8 = 1
	HeartbeatStateRunning  uint8 = 2
	HeartbeatStateStopping uint8 = 3
	HeartbeatStateStopped  uint8 = 4
	HeartbeatStateFailed   uint8 = 5
)

// --- Health states (orthogonal to lifecycle) ---
//
// Health is carried in the heartbeat body as a TLV extension
// (ExtHealth, 1-byte value). The host treats health independently
// of the lifecycle state: a VM can be Running+Degraded, Booting+
// Degraded, etc. Terminal lifecycle states (Stopped/Failed/
// Unreachable on the host) are always reported as Healthy by
// convention; the host stops reading health once lifecycle ends.

const (
	HealthHealthy  uint8 = 0
	HealthDegraded uint8 = 1
)

// --- Heartbeat flags ---

const (
	HeartbeatFlagBusy         uint8 = 1 << 0
	HeartbeatFlagMaintenance  uint8 = 1 << 1
	HeartbeatFlagReadOnly     uint8 = 1 << 2
	HeartbeatFlagLowResources uint8 = 1 << 3
)

// --- Heartbeat extension types (TLVs in the body) ---

const (
	ExtCPUUsage    uint16 = 1
	ExtMemoryUsage uint16 = 2
	ExtQueueDepth  uint16 = 3
	// ExtHealth is a 1-byte TLV carrying the Health state
	// (HealthHealthy/HealthDegraded). If absent, the host
	// assumes HealthHealthy (default).
	ExtHealth uint16 = 4
	// ExtFailureReason is a variable-length string TLV the
	// guest emits only when State=Failed. Carries a short
	// machine-readable reason (e.g. "init_timeout",
	// "mount_failed"). Absence on a Failed heartbeat is fine;
	// the host just stores LastError="guest reported Failed".
	ExtFailureReason uint16 = 5
)

// --- ExecChannel — stream channel identifiers ---

const (
	ExecChannelStdout   uint8 = 0x00
	ExecChannelStderr   uint8 = 0x01
	ExecChannelProgress uint8 = 0x10
	ExecChannelLog      uint8 = 0x11
	ExecChannelMetric   uint8 = 0x12
	ExecChannelDebug    uint8 = 0x13
)

// --- Event log levels ---

const (
	LogLevelDebug uint8 = 0x00
	LogLevelInfo  uint8 = 0x01
	LogLevelWarn  uint8 = 0x02
	LogLevelError uint8 = 0x03
)

// --- File transfer direction ---

const (
	XferDirImport uint8 = 0x00
	XferDirExport uint8 = 0x01
)

type Frame struct {
	Type  uint8
	Flags uint8
	MsgID uint32
	Body  []byte
}

func ReadMVCPFrame(r io.Reader) (*Frame, error) {
	payload, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	if len(payload) < frameHeaderSize {
		return nil, io.ErrUnexpectedEOF
	}
	return &Frame{
		Type:  payload[0],
		Flags: payload[1],
		MsgID: binary.BigEndian.Uint32(payload[2:6]),
		Body:  payload[6:],
	}, nil
}

func WriteMVCPFrame(w io.Writer, f *Frame) error {
	var buf bytes.Buffer
	buf.Grow(frameHeaderSize + len(f.Body))
	buf.WriteByte(f.Type)
	buf.WriteByte(f.Flags)
	var id [4]byte
	binary.BigEndian.PutUint32(id[:], f.MsgID)
	buf.Write(id[:])
	buf.Write(f.Body)
	return WriteFrame(w, buf.Bytes())
}

func EncodeStarted(stream bool) []byte {
	var buf bytes.Buffer
	WriteBool(&buf, stream)
	return buf.Bytes()
}
