package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/scraper"
	"game-price-tracker/myimplementations/types"
	"game-price-tracker/myimplementations/utils"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const MAX_WATCH int = 25

// HandleTrack handles the /track slash command. It starts, or reschedules, a
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
func HandleTrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	userID := utils.GetUserId(i)

	// Reject intervals that are too short or use an unknown unit.
	if err := utils.ValidateInterval(amount, unit); err != nil {
		reply("Sorry, " + err.Error() + ".")
		slog.Warn("Invalid interval", "error", err)
		return
	}

	// Insert or refresh the game so the user sees the latest price and the
	// watch starts from an up-to-date baseline.
	game, err := scraper.UpsertGame(app.Games, appID)
	if err != nil {
		slog.Error("UpsertGame failed", "appId", appID, "error", err)
		reply("Couldn't fetch that game. Check the app ID and try again.")
		return
	}

	// Don't let the database call hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	watchCount, err := app.Watches.CountDocuments(ctx, bson.M{"user_id": userID})
	if err != nil {
		slog.Error("Watch lookup failed", "error", err)
		reply("Something went wrong. Try again in a moment.")
		return
	}

	if watchCount >= int64(MAX_WATCH) {
		reply(fmt.Sprintf("You're tracking the maximum of %d games. Use /untrack to free up a slot.\n", MAX_WATCH))
		return
	}

	// Create or update this user's watch for this game. The filter
	// (user_id + app_id) means one watch per user per game.
	now := time.Now()
	filter := bson.M{"user_id": userID, "app_id": appID}
	update := bson.M{
		"$set": bson.M{
			"channel_id":      i.ChannelID,
			"interval_amount": amount,
			"interval_unit":   unit,
			"next_check_at":   utils.NextCheck(now, amount, unit),
		},
		"$setOnInsert": bson.M{
			"last_notified_price": game.PriceSnapshot.DiscountPrice,
			"created_at":          now,
		},
	}
	upsertOpts := options.UpdateOne().SetUpsert(true)

	_, err = app.Watches.UpdateOne(ctx, filter, update, upsertOpts)
	if mongo.IsDuplicateKeyError(err) {
		// Lost an upsert race with a concurrent /track; the watch exists now.
		_, err = app.Watches.UpdateOne(ctx, filter, update, upsertOpts)
	}

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
