# Server Domme Discord Bot

Server Domme empowers moderators and organizers to assign tasks, manage roles, and keep channels tidy with ease. Whether you're fostering community engagement, organizing tasks, or maintaining order, this bot puts you firmly in control—with just the right amount of attitude.

---

## 💎 Features

### 🎭 Roleplay

* **`/ask`**
  Request permission to contact another member. Because politeness is protocol.

* **`/task`**
  Assign or manage your personal task. Comes with timers, reminders, and... consequences.
  *(Proof submission via buttons. Whimpering optional.)*

* **`/punish [user]`**
  Assigns the “brat” role with a snarky, randomly chosen reprimand. Only users with the `punisher` role may wield the paddle.

* **`/release [user]`**
  Removes the “brat” role and grants reprieve. Forgiveness... how boring.

---

### 🧹 Channel Cleanup

* **`/del-now`**
  Obliterates all messages in the current channel. Total devastation. No warnings.

* **`/del-auto [older_than]`**
  Enables recurring purges of messages older than a set duration (e.g., 2h, 1d, 1w). Set it and forget them.

* **`/del-stop`**
  Stops any active deletion jobs. Sometimes mercy is sexy.

* **`/del-jobs`**
  Lists all active deletion jobs. Because control means knowing everything.

---

### 🏰 Court Administration

* **`/set-role`**
  Assign bot roles: `punisher`, `victim`, `tasker`. No title, no power.

* **`/log`**
  View recent command usage—who served, who sinned.

* **`/dump-db`**
  Export the bot's full internal datastore. Secret scrolls for your eyes only.

* **`/dump-tasks`**
  Export all active tasks. Useful for audits, or just shaming the lazy.

* **`/init-commands`**
  Re-register all slash commands with Discord. It’s like snapping your fingers and realigning the universe.

---

### 🕯️ Lore & Insight

* **`/ping`**
  Pong. Yes, she’s listening.

* **`/help`**
  Show the full list of Server Domme commands—organized, brutal, delicious.

* **`/about`**
  Get info about the bot and the entity that birthed her.

---

## 🧷 Requirements

* **Server Roles Configured for:**

  * `punisher` – Users who can assign/release brat roles.
  * `assigned` – The “brat” role itself.
  * `victim` – Users eligible for punishment.
  * `tasker` – Who can take tasks designed for this role (e.g. gender based filter).

---

## 🛠 Setup

1. Clone this repository.
2. Add your bot token to the configuration file.
3. Define role IDs and setup your guild structure.
4. Build and run the bot:
   `go build && ./server-domme`
5. Invite her into your server. She’s waiting.

---

## ⚠️ Disclaimer

This bot contains **suggestive language**, **power dynamics**, and **dominant sass** not suitable for the faint-hearted or humorless. Use responsibly, and only with **consenting adults**.
