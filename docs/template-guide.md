# Template Customization Guide

## Overview

tresbbs uses template files to render all screens. Templates are plain text
files with @VARIABLE placeholders that are replaced with actual values at
runtime. This allows complete customization of the BBS appearance without
recompiling.

Template files are located in the `templates/` directory.

---

## Template Syntax

### Variables

Variables are prefixed with `@` and replaced with their values:

```
Welcome to @BOARDNAME, @FIRST!
You have @TIMELEFT minutes remaining.
```

### Color Codes

Color codes use the `@X__` format:

| Code | Color | Use |
|------|-------|-----|
| @X0A | Light Green | Labels, headers, menus |
| @X0B | Light Cyan | Secondary text, borders |
| @X0C | Light Red | Warnings, errors |
| @X0D | Light Magenta | Special emphasis |
| @X0E | Yellow | Values, user input |
| @X0F | White | Default body text |
| @X70 | Reverse Video | Highlighted selections |

Example:
```
@X0AThis is green text@X0F and this is white.
```

### Control Sequences

| Sequence | Action |
|----------|--------|
| @CLS | Clear screen |
| @PAUSE | Wait for keypress |
| @BEEP | Sound bell |
| @HANGUP | Disconnect user |
| @MOREON | Enable page-by-page prompting |
| @MOREOFF | Disable page-by-page prompting |
| @BREAKON | Enable Ctrl-Break |
| @BREAKOFF | Disable Ctrl-Break |

---

## Available Variables

### User Variables

| Variable | Description |
|----------|-------------|
| @USER | User's full name |
| @ALIAS | User's alias |
| @FIRST | User's first name |
| @PHONE | Phone number |
| @CITY | City |
| @BIRTHDATE | Date of birth |
| @SECURITY | Security level |
| @CALLS | Total logins |
| @CALLSTODAY | Logins today |
| @DOWNLOADS | Total downloads |
| @UPLOADS | Total uploads |
| @KDOWNLOADED | Kilobytes downloaded |
| @KUPLOADED | Kilobytes uploaded |
| @MESSAGES | Total messages posted |
| @MESSAGESTODAY | Messages today |
| @FILERATIO | Upload/download ratio |
| @LASTDATEON | Last login date |
| @LASTTIMEON | Last login time |
| @REGISTRATIONNUMBER | Registration number |
| @SUBSCRIPTIONDATE | Subscription expiration |

### Session Variables

| Variable | Description |
|----------|-------------|
| @NODE | Current node number |
| @BAUDRATE | Connection speed |
| @TIMELEFT | Minutes remaining |
| @TIMEREMAININGFORDAY | Daily time remaining |
| @TIMETHISCALL | Minutes this session |

### System Variables

| Variable | Description |
|----------|-------------|
| @BOARDNAME | BBS name |
| @SYSOPNAME | Sysop name |
| @VERSIONNUMBER | tresbbs version |
| @SYSTEMDATE | Current date |
| @SYSTEMTIME | Current time |
| @SYSTEMCALLS | Total system logins |
| @SYSTEMCALLSTODAY | Today's logins |
| @TOTALUSERS | Registered users |
| @BBSSTARTDATE | BBS start date |

---

## Template Files

| File | Purpose |
|------|---------|
| welcome.tpl | Login/welcome screen |
| main.tpl | Main menu |
| messages.tpl | Message conference menu |
| files.tpl | File area menu |
| doors.tpl | Door menu |
| sysop.tpl | Sysop menu |
| user_config.tpl | User configuration |
| who_online.tpl | Who's online display |
| bulletins.tpl | Bulletin listing |
| chat.tpl | Chat interface |
| caller_log.tpl | Caller log display |

---

## Creating Custom Templates

1. Create a new `.tpl` file in `templates/`
2. Use @VARIABLE placeholders for dynamic content
3. Use @X__ codes for colors
4. Reference the template in the session code

### Example: Custom Welcome

```
@X0A╔══════════════════════════════════════════════════════════════╗
@X0A║                                                            ║
@X0A║  @X0EWelcome to @BOARDNAME@X0A                                 ║
@X0A║                                                            ║
@X0A║  @X0BYou are caller #@SYSTEMCALLSTODAY today!@X0A              ║
@X0A║  @X0BYou have @X0ETIMELEFT@X0B minutes remaining.@X0A             ║
@X0A║                                                            ║
@X0A╚══════════════════════════════════════════════════════════════╝
@X0F
```

---

## Tips

- Use box-drawing characters (╔═╗║╚═╝) for a classic BBS look
- Keep lines under 80 characters for compatibility
- Use color sparingly — too much is hard to read
- Test templates with different data lengths
- Backup templates before editing
