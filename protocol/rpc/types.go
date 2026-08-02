package rpc

import (
	"bytes"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

type Response struct {
	Type uint8
	Body []byte
}

type StreamFrame struct {
	Type uint8
	Body []byte
	More bool
}

type Request struct {
	Type  uint8
	MsgID uint32
	Flags uint8
	Body  []byte

	conn io.ReadWriter
}

func (r *Request) Respond(msgType uint8, body []byte) error {
	return protocol.WriteMVCPFrame(r.conn, &protocol.Frame{
		Type:  msgType,
		Flags: protocol.FlagResponse,
		MsgID: r.MsgID,
		Body:  body,
	})
}

func (r *Request) Stream(msgType uint8, body []byte) error {
	return protocol.WriteMVCPFrame(r.conn, &protocol.Frame{
		Type:  msgType,
		Flags: protocol.FlagStreamMore,
		MsgID: r.MsgID,
		Body:  body,
	})
}

func (r *Request) StreamEnd(msgType uint8, body []byte) error {
	return protocol.WriteMVCPFrame(r.conn, &protocol.Frame{
		Type:  msgType,
		Flags: protocol.FlagResponse,
		MsgID: r.MsgID,
		Body:  body,
	})
}

func (r *Request) Error(code uint16, message string) error {
	var buf bytes.Buffer
	protocol.WriteUint16(&buf, code)
	protocol.WriteString(&buf, message)
	return protocol.WriteMVCPFrame(r.conn, &protocol.Frame{
		Type:  protocol.TypeERROR,
		Flags: protocol.FlagResponse,
		MsgID: r.MsgID,
		Body:  buf.Bytes(),
	})
}

func (r *Request) Started(stream bool) error {
	return protocol.WriteMVCPFrame(r.conn, &protocol.Frame{
		Type:  protocol.TypeSTARTED,
		Flags: 0,
		MsgID: r.MsgID,
		Body:  protocol.EncodeStarted(stream),
	})
}
