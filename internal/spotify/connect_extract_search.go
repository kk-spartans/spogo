package spotify

func extractSearchItems(payload map[string]any, kind string) ([]Item, int) {
	for _, path := range searchPaths(kind) {
		if container, ok := getMap(payload, path...); ok {
			items := extractItemsFromContainer(container, kind)
			total := getInt(container, "totalCount")
			if total == 0 {
				total = len(items)
			}
			return items, total
		}
	}
	items := collectItemsByKind(payload, kind)
	return items, len(items)
}

func extractItemFromPayload(payload map[string]any, kind string) (Item, bool) {
	if kind == "track" {
		if m, ok := getMap(payload, "data", "trackUnion"); ok {
			if item, ok := extractItem(m, kind); ok {
				return item, true
			}
		}
		if m, ok := getMap(payload, "data", "track"); ok {
			if item, ok := extractItem(m, kind); ok {
				return item, true
			}
		}
	}
	items := collectItemsByKind(payload, kind)
	if len(items) == 0 {
		return Item{}, false
	}
	return items[0], true
}

func searchPaths(kind string) [][]string {
	switch kind {
	case "track":
		return [][]string{{"data", "searchV2", "tracksV2"}}
	case "album":
		return [][]string{{"data", "searchV2", "albumsV2"}, {"data", "searchV2", "albums"}}
	case "artist":
		return [][]string{{"data", "searchV2", "artists"}}
	case "playlist":
		return [][]string{{"data", "searchV2", "playlists"}}
	case "show":
		return [][]string{{"data", "searchV2", "podcasts"}, {"data", "searchV2", "shows"}}
	case "episode":
		return [][]string{{"data", "searchV2", "episodes"}}
	default:
		return nil
	}
}

func extractItemsFromContainer(container map[string]any, kind string) []Item {
	itemsRaw, ok := container["items"].([]any)
	if !ok {
		return collectItemsByKind(container, kind)
	}
	items := make([]Item, 0, len(itemsRaw))
	for _, raw := range itemsRaw {
		if item, ok := extractItem(raw, kind); ok {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return collectItemsByKind(container, kind)
	}
	return items
}
