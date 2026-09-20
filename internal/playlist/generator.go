package playlist

import (
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"go-navi-smart-playlist/internal/config"
	"go-navi-smart-playlist/internal/features"
	"go-navi-smart-playlist/internal/model"
	"go-navi-smart-playlist/internal/scoring"
	"go-navi-smart-playlist/internal/similarity"
	"go-navi-smart-playlist/internal/state"
)

const (
	defaultArtistLimit = 3
	defaultAlbumLimit  = 3
	maxBackfillLimit   = 5
	quickTrackSeconds  = 4 * 60
	longformSeconds    = 8 * 60
)

type Definition struct {
	Name   string
	Tracks []model.Track
}

type Generator struct {
	size        int
	minBackfill int
	engine      *scoring.Engine
	logger      *log.Logger
}

type scoredTrack struct {
	track features.TrackFeatures
	score float64
}

type recipe struct {
	name       string
	eligible   func(features.TrackFeatures) bool
	score      func(features.TrackFeatures) float64
	carryover  float64
	weekJitter float64
}

func NewGenerator(cfg config.Config, logger *log.Logger) *Generator {
	return &Generator{
		size:        cfg.PlaylistSize,
		minBackfill: cfg.MinBackfill,
		engine:      scoring.New(cfg.Weights),
		logger:      logger,
	}
}

func (g *Generator) Generate(dataset features.Dataset, previous *state.HistoryState, now time.Time) []Definition {
	crossPlaylistCounts := make(map[string]int)
	ranked := make(map[string][]features.TrackFeatures)

	recipes := g.recipes()
	recipeMap := make(map[string]recipe, len(recipes))
	for _, r := range recipes {
		recipeMap[r.name] = r
	}

	// Priority generation order:
	// 1. High-intent / specific recipes
	// 2. Discovery recipes
	// 3. Recall & General mixes
	// 4. Duration-based mixes
	priorityNames := []string{
		"Fresh & Unplayed",
		"Rising This Week",
		"Top This Month",
		"Forgotten Favorites",
		"Discover Weekly",
		"Deep Cuts",
		"Hidden Gems",
		"More Like Hidden Gems",
		"Rediscover",
		"Long Time No See",
		"Comfort Shuffle",
		"Artist Adjacent Comfort",
		"Quick Mix",
		"Longform",
	}

	for _, name := range priorityNames {
		switch name {
		case "More Like Hidden Gems":
			ranked[name] = g.similarPlaylist(
				name,
				dataset,
				previous,
				now,
				ranked["Hidden Gems"],
				false,
				-0.35,
				0.25,
				crossPlaylistCounts,
				func(track features.TrackFeatures, similarityScore, genreMatch float64) float64 {
					return 1.6*similarityScore + 0.3*genreMatch + g.engine.BaseScore(track) + 0.8*track.NoveltyScore
				},
			)
		case "Artist Adjacent Comfort":
			ranked[name] = g.similarPlaylist(
				name,
				dataset,
				previous,
				now,
				ranked["Comfort Shuffle"],
				true,
				-0.2,
				0.35,
				crossPlaylistCounts,
				func(track features.TrackFeatures, similarityScore, genreMatch float64) float64 {
					return 1.5*similarityScore + 0.4*genreMatch + g.engine.BaseScore(track) + 0.5*track.StabilityScore
				},
			)
		default:
			if rec, ok := recipeMap[name]; ok {
				ranked[name] = g.rankRecipe(rec, dataset.Items, previous, now, crossPlaylistCounts)
			}
		}

		for _, item := range ranked[name] {
			crossPlaylistCounts[item.Track.ID]++
		}
	}

	catalogOrder := []string{
		"Discover Weekly",
		"Rediscover",
		"Top This Month",
		"Hidden Gems",
		"Long Time No See",
		"Comfort Shuffle",
		"More Like Hidden Gems",
		"Artist Adjacent Comfort",
		"Fresh & Unplayed",
		"Forgotten Favorites",
		"Rising This Week",
		"Deep Cuts",
		"Quick Mix",
		"Longform",
	}

	definitions := make([]Definition, 0, len(catalogOrder))
	for _, name := range catalogOrder {
		tracks := toTracks(ranked[name])
		interleaved := interleaveArtists(tracks)
		definition := Definition{Name: name, Tracks: interleaved}
		definitions = append(definitions, definition)
		g.logger.Printf("generated playlist %q with %d tracks", definition.Name, len(definition.Tracks))
	}

	return definitions
}

