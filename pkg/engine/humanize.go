package engine

import (
	"maps"
	"math"
	"strconv"
	"strings"

	"barbiton/pkg/score"
)

const (
	timingSigma   = 0.012
	maxJitter     = 0.050
	velocityJit   = 0.05
	staccatoJit   = 0.10
	legatoMin     = 0.05
	legatoRange   = 0.10
	defaultDurJit = 0.03
	minDuration   = 0.01
	downbeatTol   = 1e-6
)

type humanRNG struct {
	state    uint64
	spare    float64
	hasSpare bool
}

func newHumanRNG(seed uint64) *humanRNG {
	if seed == 0 {
		seed = 0x9E3779B97F4A7C15
	}
	return &humanRNG{state: seed}
}

func (r *humanRNG) uniform() float64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return float64(r.state>>11) / float64(uint64(1)<<53)
}

func (r *humanRNG) gauss() float64 {
	if r.hasSpare {
		r.hasSpare = false
		return r.spare
	}
	u1 := r.uniform()
	for u1 <= 1e-12 {
		u1 = r.uniform()
	}
	u2 := r.uniform()
	mag := math.Sqrt(-2 * math.Log(u1))
	r.spare = mag * math.Sin(2*math.Pi*u2)
	r.hasSpare = true
	return mag * math.Cos(2*math.Pi*u2)
}

func Humanize(s *score.Score, strength float32, seed uint64) *score.Score {
	if s == nil {
		return nil
	}
	out := copyScore(s)
	st := clampF64(float64(strength), 0, 1)
	if st == 0 {
		return out
	}
	secPerBeat := 60 / s.Metadata.BPM
	if !(secPerBeat > 0) || math.IsInf(secPerBeat, 0) {
		return out
	}
	beats := float64(BeatsPerBar(s.Metadata.TimeSignature))

	for ti := range out.Tracks {
		rng := newHumanRNG(seed + uint64(ti)*0x9E3779B97F4A7C15)
		tr := &out.Tracks[ti]
		for ni := range tr.Notes {
			n := &tr.Notes[ni]

			beatPos := n.Time / secPerBeat
			beatInBar := beatPos - math.Floor(beatPos/beats)*beats
			onDownbeat := beatInBar < downbeatTol || beats-beatInBar < downbeatTol

			if !onDownbeat {
				jitter := clampF64(rng.gauss()*timingSigma*st, -maxJitter, maxJitter)
				n.Time = math.Max(0, n.Time+jitter)
			}

			w := metricWeight(beatInBar, int(beats))
			w = 1 + (w-1)*st
			v := float64(n.Velocity) * w * (1 + velocityJit*st*rng.gauss())
			n.Velocity = float32(clampF64(v, 0, 1))

			orig := n.Duration
			dur := orig
			switch strings.ToLower(n.Articulation.Type) {
			case "staccato":
				dur *= 1 + staccatoJit*st*rng.gauss()
			case "legato":
				dur *= 1 + st*(legatoMin+legatoRange*rng.uniform())
			default:
				dur *= 1 + defaultDurJit*st*rng.gauss()
			}
			n.Duration = clampF64(dur, minDuration, orig*4)
		}
	}
	return out
}

func metricWeight(beatInBar float64, beats int) float64 {
	if beats < 1 {
		beats = 4
	}
	whole := math.Round(beatInBar)
	frac := math.Abs(beatInBar - whole)
	idx := int(whole)
	if idx >= beats {
		idx = 0
	}
	switch {
	case frac > 0.4 && frac < 0.6:
		return 0.75
	case frac > 0.25:
		return 0.7
	}
	switch beats {
	case 4:
		return [4]float64{1.0, 0.85, 0.95, 0.8}[idx]
	case 3:
		return [3]float64{1.0, 0.85, 0.75}[idx]
	case 2:
		return [2]float64{1.0, 0.85}[idx]
	}
	if idx == 0 {
		return 1.0
	}
	if idx == beats/2 {
		return 0.95
	}
	return 0.85
}

func BeatsPerBar(timeSignature string) int {
	num, _, ok := strings.Cut(timeSignature, "/")
	if !ok {
		return 4
	}
	n, err := strconv.Atoi(strings.TrimSpace(num))
	if err != nil || n < 1 {
		return 4
	}
	return n
}

func copyScore(s *score.Score) *score.Score {
	out := &score.Score{Metadata: s.Metadata}
	out.Tracks = make([]score.Track, len(s.Tracks))
	for i, tr := range s.Tracks {
		cp := tr
		cp.Instrument.Parameters = maps.Clone(tr.Instrument.Parameters)
		cp.Notes = make([]score.NoteEvent, len(tr.Notes))
		copy(cp.Notes, tr.Notes)
		for j := range cp.Notes {
			cp.Notes[j].Articulation.Params = maps.Clone(cp.Notes[j].Articulation.Params)
		}
		out.Tracks[i] = cp
	}
	return out
}

func clampF64(v, lo, hi float64) float64 {
	switch {
	case v < lo || v != v:
		return lo
	case v > hi:
		return hi
	}
	return v
}
