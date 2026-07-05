// Package sqlite implements the StoragePort using SQLite.
//
// In original TriBBS, data was stored in binary files (USERS.DAT, FAREA.DAT,
// etc.). In tribbs, we use SQLite for the same data with the same semantics —
// fixed-length user records, indexed lookups, and audit logging.
package sqlite

import (
	"database/sql"
	"fmt"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/port"
)

// Ensure we implement the port interface.
var _ port.StoragePort = (*Storage)(nil)

// Storage implements StoragePort using SQLite.
type Storage struct {
	db *sql.DB
}

// New creates a new SQLite storage backend.
func New(dbPath string) (*Storage, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	s := &Storage{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

// migrate creates the database schema if it doesn't exist.
func (s *Storage) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			record_number INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			alias TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			phone TEXT DEFAULT '',
			city TEXT DEFAULT '',
			birthdate TEXT DEFAULT '',
			address TEXT DEFAULT '',
			email TEXT DEFAULT '',
			street_address TEXT DEFAULT '',
			state TEXT DEFAULT '',
			zipcode TEXT DEFAULT '',
			country TEXT DEFAULT '',
			caller_id TEXT DEFAULT '',
			block_caller_id INTEGER DEFAULT 0,
			security_level INTEGER DEFAULT 10,
			node_security INTEGER DEFAULT 10,
			registration TEXT DEFAULT '',
			subscription TEXT,
			locked_out INTEGER DEFAULT 0,
			deleted INTEGER DEFAULT 0,
			total_calls INTEGER DEFAULT 0,
			messages_posted INTEGER DEFAULT 0,
			files_downloaded INTEGER DEFAULT 0,
			files_uploaded INTEGER DEFAULT 0,
			k_downloaded INTEGER DEFAULT 0,
			k_uploaded INTEGER DEFAULT 0,
			downloads_today INTEGER DEFAULT 0,
			uploads_today INTEGER DEFAULT 0,
			messages_today INTEGER DEFAULT 0,
			calls_today INTEGER DEFAULT 0,
			last_login TEXT DEFAULT '',
			time_left_today INTEGER DEFAULT 60,
			daily_file_limit INTEGER DEFAULT 10,
			daily_byte_limit INTEGER DEFAULT 1024,
			last_file_check TEXT DEFAULT '',
			editor TEXT DEFAULT 'Full',
			protocol TEXT DEFAULT 'Z',
			ansi_mode INTEGER DEFAULT 1,
			screen_width INTEGER DEFAULT 80,
			file_ratio REAL DEFAULT 0.0,
			byte_ratio REAL DEFAULT 0.0,
			sec_file_ratio REAL DEFAULT 0.0,
			sec_byte_ratio REAL DEFAULT 0.0,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS conferences (
			name TEXT PRIMARY KEY,
			security_level INTEGER DEFAULT 0,
			private_conf INTEGER DEFAULT 0,
			echo INTEGER DEFAULT 0,
			message_count INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conference TEXT NOT NULL,
			tick INTEGER DEFAULT 0,
			author TEXT NOT NULL,
			reply_to INTEGER DEFAULT 0,
			subject TEXT DEFAULT '',
			body TEXT DEFAULT '',
			posted_at TEXT DEFAULT (datetime('now')),
			FOREIGN KEY (conference) REFERENCES conferences(name)
		)`,
		`CREATE TABLE IF NOT EXISTS file_areas (
			name TEXT PRIMARY KEY,
			description TEXT DEFAULT '',
			path TEXT DEFAULT '',
			security_level INTEGER DEFAULT 0,
			cdrom INTEGER DEFAULT 0,
			sort_type TEXT DEFAULT 'name'
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			area TEXT NOT NULL,
			name TEXT NOT NULL,
			size INTEGER DEFAULT 0,
			description TEXT DEFAULT '',
			uploaded_by TEXT DEFAULT '',
			uploaded_at TEXT DEFAULT (datetime('now')),
			downloads INTEGER DEFAULT 0,
			FOREIGN KEY (area) REFERENCES file_areas(name)
		)`,
		`CREATE TABLE IF NOT EXISTS bulletins (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			content TEXT DEFAULT '',
			security INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			time TEXT DEFAULT '00:00',
			day TEXT DEFAULT 'Daily',
			file TEXT DEFAULT '',
			slide INTEGER DEFAULT 0,
			executed_today INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS doors (
			name TEXT PRIMARY KEY,
			hotkey TEXT DEFAULT '',
			description TEXT DEFAULT '',
			command TEXT DEFAULT '',
			drop_format TEXT DEFAULT 'DOORSYS',
			security INTEGER DEFAULT 0,
			time_limit INTEGER DEFAULT 30
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			board_name TEXT DEFAULT 'TriBBS',
			sysop_name TEXT DEFAULT 'Sysop',
			system_password TEXT DEFAULT '',
			max_nodes INTEGER DEFAULT 1,
			max_baud INTEGER DEFAULT 14400,
			new_user_security INTEGER DEFAULT 10,
			sysop_security INTEGER DEFAULT 90,
			max_time_per_logon INTEGER DEFAULT 60,
			new_user_time_limit INTEGER DEFAULT 30,
			allow_aliases INTEGER DEFAULT 1,
			allow_300_baud INTEGER DEFAULT 0,
			allow_1200_baud INTEGER DEFAULT 0,
			allow_2400_baud INTEGER DEFAULT 1,
			default_protocol TEXT DEFAULT 'Z',
			bbs_start_date TEXT DEFAULT '',
			enable_caller_id INTEGER DEFAULT 0,
			block_no_caller_id INTEGER DEFAULT 0,
			block_blocked_cid INTEGER DEFAULT 0,
			default_country TEXT DEFAULT '',
			allow_new_users INTEGER DEFAULT 1,
			min_security_level INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS caller_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entry TEXT NOT NULL,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_posts_conference ON posts(conference)`,
		`CREATE INDEX IF NOT EXISTS idx_files_area ON files(area)`,
		`CREATE INDEX IF NOT EXISTS idx_caller_log_created ON caller_log(created_at)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w\nSQL: %s", err, m)
		}
	}

	// Idempotent column additions for config rows created before these fields
	// existed (SQLite has no ADD COLUMN IF NOT EXISTS).
	for _, add := range []struct{ col, ddl string }{
		{"allow_new_users", "ALTER TABLE config ADD COLUMN allow_new_users INTEGER DEFAULT 1"},
		{"min_security_level", "ALTER TABLE config ADD COLUMN min_security_level INTEGER DEFAULT 0"},
	} {
		if !s.columnExists("config", add.col) {
			if _, err := s.db.Exec(add.ddl); err != nil {
				return fmt.Errorf("add column %s: %w", add.col, err)
			}
		}
	}

	// Insert default config if not exists
	_, err := s.db.Exec(`INSERT OR IGNORE INTO config (id, board_name, sysop_name, bbs_start_date)
		VALUES (1, 'TriBBS', 'Sysop', ?)`, time.Now().Format("01/02/2006"))
	if err != nil {
		return fmt.Errorf("insert default config: %w", err)
	}

	return nil
}

// GetUser implements StoragePort.
const userColumns = `
	record_number, name, alias, password_hash,
	phone, city, birthdate, address,
	email, street_address, state, zipcode, country,
	caller_id, block_caller_id,
	security_level, node_security, registration,
	subscription, locked_out, deleted,
	total_calls, messages_posted,
	files_downloaded, files_uploaded,
	k_downloaded, k_uploaded,
	downloads_today, uploads_today,
	messages_today, calls_today,
	last_login, time_left_today,
	daily_file_limit, daily_byte_limit,
	last_file_check, editor, protocol,
	ansi_mode, screen_width,
	file_ratio, byte_ratio,
	sec_file_ratio, sec_byte_ratio`

func (s *Storage) GetUser(recordNumber int) (*domain.User, error) {
	row := s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE record_number = ? AND deleted = 0`, recordNumber)
	return scanUser(row)
}

// GetUserByName implements StoragePort.
func (s *Storage) GetUserByName(name string) (*domain.User, error) {
	row := s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE name = ? AND deleted = 0`, name)
	return scanUser(row)
}

