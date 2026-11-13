package sampler

import (
	"bytes"
	"io"
)

type Tokenizer struct {
	data     []byte
	pos      int
	tokenBuf []byte

	Strays       int
	CommentSkips int
}

func NewTokenizer(data []byte) *Tokenizer {
	return &Tokenizer{data: data, tokenBuf: make([]byte, 0, 64)}
}

func (t *Tokenizer) Reset(data []byte) {
	t.data = data
	t.pos = 0
	t.tokenBuf = t.tokenBuf[:0]
	t.Strays = 0
	t.CommentSkips = 0
}

func (t *Tokenizer) NextOpcode() (key []byte, value []byte, err error) {
	for {
		t.skipSpaceAndComments()
		if t.pos >= len(t.data) {
			return nil, nil, io.EOF
		}
		c := t.data[t.pos]
		switch {
		case c == '<':

			start := t.pos
			if i := bytes.IndexByte(t.data[t.pos:], '>'); i >= 0 {
				t.pos += i + 1
			} else {
				t.pos = len(t.data)
			}
			return t.data[start:t.pos], nil, nil
		case isKeyByte(c):
			start := t.pos
			for t.pos < len(t.data) && isKeyByte(t.data[t.pos]) {
				t.pos++
			}

			scan := t.pos
			for scan < len(t.data) && (t.data[scan] == ' ' || t.data[scan] == '\t') {
				scan++
			}
			if scan < len(t.data) && t.data[scan] == '=' {
				key = t.data[start:t.pos]
				t.pos = scan + 1
				return key, t.scanValue(), nil
			}
			t.Strays++
		default:
			t.pos++
			t.Strays++
		}
	}
}

func (t *Tokenizer) LowerKey(key []byte) []byte {
	t.tokenBuf = append(t.tokenBuf[:0], key...)
	for i, c := range t.tokenBuf {
		if c >= 'A' && c <= 'Z' {
			t.tokenBuf[i] = c + 32
		}
	}
	return t.tokenBuf
}

func (t *Tokenizer) scanValue() []byte {

	for t.pos < len(t.data) && isSpace(t.data[t.pos]) {
		t.pos++
	}
	if t.pos >= len(t.data) || t.data[t.pos] == '<' || t.atOpcodeStart() {
		return nil
	}
	start := t.pos
	end := t.pos
	for t.pos < len(t.data) {
		c := t.data[t.pos]
		if isSpace(c) {
			save := t.pos
			t.skipSpaceAndComments()
			if t.pos >= len(t.data) || t.data[t.pos] == '<' || t.atOpcodeStart() {
				t.pos = save
				return t.data[start:end]
			}

			t.pos = save + 1
			continue
		}
		t.pos++
		end = t.pos
	}
	return t.data[start:end]
}

func (t *Tokenizer) atOpcodeStart() bool {
	i := t.pos
	for i < len(t.data) && isKeyByte(t.data[i]) {
		i++
	}
	if i == t.pos {
		return false
	}
	for i < len(t.data) && (t.data[i] == ' ' || t.data[i] == '\t') {
		i++
	}
	return i < len(t.data) && t.data[i] == '='
}

func (t *Tokenizer) skipSpaceAndComments() {
	for t.pos < len(t.data) {
		c := t.data[t.pos]
		switch {
		case isSpace(c):
			t.pos++
		case c == '/' && t.pos+1 < len(t.data) && t.data[t.pos+1] == '/':
			t.skipLine()
			t.CommentSkips++
		case c == '/' && t.pos+1 < len(t.data) && t.data[t.pos+1] == '*':
			if i := bytes.Index(t.data[t.pos+2:], []byte("*/")); i >= 0 {
				t.pos += i + 4
			} else {
				t.pos = len(t.data)
			}
			t.CommentSkips++
		case c == '#' || c == ';':
			t.skipLine()
			t.CommentSkips++
		case c == '/':
			t.pos++
			t.Strays++
		default:
			return
		}
	}
}

func (t *Tokenizer) skipLine() {
	for t.pos < len(t.data) && t.data[t.pos] != '\n' {
		t.pos++
	}
	if t.pos < len(t.data) {
		t.pos++
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v' || c == '\f'
}

func isKeyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
