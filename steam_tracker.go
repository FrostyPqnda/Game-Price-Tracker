package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"game-price-tracker/bot"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/logging"
	"game-price-tracker/myimplementations/types"

	"github.com/joho/godotenv"
)

// serveHealth gives Render a port to detect. The bot itself doesn't need it.
func serveHealth() {
	port := os.Getenv("PORT") // Render sets this; default is 10000
	if port == "" {
		port = "10000"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	go func() {
		if err := http.ListenAndServe(":"+port, mux); err != nil {
			log.Printf("health server stopped: %v", err)
		}
	}()
}

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, relying on real environment variables", "error", err)
	}

	serveHealth()

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
		Watches: db.Collection("watches"),
	}

	if err := database.EnsureIndexes(app.Watches); err != nil {
		return fmt.Errorf("could not ensure indexes: %w", err)
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
