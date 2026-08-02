package protocol

import (
	"bytes"
	"encoding"
	"fmt"
	"io"
)

type Message interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type DecodeFunc func(r io.Reader) (Message, error)

var decodeRegistry = map[uint8]DecodeFunc{}

func RegisterMessage(typ uint8, fn DecodeFunc) {
	decodeRegistry[typ] = fn
}

func DecodeMessage(typ uint8, r io.Reader) (Message, error) {
	fn, ok := decodeRegistry[typ]
	if !ok {
		return nil, fmt.Errorf("mvcp: unknown message type 0x%02X", typ)
	}
	return fn(r)
}

func DecodeMessageBody(typ uint8, body []byte) (Message, error) {
	return DecodeMessage(typ, bytes.NewReader(body))
}
