package messages

import (
	"testing"

	"github.com/Wolf258/mvcp/protocol"
)

func TestSyncFilesystemsMessageRegistration(t *testing.T) {
	tests := []struct {
		name string
		typ  uint8
		msg  protocol.Message
		want any
	}{
		{
			name: "request",
			typ:  protocol.TypeSYNCFILESYSTEMS,
			msg:  &SyncFilesystemsMsg{},
			want: &SyncFilesystemsMsg{},
		},
		{
			name: "acknowledgement",
			typ:  protocol.TypeSYNCFILESYSTEMSACK,
			msg:  &SyncFilesystemsAckMsg{},
			want: &SyncFilesystemsAckMsg{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := tt.msg.MarshalBinary()
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if len(body) != 0 {
				t.Fatalf("body length: got %d, want 0", len(body))
			}

			decoded, err := protocol.DecodeMessageBody(tt.typ, body)
			if err != nil {
				t.Fatalf("decode registered message: %v", err)
			}
			switch tt.want.(type) {
			case *SyncFilesystemsMsg:
				if _, ok := decoded.(*SyncFilesystemsMsg); !ok {
					t.Fatalf("decoded type: got %T, want *SyncFilesystemsMsg", decoded)
				}
			case *SyncFilesystemsAckMsg:
				if _, ok := decoded.(*SyncFilesystemsAckMsg); !ok {
					t.Fatalf("decoded type: got %T, want *SyncFilesystemsAckMsg", decoded)
				}
			}
		})
	}
}

func TestSyncFilesystemsMessagesRejectPayload(t *testing.T) {
	for _, typ := range []uint8{protocol.TypeSYNCFILESYSTEMS, protocol.TypeSYNCFILESYSTEMSACK} {
		if _, err := protocol.DecodeMessageBody(typ, []byte{0x01}); err == nil {
			t.Fatalf("type 0x%02X accepted a non-empty payload", typ)
		}
	}
}

func TestSyncFilesystemsTypeAssignments(t *testing.T) {
	if protocol.TypeSYNCFILESYSTEMS != 0x40 {
		t.Fatalf("TypeSYNCFILESYSTEMS: got 0x%02X, want 0x40", protocol.TypeSYNCFILESYSTEMS)
	}
	if protocol.TypeSYNCFILESYSTEMSACK != 0x41 {
		t.Fatalf("TypeSYNCFILESYSTEMSACK: got 0x%02X, want 0x41", protocol.TypeSYNCFILESYSTEMSACK)
	}
}
