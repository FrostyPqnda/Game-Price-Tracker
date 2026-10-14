package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"game-price-tracker/bot"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/logging"
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

	token := os.Getenv("DISCORD_TOKEN_ID")
	if token == "" {
		return fmt.Errorf("DISCORD_TOKEN_ID environment variable not set")
	}
	bot.BotToken = token
	slog.Info("Starting Discord bot")
	bot.Run(app)
	return nil
}
