// Package domain is the TriBBS simulation core: users, messages, files, and
// the BBS state. It has NO knowledge of how a session is transported or
// rendered — that lives behind the port interfaces. Everything BBS-y hangs
// off the edges, never here.
//
// This is a faithful recreation of TriBBS 5.01's data model, extracted from
// the original binary's string analysis. Field names, sizes, and semantics
// match the original as closely as possible.
package domain

import "time"

// ===================================================================
// User Record
//
// This is the core user structure. Field names and sizes come from
// the TRIMAN admin tool's display strings (e.g., "User Name...: %-30s"
// tells us the name field is 30 characters).
//
// In the original TriBBS, this was a fixed-length binary record in
// USERS.DAT. In tresbbs, it's stored in SQLite with the same semantics.
// ===================================================================

type User struct {
	// Identity — from "User Name...: %s", "Alias Name..: %s", etc.
	Name     string `json:"name" db:"name"`         // 30 chars max
	Alias    string `json:"alias" db:"alias"`        // 30 chars max
	Password string `json:"-" db:"password_hash"`    // never serialize
	Phone    string `json:"phone,omitempty" db:"phone"`
	City     string `json:"city,omitempty" db:"city"`
	BirthDate string `json:"birthdate,omitempty" db:"birthdate"`
	Address  string `json:"address,omitempty" db:"address"`

	// v11.6+ fields — extended address and contact info
	Email         string `json:"email,omitempty" db:"email"`                   // @EMAILADDRESS
	StreetAddress string `json:"street_address,omitempty" db:"street_address"` // @STREETADDRESS
	State         string `json:"state,omitempty" db:"state"`                   // @STATE
	ZipCode       string `json:"zipcode,omitempty" db:"zipcode"`               // @ZIPCODE
	Country       string `json:"country,omitempty" db:"country"`               // @COUNTRY

	// Caller ID (v11.6+)
	CallerID      string `json:"caller_id,omitempty" db:"caller_id"`             // Phone number from caller ID
	BlockCallerID bool   `json:"block_caller_id" db:"block_caller_id"`           // User blocks their caller ID

	// Security — from "Security Level..........: %-5d"
	SecurityLevel  int    `json:"security_level" db:"security_level"`
	NodeSecurity   int    `json:"node_security" db:"node_security"`
	Registration   string `json:"registration" db:"registration"`
	Subscription   *time.Time `json:"subscription,omitempty" db:"subscription"`
	LockedOut      bool   `json:"locked_out" db:"locked_out"`
	Deleted        bool   `json:"deleted" db:"deleted"`

	// Statistics — from "Calls    : %-4d", "Messages....: %ld", etc.
	TotalCalls      int64 `json:"total_calls" db:"total_calls"`
	MessagesPosted  int64 `json:"messages_posted" db:"messages_posted"`
	FilesDownloaded int64 `json:"files_downloaded" db:"files_downloaded"`
	FilesUploaded   int64 `json:"files_uploaded" db:"files_uploaded"`
	KDownloaded     int64 `json:"k_downloaded" db:"k_downloaded"`
	KUploaded       int64 `json:"k_uploaded" db:"k_uploaded"`
	DownloadsToday  int   `json:"downloads_today" db:"downloads_today"`
	UploadsToday    int   `json:"uploads_today" db:"uploads_today"`
	MessagesToday   int   `json:"messages_today" db:"messages_today"`
	CallsToday      int   `json:"calls_today" db:"calls_today"`

	// Session State
	LastLogin       time.Time `json:"last_login" db:"last_login"`
	TimeLeftToday   int       `json:"time_left_today" db:"time_left_today"`
	DailyFileLimit  int       `json:"daily_file_limit" db:"daily_file_limit"`
	DailyByteLimit  int       `json:"daily_byte_limit" db:"daily_byte_limit"`
	LastFileCheck   time.Time `json:"last_file_check" db:"last_file_check"`

	// Preferences — from "Default Editor..........: %-18s"
	Editor      string `json:"editor" db:"editor"`
	Protocol    byte   `json:"protocol" db:"protocol"`      // X/K/Y/G/Z
	ANSIMode    int    `json:"ansi_mode" db:"ansi_mode"`    // 0=none, 1=ANSI, 2=RIP
	ScreenWidth int    `json:"screen_width" db:"screen_width"`

	// Ratio Tracking
	FileRatio    float64 `json:"file_ratio" db:"file_ratio"`
	ByteRatio    float64 `json:"byte_ratio" db:"byte_ratio"`
	SecFileRatio float64 `json:"sec_file_ratio" db:"sec_file_ratio"`
	SecByteRatio float64 `json:"sec_byte_ratio" db:"sec_byte_ratio"`

	// Record metadata
	RecordNumber int       `json:"record_number" db:"record_number"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// ===================================================================
// Node Status
//
// In original TriBBS, this was written to %s\NODE%d.%d files for
// inter-node awareness. In tresbbs, it's an in-memory struct shared
// via goroutines.
// ===================================================================

type NodeStatus struct {
	NodeNumber    int       `json:"node_number"`
	Active        bool      `json:"active"`
	UserName      string    `json:"user_name"`
	UserAlias     string    `json:"user_alias"`
	SecurityLevel int       `json:"security_level"`
	BaudRate      string    `json:"baud_rate"` // "SSH", "Telnet", "Local"
	Activity      string    `json:"activity"`
	LoginTime     time.Time `json:"login_time"`
	ChatAvail     bool      `json:"chat_available"`
	PagePending   bool      `json:"page_pending"`
	PageFrom      string    `json:"page_from"`
}

// ===================================================================
// Message / Conference
//
// The message base. In original TriBBS, messages were stored in
// packed binary files (M*.IDX). In tresbbs, they're in SQLite.
// ===================================================================

type Conference struct {
	Name           string `json:"name" db:"name"`
	SecurityLevel  int    `json:"security_level" db:"security_level"`
	PrivateConf    bool   `json:"private_conf" db:"private_conf"`
	Echo           bool   `json:"echo" db:"echo"` // networked
	MessageCount   int64  `json:"message_count" db:"message_count"`
}

type Post struct {
	ID        int64     `json:"id" db:"id"`
	Conference string   `json:"conference" db:"conference"`
	Tick      int       `json:"tick" db:"tick"`
	Author    string    `json:"author" db:"author"`
	ReplyTo   int64     `json:"reply_to" db:"reply_to"`
	Subject   string    `json:"subject" db:"subject"`
	Body      string    `json:"body" db:"body"`
	PostedAt  time.Time `json:"posted_at" db:"posted_at"`
}

// ===================================================================
// File Area
//
// File areas are directories of downloadable files. In original TriBBS,
// each area had a FAREA.DAT entry and a FILES.BAK list. In tresbbs,
// they're tracked in SQLite with actual files on disk.
// ===================================================================

type FileArea struct {
	Name          string `json:"name" db:"name"`
	Description   string `json:"description" db:"description"`
	Path          string `json:"path" db:"path"`
	SecurityLevel int    `json:"security_level" db:"security_level"`
	CDROM         bool   `json:"cdrom" db:"cdrom"` // read-only area
	SortType      string `json:"sort_type" db:"sort_type"`
}

type FileEntry struct {
	ID          int64     `json:"id" db:"id"`
	Area        string    `json:"area" db:"area"`
	Name        string    `json:"name" db:"name"`
	Size        int64     `json:"size" db:"size"`
	Description string    `json:"description" db:"description"`
	UploadedBy  string    `json:"uploaded_by" db:"uploaded_by"`
	UploadedAt  time.Time `json:"uploaded_at" db:"uploaded_at"`
	Downloads   int       `json:"downloads" db:"downloads"`
}

// ===================================================================
// Bulletin
//
// Simple text bulletins displayed at login or on demand.
// ===================================================================

type Bulletin struct {
	ID       int    `json:"id" db:"id"`
	Name     string `json:"name" db:"name"`
	Content  string `json:"content" db:"content"`
	Security int    `json:"security" db:"security"`
}

// ===================================================================
// Event
//
// Scheduled events — timed external programs. In original TriBBS,
// these were stored in EVENTS.DAT.
// ===================================================================

type Event struct {
	ID            int    `json:"id" db:"id"`
	Time          string `json:"time" db:"time"`           // "HH:MM"
	Day           string `json:"day" db:"day"`             // "Mon", "Tue", etc. or "Daily"
	File          string `json:"file" db:"file"`           // batch file to run
	Slide         bool   `json:"slide" db:"slide"`         // run at next opportunity
	ExecutedToday bool   `json:"executed_today" db:"executed_today"`
}

// ===================================================================
// Door
//
// External door programs. In original TriBBS, these were configured
// in DOORS.MNU files.
// ===================================================================

type Door struct {
	Name        string `json:"name" db:"name"`
	HotKey      byte   `json:"hotkey" db:"hotkey"`
	Description string `json:"description" db:"description"`
	Command     string `json:"command" db:"command"`     // executable path
	DropFormat  string `json:"drop_format" db:"drop_format"` // "DOORSYS", "DORINFO"
	Security    int    `json:"security" db:"security"`
	TimeLimit   int    `json:"time_limit" db:"time_limit"` // minutes
}

// ===================================================================
// System Configuration
//
// The master BBS configuration. In original TriBBS, this was split
// across SYSDAT1.DAT and SYSDAT2.DAT.
// ===================================================================

type Config struct {
	BoardName        string `json:"board_name" db:"board_name"`
	SysopName        string `json:"sysop_name" db:"sysop_name"`
	SystemPassword   string `json:"-" db:"system_password"`
	MaxNodes         int    `json:"max_nodes" db:"max_nodes"`
	MaxBaud          int    `json:"max_baud" db:"max_baud"`
	NewUserSecurity  int    `json:"new_user_security" db:"new_user_security"`
	SysopSecurity    int    `json:"sysop_security" db:"sysop_security"`
	MaxTimePerLogon  int    `json:"max_time_per_logon" db:"max_time_per_logon"`
	NewUserTimeLimit int    `json:"new_user_time_limit" db:"new_user_time_limit"`
	AllowAliases     bool   `json:"allow_aliases" db:"allow_aliases"`
	Allow300Baud     bool   `json:"allow_300_baud" db:"allow_300_baud"`
	Allow1200Baud    bool   `json:"allow_1200_baud" db:"allow_1200_baud"`
	Allow2400Baud    bool   `json:"allow_2400_baud" db:"allow_2400_baud"`
	DefaultProtocol  byte   `json:"default_protocol" db:"default_protocol"`
	BBSStartDate     string `json:"bbs_start_date" db:"bbs_start_date"`

	// Caller ID settings (v11.6+)
	EnableCallerID   bool   `json:"enable_caller_id" db:"enable_caller_id"`     // Enable caller ID support
	BlockNoCallerID bool   `json:"block_no_caller_id" db:"block_no_caller_id"` // Block calls with no caller ID
	BlockBlockedCID bool   `json:"block_blocked_cid" db:"block_blocked_cid"`   // Block calls with blocked caller ID
	DefaultCountry  string `json:"default_country" db:"default_country"`        // @DEFAULTCOUNTRY

	// Access gates
	AllowNewUsers    bool `json:"allow_new_users" db:"allow_new_users"`       // false => "No new users on this node."
	MinSecurityLevel int  `json:"min_security_level" db:"min_security_level"` // login denied below this
}

// ===================================================================
// Action Types
//
// What a user can do during a session. These are the menu options
// mapped to actions.
// ===================================================================

type ActionKind string

const (
	ActionPost       ActionKind = "post"
	ActionReply      ActionKind = "reply"
	ActionMail       ActionKind = "mail"
	ActionDownload   ActionKind = "download"
	ActionUpload     ActionKind = "upload"
	ActionDoor       ActionKind = "door"
	ActionChat       ActionKind = "chat"
	ActionLogoff     ActionKind = "logoff"
	ActionIdle       ActionKind = "idle"
	ActionWhoOnline  ActionKind = "who_online"
	ActionReadBullet ActionKind = "read_bulletin"
	ActionUserConfig ActionKind = "user_config"
)
