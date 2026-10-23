// Package segment files the photos of one capture session by the identity
// photos taken among them, the way a film editor cuts footage by its slates.
package segment

import (
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"pidex/internal/teamconfig"
)

// Anchor is an identity resolved from a photo.
type Anchor struct {
	SubjectID  uuid.UUID
	Confidence float64
}

type Item struct {
	ID         uuid.UUID
	CapturedAt time.Time
	Lat, Lng   *float64
	// Manual photos keep their subject and are not re-filed.
	Manual bool
	// Anchor is set when this photo itself identified a subject.
	Anchor *Anchor
	// Barrier marks an identity that was seen but not resolved. Anchors do
	// not carry across it: the photos after a new, unrecognized wristband
	// must not inherit the previous patient.
	Barrier bool
	// Locked is the subject the banner showed when the photo was taken.
	Locked *uuid.UUID
}

type Source string

const (
	SourceSelf        Source = "self"
	SourceAnchorAhead Source = "anchor_before"
	SourceAnchorAfter Source = "anchor_after"
	SourceLocked      Source = "locked"
	SourceNone        Source = "none"
)

type Decision struct {
	PhotoID    uuid.UUID
	SubjectID  *uuid.UUID
	Confidence float64
	Source     Source
}

// Confidence carried from an anchor to its neighbours. A photo after its
// anchor is the normal "shoot the wristband first" flow; a photo before the
// next anchor is plausible but weaker; the banner alone is weaker still.
const (
	carryAhead  = 0.99
	carryAfter  = 0.9
	lockedScore = 0.8
)

// Assign returns a decision for every non-manual item. Items must belong to
// one session; order does not matter.
func Assign(items []Item, rules teamconfig.SessionRules) []Decision {
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CapturedAt.Before(sorted[j].CapturedAt) })

	runs := splitRuns(sorted, rules)
	var out []Decision
	for _, run := range runs {
		for i, it := range run {
			if it.Manual {
				continue
			}
			out = append(out, decide(run, i, rules))
		}
	}
	return out
}

func decide(run []Item, i int, rules teamconfig.SessionRules) Decision {
	it := run[i]
	if it.Anchor != nil {
		id := it.Anchor.SubjectID
		return Decision{PhotoID: it.ID, SubjectID: &id, Confidence: it.Anchor.Confidence, Source: SourceSelf}
	}
	valid := func(a, b time.Time) bool {
		if rules.AnchorValidMinutes <= 0 {
			return true
		}
		return b.Sub(a).Abs() <= time.Duration(rules.AnchorValidMinutes)*time.Minute
	}
	if it.Barrier {
		return Decision{PhotoID: it.ID, Source: SourceNone}
	}
	for j := i - 1; j >= 0; j-- {
		if run[j].Barrier {
			break
		}
		if a := run[j].Anchor; a != nil {
			if valid(run[j].CapturedAt, it.CapturedAt) {
				id := a.SubjectID
				return Decision{PhotoID: it.ID, SubjectID: &id, Confidence: a.Confidence * carryAhead, Source: SourceAnchorAhead}
			}
			break
		}
	}
	for j := i + 1; j < len(run); j++ {
		if run[j].Barrier {
			break
		}
		if a := run[j].Anchor; a != nil {
			if valid(it.CapturedAt, run[j].CapturedAt) {
				id := a.SubjectID
				return Decision{PhotoID: it.ID, SubjectID: &id, Confidence: a.Confidence * carryAfter, Source: SourceAnchorAfter}
			}
			break
		}
	}
	if it.Locked != nil {
		id := *it.Locked
		return Decision{PhotoID: it.ID, SubjectID: &id, Confidence: lockedScore, Source: SourceLocked}
	}
	return Decision{PhotoID: it.ID, Source: SourceNone}
}

// splitRuns cuts the session wherever the photographer paused too long or
// moved too far; an anchor never reaches across a cut.
func splitRuns(items []Item, rules teamconfig.SessionRules) [][]Item {
	var runs [][]Item
	var cur []Item
	gap := time.Duration(rules.GapMinutes) * time.Minute
	for i, it := range items {
		if i > 0 {
			prev := items[i-1]
			if it.CapturedAt.Sub(prev.CapturedAt) > gap || moved(prev, it, rules.MaxJumpMeters) {
				runs = append(runs, cur)
				cur = nil
			}
		}
		cur = append(cur, it)
	}
	if len(cur) > 0 {
		runs = append(runs, cur)
	}
	return runs
}

func moved(a, b Item, maxMeters int) bool {
	if maxMeters <= 0 || a.Lat == nil || a.Lng == nil || b.Lat == nil || b.Lng == nil {
		return false
	}
	return haversineMeters(*a.Lat, *a.Lng, *b.Lat, *b.Lng) > float64(maxMeters)
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}
