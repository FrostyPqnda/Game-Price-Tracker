package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"game-price-tracker/myimplementations/utils"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

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
func HandleUntrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	userID := utils.GetUserId(i)

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
