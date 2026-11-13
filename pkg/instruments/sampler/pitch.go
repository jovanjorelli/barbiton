package sampler

func ParsePitch(name []byte) (int, bool) {
	i := 0
	for i < len(name) && isSpace(name[i]) {
		i++
	}
	if i >= len(name) {
		return 0, false
	}
	offset, ok := noteOffset(name[i])
	if !ok {
		return 0, false
	}
	i++

	for i < len(name) {
		switch c := lowerByte(name[i]); c {
		case '#', 's':
			offset++
		case 'b', 'f':
			offset--
		default:
			goto octave
		}
		i++
	}
octave:

	sign := 1
	if i < len(name) && (name[i] == '+' || name[i] == '-') {
		if name[i] == '-' {
			sign = -1
		}
		i++
	}
	digits, oct := 0, 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		oct = oct*10 + int(name[i]-'0')
		if oct > 32 {
			return 0, false
		}
		i++
		digits++
	}
	if digits == 0 {
		return 0, false
	}
	for i < len(name) && isSpace(name[i]) {
		i++
	}
	if i != len(name) {
		return 0, false
	}

	midi := (oct*sign+1)*12 + offset
	if midi < 0 || midi > 127 {
		return 0, false
	}
	return midi, true
}

func noteOffset(c byte) (int, bool) {
	switch lowerByte(c) {
	case 'c':
		return 0, true
	case 'd':
		return 2, true
	case 'e':
		return 4, true
	case 'f':
		return 5, true
	case 'g':
		return 7, true
	case 'a':
		return 9, true
	case 'b':
		return 11, true
	}
	return 0, false
}

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
