package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
)

// executeDoor runs a door program with proper drop file generation.
func (s *Session) executeDoor(door domain.Door) {
	s.display.Clear()
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("Launching: %s", door.Name))
	s.display.ResetColor()

	// Check door time limit
	if door.TimeLimit > 0 && s.user.TimeLeftToday < door.TimeLimit {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Not enough time! Need %d min, have %d min.", door.TimeLimit, s.user.TimeLeftToday))
		s.pause()
		return
	}

	// Log the door execution
	startTime := time.Now()
	s.storage.LogCaller(fmt.Sprintf("Executed door: %s at %s.", door.Name, startTime.Format("15:04")))

	// Write drop files
	dropDir := filepath.Join(os.TempDir(), "tresbbs")
	os.MkdirAll(dropDir, 0755)

	// Write drop files. Like TriBBS, we write several formats for door
	// compatibility; the door's DropFormat can restrict to one (empty/"ALL"
	// writes them all).
	s.writeDropFiles(dropDir, door.DropFormat)

	// Execute the door command
	if door.Command != "" {
		cmd := exec.Command("sh", "-c", door.Command)
		cmd.Dir = dropDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Door error: %v", err))
			s.pause()
		}
	} else {
		s.display.SetColor('C')
		s.display.WriteLine("No command configured for this door.")
		s.pause()
	}

	// Calculate time used
	endTime := time.Now()
	minutesUsed := int(endTime.Sub(startTime).Minutes())

	// Re-read drop files for changes
	s.readDoorSys(filepath.Join(dropDir, "DOOR.SYS"))

	// Deduct time
	if door.TimeLimit > 0 {
		if minutesUsed > door.TimeLimit {
			minutesUsed = door.TimeLimit
		}
		s.user.TimeLeftToday -= minutesUsed
		if s.user.TimeLeftToday < 0 {
			s.user.TimeLeftToday = 0
		}
		s.storage.SaveUser(s.user)
	}

	// Log return
	s.storage.LogCaller(fmt.Sprintf("Returned from door at %s.", endTime.Format("15:04")))
	_ = minutesUsed

	// Clean up
	os.RemoveAll(dropDir)
}

// writeDoorSys writes a DOOR.SYS drop file.
func (s *Session) writeDoorSys(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	// DOOR.SYS format (standard fields, one per line)
	fmt.Fprintf(f, "COM%d\n", s.nodeNum)             // COM port
	fmt.Fprintf(f, "0\n")                              // Baud rate (0 for local)
	fmt.Fprintf(f, "8\n")                              // Data bits
	fmt.Fprintf(f, "1\n")                              // Stop bits
	fmt.Fprintf(f, "N\n")                              // Parity
	fmt.Fprintf(f, "%d\n", s.nodeNum)                  // Node number
	fmt.Fprintf(f, "Y\n")                              // Screen display
	fmt.Fprintf(f, "N\n")                              // Printer
	fmt.Fprintf(f, "Y\n")                              // Page bell
	fmt.Fprintf(f, "Y\n")                              // Caller alarm
	fmt.Fprintf(f, "%s\n", s.user.Name)                // User name
	fmt.Fprintf(f, "%s\n", s.user.Alias)               // User alias
	fmt.Fprintf(f, "%s\n", s.user.City)                // City
	fmt.Fprintf(f, "%s\n", s.user.Password)            // Password
	fmt.Fprintf(f, "%d\n", s.user.SecurityLevel)       // Security level
	fmt.Fprintf(f, "%d\n", s.user.TotalCalls)          // Total calls
	fmt.Fprintf(f, "%d\n", s.user.FilesUploaded)       // Uploads
	fmt.Fprintf(f, "%d\n", s.user.FilesDownloaded)     // Downloads
	fmt.Fprintf(f, "%d\n", s.user.KUploaded)           // KB uploaded
	fmt.Fprintf(f, "%d\n", s.user.KDownloaded)         // KB downloaded
	fmt.Fprintf(f, "%d\n", s.user.TimeLeftToday)       // Minutes left
	fmt.Fprintf(f, "%s\n", time.Now().Format("01/02/06")) // Date
	fmt.Fprintf(f, "%s\n", time.Now().Format("03:04"))    // Time
	fmt.Fprintf(f, "%d\n", 0)                          // Fossil (0=no)
	fmt.Fprintf(f, "GR\n")                             // ANSI mode
}

