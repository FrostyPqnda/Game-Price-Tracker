package bot

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"time"

	command "game-price-tracker/commands"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var BotToken string

// A list of available commands for the bot
var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "track",
		Description: "Tracks a Steam game's price",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "id",
				Description: "Steam app ID (the number in the store URL)",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "every",
				Description: "How often to check",
				Required:    true,
				MinValue:    func() *float64 { v := 1.0; return &v }(),
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "unit",
				Description: "Minutes, hours, days, weeks or months",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "minutes", Value: "minute"},
					{Name: "hours", Value: "hour"},
					{Name: "days", Value: "day"},
					{Name: "weeks", Value: "week"},
					{Name: "months", Value: "month"},
				},
			},
		},
	},
	{
		Name:        "untrack",
		Description: "Untrack a Steam game",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "id",
				Description: "Steam app ID (the number in the store URL)",
				Required:    true,
			},
		},
	},
	{
		Name:        "watchlist",
		Description: "Display currently tracked games under a user",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionInteger,
				Name:        "page",
				Description: "Page number",
				Required:    true,
			},
		},
	},
}

// Initialize a map storing each command to a specific function
type handlerFunc func(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate)

var handlers = map[string]handlerFunc{
	"track":   handleTrack,
	"untrack": handleUntrack,
}

// nextCheck calculates the next ping time from the current time using
// the specified unit.
func nextCheck(from time.Time, amount int64, unit string) time.Time {
	n := int(amount)
	switch unit {
	case "minute":
		return from.Add(time.Duration(n) * time.Minute)
	case "hour":
		return from.Add(time.Duration(n) * time.Hour)
	case "day":
		return from.AddDate(0, 0, n)
	case "week":
		return from.AddDate(0, 0, 7*n)
	case "month":
		return from.AddDate(0, n, 0)
	}
	return from.AddDate(0, 0, 1) // fallback: 1 day
}

// validateInterval verifies that the given amount fits within the specified
// range for the provided unit.
//
// The minimum ping time for the bot is 30 minutes.
func validateInterval(amount int64, unit string) error {
	if amount < 1 {
		return fmt.Errorf("the interval must be at least 1")
	}

	switch unit {
	case "minute":
		if amount < 30 {
			return fmt.Errorf("the minimum interval is 30 minutes")
		}
	case "hour", "day", "week", "month":
		// Already covered in the base case above
	default:
		return fmt.Errorf("unknown unit %q", unit)
	}
	return nil
}

func getUserId(i *discordgo.InteractionCreate) string {
	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	return userID
}

// handleTrack handles the /track slash command. It starts, or reschedules, a
// price watch for the user who ran the command.
//
// The command takes three options:
//   - id: the Steam app ID of the game
//   - every: how many units to wait between price checks
//   - unit: one of "minute", "hour", "day", "week" or "month"
//
// handleTrack acknowledges the interaction immediately with a deferred
// response, because fetching the game and writing to MongoDB can exceed
// Discord's 3-second limit. It then validates the interval (see
// validateInterval), makes sure the game is stored and up to date through
// UpsertGame, and upserts a Watch keyed by (user_id, app_id).
//
// Running /track again for the same user and game replaces the channel, the
// interval and the next check time, but leaves last_notified_price and
// created_at unchanged. The watch's channel is the one where the command was
// run, and it is where price-drop alerts will be posted.
//
// The final result, or a short error message if validation, the game lookup
// or the save fails, is sent by editing the deferred response. Nothing is
// returned. Errors are logged with slog and are not exposed to the user.
func handleTrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Acknowledge right away; Discord requires a response within 3 seconds.
	// Fetching the game and writing to MongoDB can take longer, so the real
	// reply is sent later by editing this deferred response.
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Index the command's options by name so they can be read safely.
	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		opts[o.Name] = o
	}
	// The Steam app ID of the game to track.
	appID := int(opts["id"].IntValue())
	// How many units to wait between checks (e.g. the 2 in "every 2 days").
	amount := opts["every"].IntValue()
	// The unit: "minute", "hour", "day", "week" or "month".
	unit := opts["unit"].StringValue()

	// Helper that fills in the deferred "thinking..." message.
	reply := func(msg string) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}

	// Works in servers (Member) and in DMs (User).
	userID := getUserId(i)

	// Reject intervals that are too short or use an unknown unit.
	if err := validateInterval(amount, unit); err != nil {
		reply("Sorry, " + err.Error() + ".")
		slog.Warn("Invalid interval", "error", err)
		return
	}

	// Insert or refresh the game so the user sees the latest price and the
	// watch starts from an up-to-date baseline.
	game, err := command.UpsertGame(app.Games, appID)
	if err != nil {
		slog.Error("UpsertGame failed", "appId", appID, "error", err)
		reply("Couldn't fetch that game. Check the app ID and try again.")
		return
	}

	// Don't let the database call hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create or update this user's watch for this game. The filter
	// (user_id + app_id) means one watch per user per game.
	now := time.Now()
	_, err = app.Watches.UpdateOne(ctx,
		bson.M{"user_id": userID, "app_id": appID},
		bson.M{
			// Runs on every /track, so re-running the command changes the
			// channel, the interval and the next check time.
			"$set": bson.M{
				"channel_id":      i.ChannelID, // where alerts will be posted
				"interval_amount": amount,
				"interval_unit":   unit,
				"next_check_at":   nextCheck(now, amount, unit), // when the notifier picks it up
			},
			// Runs only when the watch is first created, so re-tracking
			// doesn't reset the price baseline or creation time.
			"$setOnInsert": bson.M{
				"user_id":             userID,
				"app_id":              appID,
				"last_notified_price": game.PriceSnapshot.DiscountPrice,
				"created_at":          now,
			},
		},
		options.UpdateOne().SetUpsert(true), // insert if no matching watch exists
	)

	// Saving failed: log the details, show the user a short message.
	if err != nil {
		slog.Error("watch upsert failed", "error", err)
		reply("Found the game, but couldn't save your watch.")
		return
	}

	// Success: confirm the game, its current price and the check interval.
	reply(fmt.Sprintf("Now tracking **%s** (current price: %.2f). I'll check every %d %s(s).",
		game.Title, game.PriceSnapshot.DiscountPrice, amount, unit))
}

