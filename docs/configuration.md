# Configuration Guide

## System Configuration

All configuration is stored in the SQLite database (`tresbbs.db`) and can be
edited via the sysop menu (`S` from main menu, then `7` for System Configuration).

### Core Settings

| Setting | Default | Description |
|---------|---------|-------------|
| Board Name | TriBBS | Name displayed in welcome screen |
| Sysop Name | Sysop | System operator name |
| System Password | (empty) | Password required to log in (blank = none) |
| Max Nodes | 4 | Maximum simultaneous users |
| New User Security | 10 | Security level for new registrations |
| Sysop Security | 90 | Minimum level for sysop functions |
| Max Time/Logon | 60 | Minutes per session |
| New User Time | 30 | Minutes for new users |
| Allow Aliases | Yes | Allow alias names |
| Default Protocol | Z | Default file transfer protocol |

### Baud Rate Gates

Configure which baud rates are allowed:

| Setting | Default | Description |
|---------|---------|-------------|
| Allow 300 | No | Allow 300 baud (very slow) |
| Allow 1200 | No | Allow 1200 baud |
| Allow 2400 | Yes | Allow 2400 baud |

### Security Levels

Security levels control access to features:

| Level | Access |
|-------|--------|
| 0-9 | Limited access |
| 10 | Normal user (default for new users) |
| 20-49 | Extended access |
| 50-79 | Co-sysop level |
| 80-89 | Near-sysop |
| 90+ | Full sysop access |

---

## Network Configuration

### Telnet (Default)

Edit `cmd/tresbbs-server/main.go`:

```go
addr = flag.String("addr", ":2323", "Telnet listen address")
```

### SSH

Generate host key:

```bash
ssh-keygen -t ed25519 -f ssh_host_key
```

SSH server listens on port 2222 by default.

### Firewall

Open the listening port:

```bash
# macOS
sudo pfctl -e -f /etc/pf.conf

# Linux
sudo ufw allow 2323/tcp
```

---

## Database Configuration

The SQLite database is created automatically at `tresbbs.db`.

### Backup

```bash
cp tresbbs.db tresbbs.db.backup
```

### Reset

```bash
rm tresbbs.db
# Database will be recreated on next start
```

---

## Template Configuration

Templates are stored in `templates/` and use the @VARIABLE system.

See [Template Customization Guide](template-guide.md) for details.

---

## Event Configuration

Events are configured via the sysop menu (Sysop Menu → Edit Events).

| Field | Format | Example |
|-------|--------|---------|
| Time | HH:MM | 02:00 |
| Day | Mon-Sun or Daily | Daily |
| File | Command to run | /usr/local/bin/cleanup.sh |
| Slide | Yes/No | Yes |

Sliding events run at the next opportunity after their scheduled time.
Non-sliding events run at exactly the specified time.
