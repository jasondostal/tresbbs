// Package protocol implements file transfer protocols for tresbbs.
//
// The original TriBBS supported five file transfer protocols:
//   - Xmodem (128-byte blocks, simple checksum)
//   - Xmodem-1K (1024-byte blocks, CRC-16)
//   - Ymodem (batch transfers with filename)
//   - Ymodem-G (go-ahead variant, streaming)
//   - Zmodem (resume support, batch, CRC-32, sliding window)
//
// These protocols were designed for serial modems with unreliable connections.
// They use error detection (checksums/CRC) and retransmission to ensure
// file integrity over noisy phone lines.
//
// For tresbbs, we implement these protocols over TCP. The core algorithms
// are the same — the transport layer is just more reliable now.
package protocol

import (
	"fmt"
	"io"
)

// Protocol identifiers matching the original TriBBS menu system.
const (
	ProtoAscii    byte = 'A'
	ProtoXmodem   byte = 'X'
	ProtoXmodem1K byte = 'K'
	ProtoYmodem   byte = 'Y'
	ProtoYmodemG  byte = 'G'
	ProtoZmodem   byte = 'Z'
)

// Protocol names for display.
var ProtocolNames = map[byte]string{
	ProtoAscii:    "Ascii",
	ProtoXmodem:   "Xmodem",
	ProtoXmodem1K: "Xmodem-1K",
	ProtoYmodem:   "Ymodem",
	ProtoYmodemG:  "Ymodem-G",
	ProtoZmodem:   "Zmodem",
}

// Xmodem control characters.
const (
	SOH  = 0x01 // Start of Header (128-byte block)
	STX  = 0x02 // Start of Header (1024-byte block, Xmodem-1K)
	EOT  = 0x04 // End of Transmission
	ACK  = 0x06 // Acknowledge
	NAK  = 0x15 // Negative Acknowledge
	CAN  = 0x18 // Cancel
	CRC  = 0x43 // 'C' — request CRC-16 mode
	SUB  = 0x1a // Substitute (padding)
)

// XmodemSender handles sending a file via Xmodem/Xmodem-1K.
type XmodemSender struct {
	conn     io.ReadWriter
	blockNum byte
	useCRC   bool
	use1K    bool
}

// NewXmodemSender creates a new Xmodem sender.
// If useCRC is true, uses CRC-16 instead of simple checksum.
// If use1K is true, uses 1024-byte blocks (Xmodem-1K).
func NewXmodemSender(conn io.ReadWriter, useCRC, use1K bool) *XmodemSender {
	return &XmodemSender{
		conn:     conn,
		blockNum: 1,
		useCRC:   useCRC,
		use1K:    use1K,
	}
}

// Send sends a file using Xmodem protocol.
func (s *XmodemSender) Send(data []byte) error {
	// Wait for receiver to send NAK or CRC request
	initByte, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for receiver: %w", err)
	}

	if initByte == CRC {
		s.useCRC = true
	} else if initByte != NAK {
		return fmt.Errorf("expected NAK or CRC, got %02x", initByte)
	}

	// Send data in blocks
	offset := 0
	for offset < len(data) {
		blockSize := 128
		if s.use1K {
			blockSize = 1024
		}

		// Get block data (pad with SUB if needed)
		end := offset + blockSize
		if end > len(data) {
			end = len(data)
		}
		block := make([]byte, blockSize)
		copy(block, data[offset:end])
		for i := end - offset; i < blockSize; i++ {
			block[i] = SUB
		}

		// Send block
		if err := s.sendBlock(block); err != nil {
			return fmt.Errorf("sending block %d: %w", s.blockNum, err)
		}

		// Wait for ACK
		ack, err := s.readByte()
		if err != nil {
			return fmt.Errorf("waiting for ACK: %w", err)
		}
		if ack == CAN {
			return fmt.Errorf("receiver cancelled")
		}
		if ack != ACK {
			// NAK — resend block
			continue
		}

		s.blockNum++
		offset = end
	}

	// Send EOT
	if _, err := s.conn.Write([]byte{EOT}); err != nil {
		return fmt.Errorf("sending EOT: %w", err)
	}

	// Wait for final ACK
	ack, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for final ACK: %w", err)
	}
	if ack != ACK {
		return fmt.Errorf("expected ACK for EOT, got %02x", ack)
	}

	return nil
}