// writeDorinfoDef writes a DORINFO1.DEF drop file.
func (s *Session) writeDorinfoDef(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	// DORINFO1.DEF format
	fmt.Fprintf(f, "%s\n", s.config.BoardName)         // BBS name
	fmt.Fprintf(f, "%s\n", s.config.SysopName)         // Sysop first name
	fmt.Fprintf(f, "\n")                                // Sysop last name
	fmt.Fprintf(f, "COM%d\n", s.nodeNum)               // COM port
	fmt.Fprintf(f, "0,N,8,1\n")                         // Baud,N,8,1
	fmt.Fprintf(f, "%d\n", s.nodeNum)                  // Node
	fmt.Fprintf(f, "%s\n", s.user.Name)                // User first name
	fmt.Fprintf(f, "\n")                                // User last name
	fmt.Fprintf(f, "%s\n", s.user.Alias)               // User alias
	fmt.Fprintf(f, "%d\n", s.user.SecurityLevel)       // Security level
	fmt.Fprintf(f, "%d\n", s.user.TimeLeftToday)       // Minutes left
	fmt.Fprintf(f, "%d\n", map[bool]int{true: 1, false: 0}[s.user.ANSIMode > 0]) // ANSI
	fmt.Fprintf(f, "%d\n", s.user.ScreenWidth)         // Screen width
	fmt.Fprintf(f, "1\n")                               // Expert mode
}

// readDoorSys reads back a DOOR.SYS after door execution.
func (s *Session) readDoorSys(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Read updated fields (like time remaining)
	var lines []string
	var line string
	for {
		_, err := fmt.Fscanf(f, "%s\n", &line)
		if err != nil {
			break
		}
		lines = append(lines, line)
	}

	// Update time remaining if the door changed it
	if len(lines) > 20 {
		var timeLeft int
		fmt.Sscanf(lines[20], "%d", &timeLeft)
		if timeLeft > 0 && timeLeft != s.user.TimeLeftToday {
			s.user.TimeLeftToday = timeLeft
			s.storage.SaveUser(s.user)
		}
	}
}

// writeCallInfoBbs writes a CALLINFO.BBS drop file.
func (s *Session) writeCallInfoBbs(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	// CALLINFO.BBS format (PCBoard compatible)
	fmt.Fprintf(f, "%s\n", s.user.Name)
	fmt.Fprintf(f, "%s\n", s.user.Alias)
	fmt.Fprintf(f, "COM%d\n", s.nodeNum)
	fmt.Fprintf(f, "%d\n", 0) // Baud rate (0 for local)
	fmt.Fprintf(f, "%d\n", 8) // Data bits
	fmt.Fprintf(f, "N\n") // Parity
	fmt.Fprintf(f, "%d\n", 1) // Stop bits
	fmt.Fprintf(f, "%d\n", s.user.SecurityLevel)
	fmt.Fprintf(f, "%d\n", s.user.TotalCalls)
	fmt.Fprintf(f, "%d\n", s.user.TimeLeftToday)
	fmt.Fprintf(f, "%s\n", time.Now().Format("01/02/06"))
	fmt.Fprintf(f, "%s\n", time.Now().Format("03:04"))
	fmt.Fprintf(f, "%d\n", s.user.ScreenWidth)
	fmt.Fprintf(f, "%d\n", map[bool]int{true: 1, false: 0}[s.user.ANSIMode > 0])
}

