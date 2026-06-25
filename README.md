# Server Domme — Your Dominant Discord Assistant

> *“Discipline. Sass. Control.  
All in one bot, ready to whip your server into shape.”*  

![Discord](https://img.shields.io/badge/Discord-Bot-5865F2?logo=discord&logoColor=white) ![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) ![GitHub Repo size](https://img.shields.io/github/repo-size/keshon/server-domme) ![License](https://img.shields.io/github/license/keshon/server-domme) [![GitHub stars](https://img.shields.io/github/stars/keshon/server-domme?style=social)](https://github.com/keshon/server-domme)

---

## Why Server Domme?

**Server Domme combines several key features in one place**:

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
  - **`/help category** — View commands grouped by category
  - **/help group** — View commands grouped by group
  - **/help flat** — View all commands as a flat list

### 📢 Utilities

- **Announce (context command)** — Send a message on bot's behalf
- **/announce** — Send a message on bot's behalf
- **/shortlink** — Shorten URLs and manage your links
  - **`/shortlink create** — Shorten a URL
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
  - **`/discipline punish** — Assign the brat role
  - **/discipline release** — Remove the brat role
- **/task** — Assign yourself a new random task

### 🎞️ Media

- **/media** — Post a random media file
- **/upload-media** — Upload one or multiple media files

### 🧹 Cleanup

- **/purge** — Manage message purges
  - **`/purge auto** — Regularly purge old messages in this channel
  - **/purge now** — Schedule or perform an immediate purge
  - **/purge jobs** — List all active purge jobs
  - **/purge stop** — Stop ongoing purge in this channel

### ⚙️ Settings

- **/settings** — Server settings
  - **`/settings announce channel-set** — Set the announcement channel
  - **/settings announce channel-show** — Show the current announcement channel
  - **/settings announce channel-reset** — Remove the announcement channel
  - **/settings confess channel-set** — Set the confession channel
  - **/settings confess channel-show** — Show the current confession channel
  - **/settings confess channel-reset** — Remove the confession channel
  - **/settings discipline roles-set** — Configure discipline roles
  - **/settings discipline roles-show** — Show configured discipline roles
  - **/settings discipline roles-reset** — Reset discipline role configuration
  - **/settings media category-add** — Add a media category
  - **/settings media category-list** — List media categories
  - **/settings media category-remove** — Remove a media category
  - **/settings media default-set** — Set the default media category
  - **/settings media default-show** — Show the default media category
  - **/settings media default-reset** — Clear the default media category
  - **/settings task role-set** — Configure the Tasker role
  - **/settings task role-show** — Show the configured Tasker role
  - **/settings task role-reset** — Reset the Tasker role configuration
  - **/settings task tasks-upload** — Upload a task list
  - **/settings task tasks-download** — Download the current task list
  - **/settings task tasks-reset** — Reset tasks to defaults
  - **/settings task cooldown-set** — Set task cooldown duration
  - **/settings task cooldown-show** — Show cooldown settings and active cooldowns
  - **/settings translate channel-add** — Enable translation reactions in a channel
  - **/settings translate channel-remove** — Disable translation reactions in a channel
  - **/settings translate channel-list** — List translation-enabled channels
  - **/settings translate channels-clear** — Remove all translation-enabled channels
  - **/settings commands log** — Review recently used commands
  - **/settings commands status** — Show enabled and disabled command groups
  - **/settings commands enable** — Enable a command group
  - **/settings commands disable** — Disable a command group

### 🛠️ Maintenance

- **/maintenance** — Bot maintenance commands
  - **`/maintenance ping** — Check bot latency
  - **/maintenance export-data** — Export the current server database as JSON
  - **/maintenance status** — Retrieve guild statistics
  - **/maintenance sync** — Re-register slash commands


---

## Setup (Self-Hosting)

1. Clone this repository.
2. Add your bot token to the configuration file.
3. Define role IDs and setup your guild structure.
4. Build and run the bot:
   `go build && ./server-domme`
5. Invite her into your server. She’s waiting.

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

## Disclaimer

This bot contains **suggestive language**, **power dynamics**, and **dominant sass** not suitable for the faint-hearted or humorless. Use responsibly, and only with **consenting adults**.
