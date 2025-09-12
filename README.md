# Server Domme — Your Dominant Discord Assistant

> *“Discipline. Sass. Music. Control.  
All in one bot, ready to whip your server into shape.”*  

![Discord](https://img.shields.io/badge/Discord-Bot-5865F2?logo=discord&logoColor=white) ![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) ![GitHub Repo size](https://img.shields.io/github/repo-size/keshon/server-domme) ![License](https://img.shields.io/github/license/keshon/server-domme) [![GitHub stars](https://img.shields.io/github/stars/keshon/server-domme?style=social)](https://github.com/keshon/server-domme)

---

## ✨ Why Server Domme?

**Server Domme combines several key features in one place**:

* 🎵 Plays music from YouTube and SoundCloud
* 🧹 Automates channel cleanup and message purges
* 🎭 Supports roleplay interactions, task management, and anonymous confessions
* ⚙️ Offers admin tools for command and server configuration
* 💬 Responds to mentions with context-aware interactions

It’s designed to be practical for server management while providing light, interactive roleplay features. The bot can be easily expanded with new commands due to its **modular architecture**. 

---

## 📜 Available Commands

### 🕯️ Information

- **/about** — Discover the origin of this bot
- **/help** — Get a list of available commands

### 🎵 Music

- **/music-next** — Skip to the next track
- **/music-play** — Play music track
- **/music-stop** — Stop playback and clear queue

### 📢 Utilities

- **/Announce** — Send a message to the announcement channel
- **/translate (reaction)** — Translate message on flag emoji reaction

### 🎲 Gameplay

- **/roll** — Roll dices like `2d20+1d6-2`

### 🎭 Roleplay

- **/ask** — Request permission to contact another member
- **/confess** — Send an anonymous confession
- **/punish** — Assign the brat role
- **/release** — Remove the brat role
- **/task** — Assign or manage your personal task

### 💬 Chat

- **/chat** — Responds when bot is mentioned (just a test)

### 🧹 Cleanup

- **/purge-auto** — Purge messages regularly in this channel
- **/purge-jobs** — List all active purge jobs
- **/purge-now** — Purge messages in this channel
- **/purge-stop** — Halt ongoing purge in this channel

### ⚙️ Settings

- **/commands-log** — Review recent commands and their punishments
- **/commands-status** — Check which command is enabled or disabled
- **/commands-toggle** — Enable or disable a group of commands
- **/set-channels** — Setup special-purpose channels
- **/set-roles** — Setup special-purpose roles
- **/set-tasks** — Upload a new task list for this server

### 🛠️ Maintenance

- **/dump-db** — Dumps server database as JSON
- **/dump-tasks** — Dumps all tasks as JSON
- **/ping** — Check bot latency


---

## 🛠 Setup (Self-Hosting)

1. Clone this repository.
2. Add your bot token to the configuration file.
3. Define role IDs and setup your guild structure.
4. Build and run the bot:
   `go build && ./server-domme`
5. Invite her into your server. She’s waiting.

FFMPEG and YTDLP is required for music playback/streaming.

---

## ⚠️ Disclaimer

This bot contains **suggestive language**, **power dynamics**, and **dominant sass** not suitable for the faint-hearted or humorless. Use responsibly, and only with **consenting adults**.
