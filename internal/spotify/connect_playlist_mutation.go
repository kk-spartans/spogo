package spotify

import (
	"context"
	"errors"
	"fmt"
)

var errPlaylistNotWritable = errors.New("playlist is not writable")

func (c *ConnectClient) addTracks(ctx context.Context, playlistID string, uris []string) error {
	if err := c.ensurePlaylistWritable(ctx, playlistID); err != nil {
		return err
	}
	_, err := c.graphQL(ctx, "addToPlaylist", map[string]any{
		"playlistUri":      "spotify:playlist:" + playlistID,
		"playlistItemUris": uris,
		"newPosition": map[string]any{
			"moveType": "TOP_OF_PLAYLIST",
			"fromUid":  nil,
		},
	})
	return err
}

func (c *ConnectClient) removeTracks(ctx context.Context, playlistID string, uris []string) error {
	if err := c.ensurePlaylistWritable(ctx, playlistID); err != nil {
		return err
	}
	uids, err := c.playlistTrackUIDs(ctx, playlistID, uris)
	if err != nil {
		return err
	}
	_, err = c.graphQL(ctx, "removeFromPlaylist", map[string]any{
		"playlistUri": "spotify:playlist:" + playlistID,
		"uids":        uids,
	})
	return err
}

func (c *ConnectClient) playlistTrackUIDs(ctx context.Context, playlistID string, uris []string) ([]string, error) {
	if len(uris) == 0 {
		return nil, fmt.Errorf("track uri required")
	}
	need := map[string]int{}
	for _, uri := range uris {
		need[uri]++
	}
	uids := make([]string, 0, len(uris))
	offset := 0
	const limit = 100
	for len(uids) < len(uris) {
		payload, err := c.graphQL(ctx, "fetchPlaylist", playlistTrackVariables(playlistID, limit, offset))
		if err != nil {
			return nil, err
		}
		found, total := extractPlaylistTrackUIDs(payload, need)
		uids = append(uids, found...)
		if total <= 0 || offset+limit >= total {
			break
		}
		offset += limit
	}
	if len(uids) != len(uris) {
		return nil, fmt.Errorf("playlist items not found for removal")
	}
	return uids, nil
}

func extractPlaylistTrackUIDs(payload map[string]any, need map[string]int) ([]string, int) {
	content, ok := getMap(payload, "data", "playlistV2", "content")
	if !ok {
		return nil, 0
	}
	rawItems, _ := content["items"].([]any)
	uids := make([]string, 0)
	for _, raw := range rawItems {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		wrapper, ok := m["itemV2"].(map[string]any)
		if !ok {
			continue
		}
		uid := getString(wrapper, "uid")
		if uid == "" {
			uid = getString(m, "uid")
		}
		dataM, _ := wrapper["data"].(map[string]any)
		uri := playlistTrackURI(dataM)
		if uid == "" || uri == "" || need[uri] <= 0 {
			continue
		}
		need[uri]--
		uids = append(uids, uid)
	}
	return uids, getInt(content, "totalCount")
}

func playlistTrackURI(data map[string]any) string {
	if data == nil {
		return ""
	}
	if uri := getString(data, "uri"); uri != "" {
		return uri
	}
	if track, ok := data["track"].(map[string]any); ok {
		if uri := getString(track, "uri"); uri != "" {
			return uri
		}
	}
	return findFirstURI(data, "track")
}

func (c *ConnectClient) ensurePlaylistWritable(ctx context.Context, playlistID string) error {
	payload, err := c.graphQL(ctx, "playlistPermissions", map[string]any{
		"uri": "spotify:playlist:" + playlistID,
	})
	if err != nil {
		return err
	}
	caps, ok := getMap(payload, "data", "playlistV2", "currentUserCapabilities")
	if !ok {
		return fmt.Errorf("playlist permissions missing")
	}
	if !getBool(caps, "canEditItems") {
		return fmt.Errorf("%w: %s", errPlaylistNotWritable, playlistID)
	}
	return nil
}