func (g *Generator) recipes() []recipe {
	return []recipe{
		{
			name: "Discover Weekly",
			eligible: func(track features.TrackFeatures) bool {
				if !track.HasLastPlayed {
					return track.DaysSinceAdded > 180
				}
				return track.Track.PlayCount <= 5
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.6*track.NoveltyScore +
					0.6*freshnessCurve(track.DaysSinceAdded, 0, 180) -
					0.7*track.PlayCountPercentile -
					0.4*track.RepeatFatigueScore
			},
			carryover:  -0.65,
			weekJitter: 0.35,
		},
		{
			name: "Rediscover",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed >= 45 && track.DaysSinceLastPlayed < 120
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.4*windowScore(track.DaysSinceLastPlayed, 45, 120, 30) +
					0.9*track.PlayCountPercentile +
					0.4*track.StabilityScore -
					0.3*track.RepeatFatigueScore
			},
			carryover:  -0.1,
			weekJitter: 0.15,
		},
		{
			name: "Top This Month",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed <= 31 && (track.PlayCountDelta > 0 || track.Track.PlayCount > 0)
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.8*windowScore(track.DaysSinceLastPlayed, 0, 31, 12) +
					1.2*deltaScore(track.PlayCountDelta) +
					0.5*track.PlayCountPercentile
			},
			carryover:  0.15,
			weekJitter: 0.05,
		},
		{
			name: "Hidden Gems",
			eligible: func(track features.TrackFeatures) bool {
				return track.PlayCountPercentile <= 0.55 || track.Track.Starred || track.Track.Rating >= 4
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.5*track.NoveltyScore +
					0.8*(1-track.PlayCountPercentile) +
					0.45*preferenceScore(track) +
					0.3*windowScore(track.DaysSinceLastPlayed, 21, 240, 60)
			},
			carryover:  -0.3,
			weekJitter: 0.25,
		},
		{
			name: "Long Time No See",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed >= 120
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.6*longTailScore(track.DaysSinceLastPlayed, 120, 60) +
					0.8*track.PlayCountPercentile +
					0.4*track.StabilityScore
			},
			carryover:  -0.4,
			weekJitter: 0.2,
		},
		{
			name: "Comfort Shuffle",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && (track.PlayCountPercentile >= 0.55 || track.Track.Starred || track.Track.Rating >= 4)
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) +
					1.1*preferenceScore(track) +
					0.5*track.StabilityScore -
					0.7*track.RepeatFatigueScore
			},
			carryover:  0.1,
			weekJitter: 0.9,
		},
		{
			name: "Fresh & Unplayed",
			eligible: func(track features.TrackFeatures) bool {
				return !track.HasLastPlayed && track.DaysSinceAdded <= 180
			},
			score: func(track features.TrackFeatures) float64 {
				return 2.0*freshnessCurve(track.DaysSinceAdded, 0, 180) +
					1.8*track.NoveltyScore +
					0.4*track.ArtistAffinity +
					0.3*preferenceScore(track)
			},
			carryover:  -0.8,
			weekJitter: 0.4,
		},
		{
			name: "Forgotten Favorites",
			eligible: func(track features.TrackFeatures) bool {
				favorite := track.Track.Starred || track.Track.Rating >= 4 || track.PlayCountPercentile >= 0.65
				return favorite && track.HasLastPlayed && track.DaysSinceLastPlayed >= 120
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.5*preferenceScore(track) +
					1.4*longTailScore(track.DaysSinceLastPlayed, 120, 75) +
					0.6*track.StabilityScore
			},
			carryover:  -0.5,
			weekJitter: 0.25,
		},
		{
			name: "Rising This Week",
			eligible: func(track features.TrackFeatures) bool {
				return track.HasLastPlayed && track.DaysSinceLastPlayed <= 14 && track.PlayCountDelta > 0
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.8*deltaScore(track.PlayCountDelta) +
					1.3*track.RecencyTrendScore +
					0.6*track.PlayCountPercentile +
					0.3*preferenceScore(track)
			},
			carryover:  0.1,
			weekJitter: 0.05,
		},
		{
			name: "Deep Cuts",
			eligible: func(track features.TrackFeatures) bool {
				return track.ArtistAffinity >= 0.35 && track.PlayCountPercentile <= 0.55
			},
			score: func(track features.TrackFeatures) float64 {
				return 1.7*track.ArtistAffinity +
					1.4*track.NoveltyScore +
					0.5*preferenceScore(track) +
					0.4*freshnessCurve(track.DaysSinceAdded, 0, 365)
			},
			carryover:  -0.35,
			weekJitter: 0.35,
		},
		{
			name: "Quick Mix",
			eligible: func(track features.TrackFeatures) bool {
				return track.Track.Duration > 0 && track.Track.Duration <= quickTrackSeconds
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) + 0.5*preferenceScore(track) - 0.4*track.RepeatFatigueScore
			},
			carryover:  -0.15,
			weekJitter: 0.65,
		},
		{
			name: "Longform",
			eligible: func(track features.TrackFeatures) bool {
				return track.Track.Duration >= longformSeconds
			},
			score: func(track features.TrackFeatures) float64 {
				return g.engine.BaseScore(track) + 0.6*preferenceScore(track) - 0.3*track.RepeatFatigueScore
			},
			carryover:  -0.1,
			weekJitter: 0.5,
		},
	}
}

