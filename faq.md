# GameDip FAQ

Common questions about the GameDip Discord bot. For setup and command details, see the [README](README.md).

## Using the bot

### How do I find a game's Steam app ID?

Open the game's page on the Steam store. The app ID is the number in the URL, for example `store.steampowered.com/app/`**`4078430`**`/Game_Name`. Use that number as the `id` option in `/track`.

### How do I start tracking a game?

Run `/track` with the app ID, how often to check, and the unit:

```
/track id:4078430 every:12 unit:hours
```

The bot fetches the current price as your starting point and posts any price-drop alerts in the channel where you ran the command.

### How do I stop tracking a game?

Run `/untrack id:<app id>`. Only your own watch is removed. Other users tracking the same game aren't affected.

### How do I see what I'm tracking?

Run `/watchlist`. It shows five games per page with each game's title, current price, discount and check interval. Use the optional `page` option to move through longer lists.

### Can other people see my watchlist?

No. Watches (who tracks what) are stored per user and are only shown to the user who created them. Game data itself (title, URL, prices) is shared and public.

### Is there a limit on how many games I can track?

Yes, you can track up to 25 games.

### Why is there a minimum check interval for minutes?

Intervals in minutes have a minimum (30 by default) to be considerate of Steam's servers. If you need something more frequent than that, it's better to check the Steam store page directly.

## Notifications

### Why didn't I get a notification?

Work through these in order:

1. **The price has to drop below the last price you were notified about.** A price that stays the same or goes up doesn't trigger an alert.
2. **The check may not be due yet.** Games are checked on the interval you chose, so a 1-week interval means up to a week between checks.
3. **The bot needs permission to post in the channel.** Make sure it can view the channel, send messages, and mention users there. If the channel was deleted or the bot lost access, alerts can't be delivered.
4. **The bot might be offline.** If it isn't running, no checks happen until it's back.

### Which channel do alerts go to?

The channel where you ran `/track`. Running `/track` again for the same game from a different channel changes where alerts are posted.

### Does running `/track` again reset my price baseline?

No. Running `/track` for a game you already track changes its interval and the alert channel, but your price baseline stays where it was. Alerts are still compared against the last price you were notified about.

### Will I be notified more than once for the same price drop?

No. After an alert, the baseline moves to the new lower price, so you're only notified again if the price drops below that.

## Prices and data

### What currency are prices in?

USD

### What does the discount percentage mean?

It's the current discount off the game's regular price, stored as a percentage.

### Does the bot keep a price history?

Yes. When a game's price changes, the previous price is appended to that game's price history.

### What happens if Steam changes its store pages?

Price fetching depends on Steam's store pages, so a change on Steam's side can temporarily stop prices from updating. If prices look stale or stop updating, please open an issue on GitHub.

### How do I remove my data?

`/untrack` removes your watch for a game. Run it for each game on your watchlist to remove all of your watches. The shared game data (title, URL, prices) is public and isn't tied to you. If you want help with anything beyond that, open an issue.

## Privacy and permissions

### What does the bot do with my messages?

Nothing. GameDip doesn't read message content, moderate, or respond to chat. It only responds to its slash commands (`/track`, `/untrack`, `/watchlist`).

### Is GameDip affiliated with Steam or Valve?

No. GameDip is an independent, unofficial Steam price tracker and is not affiliated with or endorsed by Valve Corporation.

## Other Questions

### Why did you build this, there are other better trackers?

I built it for fun.

### This tracker sucks, why would anyone use it?

Good question. I honestly ask myself that too.  

### Why does the bot stop working from time to time?

Because I don't have a job.

### Was this built with AI tools?

Yes. It would've been far more dishonest for me to say I didn't use it. 

### Will this project ever be updated?

Maybe. 

## Still stuck?

Open an issue on the GitHub repository with what you ran, what you expected, and what happened.