package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// FilterHelp lists the filter keys ParseFilter accepts. It is used in the
// "unknown filter" error and is suitable for command-line help text.
const FilterHelp = "sale, free, under:<price>, over:<price>, discount:<min%>, name:<text>, prefix:<text>"

// ParseFilter turns a comma-separated filter spec such as "sale,under:10" into
// a MongoDB filter for the games collection.
//
// Each part is either a bare key or key:value. Keys are case-insensitive:
//
//   - sale: the game is currently discounted (non-zero discount).
//   - free: the current price is exactly 0.
//   - under:N: current price is above 0 and at most N, so free listings are
//     excluded.
//   - over:N: current price is at least N.
//   - discount:N: the discount is at least N percent, whether stored as 75 or
//     -75.
//   - name:TEXT: the title contains TEXT, ignoring case.
//   - prefix:TEXT: the title starts with TEXT, ignoring case.
//
// All parts must match (they are combined with AND). Prices are compared
// against the current price_snapshot.discount_price. Text values are matched
// literally, not as regular expressions.
//
// An empty or all-whitespace spec returns an empty filter that matches every
// game. It returns an error for an unknown key, a missing or non-numeric
// value where one is required, or empty text for name and prefix.
func ParseFilter(spec string) (bson.M, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return bson.M{}, nil
	}

	var clauses []bson.M
	for _, part := range strings.Split(spec, ",") {
		// Cut on the first ":" only, so values may themselves contain
		// colons (e.g. name:half-life: alyx). A bare key leaves val empty.
		key, val, _ := strings.Cut(strings.TrimSpace(part), ":")
		key = strings.ToLower(key)

		// num parses val as a number for the current key, with an error
		// message that shows the expected syntax.
		num := func() (float64, error) {
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return 0, fmt.Errorf("filter %q needs a number, e.g. %s:10", key, key)
			}
			return n, nil
		}

		switch key {
		case "sale":
			// Same definition of "on sale" as DisplayStats: any non-zero discount.
			clauses = append(clauses, bson.M{"price_snapshot.discount": bson.M{"$ne": 0}})
		case "free":
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": 0})
		case "under":
			n, err := num()
			if err != nil {
				return nil, err
			}
			// $gt 0 keeps free/unpriced listings out of "under" results
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": bson.M{"$gt": 0, "$lte": n}})
		case "over":
			n, err := num()
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, bson.M{"price_snapshot.discount_price": bson.M{"$gte": n}})
		case "discount":
			n, err := num()
			if err != nil {
				return nil, err
			}
			// $abs makes this work whether discounts are stored as 75 or -75.
			// $expr is needed to apply $abs to a field inside a match.
			clauses = append(clauses, bson.M{"$expr": bson.M{
				"$gte": bson.A{bson.M{"$abs": "$price_snapshot.discount"}, n},
			}})
		case "prefix":
			if val == "" {
				return nil, fmt.Errorf("filter \"prefix\" needs text, e.g. prefix:half")
			}
			// QuoteMeta escapes regex characters so user input is literal;
			// "^" anchors the match to the start of the title.
			clauses = append(clauses, bson.M{"title": bson.M{
				"$regex": "^" + regexp.QuoteMeta(val), "$options": "i",
			}})
		case "name":
			if val == "" {
				return nil, fmt.Errorf("filter \"name\" needs text, e.g. name:dark")
			}
			clauses = append(clauses, bson.M{"title": bson.M{
				"$regex": regexp.QuoteMeta(val), "$options": "i",
			}})
		default:
			return nil, fmt.Errorf("unknown filter %q (available: %s)", key, FilterHelp)
		}
	}

	// Avoid wrapping a single clause in a needless $and.
	if len(clauses) == 1 {
		return clauses[0], nil
	}
	return bson.M{"$and": clauses}, nil
}
