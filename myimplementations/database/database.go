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

// UpsertGameData upserts a game and its price record into the Game document
// if it does not exist. Otherwise, it will update the price history
// if there has been a change, i.e., discount change or price change
func UpsertGameData(collection *mongo.Collection, g types.Game) error {
	ctx := context.TODO()

	// Search for the game inside the Game document using its
	// app id.
	//
	// If it does not exists, insert into the document
	// and return the error if there was one.
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": g.AppId},
	).Decode(&existing)

	if err == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(context.TODO(), g)
		if err != nil {
			slog.Error(
				"Failed to insert new game",
				"app_id", g.AppId,
				"game", g.Title,
				"error", err,
			)

			return err
		}

		slog.Info(
			"Inserted new game",
			"app_id", g.AppId,
			"game", g.Title,
		)

		return nil
	}

	// If there is an error but insertion failed, return it
	if err != nil {
		slog.Error(
			"Failed to find game",
			"app_id", g.AppId,
			"game", g.Title,
			"error", err,
		)
		return err
	}

	// Get the game's current price
	rec := g.PriceSnapshot

	// Get the last price record from its price history
	last := existing.PriceHistory[len(existing.PriceHistory)-1]

	// True under any of the following condition
	// - The game does not have a pre-existing price history
	// - The last discount price does not match the current discount price
	// - The last original price does not match the current original price
	changed := len(existing.PriceHistory) == 0 ||
		last.DiscountPrice != rec.DiscountPrice ||
		last.OriginalPrice != rec.OriginalPrice

	// Initialize an update variable to update/set these
	// 3 specific field
	update := bson.M{
		"$set": bson.M{
			"price_snapshot": rec,
			"title":          g.Title,
			"url":            g.URL,
		},
	}

	// If there was a change in the game's price history, add it to the
	// update map
	if changed {
		update["$push"] = bson.M{"price_history": rec}
		slog.Info(
			"Updated game price data",
			"app_id", g.AppId,
			"game", g.Title,
			"old_price_data", last,
			"new_price_data", rec,
		)
	}

	// Update the game with the field using its app id and return the error
	_, err = collection.UpdateOne(context.TODO(), bson.M{"_id": g.AppId}, update)
	return err
}

// crawlStateId is the _id of the single document in the metadata collection
// that stores the crawler's checkpoint. All three functions below read or
// write that one document.
const crawlStateId = "steam_crawl"

// LoadMetadataState reads the saved crawl checkpoint from collection and
// returns the last page that was crawled and the total number of pages.
//
// If no checkpoint has been saved yet, it returns 0, 0 and a nil error, so
// callers should treat a zero last page as "start from the beginning". Any
// other failure is logged and returned.
//
// The lookup has no timeout.
func LoadMetadataState(collection *mongo.Collection) (int, int, error) {
	ctx := context.TODO()

	var state types.Metadata
	err := collection.FindOne(
		ctx,
		bson.M{"_id": crawlStateId},
	).Decode(&state)

	// A missing document is the normal first-run case, not an error.
	if err == mongo.ErrNoDocuments {
		slog.Info("No existing crawl checkpoint found, starting from page 1")
		return 0, 0, nil
	}

	if err != nil {
		slog.Error("Failed to fetch crawl state", "error", err)
		return 0, 0, err
	}

	slog.Info("Resuming crawl from checkpoint", "last_page", state.LastPage, "total_pages", state.TotalPages)
	return state.LastPage, state.TotalPages, nil
}

// SaveCrawlRange stores page as the last crawled page and totalPages as the
// total page count in the crawl checkpoint document, and records the update
// time. The document is created if it doesn't exist.
//
// Use it when the total page count is known or has changed; use
// SaveCurrentCrawl to advance only the page number. It returns any database
// error after logging it. The update has no timeout.
func SaveCrawlRange(collection *mongo.Collection, page, totalPages int) error {
	// TODO context: no deadline, so a hung database connection blocks forever.
	ctx := context.TODO()

	set := bson.M{
		"last_page":   page,
		"total_pages": totalPages,
		"updated_at":  time.Now(),
	}

	// $set only touches the listed fields, and SetUpsert(true) creates the
	// document on the first save.
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": crawlStateId},
		bson.M{"$set": set},
		options.UpdateOne().SetUpsert(true),
	)

	if err != nil {
		slog.Error("Failed to save crawl range", "page", page, "total_pages", totalPages, "error", err)
	}

	return err
}

// SaveCurrentCrawl stores page as the last crawled page in the crawl
// checkpoint document and records the update time, leaving the saved total
// page count untouched. The document is created if it doesn't exist.
//
// It is the lightweight checkpoint to call after each page is crawled. It
// returns any database error after logging it. The update has no timeout.
func SaveCurrentCrawl(collection *mongo.Collection, page int) error {
	// TODO context: no deadline, so a hung database connection blocks forever.
	ctx := context.TODO()

	set := bson.M{"last_page": page, "updated_at": time.Now()}

	// Because only last_page and updated_at are set, total_pages keeps
	// whatever SaveCrawlRange stored earlier.
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": crawlStateId},
		bson.M{"$set": set},
		options.UpdateOne().SetUpsert(true),
	)

	if err != nil {
		slog.Error("Failed to save crawl checkpoint", "page", page, "error", err)
	}

	return err
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
