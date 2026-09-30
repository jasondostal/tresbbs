// Package session manages the BBS session lifecycle.
//
// This is the core session loop that handles:
//   - Login/registration
//   - Main menu dispatch
//   - Message base (read/post/reply)
//   - File areas (list/download/upload)
//   - Door execution
//   - Chat between nodes
//   - Bulletins
//   - User configuration
//   - Sysop functions
package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jasondostal/tresbbs/domain"
	"github.com/jasondostal/tresbbs/internal/auth"
	"github.com/jasondostal/tresbbs/internal/callerid"
	"github.com/jasondostal/tresbbs/internal/doormenu"
	"github.com/jasondostal/tresbbs/internal/fileutil"
	"github.com/jasondostal/tresbbs/internal/menu"
	"github.com/jasondostal/tresbbs/internal/modem"
	"github.com/jasondostal/tresbbs/internal/protocol"
	"github.com/jasondostal/tresbbs/internal/template"
	"github.com/jasondostal/tresbbs/internal/ui"
	"github.com/jasondostal/tresbbs/port"
)

// Session represents an active BBS session.
type Session struct {
	display  port.DisplayPort
	storage  port.StoragePort
	nodes    port.NodePort
	config   *domain.Config
	user     *domain.User
	nodeNum  int
	baudRate string

	// currentConf is the joined message conference (TriBBS current-conference
	// model): Read/Post act on it, JOIN or a number key changes it.
	currentConf string

	// flagged is the per-session list of files tagged for batch download.
	flagged        []domain.FileEntry
	loginAt        time.Time
	ctx            context.Context
	cancel         context.CancelFunc
	templateEngine *template.Engine
	templateData   *template.TemplateData
	rateLimiter    *auth.RateLimiter

	// Template flow-control state (@MOREON/@MOREOFF, @BREAKON/@BREAKOFF, @HANGUP).
	moreEnabled  bool
	breakEnabled bool
	hangup       bool
	remoteAddr   string

	// menuDir is the directory holding TriBBS .MNU files (a dropped-in NWORK/).
	// Empty means "use the embedded stock menus". See internal/menu.
	menuDir string

	// expert is the caller's expert-mode toggle: when set, menu boxes are
	// suppressed and only the command prompt is shown (TriBBS <X> Expert Mode).
	expert bool
}

// New creates a new BBS session.
func New(
	display port.DisplayPort,
	storage port.StoragePort,
	nodes port.NodePort,
	config *domain.Config,
	nodeNum int,
	baudRate string,
	menuDir string,
	remoteAddr string,
) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	templateEngine := template.NewEngine(config, "templates")
	return &Session{
		display:        display,
		storage:        storage,
		nodes:          nodes,
		config:         config,
		nodeNum:        nodeNum,
		baudRate:       baudRate,
		menuDir:        menuDir,
		loginAt:        time.Now(),
		ctx:            ctx,
		cancel:         cancel,
		templateEngine: templateEngine,
		templateData: &template.TemplateData{
			Config: config,
			Session: &template.SessionData{
				NodeNumber: nodeNum,
				BaudRate:   baudRate,
				LoginTime:  time.Now(),
			},
		},
		rateLimiter: auth.NewRateLimiter(5, 15*time.Minute),
		remoteAddr:  remoteAddr,
	}
}

// Run executes the main BBS session loop.
func (s *Session) Run() {
	defer s.cancel()

	// A dropped carrier unwinds via panic (see carrierLost); recover it here so
	// the goroutine exits cleanly instead of crashing the server. Real panics
	// (bugs) are re-thrown.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(disconnectPanic); !ok {
				panic(r)
			}
		}
	}()

	// Simulate the modem answer so the session reports an authentic baud rate
	// (CONNECT 14400) over the telnet/SSH transport, like real TriBBS.
	s.simulateModemConnect()

	// System password prompt (from PASSWORD.DAT)
	if !s.systemPasswordPrompt() {
		return
	}

	// Login sequence
	if !s.login() {
		return
	}

	// Register node
	s.nodes.RegisterNode(&domain.NodeStatus{
		NodeNumber:    s.nodeNum,
		Active:        true,
		UserName:      s.user.Name,
		UserAlias:     s.user.Alias,
		SecurityLevel: s.user.SecurityLevel,
		BaudRate:      s.baudRate,
		Activity:      "Main Menu",
		LoginTime:     s.loginAt,
	})
	defer s.nodes.UnregisterNode(s.nodeNum)

	// Honor the user's saved graphics mode (RIPscrip if ANSIMode == 2).
	s.applyGraphicsMode()

	// Main session loop
	for {
		// A template @HANGUP directive requests immediate disconnect.
		if s.hangup {
			return
		}

		// Check time limit
		if s.user.TimeLeftToday <= 0 {
			s.display.SetColor('C')
			s.display.WriteLine("\r\nTime limit reached! Auto-logoff.")
			s.pause()
			return
		}

		// Update time remaining
		elapsed := int(time.Since(s.loginAt).Minutes())
		s.user.TimeLeftToday = s.config.MaxTimePerLogon - elapsed
		if s.user.TimeLeftToday < 0 {
			s.user.TimeLeftToday = 0
		}

		// Update node status
		s.nodes.UpdateNode(&domain.NodeStatus{
			NodeNumber:    s.nodeNum,
			Active:        true,
			UserName:      s.user.Name,
			UserAlias:     s.user.Alias,
			SecurityLevel: s.user.SecurityLevel,
			BaudRate:      s.baudRate,
			Activity:      "Main Menu",
			LoginTime:     s.loginAt,
		})

		// Deliver any pending inter-node page (someone paged this node for
		// chat). Polled here so it reaches the caller when they land on the
		// main menu — previously CheckPage was never called and pages were
		// silently dropped.
		if paged, from, _ := s.nodes.CheckPage(s.nodeNum); paged {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("\r\n*** %s is paging you for chat! ***", from))
			s.display.ResetColor()
			s.nodes.ClearPage(s.nodeNum)
			s.pause()
		}

		choice, m := s.showMenu("MAIN.MNU")
		if choice == '?' {
			// Help: force the full menu to redraw next pass even in expert mode.
			continue
		}
		if m != nil {
			if it, ok := m.Find(choice, s.userSecurity()); ok {
				s.logMenuSelection("Main Menu", it.Description)
			}
		}
		if s.dispatchMain(choice) {
			return
		}
	}
}

