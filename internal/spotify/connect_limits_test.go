package spotify

import "testing"

func TestNormalizeConnectLimits(t *testing.T) {
	if got := normalizeLibraryLimit(0); got != 50 {
		t.Fatalf("library default = %d", got)
	}
	if got := normalizeLibraryLimit(7); got != 7 {
		t.Fatalf("library custom = %d", got)
	}
	if got := normalizePlaylistTrackLimit(0); got != 25 {
		t.Fatalf("playlist default = %d", got)
	}
	if got := normalizePlaylistTrackLimit(9); got != 9 {
		t.Fatalf("playlist custom = %d", got)
	}
	if got := normalizeSearchLimit(0); got != 10 {
		t.Fatalf("search default = %d", got)
	}
	if got := normalizeSearchLimit(4); got != 4 {
		t.Fatalf("search custom = %d", got)
	}
	if got := normalizeOffset(-1); got != 0 {
		t.Fatalf("offset default = %d", got)
	}
	if got := normalizeOffset(3); got != 3 {
		t.Fatalf("offset custom = %d", got)
	}
}
