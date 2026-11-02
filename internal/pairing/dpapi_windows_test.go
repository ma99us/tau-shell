//go:build windows

package pairing

import (
	"bytes"
	"testing"
)

func TestDPAPISealerRoundTripAndHidesPlaintext(t *testing.T) {
	plain := []byte("a private key would be here")
	sealed, err := DPAPISealer{}.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, plain) {
		t.Fatal("sealed data must not contain the plaintext")
	}
	opened, err := DPAPISealer{}.Open(sealed)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("round trip: %q %v", opened, err)
	}
	sealed[len(sealed)/2] ^= 0xFF
	if _, err := (DPAPISealer{}).Open(sealed); err == nil {
		t.Fatal("tampered data must not open")
	}
}
