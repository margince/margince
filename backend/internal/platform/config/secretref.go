// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"fmt"
	"strings"
)

// resolveRef turns ${file:p} or ${env:N} into the value; any other text is the
// literal. The path, never the value, goes into the origin and any error.
func resolveRef(text string, readFile func(string) ([]byte, error), getenv func(string) (string, bool)) (value, file string, isRef bool, err error) {
	inner, ok := strings.CutPrefix(text, "${")
	if !ok || !strings.HasSuffix(inner, "}") {
		return text, "", false, nil
	}
	inner = strings.TrimSuffix(inner, "}")
	switch kind, arg, _ := strings.Cut(inner, ":"); kind {
	case "file":
		raw, err := readFile(arg)
		if err != nil {
			return "", arg, true, fmt.Errorf("cannot read secret file %s", arg)
		}
		return strings.TrimSuffix(string(raw), "\n"), arg, true, nil
	case "env":
		v, set := getenv(arg)
		if !set {
			return "", "", true, fmt.Errorf("references %s, which is not set", arg)
		}
		return v, "", true, nil
	}
	return text, "", false, nil
}
