# User Manual

## Welcome to tresbbs!

tresbbs is a classic Bulletin Board System (BBS) — a place to connect,
share messages, download files, and play games. Think of it as a
private social network with a retro vibe.

---

## Getting Started

### Connecting

Use a telnet client to connect:

```bash
telnet bbs.example.com 2323
```

Or for encrypted connections:

```bash
ssh bbs.example.com -p 2222
```

### Registering

1. Connect to the BBS
2. Enter your name when prompted
3. Type `y` for new user
4. Choose an alias (screen name)
5. Enter your real name
6. Enter your city
7. Enter your phone number (optional)
8. Choose a password
9. You're in!

### Logging In

1. Connect to the BBS
2. Enter your name or alias
3. Enter your password
4. Welcome back!

---

## Main Menu

```
╔══════════════════════════════════════════════════════════════╗
║  Welcome, YourAlias                                        ║
╠══════════════════════════════════════════════════════════════╣
║   [M] Messages        [F] Files         [D] Doors          ║
║   [C] Chat            [W] Who's Online  [U] User Config    ║
║   [B] Bulletins       [G] Goodbye       [S] Sysop Menu     ║
╚══════════════════════════════════════════════════════════════╝
```

Press the letter in brackets to select an option.

---

## Messages [M]

Read and post messages in different conferences.

### Reading Messages

1. Press `R` to read messages
2. Messages are listed with number, author, and subject
3. Enter a message number to read it
4. Press `0` to go back

### Posting Messages

1. Press `P` to post
2. Enter the conference name
3. Enter your subject
4. Type your message
5. Press Enter on an empty line to finish

### Message Conferences

Different areas for different topics:
- **General** — Random chat
- **Help** — Ask questions
- **Feedback** — Suggestions for the BBS

---

## Files [F]

Download and upload files.

### Browsing Files

1. Press `L` to list files
2. Enter the area name
3. Files are listed with name, size, and description
4. Enter a file number to download

### Uploading Files

1. Press `U` to upload
2. Select your protocol
3. Send the file with your terminal client

### Protocols

| Protocol | Speed | Reliability |
|----------|-------|-------------|
| Xmodem | Slow | Good |
| Xmodem-1K | Medium | Good |
| Ymodem | Fast | Good |
| Zmodem | Fast | Best (resume support) |

---

## Doors [D]

Play games and run utilities.

### Available Doors

The sysop configures which doors are available. Common doors:
- **Legend of the Red Dragon** — Classic RPG
- **TradeWars 2002** — Space trading game
- **Barren Realms Elite** — Strategy game

### Playing Doors

1. Press `D` for doors
2. Select a door by its key
3. The door launches
4. Play the game
5. Exit the game to return to BBS

---

## Chat [C]

Talk to other users in real-time.

### Paging for Chat

1. Press `C` for chat
2. Press `P` to page a user
3. Enter their node number
4. Wait for response

### Who's Online

1. Press `W` from main menu
2. See who's connected and what they're doing
3. Node numbers are shown

---

## User Config [U]

Change your settings.

### Available Options

- **Change Password** — Update your password
- **Change Editor** — Full screen or line editor
- **Change Protocol** — Default file transfer method

---

## Bulletins [B]

Read announcements from the sysop.

1. Press `B` for bulletins
2. Select a bulletin to read
3. Press `X` to exit

---

## Logging Off [G]

Press `G` to log off gracefully. This:
- Saves your session stats
- Logs your logout
- Returns the phone line

---

## Tips

### Keyboard Shortcuts

- **Enter** — Confirm selection
- **Escape** — Cancel/back
- **Ctrl+C** — Cancel current operation

### Time Limits

Your session has a time limit (usually 60 minutes). Watch your
remaining time in the status bar.

### File Ratios

You may need to upload files before downloading. The ratio is
set by the sysop (e.g., 1:3 means upload 1 for every 3 downloads).

### Security Levels

Higher levels unlock more features. Be active and helpful to
earn higher security from the sysop.

---

## Etiquette

- Be respectful to other users
- Don't spam messages
- Don't hog the phone line
- Upload quality files
- Report problems to the sysop

---

## Troubleshooting

### Can't Connect

- Check the address and port
- Verify telnet client is installed
- Check your internet connection

### Slow Transfers

- Use Zmodem protocol
- Check your modem speed
- Try at off-peak hours

### Lost Connection

- Reconnect and log in again
- Your session will be restored
- Check carrier detect

### Password Problems

- Contact the sysop
- Passwords are case-sensitive
- Don't share your password

---

## Glossary

| Term | Definition |
|------|------------|
| **BBS** | Bulletin Board System |
| **Sysop** | System Operator (admin) |
| **Node** | A connection point |
| **Door** | External program |
| **Protocol** | File transfer method |
| **Carrier** | Modem connection signal |
| **Upload** | Send file to BBS |
| **Download** | Receive file from BBS |
| **Conference** | Message area |
| **Bulletin** | Announcement |

---

## Support

For help, contact the sysop via:
- Message to Sysop (from message menu)
- Chat request
- Email (if configured)

Welcome to the BBS community!