// writeDoor32Sys writes a DOOR32.SYS drop file for 32-bit doors (v11.6+).
// DOOR32.SYS is an extended format that supports 32-bit Windows door programs.
func (s *Session) writeDoor32Sys(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	// DOOR32.SYS format (extended fields for 32-bit doors)
	fmt.Fprintf(f, "DOOR32.SYS\n")                // File identifier
	fmt.Fprintf(f, "%d\n", 1)                      // Version
	fmt.Fprintf(f, "COM%d\n", s.nodeNum)            // COM port
	fmt.Fprintf(f, "%d\n", 0)                       // Baud rate (0 for local/TCP)
	fmt.Fprintf(f, "%d\n", 8)                       // Data bits
	fmt.Fprintf(f, "N\n")                            // Parity
	fmt.Fprintf(f, "%d\n", 1)                       // Stop bits
	fmt.Fprintf(f, "%d\n", s.nodeNum)               // Node number
	fmt.Fprintf(f, "%s\n", s.user.Name)             // User name
	fmt.Fprintf(f, "%s\n", s.user.Alias)            // User alias
	fmt.Fprintf(f, "%s\n", s.user.City)             // City
	fmt.Fprintf(f, "%s\n", s.user.Password)         // Password (plaintext in original)
	fmt.Fprintf(f, "%d\n", s.user.SecurityLevel)    // Security level
	fmt.Fprintf(f, "%d\n", s.user.TotalCalls)       // Total calls
	fmt.Fprintf(f, "%d\n", s.user.FilesUploaded)    // Uploads
	fmt.Fprintf(f, "%d\n", s.user.FilesDownloaded)  // Downloads
	fmt.Fprintf(f, "%d\n", s.user.KUploaded)        // KB uploaded
	fmt.Fprintf(f, "%d\n", s.user.KDownloaded)      // KB downloaded
	fmt.Fprintf(f, "%d\n", s.user.TimeLeftToday)    // Minutes left
	fmt.Fprintf(f, "%s\n", time.Now().Format("01/02/06 03:04:05 PM"))  // Date/time
	fmt.Fprintf(f, "%d\n", s.user.ScreenWidth)      // Screen width
	fmt.Fprintf(f, "%d\n", s.user.ANSIMode)         // ANSI mode (0=none, 1=ANSI, 2=RIP)
	fmt.Fprintf(f, "%s\n", s.config.BoardName)      // BBS name
	fmt.Fprintf(f, "%s\n", s.config.SysopName)      // Sysop name
}

// writeUtiDoorTxt writes a UTIDOOR.TXT drop file.
func (s *Session) writeUtiDoorTxt(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	// UTIDOOR.TXT format (UTI door standard)
	fmt.Fprintf(f, "COM%d:\n", s.nodeNum)
	fmt.Fprintf(f, "%d:\n", 0) // Baud rate
	fmt.Fprintf(f, "8:N:1\n")
	fmt.Fprintf(f, "%d:\n", s.user.SecurityLevel)
	fmt.Fprintf(f, "%s:\n", s.user.Name)
	fmt.Fprintf(f, "%s:\n", s.user.Alias)
	fmt.Fprintf(f, "%d:\n", s.user.FilesDownloaded)
	fmt.Fprintf(f, "%d:\n", s.user.FilesUploaded)
	fmt.Fprintf(f, "%d:\n", s.user.KDownloaded)
	fmt.Fprintf(f, "%d:\n", s.user.KUploaded)
	fmt.Fprintf(f, "%d:\n", s.user.TotalCalls)
	fmt.Fprintf(f, "%d:\n", s.user.TimeLeftToday)
	fmt.Fprintf(f, "%s:\n", time.Now().Format("01/02/06"))
	fmt.Fprintf(f, "%s:\n", time.Now().Format("03:04"))
	fmt.Fprintf(f, "%d:\n", s.user.ScreenWidth)
	fmt.Fprintf(f, "%d:\n", map[bool]int{true: 1, false: 0}[s.user.ANSIMode > 0])
}

