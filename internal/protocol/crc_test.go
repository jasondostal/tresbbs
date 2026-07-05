package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestCRC16XMODEM(t *testing.T) {
	// The canonical CRC-16/XMODEM check value: CRC of "123456789" is 0x31C3.
	if got := crc16([]byte("123456789")); got != 0x31C3 {
		t.Errorf("crc16(\"123456789\") = 0x%04X, want 0x31C3", got)
	}
	if got := crc16(nil); got != 0 {
		t.Errorf("crc16(empty) = 0x%04X, want 0", got)
	}
	// crc16 (protocol) and crc16Zmodem must agree — both are CRC-16/XMODEM.
	sample := []byte("The quick brown fox jumps over the lazy dog")
	if crc16(sample) != crc16Zmodem(sample) {
		t.Errorf("crc16 and crc16Zmodem disagree: 0x%04X vs 0x%04X", crc16(sample), crc16Zmodem(sample))
	}
}

func TestSimpleChecksum(t *testing.T) {
	// sum mod 256
	if got := simpleChecksum([]byte{0xFF, 0x01, 0x10}); got != 0x10 {
		t.Errorf("simpleChecksum = 0x%02X, want 0x10", got)
	}
}

// TestXmodemRoundTrip sends real data sender→receiver over an in-memory pipe and
// verifies it arrives intact, exercising the block framing + CRC verification.
func TestXmodemRoundTrip(t *testing.T) {
	for _, use1K := range []bool{false, true} {
		a, b := net.Pipe()
		data := bytes.Repeat([]byte("TresBBS xfer test. "), 70) // multi-block payload
		errc := make(chan error, 1)
		go func() {
			errc <- NewXmodemSender(a, use1K, true).Send(data)
			a.Close()
		}()
		got, err := NewXmodemReceiver(b, true).Receive()
		if err != nil {
			t.Fatalf("use1K=%v receive: %v", use1K, err)
		}
		if serr := <-errc; serr != nil {
			t.Fatalf("use1K=%v send: %v", use1K, serr)
		}
		// The last block is SUB-padded, so our payload is a prefix of the result.
		if !bytes.HasPrefix(got, data) {
			t.Errorf("use1K=%v: received %d bytes, not prefixed by the %d sent", use1K, len(got), len(data))
		}
	}
}
