package games

import (
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type CatalogQuery struct {
	Q, Availability, Sort string
}

func ParseCatalogQuery(raw string) (CatalogQuery, error) {
	query := CatalogQuery{Availability: "ALL", Sort: "RECOMMENDED"}
	values, err := url.ParseQuery(raw)
	if err != nil || !utf8.ValidString(raw) {
		return query, ErrInvalidInput
	}
	for key, value := range values {
		if len(value) != 1 || !utf8.ValidString(value[0]) {
			return query, ErrInvalidInput
		}
		switch key {
		case "q":
			if strings.ContainsFunc(value[0], unicode.IsControl) {
				return query, ErrInvalidInput
			}
			query.Q = strings.TrimSpace(value[0])
			if utf8.RuneCountInString(query.Q) > 128 {
				return query, ErrInvalidInput
			}
		case "availability":
			switch value[0] {
			case "ALL", "PLAY", "MAINTENANCE", "TEMPORARILY_UNAVAILABLE", "COMING_SOON", "RETIRED":
				query.Availability = value[0]
			default:
				return query, ErrInvalidInput
			}
		case "sort":
			if value[0] != "RECOMMENDED" && value[0] != "NAME" {
				return query, ErrInvalidInput
			}
			query.Sort = value[0]
		default:
			return query, ErrInvalidInput
		}
	}
	return query, nil
}

// Apply only to the public, resolved Catalog output; no authorization or runtime decisions live here.
func FilterCatalog(items []CatalogEntry, query CatalogQuery) []CatalogEntry {
	filtered := make([]CatalogEntry, 0, len(items))
	needle := strings.ToLower(query.Q)
	for _, entry := range items {
		if query.Availability != "ALL" && entry.State != query.Availability {
			continue
		}
		if !strings.Contains(strings.ToLower(entry.Title), needle) && !strings.Contains(strings.ToLower(entry.Slug), needle) {
			continue
		}
		filtered = append(filtered, entry)
	}
	if query.Sort == "NAME" {
		sort.SliceStable(filtered, func(i, j int) bool {
			a, b := strings.ToLower(filtered[i].Title), strings.ToLower(filtered[j].Title)
			if a == b {
				return filtered[i].Slug < filtered[j].Slug
			}
			return a < b
		})
	}
	return filtered
}
