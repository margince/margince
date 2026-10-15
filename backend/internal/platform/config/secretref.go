// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"errors"
	"fmt"
	"strings"
)

// refEnv is the ${env:NAME} reference kind.
const refEnv = "env"

// refNone declares a secret absent. It uses the reference grammar so it can
// never be mistaken for a password: a secret refuses every literal.
const refNone = "${none}"

// secretFileLimit bounds a ${file:} secret. A password, key or token is far
// smaller; a larger file is a path typed wrong.
const secretFileLimit = 64 << 10

var (
	errNoneRefused = errors.New(refNone + " declares a credential absent, which this setting does not accept; delete the key to leave it unset")
	errNoneValue   = errors.New("the reference resolves to " + refNone + "; write " + refNone + " itself where the setting is declared")
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
		if len(raw) > secretFileLimit {
			return "", arg, true, fmt.Errorf("secret file %s is %d bytes, more than a secret holds (limit %d); check the path", arg, len(raw), secretFileLimit)
		}
		return refused(strings.TrimSuffix(string(raw), "\n"), arg)
	case refEnv:
		v, set := getenv(arg)
		if !set {
			return "", "", true, fmt.Errorf("references %s, which is not set", arg)
		}
		return refused(v, "")
	}
	return text, "", false, nil
}

// refused stops a source that holds the absence marker. Whether it means a
// password of that spelling or a removal cannot be told, so neither is taken.
func refused(value, file string) (string, string, bool, error) {
	if strings.TrimSpace(value) == refNone {
		return "", file, true, errNoneValue
	}
	return value, file, true, nil
}

// declaredNone is ${none} written where the secret is declared.
func declaredNone(text string) bool { return strings.TrimSpace(text) == refNone }
