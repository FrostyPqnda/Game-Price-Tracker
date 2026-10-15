# Privacy Policy

**Effective date:** October 15, 2026

This policy explains what data the Steam Price Tracker Discord bot ("the bot") collects, how it is used, and how you can have it removed. The bot is an independent, unofficial project and is not affiliated with Valve Corporation or Steam.

## What the bot stores

When you use `/track`, the bot saves a "watch" containing:

- Your Discord user ID
- The ID of the Discord channel where you ran the command
- The Steam app ID of the game you are tracking
- How often you want it checked (for example, every 2 days)
- The last price you were notified about
- When the watch was created and when it will next be checked

The bot also stores public game data from Steam, such as the title, store URL, current price, discount and price history. This data is not tied to any user.

## What the bot does not collect

- The content of your messages. The bot does not read messages and does not request the message content intent. It only responds to its slash commands.
- Your username, avatar, email address, server list, or any other profile information.
- Payment information or Steam account details.

## How your data is used

Your data is used only to run the service: to check prices on the schedule you chose, to send you price-drop alerts, and to show you your own watchlist. It is not sold, shared with third parties, used for advertising, or used to build profiles.

## Who can see your data

- **Other users:** `/watchlist` and `/untrack` only ever show or change your own watches. Other users cannot see what you track through the bot.
- **Price alerts:** alerts are posted in the channel where you ran `/track` and mention you. Anyone who can read that channel can see the alert, including the game name. Run `/track` in a channel you are comfortable with.
- **The operator:** the person running the bot has access to the database where watches are stored.
- **Discord and Steam:** the bot runs on Discord, so Discord's own privacy policy applies to your use of Discord. Price data is requested from Steam's public store, and your Discord information is not sent to Steam.

## Logs

The bot keeps log files for troubleshooting. Logs may include Discord user IDs, app IDs, and error messages. Log files are rotated and automatically deleted after 14 days.

## Where data is stored and how long

Data is stored in a MongoDB database controlled by the bot's operator. A watch is kept until you remove it with `/untrack` or ask for it to be deleted. Public game data may be kept indefinitely.

## Deleting your data

- Use `/untrack <id>` to remove a watch.
- To have all of your data deleted, contact the operator (see below) and include your Discord user ID.

Removing the bot from your server stops alerts to that server's channels, but it does not by itself delete stored watches. Use `/untrack` or contact the operator.

## Security

Credentials are kept in environment variables and not in the code repository. No system is perfectly secure, so please don't treat the bot as a place to store sensitive information.

## Children

Discord requires users to meet its minimum age requirements. The bot does not knowingly collect data from anyone under that age.

## Changes to this policy

This policy may be updated from time to time. The effective date above shows the latest version, and changes are recorded in the project's Git history.

## Contact

For questions or deletion requests, open an issue at <https://github.com/FrostyPqnda/Steam-Price-Tracker/issues> or contact the operator on Discord at `YOUR_DISCORD_HANDLE`.