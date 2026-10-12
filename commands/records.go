package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-runewidth"

	mytable "game-price-tracker/myimplementations/table"
	"game-price-tracker/myimplementations/types"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DisplayRecords prints one page of tracked games, sorted by app_id, to
// standard output as a table of 25 games per page
//
// Only games matching filter are shown; pass an empty bson.M{} to match
// everything.
func DisplayRecords(collection *mongo.Collection, page int, filter bson.M) {
	ctx := context.TODO()

	// Clamp invalid page numbers rather than reporting an error.
	if page < 1 {
		page = 1
	}
	pageSize := int64(25)
	skip := (int64(page) - 1) * pageSize

	// Count first so we can report the total page count and reject
	// out-of-range pages before running the real query.
	total, err := collection.CountDocuments(ctx, filter) // was bson.D{}
	if err != nil {
		slog.Error("Failed to count documents", "error", err)
		return
	}
	if total == 0 {
		fmt.Println("No games match that filter.")
		return
	}

	// Integer ceiling division: rounds up so a partial last page counts.
	totalPages := int((total + pageSize - 1) / pageSize)

	if page > totalPages {
		fmt.Printf("Page %d is out of range (total pages: %d)\n", page, totalPages)
		return
	}

	slog.Info("Running DisplayRecords", "page", page, "pageSize", pageSize, "skip", skip)

	// Sort is required for stable pagination: without it, skip/limit can
	// return overlapping or missing documents between pages.
	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: 1}}).
		SetLimit(pageSize).
		SetSkip(skip)

	cursor, err := collection.Find(ctx, filter, opts) // was bson.D{}
	if err != nil {
		slog.Error("Failed to query", "page", page, "error", err)
		return
	}

	// All decodes every document and closes the cursor for us.
	var results []types.Game
	if err := cursor.All(ctx, &results); err != nil {
		slog.Error("Failed to load", "page", page, "error", err)
		return
	}

	slog.Info("Fetched records", "page", page, "count", len(results))

	printGames(results, page, totalPages)

	slog.Info("DisplayRecords completed", "page", page, "count", len(results))
}

// printGames renders games as a table on standard output, one row per game,
// showing its app ID, title, current price, current discount, lowest recorded
// price, and the time of the latest price check.
//
// page and totalPages are used only for the table title. The caption suggests
// the command for the following page.
func printGames(games []types.Game, page, totalPages int) {
	t := mytable.NewTable(fmt.Sprintf("Steam Price Tracker — Records - Page %d of %d", page, totalPages))

	t.AppendHeader(table.Row{"APP ID", "TITLE", "PRICE", "DISCOUNT", "LOWEST", "CHECKED"})

	for _, g := range games {
		s := g.PriceSnapshot

		// Default to the plain price; only discounted games get the
		// "was" price and a discount percentage.
		price := fmt.Sprintf("$%.2f", s.DiscountPrice)
		discount := "-"

		// Discount is stored as a negative number (e.g. -50), so a value
		// below zero means the game is currently on sale.
		if s.Discount < 0 {
			price = fmt.Sprintf("$%.2f (was $%.2f)", s.DiscountPrice, s.OriginalPrice)
			discount = text.FgGreen.Sprintf("%d%%", s.Discount*-1)
		}

		t.AppendRow(table.Row{
			g.AppId,
			// runewidth counts display columns, so wide (e.g. CJK)
			// characters don't break the table layout.
			runewidth.Truncate(g.Title, 40, "…"),
			price,
			discount,
			fmt.Sprintf("$%.2f", lowestPrice(g)),
			s.CheckedAt.Local().Format("2006-01-02 15:04"),
		})
	}

	// Right-align the numeric columns (app ID, discount, lowest) so they line up.
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, Align: text.AlignRight},
		{Number: 4, Align: text.AlignRight, AlignHeader: text.AlignRight},
		{Number: 5, Align: text.AlignRight, AlignHeader: text.AlignRight},
	})

	t.SetCaption("Showing %d games. Next: -command=records -page=%d", len(games), page+1)
	t.Render()
}

// lowestPrice returns the lowest DiscountPrice recorded for g, considering
// both its current PriceSnapshot and its PriceHistory.
func lowestPrice(g types.Game) float64 {
	// Start from the current snapshot so the result is always a real price,
	// even when the history is empty.
	low := g.PriceSnapshot.DiscountPrice
	for _, h := range g.PriceHistory {
		if h.DiscountPrice < low {
			low = h.DiscountPrice
		}
	}
	return low
}
