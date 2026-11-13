package mixer

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"

	"barbiton/pkg/dsp"
	"barbiton/pkg/instruments"
)

const (
	DefaultClipThreshold float32 = 0.9

	DefaultReverbWet float32 = 0.25

	maxTrackRooms = 8

	minParallelTracks = 8
)

type TrackBuffer struct {
	instrument      instruments.Instrument
	buffer          []float32
	scratchL        []float32
	scratchR        []float32
	scratch         dsp.StereoBuffer
	pan             float32
	volume          float32
	send            float32
	delay           *dsp.StereoDelay
	delayWet        float32
	muted           bool
	reverbIdx       int
	sidechainSource int
	sidechain       *dsp.SidechainCompressor
	distortion      *dsp.Distortion
	ott             *dsp.OTT
	turntable       *dsp.Turntable
}

type Mixer struct {
	tracks       []TrackBuffer
	masterBuffer []float32
	sendPool     []float32
	sampleRate   int

	maxBlock       int
	lastN          int
	pool           []float32
	poolUsed       int
	render         *dsp.StereoBuffer
	clipper        SoftClipper
	limiter        *dsp.Limiter
	useLimiter     bool
	useParallel    bool
	parWg          sync.WaitGroup
	parErrMu       sync.Mutex
	parErr         error
	monoBassHz     float32
	monoBassA      float32
	bassYL         float32
	bassYR         float32
	reverb         *dsp.Reverb
	reverbs        []*dsp.Reverb
	reverbWet      float32
	convReverb     *dsp.ConvolutionReverb
	useConvolution bool
	multiband      *dsp.MultibandDynamics
	useMultiband   bool
	ott            *dsp.OTT
	useOTT         bool
	turntable      *dsp.Turntable
	useTurntable   bool
}

func New(sampleRate, maxTracks, maxBlock int) *Mixer {
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if maxTracks < 1 {
		maxTracks = 1
	}
	if maxBlock < 1 {
		maxBlock = 1
	}
	hall, _ := dsp.PresetParams("hall")
	master := dsp.NewReverb(float64(sampleRate), hall)
	mb := dsp.NewMultibandDynamics(float64(sampleRate), dsp.DefaultMultibandConfig())
	conv := dsp.NewConvolutionReverb(float64(sampleRate), dsp.DefaultConvolutionConfig())
	ottProcessor := dsp.NewOTT(float64(sampleRate), dsp.DefaultOTTConfig())
	ttProcessor := dsp.NewTurntable(float64(sampleRate))
	return &Mixer{
		tracks:         make([]TrackBuffer, 0, maxTracks),
		masterBuffer:   make([]float32, 2*maxBlock),
		sendPool:       make([]float32, (maxTrackRooms+1)*2*maxBlock),
		sampleRate:     sampleRate,
		maxBlock:       maxBlock,
		pool:           make([]float32, maxTracks*4*maxBlock),
		render:         dsp.NewStereoBuffer(maxBlock),
		clipper:        *NewSoftClipper(DefaultClipThreshold),
		limiter:        dsp.NewLimiter(float64(sampleRate)),
		reverb:         master,
		reverbs:        []*dsp.Reverb{master},
		reverbWet:      DefaultReverbWet,
		convReverb:     conv,
		useConvolution: false,
		multiband:      mb,
		useMultiband:   false,
		ott:            ottProcessor,
		useOTT:         false,
		turntable:      ttProcessor,
		useTurntable:   false,
	}
}