// login handles the login/registration flow.
func (s *Session) login() bool {
	s.display.Clear()
	s.showWelcome()

	// Caller ID check (v11.6+)
	if s.config.EnableCallerID {
		callerID := s.getCallerID()
		result := callerid.ValidateCallerID(s.config, callerID, nil)
		if result.Status != callerid.StatusAllowed {
			s.display.SetColor('C')
			s.display.WriteLine(result.Message)
			s.storage.LogCaller(callerid.FormatCallerIDLog(callerID, result.Status))
			s.pause()
			return false
		}
		if callerID != "" {
			s.display.SetColor('B')
			s.display.WriteLine(fmt.Sprintf("Caller ID: %s", callerid.FormatCallerID(callerID)))
			s.display.ResetColor()
		}
	}

	s.display.SetColor('E') // Yellow
	s.display.Write("User Name: ")
	s.display.ResetColor()
	name := s.readLine()

	if name == "" {
		s.display.WriteLine("No name entered. Goodbye!")
		s.pause()
		return false
	}

	// Check joker file (banned usernames)
	if s.isBannedUsername(name) {
		s.display.SetColor('C')
		s.display.WriteLine("Access denied.")
		s.storage.LogCaller(fmt.Sprintf("Banned username attempt: %s", name))
		s.pause()
		return false
	}

	// Look up existing user by real name or alias (you register with a real
	// name but log in with either — matching TriBBS).
	user, err := s.storage.GetUserByName(name)
	if err != nil {
		user, err = s.storage.GetUserByAlias(name)
	}
	if err != nil {
		// Not found - new-user path, if this node accepts new users.
		if !s.config.AllowNewUsers {
			s.display.SetColor('C')
			s.display.WriteLine("Caller denied access: No new users on this node.")
			s.display.ResetColor()
			s.storage.LogCaller("Caller denied access: No new users on this node.")
			s.pause()
			return false
		}
		if s.yesNo("New user?", false) {
			return s.registerNewUser(name)
		}
		s.display.WriteLine("Goodbye!")
		s.pause()
		return false
	}

	// Existing user - check if locked out
	if user.LockedOut {
		s.display.SetColor('C')
		s.display.WriteLine("Caller was previously locked out or deleted.")
		s.storage.LogCaller(fmt.Sprintf("Locked account attempt: %s", name))
		s.pause()
		return false
	}

	// Minimum-security access gate.
	if user.SecurityLevel < s.config.MinSecurityLevel {
		s.display.SetColor('C')
		s.display.WriteLine("Caller denied access: Security level too low.")
		s.display.ResetColor()
		s.storage.LogCaller("Caller denied access: Security level too low.")
		s.pause()
		return false
	}

	// Check subscription expiry
	if user.Subscription != nil && user.Subscription.Before(time.Now()) {
		s.display.SetColor('C')
		s.display.WriteLine("Your subscription has expired.")
		s.storage.LogCaller(fmt.Sprintf("Expired subscription attempt: %s", name))
		s.pause()
		return false
	}

	// Brute-force protection: block after too many failed attempts on this account
	if s.rateLimiter.IsBlocked(name) {
		s.storage.LogCaller(fmt.Sprintf("Login blocked (too many failed attempts): %s", name))
		s.display.SetColor('C')
		s.display.WriteLine("Too many failed attempts. Try again later.")
		s.pause()
		return false
	}

	// Password prompt with brute force protection
	s.display.SetColor('E')
	s.display.Write("Password: ")
	s.display.ResetColor()
	password := s.readPassword()

	// Verify password
	if !auth.CheckPassword(password, user.Password) {
		s.rateLimiter.RecordFailure(name)
		s.storage.LogCaller(fmt.Sprintf("Incorrect password attempt: %s.", name))
		s.display.SetColor('C')
		s.display.WriteLine("Incorrect password!")
		s.pause()
		return false
	}
	s.rateLimiter.RecordSuccess(name)

	// Check duplicate login
	dup, node, _ := s.nodes.IsDuplicateLogin(user.Name)
	if dup {
		s.display.SetColor('C')
		s.display.WriteLine("Account already active on another node.")
		s.storage.LogCaller(fmt.Sprintf("Duplicate login blocked: %s (already on node %d)", name, node))
		s.pause()
		return false
	}

	// Update user stats
	user.CallsToday++
	user.TotalCalls++
	user.LastLogin = time.Now()
	user.TimeLeftToday = s.config.MaxTimePerLogon
	s.storage.SaveUser(user)

	// Log the login
	s.storage.LogCaller(fmt.Sprintf("%s logged on at %s at %s baud on Node %d.",
		user.Name, time.Now().Format("03:04 PM"), s.baudRate, s.nodeNum))

	s.user = user
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Welcome back, %s!", user.Alias))
	s.pause()
	return true
}

// registerNewUser handles new user registration.
func (s *Session) registerNewUser(name string) bool {
	// Run the sysop-editable NEWUSER.QUE questionnaire (falls back to a built-in
	// default). Bound fields populate the user record; free-form questions are
	// stored to the caller's .ANS file, as in the original TriBBS.
	items := loadNewUserQue("NEWUSER.QUE")
	fields := map[string]string{}
	var qa []string
	qnum := 0
	for _, it := range items {
		s.display.SetColor('E')
		s.display.Write(it.prompt)
		s.display.ResetColor()
		var ans string
		if it.field == "password" {
			ans = s.readPassword()
		} else {
			ans = s.readLine()
		}
		if it.field == "?" {
			qnum++
			qa = append(qa, fmt.Sprintf("Q%d. %s", qnum, strings.TrimSpace(it.prompt)),
				fmt.Sprintf("A%d. %s", qnum, ans))
		} else {
			fields[it.field] = ans
		}
	}

	alias := fields["alias"]
	if alias == "" {
		alias = name
	}
	realName := fields["name"]
	if realName == "" {
		realName = name
	}

	hashedPassword, err := auth.HashPassword(fields["password"])
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("Error creating account!")
		s.pause()
		return false
	}

	user := &domain.User{
		Name:           realName,
		Alias:          alias,
		Password:       hashedPassword,
		Phone:          fields["phone"],
		City:           fields["city"],
		SecurityLevel:  s.config.NewUserSecurity,
		ANSIMode:       1,
		ScreenWidth:    80,
		TimeLeftToday:  s.config.NewUserTimeLimit,
		DailyFileLimit: 10,
		DailyByteLimit: 1024,
		Protocol:       s.config.DefaultProtocol,
		Editor:         "Full",
		StreetAddress:  fields["street"],
		State:          fields["state"],
		ZipCode:        fields["zip"],
		Email:          fields["email"],
		Country:        s.config.DefaultCountry,
	}

	recordNum, err := s.storage.AddUser(user)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error creating account: %v", err))
		s.pause()
		return false
	}

	user.RecordNumber = recordNum
	user.TotalCalls = 1
	user.CallsToday = 1
	user.LastLogin = time.Now()
	s.storage.SaveUser(user)

	// Store questionnaire answers and log per TriBBS's caller-log format.
	if err := writeAnsFile(alias, qa); err != nil {
		s.storage.LogCaller(fmt.Sprintf("Could not write %s.ANS: %v", alias, err))
	}
	s.storage.LogCaller(fmt.Sprintf("NEW USER added: %s", realName))
	if len(qa) > 0 {
		s.storage.LogCaller("NEW USER questionnaire completed.")
	}

	s.user = user
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Welcome to %s, %s!", s.config.BoardName, alias))
	s.pause()
	return true
}

// minutesOn is the whole minutes elapsed since login (TriBBS's @TIMEON).
func (s *Session) minutesOn() int {
	return int(time.Since(s.loginAt).Minutes())
}

// timeLeftMinutes is the caller's remaining minutes today (@TIMELEFT).
func (s *Session) timeLeftMinutes() int {
	if s.user == nil {
		return 0
	}
	if s.user.TimeLeftToday < 0 {
		return 0
	}
	return s.user.TimeLeftToday
}

// locationStr is the caller's "City, ST" for the status bar.
func (s *Session) locationStr() string {
	if s.user == nil {
		return ""
	}
	if s.user.State != "" {
		return s.user.City + ", " + s.user.State
	}
	return s.user.City
}

// showMenu renders a TriBBS .MNU-driven menu screen — the framed two-column box
// (drawn from the .MNU's own colors and items, filtered to the caller's security
// level), the "You have been on..." line, the persistent status bar, and the
// "Enter Selection - [keys ?]?" prompt — then returns the caller's key press.
// In expert mode the box and on-time line are suppressed. The loaded *menu.Menu
// is returned so callers can dispatch/log by the pressed item's fields.
func (s *Session) showMenu(file string) (byte, *menu.Menu) {
	m, err := menu.LoadOrDefault(s.menuDir, file)
	if err != nil {
		// Should not happen — the stock menus are embedded — but never trap the
		// caller in a blank screen if it does.
		s.display.WriteLine("Menu temporarily unavailable.")
		return s.readChar(), nil
	}

	sec := 0
	if s.user != nil {
		sec = s.user.SecurityLevel
	}

	s.display.Clear()
	if !s.expert {
		// A sysop's custom display screen (MAINn.ANS, MESSALL.BBS, ...) overrides
		// the auto-drawn box; otherwise render the box + on-time line from the .MNU.
		if disp := s.findMenuDisplay(m.Name); disp != "" && s.renderDisplayFile(disp) {
			s.display.Write("\r\n")
		} else {
			s.display.Write(m.Render(menu.RenderContext{BoardName: s.config.BoardName, Security: sec}))
			s.display.Write("\r\n\r\n  ")
			s.display.Write(menu.OnlineLine(s.minutesOn(), s.timeLeftMinutes()))
			s.display.Write("\r\n")
		}
	}

	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("\r\nEnter Selection - [%s ?]? ", m.PromptKeys(sec)))
	s.display.ResetColor()

	s.drawStatusBar()
	return s.readChar(), m
}

