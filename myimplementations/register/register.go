package register

import (
	"errors"
	"fmt"

	"game-price-tracker/commands"
	"game-price-tracker/myimplementations/filter"
	"game-price-tracker/myimplementations/types"
)

// requireID checks that the -id option was provided and is a positive number.
// Steam app IDs are always positive, so zero (the unset value) and negative
// numbers are both rejected.
func requireID(o types.Options) error {
	if o.ID <= 0 {
		return errors.New("missing or invalid -id")
	}
	return nil
}

// LoadRegisters returns the table of CLI commands, keyed by the name the user
// types for -command. Each entry is a types.Command: it receives the shared
// App (database collections) and the parsed Options, validates the options it
// needs, then runs the matching function from the commands package.
//
// The commands are:
//
//   - stats: summary statistics for the whole tracker.
//   - records: one page of games, optionally narrowed by -filter and -page.
//   - lowest: the lowest recorded price for the game given by -id.
//   - history: a price history chart for the game given by -id.
//   - upsert: insert or update the game given by -id.
//   - crawl: crawl the store, optionally limited by -startPage and -endPage.
//   - help: print usage text.
//
// A returned error means the options were invalid, and the caller is expected
// to report it. Most of the underlying commands print their own output and
// only log database failures, so a nil error does not guarantee that the
// operation succeeded.
func LoadRegisters() map[string]types.Command {
	return map[string]types.Command{
		"stats": func(a *types.App, o types.Options) error {
			commands.DisplayStats(a.Games)
			return nil
		},
		"records": func(a *types.App, o types.Options) error {
			// Parse the filter first so a bad -filter fails before any
			// database work. An empty filter matches every game.
			f, err := filter.ParseFilter(o.Filter)
			if err != nil {
				return err
			}
			// A page below 1 is clamped by DisplayRecords, so no check here.
			commands.DisplayRecords(a.Games, o.Page, f)
			return nil
		},
		"lowest": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			commands.SearchLowestPrice(a.Games, o.ID)
			return nil
		},
		"history": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			commands.PriceHistory(a.Games, o.ID)
			return nil
		},
		"upsert": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			game, err := commands.UpsertGame(a.Games, o.ID)
			if err != nil {
				return err
			}
			fmt.Printf("Saved %s (%d)\n", game.Title, game.AppId)
			return nil
		},
		"crawl": func(a *types.App, o types.Options) error {
			// 0 means "not set" for both flags, so only negatives are invalid.
			if o.StartPage < 0 || o.EndPage < 0 {
				return errors.New("-startPage and -endPage must be positive")
			}
			// Only compare when an end page was given; 0 means "to the last page".
			if o.EndPage != 0 && o.StartPage > o.EndPage {
				return fmt.Errorf("-startPage (%d) must be <= -endPage (%d)", o.StartPage, o.EndPage)
			}
			// With both flags at 0 this is the resumable full crawl; with
			// either set, it is a ranged crawl that leaves saved progress alone.
			commands.Crawl(a.Games, a.State, o.StartPage, o.EndPage)
			return nil
		},
		"help": func(a *types.App, o types.Options) error {
			commands.PrintHelp()
			return nil
		},
	}
}
