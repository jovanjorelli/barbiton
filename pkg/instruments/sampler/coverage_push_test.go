package sampler

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
	"barbiton/pkg/wav"
)

func TestPushCommentOpcode(t *testing.T) {
	f, err := ParseSFZ([]byte("comment=hello world\n<region> sample=x.wav lokey=60\n"))
	if err != nil {
		t.Fatalf("ParseSFZ: %v", err)
	}
	if len(f.Regions) != 1 {
		t.Fatalf("regions = %d, want 1", len(f.Regions))
	}
}

func TestPushDefaultPathOpcode(t *testing.T) {
	dir := t.TempDir()
	writeTestWav(t, filepath.Join(dir, "sub_c4.wav"), wav.PCM24, 48000, 500, 1, 261.63)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestWav(t, filepath.Join(sub, "c4.wav"), wav.PCM24, 48000, 500, 1, 261.63)
	sfz := "<control> default_path=sub/\n<region> sample=c4.wav lokey=60\n"
	path := filepath.Join(dir, "d.sfz")
	if err := os.WriteFile(path, []byte(sfz), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadSFZ(path)
	if err != nil {
		t.Fatalf("LoadSFZ: %v", err)
	}
	if len(f.Regions) != 1 || f.Regions[0].Sample == nil {
		t.Fatalf("default_path region = %+v", f.Regions)
	}
}

func TestPushLFOOpcodes(t *testing.T) {
	f, err := ParseSFZ([]byte("<region> sample=x.wav lokey=60 lfo01_freq=5.5 lfo01_wave=2 lfo01_freq_wave=bad lfo02_delay=0.1\n"))
	if err != nil {
		t.Fatalf("ParseSFZ: %v", err)
	}
	if len(f.Regions) != 1 {
		t.Fatalf("regions = %d, want 1", len(f.Regions))
	}
	r := f.Regions[0]
	if r.LFO[0].Freq != 5.5 {
		t.Fatalf("lfo freq = %v, want 5.5", r.LFO[0].Freq)
	}
	if r.LFO[0].Wave != 2 {
		t.Fatalf("lfo wave = %v, want 2", r.LFO[0].Wave)
	}
}

func TestPushParseIncludePathVariants(t *testing.T) {
	if got := parseIncludePath(`#include "lib/strings.sfz" ; comment`); got != "lib/strings.sfz" {
		t.Fatalf("quoted = %q", got)
	}
	if got := parseIncludePath(`#include 'lib/piano.sfz'`); got != "lib/piano.sfz" {
		t.Fatalf("squote = %q", got)
	}
	if got := parseIncludePath(`#include`); got != "" {
		t.Fatalf("bare = %q, want empty", got)
	}
	if got := parseIncludePath(`#include "unterminated`); got != "" {
		t.Fatalf("unterminated = %q, want empty", got)
	}
	if got := parseIncludePath(`#include lib/bass.sfz // voice`); got != "lib/bass.sfz" {
		t.Fatalf("unquoted = %q", got)
	}
}

func TestPushResolveEdges(t *testing.T) {
	r := NewPathResolver(t.TempDir())
	if _, err := r.Resolve("   "); err == nil {
		t.Fatal("empty resolve accepted")
	}
	if _, err := r.Resolve(filepath.Join(t.TempDir(), "nope-not-here.wav")); err == nil {
		t.Fatal("absolute missing resolve accepted")
	}
}

func TestPushResolveCacheCounters(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Bank"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Bank", "Violin.wav")
	if err := os.WriteFile(want, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewPathResolver(dir)
	beforeMiss := r.Misses()
	beforeHit := r.Hits()
	p1, err := r.Resolve("bank/violin.wav")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if r.Misses() != beforeMiss+1 {
		t.Fatalf("misses = %d, want %d", r.Misses(), beforeMiss+1)
	}
	p2, err := r.Resolve("bank/violin.wav")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if p1 != p2 {
		t.Fatalf("cached = %q, want %q", p2, p1)
	}
	if r.Hits() != beforeHit+1 {
		t.Fatalf("hits = %d, want %d", r.Hits(), beforeHit+1)
	}
	if r2 := r.ForDir(dir); r2 == nil {
		t.Fatal("ForDir returned nil")
	}
}

func TestPushReadSampleBoundedEdges(t *testing.T) {
	src := []float32{0, 10, 20, 30}
	if got := readSampleBounded(src, 5, 3, 3); got != float64(src[3]) {
		t.Fatalf("degenerate hi<=lo = %v", got)
	}
	if got := readSampleBounded(src, -5, 0, 4); got != 0 {
		t.Fatalf("low clamp = %v", got)
	}
	if got := readSampleBounded(src, 99, 0, 4); got != 30 {
		t.Fatalf("high clamp = %v", got)
	}
}

func TestPushXfadeGainZones(t *testing.T) {
	rg := &Region{XfinLoVel: 0, XfinHiVel: 100, XfoutLoVel: 0, XfoutHiVel: 127}
	if got := xfadeGain(rg, 60, 50); got <= 0 || got > 1 {
		t.Fatalf("mid gain = %v, want (0,1]", got)
	}
	flat := &Region{}
	if got := xfadeGain(flat, 60, 80); got != 1 {
		t.Fatalf("flat gain = %v, want 1", got)
	}
	swapped := &Region{XfinLoVel: 100, XfinHiVel: 0}
	if got := xfadeGain(swapped, 60, 50); got <= 0 || got > 1 {
		t.Fatalf("swapped gain = %v, want (0,1]", got)
	}
}

func TestPushSetParametersClamps(t *testing.T) {
	s := New(48000)
	s.SetParameters(map[string]float32{"master_volume": 5, "attack": -1, "release": 99, "unknown_key": 1})
	if s.masterGain != 1 {
		t.Fatalf("master = %v, want 1", s.masterGain)
	}
	s.SetParameters(map[string]float32{"master_volume": -2})
	if s.masterGain != 0 {
		t.Fatalf("master = %v, want 0", s.masterGain)
	}
}

func TestPushTrackRegisterIdempotent(t *testing.T) {
	s := New(48000)
	v := &s.voices[0]
	v.ptrack = &s.pitchTrack
	v.trackedPitch = 60
	v.trackRegister(60)
	n := s.pitchTrack[60].n
	v.trackRegister(60)
	if s.pitchTrack[60].n != n {
		t.Fatalf("re-register changed n %d -> %d", n, s.pitchTrack[60].n)
	}
}

func TestPushFinishStealFree(t *testing.T) {
	s := New(48000)
	v := &s.voices[0]
	v.Active = true
	v.isStealing = true
	v.stealFree = true
	v.finishSteal()
	if v.Active || v.isStealing || v.stealFree {
		t.Fatalf("steal-free not cleared: %+v", v)
	}
}

func TestPushLoadSFZMissing(t *testing.T) {
	if _, err := LoadSFZ(filepath.Join(t.TempDir(), "missing.sfz")); err == nil {
		t.Fatal("missing sfz accepted")
	}
}

func TestPushNewDefaultsRate(t *testing.T) {
	for _, sr := range []float64{0, -1, -48000} {
		s := New(sr)
		if s.sampleRate != 48000 {
			t.Fatalf("New(%v).sampleRate = %v, want 48000", sr, s.sampleRate)
		}
	}
}

func TestPushVoiceProcessGuards(t *testing.T) {
	buf := []float32{9, 9, 9, 9}
	v := &Voice{}
	v.Process(buf, 0.01)
	v.Active = true
	v.Process(buf, 0.01)
	v.Region = &Region{}
	v.Process(buf, 0.01)
	v.Process(nil, 0.01)
	v.Process(buf, 0)
	v.Process(buf, -1)
	for i, x := range buf {
		if x != 9 {
			t.Fatalf("buf[%d] = %v, want untouched 9", i, x)
		}
	}
}

func TestPushTrackRegisterEdges(t *testing.T) {
	s := New(48000)
	v := &s.voices[1]
	v.ptrack = nil
	v.trackRegister(60)
	v.ptrack = &s.pitchTrack
	v.trackRegister(-1)
	v.trackRegister(200)
	if s.pitchTrack[60].n != 0 {
		t.Fatal("out-of-range register polluted pitch 60")
	}

	for i := 0; i < maxPitchVoices; i++ {
		w := &s.voices[2+i]
		w.ptrack = &s.pitchTrack
		w.trackedPitch = -1
		w.trackRegister(61)
	}
	if s.pitchTrack[61].n != maxPitchVoices {
		t.Fatalf("roster n = %d, want %d", s.pitchTrack[61].n, maxPitchVoices)
	}
	extra := &Voice{ptrack: &s.pitchTrack, trackedPitch: -1}
	extra.trackRegister(61)
	if s.pitchTrack[61].n != maxPitchVoices {
		t.Fatalf("cap overflow n = %d, want %d", s.pitchTrack[61].n, maxPitchVoices)
	}
}

func TestPushNoteOnParamsGuards(t *testing.T) {
	s := New(48000)
	np := instruments.NoteParams{}
	s.NoteOnParams(60, 0.8, np)
	s.NoteOnParams(-1, 0.8, np)
	s.NoteOnParams(200, 0.8, np)
	if got := s.ActiveVoices(); got != 0 {
		t.Fatalf("voices = %d, want 0", got)
	}
}

func TestPushClampFSlashClean(t *testing.T) {
	if got := clampF(99, 0, 1); got != 1 {
		t.Fatalf("clamp high = %v", got)
	}
	if got := clampF(-9, 0, 1); got != 0 {
		t.Fatalf("clamp low = %v", got)
	}
	if got := clampF(0.5, 0, 1); got != 0.5 {
		t.Fatalf("clamp mid = %v", got)
	}
	if got := clampF(math.NaN(), 0, 1); got != 0 {
		t.Fatalf("clamp NaN = %v, want lo", got)
	}
	for in, want := range map[string]string{
		"":         "",
		"/":        "/",
		"a//b":     "a/b",
		"a/./b":    "a/b",
		"a/b/../c": "a/c",
		"..":       "..",
	} {
		if got := slashClean(in); got != want {
			t.Fatalf("slashClean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPushDefaultPathNoSlash(t *testing.T) {
	var ctx ParseContext
	handleDefaultPath(&ctx, []byte("sub"))
	if ctx.defaultPath != "sub/" {
		t.Fatalf("defaultPath = %q, want sub/", ctx.defaultPath)
	}
	handleDefaultPath(&ctx, []byte("sub/"))
	if ctx.defaultPath != "sub/" {
		t.Fatalf("defaultPath = %q, want sub/", ctx.defaultPath)
	}
	handleComment(&ctx, []byte("anything"))
}

func TestPushBadValueOpcodes(t *testing.T) {
	f, err := ParseSFZ([]byte("<region> sample=x.wav lokey=60 lfo01_freq=bad seq_length=0 keycenter=zz9 ampeg_attack=bad\n"))
	if err != nil {
		t.Fatalf("ParseSFZ: %v", err)
	}
	if len(f.Regions) != 1 {
		t.Fatalf("regions = %d, want 1", len(f.Regions))
	}
}

func TestPushXfadeKeyZones(t *testing.T) {
	rg := &Region{XfinLoKey: 40, XfinHiKey: 80, XfoutLoKey: 0, XfoutHiKey: 127}
	lo := xfadeGain(rg, 40, 80)
	mid := xfadeGain(rg, 60, 80)
	hi := xfadeGain(rg, 80, 80)
	if !(lo <= mid && mid <= hi) {
		t.Fatalf("key gains not rising: %v %v %v", lo, mid, hi)
	}
	if got := xfadeGain(rg, 200, 80); got < 0 || got > 1 {
		t.Fatalf("out-of-zone gain = %v", got)
	}
}

func TestPushResolveAbsolute(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "Kick.wav")
	if err := os.WriteFile(abs, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewPathResolver(dir)
	got, err := r.Resolve(abs)
	if err != nil {
		t.Fatalf("absolute resolve: %v", err)
	}
	if got != abs {
		t.Fatalf("absolute = %q, want %q", got, abs)
	}
}

func TestPushWindowsResolver(t *testing.T) {
	dir := t.TempDir()
	r := &PathResolver{basePath: dir, cache: &resolveCache{entries: make(map[string]string)}, isWindows: true}
	if _, err := r.Resolve("missing.wav"); err == nil {
		t.Fatal("windows missing resolve accepted")
	}
}

func TestPushDotDotWalk(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Bank"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Bank", "Violin.wav")
	if err := os.WriteFile(want, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewPathResolver(dir)
	got, err := r.Resolve("bank/../BANK/violin.wav")
	if err != nil {
		t.Fatalf("dotdot resolve: %v", err)
	}
	if got != want {
		t.Fatalf("dotdot = %q, want %q", got, want)
	}
}

func TestPushXfadeExtremeClamps(t *testing.T) {
	rg := &Region{XfinLoVel: 10, XfinHiVel: 100, XfoutLoVel: 20, XfoutHiVel: 110,
		XfinLoKey: 30, XfinHiKey: 90, XfoutLoKey: 10, XfoutHiKey: 100}
	for _, tc := range [][2]int{{-5, 60}, {500, 60}, {80, -10}, {80, 200}} {
		if got := xfadeGain(rg, tc[1], tc[0]); got < 0 || got > 1 {
			t.Fatalf("xfadeGain(%d,%d) = %v out of [0,1]", tc[1], tc[0], got)
		}
	}
}

func pushRegion(sd *SampleData, lo, hi int) Region {
	r := newRegion()
	r.SamplePath, r.Sample = "t.wav", sd
	r.LoKey, r.HiKey, r.PitchKeyCenter = lo, hi, lo
	return r
}

func pushSample(frames int) *SampleData {
	sd := &SampleData{Samples: make([]float32, frames), SampleRate: 48000, Channels: 1}
	for i := range sd.Samples {
		sd.Samples[i] = 0.5
	}
	return sd
}

func TestPushNoteOnVelocityClamp(t *testing.T) {
	s := New(48000)
	s.Load(&SFZFile{Regions: []Region{pushRegion(pushSample(48000), 0, 127)}})
	s.NoteOn(60, 5.0)
	if got := s.ActiveVoices(); got != 1 {
		t.Fatalf("voices = %d, want 1", got)
	}
}

func TestPushOffByTrigger(t *testing.T) {
	sd := pushSample(48000)
	r1 := pushRegion(sd, 60, 69)
	r2 := pushRegion(sd, 60, 69)
	r2.OffBy = 5
	s := New(48000)
	s.Load(&SFZFile{Regions: []Region{r1, r2}})
	s.NoteOn(60, 0.9)
	s.NoteOn(61, 0.9)
	if got := s.ActiveVoices(); got < 1 {
		t.Fatalf("voices = %d, want >= 1", got)
	}
}

func TestPushSamplerProcessGuards(t *testing.T) {
	s := New(48000)
	s.Process(nil, 0.01)
	s.Process(dsp.NewStereoBuffer(8), 0)
	s.Process(dsp.NewStereoBuffer(8), -1)
	big := dsp.NewStereoBuffer(9000)
	big.SetLen(9000)
	s.Process(big, 9000.0/48000)
	if len(s.mix) < 9000 {
		t.Fatalf("mix len = %d, want >= 9000 after grow", len(s.mix))
	}
}

func TestPushVoiceStealHandoff(t *testing.T) {
	s := New(48000)
	v := &s.voices[3]
	tiny := pushSample(4)
	v.Region = &Region{Sample: tiny}
	v.end = 1
	v.Phase = 99
	v.Active = true
	v.isStealing = true
	v.fadeStep = 0
	v.pendingRegion = &Region{Sample: pushSample(8)}
	v.pendingPitch, v.pendingVelocity = 64, 0.8
	v.stealSampleRate = 0
	v.Process(make([]float32, 64), 64.0/48000)
	if v.stealFadeProgress < 0 || v.stealFadeProgress > 1 {
		t.Fatalf("fade progress = %v out of [0,1]", v.stealFadeProgress)
	}
}

func TestPushRenderToCompletion(t *testing.T) {
	s := New(48000)
	s.Load(&SFZFile{Regions: []Region{pushRegion(pushSample(100), 60, 60)}})
	s.NoteOn(60, 0.9)
	buf := dsp.NewStereoBuffer(512)
	buf.SetLen(512)
	dt := 512.0 / 48000
	for i := 0; i < 10; i++ {
		s.Process(buf, dt)
	}
	s.NoteOff(60)
	for i := 0; i < 500 && s.ActiveVoices() > 0; i++ {
		s.Process(buf, dt)
	}
	if got := s.ActiveVoices(); got != 0 {
		t.Fatalf("voices = %d, want 0 after full render", got)
	}
}

func TestPushInitVoiceLoopClamp(t *testing.T) {
	s := New(48000)
	sd := pushSample(100)
	sd.LoopStart, sd.LoopEnd = 10, 80
	rg := newRegion()
	rg.Sample = sd
	rg.LoopMode = LoopContinuous
	rg.End = 50
	v := &s.voices[4]
	initVoice(v, &rg, 60, 0.8, 48000, 0.01, 0.1, 0.8, 0.2, 0, instruments.NoteParams{})
	if !v.looping || v.loopEnd != 50 {
		t.Fatalf("looping=%v end=%v, want true/50", v.looping, v.loopEnd)
	}
	rg2 := newRegion()
	rg2.Sample = sd
	rg2.LoopMode = LoopContinuous
	rg2.End = 50
	sd.LoopStart = 60
	v2 := &s.voices[5]
	initVoice(v2, &rg2, 60, 0.8, 48000, 0.01, 0.1, 0.8, 0.2, 0, instruments.NoteParams{})
	if v2.looping {
		t.Fatal("looping with start>=end accepted")
	}
}

func TestPushInterpDirect(t *testing.T) {
	src := []float32{1, 2, 3, 4}
	if got := interpSampleBounded(src, -3, 0, 4); got != 1 {
		t.Fatalf("low = %v, want 1", got)
	}
	if got := interpSampleBounded(src, 99, 0, 4); got != 4 {
		t.Fatalf("high = %v, want 4", got)
	}
	if n := resampleTo(make([]float32, 8), nil, 1, 0, false); n != 0 {
		t.Fatalf("empty src n = %d", n)
	}
	if n := resampleTo(make([]float32, 8), src, 0, 0, false); n != 0 {
		t.Fatalf("bad rate n = %d", n)
	}
	if n := resampleTo(make([]float32, 8), src, 1, math.NaN(), false); n != 0 {
		t.Fatalf("NaN phase n = %d", n)
	}
}

func TestPushExprNaNEnvelope(t *testing.T) {
	s := New(48000)
	s.Load(&SFZFile{Regions: []Region{pushRegion(pushSample(48000), 0, 127)}})
	np := instruments.NoteParams{HasExpression: true, Expression: float32(math.NaN())}
	np.Env = instruments.EnvelopeOverride{Attack: 0.05, HasAttack: true, Release: -1, HasRelease: true}
	s.NoteOnParams(60, 0.9, np)
	if got := s.ActiveVoices(); got != 1 {
		t.Fatalf("voices = %d, want 1", got)
	}
}

func TestPushBoundedLengthClamp(t *testing.T) {
	src := []float32{1, 2, 3, 4}
	if got := readSampleBounded(src, 8, 0, 10); got != 4 {
		t.Fatalf("over-len = %v, want 4", got)
	}
	if got := readSampleBounded(src, -3, -5, 4); got != 1 {
		t.Fatalf("under-len = %v, want 1", got)
	}
	if n := resampleTo(make([]float32, 4), src, 1, -5, false); n != 4 {
		t.Fatalf("neg phase n = %d, want 4", n)
	}
}

func TestPushWalkDotAndNotDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Bank"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "Bank", "Violin.wav")
	if err := os.WriteFile(want, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewPathResolver(dir)
	got, err := r.Resolve("BANK/./violin.wav")
	if err != nil {
		t.Fatalf("dot resolve: %v", err)
	}
	if got != want {
		t.Fatalf("dot = %q, want %q", got, want)
	}
	plain := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2 := NewPathResolver(plain)
	if _, err := r2.Resolve("sub/file.wav"); err == nil {
		t.Fatal("file-as-dir resolve accepted")
	}
}

func TestPushSignedEGBadValue(t *testing.T) {
	f, err := ParseSFZ([]byte("<region> sample=x.wav lokey=60 ampeg_vel2attack=bad pitcheg_depth=bad\n"))
	if err != nil {
		t.Fatalf("ParseSFZ: %v", err)
	}
	if len(f.Regions) != 1 {
		t.Fatalf("regions = %d, want 1", len(f.Regions))
	}
}