// GetUserByAlias implements StoragePort.
func (s *Storage) GetUserByAlias(alias string) (*domain.User, error) {
	row := s.db.QueryRow(`SELECT `+userColumns+` FROM users WHERE alias = ? AND deleted = 0`, alias)
	return scanUser(row)
}

// SaveUser implements StoragePort.
func (s *Storage) SaveUser(user *domain.User) error {
	_, err := s.db.Exec(`UPDATE users SET
		name = ?, alias = ?, password_hash = ?, phone = ?, city = ?, birthdate = ?, address = ?,
		email = ?, street_address = ?, state = ?, zipcode = ?, country = ?,
		caller_id = ?, block_caller_id = ?,
		security_level = ?, node_security = ?, registration = ?,
		subscription = ?, locked_out = ?, deleted = ?,
		total_calls = ?, messages_posted = ?,
		files_downloaded = ?, files_uploaded = ?,
		k_downloaded = ?, k_uploaded = ?,
		downloads_today = ?, uploads_today = ?,
		messages_today = ?, calls_today = ?,
		last_login = ?, time_left_today = ?,
		daily_file_limit = ?, daily_byte_limit = ?,
		last_file_check = ?, editor = ?, protocol = ?,
		ansi_mode = ?, screen_width = ?,
		file_ratio = ?, byte_ratio = ?,
		sec_file_ratio = ?, sec_byte_ratio = ?,
		updated_at = datetime('now')
		WHERE record_number = ?`,
		user.Name, user.Alias, user.Password, user.Phone, user.City, user.BirthDate, user.Address,
		user.Email, user.StreetAddress, user.State, user.ZipCode, user.Country,
		user.CallerID, user.BlockCallerID,
		user.SecurityLevel, user.NodeSecurity, user.Registration,
		user.Subscription, user.LockedOut, user.Deleted,
		user.TotalCalls, user.MessagesPosted,
		user.FilesDownloaded, user.FilesUploaded,
		user.KDownloaded, user.KUploaded,
		user.DownloadsToday, user.UploadsToday,
		user.MessagesToday, user.CallsToday,
		user.LastLogin.Format(time.RFC3339), user.TimeLeftToday,
		user.DailyFileLimit, user.DailyByteLimit,
		user.LastFileCheck.Format(time.RFC3339), user.Editor, string(user.Protocol),
		user.ANSIMode, user.ScreenWidth,
		user.FileRatio, user.ByteRatio,
		user.SecFileRatio, user.SecByteRatio,
		user.RecordNumber)
	return err
}