func (g *Generator) similarPlaylist(
	name string,
	dataset features.Dataset,
	previous *state.HistoryState,
	now time.Time,
	seeds []features.TrackFeatures,
	excludeSeedArtists bool,
	carryover float64,
	weekJitter float64,
	crossPlaylistCounts map[string]int,
	scorer func(track features.TrackFeatures, similarityScore, genreMatch float64) float64,
) []features.TrackFeatures {
	if len(seeds) == 0 {
		return nil
	}

	seedVectors := make([][]float64, 0, min(len(seeds), 10))
	seedIDs := make(map[string]struct{}, len(seeds))
	seedArtists := make(map[string]struct{}, len(seeds))
	seedGenreCounts := make(map[string]int, len(seeds))
	for index, seed := range seeds {
		seedIDs[seed.Track.ID] = struct{}{}
		seedArtists[canonicalKey(seed.Track.Artist)] = struct{}{}
		if genre := canonicalKey(seed.Track.Genre); genre != "__unknown__" {
			seedGenreCounts[genre]++
		}
		if index < 10 {
			seedVectors = append(seedVectors, seed.SimilarityVector)
		}
	}

	centroid := similarity.Centroid(seedVectors)
	if len(centroid) == 0 {
		return nil
	}

	scored := make([]scoredTrack, 0, len(dataset.Items))
	for _, track := range dataset.Items {
		if _, isSeed := seedIDs[track.Track.ID]; isSeed {
			continue
		}
		_, duplicateArtist := seedArtists[canonicalKey(track.Track.Artist)]
		if excludeSeedArtists && duplicateArtist {
			continue
		}

		genreMatch := 0.0
		if matches := seedGenreCounts[canonicalKey(track.Track.Genre)]; matches > 0 {
			genreMatch = float64(matches) / float64(len(seeds))
		}
		score := scorer(track, similarity.CosineSimilarity(track.SimilarityVector, centroid), genreMatch)
		if duplicateArtist {
			score -= 0.25
		}
		score += g.playlistCarryover(previous, name, track.Track.ID, carryover, now)
		score += centeredWeeklyJitter(name, track.Track.ID, now) * weekJitter
		if prior := crossPlaylistCounts[track.Track.ID]; prior > 0 {
			score -= float64(prior) * 1.5
		}
		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sortScored(scored)
	return g.finalize(name, scored, now)
}

func (g *Generator) rankRecipe(
	currentRecipe recipe,
	items []features.TrackFeatures,
	previous *state.HistoryState,
	now time.Time,
	crossPlaylistCounts map[string]int,
) []features.TrackFeatures {
	scored := make([]scoredTrack, 0, len(items))
	for _, track := range items {
		if currentRecipe.eligible != nil && !currentRecipe.eligible(track) {
			continue
		}
		score := currentRecipe.score(track)
		score += g.playlistCarryover(previous, currentRecipe.name, track.Track.ID, currentRecipe.carryover, now)
		score += centeredWeeklyJitter(currentRecipe.name, track.Track.ID, now) * currentRecipe.weekJitter
		if prior := crossPlaylistCounts[track.Track.ID]; prior > 0 {
			if currentRecipe.name == "Quick Mix" || currentRecipe.name == "Longform" {
				score -= float64(prior) * 0.4
			} else {
				score -= float64(prior) * 1.5
			}
		}
		scored = append(scored, scoredTrack{track: track, score: score})
	}

	sortScored(scored)
	return g.finalize(currentRecipe.name, scored, now)
}

func sortScored(scored []scoredTrack) {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].track.Track.ID < scored[j].track.Track.ID
		}
		return scored[i].score > scored[j].score
	})
}

