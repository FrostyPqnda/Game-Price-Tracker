# GameDip

A Discord bot that tracks Steam game prices. Tell it which games to watch and how often to check, and it pings you when a price dips. Prices and watches are stored in MongoDB.

> GameDip is an independent, unofficial Steam price tracker and is not affiliated with or endorsed by Valve Corporation.

**[Add GameDip to your server](https://discord.com/oauth2/authorize?client_id=1558910291084050553)**

## Features

- Track any Steam game by its app ID with `/track`
- Choose how often each game is checked (every X minutes, hours, days, weeks or months)
- Get a Discord mention when a tracked game's price drops
- Keep a price history per game
- View and manage your tracked games with `/watchlist` and `/untrack`

## Scope
- The Discord bot does one thing: it tracks the games you ask it to and pings you on price drops. It doesn't moderate, play music, respond to chat messages, or read message content.
- Game data (title, URL, prices) is shared and public. Watches (who tracks what) are stored per user and only ever shown to the user who created them.

## Discord Commands

| Command | Options | Description |
|---------|---------|-------------|
| `/track` | `id`, `every`, `unit` | Track a game and check its price on a schedule |
| `/untrack` | `id` | Stop tracking a game |
| `/watchlist` | `page` (optional) | Show the games you are tracking, five per page |

### `/track <id> <every> <unit>`

Tracks a game and checks its price every `<every>` `<unit>`.

- `id`: the Steam app ID (the number in the store URL, e.g. `store.steampowered.com/app/`**`4078430`**)
- `every`: how many units to wait between checks (minimum 1)
- `unit`: `minutes`, `hours`, `days`, `weeks` or `months`

Examples:

```
/track id:4078430 every:12 unit:hours
/track id:4078430 every:1 unit:weeks
```

Notes:

- When you run `/track`, the bot fetches the game's current price as your starting point.
- Running `/track` again for the same game changes its interval and the channel alerts are posted to. It does not reset your price baseline.
- Alerts are only sent when the price drops below the last price you were notified about.
- Intervals in minutes have a minimum (30 by default) to be considerate of Steam's servers.
- There is 25 limit cap on the no. of games you can track.

### `/untrack <id>`

Stops tracking the game. Only your own watch is removed. The game's stored price data is kept.

### `/watchlist [page]`

Shows the games you are tracking, five per page, with each game's title, current price, discount and check interval. `page` defaults to 1.

## Prerequisites

- Go 1.27.3 (or the version in `go.mod`)
- A MongoDB instance (local install or a hosted cluster such as MongoDB Atlas)
- A Discord application with a bot (see below)

## How It Works

1. `/track` fetches the game from Steam, stores or updates it in the `game` collection, and saves a watch with a `next_check_at` time.
2. Once a minute, the bot looks for watches whose `next_check_at` has passed.
3. For each due watch it refreshes the game's price (each game is fetched at most once per check, even if several people track it).
4. If the price is lower than the last price you were notified about, the bot mentions you in the channel where you ran `/track`.
5. The watch's next check time is moved forward by its interval.

When a game's price changes, the previous price is appended to its `price_history`. Discounts are stored as percentages.

Please be considerate of Steam's servers, and expect the price fetching to need updates if Steam changes its store pages.

## Logs

Logs are written to `log/tracker.log`, relative to the directory you run the bot from. Errors and warnings go to the log file rather than the console, so check it if something fails unexpectedly.

```bash
tail -f log/tracker.log
```

The log rotates at 50 MB. The last 10 rotated files are kept for 14 days and compressed with gzip. Set `LOG_LEVEL=debug` for more detail:

## Contributing

Issues and pull requests are welcome. Please open an issue first for larger changes.

## License

MIT License. See [LICENSE](LICENSE).