// drawStatusBar paints TriBBS's persistent two-line status bar pinned to the
// bottom of the screen (rows 23-24), preserving the caller's cursor position so
// the command prompt keeps input focus. Redrawn on every menu.
func (s *Session) drawStatusBar() {
	if s.user == nil {
		return
	}
	conn := strings.ToUpper(s.baudRate)
	if conn == "" {
		conn = "LOCAL"
	}
	bar := menu.StatusBar(menu.StatusInfo{
		Name:       s.user.Name,
		Location:   s.locationStr(),
		Node:       s.nodeNum,
		Connection: conn,
		Security:   s.user.SecurityLevel,
		Calls:      int(s.user.TotalCalls),
		TimeUsed:   s.minutesOn(),
		TimeLeft:   s.timeLeftMinutes(),
	})
	lines := strings.SplitN(bar, "\r\n", 2)
	s.display.Write("\0337") // DEC save cursor
	s.display.MoveTo(22, 0)
	s.display.Write(lines[0])
	if len(lines) > 1 {
		s.display.MoveTo(23, 0)
		s.display.Write(lines[1])
	}
	s.display.Write("\0338") // DEC restore cursor
}

// ensureCurrentConf makes sure the session's joined conference is valid and
// accessible, defaulting to the first accessible conference.
func (s *Session) ensureCurrentConf(confs []domain.Conference) {
	for _, c := range confs {
		if c.Name == s.currentConf && s.canAccessConference(c) {
			return
		}
	}
	s.currentConf = ""
	for _, c := range confs {
		if s.canAccessConference(c) {
			s.currentConf = c.Name
			return
		}
	}
}

// joinConference sets the current conference to the 1-based list index, the
// TriBBS "join a conference" action. Read/Post then act on it.
func (s *Session) joinConference(confs []domain.Conference, idx int) {
	if idx < 0 || idx >= len(confs) {
		return
	}
	if !s.canAccessConference(confs[idx]) {
		s.display.SetColor('C')
		s.display.WriteLine("Conference not available!")
		s.pause()
		return
	}
	s.currentConf = confs[idx].Name
	s.storage.LogCaller(fmt.Sprintf("Joined conference: %s", s.currentConf))
}

// messageMenu handles the message conference system, rendered from MESSAGE.MNU.
// Returns quit=true if the caller chose Goodbye. See internal/session/messagemenu.go
// for the command handlers.
func (s *Session) messageMenu() bool {
	confs, _ := s.storage.GetConferences()
	s.ensureCurrentConf(confs)
	return s.menuLoop("MESSAGE.MNU", "Message Menu", map[byte]func(){
		'C': s.changeConference,
		'E': s.postMessage,
		'R': func() { s.readConferenceMessages(s.currentConf) },
		'N': s.newMessages,
		'Y': s.yourMessages,
		'S': s.searchMessages,
		'Q': s.qwkMailMenu,
	})
}

// postMessage handles posting a new message.
func (s *Session) postMessage() {
	// TriBBS current-conference model: post to the joined conference.
	confName := s.currentConf
	if confName == "" {
		s.display.SetColor('C')
		s.display.WriteLine("No conference joined.")
		s.pause()
		return
	}
	conf, err := s.storage.GetConference(confName)
	if err != nil || conf == nil || !s.canAccessConference(*conf) {
		s.display.SetColor('C')
		s.display.WriteLine("Conference not available!")
		s.pause()
		return
	}

	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Posting to %s", confName))
	s.display.SetColor('E')
	s.display.Write("Subject: ")
	s.display.ResetColor()
	subject := s.readLine()

	s.display.SetColor('E')
	s.display.WriteLine("Enter message (empty line to finish):")
	s.display.ResetColor()

	var lines []string
	for {
		line := s.readLine()
		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	body := strings.Join(lines, "\n")
	if body == "" {
		s.display.SetColor('C')
		s.display.WriteLine("Message cancelled.")
		s.pause()
		return
	}

	post := &domain.Post{
		Conference: confName,
		Author:     s.user.Alias,
		Subject:    subject,
		Body:       body,
		Tick:       0,
	}

	if err := s.storage.AddPost(post); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error posting: %v", err))
	} else {
		s.user.MessagesPosted++
		s.user.MessagesToday++
		s.storage.SaveUser(s.user)
		s.storage.LogCaller(fmt.Sprintf("Entered message %d in %s.", post.ID, confName))
		s.display.SetColor('A')
		s.display.WriteLine("Message posted!")
	}
	s.pause()
}

// canAccessConference reports whether the current user may see a conference.
// Sysops see everything; otherwise the security gate applies and a private
// conference is visible only to its owner (a PM-<alias> mailbox belongs to
// <alias>). This is the single gate that keeps private mail private.
func (s *Session) canAccessConference(c domain.Conference) bool {
	if s.user.SecurityLevel >= s.config.SysopSecurity {
		return true
	}
	if c.SecurityLevel > s.user.SecurityLevel {
		return false
	}
	if c.PrivateConf {
		return strings.EqualFold(c.Name, "PM-"+s.user.Alias)
	}
	return true
}

// accessiblePost fetches a message by id only if its conference is one the user
// may access — blocks reading/replying-to/deleting another user's mail by
// guessing message IDs.
func (s *Session) accessiblePost(id int64) (*domain.Post, bool) {
	post, err := s.storage.GetPost(id)
	if err != nil {
		return nil, false
	}
	conf, err := s.storage.GetConference(post.Conference)
	if err != nil || conf == nil || !s.canAccessConference(*conf) {
		return nil, false
	}
	return post, true
}

// readConferenceMessages shows messages in a specific conference.
func (s *Session) readConferenceMessages(confName string) {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("=== %s ===", confName))
	s.display.ResetColor()

	posts, _ := s.storage.GetPosts(confName, 20)
	if len(posts) == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("No messages.")
		s.pause()
		return
	}

	for _, p := range posts {
		s.display.SetColor('E')
		s.display.Write(fmt.Sprintf("%4d - ", p.ID))
		s.display.SetColor('C')
		s.display.Write(p.PostedAt.Format("01/02/06 15:04") + " ")
		s.display.SetColor('F')
		s.display.Write(fmt.Sprintf("%-15s ", p.Author))
		s.display.SetColor('B')
		if p.ReplyTo > 0 {
			s.display.Write("[RE] ")
		}
		s.display.WriteLine(p.Subject)
	}

	s.display.WriteLine("")
	s.display.SetColor('E')
	s.display.Write("Read#  <R>eply  <D>elete  <M>ail  0=exit: ")
	s.display.ResetColor()
	choice := s.readLine()

	if choice == "0" || choice == "" {
		return
	}

	switch strings.ToLower(choice) {
	case "r":
		s.replyToMessage(confName)
		return
	case "d":
		s.deleteMessage(confName)
		return
	case "m":
		s.sendPrivateMail()
		return
	}

	// Try to read a specific message
	var id int64
	fmt.Sscanf(choice, "%d", &id)
	if id == 0 {
		return
	}
	post, ok := s.accessiblePost(id)
	if ok {
		s.display.Clear()
		conf, _ := s.storage.GetConference(post.Conference)
		flag := "<PUBLIC>"
		if conf != nil && conf.PrivateConf {
			flag = "<PRIVATE>"
		}
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Number  : %d", post.ID))
		if post.ReplyTo > 0 {
			s.display.WriteLine(fmt.Sprintf("Reply To: %d", post.ReplyTo))
		}
		s.display.WriteLine(fmt.Sprintf("Confer  : %s  %s", post.Conference, flag))
		s.display.WriteLine(fmt.Sprintf("From    : %s", post.Author))
		s.display.WriteLine(fmt.Sprintf("Subject : %s", post.Subject))
		s.display.WriteLine(fmt.Sprintf("Date    : %s", post.PostedAt.Format("01/02/06 03:04 PM")))
		s.display.SetColor('A')
		s.display.WriteLine("════════════════════════════════════════════════════════════")
		s.display.ResetColor()
		s.display.WriteLine(post.Body)
		s.pause()
	}
}

