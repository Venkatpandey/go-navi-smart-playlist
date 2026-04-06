package playlist

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/navidrome"
)

type playlistClient interface {
	GetPlaylists(ctx context.Context) ([]navidrome.Playlist, error)
	GetPlaylistSongCount(ctx context.Context, playlistID string) (int, error)
	CreatePlaylist(ctx context.Context, name string, songIDs []string) error
	UpdatePlaylist(ctx context.Context, playlistID string, removeCount int, songIDs []string) error
}

type Writer struct {
	client playlistClient
	logger *log.Logger
	dryRun bool
}

func NewWriter(client playlistClient, logger *log.Logger, dryRun bool) *Writer {
	return &Writer{
		client: client,
		logger: logger,
		dryRun: dryRun,
	}
}

func (w *Writer) Upsert(ctx context.Context, name string, tracks []model.Track) error {
	songIDs := extractSongIDs(tracks)
	if len(songIDs) == 0 {
		w.logger.Printf("playlist %q has no tracks, skipping write", name)
		return nil
	}

	if w.dryRun {
		w.logger.Printf("dry-run playlist %q (%d tracks): %s", name, len(songIDs), strings.Join(songIDs, ","))
		return nil
	}

	playlists, err := w.client.GetPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("get playlists: %w", err)
	}

	existingIndex := slices.IndexFunc(playlists, func(item navidrome.Playlist) bool {
		return strings.EqualFold(item.Name, name)
	})

	if existingIndex >= 0 {
		removeCount := playlists[existingIndex].SongCount
		if removeCount == 0 {
			var countErr error
			removeCount, countErr = w.client.GetPlaylistSongCount(ctx, playlists[existingIndex].ID)
			if countErr != nil {
				return fmt.Errorf("get playlist %q details: %w", name, countErr)
			}
		}

		if err := w.client.UpdatePlaylist(ctx, playlists[existingIndex].ID, removeCount, songIDs); err != nil {
			return fmt.Errorf("update playlist %q: %w", name, err)
		}

		w.logger.Printf("updated playlist %q with %d tracks", name, len(songIDs))
		return nil
	}

	if err := w.client.CreatePlaylist(ctx, name, songIDs); err != nil {
		return fmt.Errorf("create playlist %q: %w", name, err)
	}

	w.logger.Printf("created playlist %q with %d tracks", name, len(songIDs))
	return nil
}

func extractSongIDs(tracks []model.Track) []string {
	songIDs := make([]string, 0, len(tracks))
	for _, track := range tracks {
		if track.ID == "" {
			continue
		}

		songIDs = append(songIDs, track.ID)
	}

	return songIDs
}
