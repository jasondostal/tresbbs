package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Ymodem batch transfer constants.
const (
	ymodemBlockSize = 1024 // Ymodem always uses 1024-byte blocks
	ymodemHeaderNum = 0    // Block 0 is the filename/header block
)

// ymodemHeaderEntry represents a file entry in the Ymodem header block.
type ymodemHeaderEntry struct {
	Name     string
	Size     int
	ModTime  time.Time
}

// encodeYmodemHeader encodes a filename block (block 0) for Ymodem.
// Format: "filename<size> <modtime_octal> <mode_octal>\0" padded to 1024 bytes.
func encodeYmodemHeader(entry ymodemHeaderEntry) []byte {
	// Ymodem header: filename, space, size, space, modification time (octal)
	// e.g., "foo.txt 1234 1234567890\0"
	name := path.Base(entry.Name)
	sizeStr := strconv.Itoa(entry.Size)
	timeStr := strconv.FormatInt(entry.ModTime.Unix(), 8)

	header := name + " " + sizeStr + " " + timeStr
	if len(header) > ymodemBlockSize {
		header = header[:ymodemBlockSize]
	}

	// Pad with NUL to 1024 bytes
	block := make([]byte, ymodemBlockSize)
	copy(block, header)
	return block
}

// decodeYmodemHeader decodes a filename block (block 0) from Ymodem.
func decodeYmodemHeader(block []byte) ymodemHeaderEntry {
	// Find first NUL to get the header string
	nulIdx := len(block)
	for i, b := range block {
		if b == 0 {
			nulIdx = i
			break
		}
	}
	headerStr := string(block[:nulIdx])
	headerStr = strings.TrimRight(headerStr, " ")

	entry := ymodemHeaderEntry{
		ModTime: time.Now(),
	}

	parts := strings.SplitN(headerStr, " ", 3)
	if len(parts) >= 1 {
		entry.Name = parts[0]
	}
	if len(parts) >= 2 {
		if size, err := strconv.Atoi(parts[1]); err == nil {
			entry.Size = size
		}
	}
	if len(parts) >= 3 {
		if ts, err := strconv.ParseInt(parts[2], 8, 64); err == nil {
			entry.ModTime = time.Unix(ts, 0)
		}
	}

	return entry
}

// YmodemSender handles sending files via Ymodem protocol.
type YmodemSender struct {
	conn io.ReadWriter
}

// NewYmodemSender creates a new Ymodem sender.
func NewYmodemSender(conn io.ReadWriter) *YmodemSender {
	return &YmodemSender{conn: conn}
}

// SendBatch sends multiple files using Ymodem protocol.
// The map key is the filename and value is the file contents.
func (s *YmodemSender) SendBatch(files map[string][]byte) error {
	for name, data := range files {
		if err := s.sendFile(name, data); err != nil {
			return fmt.Errorf("sending %s: %w", name, err)
		}
	}

	// Send final EOT to signal end of batch
	// After all files, send an empty header block (block 0) to signal batch end
	if err := s.sendBatchEnd(); err != nil {
		return fmt.Errorf("sending batch end: %w", err)
	}

	return nil
}

// sendBatchEnd sends the end-of-batch signal (empty block 0).
func (s *YmodemSender) sendBatchEnd() error {
	// Wait for receiver to initiate (CRC or NAK)
	if _, err := s.readByte(); err != nil {
		return fmt.Errorf("waiting for receiver init: %w", err)
	}

	// Send empty block 0 to signal end of batch
	block := make([]byte, ymodemBlockSize)
	if err := s.sendDataBlock(ymodemHeaderNum, block); err != nil {
		return fmt.Errorf("sending end-of-batch block: %w", err)
	}

	// Wait for ACK
	ack, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for ACK: %w", err)
	}
	if ack != ACK {
		return fmt.Errorf("expected ACK, got %02x", ack)
	}

	// Send EOT
	if _, err := s.conn.Write([]byte{EOT}); err != nil {
		return fmt.Errorf("sending final EOT: %w", err)
	}

	return nil
}

