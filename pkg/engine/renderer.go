package engine

import (
	"os"

	"barbiton/pkg/score"
	"barbiton/pkg/wav"
)

type OfflineRenderer struct {
	eng   *Engine
	file  *os.File
	w     *wav.Writer
	left  []float32
	right []float32
	total int
	done  int
	peak  float32
	err   error
}

func NewOfflineRenderer(s *score.Score, path string, sampleRate float64) (*OfflineRenderer, error) {
	eng, err := New(s, sampleRate, DefaultBlockSize)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w, err := wav.NewWriter(f, wav.PCM24, int(sampleRate), 2)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &OfflineRenderer{
		eng:   eng,
		file:  f,
		w:     w,
		left:  make([]float32, eng.BlockSize()),
		right: make([]float32, eng.BlockSize()),
		total: eng.TotalFrames(),
	}, nil
}

func (r *OfflineRenderer) Engine() *Engine { return r.eng }

func (r *OfflineRenderer) Done() bool { return r.Err() != nil || r.done >= r.total }

func (r *OfflineRenderer) Progress() float32 {
	if r.total <= 0 {
		return 1
	}
	p := float32(r.done) / float32(r.total)
	if p > 1 {
		p = 1
	}
	return p
}

func (r *OfflineRenderer) Err() error {
	if r.err != nil {
		return r.err
	}
	if r.eng != nil {
		return r.eng.Err()
	}
	return nil
}

func (r *OfflineRenderer) Peak() float32 { return r.peak }

func (r *OfflineRenderer) Frames() int { return r.total }

func (r *OfflineRenderer) ProcessBlock() {
	if r.Done() {
		return
	}
	n := min(r.eng.BlockSize(), r.total-r.done)
	r.eng.ProcessFrames(n, r.writeChunk)
	r.done += n
	if err := r.eng.Err(); err != nil && r.err == nil {
		r.err = err
	}
}

func (r *OfflineRenderer) writeChunk(master []float32) {
	if r.err != nil {
		return
	}
	frames := len(master) / 2
	for i := 0; i < frames; i++ {
		l, rt := master[2*i], master[2*i+1]
		r.left[i], r.right[i] = l, rt
		if a := abs32(l); a > r.peak {
			r.peak = a
		}
		if a := abs32(rt); a > r.peak {
			r.peak = a
		}
	}
	if _, err := r.w.WriteFrames(r.left[:frames], r.right[:frames]); err != nil {
		r.err = err
	}
}

func (r *OfflineRenderer) Close() error {
	werr := r.w.Close()
	ferr := r.file.Close()
	switch {
	case r.err != nil:
		return r.err
	case werr != nil:
		return werr
	}
	return ferr
}
