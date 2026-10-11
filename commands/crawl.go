package commands

import (
	"fmt"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// parsePaginationData crawls the steam store webpage's pagination
// section and extracts the total no. of pages and loads it into
// *totalPages
func parsePaginationData(totalPages *int) {
	const searchURL = "https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=1"

	slog.Debug("Staring pagination data extraction", "url", searchURL)

	// Insantiate a Collecter instance to begin scraping
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Scrape this specific part of the HTML, extracting the total page count
	c.OnHTML("div.search_pagination > div.search_pagination_right", func(e *colly.HTMLElement) {
		e.ForEach("a[href]", func(_ int, el *colly.HTMLElement) {
			page, err := strconv.Atoi(strings.TrimSpace(el.Text))
			if err != nil {
				slog.Debug(
					"Skipping non-numeric pagination element",
					"text", el.Text,
				)
				return
			}

			if page > *totalPages {
				*totalPages = page
			}
		})
	})

	// Print the URL being visted
	c.OnRequest(func(r *colly.Request) {
		slog.Debug("Visiting URL", "url", r.URL.String())
	})

	// Print any error that happens during the scraping
	c.OnError(func(r *colly.Response, err error) {
		slog.Error(
			"Request failed",
			"url", r.Request.URL.String(),
			"status", r.StatusCode,
			"error", err,
		)
	})

	// Visit the Steam store page and log the error if it exists
	var err = c.Visit("https://store.steampowered.com/search?hwtype=0&category1=998&supportedlang=english&hidef2p=1&ndl=1&page=1")
	if err != nil {
		slog.Error("failed to extract pagination data", "error", err)
		return
	}

	slog.Info("pagination data extracted", "total_pages", *totalPages)
}

// extractPercent parses a percentage such as "25%" and returns its
// integer value without the percent sign (25)
//
// Leading and trailing whitespaces are ignored, as is the whitespace
// between the number and the "%".
//
// The number must be a base-10 integer. The result is not range-checked,
// so values such as "150%" and "-5%" are returned as is.
//
// It returns an error if s does not end with "%", or if the remaining
// text is not a valid int64
func extractPercent(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "%") {
		return 0, fmt.Errorf("not a percent: %q", s)
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return strconv.ParseInt(s, 10, 64)
}

// extractPrice parses the price such as "9.99" and returns it
//
// Leading and trailing whitespaces are ignored, as is the whitespace
// between the number and the "$".
//
// It returns an error if s does not start with "$"", or if the remaining
// text is not a valid float64
func extractPrice(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "$") {
		return 0, fmt.Errorf("not a price: %q", s)
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "$"))
	return strconv.ParseFloat(s, 64)
}

// cleanURL takes in URL href and cleans up the string by removing
// the trailing serial no. at the end of it
//
// Input: https://store.steampowered.com/app/<app id>/<title>/?snr=<serial no.>
// Output: https://store.steampowered.com/app/<app id>
func cleanURL(href string) string {
	if before, _, found := strings.Cut(href, "/?"); found {
		if idx := strings.LastIndex(before, "/"); idx != -1 {
			return before[:idx]
		}
		return before
	}

	return href
}

