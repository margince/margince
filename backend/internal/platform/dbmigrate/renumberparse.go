// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"bufio"
	"fmt"
	"strings"
)

// ParseRenumbers reads namespace, old version, new version, name and old digest per line.
func ParseRenumbers(text string) (map[string][]Renumber, error) {
	pairs := map[string][]Renumber{}
	lines := bufio.NewScanner(strings.NewReader(text))
	for n := 1; lines.Scan(); n++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 5 {
			return nil, fmt.Errorf("migration renumbers line %d: want namespace old_version new_version name old_digest", n)
		}
		pairs[fields[0]] = append(pairs[fields[0]], Renumber{Old: fields[1], New: fields[2], Name: fields[3], OldDigest: fields[4]})
	}
	return pairs, lines.Err()
}