// sendFile sends a single file using Ymodem protocol.
func (s *YmodemSender) sendFile(name string, data []byte) error {
	// Wait for receiver to initiate transfer (CRC or NAK)
	initByte, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for receiver: %w", err)
	}
	if initByte == CAN {
		return fmt.Errorf("receiver cancelled")
	}
	if initByte != CRC && initByte != NAK {
		return fmt.Errorf("expected CRC or NAK from receiver, got %02x", initByte)
	}

	// Build and send header block (block 0)
	header := encodeYmodemHeader(ymodemHeaderEntry{
		Name:    name,
		Size:    len(data),
		ModTime: time.Now(),
	})
	if err := s.sendDataBlock(ymodemHeaderNum, header); err != nil {
		return fmt.Errorf("sending header block: %w", err)
	}

	// Wait for ACK to header block
	ack, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for header ACK: %w", err)
	}
	if ack != ACK {
		return fmt.Errorf("expected ACK for header, got %02x", ack)
	}

	// Wait for CRC/NAK for data blocks
	dataInit, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for data init: %w", err)
	}
	if dataInit != CRC && dataInit != NAK {
		return fmt.Errorf("expected CRC or NAK for data, got %02x", dataInit)
	}

	// Send data in 1024-byte blocks
	blockNum := byte(1)
	offset := 0
	for offset < len(data) {
		end := offset + ymodemBlockSize
		if end > len(data) {
			end = len(data)
		}

		// Pad block with SUB
		block := make([]byte, ymodemBlockSize)
		copy(block, data[offset:end])

		if err := s.sendDataBlock(blockNum, block); err != nil {
			return fmt.Errorf("sending data block %d: %w", blockNum, err)
		}

		// Wait for ACK
		ack, err := s.readByte()
		if err != nil {
			return fmt.Errorf("waiting for data ACK: %w", err)
		}
		if ack == CAN {
			return fmt.Errorf("receiver cancelled")
		}
		if ack != ACK {
			return fmt.Errorf("expected ACK, got %02x", ack)
		}

		blockNum++
		offset = end
	}

	// Send EOT
	if _, err := s.conn.Write([]byte{EOT}); err != nil {
		return fmt.Errorf("sending EOT: %w", err)
	}

	// Wait for ACK
	ack, err = s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for EOT ACK: %w", err)
	}
	if ack != ACK {
		return fmt.Errorf("expected ACK for EOT, got %02x", ack)
	}

	return nil
}

// sendDataBlock sends a single Ymodem data block (always 1024 bytes with CRC-16).
func (s *YmodemSender) sendDataBlock(blockNum byte, data []byte) error {
	// Header: STX, block number, complement
	header := []byte{STX, blockNum, ^blockNum}

	// Build full packet
	packet := append(header, data...)

	// CRC-16
	crc := crc16(data)
	packet = append(packet, byte(crc>>8), byte(crc))

	_, err := s.conn.Write(packet)
	return err
}

// readByte reads a single byte from the connection.
func (s *YmodemSender) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(s.conn, buf)
	return buf[0], err
}

// YmodemReceiver handles receiving files via Ymodem protocol.
type YmodemReceiver struct {
	conn io.ReadWriter
}

// NewYmodemReceiver creates a new Ymodem receiver.
func NewYmodemReceiver(conn io.ReadWriter) *YmodemReceiver {
	return &YmodemReceiver{conn: conn}
}

// Receive receives one or more files using Ymodem protocol.
// Returns a map of filename to file contents.
func (r *YmodemReceiver) Receive() (map[string][]byte, error) {
	files := make(map[string][]byte)

	for {
		// Initiate transfer with CRC request
		if _, err := r.conn.Write([]byte{CRC}); err != nil {
			return files, fmt.Errorf("sending CRC request: %w", err)
		}

		// Read header byte
		header, err := r.readByte()
		if err != nil {
			return files, fmt.Errorf("reading header: %w", err)
		}

		switch header {
		case STX:
			// Read block 0 (filename header)
			blockNum, err := r.readByte()
			if err != nil {
				return files, fmt.Errorf("reading block number: %w", err)
			}
			if _, err := r.readByte(); err != nil { // complement
				return files, fmt.Errorf("reading complement: %w", err)
			}

			// Read 1024-byte data
			blockData := make([]byte, ymodemBlockSize)
			if _, err := io.ReadFull(r.conn, blockData); err != nil {
				return files, fmt.Errorf("reading block data: %w", err)
			}

			// Read CRC (2 bytes)
			if _, err := r.readCRC(); err != nil {
				return files, fmt.Errorf("reading CRC: %w", err)
			}

			// Send ACK
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return files, fmt.Errorf("sending ACK: %w", err)
			}

			if blockNum == ymodemHeaderNum {
				// Decode header
				entry := decodeYmodemHeader(blockData)

				if entry.Name == "" {
					// Empty header = end of batch
					return files, nil
				}

				// Receive file data
				fileData, err := r.receiveFileData()
				if err != nil {
					return files, fmt.Errorf("receiving %s: %w", entry.Name, err)
				}

				// Trim to declared size if we have it
				if entry.Size > 0 && entry.Size <= len(fileData) {
					fileData = fileData[:entry.Size]
				}

				files[entry.Name] = fileData
			}

		case EOT:
			// End of transmission (shouldn't happen at this level, but handle it)
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return files, fmt.Errorf("sending ACK: %w", err)
			}
			return files, nil

		case CAN:
			return files, fmt.Errorf("transfer cancelled by sender")

		default:
			// Unexpected byte, send NAK
			if _, err := r.conn.Write([]byte{NAK}); err != nil {
				return files, fmt.Errorf("sending NAK: %w", err)
			}
		}
	}
}

