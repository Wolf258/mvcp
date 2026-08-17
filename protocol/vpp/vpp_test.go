package vpp

import (
	"bytes"
	"testing"
)

func TestFrameRoundTripAllTypes(t *testing.T) {
	for _, tt := range []struct {
		name string
		f    *Frame
	}{
		{"data", &Frame{Type: TypeDATA, Body: []byte("hello")}},
		{"winch", &Frame{Type: TypeWINCH, Body: []byte{0x00, 0x78, 0x00, 0x28}}},
		{"detach", &Frame{Type: TypeDETACH, Body: []byte{0x00, 0x00, 0x00, 0x89}}},
		{"attach", &Frame{Type: TypeATTACH, Body: []byte{0x00, 0x05, 'l', 'i', 'n', 'u', 'x'}}},
		{"session", &Frame{Type: TypeSESSION, Body: []byte{0x00, 0x00, 0x00, 0x01}}},
		{"kill", &Frame{Type: TypeKILL}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, tt.f); err != nil {
				t.Fatalf("write: %v", err)
			}
			out, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if out.Type != tt.f.Type || !bytes.Equal(out.Body, tt.f.Body) {
				t.Fatalf("round-trip = type=%d body=%v, want type=%d body=%v", out.Type, out.Body, tt.f.Type, tt.f.Body)
			}
		})
	}
}

func TestKILLWireSize(t *testing.T) {
	// Wire layout: 4-byte BE length prefix + 1-byte type. A KILL frame
	// with an empty body is exactly 5 bytes on the wire.
	var buf bytes.Buffer
	if err := WriteFrame(&buf, &Frame{Type: TypeKILL}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.Len() != 5 {
		t.Fatalf("KILL frame wire size = %d, want 5 (4B length + 1B type)", buf.Len())
	}
	if !bytes.Equal(buf.Bytes(), []byte{0x00, 0x00, 0x00, 0x01, 0x05}) {
		t.Fatalf("KILL frame bytes = %v, want [00 00 00 01 05]", buf.Bytes())
	}
}

func TestWriteFrameMaxSize(t *testing.T) {
	// MaxFrameSize bounds the payload (type + body). A body of exactly
	// MaxFrameSize must be rejected; MaxFrameSize-1 is the largest valid.
	if err := WriteFrame(&bytes.Buffer{}, &Frame{Type: TypeDATA, Body: make([]byte, MaxFrameSize)}); err == nil {
		t.Fatal("body of MaxFrameSize must be rejected")
	}
	if err := WriteFrame(&bytes.Buffer{}, &Frame{Type: TypeDATA, Body: make([]byte, MaxFrameSize-1)}); err != nil {
		t.Fatalf("body of MaxFrameSize-1 must be accepted: %v", err)
	}
}

func TestReadFrameMaxSize(t *testing.T) {
	// Payload of MaxFrameSize+1 (type + MaxFrameSize body) must be
	// rejected by the reader; payload of MaxFrameSize is the largest valid.
	oversized := make([]byte, 4+MaxFrameSize+1)
	putBE32(oversized[:4], uint32(MaxFrameSize+1))
	oversized[4] = TypeDATA
	if _, err := ReadFrame(bytes.NewReader(oversized)); err == nil {
		t.Fatal("payload of MaxFrameSize+1 must be rejected")
	}

	largest := make([]byte, 4+MaxFrameSize)
	putBE32(largest[:4], uint32(MaxFrameSize))
	largest[4] = TypeDATA
	if _, err := ReadFrame(bytes.NewReader(largest)); err != nil {
		t.Fatalf("payload of MaxFrameSize must be accepted: %v", err)
	}
}

func putBE32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}