func (g *Generator) finalize(name string, scored []scoredTrack, now time.Time) []features.TrackFeatures {
	result := sampleWithDiversity(scored, g.size, defaultArtistLimit, defaultAlbumLimit, name, now)
	if len(result) < min(g.size, g.minBackfill) {
		result = limitDiversity(scored, g.size, maxBackfillLimit, maxBackfillLimit)
	}

	g.logger.Printf("playlist %q candidates=%d selected=%d", name, len(scored), len(result))
	return result
}

func (g *Generator) playlistCarryover(
	previous *state.HistoryState,
	playlistName, trackID string,
	adjustment float64,
	now time.Time,
) float64 {
	if previous == nil {
		return 0
	}

	penalty := 0.0

	if lastFeatured, ok := previous.TrackLastFeaturedIn(playlistName, trackID); ok {
		daysSince := now.Sub(lastFeatured).Hours() / 24
		if daysSince < 0 {
			daysSince = 0
		}
		if adjustment < 0 {
			switch {
			case daysSince <= 7:
				penalty += adjustment
			case daysSince <= 14:
				penalty += adjustment * 0.65
			case daysSince <= 21:
				penalty += adjustment * 0.35
			}
		} else if adjustment > 0 {
			switch {
			case daysSince <= 7:
				penalty += adjustment
			case daysSince <= 14:
				penalty += adjustment * 0.5
			}
		}
	} else if previous.PlaylistContains(playlistName, trackID) {
		penalty += adjustment
	}

	if lastFeaturedAt, ok := previous.TrackLastFeatured(trackID); ok {
		daysSinceGlobal := now.Sub(lastFeaturedAt).Hours() / 24
		if daysSinceGlobal < 0 {
			daysSinceGlobal = 0
		}
		switch {
		case daysSinceGlobal <= 7:
			penalty -= 0.4
		case daysSinceGlobal <= 14:
			penalty -= 0.2
		case daysSinceGlobal <= 21:
			penalty -= 0.1
		}
	}

	return penalty
}

func sampleWithDiversity(
	items []scoredTrack,
	size, maxPerArtist, maxPerAlbum int,
	playlistName string,
	now time.Time,
) []features.TrackFeatures {
	if len(items) <= size {
		return limitDiversity(items, size, maxPerArtist, maxPerAlbum)
	}

	poolSize := min(len(items), max(size+15, int(float64(size)*1.5)))
	pool := items[:poolSize]

	weights := make([]float64, len(pool))
	for i := range pool {
		weights[i] = 1.0 / math.Pow(float64(i+1), 0.75)
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(fmt.Sprintf("%s:%d", playlistName, now.Unix())))
	rng := rand.New(rand.NewSource(int64(hasher.Sum64())))

	selected := make([]features.TrackFeatures, 0, min(len(items), size))
	artistCounts := make(map[string]int)
	albumCounts := make(map[string]int)
	chosen := make(map[int]bool, poolSize)

	for len(selected) < size && len(chosen) < poolSize {
		totalWeight := 0.0
		for i, w := range weights {
			if !chosen[i] {
				artistKey := diversityKey(pool[i].track.Track.Artist, pool[i].track.Track.ID)
				albumKey := diversityKey(pool[i].track.Track.Album, pool[i].track.Track.ID)
				if artistCounts[artistKey] < maxPerArtist && albumCounts[albumKey] < maxPerAlbum {
					totalWeight += w
				}
			}
		}

		if totalWeight <= 0 {
			break
		}

		target := rng.Float64() * totalWeight
		cumulative := 0.0
		pickedIndex := -1

		for i, w := range weights {
			if !chosen[i] {
				artistKey := diversityKey(pool[i].track.Track.Artist, pool[i].track.Track.ID)
				albumKey := diversityKey(pool[i].track.Track.Album, pool[i].track.Track.ID)
				if artistCounts[artistKey] < maxPerArtist && albumCounts[albumKey] < maxPerAlbum {
					cumulative += w
					if cumulative >= target {
						pickedIndex = i
						break
					}
				}
			}
		}

		if pickedIndex < 0 {
			for i := range pool {
				if !chosen[i] {
					artistKey := diversityKey(pool[i].track.Track.Artist, pool[i].track.Track.ID)
					albumKey := diversityKey(pool[i].track.Track.Album, pool[i].track.Track.ID)
					if artistCounts[artistKey] < maxPerArtist && albumCounts[albumKey] < maxPerAlbum {
						pickedIndex = i
						break
					}
				}
			}
			if pickedIndex < 0 {
				break
			}
		}

		chosen[pickedIndex] = true
		artistKey := diversityKey(pool[pickedIndex].track.Track.Artist, pool[pickedIndex].track.Track.ID)
		albumKey := diversityKey(pool[pickedIndex].track.Track.Album, pool[pickedIndex].track.Track.ID)
		artistCounts[artistKey]++
		albumCounts[albumKey]++
		selected = append(selected, pool[pickedIndex].track)
	}

	return selected
}

