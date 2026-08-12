// frame_test.go - mvcp protocol transport
//
// Regression tests for the transport frame. WriteFrame must emit each
// frame as a single write: on the vsock hop every write() is one
// virtio packet, so splitting header/payload doubles the packets and
// interrupts per frame, and on stream sockets a single write keeps
// concurrent writers from interleaving frames.

package protocol

import (
	"bytes"
	"testing"
)

type countingWriter struct {
	calls int
	buf   bytes.Buffer
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(p)
}

func TestWriteFrameSingleWrite(t *testing.T) {
	var w countingWriter
	payload := []byte{0x03, 0x00, 0x0E, 'x'}
	if err := WriteFrame(&w, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if w.calls != 1 {
		t.Fatalf("WriteFrame made %d writes, want exactly 1 (one frame = one packet)", w.calls)
	}
	want := []byte{0, 0, 0, 4, 0x03, 0x00, 0x0E, 'x'}
	if !bytes.Equal(w.buf.Bytes(), want) {
		t.Fatalf("wire bytes = %x, want %x", w.buf.Bytes(), want)
	}
}

func TestWriteFrameEmptyPayload(t *testing.T) {
	var w countingWriter
	if err := WriteFrame(&w, nil); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if w.calls != 1 {
		t.Fatalf("WriteFrame made %d writes, want 1", w.calls)
	}
	if want := []byte{0, 0, 0, 0}; !bytes.Equal(w.buf.Bytes(), want) {
		t.Fatalf("wire bytes = %x, want %x", w.buf.Bytes(), want)
	}
}
