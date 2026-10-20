package bot

import (
	"game-price-tracker/commands"
	"game-price-tracker/myimplementations/notifier"
	"game-price-tracker/myimplementations/types"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
)

var BotToken string

// A list of available commands for the bot
var botCommands = []*discordgo.ApplicationCommand{
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
				Description: "Page number (default 1)",
				Required:    false,
				MinValue:    func() *float64 { v := 1.0; return &v }(),
			},
		},
	},
}

// Initialize a map storing each command to a specific function
type handlerFunc func(app *types.App, s *discordgo.Session, i *discordgo.InteractionCreate)

var handlers = map[string]handlerFunc{
	"track":     commands.HandleTrack,
	"untrack":   commands.HandleUntrack,
	"watchlist": commands.HandleWatchlist,
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
	registered, err := discord.ApplicationCommandBulkOverwrite(discord.State.User.ID, "", botCommands)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("registered %d commands", len(registered))

	// Start the price-drop notifier. It wakes up every minute and handles
	// only the watches that are due. Closing stop (deferred) ends it.
	stop := make(chan struct{})
	defer close(stop)
	notifier.StartNotifier(app, discord, time.Minute, stop)

	// Block here until Ctrl+C so the bot keeps running.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}
