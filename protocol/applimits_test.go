package protocol

import "testing"

func TestValidAppService(t *testing.T) {
	ok := []string{"a", "minecraft", "my-service", "a1-b2"}
	bad := []string{"", "Minecraft", "1svc", "-svc", "svc_1", "a../b", string(make([]byte, 65))}
	for _, s := range ok {
		if !ValidAppService(s) {
			t.Fatalf("ValidAppService(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if ValidAppService(s) {
			t.Fatalf("ValidAppService(%q) = true, want false", s)
		}
	}
}

func TestAppLimitsAreCoherent(t *testing.T) {
	if AppMaxCredit != 4*AppInitialWindow {
		t.Fatalf("AppMaxCredit = %d, want 4x window", AppMaxCredit)
	}
	if AppMaxDataBytes > AppMaxSendQueue || AppMaxSendQueue > AppMaxBuffered {
		t.Fatal("app limits must be monotonically increasing: data <= queue <= buffered")
	}
}
