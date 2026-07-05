// Package modem provides modem simulation for tresbbs.
//
// In the original TriBBS, the modem handled physical serial port
// communication, AT commands, baud rate negotiation, carrier detect,
// and connection management. In tresbbs, these are simulated for
// the telnet/SSH transport layer.
package modem

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// ModemState represents the current state of the modem connection.
type ModemState int

const (
	StateIdle ModemState = iota
	StateRinging
	StateConnecting
	StateConnected
	StateCarrierLost
	StateError
)

// BaudRate represents supported baud rates.
type BaudRate int

const (
	Baud300   BaudRate = 300
	Baud1200  BaudRate = 1200
	Baud2400  BaudRate = 2400
	Baud9600  BaudRate = 9600
	Baud14400 BaudRate = 14400
	Baud19200 BaudRate = 19200
	Baud38400 BaudRate = 38400
	Baud57600 BaudRate = 57600
)

// ModemConfig holds modem configuration.
type ModemConfig struct {
	Allow300   bool
	Allow1200  bool
	Allow2400  bool
	MaxBaud    BaudRate
	ModemInit  string // AT initialization string
	RingCount  int    // Number of rings before answer
}

// ModemSimulator simulates a modem connection for telnet/SSH sessions.
type ModemSimulator struct {
	config     ModemConfig
	state      ModemState
	baudRate   BaudRate
	carrier    bool
	ringCount  int
	connected  time.Time
	errorCorr  bool // Error-correcting mode (MNP/ARQ)
}

// NewModemSimulator creates a new modem simulator.
func NewModemSimulator(config ModemConfig) *ModemSimulator {
	return &ModemSimulator{
		config:  config,
		state:   StateIdle,
		carrier: false,
	}
}

// SimulateConnection simulates the modem connection sequence.
// Returns the negotiated baud rate and any error.
func (m *ModemSimulator) SimulateConnection(requestedBaud int) (BaudRate, error) {
	log.Printf("Modem: Incoming connection request at %d baud", requestedBaud)

	// Simulate ring detection
	m.state = StateRinging
	m.ringCount = 0
	for i := 0; i < m.config.RingCount; i++ {
		m.ringCount++
		log.Printf("Modem: Ring %d...", m.ringCount)
		time.Sleep(100 * time.Millisecond) // Simulate ring delay
	}

	// Check baud rate gating
	baud := BaudRate(requestedBaud)
	if !m.isBaudAllowed(baud) {
		m.state = StateError
		return 0, fmt.Errorf("baud rate %d not allowed", requestedBaud)
	}

	// Simulate carrier detect and negotiation
	m.state = StateConnecting
	log.Printf("Modem: Answering at %d baud...", baud)

	// Simulate connection time
	time.Sleep(200 * time.Millisecond)

	// Detect error-correcting modem (MNP/ARQ)
	m.errorCorr = baud >= 9600
	if m.errorCorr {
		log.Printf("Modem: Error-correcting mode (MNP/ARQ) enabled")
	}

	// Connection established
	m.state = StateConnected
	m.baudRate = baud
	m.carrier = true
	m.connected = time.Now()

	log.Printf("Modem: Connected at %d baud", baud)
	return baud, nil
}

// isBaudAllowed checks if a baud rate is allowed by configuration.
func (m *ModemSimulator) isBaudAllowed(baud BaudRate) bool {
	// Check against max baud
	if baud > m.config.MaxBaud {
		return false
	}

	// Check specific baud rate allowances
	switch baud {
	case Baud300:
		return m.config.Allow300
	case Baud1200:
		return m.config.Allow1200
	case Baud2400:
		return m.config.Allow2400
	default:
		// Higher baud rates are generally allowed
		return true
	}
}

// CheckCarrier checks if carrier is still present.
// Returns false if carrier has been lost.
func (m *ModemSimulator) CheckCarrier() bool {
	return m.carrier
}

// LostCarrier simulates losing the carrier signal.
func (m *ModemSimulator) LostCarrier() {
	m.carrier = false
	m.state = StateCarrierLost
	log.Printf("Modem: Carrier lost")
}

// GetBaudRate returns the current baud rate.
func (m *ModemSimulator) GetBaudRate() BaudRate {
	return m.baudRate
}

// GetState returns the current modem state.
func (m *ModemSimulator) GetState() ModemState {
	return m.state
}

// IsErrorCorrecting returns true if error-correcting mode is active.
func (m *ModemSimulator) IsErrorCorrecting() bool {
	return m.errorCorr
}

// GetConnectionDuration returns how long the connection has been active.
func (m *ModemSimulator) GetConnectionDuration() time.Duration {
	if m.state != StateConnected {
		return 0
	}
	return time.Since(m.connected)
}

// FormatBaudRate formats a baud rate for display.
func FormatBaudRate(baud BaudRate) string {
	switch baud {
	case Baud300:
		return "300"
	case Baud1200:
		return "1200"
	case Baud2400:
		return "2400"
	case Baud9600:
		return "9600"
	case Baud14400:
		return "14400"
	case Baud19200:
		return "19200"
	case Baud38400:
		return "38400"
	case Baud57600:
		return "57600"
	default:
		return fmt.Sprintf("%d", baud)
	}
}

// FormatModemState formats a modem state for display.
func FormatModemState(state ModemState) string {
	switch state {
	case StateIdle:
		return "Idle"
	case StateRinging:
		return "Ringing"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateCarrierLost:
		return "Carrier Lost"
	case StateError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ProcessATCommand processes an AT command string.
// Returns the modem response.
func (m *ModemSimulator) ProcessATCommand(cmd string) string {
	cmd = strings.ToUpper(strings.TrimSpace(cmd))

	if !strings.HasPrefix(cmd, "AT") {
		return "ERROR"
	}

	// Parse AT command
	cmdBody := cmd[2:]

	if cmdBody == "" || cmdBody == "Z" {
		return "OK"
	}

	if strings.HasPrefix(cmdBody, "DT") || strings.HasPrefix(cmdBody, "DP") {
		// Dial command
		return "CONNECT"
	}

	if cmdBody == "I0" || cmdBody == "I1" {
		return "tresbbs Modem Simulator v1.0"
	}

	if strings.HasPrefix(cmdBody, "S0=") {
		// Set auto-answer
		return "OK"
	}

	return "OK"
}
