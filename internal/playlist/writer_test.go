package playlist

import (
	"context"
	"log"
	"testing"

	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/navidrome"
)

type fakePlaylistClient struct {
	playlists       []navidrome.Playlist
	detailCount     int
	createdName     string
	createdIDs      []string
	updatedID       string
	updatedRemove   int
	updatedSongIDs  []string
	getDetailsCalls int
}

func (f *fakePlaylistClient) GetPlaylists(context.Context) ([]navidrome.Playlist, error) {
	return f.playlists, nil
}

func (f *fakePlaylistClient) GetPlaylistSongCount(context.Context, string) (int, error) {
	f.getDetailsCalls++
	return f.detailCount, nil
}

func (f *fakePlaylistClient) CreatePlaylist(_ context.Context, name string, songIDs []string) error {
	f.createdName = name
	f.createdIDs = append([]string(nil), songIDs...)
	return nil
}

func (f *fakePlaylistClient) UpdatePlaylist(_ context.Context, playlistID string, removeCount int, songIDs []string) error {
	f.updatedID = playlistID
	f.updatedRemove = removeCount
	f.updatedSongIDs = append([]string(nil), songIDs...)
	return nil
}

func TestWriterClearsExistingPlaylistWhenGenerationIsEmpty(t *testing.T) {
	client := &fakePlaylistClient{
		playlists: []navidrome.Playlist{{ID: "mine", Name: "Fresh & Unplayed", Owner: "alice", SongCount: 3}},
	}
	writer := NewWriter(client, log.New(testWriter{t}, "", 0), "alice")

	if err := writer.Upsert(context.Background(), "Fresh & Unplayed", nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if client.updatedID != "mine" || client.updatedRemove != 3 || len(client.updatedSongIDs) != 0 {
		t.Fatalf("expected playlist clear, got %+v", client)
	}
}

func TestWriterDoesNotUpdateAnotherOwnersPublicPlaylist(t *testing.T) {
	client := &fakePlaylistClient{
		playlists: []navidrome.Playlist{{ID: "theirs", Name: "Deep Cuts", Owner: "bob", Public: true, SongCount: 4}},
	}
	writer := NewWriter(client, log.New(testWriter{t}, "", 0), "alice")

	if err := writer.Upsert(context.Background(), "Deep Cuts", []model.Track{{ID: "song-1"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if client.updatedID != "" {
		t.Fatalf("updated another user's playlist %q", client.updatedID)
	}
	if client.createdName != "Deep Cuts" || len(client.createdIDs) != 1 {
		t.Fatalf("expected own playlist creation, got %+v", client)
	}
}

func TestWriterPrefersMatchingOwner(t *testing.T) {
	client := &fakePlaylistClient{
		playlists: []navidrome.Playlist{
			{ID: "theirs", Name: "Quick Mix", Owner: "bob", Public: true, SongCount: 4},
			{ID: "mine", Name: "Quick Mix", Owner: "alice", SongCount: 2},
		},
	}
	writer := NewWriter(client, log.New(testWriter{t}, "", 0), "alice")

	if err := writer.Upsert(context.Background(), "Quick Mix", []model.Track{{ID: "song-1"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if client.updatedID != "mine" || client.updatedRemove != 2 {
		t.Fatalf("expected own playlist update, got %+v", client)
	}
}
