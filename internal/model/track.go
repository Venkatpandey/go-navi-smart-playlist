package model

import "time"

type Track struct {
	ID         string
	Title      string
	Artist     string
	Album      string
	Genre      string
	Duration   int
	PlayCount  int
	LastPlayed time.Time
	Created    time.Time
	Rating     int
	Starred    bool
}
