# Sysop Manual

## Getting Started

### First-Time Setup

1. **Start tresbbs**:
   ```bash
   go build -o tresbbs-server ./cmd/tresbbs-server
   ./tresbbs-server -addr :2323
   ```

2. **Register first user**:
   - Connect: `telnet localhost 2323`
   - Register with your name
   - This user will have default security (10)

3. **Grant sysop access**:
   - Connect again and log in
   - Press `S` for Sysop Menu (will fail - need higher security)
   - Edit database directly to set security to 90:
     ```bash
     sqlite3 tresbbs.db "UPDATE users SET security_level=90 WHERE record_number=1;"
     ```

4. **Configure system**:
   - Log in again
   - Sysop Menu → System Configuration
   - Set board name, your name, etc.

---

## Sysop Menu

Access: Press `S` from main menu (requires security 90+)

### 1. Edit Users

- **List users**: View all registered users
- **Edit user**: Modify any user field
- **Kill user**: Mark for deletion
- **Lock out**: Prevent user from logging in

### 2. Edit Conferences

- **List conferences**: View all message areas
- **Add conference**: Create new area
- **Edit conference**: Change settings

### 3. Edit File Areas

- **List areas**: View all file areas
- **Add area**: Create new area
- **Edit area**: Change paths, security

### 4. Edit Doors

- **List doors**: View all doors
- **Add door**: Configure new door
- **Edit door**: Change command, security

### 5. View Caller Log

View last 50 log entries showing:
- Logins/logouts
- Message posts
- File transfers
- Door executions
- Security events

### 6. Pack User File

Remove deleted user records permanently.
**Warning**: Cannot be undone!

### 7. System Configuration

Edit core BBS settings:
- Board name
- Sysop name
- Security levels
- Time limits
- Baud gates

### 8. Edit Bulletins

Manage announcement bulletins shown to users.

### 9. Edit Events

Configure scheduled tasks:
- Time and day
- Command to execute
- Sliding/fixed execution

---

## Daily Operations

### Monitor Activity

- Check caller log regularly
- Watch for failed login attempts
- Monitor disk usage

### Backup

```bash
# Backup database
cp tresbbs.db backups/tresbbs_$(date +%Y%m%d).db

# Backup messages
tar czf messages_backup.tar.gz messages/
```

### Maintenance

- Pack user file periodically
- Clean old log entries
- Update bulletins
- Check event execution

---

## Security

### Password Policy

- Passwords are bcrypt-hashed
- Encourage strong passwords
- Change system password regularly

### Access Control

- Use security levels wisely
- Lock out abusive users
- Monitor failed logins

### Network Security

- Use SSH instead of telnet for internet
- Set up firewall rules
- Monitor connection attempts

---

## Troubleshooting

### Can't log in as sysop

- Check security level in database
- Verify password is correct
- Check if account is locked

### Server won't start

- Check if port is in use
- Verify database permissions
- Check system resources

### Doors don't work

- Verify door command is correct
- Check file permissions
- Test door manually

### Messages not saving

- Check database disk space
- Verify SQLite integrity
- Check file permissions

---

## Advanced Configuration

### Multiple Nodes

tresbbs supports multiple simultaneous users:

```bash
# In System Configuration
Max Nodes: 4
```

Each connection gets a node number. Users can see who's online
and chat between nodes.

### Custom Templates

Edit files in `templates/` to customize appearance:

- `welcome.tpl` — Login screen
- `main.tpl` — Main menu
- `messages.tpl` — Message areas

See [Template Guide](template-guide.md) for details.

### Event Automation

Configure events for automated tasks:

- Nightly cleanup
- Daily statistics
- Weekly reports
- Automated backups

### SSH Setup

For encrypted access:

```bash
# Generate host key
ssh-keygen -t ed25519 -f ssh_host_key

# Start with SSH enabled
./tresbbs-server -addr :2323 -ssh :2222
```

---

## Reference

### File Locations

| File | Purpose |
|------|---------|
| tresbbs.db | SQLite database |
| templates/ | Template files |
| docs/ | Documentation |

### Default Ports

| Port | Service |
|------|---------|
| 2323 | Telnet |
| 2222 | SSH |

### Security Levels

| Level | Access |
|-------|--------|
| 0-9 | Limited |
| 10 | Normal user |
| 20-49 | Extended |
| 50-79 | Co-sysop |
| 80-89 | Near-sysop |
| 90+ | Full sysop |
