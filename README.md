# Server Domme — Your Dominant Discord Assistant

> *“Discipline. Sass. Control.  
All in one bot, ready to whip your server into shape.”*  

![Discord](https://img.shields.io/badge/Discord-Bot-5865F2?logo=discord&logoColor=white) ![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white) ![GitHub Repo size](https://img.shields.io/github/repo-size/keshon/server-domme) ![License](https://img.shields.io/github/license/keshon/server-domme) [![GitHub stars](https://img.shields.io/github/stars/keshon/server-domme?style=social)](https://github.com/keshon/server-domme)

---

## Why Server Domme?

Server Domme is a self-hosted Discord server-management and roleplay bot
written in Go. She handles the housekeeping and leaves room for the sass:

- Scheduled channel cleanup and message purges.
- Guided member welcomes, announcements and anonymous confessions.
- Roleplay tasks, discipline commands and protected-user settings.
- Reaction-triggered translation, media sharing and short links.
- Per-server settings and command-group controls.

The roleplay voice belongs to the roleplay commands. Utility replies and
warnings for destructive actions stay direct.

---

## Available Commands

Use `/help` in Discord for the command list.

<details>
<summary>Command reference</summary>

### 🕯️ Information

- **/about** — Discover the origin of this bot
- **/help** — Get a list of available commands
  - **/help category** — View commands grouped by category
  - **/help group** — View commands grouped by group
  - **/help flat** — View all commands as a flat list

### 👋 Welcome

- **/welcome** — Introduce and welcome a member, the way this server does it
  - **/welcome access** — Manage who can welcome and preview. Leave empty to list allowed roles
  - **/welcome member** — Post their role's intro and welcome — checks everything first
  - **/welcome setup** — Where a role's intro and welcome go. Leave both empty to see
  - **/welcome template** — Write a role's intro or welcome text — paste it straight from Discord
  - **/welcome preview** — See a role's intro and welcome as they would be posted, without posting
  - **/welcome roles** — Every role with a welcome set up
  - **/welcome move** — Give a role's welcome to another role — a test setup to the real one
  - **/welcome remove** — Remove a role's welcome settings
  - **/welcome gif-add** — Add a gif for a role's welcomes, or to the shared pool without a role
  - **/welcome gif-remove** — Remove a gif from a role's welcomes, or from the shared pool without a role
  - **/welcome gifs** — The gifs welcomes pick from — every pool, or one role's

### 📢 Utilities

- **/announce** — Send a message on bot's behalf
- **/shortlink** — Shorten URLs and manage your links
  - **/shortlink create** — Shorten a URL
  - **/shortlink list** — List your shortened URLs
  - **/shortlink delete** — Delete a specific shortened URL
  - **/shortlink clear** — Clear all your shortened URLs
- **/translate** — Translate messages with a flag reaction

### 🎲 Gameplay

- **/roll** — Roll dice like `2d20+1d6-2`

### 🎭 Roleplay

- **/ask** — Ask for permission to contact another member
- **/confess** — Send an anonymous confession
- **/discipline** — Punish or release a brat
  - **/discipline punish** — Assign the brat role
  - **/discipline release** — Remove the brat role
- **/task** — Assign yourself a new random task

### 🎞️ Media

- **/media** — Post a random media file
- **/upload-media** — Upload one or multiple media files

### 🧹 Cleanup

- **/purge** — Manage message purges
  - **/purge auto** — Regularly purge old messages in this channel
  - **/purge now** — Schedule or perform an immediate purge
  - **/purge channel** — Allow or forbid purges in this channel — or see whether they are allowed, left empty
  - **/purge jobs** — List all active purge jobs and the channels purges are allowed in
  - **/purge stop** — Stop ongoing purge in this channel

### ⚙️ Settings

- **/settings** — Server settings
  - **/settings announce channel-set** — Set the announcement channel
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
  - **/settings translate channel-clear** — Remove all translation-enabled channels
  - **/settings commands log** — Review recently used commands
  - **/settings commands status** — Show enabled and disabled command groups
  - **/settings commands enable** — Enable a command group
  - **/settings commands disable** — Disable a command group

### 🛠️ Maintenance

- **/maintenance** — Bot maintenance commands
  - **/maintenance ping** — Check bot latency
  - **/maintenance export-data** — Export the current server database as JSON
  - **/maintenance status** — Retrieve guild statistics
  - **/maintenance sync** — Re-register slash commands


</details>

---

## Setup (Self-Hosting)

Use a binary from the [releases page](https://github.com/keshon/server-domme/releases)
or build from source with Go 1.26+. From the repository root:

```bash
cp .env.example .env
# Edit .env: set DISCORD_TOKEN and check TASKS_PATH.
go build -o server-domme-discord ./cmd/discord
./server-domme-discord
```

The example enables first-run command registration. Keep `data/` and `assets/`
beside a release binary and start from that directory. The
[running guide](docs/running.md) covers bot creation, intents, configuration,
first-run checks and persistent data. Container deployment is in the
[Docker guide](docker/README.md).

---

## Bot Permissions

Recommended permissions: View Channel, Send Messages, Embed Links, Attach Files, Read Message History, Manage Messages, Use Application Commands.

[Invite the bot](https://discord.com/oauth2/authorize?client_id=YOUR_APPLICATION_ID&scope=bot%20applications.commands&permissions=2147609600)
after replacing `YOUR_APPLICATION_ID` with your application's ID. The list and
permission mask are generated from the bot's permission definitions.

## Documentation

| Guide | Contents |
|---|---|
| [Running](docs/running.md) | Setup, configuration, feature settings, backups and migration |
| [Docker](docker/README.md) | Container build, mounted data and HTTP routing |
| [Architecture](docs/architecture.md) | Session lifecycle, dispatch, services and storage |
| [Conventions](docs/conventions.md) | Contributor rules, development commands and documentation pattern |

---

## Disclaimer

This bot contains **suggestive language**, **power dynamics**, and **dominant sass** not suitable for the faint-hearted or humorless. Use responsibly, and only with **consenting adults**.
