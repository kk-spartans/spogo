package spotify

import (
	"context"
	"errors"
	"strings"
)

func (c *ConnectClient) search(ctx context.Context, kind, query string, limit, offset int) (SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return SearchResult{}, errors.New("query required")
	}
	limit = normalizeSearchLimit(limit)
	offset = normalizeOffset(offset)
	payload, err := c.graphQL(ctx, "searchDesktop", searchVariables(query, limit, offset))
	if err != nil {
		fallback, ferr := c.searchViaWeb(ctx, kind, query, limit, offset)
		if ferr == nil {
			return fallback, nil
		}
		return SearchResult{}, ferr
	}
	items, total := extractSearchItems(payload, kind)
	return SearchResult{Type: kind, Limit: limit, Offset: offset, Total: total, Items: items}, nil
}

func (c *ConnectClient) searchViaWeb(ctx context.Context, kind, query string, limit, offset int) (SearchResult, error) {
	return c.searchViaWebAPI(ctx, kind, query, limit, offset)
}

func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func searchVariables(query string, limit, offset int) map[string]any {
	return map[string]any{
		"searchTerm":                    query,
		"offset":                        offset,
		"limit":                         limit,
		"numberOfTopResults":            5,
		"includeAudiobooks":             true,
		"includePreReleases":            true,
		"includeLocalConcertsField":     false,
		"includeArtistHasConcertsField": false,
	}
}
