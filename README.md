# go-navi-smart-playlist

Lightweight Go microservice for Navidrome that generates smart playlists from listening behavior and writes them back through the Subsonic-compatible API.

## Features

- Fetches the full music library from Navidrome and builds an in-memory track dataset
- Generates smart playlists with simple scoring logic
- Includes built-in playlists:
  - `Discover Weekly`
  - `Rediscover`
  - `Top This Month`
- Applies a diversity rule of max 2 tracks per artist per playlist
- Creates missing playlists and updates existing ones
- Runs once on startup, then every 24 hours
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

## Installation

Clone the repository and build it locally:

```bash
go build ./cmd/app
```

Or run it with Docker:

```bash
docker compose build
```

## How To Run

### Local

Set the required environment variables and start the service:

```bash
export NAVIDROME_URL=http://localhost:4533
export NAVIDROME_USER=your-user
export NAVIDROME_PASSWORD=your-password
export PLAYLIST_SIZE=50

go run ./cmd/app
```

### Docker Compose

Update the values in [`docker-compose.yml`](/go-navi-smart-playlist/docker-compose.yml), then run:

```bash
docker compose up --build -d
```

The container starts the job immediately, then refreshes playlists every 24 hours.

## Dry Run

To preview generated playlists without modifying Navidrome:

```
DRY_RUN=true NAVIDROME_URL=http://192.168.0.25:4533 NAVIDROME_USER=user-name NAVIDROME_PASSWORD=your-password go run ./cmd/app
```

This logs playlist names and track IDs instead of creating or updating playlists.

## Notes

- The service keeps all data in memory and does not use a database
- It is designed for small-to-medium personal libraries, around a few thousand tracks
- For full collection and safe playlist replacement, it uses `getAlbum` and `getPlaylist` in addition to the main playlist and album list endpoints
