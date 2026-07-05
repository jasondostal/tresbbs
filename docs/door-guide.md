# Door Installation Guide

## What Are Doors?

Doors are external programs that run alongside the BBS — games, utilities,
online encyclopedias, etc. The term "door" comes from the concept of
"opening a door" to another program.

The original TriBBS supported doors via DOOR.SYS and DORINFO1.DEF drop
files — standard formats that most door programs understood.

---

## How Doors Work

1. User selects a door from the Door menu
2. tresbbs generates drop files (DOOR.SYS, DORINFO1.DEF)
3. The door program is executed as a subprocess
4. The door reads the drop files to learn about the user
5. The door runs (game, utility, etc.)
6. When the door exits, tresbbs re-reads drop files for changes
7. User returns to the BBS

---

## Drop Files

### DOOR.SYS

Standard drop file format. Fields, one per line:

```
COM1              # COM port (0 for local)
0                 # Baud rate (0 for local)
8                 # Data bits
1                 # Stop bits
N                 # Parity
1                 # Node number
Y                 # Screen display
N                 # Printer
Y                 # Page bell
Y                 # Caller alarm
John Doe          # User name
johndoe           # User alias
New York          # City
password123       # Password
10                # Security level
42                # Total calls
5                 # Files uploaded
12                # Files downloaded
1024              # KB uploaded
2048              # KB downloaded
45                # Minutes left
01/04/26          # Date
01:23             # Time
```

### DORINFO1.DEF

Alternative format used by some doors:

```
My BBS            # BBS name
John              # Sysop first name
Doe               # Sysop last name
COM1              # COM port
0,N,8,1           # Baud,N,8,1
1                 # Node
John              # User first name
Doe               # User last name
johndoe           # User alias
10                # Security level
45                # Minutes left
1                 # ANSI (1=yes, 0=no)
80                # Screen width
1                 # Expert mode (1=yes)
```

---

## Installing Doors

### 1. Add Door to Database

Via sysop menu (Sysop Menu → Edit Doors → Add):

- **Name**: Display name
- **Hot Key**: Single character key
- **Description**: Brief description
- **Command**: Shell command to execute
- **Security**: Minimum security level
- **Time Limit**: Maximum minutes

### 2. Configure Door

Doors need to know where to find drop files. tresbbs writes drop files
to a temporary directory. Configure your door to read from:

```
/tmp/tresbbs/
```

### 3. Test Door

1. Log in as a user with sufficient security
2. Go to Door menu
3. Select the door
4. Verify it launches and exits cleanly

---

## Example: Simple Door

```bash
# In sysop menu, add a door:
# Name: Fortune
# Hot Key: F
# Command: fortune
# Security: 10
# Time Limit: 1
```

This door just runs the `fortune` command and displays a random quote.

---

## DOS Doors via DOSBox

For old DOS door games, you'll need DOSBox:

### 1. Install DOSBox

```bash
# macOS
brew install dosbox

# Linux
sudo apt install dosbox
```

### 2. Configure Door

```bash
# Command:
dosbox -c "cd C:\DOORS\GAMENAME" -c "GAMENAME.EXE" -c "exit"
```

### 3. Set Up DOS Environment

Create a DOSBox configuration that mounts the door directory.

---

## Troubleshooting

### Door doesn't launch

- Check the command path
- Verify execute permissions
- Check door logs in caller log

### Door can't find user info

- Verify drop files are being generated
- Check drop file path
- Ensure door reads correct format (DOOR.SYS vs DORINFO1.DEF)

### Door crashes

- Check door compatibility
- Try running door manually
- Check system resources

### User returned to wrong menu

- Check door exit code
- Verify session state is preserved

---

## Security Considerations

- Doors run as the tresbbs user
- Set appropriate file permissions
- Don't give doors network access unless needed
- Monitor door activity in caller log
- Set reasonable time limits
