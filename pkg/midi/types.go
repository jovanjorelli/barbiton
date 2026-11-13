package midi

import "sort"

const (
	chunkHeader = "MThd"
	chunkTrack  = "MTrk"
)

type EventType uint8

const (
	EventNoteOff EventType = iota

	EventNoteOn

	EventPolyPressure

	EventControlChange

	EventProgramChange

	EventChannelPressure

	EventPitchBend

	EventSysex

	EventMeta
)

const (
	MetaText = 0x01

	MetaCopyright = 0x02

	MetaTrackName = 0x03

	MetaInstrument = 0x04

	MetaMarker = 0x06

	MetaEndOfTrack = 0x2F

	MetaTempo = 0x51

	MetaTimeSignature = 0x58

	MetaKeySignature = 0x59
)

type Event struct {
	Tick    uint32
	Type    EventType
	Channel int
	Data1   int
	Data2   int
	Value   int
	Data    []byte
}

type Header struct {
	Format          int
	Tracks          int
	TicksPerQuarter int
}

type File struct {
	Header Header
	Tracks [][]Event
}

type Note struct {
	Channel   int
	Pitch     int
	Velocity  int
	StartTick uint32
	EndTick   uint32
}

type Tempo struct {
	Tick             uint32
	MicrosPerQuarter int
}

type TimeSig struct {
	Tick        uint32
	Numerator   int
	Denominator int
}

func BPMFromMicros(mpq int) float64 { return 60e6 / float64(mpq) }

func MicrosFromBPM(bpm float64) int {
	if !(bpm > 0) {
		bpm = 120
	}
	mpq := int(roundF(60e6 / bpm))
	if mpq < 60 {
		mpq = 60
	}
	if mpq > 6000000 {
		mpq = 6000000
	}
	return mpq
}

