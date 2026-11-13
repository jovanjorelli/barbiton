package sampler

import "math"

const (
	LoopNoLoop = "no_loop"

	LoopContinuous = "loop_continuous"

	LoopOneShot = "one_shot"
)

type EGParams struct {
	Delay, Start, Attack, Hold, Decay, Sustain, Release         float32
	Vel2Attack, Vel2Decay, Vel2Release, Key2Attack, Key2Release float32
	Depth, Vel2Depth                                            float32
}

type LFOParams struct {
	Delay, Fade, Freq                                   float32
	Volume, Amplitude, Pitch, Filter, Pan, VolumeSmooth float32
	Wave, FreqWave                                      int
}

type EQBand struct {
	Freq, BW, Gain, Veltrack float32
}

type Region struct {
	SamplePath     string
	LoKey          int
	HiKey          int
	PitchKeyCenter int
	LoVel          int
	HiVel          int
	Volume         float32
	Pan            float32
	Sample         *SampleData

	Offset        int
	End           int
	Count         int
	LoopMode      string
	LoopStart     int
	LoopEnd       int
	LoopType      int
	LoopCount     int
	LoopTune      int
	LoopCrossfade int
	Reverse       int

	XfinLoKey  int
	XfinHiKey  int
	XfinLoVel  int
	XfinHiVel  int
	XfoutLoKey int
	XfoutHiKey int
	XfoutLoVel int
	XfoutHiVel int

	PitchKeytrack int
	Transpose     int
	Tune          int
	PitchRandom   int
	PitchVeltrack int
	BendUp        int
	BendDown      int
	BendStep      int

	AmpRandom    float32
	AmpVeltrack  float32
	AmpKeytrack  float32
	AmpKeycenter int
	RtDecay      float32

	AmpEG, PitchEG, FilEG          EGParams
	HasAmpEG, HasPitchEG, HasFilEG bool
	LFO                            [2]LFOParams
	FilType                        string
	Cutoff                         float32
	Resonance                      float32
	FilKeytrack                    float32
	FilKeycenter                   int
	FilVeltrack                    float32
	FilRandom                      float32
	EQ                             [3]EQBand

	OffBy         int
	OffMode       string
	Polyphony     int
	NotePolyphony int
	Trigger       string
	GroupLabel    string

	SeqLength   int
	SeqPosition int

	LoRand float32
	HiRand float32

	SwLoKey, SwHiKey, SwDefault int
	SwLast, SwDown, SwUp        int
}

type SampleData struct {
	Samples    []float32
	SamplesR   []float32
	SampleRate int
	Channels   int
	LoopStart  int
	LoopEnd    int
}

func (sd *SampleData) Frames() int { return len(sd.Samples) }

func newRegion() Region {
	return Region{
		LoKey: 0, HiKey: 127, PitchKeyCenter: 60, PitchKeytrack: 100,
		LoVel: 0, HiVel: 127,
		Volume: 0, Pan: 0,
		Count:      -1,
		LoopMode:   LoopNoLoop,
		LoopStart:  -1,
		LoopEnd:    -1,
		XfoutLoKey: 127, XfoutHiKey: 127,
		XfoutLoVel: 127, XfoutHiVel: 127,
		Cutoff:       -1,
		FilKeycenter: 60,
		AmpKeycenter: 60,
		OffMode:      "fast",
		SeqLength:    1,
		SeqPosition:  1,
		LoRand:       0,
		HiRand:       1,
		SwLoKey:      -1,
		SwHiKey:      -1,
		SwDefault:    -1,
		SwLast:       -1,
		SwDown:       -1,
		SwUp:         -1,
	}
}

func (r *Region) ensureAmpEG() {
	if !r.HasAmpEG {
		r.AmpEG = EGParams{Sustain: 100}
		r.HasAmpEG = true
	}
}

func (r *Region) ensurePitchEG() {
	if !r.HasPitchEG {
		r.PitchEG = EGParams{}
		r.HasPitchEG = true
	}
}

func (r *Region) ensureFilEG() {
	if !r.HasFilEG {
		r.FilEG = EGParams{}
		r.HasFilEG = true
	}
}

func (r *Region) Matches(pitch, velocity int) bool {
	return r.Sample != nil &&
		pitch >= r.LoKey && pitch <= r.HiKey &&
		velocity >= r.LoVel && velocity <= r.HiVel
}

func (r *Region) LinearGain() float32 {
	return float32(math.Pow(10, float64(r.Volume)/20))
}

func (r *Region) PlaybackRateFor(pitch int, dstRate float64) float64 {
	if r.Sample == nil || dstRate <= 0 {
		return 0
	}
	semitones := float64(pitch-r.PitchKeyCenter+r.Transpose) + float64(r.Tune)/100
	return math.Pow(2, semitones/12) * float64(r.Sample.SampleRate) / dstRate
}