// writeDropFiles writes the door drop file(s) selected by a door's DropFormat.
// DOOR.SYS (near-universal) is always written; an empty or "ALL" format writes
// every supported format for maximum door compatibility, as TriBBS did.
func (s *Session) writeDropFiles(dir, format string) {
	format = strings.ToUpper(strings.TrimSpace(format))
	all := format == "" || format == "ALL"
	s.writeDoorSys(filepath.Join(dir, "DOOR.SYS"))
	if all || format == "DORINFO" {
		s.writeDorinfoDef(filepath.Join(dir, "DORINFO1.DEF"))
	}
	if all || format == "CALLINFO" {
		s.writeCallInfoBbs(filepath.Join(dir, "CALLINFO.BBS"))
	}
	if all || format == "UTIDOOR" {
		s.writeUtiDoorTxt(filepath.Join(dir, "UTIDOOR.TXT"))
	}
	if all || format == "TRIBBS" {
		s.writeTribbsSys(filepath.Join(dir, "TRIBBS.SYS"))
	}
	if all || format == "SFDOORS" {
		s.writeSfdoorsDat(filepath.Join(dir, "SFDOORS.DAT"))
	}
	// DOOR32.SYS is our modern telnet-door format (was defined but unwired).
	if all || format == "DOOR32" {
		s.writeDoor32Sys(filepath.Join(dir, "DOOR32.SYS"))
	}
}

// writeTribbsSys writes TriBBS's native TRIBBS.SYS drop file.
//
// NOTE: TriBBS's native TRIBBS.SYS byte layout isn't publicly documented. This
// is a best-effort line-oriented reconstruction carrying the same user/session
// data as the other
// drop files, so TRIBBS.SYS-aware doors have data to read. Correct against a
// real sample if one surfaces.
func (s *Session) writeTribbsSys(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	yn := func(b bool) string {
		if b {
			return "Y"
		}
		return "N"
	}
	cityState := s.user.City
	if s.user.State != "" {
		if cityState != "" {
			cityState += ", "
		}
		cityState += s.user.State
	}
	baud := 0
	fmt.Sscanf(s.baudRate, "%d", &baud)

	// TRIBBS.SYS — 20 CRLF-terminated ASCII lines. Fields 1-19 per the TriBBS
	// manual; field 20 (RIPscrip Y/N) is a 5.01 addition the manual omits,
	// recovered by capturing a real TRIBBS.SYS from TriBBS 5.01 in DOSBox
	// (which also corrected line 12 serial-port=0 and line 16 error-correcting=N).
	fmt.Fprintf(f, "%d\r\n", s.user.RecordNumber)     // 1  record number
	fmt.Fprintf(f, "%s\r\n", s.user.Name)             // 2  name
	fmt.Fprintf(f, "%s\r\n", s.user.Password)         // 3  password (bcrypt hash here; TriBBS stored plaintext)
	fmt.Fprintf(f, "%d\r\n", s.user.SecurityLevel)    // 4  security level
	fmt.Fprintf(f, "%s\r\n", yn(false))               // 5  Y=Expert N=Novice
	fmt.Fprintf(f, "%s\r\n", yn(s.user.ANSIMode > 0)) // 6  Y=ANSI N=monochrome
	fmt.Fprintf(f, "%d\r\n", s.user.TimeLeftToday)    // 7  minutes left this call
	fmt.Fprintf(f, "%s\r\n", s.user.Phone)            // 8  phone number
	fmt.Fprintf(f, "%s\r\n", cityState)               // 9  city and state
	fmt.Fprintf(f, "%s\r\n", s.user.BirthDate)        // 10 birth date
	fmt.Fprintf(f, "%d\r\n", s.nodeNum)                // 11 node number
	fmt.Fprintf(f, "0\r\n")                            // 12 serial port (0 = local)
	fmt.Fprintf(f, "%d\r\n", baud)                     // 13 baud rate (0 = local)
	fmt.Fprintf(f, "0\r\n")                            // 14 locked rate (0 = not locked)
	fmt.Fprintf(f, "%s\r\n", yn(false))                // 15 RTS/CTS
	fmt.Fprintf(f, "%s\r\n", yn(false))                // 16 error correcting
	fmt.Fprintf(f, "%s\r\n", s.config.BoardName)       // 17 board name
	fmt.Fprintf(f, "%s\r\n", s.config.SysopName)       // 18 sysop name
	fmt.Fprintf(f, "%s\r\n", s.user.Alias)             // 19 alias
	fmt.Fprintf(f, "%s\r\n", yn(s.user.ANSIMode == 2)) // 20 RIPscrip Y/N (5.01 field; absent from the 4.0 manual, confirmed by live capture)
}

