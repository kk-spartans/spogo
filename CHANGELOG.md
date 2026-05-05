# Changelog

## Unreleased

- Add `auth paste`, wire `--no-input`, and improve cookie diagnostics/cleanup (`#5`, thanks @im-zayan)
- Add `play --shuffle`, Connect library/playlist support, and context-aware Connect play payloads (`#15`, thanks @StandardGage)
- Fix Connect track artist extraction for nested artist containers and minimal artist fragments (`#7`, thanks @joelbdavies)
- Fix silent `auth import` failures by retrying Spotify auth cookie lookup across related hosts and surfacing browser warnings (`#13`)
- Fix `device set` when Connect state has no origin device by falling back to Web API transfer (`#8`)
- Fix Connect liked-track listing via `fetchLibraryTracks` with Web API fallback on payload drift (`#16`, thanks @masonc15)
- Fix Connect play when no device is active by falling back to Web API playback (`#21`, thanks @prashanthbala)
- Fix Connect volume changes by sending the volume endpoint as `PUT` (`#24`, thanks @cavit99)
- Fix sparse status/search metadata so track artists and albums are populated consistently across engines.
- Fix Connect `--device` playback when no device is active without falling back to rate-limited Web API playback.
- Fix `auth paste --no-input` by accepting the documented flag order.
- Fix playlist add/remove 429s by using Connect playlist mutations with writable-playlist checks and fallback coverage across engines (`#20`).

## 0.3.0 - 2026-03-08

- Release prep: bump CLI/spec version to `0.3.0`

## 0.2.0 - 2026-01-07

- Add `applescript` engine for direct Spotify.app control on macOS (thanks @adam91holt)
- CI: bump golangci-lint-action to support golangci-lint v2

## 0.1.0 - 2026-01-02

- Kong-powered CLI with global flags, config profiles, and env overrides
- Auth commands: cookie status/import/clear with browser/profile selection
- Cookie-based auth via steipete/sweetcookie (file cache + browser sources)
- Search tracks/albums/artists/playlists/shows/episodes
- Item info for track/album/artist/playlist/show/episode
- Playback control: play/pause/next/prev/seek/volume/shuffle/repeat/status
- Artist play (top tracks; falls back to search)
- Queue add/show
- Library list/add/remove for tracks/albums; follow/unfollow artists; playlists list
- Playlist management: create/add/remove/track list
- Device list and transfer/set
- Engines: connect (internal), web (Web API), auto (connect → web fallback)
- Rate-limit fallback on 429s where supported
- Output: human color + `--plain` + `--json` (NO_COLOR/TERM aware)
- GitHub Actions CI, linting, formatting, and coverage gate