func (f *File) Tempos() []Tempo {
	var out []Tempo
	for _, tr := range f.Tracks {
		for _, e := range tr {
			if e.Type == EventMeta && e.Data1 == MetaTempo && len(e.Data) >= 3 {
				mpq := int(e.Data[0])<<16 | int(e.Data[1])<<8 | int(e.Data[2])
				if mpq > 0 {
					out = append(out, Tempo{Tick: e.Tick, MicrosPerQuarter: mpq})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	dedup := out[:0]
	for i, t := range out {
		if i == 0 || t.Tick != out[i-1].Tick {
			dedup = append(dedup, t)
		}
	}
	return dedup
}

func (f *File) TimeSignatures() []TimeSig {
	var out []TimeSig
	for _, tr := range f.Tracks {
		for _, e := range tr {
			if e.Type == EventMeta && e.Data1 == MetaTimeSignature && len(e.Data) >= 2 {
				denomPow := int(e.Data[1])
				if denomPow < 0 || denomPow > 5 {
					continue
				}
				out = append(out, TimeSig{Tick: e.Tick, Numerator: int(e.Data[0]), Denominator: 1 << denomPow})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Tick < out[j].Tick })
	return out
}

func TrackName(events []Event) string {
	for _, kind := range []int{MetaTrackName, MetaInstrument} {
		for _, e := range events {
			if e.Type == EventMeta && e.Data1 == kind && len(e.Data) > 0 {
				return string(e.Data)
			}
		}
	}
	return ""
}

func MaxTick(events []Event) uint32 {
	var max uint32
	for _, e := range events {
		if e.Tick > max {
			max = e.Tick
		}
	}
	return max
}

func CollectNotes(events []Event, endTick uint32) []Note {
	type key struct{ ch, pitch int }
	type pending struct {
		tick uint32
		vel  int
	}
	open := make(map[key][]pending)
	var notes []Note

	closeNote := func(k key, p pending, end uint32) {
		if end <= p.tick {
			end = p.tick + 1
		}
		notes = append(notes, Note{
			Channel: k.ch, Pitch: k.pitch, Velocity: p.vel,
			StartTick: p.tick, EndTick: end,
		})
	}

	for _, e := range events {
		switch e.Type {
		case EventNoteOn:
			k := key{e.Channel, e.Data1}
			if e.Data2 > 0 {
				open[k] = append(open[k], pending{e.Tick, e.Data2})
				continue
			}
			fallthrough
		case EventNoteOff:
			k := key{e.Channel, e.Data1}
			st := open[k]
			if len(st) == 0 {
				continue
			}
			closeNote(k, st[len(st)-1], e.Tick)
			open[k] = st[:len(st)-1]
		}
	}
	for k, stack := range open {
		for _, p := range stack {
			closeNote(k, p, endTick)
		}
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].StartTick != notes[j].StartTick {
			return notes[i].StartTick < notes[j].StartTick
		}
		return notes[i].Pitch < notes[j].Pitch
	})
	return notes
}

var GMProgramNames = [128]string{
	"Acoustic Grand Piano", "Bright Acoustic Piano", "Electric Grand Piano", "Honky-tonk Piano",
	"Electric Piano 1", "Electric Piano 2", "Harpsichord", "Clavinet",
	"Celesta", "Glockenspiel", "Music Box", "Vibraphone",
	"Marimba", "Xylophone", "Tubular Bells", "Dulcimer",
	"Drawbar Organ", "Percussive Organ", "Rock Organ", "Church Organ",
	"Reed Organ", "Accordion", "Harmonica", "Tango Accordion",
	"Acoustic Guitar (nylon)", "Acoustic Guitar (steel)", "Electric Guitar (jazz)", "Electric Guitar (clean)",
	"Electric Guitar (muted)", "Overdriven Guitar", "Distortion Guitar", "Guitar Harmonics",
	"Acoustic Bass", "Electric Bass (finger)", "Electric Bass (pick)", "Fretless Bass",
	"Slap Bass 1", "Slap Bass 2", "Synth Bass 1", "Synth Bass 2",
	"Violin", "Viola", "Cello", "Contrabass",
	"Tremolo Strings", "Pizzicato Strings", "Orchestral Harp", "Timpani",
	"String Ensemble 1", "String Ensemble 2", "Synth Strings 1", "Synth Strings 2",
	"Choir Aahs", "Voice Oohs", "Synth Voice", "Orchestra Hit",
	"Trumpet", "Trombone", "Tuba", "Muted Trumpet",
	"French Horn", "Brass Section", "Synth Brass 1", "Synth Brass 2",
	"Soprano Sax", "Alto Sax", "Tenor Sax", "Baritone Sax",
	"Oboe", "English Horn", "Bassoon", "Clarinet",
	"Piccolo", "Flute", "Recorder", "Pan Flute",
	"Blown Bottle", "Shakuhachi", "Whistle", "Ocarina",
	"Lead 1 (square)", "Lead 2 (sawtooth)", "Lead 3 (calliope)", "Lead 4 (chiff)",
	"Lead 5 (charang)", "Lead 6 (voice)", "Lead 7 (fifths)", "Lead 8 (bass + lead)",
	"Pad 1 (new age)", "Pad 2 (warm)", "Pad 3 (polysynth)", "Pad 4 (choir)",
	"Pad 5 (bowed)", "Pad 6 (metallic)", "Pad 7 (halo)", "Pad 8 (sweep)",
	"FX 1 (rain)", "FX 2 (soundtrack)", "FX 3 (crystal)", "FX 4 (atmosphere)",
	"FX 5 (brightness)", "FX 6 (goblins)", "FX 7 (echoes)", "FX 8 (sci-fi)",
	"Sitar", "Banjo", "Shamisen", "Koto",
	"Kalimba", "Bagpipe", "Fiddle", "Shanai",
	"Tinkle Bell", "Agogo", "Steel Drums", "Woodblock",
	"Taiko Drum", "Melodic Tom", "Synth Drum", "Reverse Cymbal",
	"Guitar Fret Noise", "Breath Noise", "Seashore", "Bird Tweet",
	"Telephone Ring", "Helicopter", "Applause", "Gunshot",
}

func roundF(v float64) float64 {
	if v < 0 {
		return float64(int64(v - 0.5))
	}
	return float64(int64(v + 0.5))
}
