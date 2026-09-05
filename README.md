# go-navi-smart-playlist

[![Publish Docker Image](https://github.com/Venkatpandey/go-navi-smart-playlist/actions/workflows/publish.yml/badge.svg)](https://github.com/Venkatpandey/go-navi-smart-playlist/actions/workflows/publish.yml)
[![Latest Tag](https://img.shields.io/github/v/tag/Venkatpandey/go-navi-smart-playlist?label=release&sort=semver)](https://github.com/Venkatpandey/go-navi-smart-playlist/releases)
[![Release Date](https://img.shields.io/github/release-date/Venkatpandey/go-navi-smart-playlist)](https://github.com/Venkatpandey/go-navi-smart-playlist/releases)
[![Go Version](https://img.shields.io/badge/go-1.24+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/github/license/Venkatpandey/go-navi-smart-playlist)](https://github.com/Venkatpandey/go-navi-smart-playlist/blob/main/LICENSE)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/Venkatpandey/go-navi-smart-playlist/blob/main/LICENSE)


Lightweight Go microservice for Navidrome that generates smart playlists from listening behavior and writes them back through the Subsonic-compatible API.

## Features

- Fetches the full music library from Navidrome and builds an in-memory track dataset
- Generates smart playlists with recipe-specific eligibility, scoring, rotation, and diversity rules
- Includes built-in playlists:
  - `Discover Weekly`
  - `Rediscover`
  - `Top This Month`
  - `Hidden Gems`
  - `Long Time No See`
  - `Comfort Shuffle`
  - `More Like Hidden Gems`
  - `Artist Adjacent Comfort`
  - `Fresh & Unplayed`
  - `Forgotten Favorites`
  - `Rising This Week`
  - `Deep Cuts`
  - `Quick Mix`
  - `Longform`
- Persists a tiny local state cache to improve future recommendations
- Uses derived features and lightweight vector similarity for better ranking
- Applies diversity rules with caps per artist and album
- Applies playlist-specific eligibility rules so each playlist keeps its intended meaning
- Uses deterministic weekly variation to rotate discovery and shuffle playlists
- Uses genre matches when available and duration metadata for session-length playlists
- Creates missing playlists and updates existing ones
- Runs once on startup, then every 7 days
- Uses only the Go standard library

## Playlist Catalog

- `Discover Weekly`: low-play, unplayed, or recently added tracks with weekly rotation
- `Rediscover`: tracks played before, but not during the last 45 days
- `Top This Month`: tracks played during the last 31 days, weighted toward rising play counts
- `Hidden Gems`: low-play tracks, plus highly rated or starred exceptions
- `Long Time No See`: previously played tracks absent for at least 120 days
- `Comfort Shuffle`: familiar favorites with stronger weekly variation
- `More Like Hidden Gems`: behaviorally and genre-adjacent tracks, excluding every source track
- `Artist Adjacent Comfort`: comfort-adjacent tracks from different artists
- `Fresh & Unplayed`: tracks added during the last 180 days and never played
- `Forgotten Favorites`: starred, highly rated, or historically popular tracks absent for at least 180 days
- `Rising This Week`: tracks with new plays during the current collection interval and activity in the last 14 days
- `Deep Cuts`: low-play tracks from artists with strong listening history
- `Quick Mix`: tracks no longer than four minutes
- `Longform`: tracks at least eight minutes long

## Project Layout

```text
cmd/app/main.go
internal/config
internal/model
internal/navidrome
internal/collector
internal/scoring
internal/playlist
```

## Requirements

- Go 1.24+
- A reachable Navidrome instance
- Subsonic API access with:
  - `NAVIDROME_URL`
  - `NAVIDROME_USER`
  - `NAVIDROME_PASSWORD`

## Configuration

Required:

- `NAVIDROME_URL`
- `NAVIDROME_USER`
- `NAVIDROME_PASSWORD`

Optional:

- `PLAYLIST_SIZE` default: `50`
- `ALBUM_PAGE_SIZE` default: `200`
- `RUN_TIMEOUT` default: `15m`
- `SCORE_WEIGHT_PLAYCOUNT` default: `1.0`
- `SCORE_WEIGHT_RECENCY` default: `2.0`
- `SCORE_WEIGHT_FRESHNESS` default: `1.5`
- `SCORE_DECAY_DAYS` default: `45`
- `ENABLE_STATE_CACHE` default: `true`
- `STATE_FILE` default: `/tmp/go-smart-playlist/state.json`
- `STATE_DIR` optional alternative to `STATE_FILE`
- `MIN_CANDIDATE_BACKFILL` default: `20`

### Recommendation Weight Tuning

The score weights provide coarse global tuning for playlists that use the shared base score:

- `SCORE_WEIGHT_PLAYCOUNT`: increase to favor familiar and frequently played tracks; decrease to favor exploration
- `SCORE_WEIGHT_RECENCY`: increase to favor tracks played recently
- `SCORE_WEIGHT_FRESHNESS`: increase to favor tracks added to the library recently
- `SCORE_DECAY_DAYS`: controls how long recency and freshness remain influential; larger values create a wider time window

All weight values must be finite and non-negative. `SCORE_DECAY_DAYS` must be positive. The signals multiplied by these weights are normalized, so changing a weight has a predictable bounded effect.

Balanced defaults:

```yaml
environment:
  SCORE_WEIGHT_PLAYCOUNT: "1.0"
  SCORE_WEIGHT_RECENCY: "2.0"
  SCORE_WEIGHT_FRESHNESS: "1.5"
  SCORE_DECAY_DAYS: "45"
```

More discovery and recently added music:

```yaml
environment:
  SCORE_WEIGHT_PLAYCOUNT: "0.5"
  SCORE_WEIGHT_RECENCY: "1.2"
  SCORE_WEIGHT_FRESHNESS: "2.2"
  SCORE_DECAY_DAYS: "60"
```

More familiar and recently played music:

```yaml
environment:
  SCORE_WEIGHT_PLAYCOUNT: "1.7"
  SCORE_WEIGHT_RECENCY: "2.3"
  SCORE_WEIGHT_FRESHNESS: "0.8"
  SCORE_DECAY_DAYS: "60"
```

These settings affect the original recommendation playlists, similarity playlists, `Quick Mix`, and `Longform`. They do not change the dedicated formulas or eligibility rules for `Fresh & Unplayed`, `Forgotten Favorites`, `Rising This Week`, or `Deep Cuts`.

Keep `ENABLE_STATE_CACHE=true` for useful play-count deltas, stability, and playlist history. Evaluate a tuning change over at least two generation cycles because the first run has no previous snapshot. Each run writes the newly generated playlists to Navidrome.

## Installation

Clone the repository and build it locally:

```bash
go build ./cmd/app
```

For NAS deployment, the recommended path is:

1. publish a public image to GHCR
2. copy `docker-compose.yml` to the NAS
3. edit the image name and Navidrome credentials directly in the compose file
4. run `docker compose pull && docker compose up -d`

## How To Run

### Local

Set the required environment variables and start the service:

```bash
export NAVIDROME_URL=http://navidrome:4533
export NAVIDROME_USER=your-user
export NAVIDROME_PASSWORD=your-password
export PLAYLIST_SIZE=50

go run ./cmd/app
```

### Docker Compose

The included compose file is set up for image-based deployment from GHCR. Edit these values directly in [`docker-compose.yml`](/go-navi-smart-playlist/docker-compose.yml):

```yaml
image: ghcr.io/venkatpandey/go-navi-smart-playlist:latest
environment:
  NAVIDROME_URL: http://navidrome:4533
  NAVIDROME_USER: your-user
  NAVIDROME_PASSWORD: your-password
```

Update the values in [`docker-compose.yml`](/go-navi-smart-playlist/docker-compose.yml), then run:

```bash
docker compose pull
docker compose up -d
```

The container starts the job immediately, then refreshes playlists every 7 days.

To preserve recommendation state across container restarts, point `STATE_FILE` at a dedicated subfolder inside your Navidrome data path. Example:

```yaml
environment:
  STATE_FILE: /data/smart-playlist/state.json
volumes:
  - /volume1/docker/navidrome/data:/data
```

This keeps the cache isolated under `/vol1/docker/navidrome/data/smart-playlist/` while still reusing your existing storage mount.

## Multi-User Support

The current support model is:

- one Navidrome user per service instance
- one container per user
- one separate `STATE_FILE` per user

This works today without code changes. The app does not yet support multiple Navidrome users inside a single container.

Important:

- each container must use a different `NAVIDROME_USER`
- each container must use a different `NAVIDROME_PASSWORD`
- each container must use a different `STATE_FILE`

Example with two users:

```yaml
services:
  smart-playlist-alice:
    image: ghcr.io/venkatpandey/go-navi-smart-playlist:latest
    container_name: smart-playlist-alice
    restart: unless-stopped
    environment:
      NAVIDROME_URL: http://navidrome:4533
      NAVIDROME_USER: alice
      NAVIDROME_PASSWORD: alice-password
      PLAYLIST_SIZE: "50"
      ENABLE_STATE_CACHE: "true"
      STATE_FILE: /data/smart-playlist/alice/state.json
    volumes:
      - /volume1/docker/navidrome/data:/data

  smart-playlist-bob:
    image: ghcr.io/venkatpandey/go-navi-smart-playlist:latest
    container_name: smart-playlist-bob
    restart: unless-stopped
    environment:
      NAVIDROME_URL: http://navidrome:4533
      NAVIDROME_USER: bob
      NAVIDROME_PASSWORD: bob-password
      PLAYLIST_SIZE: "50"
      ENABLE_STATE_CACHE: "true"
      STATE_FILE: /data/smart-playlist/bob/state.json
    volumes:
      - /volume1/docker/navidrome/data:/data
```

This avoids cache collisions because each user writes to a different JSON state file. Playlist names can stay the same because they are created under different Navidrome user accounts.

## Notes

- The service keeps all data in memory and does not use a database
- A small JSON state file can be persisted to improve recommendations over time
- A good default cache path is `/data/smart-playlist/state.json` when `/data` is already mapped to your Navidrome host storage
- It is designed for small-to-medium personal libraries, around a few thousand tracks
- Recommendation quality improves as Navidrome accumulates more `playCount` and `last played` history
- For full collection and safe playlist replacement, it uses `getAlbum` and `getPlaylist` in addition to the main playlist and album list endpoints
