# Discord Vault 🛡️

**Discord Vault** is a high-security, decentralized file storage solution that leverages Discord's infrastructure as an encrypted backend. It provides a seamless experience for storing, managing, and retrieving files of any size through a modern web dashboard and a feature-rich Discord bot.

---

## 💎 Features

- **🔐 End-to-End Encryption**: Every file chunk is encrypted using **AES-256-GCM** before leaving your machine. Your `ENCRYPTION_KEY` never touches Discord.
- **📦 Distributed Storage**: Automatically splits large files (up to 1GB+) into smaller, manageable chunks (7MB) to comply with Discord limits.
- **🖥️ Premium Web Dashboard**: A state-of-the-art **Glassmorphism** interface featuring:
  - Real-time upload velocity & progress tracking.
  - Estimated Time of Arrival (ETA) for large transfers.
  - Interactive System Logs for real-time monitoring.
  - Secure asset management (Download / Wipe).
- **🤖 Intelligent Discord Bot**:
  - **Slash Commands**: `/upload`, `/list`, `/delete`, `/help`.
  - **Live Notifications**: Immediate feedback on both Web and Bot uploads.
  - **Security**: Granular access control via `ALLOWED_USERS`.
- **⚡ High Performance**: 
  - **Parallel Purging**: Multi-threaded deletion for instant vault clearing.
  - **Optimized Streaming**: Chunks are streamed and decrypted on the fly for maximum speed.

---

## 🛠️ Tech Stack
- **Backend**: Go (Golang)
- **Frontend**: Vanilla JS, HTML5, CSS3 (Glassmorphism)
- **Database**: SQLite (CGO-free)
- **Encryption**: AES-256-GCM
- **API**: DiscordGo

---

## 🚀 Quick Start

### 1. Prerequisites
- **Go 1.21+** installed.
- A **Discord Bot Token** and a dedicated **Storage Channel ID**.
- A 32-character encryption key.

### 2. Installation
```bash
git clone https://github.com/ZoniBoy00/DiscordVault.git
cd DiscordVault
go mod tidy
```

### 3. Configuration
Copy `.env.example` to `.env` and fill in your credentials:
```env
DISCORD_TOKEN=your_token_here
DISCORD_CHANNEL_ID=your_channel_id_here
ENCRYPTION_KEY=v8y/B?E(G+KbPeShVmYq3t6w9z$C&F)JG1  # Must be exactly 32 chars
ALLOWED_USERS=123456789,987654321                 # Optional
```

### 4. Run
```bash
go run main.go
```
Visit `http://localhost:8080` to access the command center.

---

## 🎮 Bot Commands
- `/upload`: Secure a file directly via Discord (up to 25MB).
- `/list`: Overview of all encrypted assets in the vault.
- `/delete [id]`: Permanently wipe an asset and all its chunks from Discord.
- `/help`: Detailed operational manual.

---

## 🔒 Security Architecture
1. **Packetization**: Files are read in 7MB buffers.
2. **Encryption**: Each buffer is encrypted with a unique nonce using AES-GCM.
3. **Obfuscation**: Encrypted chunks are sent to Discord with randomized hex names and a `.vault` extension.
4. **Reconstruction**: During download, chunks are fetched in order, decrypted, and streamed back as the original file.

---

## 📜 License
Licensed under the MIT License. See `LICENSE` for more information.
