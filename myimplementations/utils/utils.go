package utils

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// nextCheck calculates the next ping time from the current time using
// the specified unit.
func NextCheck(from time.Time, amount int64, unit string) time.Time {
	n := int(amount)
	switch unit {
	case "minute":
		return from.Add(time.Duration(n) * time.Minute)
	case "hour":
		return from.Add(time.Duration(n) * time.Hour)
	case "day":
		return from.AddDate(0, 0, n)
	case "week":
		return from.AddDate(0, 0, 7*n)
	case "month":
		return from.AddDate(0, n, 0)
	}
	return from.AddDate(0, 0, 1) // fallback: 1 day
}

// validateInterval verifies that the given amount fits within the specified
// range for the provided unit.
//
// The minimum ping time for the bot is 30 minutes.
func ValidateInterval(amount int64, unit string) error {
	if amount < 1 {
		return fmt.Errorf("the interval must be at least 1")
	}

	switch unit {
	case "minute":
		if amount < 30 {
			return fmt.Errorf("the minimum interval is 30 minutes")
		}
	case "hour", "day", "week", "month":
		// Already covered in the base case above
	default:
		return fmt.Errorf("unknown unit %q", unit)
	}
	return nil
}

func GetUserId(i *discordgo.InteractionCreate) string {
	userID := ""
	if i.Member != nil && i.Member.User != nil {
		userID = i.Member.User.ID
	} else if i.User != nil {
		userID = i.User.ID
	}

	return userID
}
