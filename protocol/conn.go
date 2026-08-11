package protocol

import (
	"bytes"
	"errors"
	"io"
)

// VPP is the companion console protocol (port 9001). Its handshake is
// fire-and-forget and unchanged by the MVCP HELLO negotiation: the
// console channel does not negotiate capabilities.
const (
	VPPVersion uint8 = 0x01

	vppMagicSize = 3
)

var (
	vppMagic = []byte{'V', 'P', 'P'}

	ErrBadVPPHandshake = errors.New("vpp: bad handshake magic or version")
)

func WriteVPPHandshake(w io.Writer) error {
	handshake := [4]byte{'V', 'P', 'P', VPPVersion}
	_, err := w.Write(handshake[:])
	return err
}

func ValidateVPPHandshake(r io.Reader) error {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if !bytes.Equal(buf[:vppMagicSize], vppMagic) || buf[vppMagicSize] != VPPVersion {
		return ErrBadVPPHandshake
	}
	return nil
}
