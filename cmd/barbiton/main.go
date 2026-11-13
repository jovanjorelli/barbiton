package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"barbiton/pkg/encoder"
	"barbiton/pkg/engine"
	"barbiton/pkg/instruments/sampler"
	"barbiton/pkg/midi"
	"barbiton/pkg/score"
)

const Version = "3"

type CLIConfig struct {
	ScorePath  string
	OutputPath string

	RenderMode string
	SampleRate int
	BitDepth   string
	Channels   int

	Humanize      float64
	ReverbPreset  string
	ReverbDamping float64
	NoiseGate     float64
	MasterGain    float64
	Saturation    string

	ImportMIDI string
	ExportMIDI string
	ExportJSON string

	ProfileCPU string
	ProfileMem string
	Verbose    bool
	Benchmark  bool
	Limiter    bool
	Parallel   bool
	MonoBass   string

	BatchDir string

	LoadSFZ string
}

func parseFlags() *CLIConfig {
	cfg := &CLIConfig{}
	flag.StringVar(&cfg.ScorePath, "score", "", "JSON score file")

	flag.StringVar(&cfg.ScorePath, "s", "", "input score file, shorthand")

	flag.StringVar(&cfg.OutputPath, "output", "", "output WAV path")

	flag.StringVar(&cfg.OutputPath, "o", "", "output WAV path, shorthand")

	flag.StringVar(&cfg.RenderMode, "mode", "offline", "render mode: offline|stream")

	flag.IntVar(&cfg.SampleRate, "sample-rate", engine.DefaultSampleRate, "sample rate in Hz: 44100, 48000, or 96000")

	flag.StringVar(&cfg.BitDepth, "bit-depth", "24", "WAV bit depth: 16, 24, 32 (PCM) or 32f (32-bit float)")

	flag.IntVar(&cfg.Channels, "channels", 2, "output channels: 1 (mono), 2 (stereo), 6 (5.1 surround), or 8 (7.1 surround)")

	flag.Float64Var(&cfg.Humanize, "humanize", 0, "humanization strength 0.0..1.0")

	flag.StringVar(&cfg.ReverbPreset, "reverb", "hall", "reverb preset: cathedral|hall|chapel|room|plate|none")

	flag.Float64Var(&cfg.ReverbDamping, "reverb-damping", -1, "reverb HF damping 0.0-1.0 (default -1: preset)")

	flag.Float64Var(&cfg.NoiseGate, "noise-gate", 0, "voice noise gate threshold in dBFS (default 0: off)")

	flag.Float64Var(&cfg.MasterGain, "master-gain", 1.0, "master gain 0.0..1.0")

	flag.StringVar(&cfg.Saturation, "saturation", "tape", "saturation: tape|clean")

	flag.StringVar(&cfg.ImportMIDI, "import-midi", "", "Standard MIDI File to import and render")

	flag.StringVar(&cfg.ExportMIDI, "export-midi", "", "export the loaded score to an SMF (.mid) file")

	flag.StringVar(&cfg.ExportJSON, "export-json", "", "with --import-midi: write imported score as JSON")

	flag.StringVar(&cfg.ProfileCPU, "profile-cpu", "", "write CPU profile to file")

	flag.StringVar(&cfg.ProfileMem, "profile-mem", "", "write memory profile to file")

	flag.BoolVar(&cfg.Verbose, "verbose", false, "verbose output")

	flag.BoolVar(&cfg.Verbose, "v", false, "verbose output, shorthand")

	flag.BoolVar(&cfg.Benchmark, "benchmark", false, "render 10x in memory and report average time")

	flag.BoolVar(&cfg.Limiter, "limiter", false, "mastering limiter after soft clipper")

	flag.BoolVar(&cfg.Parallel, "parallel", false, "parallel per-track voicing, sequential sum")

	flag.StringVar(&cfg.MonoBass, "mono-bass", "off", "mono bass crossover: off or cutoff Hz")

	flag.StringVar(&cfg.BatchDir, "batch-dir", "", "render all score files in directory")

	flag.StringVar(&cfg.LoadSFZ, "load-sfz", "", "validate an SFZ library before rendering")
	flag.Parse()
	return cfg
}

