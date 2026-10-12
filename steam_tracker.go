package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"game-price-tracker/bot"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/filter"
	"game-price-tracker/myimplementations/logging"
	"game-price-tracker/myimplementations/register"
	"game-price-tracker/myimplementations/types"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, relying on real environment variables", "error", err)
	}

	if err := run(); err != nil {
		slog.Error("Run failed", "error", err)
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	command := flag.String("command", "", "command to run (deals, stats, records, lowest, history, upsert, search, crawl); omit to start the Discord bot")
	id := flag.Int("id", 0, "Steam app ID")
	page := flag.Int("page", 1, "page number")
	filterFlag := flag.String("filter", "", "filter records: "+filter.FilterHelp)
	startPage := flag.Int("startPage", 0, "crawl: first results page (omit to resume)")
	endPage := flag.Int("endPage", 0, "crawl: last results page (0 = until results run out)")
	flag.Parse()

	logFile, err := logging.SetupLogging("tracker")
	if err != nil {
		return err
	}
	defer logFile.Close()

	client, err := database.ConnectMongoDB()
	if err != nil {
		return fmt.Errorf("could not connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("MongoDB disconnect error", "error", err)
		}
	}()

	db := client.Database("steam_price_tracker")
	app := &types.App{
		Games:   db.Collection("game"),
		State:   db.Collection("metadata"),
		Watches: db.Collection("watches"),
	}

	// No -command: run the Discord bot (blocks until Ctrl+C).
	if *command == "" {
		token := os.Getenv("DISCORD_TOKEN_ID")
		if token == "" {
			return fmt.Errorf("DISCORD_TOKEN_ID environment variable not set")
		}
		bot.BotToken = token
		slog.Info("Starting Discord bot")
		bot.Run(app)
		return nil
	}

	// -command given: run the CLI command as before.
	cmd, ok := register.LoadRegisters()[*command]
	if !ok {
		return fmt.Errorf("unknown command %q", *command)
	}

	slog.Info("Starting game price tracker", "command", *command)
	options := types.Options{Id: *id, Page: *page, Filter: *filterFlag, StartPage: *startPage, EndPage: *endPage}
	return cmd(app, options)
}
