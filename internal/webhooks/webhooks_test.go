package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestValidHMAC(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	mac := hmac.New(sha256.New, []byte("sekret"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !VerifySignatureForTest("sekret", body, sig) {
		t.Fatal("expected valid")
	}
	if VerifySignatureForTest("sekret", body, "deadbeef") {
		t.Fatal("expected invalid")
	}
}