func interleaveArtists(tracks []model.Track) []model.Track {
	if len(tracks) <= 2 {
		return tracks
	}

	groups := make(map[string][]model.Track)
	order := make([]string, 0)
	for _, track := range tracks {
		key := artistKey(track.Artist, track.ID)
		if _, exists := groups[key]; !exists {
			order = append(order, key)
		}
		groups[key] = append(groups[key], track)
	}

	if len(groups) == len(tracks) {
		return tracks
	}

	result := make([]model.Track, 0, len(tracks))
	lastArtistKey := ""
	remaining := len(tracks)

	for remaining > 0 {
		bestKey := ""
		bestCount := -1
		bestIndexInOrder := -1

		for idx, key := range order {
			count := len(groups[key])
			if count == 0 {
				continue
			}

			if key == lastArtistKey && remaining > count {
				continue
			}

			if count > bestCount {
				bestCount = count
				bestKey = key
				bestIndexInOrder = idx
			} else if count == bestCount && (bestIndexInOrder == -1 || idx < bestIndexInOrder) {
				bestKey = key
				bestIndexInOrder = idx
			}
		}

		if bestKey == "" {
			for _, key := range order {
				if len(groups[key]) > 0 {
					bestKey = key
					break
				}
			}
		}

		nextTrack := groups[bestKey][0]
		groups[bestKey] = groups[bestKey][1:]
		result = append(result, nextTrack)
		lastArtistKey = bestKey
		remaining--
	}

	return result
}

func artistKey(artist, trackID string) string {
	k := canonicalKey(artist)
	if k == "__unknown__" || k == "unknown artist" {
		return k + ":" + trackID
	}
	return k
}

func centeredWeeklyJitter(playlistName, trackID string, now time.Time) float64 {
	year, week := now.ISOWeek()
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d", playlistName, trackID, year, week)))
	value := float64(hasher.Sum64()%1_000_000) / 1_000_000
	return value - 0.5
}

func toTracks(items []features.TrackFeatures) []model.Track {
	tracks := make([]model.Track, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, item.Track)
	}

	return tracks
}

func limitDiversity(items []scoredTrack, size, maxPerArtist, maxPerAlbum int) []features.TrackFeatures {
	result := make([]features.TrackFeatures, 0, min(len(items), size))
	artistCounts := make(map[string]int)
	albumCounts := make(map[string]int)

	for _, item := range items {
		if len(result) >= size {
			break
		}

		artistKey := diversityKey(item.track.Track.Artist, item.track.Track.ID)
		albumKey := diversityKey(item.track.Track.Album, item.track.Track.ID)
		if artistCounts[artistKey] >= maxPerArtist || albumCounts[albumKey] >= maxPerAlbum {
			continue
		}

		artistCounts[artistKey]++
		albumCounts[albumKey]++
		result = append(result, item.track)
	}

	return result
}

func canonicalKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "__unknown__"
	}
	return trimmed
}

func diversityKey(value, trackID string) string {
	key := canonicalKey(value)
	if key == "__unknown__" || key == "unknown artist" || key == "unknown album" {
		return key + ":" + trackID
	}
	return key
}

func windowScore(value, start, end, softness float64) float64 {
	if value >= start && value <= end {
		return 1
	}
	if value < start {
		return math.Exp(-(start - value) / softness)
	}
	return math.Exp(-(value - end) / softness)
}

func longTailScore(value, start, softness float64) float64 {
	if value <= start {
		return 0
	}
	return 1 - math.Exp(-(value-start)/softness)
}

func freshnessCurve(value, start, end float64) float64 {
	if value <= start {
		return 1
	}
	if value >= end || end <= start {
		return 0
	}
	return 1 - (value-start)/(end-start)
}

func preferenceScore(track features.TrackFeatures) float64 {
	rating := clamp01(float64(track.Track.Rating) / 5.0)
	starred := boolScore(track.Track.Starred)
	return clamp01(0.5*track.PlayCountPercentile + 0.3*rating + 0.2*starred)
}

func deltaScore(value float64) float64 {
	return clamp01(value / 5.0)
}

func boolScore(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
