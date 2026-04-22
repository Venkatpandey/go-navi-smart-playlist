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
- Single-user mode:
  - `NAVIDROME_URL`
  - `NAVIDROME_USER`
  - `NAVIDROME_PASSWORD`
- Multi-user mode:
  - `NAVIDROME_URL`
  - `NAVIDROME_ADMIN_USER`
  - `NAVIDROME_ADMIN_PASSWORD`
  - `MULTI_USER_CONFIG_FILE`
  - `STATE_DIR`

## Configuration

Required for single-user mode:

- `NAVIDROME_URL`
- `NAVIDROME_USER`
- `NAVIDROME_PASSWORD`

Required for multi-user mode:

- `NAVIDROME_URL`
- `MULTI_USER_ENABLED=true`
- `NAVIDROME_ADMIN_USER`
- `NAVIDROME_ADMIN_PASSWORD`
- `MULTI_USER_CONFIG_FILE`
- `STATE_DIR`

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

`STATE_FILE` and `STATE_DIR` behavior:

- single-user mode: use `STATE_FILE` or `STATE_DIR`
- multi-user mode: use `STATE_DIR` only
- multi-user per-user cache path: `<STATE_DIR>/<username>/state.json`

Multi-user config file format:

```json
{
  "users": [
    { "username": "alice", "password": "alice-pass", "enabled": true },
    { "username": "bob", "password": "bob-pass" }
  ]
}
```

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

Single-user example:

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

Multi-user mode can manage multiple Navidrome users from one container.

How it works:

- logs into Navidrome native API with admin credentials
- discovers users from `GET /api/user`
- matches discovered users against mounted JSON credentials
- refreshes users serially, `1by1`
- continues after per-user failures
- returns partial failure if any user failed or had no enabled credentials entry

Important:

- each discovered user must exist in `MULTI_USER_CONFIG_FILE`
- disabled users in Navidrome are skipped when exposed by native API
- credentials entries not found in discovery are ignored with a warning
- native API discovery is undocumented and may change across Navidrome versions

Example `docker-compose.yml`:

```yaml
services:
  smart-playlist:
    image: ghcr.io/venkatpandey/go-navi-smart-playlist:latest
    container_name: smart-playlist
    restart: unless-stopped
    environment:
      NAVIDROME_URL: http://navidrome:4533
      MULTI_USER_ENABLED: "true"
      NAVIDROME_ADMIN_USER: admin
      NAVIDROME_ADMIN_PASSWORD: admin-password
      MULTI_USER_CONFIG_FILE: /run/secrets/navidrome-users.json
      PLAYLIST_SIZE: "50"
      DRY_RUN: "false"
      ENABLE_STATE_CACHE: "true"
      STATE_DIR: /data/smart-playlist
    volumes:
      - /volume1/docker/navidrome/data:/data
      - ./secrets/navidrome-users.json:/run/secrets/navidrome-users.json:ro
```

Example `navidrome-users.json`:

```json
{
  "users": [
    { "username": "alice", "password": "alice-password" },
    { "username": "bob", "password": "bob-password" }
  ]
}
```

Per-user state files land at:

- `/data/smart-playlist/alice/state.json`
- `/data/smart-playlist/bob/state.json`

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
- Multi-user mode uses undocumented Navidrome native endpoints only for user discovery
- It is designed for small-to-medium personal libraries, around a few thousand tracks
- Recommendation quality improves as Navidrome accumulates more `playCount` and `last played` history
- For full collection and safe playlist replacement, it uses `getAlbum` and `getPlaylist` in addition to the main playlist and album list endpoints