// receiveFileData receives the data blocks for a single file.
func (r *YmodemReceiver) receiveFileData() ([]byte, error) {
	var result []byte

	for {
		header, err := r.readByte()
		if err != nil {
			return result, fmt.Errorf("reading header: %w", err)
		}

		switch header {
		case STX:
			// 1024-byte data block
			blockNum, err := r.readByte()
			if err != nil {
				return result, fmt.Errorf("reading block number: %w", err)
			}
			if _, err := r.readByte(); err != nil { // complement
				return result, fmt.Errorf("reading complement: %w", err)
			}

			// Read data
			data := make([]byte, ymodemBlockSize)
			if _, err := io.ReadFull(r.conn, data); err != nil {
				return result, fmt.Errorf("reading block data: %w", err)
			}

			// Read CRC
			if _, err := r.readCRC(); err != nil {
				return result, fmt.Errorf("reading CRC: %w", err)
			}

			result = append(result, data...)
			_ = blockNum // TODO: verify block number

			// Send ACK
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return result, fmt.Errorf("sending ACK: %w", err)
			}

		case EOT:
			// End of file
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return result, fmt.Errorf("sending ACK: %w", err)
			}
			return result, nil

		case CAN:
			return result, fmt.Errorf("transfer cancelled")

		default:
			// Send NAK for unknown bytes
			if _, err := r.conn.Write([]byte{NAK}); err != nil {
				return result, fmt.Errorf("sending NAK: %w", err)
			}
		}
	}
}

// readByte reads a single byte from the connection.
func (r *YmodemReceiver) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r.conn, buf)
	return buf[0], err
}

// readCRC reads the 2-byte CRC from the connection.
func (r *YmodemReceiver) readCRC() (uint16, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r.conn, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf), nil
}

// YmodemGSender handles sending files via Ymodem-G protocol.
// Ymodem-G streams blocks without waiting for per-block ACK.
type YmodemGSender struct {
	conn io.ReadWriter
}

// NewYmodemGSender creates a new Ymodem-G sender.
func NewYmodemGSender(conn io.ReadWriter) *YmodemGSender {
	return &YmodemGSender{conn: conn}
}

// SendBatch sends multiple files using Ymodem-G protocol.
// The map key is the filename and value is the file contents.
func (s *YmodemGSender) SendBatch(files map[string][]byte) error {
	// Wait for receiver to send 'G' to start streaming
	gByte, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for G from receiver: %w", err)
	}
	if gByte != 'G' {
		return fmt.Errorf("expected 'G' from receiver, got %02x", gByte)
	}

	for name, data := range files {
		if err := s.sendFile(name, data); err != nil {
			return fmt.Errorf("sending %s: %w", name, err)
		}
	}

	// Send end-of-batch (empty header block)
	if err := s.sendBatchEnd(); err != nil {
		return fmt.Errorf("sending batch end: %w", err)
	}

	return nil
}