func (m *Mixer) AddTrack(instrument instruments.Instrument, pan, volume float32) int {
	if instrument == nil || len(m.tracks) == cap(m.tracks) {
		return -1
	}
	size := 2 * m.maxBlock
	buf := m.pool[m.poolUsed : m.poolUsed+size]
	m.poolUsed += size
	sl := m.pool[m.poolUsed : m.poolUsed+m.maxBlock]
	m.poolUsed += m.maxBlock
	sr := m.pool[m.poolUsed : m.poolUsed+m.maxBlock]
	m.poolUsed += m.maxBlock
	m.tracks = append(m.tracks, TrackBuffer{
		instrument:      instrument,
		buffer:          buf,
		scratchL:        sl,
		scratchR:        sr,
		scratch:         dsp.StereoBuffer{Left: sl, Right: sr},
		pan:             clamp32(pan, -1, 1),
		volume:          clamp32(volume, 0, 1),
		sidechainSource: -1,
	})
	return len(m.tracks) - 1
}

func (m *Mixer) TrackCount() int { return len(m.tracks) }

func (m *Mixer) SampleRate() int { return m.sampleRate }

func (m *Mixer) SetPan(i int, pan float32) {
	if m.valid(i) {
		m.tracks[i].pan = clamp32(pan, -1, 1)
	}
}

func (m *Mixer) SetVolume(i int, volume float32) {
	if m.valid(i) {
		m.tracks[i].volume = clamp32(volume, 0, 1)
	}
}

func (m *Mixer) SetTrackInstrument(i int, instrument instruments.Instrument) {
	if m.valid(i) && instrument != nil {
		m.tracks[i].instrument = instrument
	}
}

func (m *Mixer) SetMuted(i int, muted bool) {
	if m.valid(i) {
		m.tracks[i].muted = muted
	}
}

func (m *Mixer) SetSend(i int, send float32) {
	if m.valid(i) {
		m.tracks[i].send = clamp32(send, 0, 1)
	}
}

func (m *Mixer) TrackSend(i int) float32 {
	if !m.valid(i) {
		return 0
	}
	return m.tracks[i].send
}

func (m *Mixer) SetTrackDelay(i int, cfg dsp.DelayConfig, bpm float64) {
	if !m.valid(i) {
		return
	}
	d := dsp.NewStereoDelay(m.sampleRate, dsp.MaxDelaySeconds)
	d.Configure(cfg, bpm)
	m.tracks[i].delay = d
	m.tracks[i].delayWet = d.Wet()
}

func (m *Mixer) roomBus(r, n int) []float32 {
	base := r * 2 * m.maxBlock
	return m.sendPool[base : base+2*n]
}

func (m *Mixer) nearestRoom(p dsp.ReverbParams) int {
	best, bestD := 1, math.MaxFloat64
	for r := 1; r < len(m.reverbs); r++ {
		q := m.reverbs[r].Params()
		ds := float64(q.RoomSize - p.RoomSize)
		dd := float64(q.Damping - p.Damping)
		dw := float64(q.Width - p.Width)
		if d := ds*ds + dd*dd + dw*dw; d < bestD {
			best, bestD = r, d
		}
	}
	return best
}

func (m *Mixer) SetTrackRoom(i int, p dsp.ReverbParams) int {
	if !m.valid(i) {
		return 0
	}
	for r := 1; r < len(m.reverbs); r++ {
		if m.reverbs[r].Params() == p {
			m.tracks[i].reverbIdx = r
			return r
		}
	}
	if len(m.reverbs) > maxTrackRooms {
		r := m.nearestRoom(p)
		m.tracks[i].reverbIdx = r
		return r
	}
	m.reverbs = append(m.reverbs, dsp.NewReverb(float64(m.sampleRate), p))
	m.tracks[i].reverbIdx = len(m.reverbs) - 1
	return len(m.reverbs) - 1
}

func (m *Mixer) SetTrackRoomNone(i int) {
	if m.valid(i) {
		m.tracks[i].reverbIdx = -1
	}
}

func (m *Mixer) TrackRoom(i int) (dsp.ReverbParams, bool) {
	if !m.valid(i) {
		return dsp.ReverbParams{}, false
	}
	if r := m.tracks[i].reverbIdx; r > 0 && r < len(m.reverbs) {
		return m.reverbs[r].Params(), true
	}
	return dsp.ReverbParams{}, false
}

