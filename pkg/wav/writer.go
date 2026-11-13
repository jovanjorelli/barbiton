package wav

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync/atomic"
)

type Format uint16

const (
	PCM16 Format = 1

	PCM24 Format = 1

	PCM32 Format = 1

	Float32 Format = 3

	Extensible Format = 0xFFFE
)

const (
	int16Scale = 32768.0
	int16Min   = -32768
	int16Max   = 32767
	int24Max   = 8388607
	int32Max   = 2147483647
)

const (
	speakerFrontLeft   = 0x1
	speakerFrontRight  = 0x2
	speakerFrontCenter = 0x4
	speakerBackLeft    = 0x10
	speakerBackRight   = 0x20
)

var guidTail = []byte{0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xAA, 0x00, 0x38, 0x9B, 0x71}

func channelMask(channels int) uint32 {
	switch channels {
	case 1:
		return speakerFrontCenter
	case 2:
		return speakerFrontLeft | speakerFrontRight
	case 3:
		return 0x7
	case 4:
		return 0x33
	case 5:
		return 0x37
	case 6:
		return 0x3F
	case 8:
		return 0x63F
	}
	if channels >= 32 {
		return 0xFFFFFFFF
	}
	return uint32(1)<<uint(channels) - 1
}

const scratchFrames = 4096

type Writer struct {
	w              io.WriteSeeker
	format         Format
	subFormat      Format
	sampleRate     int
	channels       int
	bitsPerSample  int
	bytesPerSample int
	blockAlign     int
	extensible     bool
	frames         uint64
	dataSizeOff    int64
	closed         bool
	scratch        []byte
}

func NewWriter(w io.WriteSeeker, format Format, sampleRate, channels int) (*Writer, error) {
	switch format {
	case PCM24:
		return NewWriterBits(w, PCM24, 24, sampleRate, channels)
	case Float32:
		return NewWriterBits(w, Float32, 32, sampleRate, channels)
	default:
		return nil, fmt.Errorf("wav: ambiguous format tag %d, use NewWriterBits", uint16(format))
	}
}

func NewWriterBits(w io.WriteSeeker, format Format, bits, sampleRate, channels int) (*Writer, error) {
	if w == nil {
		return nil, errors.New("wav: nil destination")
	}
	if sampleRate <= 0 || channels <= 0 {
		return nil, fmt.Errorf("wav: invalid layout: %d Hz, %d channels", sampleRate, channels)
	}
	tag := uint16(format)
	var sub Format
	switch tag {
	case 1:
		sub = PCM24
		switch bits {
		case 16, 24, 32:
		default:
			return nil, fmt.Errorf("wav: unsupported PCM bit depth %d", bits)
		}
	case 3:
		sub = Float32
		if bits != 32 {
			return nil, fmt.Errorf("wav: unsupported float bit depth %d", bits)
		}
	default:
		return nil, fmt.Errorf("wav: unsupported format tag %d", tag)
	}
	extensible := sub == Float32 || channels > 2
	stored := Format(1)
	if extensible {
		stored = Extensible
	}
	wr := &Writer{
		w:              w,
		format:         stored,
		subFormat:      sub,
		sampleRate:     sampleRate,
		channels:       channels,
		bitsPerSample:  bits,
		bytesPerSample: bits / 8,
		blockAlign:     channels * bits / 8,
		extensible:     extensible,
		scratch:        make([]byte, channels*(bits/8)*scratchFrames),
	}
	if err := wr.writeHeader(); err != nil {
		return nil, err
	}
	return wr, nil
}

func (wr *Writer) writeHeader() error {
	fmtLen := uint32(16)
	headerLen := 44
	if wr.extensible {
		fmtLen = 40
		headerLen = 68
	}
	b := make([]byte, headerLen)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], 0)
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], fmtLen)
	binary.LittleEndian.PutUint16(b[20:22], uint16(wr.format))
	binary.LittleEndian.PutUint16(b[22:24], uint16(wr.channels))
	binary.LittleEndian.PutUint32(b[24:28], uint32(wr.sampleRate))
	binary.LittleEndian.PutUint32(b[28:32], uint32(wr.sampleRate*wr.blockAlign))
	binary.LittleEndian.PutUint16(b[32:34], uint16(wr.blockAlign))
	binary.LittleEndian.PutUint16(b[34:36], uint16(wr.bitsPerSample))
	end := 36
	if wr.extensible {
		binary.LittleEndian.PutUint16(b[36:38], 22)
		binary.LittleEndian.PutUint16(b[38:40], uint16(wr.bitsPerSample))
		binary.LittleEndian.PutUint32(b[40:44], channelMask(wr.channels))
		binary.LittleEndian.PutUint32(b[44:48], uint32(wr.subFormat))
		copy(b[48:60], guidTail)
		end = 60
	}
	copy(b[end:end+4], "data")
	wr.dataSizeOff = int64(end + 4)
	binary.LittleEndian.PutUint32(b[end+4:end+8], 0)
	_, err := wr.w.Write(b)
	return err
}