// sendBatchEnd sends the end-of-batch signal.
func (s *YmodemGSender) sendBatchEnd() error {
	// Send empty block 0 to signal end of batch
	block := make([]byte, ymodemBlockSize)
	if err := s.sendDataBlock(ymodemHeaderNum, block); err != nil {
		return fmt.Errorf("sending end-of-batch block: %w", err)
	}

	// Send EOT
	if _, err := s.conn.Write([]byte{EOT}); err != nil {
		return fmt.Errorf("sending final EOT: %w", err)
	}

	return nil
}

// sendFile sends a single file using Ymodem-G protocol.
func (s *YmodemGSender) sendFile(name string, data []byte) error {
	// Send header block (block 0) — no ACK wait in G mode
	header := encodeYmodemHeader(ymodemHeaderEntry{
		Name:    name,
		Size:    len(data),
		ModTime: time.Now(),
	})
	if err := s.sendDataBlock(ymodemHeaderNum, header); err != nil {
		return fmt.Errorf("sending header block: %w", err)
	}

	// Send data blocks without waiting for ACK
	blockNum := byte(1)
	offset := 0
	for offset < len(data) {
		end := offset + ymodemBlockSize
		if end > len(data) {
			end = len(data)
		}

		block := make([]byte, ymodemBlockSize)
		copy(block, data[offset:end])

		if err := s.sendDataBlock(blockNum, block); err != nil {
			return fmt.Errorf("sending data block %d: %w", blockNum, err)
		}

		blockNum++
		offset = end
	}

	// Send EOT
	if _, err := s.conn.Write([]byte{EOT}); err != nil {
		return fmt.Errorf("sending EOT: %w", err)
	}

	// Wait for ACK from receiver
	ack, err := s.readByte()
	if err != nil {
		return fmt.Errorf("waiting for EOT ACK: %w", err)
	}
	if ack != ACK {
		return fmt.Errorf("expected ACK for EOT, got %02x", ack)
	}

	return nil
}

// sendDataBlock sends a single data block (1024 bytes with CRC-16).
func (s *YmodemGSender) sendDataBlock(blockNum byte, data []byte) error {
	header := []byte{STX, blockNum, ^blockNum}
	packet := append(header, data...)
	crc := crc16(data)
	packet = append(packet, byte(crc>>8), byte(crc))
	_, err := s.conn.Write(packet)
	return err
}

// readByte reads a single byte from the connection.
func (s *YmodemGSender) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(s.conn, buf)
	return buf[0], err
}

// YmodemGReceiver handles receiving files via Ymodem-G protocol.
// Ymodem-G streams blocks without per-block ACKs.
type YmodemGReceiver struct {
	conn io.ReadWriter
}

// NewYmodemGReceiver creates a new Ymodem-G receiver.
func NewYmodemGReceiver(conn io.ReadWriter) *YmodemGReceiver {
	return &YmodemGReceiver{conn: conn}
}

