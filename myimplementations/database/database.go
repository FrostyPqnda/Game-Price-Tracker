package database

import (
	"context"
	"errors"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// ConnectMongoDB connects to MongoDB using MONGO_URI, MONGO_USERNAME and
// MONGO_PASSWORD (read from a .env file or the environment).
//
// On success it returns a connected client and a nil error.
// On failure it returns a nil client and the reason.
func ConnectMongoDB() (*mongo.Client, error) {
	// Load the .env file and log the error if it odes not exists
	if err := godotenv.Load(); err != nil {
		slog.Error("No .env file found, relying on real environment variables", "error", err)
	}

	// Get the MONGO_URI variable and log the error
	// if it is not set
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGO_URI environment variable not set")
	}

	username := os.Getenv("MONGO_USERNAME")
	password := os.Getenv("MONGO_PASSWORD")
	if username == "" || password == "" {
		return nil, fmt.Errorf("MONGO_USERNAME or MONGO_PASSWORD environment variable not set")
	}

	// Establish a connection to MongoDB and instantiate a mongoDB client
	//
	// Log an error if a connection fails
	clientOpts := options.Client().ApplyURI(mongoURI).SetAuth(options.Credential{
		Username: username,
		Password: password,
	})

	slog.Debug("Connecting to MongoDB")

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB client: %w", err)
	}

	// Create a context and ping the MongoDB, giving it a 2 second deadline
	//
	// If it fails to ping, return a null client and an error
	// Otherwise, return the client and a null error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		client.Disconnect(context.Background())
		return nil, fmt.Errorf("could not reach MongoDB: %w", err)
	}

	slog.Debug("Connection established")

	return client, nil
}

// FetchGame looks up the game with the given appID in collection. The query is
// limited to 10 seconds.
//
// It reports whether the game was found. If no game matches appID, a message
// is printed to standard output and a warning is logged. Any other failure
// (including a timeout) is only logged at error level, so nothing is printed
// to the user. In both cases the returned bool is false.
func FetchGame(collection *mongo.Collection, appId int) (types.Game, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the game with the matching appId and decode it
	var game types.Game
	err := collection.FindOne(ctx, bson.M{"_id": appId}).Decode(&game)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			slog.Warn("No game found for app ID", "appId", appId)
			fmt.Printf("No game found with app ID %d\n", appId)
		} else {
			slog.Error("Failed to fetch game", "appId", appId, "error", err)
		}
		return types.Game{}, false
	}

	return game, true
}