func main() {

	log.SetPrefix("barbiton: ")
	log.SetFlags(0)
	cfg := parseFlags()

	if cfg.ProfileCPU != "" {
		if err := startCPUProfile(cfg.ProfileCPU); err != nil {
			log.Fatalf("profile-cpu: %v", err)
		}
		defer stopCPUProfile()
	}
	if cfg.ProfileMem != "" {
		defer func() {
			if err := writeMemProfile(cfg.ProfileMem); err != nil {
				log.Fatalf("profile-mem: %v", err)
			}
		}()
	}

	if strings.TrimSpace(cfg.BatchDir) != "" {
		if err := runBatch(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}

	if strings.TrimSpace(cfg.LoadSFZ) != "" {
		if err := inspectSFZ(cfg.LoadSFZ, cfg.Verbose); err != nil {
			log.Fatal(err)
		}
	}

	s, err := loadScore(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Humanize > 0 {
		if cfg.Verbose {
			log.Printf("applying humanize strength %.2f", cfg.Humanize)
		}
		s = engine.Humanize(s, float32(cfg.Humanize), 1)
	}

	if cfg.Benchmark {
		avg, err := runBenchmark(cfg, s)
		if err != nil {
			log.Fatalf("benchmark: %v", err)
		}
		fmt.Printf("benchmark: 10 renders, avg %v per render\n", avg.Round(time.Millisecond))
	}

	progress := make(chan float64, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range progress {
			fmt.Fprintf(os.Stderr, "\rRendering: %5.1f%%", p*100)
		}
		fmt.Fprintln(os.Stderr)
	}()

	stats, err := renderToFile(cfg, s, progress)
	close(progress)
	<-done
	if err != nil {
		log.Fatalf("render: %v", err)
	}

	if cfg.ExportMIDI != "" {
		if err := exportMIDIFile(s, cfg.ExportMIDI); err != nil {
			log.Fatalf("midi export: %v", err)
		}
		if cfg.Verbose {
			log.Printf("exported MIDI to %s", cfg.ExportMIDI)
		}
	}

	fmt.Printf("Rendered %d tracks -> %s (%.2f s, peak %.3f, %v)\n",
		len(s.Tracks), stats.path, stats.seconds, stats.peak,
		stats.elapsed.Round(time.Millisecond))
}

func inspectSFZ(path string, verbose bool) error {
	f, err := sampler.LoadSFZ(path)
	if err != nil {
		return fmt.Errorf("load-sfz: %w", err)
	}
	samples := 0
	for i := range f.Regions {
		if f.Regions[i].Sample != nil {
			samples++
		}
		if verbose {
			log.Printf("region %d: %q keys %d-%d", i,
				f.Regions[i].SamplePath, f.Regions[i].LoKey, f.Regions[i].HiKey)
		}
	}
	fmt.Printf("SFZ %s: %d regions, %d decoded samples\n", path, len(f.Regions), samples)
	return nil
}

func loadScore(cfg *CLIConfig) (*score.Score, error) {
	if strings.TrimSpace(cfg.ImportMIDI) != "" {
		data, err := os.ReadFile(cfg.ImportMIDI)
		if err != nil {
			return nil, fmt.Errorf("midi import: %w", err)
		}
		f, err := midi.ReadFile(data)
		if err != nil {
			return nil, fmt.Errorf("midi import: %w", err)
		}
		s, err := midi.ToScore(f)
		if err != nil {
			return nil, fmt.Errorf("midi import: %w", err)
		}
		if err := score.Validate(s); err != nil {
			return nil, fmt.Errorf("imported score invalid: %w", err)
		}
		if cfg.Verbose {
			log.Printf("imported %s: %q, %d tracks, %d notes",
				cfg.ImportMIDI, s.Metadata.Title, len(s.Tracks), s.TotalNotes())
		}
		if strings.TrimSpace(cfg.ExportJSON) != "" {
			js, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				return nil, fmt.Errorf("export json: %w", err)
			}
			js = append(js, '\n')
			if err := os.WriteFile(cfg.ExportJSON, js, 0o644); err != nil {
				return nil, fmt.Errorf("export json: %w", err)
			}
			if cfg.Verbose {
				log.Printf("wrote imported score to %s", cfg.ExportJSON)
			}
		}
		return s, nil
	}
	if strings.TrimSpace(cfg.ScorePath) == "" {
		return nil, fmt.Errorf("no input: provide --score=path or --import-midi=path (use -h for help)")
	}
	s, err := score.ParseFile(cfg.ScorePath)
	if err != nil {
		return nil, fmt.Errorf("score parse: %w", err)
	}
	if err := score.Validate(s); err != nil {
		return nil, fmt.Errorf("invalid score: %w", err)
	}
	if cfg.Verbose {
		log.Printf("loaded %s: %q, %g BPM %s, %d tracks, %d notes, %.2f s",
			cfg.ScorePath, s.Metadata.Title, s.Metadata.BPM, s.Metadata.TimeSignature,
			len(s.Tracks), s.TotalNotes(), s.Duration())
	}
	return s, nil
}

type renderStats struct {
	path    string
	frames  int
	seconds float64
	peak    float32
	elapsed time.Duration
}

func renderToFile(cfg *CLIConfig, s *score.Score, progress chan<- float64) (renderStats, error) {
	var stats renderStats
	mode := strings.ToLower(strings.TrimSpace(cfg.RenderMode))
	if mode == "" {
		mode = "offline"
	}
	if mode != "offline" && mode != "stream" {
		return stats, fmt.Errorf("unknown --mode %q (want offline|stream)", cfg.RenderMode)
	}
	var spec encoder.Spec
	var err error
	if spec, err = encoder.Resolve(cfg.BitDepth, cfg.Channels, cfg.SampleRate); err != nil {
		return stats, err
	}
	sampleRate, channels := spec.SampleRate, spec.Channels
	out := strings.TrimSpace(cfg.OutputPath)
	if out == "" {
		out = "out.wav"
	}
	if cfg.Verbose {
		log.Printf("render mode=%s rate=%d bit=%s ch=%d reverb=%s gain=%.2f -> %s",
			mode, sampleRate, cfg.BitDepth, channels, cfg.ReverbPreset, cfg.MasterGain, out)
	}

	eng, err := engine.New(s, float64(sampleRate), engine.DefaultBlockSize)
	if err != nil {
		return stats, err
	}
	applyReverbPreset(eng, cfg.ReverbPreset)
	if err := applySaturation(eng, cfg.Saturation); err != nil {
		return stats, err
	}
	if err := applyReverbDamping(eng, cfg.ReverbDamping); err != nil {
		return stats, err
	}
	if err := applyNoiseGate(eng, cfg.NoiseGate); err != nil {
		return stats, err
	}
	eng.Mixer().SetLimiterEnabled(cfg.Limiter)
	eng.Mixer().SetParallel(cfg.Parallel)
	if err := applyMonoBass(eng, cfg.MonoBass); err != nil {
		return stats, err
	}
	gain := float32(cfg.MasterGain)
	if !(gain >= 0) || gain > 1 {
		if gain < 0 {
			gain = 0
		} else {
			gain = 1
		}
	}

	f, err := os.Create(out)
	if err != nil {
		return stats, err
	}
	defer f.Close()
	w, err := spec.NewWriter(f)
	if err != nil {
		return stats, err
	}

	block := eng.BlockSize()
	left := make([]float32, block)
	right := make([]float32, block)
	var mono []float32
	if channels == 1 {
		mono = make([]float32, block)
	}
	total := eng.TotalFrames()
	done := 0
	peak := float32(0)
	start := time.Now()

	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)

	report := func() {
		if progress == nil || total <= 0 {
			return
		}
		select {
		case progress <- float64(done) / float64(total):
		default:
		}
	}

	for done < total {
		n := block
		if total-done < n {
			n = total - done
		}
		var writeErr error
		eng.ProcessFrames(n, func(master []float32) {
			frames := len(master) / 2
			for i := 0; i < frames; i++ {
				l := master[2*i] * gain
				r := master[2*i+1] * gain
				left[i], right[i] = l, r
				if a := abs32(l); a > peak {
					peak = a
				}
				if a := abs32(r); a > peak {
					peak = a
				}
			}
			if channels == 1 {
				encoder.DownmixMono(mono[:frames], left[:frames], right[:frames], frames)
				_, writeErr = w.WriteFrames(mono[:frames])
			} else {
				_, writeErr = w.WriteFrames(left[:frames], right[:frames])
			}
		})
		if writeErr != nil {
			w.Close()
			f.Close()
			return stats, writeErr
		}
		if err := eng.Err(); err != nil {
			w.Close()
			f.Close()
			return stats, err
		}
		done += n
		report()
	}
	if err := w.Close(); err != nil {
		f.Close()
		return stats, err
	}
	if err := f.Close(); err != nil {
		return stats, err
	}
	elapsed := time.Since(start)
	stats = renderStats{
		path:    out,
		frames:  total,
		seconds: float64(total) / float64(sampleRate),
		peak:    peak,
		elapsed: elapsed,
	}
	return stats, nil
}

