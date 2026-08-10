// Package protocol implements the MVCP binary wire protocol: framing,
// message encoding/decoding, and message registration.
package protocol

import (
	"bytes"
	"errors"
	"io"
)

const (
	MVCPVersion uint8 = 0x01
	VPPVersion  uint8 = 0x01

	mvcpMagicSize = 4
	vppMagicSize  = 3
)

var (
	mvcpMagic = []byte{'M', 'V', 'C', 'P'}
	vppMagic  = []byte{'V', 'P', 'P'}

	ErrBadMVCPHandshake = errors.New("mvcp: bad handshake magic or version")
	ErrBadVPPHandshake  = errors.New("vpp: bad handshake magic or version")
)

func WriteMVCPHandshake(w io.Writer) error {
	handshake := [5]byte{'M', 'V', 'C', 'P', MVCPVersion}
	_, err := w.Write(handshake[:])
	return err
}

func WriteVPPHandshake(w io.Writer) error {
	handshake := [4]byte{'V', 'P', 'P', VPPVersion}
	_, err := w.Write(handshake[:])
	return err
}

func ValidateMVCPHandshake(r io.Reader) error {
	var buf [5]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return err
	}
	if !bytes.Equal(buf[:mvcpMagicSize], mvcpMagic) || buf[mvcpMagicSize] != MVCPVersion {
		return ErrBadMVCPHandshake
	}
	return nil
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
