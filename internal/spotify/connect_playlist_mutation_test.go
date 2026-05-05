package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestConnectAddTracksUsesPathfinderMutation(t *testing.T) {
	var variables map[string]any
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Query().Get("operationName") {
		case "playlistPermissions":
			return jsonResponse(http.StatusOK, playlistWritablePayload(true)), nil
		case "addToPlaylist":
			if err := json.Unmarshal([]byte(req.URL.Query().Get("variables")), &variables); err != nil {
				t.Fatalf("variables: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{"data": map[string]any{"addToPlaylist": true}}), nil
		default:
			return textResponse(http.StatusNotFound, "missing"), nil
		}
	})
	client := newConnectClientForTests(transport)
	client.hashes.hashes["playlistPermissions"] = "hash"
	client.hashes.hashes["addToPlaylist"] = "hash"

	err := client.AddTracks(context.Background(), "p1", []string{"spotify:track:t1"})
	if err != nil {
		t.Fatalf("add tracks: %v", err)
	}
	if got := getString(variables, "playlistUri"); got != "spotify:playlist:p1" {
		t.Fatalf("playlistUri = %q", got)
	}
	uris, _ := variables["playlistItemUris"].([]any)
	if len(uris) != 1 || uris[0] != "spotify:track:t1" {
		t.Fatalf("playlistItemUris = %#v", variables["playlistItemUris"])
	}
	position, _ := variables["newPosition"].(map[string]any)
	if got := getString(position, "moveType"); got != "TOP_OF_PLAYLIST" {
		t.Fatalf("moveType = %q", got)
	}
}

func TestConnectRemoveTracksUsesResolvedPlaylistUIDs(t *testing.T) {
	operations := []string{}
	var removeVariables map[string]any
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		op := req.URL.Query().Get("operationName")
		operations = append(operations, op)
		switch op {
		case "playlistPermissions":
			return jsonResponse(http.StatusOK, playlistWritablePayload(true)), nil
		case "fetchPlaylist":
			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"playlistV2": map[string]any{"content": map[string]any{
					"totalCount": 2,
					"items": []any{
						map[string]any{"itemV2": map[string]any{
							"uid":  "uid-1",
							"data": map[string]any{"track": map[string]any{"uri": "spotify:track:t1"}},
						}},
						map[string]any{"itemV2": map[string]any{
							"uid":  "uid-2",
							"data": map[string]any{"track": map[string]any{"uri": "spotify:track:t2"}},
						}},
					},
				}}},
			}), nil
		case "removeFromPlaylist":
			if err := json.Unmarshal([]byte(req.URL.Query().Get("variables")), &removeVariables); err != nil {
				t.Fatalf("variables: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{"data": map[string]any{"removeFromPlaylist": true}}), nil
		default:
			return textResponse(http.StatusNotFound, "missing"), nil
		}
	})
	client := newConnectClientForTests(transport)
	client.hashes.hashes["playlistPermissions"] = "hash"
	client.hashes.hashes["fetchPlaylist"] = "hash"
	client.hashes.hashes["removeFromPlaylist"] = "hash"

	err := client.RemoveTracks(context.Background(), "p1", []string{"spotify:track:t2"})
	if err != nil {
		t.Fatalf("remove tracks: %v", err)
	}
	if len(operations) != 3 || operations[0] != "playlistPermissions" || operations[1] != "fetchPlaylist" || operations[2] != "removeFromPlaylist" {
		t.Fatalf("operations = %#v", operations)
	}
	if got := getString(removeVariables, "playlistUri"); got != "spotify:playlist:p1" {
		t.Fatalf("playlistUri = %q", got)
	}
	uids, _ := removeVariables["uids"].([]any)
	if len(uids) != 1 || uids[0] != "uid-2" {
		t.Fatalf("uids = %#v", removeVariables["uids"])
	}
}

func TestConnectRemoveTracksFindsUIDOnLaterPlaylistPage(t *testing.T) {
	fetches := 0
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Query().Get("operationName") {
		case "playlistPermissions":
			return jsonResponse(http.StatusOK, playlistWritablePayload(true)), nil
		case "fetchPlaylist":
			fetches++
			var vars map[string]any
			if err := json.Unmarshal([]byte(req.URL.Query().Get("variables")), &vars); err != nil {
				t.Fatalf("variables: %v", err)
			}
			items := []any{map[string]any{"itemV2": map[string]any{
				"uid":  "uid-other",
				"data": map[string]any{"track": map[string]any{"uri": "spotify:track:other"}},
			}}}
			if getInt(vars, "offset") == 100 {
				items = []any{map[string]any{"itemV2": map[string]any{
					"uid":  "uid-target",
					"data": map[string]any{"track": map[string]any{"uri": "spotify:track:target"}},
				}}}
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"playlistV2": map[string]any{"content": map[string]any{
					"totalCount": 150,
					"items":      items,
				}}},
			}), nil
		case "removeFromPlaylist":
			return jsonResponse(http.StatusOK, map[string]any{"data": map[string]any{"removeFromPlaylist": true}}), nil
		default:
			return textResponse(http.StatusNotFound, "missing"), nil
		}
	})
	client := newConnectClientForTests(transport)
	client.hashes.hashes["playlistPermissions"] = "hash"
	client.hashes.hashes["fetchPlaylist"] = "hash"
	client.hashes.hashes["removeFromPlaylist"] = "hash"

	err := client.RemoveTracks(context.Background(), "p1", []string{"spotify:track:target"})
	if err != nil {
		t.Fatalf("remove tracks: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("fetches = %d", fetches)
	}
}

func TestConnectAddTracksFallsBackToWeb(t *testing.T) {
	webCalled := false
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return textResponse(http.StatusInternalServerError, "fail"), nil
	})
	client := newConnectClientForTests(transport)
	client.hashes.hashes["playlistPermissions"] = "hash"
	client.hashes.hashes["addToPlaylist"] = "hash"
	client.web = mustNewWebClientForPlaylistMutationTest(t, func(w http.ResponseWriter, r *http.Request) {
		webCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.AddTracks(context.Background(), "p1", []string{"spotify:track:t1"})
	if err != nil {
		t.Fatalf("add tracks fallback: %v", err)
	}
	if !webCalled {
		t.Fatalf("expected web fallback")
	}
}

func TestConnectAddTracksRejectsNonWritablePlaylist(t *testing.T) {
	webCalled := false
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("operationName") != "playlistPermissions" {
			return textResponse(http.StatusNotFound, "missing"), nil
		}
		return jsonResponse(http.StatusOK, playlistWritablePayload(false)), nil
	})
	client := newConnectClientForTests(transport)
	client.hashes.hashes["playlistPermissions"] = "hash"
	client.hashes.hashes["addToPlaylist"] = "hash"
	client.web = mustNewWebClientForPlaylistMutationTest(t, func(w http.ResponseWriter, r *http.Request) {
		webCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	err := client.AddTracks(context.Background(), "p1", []string{"spotify:track:t1"})
	if !errors.Is(err, errPlaylistNotWritable) {
		t.Fatalf("expected not writable error, got %v", err)
	}
	if webCalled {
		t.Fatalf("did not expect web fallback")
	}
}

func playlistWritablePayload(writable bool) map[string]any {
	return map[string]any{
		"data": map[string]any{"playlistV2": map[string]any{
			"currentUserCapabilities": map[string]any{"canEditItems": writable},
		}},
	}
}

func mustNewWebClientForPlaylistMutationTest(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	client, closeFn := newTestClient(t, handler)
	t.Cleanup(closeFn)
	return client
}