// writeSfdoorsDat writes a SpitFire-style SFDOORS.DAT drop file.
//
// NOTE: reconstructed best-effort — the exact SpitFire SFDOORS.DAT field layout
// isn't in our specs; carries standard user/session fields for SFDOORS-aware
// doors.
func (s *Session) writeSfdoorsDat(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	tf := func(b bool) string {
		if b {
			return "TRUE"
		}
		return "FALSE"
	}
	first := s.user.Name
	if i := strings.IndexByte(first, ' '); i > 0 {
		first = first[:i]
	}
	cityState := s.user.City
	if s.user.State != "" {
		if cityState != "" {
			cityState += ", "
		}
		cityState += s.user.State
	}
	baud := 0
	fmt.Sscanf(s.baudRate, "%d", &baud)
	now := time.Now()
	secs := now.Hour()*3600 + now.Minute()*60 + now.Second() // seconds since midnight

	// SFDOORS.DAT — 32 CRLF ASCII lines. SpitFire deliberately never published
	// this record structure ("contact Buffalo Creek"); this layout was recovered
	// by capturing a real file from TriBBS 5.01 (door type S) in DOSBox. Fields
	// marked (stat/limit/flag) are reproduced with TriBBS's captured defaults;
	// the identity/time/security/baud/contact fields are populated live.
	fmt.Fprintf(f, "%d\r\n", s.user.RecordNumber)      // 1  record number
	fmt.Fprintf(f, "%s\r\n", s.user.Name)              // 2  full name
	fmt.Fprintf(f, "%s\r\n", s.user.Password)          // 3  password (bcrypt hash here; SpitFire stored plaintext)
	fmt.Fprintf(f, "%s\r\n", first)                    // 4  first name
	fmt.Fprintf(f, "%d\r\n", s.user.FilesUploaded)     // 5  uploads
	fmt.Fprintf(f, "%d\r\n", s.user.FilesDownloaded)   // 6  downloads
	fmt.Fprintf(f, "%d\r\n", s.user.TotalCalls)        // 7  (stat)
	fmt.Fprintf(f, "%d\r\n", secs)                     // 8  current time (seconds since midnight)
	fmt.Fprintf(f, "C:\\TRIBBS\\\r\n")                 // 9  BBS home path
	fmt.Fprintf(f, "%s\r\n", tf(s.user.ANSIMode > 0))  // 10 ANSI graphics
	fmt.Fprintf(f, "%d\r\n", s.user.SecurityLevel)     // 11 security level
	fmt.Fprintf(f, "0\r\n")                            // 12 (stat)
	fmt.Fprintf(f, "0\r\n")                            // 13 (stat)
	fmt.Fprintf(f, "%d\r\n", s.user.TimeLeftToday)     // 14 minutes left this call
	fmt.Fprintf(f, "%d\r\n", secs)                     // 15 (time)
	fmt.Fprintf(f, "0\r\n")                            // 16 (stat)
	fmt.Fprintf(f, "FALSE\r\n")                        // 17 (flag)
	fmt.Fprintf(f, "FALSE\r\n")                        // 18 (flag)
	fmt.Fprintf(f, "TRUE\r\n")                         // 19 (flag)
	fmt.Fprintf(f, "%d\r\n", baud)                     // 20 baud rate
	fmt.Fprintf(f, "FALSE\r\n")                        // 21 (flag: error correcting)
	fmt.Fprintf(f, "0\r\n")                            // 22 (stat)
	fmt.Fprintf(f, "0\r\n")                            // 23 (stat)
	fmt.Fprintf(f, "%d\r\n", s.nodeNum)                // 24 node number
	fmt.Fprintf(f, "100\r\n")                          // 25 (limit)
	fmt.Fprintf(f, "0\r\n")                            // 26 (stat)
	fmt.Fprintf(f, "1000000\r\n")                      // 27 (daily byte limit)
	fmt.Fprintf(f, "0\r\n")                            // 28 (stat)
	fmt.Fprintf(f, "0\r\n")                            // 29 (stat)
	fmt.Fprintf(f, "0\r\n")                            // 30 (stat)
	fmt.Fprintf(f, "%s\r\n", s.user.Phone)             // 31 phone number
	fmt.Fprintf(f, "%s\r\n", cityState)                // 32 city and state
}