// AddUser implements StoragePort.
func (s *Storage) AddUser(user *domain.User) (int, error) {
	result, err := s.db.Exec(`INSERT INTO users (
		name, alias, password_hash, phone, city, birthdate, address,
		email, street_address, state, zipcode, country,
		caller_id, block_caller_id,
		security_level, node_security, editor, protocol, ansi_mode, screen_width
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.Name, user.Alias, user.Password, user.Phone, user.City, user.BirthDate, user.Address,
		user.Email, user.StreetAddress, user.State, user.ZipCode, user.Country,
		user.CallerID, user.BlockCallerID,
		user.SecurityLevel, user.NodeSecurity, user.Editor, string(user.Protocol), user.ANSIMode, user.ScreenWidth)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

// DeleteUser implements StoragePort.
func (s *Storage) DeleteUser(recordNumber int) error {
	_, err := s.db.Exec(`UPDATE users SET deleted = 1 WHERE record_number = ?`, recordNumber)
	return err
}

// PackUsers implements StoragePort.
func (s *Storage) PackUsers() (int, error) {
	result, err := s.db.Exec(`DELETE FROM users WHERE deleted = 1`)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ListUsers implements StoragePort.
func (s *Storage) ListUsers() ([]domain.User, error) {
	rows, err := s.db.Query(`SELECT `+userColumns+` FROM users WHERE deleted = 0 ORDER BY record_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		u, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, nil
}

// UserCount implements StoragePort.
func (s *Storage) UserCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE deleted = 0`).Scan(&count)
	return count, err
}

// GetConferences implements StoragePort.
func (s *Storage) GetConferences() ([]domain.Conference, error) {
	rows, err := s.db.Query(`SELECT * FROM conferences ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var confs []domain.Conference
	for rows.Next() {
		var c domain.Conference
		err := rows.Scan(&c.Name, &c.SecurityLevel, &c.PrivateConf, &c.Echo, &c.MessageCount)
		if err != nil {
			return nil, err
		}
		confs = append(confs, c)
	}
	return confs, nil
}

