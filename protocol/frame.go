package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

const MaxFrameSize = 16 * 1024 * 1024

var ErrFrameTooLarge = errors.New("mvcp: frame exceeds maximum size")

func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func WriteFrame(w io.Writer, payload []byte) error {
	// Single write: header + payload in one syscall. On the vsock hop
	// each write() is one virtio packet delivered to the guest, so
	// splitting the frame into two writes doubles the packets,
	// interrupts, and wake-ups per frame. A single write also makes
	// concurrent writers safe on stream sockets: each frame is queued
	// contiguously and can never interleave with another frame.
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	_, err := w.Write(buf)
	return err
}