// sendBlock sends a single Xmodem block.
func (s *XmodemSender) sendBlock(data []byte) error {
	// Header: SOH/STX, block number, complement
	header := []byte{SOH, s.blockNum, ^s.blockNum}
	if s.use1K {
		header[0] = STX
	}

	// Data
	block := append(header, data...)

	// Checksum/CRC (CRC-16 is big-endian on the wire)
	if s.useCRC {
		crc := crc16(data)
		block = append(block, byte(crc>>8), byte(crc))
	} else {
		checksum := simpleChecksum(data)
		block = append(block, checksum)
	}

	_, err := s.conn.Write(block)
	return err
}

// readByte reads a single byte from the connection.
func (s *XmodemSender) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(s.conn, buf)
	return buf[0], err
}

// simpleChecksum calculates the simple Xmodem checksum.
func simpleChecksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

// crc16 calculates CRC-16/XMODEM (polynomial 0x1021, init 0x0000, MSB-first) —
// the CRC used by Xmodem-CRC/Xmodem-1K and Ymodem. (The previous version
// returned the top 16 bits of a CRC-32, which is not a CRC-16 at all and gave
// no real error detection.)
func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// XmodemReceiver handles receiving a file via Xmodem/Xmodem-1K.
type XmodemReceiver struct {
	conn     io.ReadWriter
	blockNum byte
	useCRC   bool
}

// NewXmodemReceiver creates a new Xmodem receiver.
func NewXmodemReceiver(conn io.ReadWriter, useCRC bool) *XmodemReceiver {
	return &XmodemReceiver{
		conn:     conn,
		blockNum: 1,
		useCRC:   useCRC,
	}
}

// Receive receives a file using Xmodem protocol.
func (r *XmodemReceiver) Receive() ([]byte, error) {
	// Send CRC request or NAK to initiate transfer
	if r.useCRC {
		if _, err := r.conn.Write([]byte{CRC}); err != nil {
			return nil, fmt.Errorf("sending CRC request: %w", err)
		}
	} else {
		if _, err := r.conn.Write([]byte{NAK}); err != nil {
			return nil, fmt.Errorf("sending NAK: %w", err)
		}
	}

	var result []byte
	for {
		// Read header byte
		header, err := r.readByte()
		if err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}

		switch header {
		case SOH, STX:
			// Data block
			blockSize := 128
			if header == STX {
				blockSize = 1024
			}

			// Read block number and complement
			blockNum, err := r.readByte()
			if err != nil {
				return nil, fmt.Errorf("reading block number: %w", err)
			}
			if _, err = r.readByte(); err != nil { // complement
				return nil, fmt.Errorf("reading complement: %w", err)
			}

			// Read data
			data := make([]byte, blockSize)
			if _, err := io.ReadFull(r.conn, data); err != nil {
				return nil, fmt.Errorf("reading block data: %w", err)
			}

			// Read and verify the block's checksum/CRC.
			var valid bool
			if r.useCRC {
				crcBytes := make([]byte, 2)
				if _, err := io.ReadFull(r.conn, crcBytes); err != nil {
					return nil, fmt.Errorf("reading CRC: %w", err)
				}
				got := uint16(crcBytes[0])<<8 | uint16(crcBytes[1])
				valid = got == crc16(data)
			} else {
				checksum, err := r.readByte()
				if err != nil {
					return nil, fmt.Errorf("reading checksum: %w", err)
				}
				valid = checksum == simpleChecksum(data)
			}

			if !valid {
				// Corrupt block — ask the sender to retransmit it.
				if _, err := r.conn.Write([]byte{NAK}); err != nil {
					return nil, fmt.Errorf("sending NAK: %w", err)
				}
				continue
			}

			if blockNum == r.blockNum-1 {
				// Duplicate of the previous block (a lost ACK made the sender
				// resend) — re-ACK but don't store it again.
				if _, err := r.conn.Write([]byte{ACK}); err != nil {
					return nil, fmt.Errorf("sending ACK: %w", err)
				}
				continue
			}

			// Accept the new block.
			result = append(result, data...)
			r.blockNum++
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return nil, fmt.Errorf("sending ACK: %w", err)
			}

		case EOT:
			// End of transmission
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return nil, fmt.Errorf("sending final ACK: %w", err)
			}
			return result, nil

		case CAN:
			// Transfer cancelled
			return nil, fmt.Errorf("transfer cancelled by sender")

		default:
			// Unknown byte — send NAK
			if _, err := r.conn.Write([]byte{NAK}); err != nil {
				return nil, fmt.Errorf("sending NAK: %w", err)
			}
		}
	}
}

// Helper to read a single byte
func (r *XmodemReceiver) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r.conn, buf)
	return buf[0], err
}