// Receive receives one or more files using Ymodem-G protocol.
// Returns a map of filename to file contents.
func (r *YmodemGReceiver) Receive() (map[string][]byte, error) {
	files := make(map[string][]byte)

	// Send 'G' to start streaming mode
	if _, err := r.conn.Write([]byte{'G'}); err != nil {
		return files, fmt.Errorf("sending G: %w", err)
	}

	for {
		// Read header byte
		header, err := r.readByte()
		if err != nil {
			return files, fmt.Errorf("reading header: %w", err)
		}

		switch header {
		case STX:
			// Read block number and complement
			blockNum, err := r.readByte()
			if err != nil {
				return files, fmt.Errorf("reading block number: %w", err)
			}
			if _, err := r.readByte(); err != nil { // complement
				return files, fmt.Errorf("reading complement: %w", err)
			}

			// Read data
			data := make([]byte, ymodemBlockSize)
			if _, err := io.ReadFull(r.conn, data); err != nil {
				return files, fmt.Errorf("reading block data: %w", err)
			}

			// Read CRC
			if _, err := r.readCRC(); err != nil {
				return files, fmt.Errorf("reading CRC: %w", err)
			}

			if blockNum == ymodemHeaderNum {
				// Header block — decode filename
				entry := decodeYmodemHeader(data)

				if entry.Name == "" {
					// Empty header = end of batch
					// Send ACK and return
					if _, err := r.conn.Write([]byte{ACK}); err != nil {
						return files, fmt.Errorf("sending ACK: %w", err)
					}
					return files, nil
				}

				// Receive file data
				fileData, err := r.receiveFileData()
				if err != nil {
					return files, fmt.Errorf("receiving %s: %w", entry.Name, err)
				}

				// Trim to declared size
				if entry.Size > 0 && entry.Size <= len(fileData) {
					fileData = fileData[:entry.Size]
				}

				files[entry.Name] = fileData
			}

		case EOT:
			// End of file or batch
			// Send ACK
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return files, fmt.Errorf("sending ACK: %w", err)
			}

			// Check if more files follow
			next, err := r.readByte()
			if err != nil {
				// Connection closed, we're done
				return files, nil
			}
			if next == CAN {
				return files, fmt.Errorf("transfer cancelled")
			}
			if next == STX {
				// More files — push back by handling in next iteration
				// We need to handle this byte, so re-enter the loop
				// But we already consumed the byte. In G mode, we just continue.
				// The next iteration will handle the STX.
				// Actually, we need to handle this specially since we read ahead.
				// Let's handle the block directly.
				blockNum, err := r.readByte()
				if err != nil {
					return files, fmt.Errorf("reading block number: %w", err)
				}
				if _, err := r.readByte(); err != nil {
					return files, fmt.Errorf("reading complement: %w", err)
				}

				blockData := make([]byte, ymodemBlockSize)
				if _, err := io.ReadFull(r.conn, blockData); err != nil {
					return files, fmt.Errorf("reading block data: %w", err)
				}
				if _, err := r.readCRC(); err != nil {
					return files, fmt.Errorf("reading CRC: %w", err)
				}

				if blockNum == ymodemHeaderNum {
					entry := decodeYmodemHeader(blockData)
					if entry.Name == "" {
						if _, err := r.conn.Write([]byte{ACK}); err != nil {
							return files, fmt.Errorf("sending ACK: %w", err)
						}
						return files, nil
					}

					fileData, err := r.receiveFileData()
					if err != nil {
						return files, fmt.Errorf("receiving %s: %w", entry.Name, err)
					}
					if entry.Size > 0 && entry.Size <= len(fileData) {
						fileData = fileData[:entry.Size]
					}
					files[entry.Name] = fileData
				}
			}

		case CAN:
			return files, fmt.Errorf("transfer cancelled by sender")

		default:
			// In G mode, unexpected bytes might be errors
			// Send CAN to abort
			r.conn.Write([]byte{CAN})
			return files, fmt.Errorf("unexpected byte in G mode: %02x", header)
		}
	}
}

// receiveFileData receives data blocks for a single file in G mode.
func (r *YmodemGReceiver) receiveFileData() ([]byte, error) {
	var result []byte

	for {
		header, err := r.readByte()
		if err != nil {
			return result, fmt.Errorf("reading header: %w", err)
		}

		switch header {
		case STX:
			// 1024-byte data block
			blockNum, err := r.readByte()
			if err != nil {
				return result, fmt.Errorf("reading block number: %w", err)
			}
			if _, err := r.readByte(); err != nil { // complement
				return result, fmt.Errorf("reading complement: %w", err)
			}

			// Read data
			data := make([]byte, ymodemBlockSize)
			if _, err := io.ReadFull(r.conn, data); err != nil {
				return result, fmt.Errorf("reading block data: %w", err)
			}

			// Read CRC
			if _, err := r.readCRC(); err != nil {
				return result, fmt.Errorf("reading CRC: %w", err)
			}

			result = append(result, data...)
			_ = blockNum

			// No ACK in G mode — just keep reading

		case EOT:
			// End of file
			// Send ACK
			if _, err := r.conn.Write([]byte{ACK}); err != nil {
				return result, fmt.Errorf("sending ACK: %w", err)
			}
			return result, nil

		case CAN:
			return result, fmt.Errorf("transfer cancelled")

		default:
			// Unexpected byte in G mode — send CAN
			r.conn.Write([]byte{CAN})
			return result, fmt.Errorf("unexpected byte in G mode: %02x", header)
		}
	}
}

// readByte reads a single byte from the connection.
func (r *YmodemGReceiver) readByte() (byte, error) {
	buf := make([]byte, 1)
	_, err := io.ReadFull(r.conn, buf)
	return buf[0], err
}

// readCRC reads the 2-byte CRC from the connection.
func (r *YmodemGReceiver) readCRC() (uint16, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r.conn, buf); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(buf), nil
}