// replyToMessage handles replying to a message.
func (s *Session) replyToMessage(confName string) {
	s.display.SetColor('E')
	s.display.Write("Message # to reply to: ")
	s.display.ResetColor()
	idStr := s.readLine()
	var replyTo int64
	fmt.Sscanf(idStr, "%d", &replyTo)
	if replyTo == 0 {
		return
	}

	orig, ok := s.accessiblePost(replyTo)
	if !ok {
		s.display.SetColor('C')
		s.display.WriteLine("Message not found!")
		s.pause()
		return
	}

	subject := "Re: " + orig.Subject
	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("Subject [%s]: ", subject))
	s.display.ResetColor()
	newSubj := s.readLine()
	if newSubj != "" {
		subject = newSubj
	}

	s.display.SetColor('E')
	s.display.WriteLine("Enter reply (empty line to finish):")
	s.display.ResetColor()

	var lines []string
	for {
		line := s.readLine()
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	body := strings.Join(lines, "\n")
	if body == "" {
		s.display.SetColor('C')
		s.display.WriteLine("Reply cancelled.")
		s.pause()
		return
	}

	post := &domain.Post{
		Conference: confName,
		Author:     s.user.Alias,
		ReplyTo:    replyTo,
		Subject:    subject,
		Body:       body,
	}
	if err := s.storage.AddPost(post); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.user.MessagesPosted++
		s.user.MessagesToday++
		s.storage.SaveUser(s.user)
		s.storage.LogCaller(fmt.Sprintf("Entered message %d in %s.", post.ID, confName))
		s.display.SetColor('A')
		s.display.WriteLine("Reply posted!")
	}
	s.pause()
}

// deleteMessage handles message deletion.
func (s *Session) deleteMessage(confName string) {
	s.display.SetColor('E')
	s.display.Write("Message # to delete: ")
	s.display.ResetColor()
	idStr := s.readLine()
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	if id == 0 {
		return
	}

	post, ok := s.accessiblePost(id)
	if !ok {
		s.display.SetColor('C')
		s.display.WriteLine("Message not found!")
		s.pause()
		return
	}

	if post.Author != s.user.Alias && s.user.SecurityLevel < s.config.SysopSecurity {
		s.display.SetColor('C')
		s.display.WriteLine("You can only delete your own messages!")
		s.pause()
		return
	}

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete '%s'? (y/n): ", post.Subject))
	s.display.ResetColor()
	if strings.ToLower(s.readLine()) == "y" {
		if err := s.storage.DeletePost(id); err != nil {
			s.display.SetColor('C')
			s.display.WriteLine(fmt.Sprintf("Error: %v", err))
		} else {
			s.storage.LogCaller(fmt.Sprintf("%s deleted message #%d", s.user.Alias, id))
			s.display.SetColor('A')
			s.display.WriteLine("Deleted!")
		}
	}
	s.pause()
}

// sendPrivateMail handles sending private mail.
func (s *Session) sendPrivateMail() {
	s.display.SetColor('E')
	s.display.Write("Send to (alias): ")
	s.display.ResetColor()
	toAlias := s.readLine()
	if toAlias == "" {
		return
	}

	recipient, err := s.storage.GetUserByAlias(toAlias)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("User not found!")
		s.pause()
		return
	}

	s.display.SetColor('E')
	s.display.Write("Subject: ")
	s.display.ResetColor()
	subject := s.readLine()

	s.display.SetColor('E')
	s.display.WriteLine("Enter message (empty line to finish):")
	s.display.ResetColor()

	var lines []string
	for {
		line := s.readLine()
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	body := strings.Join(lines, "\n")
	if body == "" {
		return
	}

	privConf := "PM-" + recipient.Alias
	if _, err := s.storage.GetConference(privConf); err != nil {
		s.storage.AddConference(&domain.Conference{
			Name:          privConf,
			SecurityLevel: 0,
			PrivateConf:   true,
		})
	}

	post := &domain.Post{
		Conference: privConf,
		Author:     s.user.Alias,
		Subject:    subject,
		Body:       body,
	}
	if err := s.storage.AddPost(post); err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Error: %v", err))
	} else {
		s.storage.LogCaller(fmt.Sprintf("%s mailed %s", s.user.Alias, toAlias))
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("Mail sent to %s!", toAlias))
	}
	s.pause()
}

// fileMenu handles the file area system.
// fileMenu handles the file transfer system, rendered from FILES.MNU. Returns
// quit=true if the caller chose Goodbye. Handlers live in filemenu.go; the
// per-file operations (download/view/tag/edit/move/delete) happen inside the
// listFiles browser, which C/L/R/O enter.
func (s *Session) fileMenu() bool {
	return s.menuLoop("FILES.MNU", "File Menu", map[byte]func(){
		'C': s.listFiles,        // Change File Area (browser prompts for the area)
		'L': s.listFiles,        // List Files
		'N': s.scanNewFiles,     // New Files
		'T': s.searchFiles,      // Text Search File Lists
		'E': s.editBatchQueue,   // Edit Batch Queue
		'U': s.uploadFile,       // Upload File
		'D': s.downloadPrompt,   // Download File (by name)
		'V': s.viewArchivePrompt, // View Archive (by name)
		'R': s.listFiles,        // Remove File (sysop; done in the browser)
		'O': s.listFiles,        // Move File (sysop; done in the browser)
	})
}

// searchFiles searches filenames and descriptions across all accessible file
// areas for a term (TriBBS's file-search command).
func (s *Session) searchFiles() {
	s.display.SetColor('E')
	s.display.Write("Search for: ")
	s.display.ResetColor()
	term := strings.ToLower(strings.TrimSpace(s.readLine()))
	if term == "" {
		return
	}
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("=== Files matching '%s' ===", term))
	s.display.ResetColor()
	areas, _ := s.storage.GetFileAreas()
	found := 0
	for _, area := range areas {
		if area.SecurityLevel > s.user.SecurityLevel {
			continue
		}
		files, _ := s.storage.GetFiles(area.Name, 200)
		for _, f := range files {
			if strings.Contains(strings.ToLower(f.Name), term) ||
				strings.Contains(strings.ToLower(f.Description), term) {
				found++
				s.display.SetColor('E')
				s.display.Write(fmt.Sprintf("  %-12s ", f.Name))
				s.display.SetColor('F')
				s.display.Write(fmt.Sprintf("%8d ", f.Size))
				s.display.SetColor('B')
				s.display.WriteLine(fmt.Sprintf("%s (%s)", f.Description, area.Name))
			}
		}
	}
	if found == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("No matching files.")
	}
	s.storage.LogCaller(fmt.Sprintf("Searched files for: %s", term))
	s.pause()
}

// scanNewFiles shows files uploaded since user's last login.
func (s *Session) scanNewFiles() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("=== New File Scan ===")
	s.display.ResetColor()

	areas, _ := s.storage.GetFileAreas()
	found := 0
	for _, area := range areas {
		if area.SecurityLevel > s.user.SecurityLevel {
			continue
		}
		files, _ := s.storage.GetFiles(area.Name, 50)
		for _, f := range files {
			if f.UploadedAt.After(s.user.LastLogin) {
				if found == 0 {
					s.display.SetColor('E')
					s.display.WriteLine("New files since your last visit:")
					s.display.WriteLine("")
				}
				found++
				s.display.SetColor('F')
				s.display.WriteLine(fmt.Sprintf("  %-30s %8d  %s", f.Name, f.Size, area.Name))
				s.display.SetColor('B')
				s.display.WriteLine(fmt.Sprintf("    %s", f.Description))
			}
		}
	}

	if found == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("No new files since your last visit.")
	}
	s.pause()
}

// uploadFile handles file uploads.
func (s *Session) uploadFile() {
	s.display.Clear()
	s.display.SetColor('E')
	s.display.WriteLine("Upload File")
	s.display.ResetColor()
	s.display.WriteLine("")

	s.display.Write("Area (number or name): ")
	areaName := s.resolveAreaName(s.readLine())
	s.display.Write("Filename: ")
	filename := s.readLine()

	if areaName == "" || filename == "" {
		s.display.SetColor('C')
		s.display.WriteLine("Upload cancelled.")
		s.pause()
		return
	}

	s.display.SetColor('B')
	s.display.WriteLine(fmt.Sprintf("Protocol: %s", protocol.ProtocolNames[s.user.Protocol]))
	s.display.WriteLine("Waiting for file transfer...")
	s.display.ResetColor()

	err := s.uploadFileWithProtocol(areaName, filename)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Upload error: %v", err))
	} else {
		s.display.SetColor('A')
		s.display.WriteLine("Upload complete!")
	}
	s.pause()
}

