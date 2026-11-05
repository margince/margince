// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

import (
	"fmt"
	"maps"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// Mail cannot read CSS variables, so emailview writes each design token in as
// its value. This gate resolves the same tokens from tokens.css, in both
// themes, and fails when any written value drifts from the product's.
func TestEmailTokensFollowTheProductTokens(t *testing.T) {
	t.Parallel()
	light, dark := productTokenThemes(t)
	tokens := emailview.Tokens()
	if len(tokens) < 19 {
		t.Fatalf("emailview declares %d tokens; the census lost its source", len(tokens))
	}
	for _, token := range tokens {
		assertTokenValue(t, light, token.Name, token.Light, "light")
		assertTokenValue(t, dark, token.Name, token.Dark, "dark")
	}
}

func TestEmailTypeFollowsTheProductScale(t *testing.T) {
	t.Parallel()
	light, _ := productTokenThemes(t)
	shorthand := regexp.MustCompile(`^(\d+) ([\d.]+)rem / ([\d.]+)rem (.+)$`)
	for _, face := range emailview.TypeScale() {
		resolved, _, err := light.resolve("--" + face.Name)
		if err != nil {
			t.Errorf("type %s: %v", face.Name, err)
			continue
		}
		parts := shorthand.FindStringSubmatch(resolved)
		if parts == nil {
			t.Errorf("type %s resolved to %q, which is not weight size / line family", face.Name, resolved)
			continue
		}
		want := fmt.Sprintf("%s %gpx/%gpx %s", parts[1], remPixels(parts[2]), remPixels(parts[3]), firstFamily(parts[4]))
		got := fmt.Sprintf("%d %dpx/%dpx %s", face.Weight, face.Size, face.Line, face.Family)
		if got != want {
			t.Errorf("type %s = %q, product = %q", face.Name, got, want)
		}
	}
}

func assertTokenValue(t *testing.T, theme cssTheme, name, written, label string) {
	t.Helper()
	resolved, mixed, err := theme.resolve("--" + name)
	if err != nil {
		t.Errorf("%s %s: %v", label, name, err)
		return
	}
	got, err := parseColor(written)
	if err != nil {
		if written != resolved {
			t.Errorf("%s %s = %q, product = %q", label, name, written, resolved)
		}
		return
	}
	want, err := parseColor(resolved)
	if err != nil {
		t.Errorf("%s %s: product value %q is not a colour: %v", label, name, resolved, err)
		return
	}
	if mixed && !got.near(want) || !mixed && got != want {
		t.Errorf("%s %s = %s, product resolves to %s", label, name, written, want)
	}
}

type cssTheme map[string]string

func productTokenThemes(t *testing.T) (cssTheme, cssTheme) {
	t.Helper()
	source, err := os.ReadFile("../frontend/src/design-system/tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(source)
	light := declarations(cssBlock(t, css, ":root {"))
	dark := maps.Clone(light)
	maps.Copy(dark, declarations(cssBlock(t, css, `[data-theme="dark"] {`)))
	return light, dark
}

func cssBlock(t *testing.T, css, opener string) string {
	t.Helper()
	start := strings.Index(css, opener)
	if start < 0 {
		t.Fatalf("tokens.css has no %q block", opener)
	}
	depth := 0
	for i := start + len(opener) - 1; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[start+len(opener) : i]
			}
		}
	}
	t.Fatalf("tokens.css block %q never closes", opener)
	return ""
}

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func declarations(block string) cssTheme {
	theme := cssTheme{}
	for statement := range strings.SplitSeq(cssComment.ReplaceAllString(block, ""), ";") {
		name, value, found := strings.Cut(statement, ":")
		name = strings.TrimSpace(name)
		if found && strings.HasPrefix(name, "--") {
			theme[name] = strings.Join(strings.Fields(value), " ")
		}
	}
	return theme
}

var (
	cssVar = regexp.MustCompile(`var\((--[\w-]+)\)`)
	cssMix = regexp.MustCompile(`^color-mix\(in srgb, (.+) (\d+)%, (.+)\)$`)
)

