package vpp

import (
	"bytes"
	"errors"
	"io"

	"github.com/Wolf258/mvcp/protocol"
)

const (
	TypeDATA    uint8 = 0x00
	TypeWINCH   uint8 = 0x01
	TypeDETACH  uint8 = 0x02
	TypeATTACH  uint8 = 0x03
	TypeSESSION uint8 = 0x04
	TypeKILL    uint8 = 0x05

	MaxFrameSize = 64 * 1024
)

var ErrFrameTooLarge = errors.New("vpp: frame exceeds maximum size of 64 KB")

type Frame struct {
	Type uint8
	Body []byte
}

func ReadFrame(r io.Reader) (*Frame, error) {
	payload, err := protocol.ReadFrame(r)
	if err != nil {
		return nil, err
	}
	if len(payload) < 1 {
		return nil, io.ErrUnexpectedEOF
	}
	if len(payload) > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	return &Frame{Type: payload[0], Body: payload[1:]}, nil
}

func WriteFrame(w io.Writer, f *Frame) error {
	if len(f.Body)+1 > MaxFrameSize {
		return ErrFrameTooLarge
	}
	var buf bytes.Buffer
	buf.WriteByte(f.Type)
	buf.Write(f.Body)
	return protocol.WriteFrame(w, buf.Bytes())
}
