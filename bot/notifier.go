package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"
	"game-price-tracker/myimplementations/utils"

	"github.com/bwmarrin/discordgo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// StartNotifier is used to track every game that is being used for price tracking
// and will trigger in the event there is one.
func StartNotifier(app *types.App, s *discordgo.Session, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				checkWatches(app, s)
			case <-stop:
				return
			}
		}
	}()
}

// checkWatches is used to check every tracked game under the user
// and notify them via a Discord ping if there is a price drop
//
// Each successive ping is triggered by a set time interval chosen by
// user and only happens if there is a price drop
//
// The game data is also updated in correspondence to the price drop.
func checkWatches(app *types.App, s *discordgo.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	now := time.Now()
	cur, err := app.Watches.Find(ctx, bson.M{"next_check_at": bson.M{"$lte": now}})
	if err != nil {
		slog.Error("watch query failed", "error", err)
		return
	}
	var watches []types.Watch
	if err := cur.All(ctx, &watches); err != nil {
		slog.Error("Watch decode failed", "error", err)
		return
	}

	// Scrape each game at most once per tick, even if many users watch it.
	refreshed := map[int]types.Game{}

	for _, w := range watches {
		game, ok := refreshed[w.AppId]
		if !ok {
			game, err = database.UpsertGame(app.Games, w.AppId)
			if err != nil {
				slog.Error("refresh failed", "appId", w.AppId, "error", err)
				continue
			}
			refreshed[w.AppId] = game
		}

		// Notify users when the price has dropped
		price := game.PriceSnapshot.DiscountPrice
		if price < w.LastNotifiedPrice {
			_, err := s.ChannelMessageSendComplex(w.ChannelId, &discordgo.MessageSend{
				Content: fmt.Sprintf("<@%s> **%s** dropped to %.2f (was %.2f)\n%s",
					w.UserId, game.Title, price, w.LastNotifiedPrice, game.URL),
				AllowedMentions: &discordgo.MessageAllowedMentions{Users: []string{w.UserId}},
			})
			if err != nil {
				slog.Error("Notify failed", "user", w.UserId, "error", err)
				continue
			}
		}

		// Update the tracked game in MongoDB
		_, err := app.Watches.UpdateOne(ctx,
			bson.M{"user_id": w.UserId, "app_id": w.AppId},
			bson.M{"$set": bson.M{
				"last_notified_price": price,
				"next_check_at":       utils.NextCheck(now, w.IntervalAmount, w.IntervalUnit),
			}},
		)
		if err != nil {
			slog.Error("watch update failed", "error", err)
		}

		slog.Info("Notifier tick", "Id", w, "AppId", w.AppId, "UserId", w.UserId)
	}
}
