package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

const pathfinderURL = "https://api-partner.spotify.com/pathfinder/v1/query"

func (c *ConnectClient) graphQL(ctx context.Context, operation string, variables map[string]any) (map[string]any, error) {
	if c.session == nil {
		return nil, errors.New("connect client not initialized")
	}
	auth, err := c.session.auth(ctx)
	if err != nil {
		return nil, err
	}
	hash, err := c.hashes.Hash(ctx, operation)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("operationName", operation)
	if variables == nil {
		variables = map[string]any{}
	}
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, err
	}
	extensionsJSON, err := json.Marshal(map[string]any{
		"persistedQuery": map[string]any{
			"version":    1,
			"sha256Hash": hash,
		},
	})
	if err != nil {
		return nil, err
	}
	params.Set("variables", string(variablesJSON))
	params.Set("extensions", string(extensionsJSON))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pathfinderURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	applyRequestHeaders(req, requestHeaders{
		AccessToken:   auth.AccessToken,
		ClientToken:   auth.ClientToken,
		ClientVersion: auth.ClientVersion,
		Accept:        "application/json",
		Language:      c.language,
		AppPlatform:   defaultSpotifyAppPlatform,
	})
	client := c.searchClient
	if client == nil {
		client = c.client
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apiErrorFromResponse(resp)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if err := pathfinderError(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func pathfinderError(payload map[string]any) error {
	errorsValue, ok := payload["errors"]
	if !ok {
		return nil
	}
	list, ok := errorsValue.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	first, ok := list[0].(map[string]any)
	if !ok {
		return errors.New("pathfinder error")
	}
	message, _ := first["message"].(string)
	if message == "" {
		message = "pathfinder error"
	}
	return errors.New(message)
}
