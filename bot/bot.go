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
var ServerId string

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
}

type handlerFunc func(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate)

var handlers = map[string]handlerFunc{
	"track":   handleTrack,
	"untrack": handleUntrack,
}

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

func handleTrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		opts[o.Name] = o
	}
	appID := int(opts["id"].IntValue())
	amount := opts["every"].IntValue()
	unit := opts["unit"].StringValue()

	reply := func(msg string) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}

	if err := validateInterval(amount, unit); err != nil {
		reply("Sorry, " + err.Error() + ".")
		return
	}

	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	game, err := command.UpsertGame(app.Games, appID)
	if err != nil {
		slog.Error("UpsertGame failed", "appId", appID, "error", err)
		reply("Couldn't fetch that game. Check the app ID and try again.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	_, err = app.Watches.UpdateOne(ctx,
		bson.M{"user_id": userID, "app_id": appID},
		bson.M{
			// Always refresh the schedule, so re-running /track changes the interval.
			"$set": bson.M{
				"channel_id":      i.ChannelID,
				"interval_amount": amount,
				"interval_unit":   unit,
				"next_check_at":   nextCheck(now, amount, unit),
			},
			// Only set these when the watch is first created.
			"$setOnInsert": bson.M{
				"user_id":             userID,
				"app_id":              appID,
				"last_notified_price": game.PriceSnapshot.DiscountPrice,
				"created_at":          now,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		slog.Error("watch upsert failed", "error", err)
		reply("Found the game, but couldn't save your watch.")
		return
	}

	reply(fmt.Sprintf("Now tracking **%s** (current price: %.2f). I'll check every %d %s(s).",
		game.Title, game.PriceSnapshot.DiscountPrice, amount, unit))
}

func handleUntrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	opts := map[string]*discordgo.ApplicationCommandInteractionDataOption{}
	for _, o := range i.ApplicationCommandData().Options {
		opts[o.Name] = o
	}
	appID := int(opts["id"].IntValue())

	reply := func(msg string) {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
	}

	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := app.Watches.DeleteOne(ctx,
		bson.M{"user_id": userID, "app_id": appID},
	)

	if err != nil {
		slog.Error("Failed to delete tracked game", "error", err)
		reply(fmt.Sprintf("Could not untrack game ID %d\n", appID))
		return
	}

	reply(fmt.Sprintf("Untracked **%d**.", appID))
}

func Run(app *types.App) {
	discord, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatal(err)
	}

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		if h, ok := handlers[i.ApplicationCommandData().Name]; ok {
			h(app, s, i)
		}
	})

	if err := discord.Open(); err != nil {
		log.Fatal(err)
	}
	defer discord.Close()

	registered, err := discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, "", commands)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("registered %d commands", len(registered))

	// Start the price-drop notifier
	stop := make(chan struct{})
	defer close(stop)
	StartNotifier(app, discord, time.Minute, stop)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
