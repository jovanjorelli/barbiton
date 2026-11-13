package dsp

type StereoBuffer struct {
	Left     []float32
	Right    []float32
	capacity int
}

type Buffer = StereoBuffer

func NewStereoBuffer(frames int) *StereoBuffer {
	if frames < 0 {
		frames = 0
	}
	return &StereoBuffer{
		Left:     make([]float32, 0, frames),
		Right:    make([]float32, 0, frames),
		capacity: frames,
	}
}

func (b *StereoBuffer) Len() int { return len(b.Left) }

func (b *StereoBuffer) Cap() int { return b.capacity }

func (b *StereoBuffer) Reset() {
	b.Left = b.Left[:0]
	b.Right = b.Right[:0]
}

func (b *StereoBuffer) SetLen(n int) {
	if n < 0 {
		n = 0
	}
	if n > b.capacity {
		n = b.capacity
	}
	if n > len(b.Left) {
		clear(b.Left[len(b.Left):n])
		clear(b.Right[len(b.Right):n])
	}
	b.Left = b.Left[:n]
	b.Right = b.Right[:n]
}

func (b *StereoBuffer) Clear() {
	clear(b.Left)
	clear(b.Right)
}

func (b *StereoBuffer) Copy(src *StereoBuffer) {
	n := src.Len()
	if n > b.capacity {
		n = b.capacity
	}
	b.SetLen(n)
	copy(b.Left, src.Left[:n])
	copy(b.Right, src.Right[:n])
}

func (b *StereoBuffer) Mix(src *StereoBuffer) {
	n := b.Len()
	if src.Len() < n {
		n = src.Len()
	}
	if n <= 0 {
		return
	}
	dstL := b.Left[:n]
	dstR := b.Right[:n]
	srcL := src.Left[:n]
	srcR := src.Right[:n]
	_ = dstL[n-1]
	_ = dstR[n-1]
	_ = srcL[n-1]
	_ = srcR[n-1]
	for i := 0; i < n; i++ {
		dstL[i] += srcL[i]
		dstR[i] += srcR[i]
	}
}

func (b *StereoBuffer) MixGain(src *StereoBuffer, gain float32) {
	n := b.Len()
	if src.Len() < n {
		n = src.Len()
	}
	if n <= 0 {
		return
	}
	dstL := b.Left[:n]
	dstR := b.Right[:n]
	srcL := src.Left[:n]
	srcR := src.Right[:n]
	_ = dstL[n-1]
	_ = dstR[n-1]
	_ = srcL[n-1]
	_ = srcR[n-1]
	for i := 0; i < n; i++ {
		dstL[i] += srcL[i] * gain
		dstR[i] += srcR[i] * gain
	}
}