func (m *Mixer) TrackDelay(i int) *dsp.StereoDelay {
	if !m.valid(i) {
		return nil
	}
	return m.tracks[i].delay
}

func (m *Mixer) Muted(i int) bool { return m.valid(i) && m.tracks[i].muted }

func (m *Mixer) SetReverbWet(w float32) { m.reverbWet = clamp32(w, 0, 1) }

func (m *Mixer) ReverbWet() float32 { return m.reverbWet }

func (m *Mixer) SetReverbPreset(name string) bool {
	p, ok := dsp.PresetParams(name)
	if ok {
		m.reverb.SetParams(p)
	}
	return ok
}

func (m *Mixer) SetReverbDamping(d float32) {
	for _, r := range m.reverbs {
		r.SetDamping(d)
	}
}

func (m *Mixer) SetClipThreshold(t float32) { m.clipper.SetThreshold(t) }

func (m *Mixer) ClipThreshold() float32 { return m.clipper.Threshold() }

func (m *Mixer) SetSaturationBypass(bypass bool) { m.clipper.SetClean(bypass) }

func (m *Mixer) SaturationBypass() bool { return m.clipper.Clean() }

func (m *Mixer) SetLimiterEnabled(enabled bool) { m.useLimiter = enabled }

func (m *Mixer) LimiterEnabled() bool { return m.useLimiter }

func (m *Mixer) SetMonoBassCutoff(hz float32) {
	if hz <= 0 {
		m.monoBassHz = 0
		return
	}
	m.monoBassHz = hz
	a := 1 - math.Exp(-2*math.Pi*float64(hz)/float64(m.sampleRate))
	if a > 1 {
		a = 1
	}
	m.monoBassA = float32(a)
}

func (m *Mixer) MonoBassCutoff() float32 { return m.monoBassHz }

func (m *Mixer) applyMonoBass(master []float32, n int) {
	a := m.monoBassA
	yL, yR := m.bassYL, m.bassYR
	for i := 0; i < n; i++ {
		l := master[2*i]
		r := master[2*i+1]
		yL += a * (l - yL)
		yR += a * (r - yR)
		mono := (yL + yR) * 0.5
		master[2*i] = mono + (l - yL)
		master[2*i+1] = mono + (r - yR)
	}
	m.bassYL, m.bassYR = yL, yR
}

func (m *Mixer) Reverb() *dsp.Reverb { return m.reverb }

type parallelJob struct {
	m     *Mixer
	track int
	n     int
	dt    float64
}

var (
	parOnce sync.Once
	parJobs chan parallelJob
)

func runParallelJob(j parallelJob) {
	defer func() {
		if r := recover(); r != nil {
			clear(j.m.tracks[j.track].buffer[:2*j.n])
			j.m.setParallelErr(fmt.Errorf("track %d panic: %v", j.track, r))
		}
		j.m.parWg.Done()
	}()
	j.m.renderTrack(j.track, j.n, j.dt)
}

func ensureParallelPool() {
	parOnce.Do(func() {
		nw := runtime.NumCPU()
		if nw < 1 {
			nw = 1
		}
		if nw > 64 {
			nw = 64
		}
		parJobs = make(chan parallelJob, 4096)
		for w := 0; w < nw; w++ {
			go func() {
				for j := range parJobs {
					runParallelJob(j)
				}
			}()
		}
	})
}

func (m *Mixer) setParallelErr(err error) {
	m.parErrMu.Lock()
	m.parErr = errors.Join(m.parErr, err)
	m.parErrMu.Unlock()
}

func (m *Mixer) Err() error {
	m.parErrMu.Lock()
	defer m.parErrMu.Unlock()
	return m.parErr
}