// resolveAreaName maps a file-area selection to a canonical area name. It
// accepts either a 1-based number from the file-area list (as shown in the
// file menu) or a literal area name, mirroring how conferences are selected.
func (s *Session) resolveAreaName(input string) string {
	input = strings.TrimSpace(input)
	if n, err := strconv.Atoi(input); err == nil {
		areas, _ := s.storage.GetFileAreas()
		if n >= 1 && n <= len(areas) {
			return areas[n-1].Name
		}
	}
	return input
}

// listFiles shows files in an area.
func (s *Session) listFiles() {
	s.display.SetColor('E')
	s.display.Write("Area (number or name): ")
	s.display.ResetColor()
	areaName := s.resolveAreaName(s.readLine())

	// Check area security
	area, err := s.storage.GetFileArea(areaName)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine("File area not found!")
		s.pause()
		return
	}
	if area.SecurityLevel > s.user.SecurityLevel {
		s.display.SetColor('C')
		s.display.WriteLine("Security level too low for this area!")
		s.pause()
		return
	}

	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine(fmt.Sprintf("=== Files in %s ===", areaName))
		s.display.ResetColor()

		files, _ := s.storage.GetFiles(areaName, 50)
		files = fileutil.SortFiles(files, area.SortType)
		if len(files) == 0 {
			s.display.SetColor('B')
			s.display.WriteLine("No files in this area.")
			s.pause()
			return
		}

		for i, f := range files {
			mark := " "
			if s.isFlagged(f) {
				mark = "*"
			}
			s.display.SetColor('E')
			s.display.Write(fmt.Sprintf("%s[%d] %-29s ", mark, i+1, f.Name))
			s.display.SetColor('F')
			s.display.Write(fmt.Sprintf("%8d ", f.Size))
			s.display.SetColor('C')
			s.display.Write(f.UploadedAt.Format("01-02-06") + " ")
			s.display.SetColor('B')
			s.display.WriteLine(f.Description)
		}

		s.display.WriteLine("")
		s.display.SetColor('E')
		s.display.Write("<D>ownload  <T>ag  <V>iew  <E>dit  <M>ove  <X>Delete  0=exit: ")
		s.display.ResetColor()
		choice := s.readLine()

		if choice == "0" || choice == "" {
			return
		}

		fileNumPrompt := func(label string) int {
			s.display.SetColor('E')
			s.display.Write(label)
			s.display.ResetColor()
			var n int
			fmt.Sscanf(s.readLine(), "%d", &n)
			return n
		}

		switch strings.ToLower(choice) {
		case "d":
			if n := fileNumPrompt("File # to download: "); n > 0 && n <= len(files) {
				s.downloadFile(files[n-1])
			}
		case "t":
			if n := fileNumPrompt("File # to tag/untag: "); n > 0 && n <= len(files) {
				s.toggleFlag(files[n-1])
			}
		case "v":
			if n := fileNumPrompt("File # to view (archive contents): "); n > 0 && n <= len(files) {
				s.viewArchive(files[n-1])
			}
		case "e":
			s.editFileDescription(files)
		case "m":
			s.moveFile(files)
		case "x":
			s.deleteFileFromArea(files)
		}
	}
}

// editFileDescription edits a file's description.
func (s *Session) editFileDescription(files []domain.FileEntry) {
	s.display.SetColor('E')
	s.display.Write("File # to edit: ")
	s.display.ResetColor()
	numStr := s.readLine()
	var fileNum int
	fmt.Sscanf(numStr, "%d", &fileNum)
	if fileNum <= 0 || fileNum > len(files) {
		return
	}

	file := &files[fileNum-1]
	// Only uploader or sysop can edit
	if file.UploadedBy != s.user.Alias && s.user.SecurityLevel < s.config.SysopSecurity {
		s.display.SetColor('C')
		s.display.WriteLine("Only the uploader or sysop can edit descriptions!")
		s.pause()
		return
	}

	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("New description [%s]: ", file.Description))
	s.display.ResetColor()
	newDesc := s.readLine()
	if newDesc != "" {
		file.Description = newDesc
		s.storage.SaveFile(file)
		s.display.SetColor('A')
		s.display.WriteLine("Description updated!")
		s.pause()
	}
}

// moveFile moves a file to a different area.
func (s *Session) moveFile(files []domain.FileEntry) {
	if s.user.SecurityLevel < s.config.SysopSecurity {
		s.display.SetColor('C')
		s.display.WriteLine("Sysop access required!")
		s.pause()
		return
	}

	s.display.SetColor('E')
	s.display.Write("File # to move: ")
	s.display.ResetColor()
	numStr := s.readLine()
	var fileNum int
	fmt.Sscanf(numStr, "%d", &fileNum)
	if fileNum <= 0 || fileNum > len(files) {
		return
	}

	s.display.SetColor('E')
	s.display.Write("Move to area: ")
	s.display.ResetColor()
	newArea := s.readLine()
	if newArea == "" {
		return
	}

	file := &files[fileNum-1]
	oldArea := file.Area
	file.Area = newArea
	s.storage.SaveFile(file)
	s.storage.LogCaller(fmt.Sprintf("Moved file: %s.  From: %s to %s.", file.Name, oldArea, newArea))
	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("Moved to %s!", newArea))
	s.pause()
}

// deleteFileFromArea deletes a file from an area.
func (s *Session) deleteFileFromArea(files []domain.FileEntry) {
	s.display.SetColor('E')
	s.display.Write("File # to delete: ")
	s.display.ResetColor()
	numStr := s.readLine()
	var fileNum int
	fmt.Sscanf(numStr, "%d", &fileNum)
	if fileNum <= 0 || fileNum > len(files) {
		return
	}

	file := &files[fileNum-1]
	// Only uploader or sysop can delete
	if file.UploadedBy != s.user.Alias && s.user.SecurityLevel < s.config.SysopSecurity {
		s.display.SetColor('C')
		s.display.WriteLine("Only the uploader or sysop can delete files!")
		s.pause()
		return
	}

	s.display.SetColor('C')
	s.display.Write(fmt.Sprintf("Delete %s? (y/n): ", file.Name))
	s.display.ResetColor()
	if strings.ToLower(s.readLine()) == "y" {
		s.storage.DeleteFile(file.ID)
		s.storage.LogCaller(fmt.Sprintf("Removed File: %s", file.Name))
		s.display.SetColor('A')
		s.display.WriteLine("File deleted!")
		s.pause()
	}
}

// simulateModemConnect runs the modem simulator's answer/negotiation sequence
// and adopts the negotiated baud rate for this session (used by @BAUD, the
// who's-online list, and DOOR.SYS drop files).
func (s *Session) simulateModemConnect() {
	sim := modem.NewModemSimulator(modem.ModemConfig{
		Allow300:  true,
		Allow1200: true,
		Allow2400: s.config.Allow2400Baud,
		MaxBaud:   modem.BaudRate(s.config.MaxBaud),
		RingCount: 1,
	})
	// TriBBS connect sequence, in the order the original printed it — BEFORE the
	// caller is considered connected. Ev: "Initializing system at %u baud."
	// @0x1ba96, "Remote Node: Waiting for Caller" @0x1bab6, "CONNECT" @0x1bbd6,
	// "[Serial Port Locked at %u Baud]" @0x1c950, "[Error Correcting Modem
	// Detected]" @0x1c971.
	s.display.SetColor('B')
	s.display.WriteLine(fmt.Sprintf("Initializing system at %d baud.", s.config.MaxBaud))
	s.display.WriteLine("Remote Node: Waiting for Caller")
	s.display.ResetColor()

	baud, err := sim.SimulateConnection(s.config.MaxBaud)
	if err != nil {
		// Baud gate: the caller's rate isn't supported (e.g. "Sorry but %s
		// doesn't support 300 baud calls!").
		s.display.SetColor('C')
		s.display.WriteLine(err.Error())
		s.display.ResetColor()
		return
	}
	s.baudRate = modem.FormatBaudRate(baud)

	s.display.SetColor('A')
	s.display.WriteLine(fmt.Sprintf("CONNECT %s", modem.FormatBaudRate(baud)))
	s.display.WriteLine(fmt.Sprintf("[Serial Port Locked at %d Baud]", s.config.MaxBaud))
	s.display.WriteLine("[Error Correcting Modem Detected]")
	s.display.ResetColor()
}

