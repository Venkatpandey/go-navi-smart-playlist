package collector

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/navidrome"
)

type albumReader interface {
	GetAlbumList2(ctx context.Context, size, offset int) ([]navidrome.Album, error)
	GetAlbum(ctx context.Context, albumID string) ([]navidrome.TrackPayload, error)
}

type Collector struct {
	client   albumReader
	pageSize int
	logger   *log.Logger
}

func New(client albumReader, logger *log.Logger) *Collector {
	return NewWithPageSize(client, 200, logger)
}

func NewWithPageSize(client albumReader, pageSize int, logger *log.Logger) *Collector {
	if pageSize <= 0 {
		pageSize = 200
	}

	return &Collector{
		client:   client,
		pageSize: pageSize,
		logger:   logger,
	}
}

func (c *Collector) Collect(ctx context.Context) ([]model.Track, error) {
	var tracks []model.Track

	for offset := 0; ; offset += c.pageSize {
		albums, err := c.client.GetAlbumList2(ctx, c.pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("fetch album page at offset %d: %w", offset, err)
		}

		if len(albums) == 0 {
			break
		}

		for _, album := range albums {
			songs, err := c.client.GetAlbum(ctx, album.ID)
			if err != nil {
				return nil, fmt.Errorf("fetch album %s: %w", album.ID, err)
			}

			for _, song := range songs {
				if song.ID == "" {
					continue
				}

				tracks = append(tracks, normalizeTrack(song))
			}
		}

		c.logger.Printf("fetched %d albums, %d total tracks so far", len(albums), len(tracks))

		if len(albums) < c.pageSize {
			break
		}
	}

	return tracks, nil
}

func normalizeTrack(input navidrome.TrackPayload) model.Track {
	return model.Track{
		ID:         input.ID,
		Title:      safeString(input.Title, "Unknown Title"),
		Artist:     safeString(input.Artist, "Unknown Artist"),
		Album:      safeString(input.Album, "Unknown Album"),
		PlayCount:  max(input.PlayCount, 0),
		LastPlayed: parseTime(input.Played),
		Created:    parseTime(input.Created),
		Rating:     max(input.UserRating, 0),
		Starred:    input.Starred != "",
	}
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
	}

	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return parsed.UTC()
		}
	}

	return time.Time{}
}

func safeString(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}

	return trimmed
}

func max(value, minValue int) int {
	if value < minValue {
		return minValue
	}

	return value
}