func (m *Mixer) renderTrack(i int, n int, dt float64) {
	tr := &m.tracks[i]
	buf := tr.buffer[:2*n]
	tr.scratch.Left = tr.scratchL[:n]
	tr.scratch.Right = tr.scratchR[:n]
	clear(tr.scratch.Left)
	clear(tr.scratch.Right)
	tr.instrument.Process(&tr.scratch, dt)
	sl := tr.scratch.Left[:n]
	sr := tr.scratch.Right[:n]
	if tr.distortion != nil {
		tr.distortion.ProcessStereo(sl, sr)
	}
	if tr.ott != nil {
		tr.ott.ProcessStereo(sl, sr)
	}
	if tr.turntable != nil {
		tr.turntable.ProcessStereo(sl, sr)
	}
	_ = buf[2*n-1]
	_ = sl[n-1]
	_ = sr[n-1]
	for j := 0; j < n; j++ {
		buf[2*j] = sl[j]
		buf[2*j+1] = sr[j]
	}
}

func (m *Mixer) renderTrackSequential(i, n int, dt float64, recoverPanic bool) {
	if recoverPanic {
		defer func() {
			if r := recover(); r != nil {
				clear(m.tracks[i].buffer[:2*n])
				m.setParallelErr(fmt.Errorf("track %d panic: %v", i, r))
			}
		}()
	}
	tr := &m.tracks[i]
	buf := tr.buffer[:2*n]
	m.render.SetLen(n)
	m.render.Clear()
	tr.instrument.Process(m.render, dt)
	sl := m.render.Left[:n]
	sr := m.render.Right[:n]
	if tr.distortion != nil {
		tr.distortion.ProcessStereo(sl, sr)
	}
	if tr.ott != nil {
		tr.ott.ProcessStereo(sl, sr)
	}
	if tr.turntable != nil {
		tr.turntable.ProcessStereo(sl, sr)
	}
	_ = buf[2*n-1]
	_ = sl[n-1]
	_ = sr[n-1]
	for j := 0; j < n; j++ {
		buf[2*j] = sl[j]
		buf[2*j+1] = sr[j]
	}
}

func (m *Mixer) SetParallel(enabled bool) { m.useParallel = enabled }

func (m *Mixer) Parallel() bool { return m.useParallel }

func (m *Mixer) Process(blockSize int) {
	n := min(blockSize, m.maxBlock)
	if n < 1 {
		m.lastN = 0
		return
	}
	m.lastN = n
	dt := 1 / float64(m.sampleRate)

	master := m.masterBuffer[:2*n]
	clear(master)
	if m.reverbWet > 0 {
		for r := 0; r < len(m.reverbs); r++ {
			clear(m.roomBus(r, n))
		}
	}

	activeCount := 0
	for i := range m.tracks {
		if !m.tracks[i].muted {
			activeCount++
		}
	}

	runParallel := m.useParallel && activeCount > minParallelTracks && runtime.GOMAXPROCS(0) > 1

	if runParallel {
		ensureParallelPool()
		m.parWg.Add(activeCount)
		for i := range m.tracks {
			if m.tracks[i].muted {
				continue
			}
			parJobs <- parallelJob{m: m, track: i, n: n, dt: dt}
		}
		for {
			select {
			case j := <-parJobs:
				runParallelJob(j)
			default:
				goto waitDone
			}
		}
	waitDone:
		m.parWg.Wait()
	} else if m.useParallel {
		for i := range m.tracks {
			if m.tracks[i].muted {
				continue
			}
			m.renderTrackSequential(i, n, dt, true)
		}
	} else {
		for i := range m.tracks {
			if m.tracks[i].muted {
				continue
			}
			m.renderTrackSequential(i, n, dt, false)
		}
	}

	for i := range m.tracks {
		tr := &m.tracks[i]
		if tr.muted || tr.sidechain == nil || tr.sidechainSource < 0 || tr.sidechainSource >= len(m.tracks) || tr.sidechainSource == i {
			continue
		}
		src := m.tracks[tr.sidechainSource].buffer[:2*n]
		tr.sidechain.ProcessInterleaved(tr.buffer[:2*n], src)
	}

	for i := range m.tracks {
		tr := &m.tracks[i]
		if tr.muted {
			continue
		}
		m.accumulateTrack(master, tr, n)
	}

	if m.reverbWet > 0 {
		if m.useConvolution && m.convReverb != nil {
			send := m.roomBus(0, n)
			for i := 0; i < n; i++ {
				wl, wr := m.convReverb.ProcessFrameWet(send[2*i], send[2*i+1])
				master[2*i] += wl * m.reverbWet
				master[2*i+1] += wr * m.reverbWet
			}
		}
		for r := 0; r < len(m.reverbs); r++ {
			send := m.roomBus(r, n)
			rev := m.reverbs[r]
			for i := 0; i < n; i++ {
				wl, wr := rev.ProcessFrameWet(send[2*i], send[2*i+1])
				master[2*i] += wl * m.reverbWet
				master[2*i+1] += wr * m.reverbWet
			}
		}
	}

	if m.useMultiband && m.multiband != nil {
		m.multiband.ProcessInterleaved(master)
	}
	if m.useOTT && m.ott != nil {
		m.ott.ProcessInterleaved(master)
	}
	if m.useTurntable && m.turntable != nil {
		m.turntable.ProcessInterleaved(master)
	}

	m.applySoftClipper(n)
	if m.useLimiter {

		m.limiter.ProcessInterleaved(master)
	}
	if m.monoBassHz > 0 {
		m.applyMonoBass(master, n)
	}
}