// logMenuSelection records a menu choice to the caller log, matching TriBBS's
// dispatch-log format ("Selected %s from the %s.").
func (s *Session) logMenuSelection(menu, item string) {
	if item == "" || s.storage == nil {
		return
	}
	s.storage.LogCaller(fmt.Sprintf("Selected %s from the %s.", item, menu))
}

// writeTemplate writes rendered template output, acting on the flow-control
// sentinels the template engine embeds for @PAUSE/@HANGUP/@MOREON/@MOREOFF/
// @BREAKON/@BREAKOFF. Text is flushed to the display up to each directive so a
// mid-screen @PAUSE stops at the right place. Returns true if @HANGUP was seen.
func (s *Session) writeTemplate(rendered string) bool {
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			s.display.Write(ui.NormalizeBoxLines(buf.String()))
			buf.Reset()
		}
	}
	runes := []rune(rendered)
	for i := 0; i < len(runes); i++ {
		if runes[i] == template.CtrlMarker && i+1 < len(runes) {
			flush()
			switch runes[i+1] {
			case 'P': // @PAUSE
				s.pause()
			case 'H': // @HANGUP
				s.hangup = true
				return true
			case 'M': // @MOREON
				s.moreEnabled = true
			case 'm': // @MOREOFF
				s.moreEnabled = false
			case 'B': // @BREAKON
				s.breakEnabled = true
			case 'b': // @BREAKOFF
				s.breakEnabled = false
			}
			i++ // consume the command rune
			continue
		}
		buf.WriteRune(runes[i])
	}
	flush()
	return false
}

// applyGraphicsMode enables RIPscrip rendering on the display when the user's
// ANSIMode is 2 (RIP). The DisplayPort interface doesn't expose SetRIPMode, so
// we probe for it — the ANSI adapter implements it; others are unaffected.
func (s *Session) applyGraphicsMode() {
	if rip, ok := s.display.(interface{ SetRIPMode(bool) }); ok {
		rip.SetRIPMode(s.user.ANSIMode == 2)
	}
}

// doorMenu shows available doors.
func (s *Session) doorMenu() {
	for {
		s.display.Clear()
		rendered, err := s.templateEngine.RenderFile("doors.tpl", s.templateData)
		if err != nil {
			s.display.SetColor('A')
			s.display.WriteLine("Doors")
		} else {
			s.writeTemplate(rendered)
		}

		doors, _ := s.storage.GetDoors()
		if len(doors) == 0 {
			// Fall back to a legacy TriBBS DOORS.MNU (monthly menu for the
			// current month, e.g. DOORS07.MNU) if one is present on disk.
			if mnu, err := doormenu.LoadDoorsFromMnu(doormenu.GetMonthlyDoorsMnu(".")); err == nil {
				doors = mnu
			}
		}
		if len(doors) == 0 {
			s.display.SetColor('B')
			s.display.WriteLine("║  No doors configured.                                       ║")
		} else {
			for _, d := range doors {
				s.display.SetColor('F')
				s.display.WriteLine(fmt.Sprintf("║   <%c> %-52s ║", d.HotKey, d.Name))
				s.display.SetColor('B')
				s.display.WriteLine(fmt.Sprintf("║       %-52s ║", d.Description))
			}
		}

		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine("║   <X> Exit                                                  ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.menuPrompt("DOORS", "X")

		choice := s.menuKey()
		if len(choice) == 0 {
			continue
		}

		if choice[0] == 'x' || choice[0] == 'X' {
			return
		}

		// Find and execute the door
		for _, d := range doors {
			if strings.EqualFold(string(choice[0]), string(d.HotKey)) {
				if d.Security > s.user.SecurityLevel {
					s.display.SetColor('C')
					s.display.WriteLine("Security level too low!")
					s.pause()
				} else {
					s.executeDoor(d)
				}
				break
			}
		}
	}
}

// chatRequest sends a chat request to the sysop.
func (s *Session) chatRequest() {
	// TriBBS <C> Chat = page the sysop directly. Find an online sysop-level node
	// (other than ourselves) and page it; the teleconference stays available as
	// a modern extra below.
	s.display.Clear()
	first := s.user.Name
	if i := strings.IndexByte(first, ' '); i > 0 {
		first = first[:i]
	}
	s.storage.LogCaller("Requested chat.")

	sysopNode := 0
	for _, n := range s.nodes.GetNodes() {
		if n.NodeNumber != s.nodeNum && n.Active && n.SecurityLevel >= s.config.SysopSecurity {
			sysopNode = n.NodeNumber
			break
		}
	}

	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("Hi %s.  Paging the sysop for chat...", first))
	s.display.ResetColor()
	switch {
	case sysopNode == 0:
		s.display.SetColor('C')
		s.display.WriteLine("The sysop is not available for chat right now.")
	case s.nodes.SendPage(s.nodeNum, sysopNode, s.user.Alias) != nil:
		s.display.SetColor('C')
		s.display.WriteLine("The sysop is not accepting pages right now.")
	default:
		s.firePageBAT("", sysopNode)
		s.display.SetColor('A')
		s.display.WriteLine("The sysop has been paged. Please wait...")
	}
	s.display.ResetColor()
	s.pause()
	// TriBBS <P> pages the sysop and returns to the menu. The live
	// teleconference is its own command (<T> TeleChat), so we don't drop the
	// caller into it here.
}

// whoOnline shows who's currently logged in.
func (s *Session) whoOnline() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine("║  Who's Online                                               ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

	nodes := s.nodes.GetNodes()
	activeCount := 0
	for _, n := range nodes {
		if n.Active {
			activeCount++
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║  Node %2d: %-20s %-25s ║",
				n.NodeNumber, n.UserAlias, n.Activity))
		}
	}

	if activeCount == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("║  No other users online.                                     ║")
	}

	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine(fmt.Sprintf("║  Total users online: %-38d ║", activeCount))
	s.display.SetColor('A')
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()

	s.display.SetColor('B')
	s.display.Write("Press Enter to continue...")
	s.display.ResetColor()
	s.readLine()
}

