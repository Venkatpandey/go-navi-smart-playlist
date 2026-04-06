package audio

import (
	"context"

	"go-navi-smart-playlist/internal/model"
)

type EmbeddingProvider interface {
	Name() string
	Embedding(ctx context.Context, track model.Track) ([]float64, error)
}

type NoopProvider struct{}

func (NoopProvider) Name() string {
	return "noop"
}

func (NoopProvider) Embedding(context.Context, model.Track) ([]float64, error) {
	return nil, nil
}