// Crawl traverses the Steam store search webpage and scrapes the rows of game data into
// gameCollection.
//
// It also provides two optional flags for start and endpoints to allow users to allow scrape
// only the specified page ranges [start, end].
//
// It provides the state saving to allow users to run the function from the current state
// instead of starting back at the 1st page.
func Crawl(gameCollection, stateCollection *mongo.Collection, startFlag, endFlag int) {
	slog.Info("Running Crawl")

	// Initialize a Collector instance to begin crawling
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Setup a delay to avoid hitting the page too many times and triggering its rate limit
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*store.steampowered.com*",
		Delay:       90 * time.Second,
		RandomDelay: 30 * time.Second,
	})
	c.SetRequestTimeout(30 * time.Second)

	gamesFound := 0                     // No. of games found
	upsertErrors := 0                   // No. of upsertion errors
	extractionWarnings := 0             // No. of extraction warnings
	var pagesAttempted, pagesFailed int // No. of pages attempted and failed
	var failedPages []int               // List of failed pages

	// Search the Steam store games list
	c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
		// Extract the game's app id and end the crawling if there was
		// an error
		id, err := strconv.Atoi(e.Attr("data-ds-appid"))
		if err != nil {
			slog.Error("Failed to extract app id", "error", err)
			return
		}

		// Extract the discount, original price, and discount price
		discount, discErr := extractPercent(e.ChildText("div.discount_pct"))
		originalPrice, origErr := extractPrice(e.ChildText("div.discount_original_price"))
		discountPrice, finalErr := extractPrice(e.ChildText("div.discount_final_price"))
		isNoDiscount := e.DOM.Find("div.discount_block").HasClass("no_discount")

		// Steam omits the discount % and original price fields entirely when
		// there's no discount, so those two "errors" are expected in that case.
		// Only log them when they're NOT explained by a no-discount listing.
		if !isNoDiscount && (discErr != nil || origErr != nil) {
			slog.Warn("Partial price extraction",
				"app_id", id,
				"discount_err", discErr,
				"original_price_err", origErr,
			)
			extractionWarnings++
		}

		if finalErr != nil {
			slog.Warn("Failed to extract discount price", "app_id", id, "error", finalErr)
			extractionWarnings++
		}

		// If the game does not have a discount, set the original price
		// and the discount price to be the same
		//
		// Context: Steam puts the final price in the discount field
		// even if there is no discount
		if isNoDiscount {
			originalPrice = discountPrice
			discount = 0
		}

		// Create a PriceRecord data at the most recent time checked
		price := types.PriceRecord{
			OriginalPrice: originalPrice,
			DiscountPrice: discountPrice,
			Discount:      discount * -1,
			CheckedAt:     time.Now(),
		}

		// Create a Game data storing
		// - id
		// - title
		// - url
		// - current price snapshot data
		// - price history
		game := types.Game{
			AppId:         id,
			Title:         e.ChildText("span.title"),
			URL:           cleanURL(e.Attr("href")),
			PriceSnapshot: price,
			PriceHistory:  []types.PriceRecord{price},
		}

		// Upsert the game into the Game document and throw an error if it fails
		if err := database.UpsertGameData(gameCollection, game); err != nil {
			slog.Error("Upsertion error",
				"app_id", game.AppId,
				"title", game.Title,
				"error", err,
			)
			upsertErrors++
			return
		}
		gamesFound++
	})

	// Log the current page being visited
	c.OnRequest(func(r *colly.Request) {
		slog.Info("Visiting URL", "url", r.URL.String())
	})

	// Colly sends non-2xx responses (including 429) to OnError, not
	// OnResponse, so the rate-limit backoff lives here.
	c.OnError(func(r *colly.Response, err error) {
		slog.Error("Request error", "url", r.Request.URL.String(), "status", r.StatusCode, "error", err)
		if r.StatusCode == 429 {
			slog.Warn("Rate limited, backing off 5 minutes...")
			time.Sleep(5 * time.Minute)
		}
	})

	ranged := startFlag > 0 || endFlag > 0
	var startPage, lastPage int

	if ranged {
		// Always look up the real total so -endPage beyond the last page is clamped
		var totalPages int
		parsePaginationData(&totalPages)
		if totalPages == 0 {
			slog.Error("Pagination extraction returned 0 pages, aborting crawl")
			return
		}

		// Clamp the starting  page to be the maximum between
		// the start range and 1.
		startPage = max(startFlag, 1)

		// Set lastPage to be the totalPages initially then
		// set it to the specified endPage if the endPage
		// if the end flag exists and is less than totalPages
		lastPage = totalPages
		if endFlag > 0 && endFlag < totalPages {
			lastPage = endFlag
		}

		// Notify users that startPage exceeds lastPage
		if startPage > lastPage {
			fmt.Printf("-startPage (%d) is past the last page (%d)\n", startPage, lastPage)
			return
		}

		// Log the crawl range
		slog.Info("Ranged crawl: saved progress is not read or changed",
			"start_page", startPage, "last_page", lastPage, "total_pages", totalPages)
	} else {
		// last is the last page COMPLETED, so resume at last+1.
		last, total, err := database.LoadMetadataState(stateCollection)
		if err != nil {
			slog.Error("Failed to load metadata", "error", err)
			return
		}

		if last >= total {
			// No checkpoint yet, or the previous crawl finished: start a new cycle.
			var totalPages int
			parsePaginationData(&totalPages)
			if totalPages == 0 {
				slog.Error("Pagination extraction returned 0 pages, aborting crawl")
				return
			}

			// Save 0 as the last completed page: nothing is done yet in this cycle.
			if err := database.SaveCrawlRange(stateCollection, 0, totalPages); err != nil {
				slog.Error("Failed to save initial crawl range, aborting", "error", err)
				return
			}

			startPage, lastPage = 1, totalPages
			slog.Info("Starting new crawl cycle", "total_pages", totalPages)
		} else {
			startPage, lastPage = last+1, total
			slog.Info("Resuming crawl", "start_page", startPage, "last_page", lastPage)
		}
	}

	// Visit every page in [startPage, lastPage]
	for page := startPage; page <= lastPage; page++ {
		// Visit the specified page
		pagesAttempted++
		url := fmt.Sprintf("https://store.steampowered.com/search?hwtype=0&category1=998&supportedlang=english&hidef2p=1&ndl=1&sort_by=Released_ASC&page=%d", page)
		if err := c.Visit(url); err != nil {
			slog.Error("Failed to visit", "url", url, "page", page, "error", err)
			pagesFailed++
			failedPages = append(failedPages, page)

			if !ranged {
				// The checkpoint means "everything up to here is done", so
				// don't move past a failed page. The next run resumes at it.
				slog.Warn("Stopping crawl at failed page", "page", page)
				break
			}
			continue
		}

		// Never save the crawl state if we use a ranged value
		if ranged {
			continue // never touch saved progress on a ranged crawl
		}

		// Attempt to save the current crawl after every page visit
		if err := database.SaveCurrentCrawl(stateCollection, page); err != nil {
			slog.Error("Failed to save crawl checkpoint", "page", page, "error", err)
		}
	}

	slog.Info("Crawl completed",
		"pagesAttempted", pagesAttempted,
		"pagesFailed", pagesFailed,
		"failedPages", failedPages,
		"totalPagesAvailable", lastPage,
		"gamesFound", gamesFound,
		"upsertErrors", upsertErrors,
		"extractionWarnings", extractionWarnings,
	)
}
