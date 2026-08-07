package log

import "strings"

// stripANSI removes ANSI/VT escape sequences from s so log messages stay
// plain text in every sink and subscriber. sing-box's log formatter emits
// colorized lines even to a PlatformWriter (SGR codes around the level
// prefix and connection ids), so engine messages arrive here with embedded
// CSI sequences; stripping them at the single log() choke point keeps the
// console, ring buffer, and bridge events clean and consistent.
//
// Supported: CSI (ESC [ ... final in 0x40..0x7e), OSC (ESC ] ... BEL or
// ESC \), and any other two-byte ESC + printable/control sequence. A lone
// trailing ESC is dropped. Returns s unchanged when no ESC is present.
func stripANSI(s string) string {
	if !strings.ContainsRune(s, '\x1b') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '\x1b' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(s) {
			break
		}
		next := s[i+1]
		i += 2
		switch {
		case next == '[': // CSI
			for i < len(s) {
				c = s[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
		case next == ']': // OSC
			for i < len(s) {
				c = s[i]
				i++
				if c == 0x07 { // BEL terminates
					break
				}
				if c == '\x1b' && i < len(s) && s[i] == '\\' { // ST terminates
					i++
					break
				}
			}
		default: // two-byte sequence
		}
	}
	return b.String()
}