func (wr *Writer) WriteFrames(chans ...[]float32) (int, error) {
	if wr.closed {
		return 0, errors.New("wav: write after Close")
	}
	if len(chans) > wr.channels {
		return 0, fmt.Errorf("wav: got %d channel slices, want at most %d", len(chans), wr.channels)
	}
	n := 0
	for _, c := range chans {
		if len(c) > n {
			n = len(c)
		}
	}
	if n == 0 {
		return 0, nil
	}
	if need := n * wr.blockAlign; len(wr.scratch) < need {
		wr.scratch = make([]byte, need)
	}
	switch {
	case wr.subFormat == Float32:
		wr.encodeFloat32(chans, n)
	case wr.bitsPerSample == 16:
		wr.encodePCM16(chans, n)
	case wr.bitsPerSample == 32:
		wr.encodePCM32(chans, n)
	default:
		wr.encodePCM24(chans, n)
	}
	if _, err := wr.w.Write(wr.scratch[:n*wr.blockAlign]); err != nil {
		return 0, err
	}
	wr.frames += uint64(n)
	return n, nil
}

func (wr *Writer) encodePCM16(chans [][]float32, n int) {
	for i := 0; i < n; i++ {
		for ch := 0; ch < wr.channels; ch++ {
			var s float32
			if ch < len(chans) && i < len(chans[ch]) {
				s = chans[ch][i]
			}
			off := (i*wr.channels + ch) * 2
			binary.LittleEndian.PutUint16(wr.scratch[off:off+2], uint16(floatToInt16(s)))
		}
	}
}

func (wr *Writer) encodePCM24(chans [][]float32, n int) {
	for i := 0; i < n; i++ {
		for ch := 0; ch < wr.channels; ch++ {
			var s float32
			if ch < len(chans) && i < len(chans[ch]) {
				s = chans[ch][i]
			}
			v := floatToInt24(s)
			off := (i*wr.channels + ch) * 3
			wr.scratch[off] = byte(v)
			wr.scratch[off+1] = byte(v >> 8)
			wr.scratch[off+2] = byte(v >> 16)
		}
	}
}

func (wr *Writer) encodePCM32(chans [][]float32, n int) {
	for i := 0; i < n; i++ {
		for ch := 0; ch < wr.channels; ch++ {
			var s float32
			if ch < len(chans) && i < len(chans[ch]) {
				s = chans[ch][i]
			}
			off := (i*wr.channels + ch) * 4
			binary.LittleEndian.PutUint32(wr.scratch[off:off+4], uint32(floatToInt32(s)))
		}
	}
}

func (wr *Writer) encodeFloat32(chans [][]float32, n int) {
	for i := 0; i < n; i++ {
		for ch := 0; ch < wr.channels; ch++ {
			var s float32
			if ch < len(chans) && i < len(chans[ch]) {
				s = chans[ch][i]
			}
			off := (i*wr.channels + ch) * 4
			binary.LittleEndian.PutUint32(wr.scratch[off:off+4], math.Float32bits(s))
		}
	}
}

var ditherPRNG uint64 = 0xda942042e4dd58b5

func tpdfDither() float64 {
	for {
		old := atomic.LoadUint64(&ditherPRNG)
		next := old
		next ^= next << 13
		next ^= next >> 7
		next ^= next << 17
		if atomic.CompareAndSwapUint64(&ditherPRNG, old, next) {
			r1 := float64(uint32(next))/4294967296.0 - 0.5
			r2 := float64(uint32(next>>32))/4294967296.0 - 0.5
			return r1 + r2
		}
	}
}

func floatToInt16(f float32) int16 {
	v := float64(f) * int16Scale
	switch {
	case math.IsNaN(v):
		return 0
	case v >= int16Max:
		return int16Max
	case v <= int16Min:
		return int16Min
	}
	rounded := math.Round(v + tpdfDither())
	if rounded >= int16Max {
		return int16Max
	}
	if rounded <= int16Min {
		return int16Min
	}
	return int16(rounded)
}

func floatToInt24(f float32) int32 {
	v := float64(f) * int24Max
	switch {
	case math.IsNaN(v):
		return 0
	case v >= int24Max:
		return int24Max
	case v <= -int24Max:
		return -int24Max
	}
	return int32(math.Round(v))
}

func floatToInt32(f float32) int32 {
	v := float64(f) * int32Max
	switch {
	case math.IsNaN(v):
		return 0
	case v >= int32Max:
		return int32Max
	case v <= -int32Max:
		return -int32Max
	}
	return int32(math.Round(v))
}

func (wr *Writer) SampleRate() int { return wr.sampleRate }

func (wr *Writer) Channels() int { return wr.channels }

func (wr *Writer) Frames() uint64 { return wr.frames }

func (wr *Writer) Close() error {
	if wr.closed {
		return nil
	}
	wr.closed = true
	dataSize := wr.frames * uint64(wr.blockAlign)

	riffSize := uint64(wr.dataSizeOff) - 4 + dataSize
	if riffSize > math.MaxUint32 {
		return errors.New("wav: stream exceeds the 4 GiB RIFF size limit")
	}
	var patch [4]byte
	if err := wr.patchAt(4, uint32(riffSize), patch[:]); err != nil {
		return err
	}
	if err := wr.patchAt(wr.dataSizeOff, uint32(dataSize), patch[:]); err != nil {
		return err
	}
	_, err := wr.w.Seek(0, io.SeekEnd)
	return err
}

func (wr *Writer) patchAt(off int64, v uint32, scratch []byte) error {
	binary.LittleEndian.PutUint32(scratch, v)
	if _, err := wr.w.Seek(off, io.SeekStart); err != nil {
		return err
	}
	_, err := wr.w.Write(scratch)
	return err
}