// userConfig shows the user configuration menu.
func (s *Session) userConfig() {
	for {
		s.display.Clear()
		s.display.SetColor('A')
		s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
		s.display.SetColor('E')
		s.display.WriteLine("║  User Configuration                                         ║")
		s.display.SetColor('A')
		s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
		s.display.SetColor('F')
		s.display.WriteLine(fmt.Sprintf("║  Name:      %-47s ║", s.user.Name))
		s.display.WriteLine(fmt.Sprintf("║  Alias:     %-47s ║", s.user.Alias))
		s.display.WriteLine(fmt.Sprintf("║  City:      %-47s ║", s.user.City))
		s.display.WriteLine(fmt.Sprintf("║  Phone:     %-47s ║", s.user.Phone))
		s.display.WriteLine(fmt.Sprintf("║  Security:  %-47d ║", s.user.SecurityLevel))
		s.display.WriteLine(fmt.Sprintf("║  Protocol:  %-47s ║", protocol.ProtocolNames[s.user.Protocol]))
		s.display.WriteLine(fmt.Sprintf("║  ANSI Mode:%-47s ║", ansiModeName(s.user.ANSIMode)))
		s.display.WriteLine(fmt.Sprintf("║  Width:     %-47d ║", s.user.ScreenWidth))
		s.display.WriteLine(fmt.Sprintf("║  Editor:    %-47s ║", s.user.Editor))
		// v11.6+ fields
		s.display.WriteLine(fmt.Sprintf("║  Email:     %-47s ║", s.user.Email))
		s.display.WriteLine(fmt.Sprintf("║  Address:   %-47s ║", s.user.StreetAddress))
		s.display.WriteLine(fmt.Sprintf("║  City/State:%-47s ║", s.user.City+", "+s.user.State))
		s.display.WriteLine(fmt.Sprintf("║  ZIP:       %-47s ║", s.user.ZipCode))
		s.display.SetColor('A')
		s.display.WriteLine("╠═══════════════════════════════════════════════════════════════════╗")
		s.display.SetColor('F')
		s.display.WriteLine("║   <P> Password   <A> Alias     <H> Phone     <E> Editor        ║")
		s.display.WriteLine("║   <T> Protocol   <N> ANSI Mode <W> Width     <C> Chat Page     ║")
		s.display.WriteLine("║   <@> Email      <S> Street    <I> City/State <Z> ZIP          ║")
		s.display.WriteLine("║   <X> Exit                                                      ║")
		s.display.SetColor('A')
		s.display.WriteLine("╚═══════════════════════════════════════════════════════════════════╝")
		s.display.ResetColor()
		s.menuPrompt("CONFIG", "P A H E T N W C @ S I Z X")

		choice := s.menuKey()
		if len(choice) == 0 {
			continue
		}

		switch choice[0] {
		case 'x', 'X':
			return
		case 'p', 'P':
			s.display.SetColor('E')
			s.display.Write("New password: ")
			s.display.ResetColor()
			newPassword := s.readPassword()
			hashedPassword, err := auth.HashPassword(newPassword)
			if err != nil {
				s.display.SetColor('C')
				s.display.WriteLine("Error hashing password!")
			} else {
				s.user.Password = hashedPassword
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("Password changed!")
				s.storage.LogCaller("Caller changed password.")
			}
			s.pause()
		case 'a', 'A':
			if !s.config.AllowAliases {
				s.display.SetColor('C')
				s.display.WriteLine("Aliases not allowed on this system.")
				s.pause()
				continue
			}
			s.display.SetColor('E')
			s.display.Write("New alias: ")
			s.display.ResetColor()
			alias := s.readLine()
			if alias != "" {
				existing, _ := s.storage.GetUserByAlias(alias)
				if existing != nil && existing.RecordNumber != s.user.RecordNumber {
					s.display.SetColor('C')
					s.display.WriteLine("Alias already taken!")
				} else {
					s.user.Alias = alias
					s.storage.SaveUser(s.user)
					s.display.SetColor('A')
					s.display.WriteLine("Alias changed!")
					s.storage.LogCaller("Caller changed alias.")
				}
			}
			s.pause()
		case 'h', 'H':
			s.display.SetColor('E')
			s.display.Write("New phone: ")
			s.display.ResetColor()
			phone := s.readLine()
			if phone != "" {
				s.user.Phone = phone
				s.storage.SaveUser(s.user)
				s.storage.LogCaller("Caller changed phone number.")
				s.display.SetColor('A')
				s.display.WriteLine("Phone changed!")
			}
			s.pause()
		case 'e', 'E':
			s.display.SetColor('E')
			s.display.Write("Editor (Full/Line): ")
			s.display.ResetColor()
			editor := s.readLine()
			if editor != "" {
				s.user.Editor = editor
				s.storage.SaveUser(s.user)
				s.storage.LogCaller("Caller changed editor.")
				s.display.SetColor('A')
				s.display.WriteLine("Editor changed!")
			}
			s.pause()
		case 't', 'T':
			s.display.SetColor('E')
			s.display.WriteLine("Protocols: <A> Ascii  <X> Xmodem  <K> Xmodem-1K  <Y> Ymodem  <G> Ymodem-G  <Z> Zmodem")
			s.display.Write("Select: ")
			s.display.ResetColor()
			p := s.readLine()
			if len(p) > 0 {
				proto := byte(p[0])
				if _, ok := protocol.ProtocolNames[proto]; ok {
					s.user.Protocol = proto
					s.storage.SaveUser(s.user)
					s.storage.LogCaller("Caller changed protocol.")
					s.display.SetColor('A')
					s.display.WriteLine("Protocol changed!")
				} else {
					s.display.SetColor('C')
					s.display.WriteLine("Invalid protocol!")
				}
			}
			s.pause()
		case 'n', 'N':
			s.display.SetColor('E')
			s.display.WriteLine("ANSI: [0] None  [1] ANSI  [2] RIPscrip")
			s.display.Write("Select: ")
			s.display.ResetColor()
			m := s.readLine()
			if len(m) > 0 {
				mode := int(m[0] - '0')
				if mode >= 0 && mode <= 2 {
					s.user.ANSIMode = mode
					s.storage.SaveUser(s.user)
					s.applyGraphicsMode()
					s.display.SetColor('A')
					s.display.WriteLine("ANSI mode changed!")
				} else {
					s.display.SetColor('C')
					s.display.WriteLine("Invalid mode!")
				}
			}
			s.pause()
		case 'w', 'W':
			s.display.SetColor('E')
			s.display.Write("Screen width (40-132): ")
			s.display.ResetColor()
			wStr := s.readLine()
			var w int
			fmt.Sscanf(wStr, "%d", &w)
			if w >= 40 && w <= 132 {
				s.user.ScreenWidth = w
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("Width changed!")
			} else {
				s.display.SetColor('C')
				s.display.WriteLine("Invalid! (40-132)")
			}
			s.pause()
		case 'c', 'C':
			avail := s.yesNo("Accept chat pages?", true)
			s.nodes.SetChatAvail(s.nodeNum, avail)
			s.storage.LogCaller("Caller changed chat status.")
			s.display.SetColor('A')
			if avail {
				s.display.WriteLine("Chat pages enabled.")
			} else {
				s.display.WriteLine("Chat pages disabled.")
			}
			s.pause()
		case '@': // Email (v11.6+)
			s.display.SetColor('E')
			s.display.Write("New email: ")
			s.display.ResetColor()
			email := s.readLine()
			if email != "" {
				s.user.Email = email
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("Email changed!")
				s.storage.LogCaller(fmt.Sprintf("%s changed email", s.user.Alias))
			}
			s.pause()
		case 's', 'S': // Street address (v11.6+)
			s.display.SetColor('E')
			s.display.Write("New street address: ")
			s.display.ResetColor()
			street := s.readLine()
			if street != "" {
				s.user.StreetAddress = street
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("Street address changed!")
			}
			s.pause()
		case 'i', 'I': // City/State (v11.6+)
			s.display.SetColor('E')
			s.display.Write("New city: ")
			s.display.ResetColor()
			city := s.readLine()
			if city != "" {
				s.user.City = city
			}
			s.display.SetColor('E')
			s.display.Write("New state: ")
			s.display.ResetColor()
			state := s.readLine()
			if state != "" {
				s.user.State = state
			}
			if city != "" || state != "" {
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("City/State changed!")
			}
			s.pause()
		case 'z', 'Z': // ZIP code (v11.6+)
			s.display.SetColor('E')
			s.display.Write("New ZIP code: ")
			s.display.ResetColor()
			zip := s.readLine()
			if zip != "" {
				s.user.ZipCode = zip
				s.storage.SaveUser(s.user)
				s.display.SetColor('A')
				s.display.WriteLine("ZIP code changed!")
			}
			s.pause()
		}
	}
}

// ansiModeName returns the display name for an ANSI mode.
func ansiModeName(mode int) string {
	switch mode {
	case 0:
		return "None"
	case 1:
		return "ANSI"
	case 2:
		return "RIPscrip"
	default:
		return "Unknown"
	}
}

// readBulletins shows available bulletins.
func (s *Session) readBulletins() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine("║  Bulletins                                                  ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")

	bulletins, _ := s.storage.GetBulletins()
	if len(bulletins) == 0 {
		s.display.SetColor('B')
		s.display.WriteLine("║  No bulletins available.                                    ║")
	} else {
		for i, b := range bulletins {
			s.display.SetColor('F')
			s.display.WriteLine(fmt.Sprintf("║   [%d] - %-50s ║", i+1, b.Name))
		}
	}

	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine("║   <X> Exit                                                  ║")
	s.display.SetColor('A')
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()
	s.menuPrompt("BULLETINS", "X")

	choice := s.menuKey()
	if len(choice) > 0 && choice[0] != 'x' && choice[0] != 'X' {
		// Selection is a 1-based position in the list shown above, not the
		// bulletin's database ID (those only coincide until a bulletin is
		// deleted). Index the list we already rendered.
		var idx int
		fmt.Sscanf(choice, "%d", &idx)
		if idx >= 1 && idx <= len(bulletins) {
			bulletin := bulletins[idx-1]
			s.storage.LogCaller(fmt.Sprintf("Read Bulletin No. %d.", idx))
			s.display.Clear()
			s.display.SetColor('A')
			s.display.WriteLine(fmt.Sprintf("=== %s ===", bulletin.Name))
			s.display.ResetColor()
			s.display.WriteLine(bulletin.Content)
			s.pause()
		}
	}
}

