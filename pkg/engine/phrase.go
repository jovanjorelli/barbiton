package engine

import (
	"sort"

	"barbiton/pkg/dsp"
	"barbiton/pkg/score"
)

const maxEngineRooms = 8

func breathTimes(tr *score.Track, globalBreathMs float64) []float64 {
	times := make([]float64, len(tr.Notes))
	for j := range tr.Notes {
		times[j] = tr.Notes[j].Time
	}
	return times
}

func breathDurations(tr *score.Track, globalBreathMs float64) []float64 {
	durs := make([]float64, len(tr.Notes))
	for j := range tr.Notes {
		durs[j] = tr.Notes[j].Duration
	}
	breath := tr.BreathMs
	if breath == 0 {
		breath = globalBreathMs
	}
	if breath <= 0 || len(durs) < 2 {
		return durs
	}
	gap := breath / 1000.0
	order := make([]int, len(tr.Notes))
	for j := range order {
		order[j] = j
	}
	sort.SliceStable(order, func(a, b int) bool {
		return tr.Notes[order[a]].Time < tr.Notes[order[b]].Time
	})
	prevPhrase := 0
	var prevIndices []int
	for _, j := range order {
		currPhrase := tr.Notes[j].PhraseID
		if currPhrase != 0 && prevPhrase != 0 && currPhrase != prevPhrase {
			currStart := tr.Notes[j].Time
			for _, pIdx := range prevIndices {
				pStart := tr.Notes[pIdx].Time
				pDur := durs[pIdx]
				if pStart+pDur > currStart-gap {
					newDur := (currStart - gap) - pStart
					if newDur < 0.01 {
						newDur = 0.01
					}
					if newDur < pDur {
						durs[pIdx] = newDur
					}
				}
			}
		}
		if currPhrase != prevPhrase {
			prevIndices = prevIndices[:0]
			prevPhrase = currPhrase
		}
		prevIndices = append(prevIndices, j)
	}
	return durs
}

type roomEntry struct {
	params dsp.ReverbParams
	uses   int
}

func roomDistance(a, b dsp.ReverbParams) float64 {
	ds := float64(a.RoomSize - b.RoomSize)
	dd := float64(a.Damping - b.Damping)
	dw := float64(a.Width - b.Width)
	return ds*ds + dd*dd + dw*dw
}

func resolveRoom(r *score.Room) (dsp.ReverbParams, bool, bool) {
	if r.IsPreset {
		if r.Preset == "none" {
			return dsp.ReverbParams{}, true, true
		}
		p, ok := dsp.PresetParams(r.Preset)
		return p, false, ok
	}
	return dsp.ReverbParams{
		RoomSize: dsp.ClampF32(float32(r.Config.Size), 0, 1),
		Damping:  dsp.ClampF32(float32(r.Config.Damping), 0, 1),
		Width:    dsp.ClampF32(float32(r.Config.Width), 0, 1),
		Wet:      0.4,
		Dry:      0.6,
		PreDelay: 0.02,
	}, false, true
}

func (e *Engine) assignRooms(s *score.Score) {
	type pending struct {
		track int
		none  bool
		entry int
	}
	var (
		entries []roomEntry
		items   []pending
	)
	for i := range s.Tracks {
		r := s.Tracks[i].Room
		if r == nil {
			continue
		}
		params, none, ok := resolveRoom(r)
		if !ok {
			continue
		}
		if none {
			items = append(items, pending{track: i, none: true})
			continue
		}
		at := -1
		for k := range entries {
			if entries[k].params == params {
				at = k
				break
			}
		}
		if at < 0 {
			at = len(entries)
			entries = append(entries, roomEntry{params: params})
		}
		entries[at].uses++
		items = append(items, pending{track: i, entry: at})
	}
	for len(entries) > maxEngineRooms {

		victim := 0
		for k := range entries {
			if entries[k].uses < entries[victim].uses {
				victim = k
			}
		}
		best, bestD := -1, -1.0
		for k := range entries {
			if k == victim {
				continue
			}
			if d := roomDistance(entries[victim].params, entries[k].params); best < 0 || d < bestD {
				best, bestD = k, d
			}
		}
		entries[best].uses += entries[victim].uses
		survivor := best
		if survivor > victim {
			survivor--
		}
		for k := range items {
			if items[k].entry == victim {
				items[k].entry = survivor
			} else if items[k].entry > victim {
				items[k].entry--
			}
		}
		entries = append(entries[:victim], entries[victim+1:]...)
	}
	for _, it := range items {
		if it.none {
			e.mix.SetTrackRoomNone(it.track)
			continue
		}
		e.mix.SetTrackRoom(it.track, entries[it.entry].params)
	}
}