// resolve follows var() references to a value. mixed reports a color-mix,
// whose rounding is the only drift the gate allows.
func (theme cssTheme) resolve(name string) (value string, mixed bool, err error) {
	value, ok := theme[name]
	if !ok {
		return "", false, fmt.Errorf("tokens.css does not declare %s", name)
	}
	for depth := 0; strings.Contains(value, "var("); depth++ {
		if depth > 8 {
			return "", false, fmt.Errorf("%s does not resolve", name)
		}
		var missing string
		value = cssVar.ReplaceAllStringFunc(value, func(ref string) string {
			target := cssVar.FindStringSubmatch(ref)[1]
			found, declared := theme[target]
			if !declared {
				missing = target
			}
			return found
		})
		if missing != "" {
			return "", false, fmt.Errorf("%s refers to %s, which tokens.css does not declare", name, missing)
		}
	}
	if mix := cssMix.FindStringSubmatch(value); mix != nil {
		mixedValue, err := mixColors(mix[1], mix[2], mix[3])
		return mixedValue, true, err
	}
	return value, false, nil
}

type rgba struct{ r, g, b, a float64 }

func (c rgba) String() string {
	return fmt.Sprintf("rgba(%.0f, %.0f, %.0f, %.3g)", c.r, c.g, c.b, c.a)
}

func (c rgba) near(other rgba) bool {
	return math.Abs(c.r-other.r) <= 1 && math.Abs(c.g-other.g) <= 1 && math.Abs(c.b-other.b) <= 1 && math.Abs(c.a-other.a) <= 0.005
}

// mixColors follows color-mix in srgb, which mixes premultiplied channels.
func mixColors(first, percent, second string) (string, error) {
	a, err := parseColor(first)
	if err != nil {
		return "", err
	}
	b, err := parseColor(second)
	if err != nil {
		return "", err
	}
	p, err := strconv.ParseFloat(percent, 64)
	if err != nil {
		return "", err
	}
	p /= 100
	alpha := a.a*p + b.a*(1-p)
	if alpha == 0 {
		return rgba{}.String(), nil
	}
	channel := func(x, y float64) float64 { return (x*a.a*p + y*b.a*(1-p)) / alpha }
	return rgba{channel(a.r, b.r), channel(a.g, b.g), channel(a.b, b.b), alpha}.String(), nil
}

var rgbaPattern = regexp.MustCompile(`^rgba?\(\s*([\d.]+),\s*([\d.]+),\s*([\d.]+)(?:,\s*([\d.]+))?\s*\)$`)

func parseColor(value string) (rgba, error) {
	value = strings.TrimSpace(value)
	if value == "transparent" {
		return rgba{}, nil
	}
	if strings.HasPrefix(value, "#") {
		hex := value[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil || len(hex) != 6 {
			return rgba{}, fmt.Errorf("not a hex colour: %s", value)
		}
		return rgba{float64(n >> 16), float64(n >> 8 & 0xff), float64(n & 0xff), 1}, nil
	}
	parts := rgbaPattern.FindStringSubmatch(value)
	if parts == nil {
		return rgba{}, fmt.Errorf("not a colour: %s", value)
	}
	channel := func(text string) float64 {
		number, _ := strconv.ParseFloat(text, 64) //craft:ignore T2 the pattern admits only decimal digits
		return number
	}
	alpha := 1.0
	if parts[4] != "" {
		alpha = channel(parts[4])
	}
	return rgba{channel(parts[1]), channel(parts[2]), channel(parts[3]), alpha}, nil
}

func remPixels(rem string) float64 {
	value, _ := strconv.ParseFloat(rem, 64) //craft:ignore T2 the pattern admits only decimal digits
	return value * 16
}

func firstFamily(stack string) string {
	family, _, _ := strings.Cut(stack, ",")
	return strings.Trim(strings.TrimSpace(family), `"`)
}