func applyReverbPreset(eng *engine.Engine, preset string) {
	name := strings.ToLower(strings.TrimSpace(preset))
	if name == "" {
		return
	}
	if name == "none" || name == "off" || name == "dry" {
		eng.Mixer().SetReverbWet(0)
		return
	}
	if !eng.Mixer().SetReverbPreset(name) {
		log.Printf("unknown --reverb %q, keeping default", preset)
	}
}

func runBenchmark(cfg *CLIConfig, s *score.Score) (time.Duration, error) {
	var sampleRate int
	switch cfg.SampleRate {
	case 44100, 48000, 96000:
		sampleRate = cfg.SampleRate
	default:
		return 0, fmt.Errorf("unsupported --sample-rate %d (want 44100, 48000, or 96000)", cfg.SampleRate)
	}
	const runs = 10
	debug.SetGCPercent(-1)
	defer debug.SetGCPercent(100)
	start := time.Now()
	for i := 0; i < runs; i++ {
		eng, err := engine.New(s, float64(sampleRate), engine.DefaultBlockSize)
		if err != nil {
			return 0, err
		}
		applyReverbPreset(eng, cfg.ReverbPreset)
		eng.Mixer().SetParallel(cfg.Parallel)
		total := eng.TotalFrames()
		done := 0
		for done < total {
			n := eng.BlockSize()
			if total-done < n {
				n = total - done
			}
			eng.ProcessFrames(n, nil)
			done += n
		}
	}
	return time.Since(start) / runs, nil
}

