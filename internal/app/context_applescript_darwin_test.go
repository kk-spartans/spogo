//go:build darwin
// +build darwin

package app

import (
	"testing"

	"github.com/steipete/spogo/internal/config"
	"github.com/steipete/spogo/internal/cookies"
)

func TestNewAppleScriptClientDarwin(t *testing.T) {
	ctx := &Context{Profile: config.Profile{Engine: "applescript"}}
	client, err := ctx.newAppleScriptClient(cookies.FileSource{Path: "/tmp/missing-spogo-cookies.json"})
	if err != nil {
		t.Fatalf("new applescript client: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client")
	}
}
