# Server Domme Discord Bot

A delightfully cruel Discord bot built for servers where discipline is desired, sass is inevitable, and control is everything.  
This bot enables Dom(me)s to assign punishments, issue tasks, and release brats from their roles—if they behave.

## Features

- **/punish [target]**  
  Assigns the "brat" role to the selected user with a randomly chosen, snarky message. The user must have the configured "punisher" role to issue punishments.

- **/release [target]**  
  Removes the "brat" role from a previously punished user. Only users with the "punisher" role can release the shame-clad.

- **/task**  
  Assigns a randomized task to the user. Tasks come with a timer, a reminder before expiration, and consequences for failure.  
  (Proof submission and interaction handled via buttons.)

## Requirements

- Guild roles set for:
  - `punisher` (who can punish and release)
  - `assigned` (the role brats are given)
  - `victim` (who can be punished)

## Setup

1. Clone the repo.
2. Configure your bot token.
3. Ensure your server roles are set properly in your storage backend.
4. Build and run the bot.
5. Invite the bot to your server.

## Disclaimer

This bot contains suggestive language, power dynamics, and sass not suited for the faint of heart or humorless. Use responsibly and with consenting adults.

---

Because sometimes, moderation should come with a whip crack and a smirk.
