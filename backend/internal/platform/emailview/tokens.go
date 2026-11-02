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
	textOnAccentControl
	warningText
	infoText
	successText
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
	textOnAccentControl: {"textOnAccentControl", "#f4faf7", "#07221d"},
	warningText:         {"warningText", "#885700", "#f7b750"},
	infoText:            {"infoText", "#005eb5", "#3d99ff"},
	successText:         {"successText", "#007132", "#17c964"},
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
	headingXLarge  = Face{"fontHeadingXLarge", 700, 28, 32, headingFamily, headingStack}
	headingSmall   = Face{"fontHeadingSmall", 700, 16, 20, headingFamily, headingStack}
	wordmark       = Face{"fontHeadingMedium", 700, 20, 24, headingFamily, headingStack}
	headingXXSmall = Face{"fontHeadingXXSmall", 700, 12, 16, headingFamily, headingStack}
	bodyLarge      = Face{"fontBodyLarge", 400, 16, 24, bodyFamily, bodyStack}
	body           = Face{"fontBody", 400, 14, 20, bodyFamily, bodyStack}
	bodySmall      = Face{"fontBodySmall", 400, 12, 16, bodyFamily, bodyStack}
)

// The product stacks end in system-ui, which older mail clients drop; Arial
// keeps the measure close where the web fonts are blocked.
const (
	headingFamily = "Outfit"
	bodyFamily    = "Geist"
	headingStack  = "'" + headingFamily + "',system-ui,Arial,sans-serif"
	bodyStack     = "'" + bodyFamily + "',system-ui,Arial,sans-serif"
)

// TypeScale lists the type rungs the mail layout uses.
func TypeScale() []Face {
	return []Face{headingXLarge, wordmark, headingSmall, headingXXSmall, bodyLarge, body, bodySmall}
}

func (f Face) font() string {
	return fmt.Sprintf("%d %dpx/%dpx %s", f.Weight, f.Size, f.Line, f.fallbackList)
}

// medium is the same rung at the product's medium weight, as labels and
// buttons set it.
func (f Face) medium() string {
	return strings.Replace(f.font(), fmt.Sprint(f.Weight), fontWeightMedium.light(), 1)
}

// darkSheet repaints every role class for a dark reader. Inline styles carry
// the light theme, which is what a client that strips <style> shows.
func darkSheet(scope string) string {
	var sheet strings.Builder
	for _, value := range tokens[bgPage:] {
		name := value.Name
		switch {
		case value.Dark == value.Light:
		case isColor(value.Light):
			fmt.Fprintf(&sheet, "%[1]s .c-%[2]s{color:%[3]s!important}%[1]s .bg-%[2]s{background:%[3]s!important}%[1]s .bd-%[2]s{border-color:%[3]s!important}", scope, name, value.Dark)
		case strings.HasPrefix(name, "shadow-"):
			fmt.Fprintf(&sheet, "%s .sh-%s{box-shadow:%s!important}", scope, name, value.Dark)
		}
	}
	return sheet.String()
}

func isColor(value string) bool {
	return strings.HasPrefix(value, "#") || strings.HasPrefix(value, "rgba(")
}
