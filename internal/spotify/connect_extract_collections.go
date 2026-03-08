package spotify

// extractLibraryV3Items navigates the specific libraryV3 response path
// data.me.libraryV3.items[i].item.data to extract items of the given kind.
// Using a targeted path avoids the duplicates and fake sort-category entries
// that a full recursive walk would produce.
func extractLibraryV3Items(payload map[string]any, kind string) ([]Item, int) {
	lib, ok := getMap(payload, "data", "me", "libraryV3")
	if !ok {
		return nil, 0
	}
	return extractWrappedCollectionItems(lib, "items", "item", "data", "totalCount", kind)
}

func extractPlaylistContentItems(payload map[string]any, kind string) ([]Item, int) {
	content, ok := getMap(payload, "data", "playlistV2", "content")
	if !ok {
		return nil, 0
	}
	return extractWrappedCollectionItems(content, "items", "itemV2", "data", "totalCount", kind)
}

func extractWrappedCollectionItems(container map[string]any, itemsKey, wrapperKey, dataKey, totalKey, kind string) ([]Item, int) {
	rawItems, _ := container[itemsKey].([]any)
	items := make([]Item, 0, len(rawItems))
	seen := map[string]struct{}{}
	for _, raw := range rawItems {
		dataM, ok := extractWrappedData(raw, wrapperKey, dataKey)
		if !ok {
			continue
		}
		item, ok := extractItem(dataM, kind)
		if !ok {
			continue
		}
		if _, dup := seen[item.URI]; dup {
			continue
		}
		seen[item.URI] = struct{}{}
		items = append(items, item)
	}
	total := getInt(container, totalKey)
	if total == 0 {
		total = len(items)
	}
	return items, total
}

func extractWrappedData(raw any, wrapperKey, dataKey string) (map[string]any, bool) {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	wrapper, ok := m[wrapperKey].(map[string]any)
	if !ok {
		return nil, false
	}
	dataM, ok := wrapper[dataKey].(map[string]any)
	return dataM, ok
}
