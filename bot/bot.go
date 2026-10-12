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
		},
	},
}

type handlerFunc func(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate)

var handlers = map[string]handlerFunc{
	"track": handleTrack,
}

func handleTrack(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate) {
	// Defer: Mongo + scraping can exceed Discord's 3-second limit.
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	appID := int(i.ApplicationCommandData().Options[0].IntValue())

	// Member is set in servers, User in DMs.
	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	var msg string
	game, err := command.UpsertGame(app.Games, appID)
	if err != nil {
		slog.Error("UpsertGame failed", "appId", appID, "error", err)
		msg = "Couldn't fetch that game. Check the app ID and try again."
	} else {
		// Record who wants alerts, and where.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = app.Watches.UpdateOne(ctx,
			bson.M{"user_id": userID, "app_id": appID},
			bson.M{"$setOnInsert": types.Watch{
				UserId:            userID,
				ChannelId:         i.ChannelID,
				AppId:             appID,
				LastNotifiedPrice: game.PriceSnapshot.DiscountPrice,
				CreatedAt:         time.Now(),
			}},
			options.UpdateOne().SetUpsert(true),
		)
		if err != nil {
			slog.Error("watch upsert failed", "error", err)
			msg = "Found the game, but couldn't save your watch."
		} else {
			msg = fmt.Sprintf("Now tracking **%s** (current price: %.2f)", game.Title, game.PriceSnapshot.DiscountPrice)
		}
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg})
}

func checkNilErr(e error) {
	if e != nil {
		log.Fatal("Error message")
	}
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
