package messages

import (
	"bytes"
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

func decodeAppFrame(t *testing.T, typ uint8, body []byte) (protocol.Message, error) {
	t.Helper()
	var buf bytes.Buffer
	if err := protocol.WriteMVCPFrame(&buf, &protocol.Frame{Type: typ, Body: body}); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	frame, err := protocol.ReadMVCPFrame(&buf)
	if err != nil {
		return nil, err
	}
	return protocol.DecodeMessage(frame.Type, bytes.NewReader(frame.Body))
}

func TestAppMessageTypeBytes(t *testing.T) {
	cases := map[uint8]uint8{
		0x50: protocol.TypeAPPOPEN, 0x51: protocol.TypeAPPACCEPT, 0x52: protocol.TypeAPPREJECT,
		0x53: protocol.TypeAPPDATA, 0x54: protocol.TypeAPPCREDIT, 0x55: protocol.TypeAPPCLOSE, 0x56: protocol.TypeAPPRESET,
	}
	for want, got := range cases {
		if got != want {
			t.Fatalf("type byte = 0x%02X, want 0x%02X", got, want)
		}
	}
}

func roundTripApp(t *testing.T, typ uint8, msg protocol.Message) protocol.Message {
	t.Helper()
	body, err := msg.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded, err := decodeAppFrame(t, typ, body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func TestAppMessagesRoundTrip(t *testing.T) {
	open := roundTripApp(t, protocol.TypeAPPOPEN, &AppOpen{StreamID: 0x80000001, Service: "minecraft", Meta: []byte{1, 2}}).(*AppOpen)
	if open.StreamID != 0x80000001 || open.Service != "minecraft" || !bytes.Equal(open.Meta, []byte{1, 2}) {
		t.Fatalf("AppOpen round trip = %+v", open)
	}
	accept := roundTripApp(t, protocol.TypeAPPACCEPT, &AppAccept{StreamID: 3, Meta: []byte("m"), Grant: 1024}).(*AppAccept)
	if accept.Grant != 1024 || !bytes.Equal(accept.Meta, []byte("m")) {
		t.Fatalf("AppAccept round trip = %+v", accept)
	}
	if r := roundTripApp(t, protocol.TypeAPPREJECT, &AppReject{StreamID: 3, Code: protocol.ErrorCodeAppNotAuthorized, Message: "no"}).(*AppReject); r.Code != protocol.ErrorCodeAppNotAuthorized {
		t.Fatalf("AppReject round trip = %+v", r)
	}
	if d := roundTripApp(t, protocol.TypeAPPDATA, &AppData{StreamID: 3, Data: []byte("x")}).(*AppData); !bytes.Equal(d.Data, []byte("x")) {
		t.Fatalf("AppData round trip = %+v", d)
	}
	if c := roundTripApp(t, protocol.TypeAPPCREDIT, &AppCredit{StreamID: 3, Bytes: 99}).(*AppCredit); c.Bytes != 99 {
		t.Fatalf("AppCredit round trip = %+v", c)
	}
	if cl := roundTripApp(t, protocol.TypeAPPCLOSE, &AppClose{StreamID: 3}).(*AppClose); cl.StreamID != 3 {
		t.Fatalf("AppClose round trip = %+v", cl)
	}
	if r := roundTripApp(t, protocol.TypeAPPRESET, &AppReset{StreamID: 3, Code: protocol.ErrorCodeAppPeerGone, Message: "gone"}).(*AppReset); r.Code != protocol.ErrorCodeAppPeerGone {
		t.Fatalf("AppReset round trip = %+v", r)
	}
}

func TestAppDataLimits(t *testing.T) {
	tooBig := &AppData{StreamID: 1, Data: make([]byte, protocol.AppMaxDataBytes+1)}
	if _, err := tooBig.MarshalBinary(); err == nil {
		t.Fatal("AppData.MarshalBinary accepted >64KiB")
	}
	tooBigMeta := &AppOpen{StreamID: 1, Service: "svc", Meta: make([]byte, protocol.AppMaxMetaBytes+1)}
	if _, err := tooBigMeta.MarshalBinary(); err == nil {
		t.Fatal("AppOpen.MarshalBinary accepted >1KiB meta")
	}
}

func TestAppDecodeRejectsTrailingBytes(t *testing.T) {
	body, _ := (&AppClose{StreamID: 1}).MarshalBinary()
	body = append(body, 0xFF)
	if _, err := decodeAppFrame(t, protocol.TypeAPPCLOSE, body); err == nil {
		t.Fatal("decode accepted trailing bytes")
	}
}