func (m *Mixer) SetTrackSidechain(trackIdx, sourceTrackIdx int, cfg dsp.SidechainConfig) {
	if !m.valid(trackIdx) {
		return
	}
	m.tracks[trackIdx].sidechainSource = sourceTrackIdx
	m.tracks[trackIdx].sidechain = dsp.NewSidechainCompressor(float64(m.sampleRate), cfg)
}

func (m *Mixer) TrackSidechain(trackIdx int) (*dsp.SidechainCompressor, int) {
	if !m.valid(trackIdx) {
		return nil, -1
	}
	return m.tracks[trackIdx].sidechain, m.tracks[trackIdx].sidechainSource
}

func (m *Mixer) SetConvolutionEnabled(enabled bool) {
	m.useConvolution = enabled
}

func (m *Mixer) ConvolutionEnabled() bool {
	return m.useConvolution
}

func (m *Mixer) SetConvolutionConfig(cfg dsp.ConvolutionConfig) {
	if m.convReverb != nil {
		m.convReverb.SetConfig(cfg)
	}
}

func (m *Mixer) SetConvolutionIR(irL, irR []float32) {
	if m.convReverb != nil {
		m.convReverb.SetImpulseResponse(irL, irR)
		m.useConvolution = true
	}
}

func (m *Mixer) SetConvolutionIRFile(path string) error {
	if m.convReverb == nil {
		m.convReverb = dsp.NewConvolutionReverb(float64(m.sampleRate), dsp.DefaultConvolutionConfig())
	}
	if err := m.convReverb.LoadIRFile(path); err != nil {
		return err
	}
	m.useConvolution = true
	return nil
}

func (m *Mixer) Convolution() *dsp.ConvolutionReverb {
	return m.convReverb
}

func (m *Mixer) SetMultibandEnabled(enabled bool) {
	m.useMultiband = enabled
}

func (m *Mixer) MultibandEnabled() bool {
	return m.useMultiband
}

func (m *Mixer) SetMultibandConfig(cfg dsp.MultibandConfig) {
	if m.multiband != nil {
		m.multiband.SetConfig(cfg)
	}
}

func (m *Mixer) SetOTTEnabled(enabled bool) {
	m.useOTT = enabled
}

func (m *Mixer) OTTEnabled() bool {
	return m.useOTT
}

