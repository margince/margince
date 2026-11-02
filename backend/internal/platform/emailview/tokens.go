// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package emailview

import (
	"fmt"
	"strings"
)

// Token is one design-system value as mail must carry it. Mail clients read
// neither CSS variables nor the product stylesheet, so each token is written
// in as its resolved value. gates/emaildesign_test.go holds these to
// frontend/src/design-system/tokens.css in both themes.
type Token struct{ Name, Light, Dark string }

type token int

const (
	// noToken is the zero value: a look that leaves a property unpainted.
	noToken token = iota
	bgPage
	bgElevated
	pane
	paneEdge
	bgWell
	borderSubtle
	textPrimary
	textSecondary
	accent
	accentText
	accentMed
	textOnAccentControl
	ai
	aiText
	aiLight
	warningSurface
	warningBorder
	warningText
	infoSurface
	infoBorder
	infoText
	successSurface
	successBorder
	successText
	dangerSurface
	dangerBorder
	dangerText
	rControl
	rMd
	rLg
	shadowRest
	fontWeightMedium
	tokenCount
)

var tokens = [tokenCount]Token{
	bgPage:              {"bgPage", "#eff6f3", "#0c1311"},
	bgElevated:          {"bgElevated", "#fbfcfb", "#161d1a"},
	pane:                {"pane", "#fcfffe", "rgba(239, 247, 244, 0.045)"},
	paneEdge:            {"paneEdge", "#d8e2de", "rgba(239, 247, 244, 0.09)"},
	bgWell:              {"bgWell", "#f6f8f7", "#111815"},
	borderSubtle:        {"borderSubtle", "#e3eae6", "#22352e"},
	textPrimary:         {"textPrimary", "#182c28", "#edf6f4"},
	textSecondary:       {"textSecondary", "#55645f", "#9aa8a3"},
	accent:              {"accent", "#045647", "#3aae94"},
	accentText:          {"accentText", "#045647", "#66d5ba"},
	accentMed:           {"accentMed", "rgba(4, 86, 71, 0.17)", "rgba(58, 174, 148, 0.28)"},
	textOnAccentControl: {"textOnAccentControl", "#f4faf7", "#07221d"},
	ai:                  {"ai", "#5b61d6", "#5b61d6"},
	aiText:              {"aiText", "#3f45b0", "#9ba0f0"},
	aiLight:             {"aiLight", "rgba(91, 97, 214, 0.08)", "rgba(91, 97, 214, 0.08)"},
	warningSurface:      {"warningSurface", "#faeed9", "#3a3623"},
	warningBorder:       {"warningBorder", "rgba(245, 165, 36, 0.45)", "rgba(247, 183, 80, 0.45)"},
	warningText:         {"warningText", "#885700", "#f7b750"},
	infoSurface:         {"infoSurface", "#ddeefb", "#142935"},
	infoBorder:          {"infoBorder", "rgba(4, 133, 247, 0.45)", "rgba(4, 133, 247, 0.45)"},
	infoText:            {"infoText", "#005eb5", "#3d99ff"},
	successSurface:      {"successSurface", "#e0f6e9", "#163223"},
	successBorder:       {"successBorder", "rgba(23, 201, 100, 0.45)", "rgba(23, 201, 100, 0.45)"},
	successText:         {"successText", "#007132", "#17c964"},
	dangerSurface:       {"dangerSurface", "#fbe8e8", "#362220"},
	dangerBorder:        {"dangerBorder", "rgba(255, 56, 60, 0.45)", "rgba(219, 59, 62, 0.45)"},
	dangerText:          {"dangerText", "#c2001a", "#ff615e"},
	rControl:            {"r-control", "12px", "12px"},
	rMd:                 {"r-md", "16px", "16px"},
	rLg:                 {"r-lg", "20px", "20px"},
	shadowRest:          {"shadow-rest", "0 1px 2px 0 rgb(0 0 0 / 0.05)", "0 1px 2px 0 rgb(0 0 0 / 0.4)"},
	fontWeightMedium:    {"fontWeightMedium", "500", "500"},
}

// Tokens lists every design token the mail layout writes in.
func Tokens() []Token { return tokens[bgPage:] }

func (t token) light() string { return tokens[t].Light }

// Face is one rung of the product type scale, as a mail `font` shorthand.
type Face struct {
	Name         string
	Weight       int
	Size, Line   int
	Family       string
	fallbackList string
}

var (
	headingXLarge  = Face{"fontHeadingXLarge", 700, 28, 32, "Outfit", headingStack}
	headingSmall   = Face{"fontHeadingSmall", 700, 16, 20, "Outfit", headingStack}
	headingXXSmall = Face{"fontHeadingXXSmall", 700, 12, 16, "Outfit", headingStack}
	bodyLarge      = Face{"fontBodyLarge", 400, 16, 24, "Geist", bodyStack}
	body           = Face{"fontBody", 400, 14, 20, "Geist", bodyStack}
	bodySmall      = Face{"fontBodySmall", 400, 12, 16, "Geist", bodyStack}
)

// The product stacks end in system-ui, which older mail clients drop; Arial
// keeps the measure close where the web fonts are blocked.
const (
	headingStack = "'Outfit',system-ui,Arial,sans-serif"
	bodyStack    = "'Geist',system-ui,Arial,sans-serif"
)

// TypeScale lists the type rungs the mail layout uses.
func TypeScale() []Face {
	return []Face{headingXLarge, headingSmall, headingXXSmall, bodyLarge, body, bodySmall}
}

func (f Face) font() string {
	return fmt.Sprintf("%d %dpx/%dpx %s", f.Weight, f.Size, f.Line, f.fallbackList)
}

// medium is the same rung at the product's medium weight, as labels and
// buttons set it.
func (f Face) medium() string {
	return strings.Replace(f.font(), fmt.Sprint(f.Weight), fontWeightMedium.light(), 1)
}

// darkSheet re-colours every role class for a dark reader. Inline styles carry
// the light theme, which is what a client that strips <style> shows.
func darkSheet(scope string) string {
	var sheet strings.Builder
	for _, value := range tokens[bgPage:] {
		if value.Dark == value.Light || !isColor(value.Light) {
			continue
		}
		name := value.Name
		fmt.Fprintf(&sheet, "%[1]s .c-%[2]s{color:%[3]s!important}%[1]s .bg-%[2]s{background:%[3]s!important}%[1]s .bd-%[2]s{border-color:%[3]s!important}", scope, name, value.Dark)
	}
	return sheet.String()
}

func isColor(value string) bool {
	return strings.HasPrefix(value, "#") || strings.HasPrefix(value, "rgba(")
}
