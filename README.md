# Spiralydata

Spiralydata is a real-time file synchronization software that allows multiple users to share and sync files across different machines over local network or Internet.

## Table of Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Host Mode](#host-mode)
  - [User Mode](#user-mode)
  - [Configuration Management](#configuration-management)
- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [Network Setup](#network-setup)
- [Project Structure](#project-structure)
- [Support](#support)

## Features

### Bidirectional Synchronization
- Files added, modified, or deleted are automatically synchronized between client and server
- Real-time change detection using filesystem watchers
- Periodic scanning to catch missed changes

### Multi-User Support
- Multiple clients can connect to the same host simultaneously
- Each client receives updates in real-time
- Isolated client sessions with shared data space

### Authentication System
- Each server generates a unique 6-digit ID
- Clients must provide the correct ID to connect
- Failed authentication attempts allow retry without restarting

### Robustness
- Periodic scanner to catch any missed file changes
- Handles simultaneous modifications and deletions
- Protection against synchronization loops
- Automatic retry mechanism for file operations

### Portable
- Single executable file can be placed anywhere
- Automatically creates synchronization folder next to executable
- Configuration file stored with executable
- No installation required

### Cross-Platform
- Works on Windows and Linux
- Developed in Go for fast and portable execution
- Consistent behavior across operating systems

## Requirements

- Go 1.16 or higher (for compilation only)
- Network connectivity between host and clients
- Port forwarding configured for Internet access (optional)

### Supported Operating Systems
- Windows 10/11
- Linux (Ubuntu 20.04+, Debian, Fedora, Arch, etc.)
- macOS (experimental)

## Installation

### From Source

1. Clone the repository:
```bash
git clone https://github.com/Spiralyfox/Spiralydata.git
cd Spiralydata/source_code
```

2. Install dependencies:
```bash
go mod download
```

3. Compile the project:

**On Windows:**
```batch
setup_windows.bat
```

**On Linux/Mac:**
```bash
chmod +x setup_linux.sh
./setup_linux.sh
```

This will generate:
- `spiralydata.exe` (Windows executable)
- `spiralydata` (Linux/Mac executable)

### From Pre-compiled Binaries

Download the latest release from the [Releases](https://github.com/Spiralyfox/Spiralydata/releases) page and extract the executable for your platform.

## Quick Start

### As Host (Server)

1. Run the executable
2. Select "Host" mode
3. Enter a port number (e.g., 1234)
4. Create and remember a 6-digit ID
5. Share this ID with users who want to connect

### As User (Client)

1. Run the executable
2. Select "User" mode
3. Enter server address (IP:PORT)
4. Enter the 6-digit host ID
5. Synchronization starts automatically

## Usage

### Host Mode

The host mode runs a WebSocket server that clients connect to. All files in the `Spiralydata` folder are synchronized with connected clients.

**Starting a host:**
```
1. Run spiralydata executable
2. Choose option 2 (Host)
3. Enter port (example: 1234)
4. Enter 6-digit ID (example: 123456)
```

**Server output:**
```
Server started
ID: 123456
Folder: /path/to/Spiralydata
Waiting for connections...

Type 'x' then Enter to stop the server
```

**When a client connects:**
```
Client_1 connected (ID verified)
Connected clients: 1
Complete structure sent
```

### User Mode

The user mode connects to a host server and synchronizes the local `Spiralydata` folder.

**Connecting to a host:**
```
1. Run spiralydata executable
2. Choose option 1 (User)
3. Enter server address (example: 192.168.1.100:1234)
4. Enter host ID (example: 123456)
```

**Client output:**
```
Attempting connection to 192.168.1.100:1234...
Connection established - Synchronization in progress...
Connected to host 123456
Server: 192.168.1.100:1234

Type 'x' then Enter to disconnect
```

### Configuration Management

Spiralydata supports saving connection configurations for quick reconnection.

**Configuration menu:**
```
1. Load existing configuration
2. Create new configuration
3. Change host ID in configuration
4. Delete configuration
5. Connect without configuration
```

**Configuration file location:**
The `spiraly_config.json` file is created in the same directory as the executable.

**Configuration format:**
```json
{
  "server_addr": "192.168.1.100:1234",
  "host_id": "123456"
}
```

## Architecture

### System Overview

```
Host (Server)                    Client (User)
+------------------+            +------------------+
|  spiralydata     |            |  spiralydata     |
|                  |            |                  |
|  WebSocket       |<---------->|  WebSocket       |
|  Server          |            |  Client          |
|                  |            |                  |
|  File Watcher    |            |  File Watcher    |
|  Scanner         |            |  Scanner         |
|                  |            |                  |
|  /Spiralydata/   |            |  /Spiralydata/   |
+------------------+            +------------------+
```

### Components

**Server (server.go):**
- WebSocket server for client connections
- File system watcher for real-time changes
- Periodic scanner for missed changes
- Client authentication and management
- Broadcast system for multi-client updates

**Client (client.go):**
- WebSocket client for server connection
- File system watcher for local changes
- Periodic scanner for missed changes
- Authentication handler
- Configuration management

**Common:**
- `types.go`: Data structures (FileChange, AuthRequest, AuthResponse)
- `utils.go`: Helper functions (file retry, executable directory)
- `config.go`: Configuration file management
- `main.go`: Entry point and menu system

## How It Works

### File Synchronization Process

1. **Initial Sync:**
   - When a client connects, the server sends the complete directory structure
   - Client receives all files and folders
   - Both systems are now in sync

2. **Real-time Changes:**
   - File system watchers detect changes immediately
   - Changes are encoded and sent via WebSocket
   - Receiving side applies the change locally
   - Change is marked to skip next scan (prevents loops)

3. **Periodic Scanning:**
   - Every 2 seconds, both client and server scan their folders
   - New, modified, or deleted files are detected
   - Missing changes are synchronized
   - Ensures consistency even if watcher misses events

### Change Types

- **create**: New file added
- **write**: Existing file modified
- **remove**: File or folder deleted
- **mkdir**: New folder created

### Conflict Resolution

- Last write wins: Most recent change takes precedence
- No version control: Previous versions are not saved
- No merge strategy: Simultaneous edits result in last-write-wins

## Network Setup

### Local Network (LAN)

Clients and host must be on the same network or have direct network access.

**Example:**
- Host IP: 192.168.1.100
- Host Port: 1234
- Client connects to: 192.168.1.100:1234

### Internet (WAN)

For Internet access, port forwarding must be configured on the host's router.

**Steps:**
1. Find your public IP address (whatismyip.com)
2. Configure port forwarding on your router:
   - External port: 1234 (or chosen port)
   - Internal IP: Host machine IP
   - Internal port: 1234 (same as chosen port)
   - Protocol: TCP
3. Share your public IP and port with clients

**Example:**
- Public IP: 203.0.113.45
- Port: 1234
- Client connects to: 203.0.113.45:1234

**Security Note:** Only share your host ID with trusted users. Anyone with the ID can access your synchronized files.

## Project Structure

```
Spiralydata/
├── source_code/
│   ├── main.go              # Entry point and interactive menu
│   ├── server.go            # Server (host) implementation
│   ├── client.go            # Client (user) implementation
│   ├── types.go             # Data structures and types
│   ├── utils.go             # Utility functions
│   ├── config.go            # Configuration management
│   ├── setup_windows.bat    # Windows compilation script
│   ├── setup_linux.sh       # Linux compilation script
│   ├── go.mod               # Go module definition
│   └── .gitignore           # Git ignore rules
├── README.md                # This file
└── LICENSE                  # License information
```

### File Descriptions

- **main.go**: Interactive menu system, mode selection (host/user)
- **server.go**: WebSocket server, client management, file broadcasting
- **client.go**: WebSocket client, server connection, local synchronization
- **types.go**: FileChange, AuthRequest, AuthResponse structures
- **utils.go**: File retry logic, executable directory detection
- **config.go**: JSON configuration save/load/delete functions

## Support

For questions, issues, or feature requests:

- **GitHub Issues:** [https://github.com/Spiralyfox/Spiralydata/issues](https://github.com/Spiralyfox/Spiralydata/issues)
- **Email:** dauriacmatteo@gmail.com
- **Author:** Spiralyfox

---

Thank you for using Spiralydata!