func (m *Mixer) SetOTTConfig(cfg dsp.OTTConfig) {
	if m.ott != nil {
		m.ott.SetConfig(cfg)
	}
	m.useOTT = true
}

func (m *Mixer) OTT() *dsp.OTT {
	return m.ott
}

func (m *Mixer) SetTrackOTT(i int, cfg dsp.OTTConfig) {
	if !m.valid(i) {
		return
	}
	m.tracks[i].ott = dsp.NewOTT(float64(m.sampleRate), cfg)
}

func (m *Mixer) TrackOTT(i int) *dsp.OTT {
	if !m.valid(i) {
		return nil
	}
	return m.tracks[i].ott
}

func (m *Mixer) SetTrackDistortion(i int, cfg dsp.DistortionConfig) {
	if !m.valid(i) {
		return
	}
	m.tracks[i].distortion = dsp.NewDistortion(float64(m.sampleRate), cfg)
}

func (m *Mixer) TrackDistortion(i int) *dsp.Distortion {
	if !m.valid(i) {
		return nil
	}
	return m.tracks[i].distortion
}

func (m *Mixer) Multiband() *dsp.MultibandDynamics {
	return m.multiband
}

func (m *Mixer) SetTurntableEnabled(enabled bool) {
	m.useTurntable = enabled
}

func (m *Mixer) TurntableEnabled() bool {
	return m.useTurntable
}

