// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package imagenorm

import (
	"regexp"
	"strconv"
	"strings"
)

// svgPathDataAttr matches a quoted d attribute. Only a value that parses as
// path data is rewritten, so a stray d on another element is left alone.
var svgPathDataAttr = regexp.MustCompile(`(\sd\s*=\s*)("[^"]*"|'[^']*')`)

var svgPathArity = map[byte]int{
	'm': 2, 'l': 2, 'h': 1, 'v': 1, 'c': 6, 's': 4, 'q': 4, 't': 2, 'a': 7, 'z': 0,
}

// splitImplicitPathRepeats rewrites every path so each parameter set carries
// its own command letter. The renderer passes the whole parameter list of a
// repeated arc to its arc routine, so every arc after the first in one command
// is drawn with the first arc's radii and end point.
func splitImplicitPathRepeats(src []byte) []byte {
	return svgPathDataAttr.ReplaceAllFunc(src, func(match []byte) []byte {
		parts := svgPathDataAttr.FindSubmatch(match)
		value := string(parts[2])
		quote := value[:1]
		expanded, ok := expandPathData(value[1 : len(value)-1])
		if !ok {
			return match
		}
		return []byte(string(parts[1]) + quote + expanded + quote)
	})
}

func expandPathData(d string) (string, bool) {
	var out strings.Builder
	pos := 0
	var cmd byte
	for {
		pos = skipPathSeparators(d, pos)
		if pos >= len(d) {
			break
		}
		if c := d[pos]; isPathCommand(c) {
			cmd = c
			pos++
			if lower(c) == 'z' {
				out.WriteByte(c)
				out.WriteByte(' ')
				continue
			}
		} else if cmd == 0 || lower(cmd) == 'z' {
			return "", false
		}
		params, next, ok := readPathParams(d, pos, lower(cmd))
		if !ok {
			return "", false
		}
		pos = next
		out.WriteByte(cmd)
		out.WriteString(strings.Join(params, " "))
		out.WriteByte(' ')
		// A moveto's extra pairs are implicit linetos.
		switch cmd {
		case 'M':
			cmd = 'L'
		case 'm':
			cmd = 'l'
		}
	}
	return strings.TrimSpace(out.String()), true
}

func isPathCommand(c byte) bool {
	_, ok := svgPathArity[lower(c)]
	return ok
}

func lower(c byte) byte { return c | 0x20 }

func skipPathSeparators(d string, pos int) int {
	for pos < len(d) && (d[pos] == ',' || d[pos] == ' ' || d[pos] == '\t' || d[pos] == '\n' || d[pos] == '\r') {
		pos++
	}
	return pos
}

func readPathParams(d string, pos int, cmd byte) ([]string, int, bool) {
	params := make([]string, 0, svgPathArity[cmd])
	for i := 0; i < svgPathArity[cmd]; i++ {
		pos = skipPathSeparators(d, pos)
		var tok string
		var ok bool
		if cmd == 'a' && (i == 3 || i == 4) {
			// Arc flags are a single digit and may touch the next number.
			if pos < len(d) && (d[pos] == '0' || d[pos] == '1') {
				tok, pos, ok = d[pos:pos+1], pos+1, true
			}
		} else {
			tok, pos, ok = scanPathNumber(d, pos)
		}
		if !ok {
			return nil, pos, false
		}
		params = append(params, tok)
	}
	return params, pos, true
}

func scanPathNumber(d string, pos int) (string, int, bool) {
	start := pos
	if pos < len(d) && (d[pos] == '+' || d[pos] == '-') {
		pos++
	}
	digits := 0
	for pos < len(d) && d[pos] >= '0' && d[pos] <= '9' {
		pos++
		digits++
	}
	if pos < len(d) && d[pos] == '.' {
		pos++
		for pos < len(d) && d[pos] >= '0' && d[pos] <= '9' {
			pos++
			digits++
		}
	}
	if digits == 0 {
		return "", pos, false
	}
	if pos < len(d) && (d[pos] == 'e' || d[pos] == 'E') {
		exp := pos + 1
		if exp < len(d) && (d[exp] == '+' || d[exp] == '-') {
			exp++
		}
		if exp < len(d) && d[exp] >= '0' && d[exp] <= '9' {
			for exp < len(d) && d[exp] >= '0' && d[exp] <= '9' {
				exp++
			}
			pos = exp
		}
	}
	tok := d[start:pos]
	if _, err := strconv.ParseFloat(tok, 64); err != nil {
		return "", pos, false
	}
	return tok, pos, true
}
