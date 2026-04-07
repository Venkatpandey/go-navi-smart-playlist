# go-navi-smart-playlist

Lightweight Go microservice for Navidrome that generates smart playlists from listening behavior and writes them back through the Subsonic-compatible API.

## Features

- Fetches the full music library from Navidrome and builds an in-memory track dataset
- Generates smart playlists with simple scoring logic
- Includes built-in playlists:
  - `Discover Weekly`
  - `Rediscover`
  - `Top This Month`
  - `Hidden Gems`
  - `Long Time No See`
  - `Comfort Shuffle`
  - `More Like Hidden Gems`
  - `Artist Adjacent Comfort`
- Persists a tiny local state cache to improve future recommendations
- Uses derived features and lightweight vector similarity for better ranking
- Applies diversity rules with caps per artist and album
- Creates missing playlists and updates existing ones
- Runs once on startup, then every 7 days
- Supports `DRY_RUN=true` to preview playlists without writing changes
- Uses only the Go standard library

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
- `DRY_RUN` default: `false`
- `RUN_TIMEOUT` default: `15m`
- `SCORE_WEIGHT_PLAYCOUNT` default: `1.0`
- `SCORE_WEIGHT_RECENCY` default: `2.0`
- `SCORE_WEIGHT_FRESHNESS` default: `1.5`
- `SCORE_DECAY_DAYS` default: `45`
- `ENABLE_STATE_CACHE` default: `true`
- `STATE_FILE` default: `/tmp/go-smart-playlist/state.json`
- `STATE_DIR` optional alternative to `STATE_FILE`
- `MIN_CANDIDATE_BACKFILL` default: `20`

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
      DRY_RUN: "false"
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
      DRY_RUN: "false"
      ENABLE_STATE_CACHE: "true"
      STATE_FILE: /data/smart-playlist/bob/state.json
    volumes:
      - /volume1/docker/navidrome/data:/data
```

This avoids cache collisions because each user writes to a different JSON state file. Playlist names can stay the same because they are created under different Navidrome user accounts.


```

## Dry Run

To preview generated playlists without modifying Navidrome:

```
DRY_RUN=true NAVIDROME_URL=http://192.168.0.25:4533 NAVIDROME_USER=user-name NAVIDROME_PASSWORD=your-password go run ./cmd/app
```

This logs playlist names and track IDs instead of creating or updating playlists.

## Notes

- The service keeps all data in memory and does not use a database
- A small JSON state file can be persisted to improve recommendations over time
- A good default cache path is `/data/smart-playlist/state.json` when `/data` is already mapped to your Navidrome host storage
- It is designed for small-to-medium personal libraries, around a few thousand tracks
- Recommendation quality improves as Navidrome accumulates more `playCount` and `last played` history
- For full collection and safe playlist replacement, it uses `getAlbum` and `getPlaylist` in addition to the main playlist and album list endpoints
