package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"
	"game-price-tracker/myimplementations/utils"
	"log/slog"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// HandleWatchlist handles the /watchlist slash command. It shows the caller's
// tracked games, five per page, in a single reply only they can see.
func HandleWatchlist(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Acknowledge right away (ephemeral, so only the caller sees the list).
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	reply := func(msg string) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// "page" is optional; default to the first page.
	page := int64(1)
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "page" {
			page = o.IntValue()
		}
	}
	if page < 1 {
		page = 1
	}

	// Fetch only the data owned by the user
	userID := utils.GetUserId(i)
	filter := bson.M{"user_id": userID}
	const pageSize = int64(5)

	total, err := app.Watches.CountDocuments(ctx, filter)
	if err != nil {
		slog.Error("Failed to count watches", "error", err)
		reply("Couldn't load your watchlist.")
		return
	}

	// No game is being tracked. Notify users
	if total == 0 {
		reply("You aren't tracking any games. Use `/track` to add one.")
		return
	}

	totalPages := (total + pageSize - 1) / pageSize
	if page > totalPages {
		reply(fmt.Sprintf("Page %d doesn't exist. You have %d page(s).", page, totalPages))
		return
	}

	// Pagination 
	findOpts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(pageSize).
		SetSkip((page - 1) * pageSize)

	cur, err := app.Watches.Find(ctx, filter, findOpts)
	if err != nil {
		slog.Error("Watch query failed", "error", err)
		reply("Couldn't load your watchlist.")
		return
	}
	var watches []types.Watch
	if err := cur.All(ctx, &watches); err != nil {
		slog.Error("Watch decode failed", "error", err)
		reply("Couldn't load your watchlist.")
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**Your watchlist** (page %d/%d)\n\n", page, totalPages)
	for _, w := range watches {
		game, ok := database.FetchGame(app.Games, w.AppId)
		if !ok {
			slog.Error("Failed to query game", "app id", w.AppId)
			fmt.Fprintf(&b, "• App %d (couldn't load)\n", w.AppId)
			continue
		}
		// <url> stops Discord from expanding a large embed for each link.
		fmt.Fprintf(&b, "• **%s**: %.2f (%d%% off), every %d %s(s)\n  %s\n",
			game.Title, game.PriceSnapshot.DiscountPrice, game.PriceSnapshot.Discount,
			w.IntervalAmount, w.IntervalUnit, game.URL)
	}
	reply(b.String())
}