func applySaturation(eng *engine.Engine, mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "tape":
		eng.Mixer().SetSaturationBypass(false)
		return nil
	case "clean":
		eng.Mixer().SetSaturationBypass(true)
		return nil
	default:
		return fmt.Errorf("unknown --saturation %q (want tape|clean)", mode)
	}
}

func applyReverbDamping(eng *engine.Engine, d float64) error {
	if d < 0 {
		return nil
	}
	if d > 1 || math.IsNaN(d) {
		return fmt.Errorf("unknown --reverb-damping %v (want 0.0-1.0)", d)
	}
	eng.Mixer().SetReverbDamping(float32(d))
	return nil
}

func applyNoiseGate(eng *engine.Engine, db float64) error {
	if db == 0 {
		return nil
	}
	if !(db < 0) {
		return fmt.Errorf("unknown --noise-gate %v (want 0 or negative dBFS)", db)
	}
	eng.SetNoiseGateDB(db)
	return nil
}

func applyMonoBass(eng *engine.Engine, s string) error {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "" || v == "off" {
		return nil
	}
	hz, err := strconv.Atoi(v)
	if err != nil || hz <= 0 {
		return fmt.Errorf("unknown --mono-bass %q (want off or cutoff Hz)", s)
	}
	eng.Mixer().SetMonoBassCutoff(float32(hz))
	return nil
}

func exportMIDIFile(s *score.Score, outPath string) error {
	f := midi.FromScore(s, midi.DefaultTicksPerQuarter)
	data := midi.WriteFile(f)
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	events := 0
	for _, tr := range f.Tracks {
		events += len(tr)
	}
	fmt.Printf("exported MIDI -> %s: format %d, %d PPQ, %d tracks, %d events, %d bytes\n",
		outPath, f.Header.Format, f.Header.TicksPerQuarter,
		len(f.Tracks), events, len(data))
	return nil
}

func importMIDIScoreForBatch(path string) (*score.Score, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := midi.ReadFile(data)
	if err != nil {
		return nil, err
	}
	return midi.ToScore(f)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func outputPathForBatch(inPath, outDir string) string {
	base := strings.TrimSuffix(filepath.Base(inPath), filepath.Ext(inPath)) + ".wav"
	if strings.TrimSpace(outDir) == "" {
		return filepath.Join(filepath.Dir(inPath), base)
	}
	if info, err := os.Stat(outDir); err == nil && info.IsDir() {
		return filepath.Join(outDir, base)
	}
	return outDir
}