// GetConference implements StoragePort.
func (s *Storage) GetConference(name string) (*domain.Conference, error) {
	var c domain.Conference
	err := s.db.QueryRow(`SELECT * FROM conferences WHERE name = ?`, name).
		Scan(&c.Name, &c.SecurityLevel, &c.PrivateConf, &c.Echo, &c.MessageCount)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// AddConference implements StoragePort.
func (s *Storage) AddConference(conf *domain.Conference) error {
	_, err := s.db.Exec(`INSERT INTO conferences (name, security_level, private_conf, echo) VALUES (?, ?, ?, ?)`,
		conf.Name, conf.SecurityLevel, conf.PrivateConf, conf.Echo)
	return err
}

// SaveConference implements StoragePort.
func (s *Storage) SaveConference(conf *domain.Conference) error {
	_, err := s.db.Exec(`UPDATE conferences SET security_level=?, private_conf=?, echo=? WHERE name=?`,
		conf.SecurityLevel, conf.PrivateConf, conf.Echo, conf.Name)
	return err
}

// DeleteConference implements StoragePort.
func (s *Storage) DeleteConference(name string) error {
	_, err := s.db.Exec(`DELETE FROM conferences WHERE name = ?`, name)
	return err
}

// RefreshConferenceMessageCounts recomputes every conference's cached
// message_count from the posts table (a sysop maintenance operation).
func (s *Storage) RefreshConferenceMessageCounts() error {
	_, err := s.db.Exec(`UPDATE conferences SET message_count =
		(SELECT COUNT(*) FROM posts p WHERE p.conference = conferences.name)`)
	return err
}

// GetPosts implements StoragePort.
func (s *Storage) GetPosts(conference string, limit int) ([]domain.Post, error) {
	rows, err := s.db.Query(`SELECT * FROM posts WHERE conference = ? ORDER BY id DESC LIMIT ?`, conference, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []domain.Post
	for rows.Next() {
		var p domain.Post
		var postedAt string
		err := rows.Scan(&p.ID, &p.Conference, &p.Tick, &p.Author, &p.ReplyTo, &p.Subject, &p.Body, &postedAt)
		if err != nil {
			return nil, err
		}
		p.PostedAt, _ = time.Parse(time.RFC3339, postedAt)
		posts = append(posts, p)
	}
	return posts, nil
}

// GetPost implements StoragePort.
func (s *Storage) GetPost(id int64) (*domain.Post, error) {
	var p domain.Post
	var postedAt string
	err := s.db.QueryRow(`SELECT * FROM posts WHERE id = ?`, id).
		Scan(&p.ID, &p.Conference, &p.Tick, &p.Author, &p.ReplyTo, &p.Subject, &p.Body, &postedAt)
	if err != nil {
		return nil, err
	}
	p.PostedAt, _ = time.Parse(time.RFC3339, postedAt)
	return &p, nil
}

// AddPost implements StoragePort.
func (s *Storage) AddPost(post *domain.Post) error {
	result, err := s.db.Exec(`INSERT INTO posts (conference, tick, author, reply_to, subject, body)
		VALUES (?, ?, ?, ?, ?, ?)`,
		post.Conference, post.Tick, post.Author, post.ReplyTo, post.Subject, post.Body)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	post.ID = id
	_, err = s.db.Exec(`UPDATE conferences SET message_count = message_count + 1 WHERE name = ?`, post.Conference)
	return err
}

// PostCount implements StoragePort.
func (s *Storage) PostCount(conference string) (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM posts WHERE conference = ?`, conference).Scan(&count)
	return count, err
}

// GetFileAreas implements StoragePort.
func (s *Storage) GetFileAreas() ([]domain.FileArea, error) {
	rows, err := s.db.Query(`SELECT * FROM file_areas ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var areas []domain.FileArea
	for rows.Next() {
		var a domain.FileArea
		err := rows.Scan(&a.Name, &a.Description, &a.Path, &a.SecurityLevel, &a.CDROM, &a.SortType)
		if err != nil {
			return nil, err
		}
		areas = append(areas, a)
	}
	return areas, nil
}

// GetFileArea implements StoragePort.
func (s *Storage) GetFileArea(name string) (*domain.FileArea, error) {
	var a domain.FileArea
	err := s.db.QueryRow(`SELECT * FROM file_areas WHERE name = ?`, name).
		Scan(&a.Name, &a.Description, &a.Path, &a.SecurityLevel, &a.CDROM, &a.SortType)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AddFileArea implements StoragePort.
func (s *Storage) AddFileArea(area *domain.FileArea) error {
	_, err := s.db.Exec(`INSERT INTO file_areas (name, description, path, security_level, cdrom, sort_type) VALUES (?, ?, ?, ?, ?, ?)`,
		area.Name, area.Description, area.Path, area.SecurityLevel, area.CDROM, area.SortType)
	return err
}

// SaveFileArea implements StoragePort.
func (s *Storage) SaveFileArea(area *domain.FileArea) error {
	_, err := s.db.Exec(`UPDATE file_areas SET description=?, path=?, security_level=?, cdrom=?, sort_type=? WHERE name=?`,
		area.Description, area.Path, area.SecurityLevel, area.CDROM, area.SortType, area.Name)
	return err
}

// DeleteFileArea implements StoragePort.
func (s *Storage) DeleteFileArea(name string) error {
	_, err := s.db.Exec(`DELETE FROM file_areas WHERE name = ?`, name)
	return err
}

// GetFiles implements StoragePort.
func (s *Storage) GetFiles(area string, limit int) ([]domain.FileEntry, error) {
	rows, err := s.db.Query(`SELECT * FROM files WHERE area = ? ORDER BY name LIMIT ?`, area, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.FileEntry
	for rows.Next() {
		var f domain.FileEntry
		var uploadedAt string
		err := rows.Scan(&f.ID, &f.Area, &f.Name, &f.Size, &f.Description, &f.UploadedBy, &uploadedAt, &f.Downloads)
		if err != nil {
			return nil, err
		}
		f.UploadedAt, _ = time.Parse(time.RFC3339, uploadedAt)
		files = append(files, f)
	}
	return files, nil
}

// AddFile implements StoragePort.
func (s *Storage) AddFile(file *domain.FileEntry) error {
	result, err := s.db.Exec(`INSERT INTO files (area, name, size, description, uploaded_by)
		VALUES (?, ?, ?, ?, ?)`,
		file.Area, file.Name, file.Size, file.Description, file.UploadedBy)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	file.ID = id
	return nil
}

// SaveFile implements StoragePort.
func (s *Storage) SaveFile(file *domain.FileEntry) error {
	_, err := s.db.Exec(`UPDATE files SET area=?, name=?, size=?, description=?, uploaded_by=?, downloads=? WHERE id=?`,
		file.Area, file.Name, file.Size, file.Description, file.UploadedBy, file.Downloads, file.ID)
	return err
}

// DeleteFile implements StoragePort.
func (s *Storage) DeleteFile(id int64) error {
	_, err := s.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	return err
}

// GetBulletins implements StoragePort.
func (s *Storage) GetBulletins() ([]domain.Bulletin, error) {
	rows, err := s.db.Query(`SELECT * FROM bulletins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bulletins []domain.Bulletin
	for rows.Next() {
		var b domain.Bulletin
		err := rows.Scan(&b.ID, &b.Name, &b.Content, &b.Security)
		if err != nil {
			return nil, err
		}
		bulletins = append(bulletins, b)
	}
	return bulletins, nil
}

// GetBulletin implements StoragePort.
func (s *Storage) GetBulletin(id int) (*domain.Bulletin, error) {
	var b domain.Bulletin
	err := s.db.QueryRow(`SELECT * FROM bulletins WHERE id = ?`, id).
		Scan(&b.ID, &b.Name, &b.Content, &b.Security)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// AddBulletin implements StoragePort.
func (s *Storage) AddBulletin(bulletin *domain.Bulletin) error {
	result, err := s.db.Exec(`INSERT INTO bulletins (name, content, security) VALUES (?, ?, ?)`,
		bulletin.Name, bulletin.Content, bulletin.Security)
	if err != nil {
		return err
	}
	id, _ := result.LastInsertId()
	bulletin.ID = int(id)
	return nil
}

// SaveBulletin implements StoragePort.
func (s *Storage) SaveBulletin(bulletin *domain.Bulletin) error {
	_, err := s.db.Exec(`UPDATE bulletins SET name=?, content=?, security=? WHERE id=?`,
		bulletin.Name, bulletin.Content, bulletin.Security, bulletin.ID)
	return err
}

// DeleteBulletin implements StoragePort.
func (s *Storage) DeleteBulletin(id int) error {
	_, err := s.db.Exec(`DELETE FROM bulletins WHERE id = ?`, id)
	return err
}

// DeletePost implements StoragePort.
func (s *Storage) DeletePost(id int64) error {
	_, err := s.db.Exec(`DELETE FROM posts WHERE id = ?`, id)
	return err
}

// GetEvents implements StoragePort.
func (s *Storage) GetEvents() ([]domain.Event, error) {
	rows, err := s.db.Query(`SELECT * FROM events ORDER BY time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var e domain.Event
		err := rows.Scan(&e.ID, &e.Time, &e.Day, &e.File, &e.Slide, &e.ExecutedToday)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

// SaveEvent implements StoragePort.
func (s *Storage) SaveEvent(event *domain.Event) error {
	if event.ID == 0 {
		result, err := s.db.Exec(`INSERT INTO events (time, day, file, slide, executed_today)
			VALUES (?, ?, ?, ?, ?)`, event.Time, event.Day, event.File, event.Slide, event.ExecutedToday)
		if err != nil {
			return err
		}
		id, _ := result.LastInsertId()
		event.ID = int(id)
	} else {
		_, err := s.db.Exec(`UPDATE events SET time=?, day=?, file=?, slide=?, executed_today=?
			WHERE id=?`, event.Time, event.Day, event.File, event.Slide, event.ExecutedToday, event.ID)
		return err
	}
	return nil
}

// DeleteEvent implements StoragePort.
func (s *Storage) DeleteEvent(id int) error {
	_, err := s.db.Exec(`DELETE FROM events WHERE id = ?`, id)
	return err
}

// GetDoors implements StoragePort.
func (s *Storage) GetDoors() ([]domain.Door, error) {
	rows, err := s.db.Query(`SELECT * FROM doors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var doors []domain.Door
	for rows.Next() {
		var d domain.Door
		var hotkey string
		err := rows.Scan(&d.Name, &hotkey, &d.Description, &d.Command, &d.DropFormat, &d.Security, &d.TimeLimit)
		if err != nil {
			return nil, err
		}
		if len(hotkey) > 0 {
			d.HotKey = hotkey[0]
		}
		doors = append(doors, d)
	}
	return doors, nil
}

// GetDoor implements StoragePort.
func (s *Storage) GetDoor(name string) (*domain.Door, error) {
	var d domain.Door
	var hotkey string
	err := s.db.QueryRow(`SELECT * FROM doors WHERE name = ?`, name).
		Scan(&d.Name, &hotkey, &d.Description, &d.Command, &d.DropFormat, &d.Security, &d.TimeLimit)
	if err != nil {
		return nil, err
	}
	if len(hotkey) > 0 {
		d.HotKey = hotkey[0]
	}
	return &d, nil
}

// AddDoor implements StoragePort.
func (s *Storage) AddDoor(door *domain.Door) error {
	_, err := s.db.Exec(`INSERT INTO doors (name, hotkey, description, command, drop_format, security, time_limit) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		door.Name, string(door.HotKey), door.Description, door.Command, door.DropFormat, door.Security, door.TimeLimit)
	return err
}

// SaveDoor implements StoragePort.
func (s *Storage) SaveDoor(door *domain.Door) error {
	_, err := s.db.Exec(`UPDATE doors SET hotkey=?, description=?, command=?, drop_format=?, security=?, time_limit=? WHERE name=?`,
		string(door.HotKey), door.Description, door.Command, door.DropFormat, door.Security, door.TimeLimit, door.Name)
	return err
}

// DeleteDoor implements StoragePort.
func (s *Storage) DeleteDoor(name string) error {
	_, err := s.db.Exec(`DELETE FROM doors WHERE name = ?`, name)
	return err
}

// parseProtoCode decodes a protocol code (domain uses a byte holding the ASCII
// letter, e.g. 'Z' for Zmodem) from its stored text form. The intended storage is
// the letter itself — matching how door hotkeys are stored/read — so "Z" -> 'Z'.
// It also tolerates a legacy numeric form ("90") written by an earlier code path
// that persisted the raw byte; protocol codes are always letters, so a purely
// numeric string is unambiguously that legacy encoding. Empty -> 'Z' (the default).
func parseProtoCode(s string) byte {
	if s == "" {
		return 'Z'
	}
	if n, err := strconv.Atoi(s); err == nil && n > 0 && n < 128 {
		return byte(n)
	}
	return s[0]
}

// GetConfig implements StoragePort.
// columnExists reports whether a table has a given column (for idempotent
// ADD COLUMN migrations).
func (s *Storage) columnExists(table, column string) bool {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

func (s *Storage) GetConfig() (*domain.Config, error) {
	var c domain.Config
	// Explicit column list (NOT SELECT *): the table's first column is `id`, which
	// the scan below does not read. SELECT * returned it as an extra column and the
	// scan failed with "expected 20 destination arguments in Scan, not 19" on every
	// boot — silently recreating the default config and pinning MaxNodes to 4.
	var defaultProtocol string
	err := s.db.QueryRow(`SELECT
		board_name, sysop_name, system_password,
		max_nodes, max_baud, new_user_security, sysop_security,
		max_time_per_logon, new_user_time_limit,
		allow_aliases, allow_300_baud, allow_1200_baud, allow_2400_baud,
		default_protocol, bbs_start_date,
		enable_caller_id, block_no_caller_id, block_blocked_cid, default_country,
		allow_new_users, min_security_level
		FROM config WHERE id = 1`).Scan(
		&c.BoardName, &c.SysopName, &c.SystemPassword,
		&c.MaxNodes, &c.MaxBaud, &c.NewUserSecurity, &c.SysopSecurity,
		&c.MaxTimePerLogon, &c.NewUserTimeLimit,
		&c.AllowAliases, &c.Allow300Baud, &c.Allow1200Baud, &c.Allow2400Baud,
		&defaultProtocol, &c.BBSStartDate,
		&c.EnableCallerID, &c.BlockNoCallerID, &c.BlockBlockedCID, &c.DefaultCountry,
		&c.AllowNewUsers, &c.MinSecurityLevel)
	if err != nil {
		return nil, err
	}
	c.DefaultProtocol = parseProtoCode(defaultProtocol)
	return &c, nil
}

// SaveConfig implements StoragePort.
func (s *Storage) SaveConfig(config *domain.Config) error {
	_, err := s.db.Exec(`UPDATE config SET
		board_name=?, sysop_name=?, system_password=?,
		max_nodes=?, max_baud=?, new_user_security=?, sysop_security=?,
		max_time_per_logon=?, new_user_time_limit=?,
		allow_aliases=?, allow_300_baud=?, allow_1200_baud=?, allow_2400_baud=?,
		default_protocol=?, bbs_start_date=?,
		enable_caller_id=?, block_no_caller_id=?, block_blocked_cid=?, default_country=?,
		allow_new_users=?, min_security_level=?
		WHERE id = 1`,
		config.BoardName, config.SysopName, config.SystemPassword,
		config.MaxNodes, config.MaxBaud, config.NewUserSecurity, config.SysopSecurity,
		config.MaxTimePerLogon, config.NewUserTimeLimit,
		config.AllowAliases, config.Allow300Baud, config.Allow1200Baud, config.Allow2400Baud,
		string(config.DefaultProtocol), config.BBSStartDate,
		config.EnableCallerID, config.BlockNoCallerID, config.BlockBlockedCID, config.DefaultCountry,
		config.AllowNewUsers, config.MinSecurityLevel)
	return err
}

// LogCaller implements StoragePort.
func (s *Storage) LogCaller(entry string) error {
	_, err := s.db.Exec(`INSERT INTO caller_log (entry) VALUES (?)`, entry)
	return err
}

// GetCallerLog implements StoragePort.
func (s *Storage) GetCallerLog(limit int) ([]string, error) {
	rows, err := s.db.Query(`SELECT entry FROM caller_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Close implements StoragePort.
func (s *Storage) Close() error {
	return s.db.Close()
}

// scanUser is a helper to scan a user from a row.
func scanUser(row *sql.Row) (*domain.User, error) {
	var u domain.User
	var lastLogin, lastFileCheck sql.NullString
	var subscription sql.NullString
	var deleted, lockedOut, blockCallerID int
	var protocolStr string

	err := row.Scan(
		&u.RecordNumber, &u.Name, &u.Alias, &u.Password,
		&u.Phone, &u.City, &u.BirthDate, &u.Address,
		&u.Email, &u.StreetAddress, &u.State, &u.ZipCode, &u.Country,
		&u.CallerID, &blockCallerID,
		&u.SecurityLevel, &u.NodeSecurity, &u.Registration,
		&subscription, &lockedOut, &deleted,
		&u.TotalCalls, &u.MessagesPosted,
		&u.FilesDownloaded, &u.FilesUploaded,
		&u.KDownloaded, &u.KUploaded,
		&u.DownloadsToday, &u.UploadsToday,
		&u.MessagesToday, &u.CallsToday,
		&lastLogin, &u.TimeLeftToday,
		&u.DailyFileLimit, &u.DailyByteLimit,
		&lastFileCheck, &u.Editor, &protocolStr,
		&u.ANSIMode, &u.ScreenWidth,
		&u.FileRatio, &u.ByteRatio,
		&u.SecFileRatio, &u.SecByteRatio,
	)
	if err != nil {
		return nil, err
	}

	u.Deleted = deleted != 0
	u.LockedOut = lockedOut != 0
	u.BlockCallerID = blockCallerID != 0
	u.Protocol = parseProtoCode(protocolStr)
	if lastLogin.Valid {
		u.LastLogin, _ = time.Parse(time.RFC3339, lastLogin.String)
	}
	if lastFileCheck.Valid {
		u.LastFileCheck, _ = time.Parse(time.RFC3339, lastFileCheck.String)
	}
	if subscription.Valid {
		t, _ := time.Parse("01/02/2006", subscription.String)
		u.Subscription = &t
	}

	return &u, nil
}

// scanUserRows is a helper to scan a user from rows.
func scanUserRows(rows *sql.Rows) (*domain.User, error) {
	var u domain.User
	var lastLogin, lastFileCheck sql.NullString
	var subscription sql.NullString
	var deleted, lockedOut, blockCallerID int
	var protocolStr string

	err := rows.Scan(
		&u.RecordNumber, &u.Name, &u.Alias, &u.Password,
		&u.Phone, &u.City, &u.BirthDate, &u.Address,
		&u.Email, &u.StreetAddress, &u.State, &u.ZipCode, &u.Country,
		&u.CallerID, &blockCallerID,
		&u.SecurityLevel, &u.NodeSecurity, &u.Registration,
		&subscription, &lockedOut, &deleted,
		&u.TotalCalls, &u.MessagesPosted,
		&u.FilesDownloaded, &u.FilesUploaded,
		&u.KDownloaded, &u.KUploaded,
		&u.DownloadsToday, &u.UploadsToday,
		&u.MessagesToday, &u.CallsToday,
		&lastLogin, &u.TimeLeftToday,
		&u.DailyFileLimit, &u.DailyByteLimit,
		&lastFileCheck, &u.Editor, &protocolStr,
		&u.ANSIMode, &u.ScreenWidth,
		&u.FileRatio, &u.ByteRatio,
		&u.SecFileRatio, &u.SecByteRatio,
	)
	if err != nil {
		return nil, err
	}

	u.Deleted = deleted != 0
	u.LockedOut = lockedOut != 0
	u.BlockCallerID = blockCallerID != 0
	u.Protocol = parseProtoCode(protocolStr)
	if lastLogin.Valid {
		u.LastLogin, _ = time.Parse(time.RFC3339, lastLogin.String)
	}
	if lastFileCheck.Valid {
		u.LastFileCheck, _ = time.Parse(time.RFC3339, lastFileCheck.String)
	}
	if subscription.Valid {
		t, _ := time.Parse("01/02/2006", subscription.String)
		u.Subscription = &t
	}

	return &u, nil
}
