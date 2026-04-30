# Server Domme — Your Dominant Discord Assistant

> *“Discipline. Sass. Music. Control.  
All in one bot, ready to whip your server into shape.”*  

![Discord](https://img.shields.io/badge/Discord-Bot-5865F2?logo=discord&logoColor=white) ![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) ![GitHub Repo size](https://img.shields.io/github/repo-size/keshon/server-domme) ![License](https://img.shields.io/github/license/keshon/server-domme) [![GitHub stars](https://img.shields.io/github/stars/keshon/server-domme?style=social)](https://github.com/keshon/server-domme)

---

## Why Server Domme?

**Server Domme combines several key features in one place**:

* 🎵 Plays music from YouTube and SoundCloud
* 🧹 Automates channel cleanup and message purges
* 🎭 Supports roleplay interactions, task management, and anonymous confessions
* ⚙️ Offers admin tools for command and server configuration
* 💬 Responds to mentions with context-aware interactions

It’s designed to be practical for server management while providing light, interactive roleplay features. The bot can be easily expanded with new commands due to its **modular architecture**. 

---

## Available Commands

### 🕯️ Information

- **/about** — Discover the origin of this bot
- **/help** — Get a list of available commands
  - **/help category** — View commands grouped by category
  - **/help group** — View commands grouped by group
  - **/help flat** — View all commands as a flat list

### 📢 Utilities

- **Announce (context command)** — Send a message on bot's behalf
- **/announce** — Send a message on bot's behalf
- **/shortlink** — Shorten URLs and manage your links
  - **/shortlink create** — Shorten a URL
  - **/shortlink list** — List your shortened URLs
  - **/shortlink delete** — Delete a specific shortened URL
  - **/shortlink clear** — Clear all your shortened URLs
- **translate (reaction)** — Translate message on flag emoji reaction

### 🎲 Gameplay

- **/roll** — Roll dices like `2d20+1d6-2`

### 🎭 Roleplay

- **/ask** — Ask for permission to contact another member
- **/confess** — Send an anonymous confession
- **/discipline** — Punish or release a brat
  - **/discipline punish** — Assign the brat role
  - **/discipline release** — Remove the brat role
- **/task** — Assign yourself a new random task

### 🎵 Music

- **/history** — Show recently played tracks (replay by id with /play)
- **/next** — Skip to the next track
- **/play** — Play a music track
- **/stop** — Stop playback and clear queue

### 🎞️ Media

- **/media** — Post a random media file
- **/upload-media** — Upload one or multiple media files

### 🧹 Cleanup

- **/purge** — Manage message purges
  - **/purge auto** — Regularly purge old messages in this channel
  - **/purge now** — Schedule or perform an immediate purge
  - **/purge jobs** — List all active purge jobs
  - **/purge stop** — Stop ongoing purge in this channel

### ⚙️ Settings

- **/commands** — Manage or inspect commands
  - **/commands log** — Review recent commands called by users
  - **/commands status** — Check which command groups are enabled or disabled
  - **/commands toggle** — Enable or disable a group of commands
  - **/commands update** — Re-register or update slash commands
- **/maintenance** — Bot maintenance commands
  - **/maintenance ping** — Check bot latency
  - **/maintenance download-db** — Download the current server database as a JSON file
  - **/maintenance status** — Retrieve statistics about the guild
- **/manage-announce** — Announcement settings
  - **/manage-announce set-channel** — Set or update the announcement channel
  - **/manage-announce reset-channel** — Reset and remove the current announcement channel
- **/manage-confess** — Confession settings
  - **/manage-confess set-channel** — Set the confession channel
  - **/manage-confess list-channel** — Show the currently configured confession channel
  - **/manage-confess reset-channel** — Remove the confession channel
- **/manage-discipline** — Discipline settings
  - **/manage-discipline set-roles** — Set or update discipline roles
  - **/manage-discipline list-roles** — List all configured discipline roles
  - **/manage-discipline reset-roles** — Reset all discipline role configurations
- **/manage-media** — Media settings
  - **/manage-media add-category** — Add a new media category
  - **/manage-media list-categories** — List all existing media categories
  - **/manage-media remove-category** — Remove a media category
  - **/manage-media set-default-category** — Set a default media category for this server
  - **/manage-media reset-default-category** — Reset the default media category to none
- **/manage-task** — Task settings
  - **/manage-task set-role** — Set or update a Tasker role
  - **/manage-task list-role** — List all task-related roles
  - **/manage-task reset-role** — Reset the Tasker role configuration
  - **/manage-task upload-tasks** — Upload a new task list for this server
  - **/manage-task download-tasks** — Download the current task list for this server
  - **/manage-task reset-tasks** — Reset the task list to default for this server
- **/manage-translate** — Translate settings
  - **/manage-translate set-channel** — Add a channel to the translate list
  - **/manage-translate reset-channel** — Remove a channel from the translate list
  - **/manage-translate list-channels** — List all channels enabled for translation reactions
  - **/manage-translate reset-all-channels** — Reset all channels for translation reactions


---

## Setup (Self-Hosting)

1. Clone this repository.
2. Add your bot token to the configuration file.
3. Define role IDs and setup your guild structure.
4. Build and run the bot:
   `go build && ./server-domme`
5. Invite her into your server. She’s waiting.

FFMPEG and YTDLP is required for music playback/streaming.

---

## Bot Permissions
- **Manage Roles**
- **View Channels**
- **Send Messages**
- **Manage Messages**
- **Embed Links**
- **Attach Files**
- **Read Message History**
- **Use Application Commands**
- **Connect**
- **Speak**

## Disclaimer

This bot contains **suggestive language**, **power dynamics**, and **dominant sass** not suitable for the faint-hearted or humorless. Use responsibly, and only with **consenting adults**.
