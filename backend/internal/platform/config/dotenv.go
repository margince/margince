// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// parseDotenv reads the subset every .env tool agrees on. It expands nothing:
// a "$" in a password stays a "$". Errors name the file, line and variable,
// never the value, which may be a secret.
func parseDotenv(path string, raw []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")
		name, value, ok := strings.Cut(text, "=")
		name = strings.TrimSpace(name)
		if !ok || !envForm.MatchString(name) {
			return nil, fmt.Errorf("%s:%d: want NAME=value", path, line)
		}
		v, err := dotenvValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %s: %w", path, line, name, err)
		}
		out[name] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return out, nil
}

func dotenvValue(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if q := v[0]; q == '"' || q == '\'' {
		end := strings.IndexByte(v[1:], q)
		if end < 0 {
			return "", fmt.Errorf("unclosed %c quote", q)
		}
		return v[1 : end+1], nil
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), nil
}