// handleUntrack handles the /untrack slash command. It stops tracking a game
// for the user who ran the command.
//
// The command takes one option:
//   - id: the Steam app ID of the game to stop tracking
//
// handleUntrack acknowledges the interaction immediately with a deferred
// response, then deletes the Watch matching (user_id, app_id). Only the
// caller's own watch is affected, so other users tracking the same game keep
// their alerts. The game itself and its price history stay in the games
// collection.
//
// The result, or a short error message if the delete fails, is sent by editing
// the deferred response. Nothing is returned. Errors are logged with slog and
// are not exposed to the user.
func handleUntrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Acknowledge right away; Discord requires a response within 3 seconds.
	// The real reply is sent later by editing this deferred response.
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Index the command's options by name so they can be read safely.
	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		opts[o.Name] = o
	}
	// The Steam app ID the user wants to stop tracking.
	appID := int(opts["id"].IntValue())

	// Helper that fills in the deferred "thinking..." message.
	reply := func(msg string) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}

	// Works in servers (Member) and in DMs (User).
	userID := getUserId(i)

	// Don't let the database call hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Delete only this user's watch for this game, so other users tracking
	// the same game are unaffected. The game and its history stay stored.
	// (Use the result's DeletedCount to detect "nothing to untrack".)
	_, err := app.Watches.DeleteOne(ctx,
		bson.M{"user_id": userID, "app_id": appID},
	)

	// A database failure: log the details, show the user a short message.
	if err != nil {
		slog.Error("Failed to delete tracked game", "error", err)
		reply(fmt.Sprintf("Could not untrack game ID %d\n", appID))
		return
	}

	// Success.
	reply(fmt.Sprintf("Untracked **%d**.", appID))
}

// Run starts the Discord bot and blocks until the process receives an
// interrupt (Ctrl+C).
//
// It opens a session using BotToken, routes slash command interactions to the
// matching entry in handlers (passing app to each), and registers the commands
// list globally with ApplicationCommandBulkOverwrite. Because the commands are
// global, they work in every server the bot is invited to, and a new or changed
// command can take a while to show up in Discord clients.
//
// Run also starts the price-drop notifier, which wakes up once a minute and
// processes any watches whose next_check_at time has passed (see
// StartNotifier). The notifier is stopped, and the Discord session closed,
// when Run returns.
//
// Run does not return an error. A failure to create the session, connect to
// Discord or register the commands is logged and terminates the process with
// log.Fatal. BotToken must be set before calling Run.
func Run(app *types.App) {
	// Create the Discord session; no connection is made until Open is called.
	discord, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatal(err)
	}

	// Route every slash command interaction to its handler by command name.
	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// Ignore buttons, autocomplete, modals, etc.
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		// Look up the handler for this command and pass it the shared app.
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(app, s, i)
		}
	})

	// Connect to Discord's gateway. The bot appears online after this.
	if err := discord.Open(); err != nil {
		log.Fatal(err)
	}
	// Close the connection cleanly when Run returns.
	defer discord.Close()

	// Register the commands globally (empty guild ID). This replaces the
	// whole list, so commands missing from `commands` are removed.
	registered, err := discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, "", commands)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("registered %d commands", len(registered))

	// Start the price-drop notifier. It wakes up every minute and handles
	// only the watches that are due. Closing stop (deferred) ends it.
	stop := make(chan struct{})
	defer close(stop)
	StartNotifier(app, discord, time.Minute, stop)

	// Block here until Ctrl+C so the bot keeps running.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