func (m *Mixer) SetTurntableConfig(cfg dsp.TurntableConfig) {
	if m.turntable == nil {
		m.turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.turntable.SetInertiaMs(cfg.InertiaMs)
	m.useTurntable = true
}

func (m *Mixer) Turntable() *dsp.Turntable {
	return m.turntable
}

func (m *Mixer) SetMasterBrake(brakeTimeMs float64, curve string) {
	if m.turntable == nil {
		m.turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.turntable.Brake(brakeTimeMs, curve)
	m.useTurntable = true
}

func (m *Mixer) SetMasterScratch(speed float32) {
	if m.turntable == nil {
		m.turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.turntable.Scratch(speed)
	m.useTurntable = true
}

func (m *Mixer) SetMasterBeatRoll(active bool, sub dsp.Subdivision, bpm float64) {
	if m.turntable == nil {
		m.turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.turntable.BeatRoll(active, sub, bpm)
	m.useTurntable = true
}

func (m *Mixer) SetMasterBackspin(speed float32, decayMs float64) {
	if m.turntable == nil {
		m.turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.turntable.Backspin(speed, decayMs)
	m.useTurntable = true
}

func (m *Mixer) SetMasterTurntableBypass() {
	if m.turntable != nil {
		m.turntable.Bypass()
	}
}

func (m *Mixer) SetTrackTurntable(i int, cfg dsp.TurntableConfig) {
	if !m.valid(i) {
		return
	}
	tt := dsp.NewTurntable(float64(m.sampleRate))
	tt.SetInertiaMs(cfg.InertiaMs)
	m.tracks[i].turntable = tt
}

func (m *Mixer) TrackTurntable(i int) *dsp.Turntable {
	if !m.valid(i) {
		return nil
	}
	return m.tracks[i].turntable
}

func (m *Mixer) SetTrackBrake(i int, brakeTimeMs float64, curve string) {
	if !m.valid(i) {
		return
	}
	if m.tracks[i].turntable == nil {
		m.tracks[i].turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.tracks[i].turntable.Brake(brakeTimeMs, curve)
}

func (m *Mixer) SetTrackScratch(i int, speed float32) {
	if !m.valid(i) {
		return
	}
	if m.tracks[i].turntable == nil {
		m.tracks[i].turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.tracks[i].turntable.Scratch(speed)
}

func (m *Mixer) SetTrackBeatRoll(i int, active bool, sub dsp.Subdivision, bpm float64) {
	if !m.valid(i) {
		return
	}
	if m.tracks[i].turntable == nil {
		m.tracks[i].turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.tracks[i].turntable.BeatRoll(active, sub, bpm)
}

func (m *Mixer) SetTrackBackspin(i int, speed float32, decayMs float64) {
	if !m.valid(i) {
		return
	}
	if m.tracks[i].turntable == nil {
		m.tracks[i].turntable = dsp.NewTurntable(float64(m.sampleRate))
	}
	m.tracks[i].turntable.Backspin(speed, decayMs)
}

func (m *Mixer) SetTrackTurntableBypass(i int) {
	if !m.valid(i) || m.tracks[i].turntable == nil {
		return
	}
	m.tracks[i].turntable.Bypass()
}

func (m *Mixer) SetOversamplingEnabled(enabled bool) {
	m.clipper.SetOversampling(enabled)
}

func (m *Mixer) OversamplingEnabled() bool {
	return m.clipper.Oversampling()
}

func (m *Mixer) accumulateTrack(master []float32, tr *TrackBuffer, n int) {
	buf := tr.buffer[:2*n]
	panL, panR := PanGains(tr.pan)
	lg := panL * tr.volume
	rg := panR * tr.volume

	hasSend := m.reverbWet > 0 && tr.send > 0 && tr.reverbIdx >= 0 && tr.reverbIdx < len(m.reverbs)
	hasDelay := tr.delay != nil && tr.delayWet > 0

	_ = master[2*n-1]
	_ = buf[2*n-1]

	if hasDelay {
		var sendBus []float32
		slg, srg := float32(0), float32(0)
		if hasSend {
			sendBus = m.roomBus(tr.reverbIdx, n)
			_ = sendBus[2*n-1]
			slg = lg * tr.send
			srg = rg * tr.send
		}
		for j := 0; j < n; j++ {
			b0 := buf[2*j]
			b1 := buf[2*j+1]
			ml := b0 * lg
			mr := b1 * rg
			master[2*j] += ml
			master[2*j+1] += mr
			if hasSend {
				sendBus[2*j] += b0 * slg
				sendBus[2*j+1] += b1 * srg
			}
			wl, wr := tr.delay.ProcessFrameWet(ml, mr)
			master[2*j] += wl * tr.delayWet
			master[2*j+1] += wr * tr.delayWet
		}
		return
	}

	if hasSend {
		sendBus := m.roomBus(tr.reverbIdx, n)
		_ = sendBus[2*n-1]
		slg := lg * tr.send
		srg := rg * tr.send
		for j := 0; j < n; j++ {
			b0 := buf[2*j]
			b1 := buf[2*j+1]
			master[2*j] += b0 * lg
			master[2*j+1] += b1 * rg
			sendBus[2*j] += b0 * slg
			sendBus[2*j+1] += b1 * srg
		}
		return
	}

	for j := 0; j < n; j++ {
		master[2*j] += buf[2*j] * lg
		master[2*j+1] += buf[2*j+1] * rg
	}
}

func (m *Mixer) applySoftClipper(n int) {
	m.clipper.ProcessSlice(m.masterBuffer[:2*n])
}

func (m *Mixer) TrackPeak(i int) float32 {
	if !m.valid(i) || m.tracks[i].muted {
		return 0
	}
	peak := float32(0)
	for _, v := range m.tracks[i].buffer[:2*m.lastN] {
		if a := abs32(v); a > peak {
			peak = a
		}
	}
	return peak
}

func (m *Mixer) Master() []float32 { return m.masterBuffer[:2*m.lastN] }

func (m *Mixer) Frames() int { return m.lastN }

func (m *Mixer) valid(i int) bool { return i >= 0 && i < len(m.tracks) }

func clampF(v, lo, hi float64) float64 {
	switch {
	case v < lo:
		return lo
	case v > hi:
		return hi
	}
	return v
}

func clamp32(v, lo, hi float32) float32 {
	switch {
	case v < lo || v != v:
		return lo
	case v > hi:
		return hi
	}
	return v
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
