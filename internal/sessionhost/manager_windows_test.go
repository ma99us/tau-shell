//go:build windows

package sessionhost

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSessionOutputWriterPreservesRawTerminalBytes(t *testing.T) {
	var got Event
	manager := &Manager{
		emit: func(event Event) {
			got = event
		},
	}
	input := []byte{0xf0, 0x9f, 0x8c, 0x8d, 0x1b, '[', '3', '1', 'm'}

	written, err := (sessionOutputWriter{manager: manager, id: "agent"}).Write(input)
	if err != nil {
		t.Fatal(err)
	}
	if written != len(input) {
		t.Fatalf("writer returned %d bytes, want %d", written, len(input))
	}
	decoded, err := base64.StdEncoding.DecodeString(got.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, input) {
		t.Fatalf("event changed terminal bytes: got %v, want %v", decoded, input)
	}
}