// downloadFile sends a file to the user using their selected protocol.
func (s *Session) downloadFile(file domain.FileEntry) {
	s.display.Clear()
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("Downloading: %s", file.Name))
	s.display.WriteLine(fmt.Sprintf("Size: %d bytes", file.Size))
	s.display.WriteLine("")
	s.display.SetColor('B')
	s.display.WriteLine(fmt.Sprintf("Protocol: %s", protocol.ProtocolNames[s.user.Protocol]))
	s.display.ResetColor()

	// Perform actual file transfer
	err := s.downloadFileWithProtocol(file)
	if err != nil {
		s.display.SetColor('C')
		s.display.WriteLine(fmt.Sprintf("Transfer error: %v", err))
		s.pause()
		return
	}

	// Update user stats
	s.user.FilesDownloaded++
	s.user.KDownloaded += file.Size / 1024
	s.storage.SaveUser(s.user)

	// Log the download
	s.storage.LogCaller(fmt.Sprintf("Downloaded file: %s.", file.Name))

	s.display.SetColor('A')
	s.display.WriteLine("Transfer complete!")
	s.pause()
}

// logoff handles the logout sequence.
func (s *Session) logoff() {
	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine(fmt.Sprintf("║  Goodbye, %-49s! ║", s.user.Alias))
	s.display.SetColor('A')
	s.display.WriteLine("║                                                            ║")
	s.display.SetColor('B')
	s.display.WriteLine("║  Thanks for calling! See you next time!                    ║")
	s.display.SetColor('A')
	s.display.WriteLine("║                                                            ║")
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()

	s.fireGoodbyeBAT()
	s.storage.LogCaller(fmt.Sprintf("%s logged off", s.user.Alias))
	s.pause()
}

// Helper methods

// getCallerID returns the caller's identification.
// In a real modem BBS, this comes from the modem's caller ID detection.
// In tresbbs (telnet/SSH), we use the remote IP address or a placeholder.
func (s *Session) getCallerID() string {
	// For telnet/SSH connections, we don't have real caller ID
	// Return empty string to indicate no caller ID available
	// The Caller ID validation will handle this based on config
	if s.remoteAddr != "" {
		// Could extract IP address here for logging
		// but caller ID is a phone-line concept
		return ""
	}
	return ""
}

func (s *Session) showWelcome() {
	rendered, err := s.templateEngine.RenderFile("welcome.tpl", s.templateData)
	if err != nil {
		// Fallback to hardcoded welcome if template not found
		s.display.SetColor('A')
		s.display.WriteLine("Welcome to " + s.config.BoardName)
		s.display.ResetColor()
		return
	}
	s.writeTemplate(rendered)
}

func (s *Session) readLine() string {
	line, err := s.display.ReadLine()
	if err != nil {
		s.carrierLost()
		return ""
	}
	return line
}

func (s *Session) readChar() byte {
	b, err := s.display.ReadKey()
	if err != nil {
		s.carrierLost()
		return 0
	}
	return b
}

// menuKey reads a single keystroke for TriBBS-style instant menu dispatch (no
// Enter required). A bare CR/LF returns "" so existing "empty = redisplay/exit"
// guards keep working. Numbered list selection is single-digit (1-9), matching
// the original's single-key menus.
func (s *Session) menuKey() string {
	b := s.readChar()
	if b == '\r' || b == '\n' || b == 0 {
		return ""
	}
	return string(b)
}

// menuPrompt renders TriBBS's canonical command prompt: "NAME - [<keys> ?]? "
// where keys is the space-joined valid-key list and every menu ends with the
// universal '?' help key. Ev: "%s - [" @0x1d0f3 + keys + " ?]? " @0x1d0fa.
func (s *Session) menuPrompt(name, keys string) {
	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("%s - [%s ?]? ", name, keys))
	s.display.ResetColor()
}

// yesNo renders a TriBBS-style "question (y/N)? " prompt (default answer shown
// capitalized) and returns true only for an explicit yes. def sets the default
// applied on a bare Enter. Ev: "%s (%c/%c)? " @0x1d21d.
func (s *Session) yesNo(question string, def bool) bool {
	y, n := 'y', 'N'
	if def {
		y, n = 'Y', 'n'
	}
	s.display.SetColor('E')
	s.display.Write(fmt.Sprintf("%s (%c/%c)? ", question, y, n))
	s.display.ResetColor()
	line := strings.TrimSpace(strings.ToLower(s.readLine()))
	if line == "" {
		return def
	}
	return line[0] == 'y'
}

func (s *Session) readPassword() string {
	pw, err := s.display.ReadPassword()
	if err != nil {
		s.carrierLost()
		return ""
	}
	return pw
}

// disconnectPanic is thrown by carrierLost to unwind the (deeply nested) menu
// loops when the caller drops. Run recovers it.
type disconnectPanic struct{}

// carrierLost aborts the session when the telnet/SSH client disconnects. A read
// error means the carrier dropped; without this the swallowed "" read makes menu
// loops respin forever (100% CPU + leaked goroutine). Rather than guard every
// nested loop, we panic and let Run recover — the standard Go idiom for aborting
// deeply nested I/O. All deferred cleanup (UnregisterNode, cancel) still runs.
func (s *Session) carrierLost() {
	if !s.hangup {
		s.hangup = true
		s.storage.LogCaller("Carrier lost — caller disconnected")
		s.cancel()
		panic(disconnectPanic{})
	}
}

// disconnected reports whether the carrier has dropped (or an @HANGUP fired).
func (s *Session) disconnected() bool { return s.hangup }

func (s *Session) pause() {
	s.display.SetColor('B')
	s.display.Write("Press Enter to continue...")
	s.display.ResetColor()
	s.readLine()
}

// systemPasswordPrompt checks for a system password (PASSWORD.DAT).
// Returns true if access is granted or no password is set.
func (s *Session) systemPasswordPrompt() bool {
	// Check if system password is configured
	if s.config.SystemPassword == "" {
		return true
	}

	s.display.Clear()
	s.display.SetColor('A')
	s.display.WriteLine("╔══════════════════════════════════════════════════════════════╗")
	s.display.SetColor('E')
	s.display.WriteLine("║  System Password Required                                   ║")
	s.display.SetColor('A')
	s.display.WriteLine("╠══════════════════════════════════════════════════════════════╣")
	s.display.SetColor('F')
	s.display.WriteLine("║  Enter system password to continue.                         ║")
	s.display.SetColor('A')
	s.display.WriteLine("╚══════════════════════════════════════════════════════════════╝")
	s.display.ResetColor()

	s.display.SetColor('E')
	s.display.Write("Please enter the system password: ")
	s.display.ResetColor()
	password := s.readPassword()

	if password != s.config.SystemPassword {
		s.display.SetColor('C')
		s.display.WriteLine("Incorrect system password!")
		s.storage.LogCaller("Incorrect system password attempt")
		s.pause()
		return false
	}

	return true
}

// isBannedUsername checks if a username is in the joker file (banned list).
func (s *Session) isBannedUsername(name string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	for _, b := range s.jokerList() {
		if lowerName == b {
			return true
		}
	}
	return false
}

// jokerList returns the banned-username list ("joker file"). TriBBS kept these
// in JOKER.DAT; we load that file if present (one name per line, # comments
// allowed) and always include the built-in reserved names.
func (s *Session) jokerList() []string {
	banned := []string{"sysop", "admin", "root", "system", "bbs"}
	data, err := os.ReadFile("JOKER.DAT")
	if err != nil {
		return banned
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		banned = append(banned, strings.ToLower(line))
	}
	return banned
}
