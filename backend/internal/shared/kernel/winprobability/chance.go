// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package winprobability

import (
	"fmt"
	"strings"
)

// Chance is a deal's one win chance and everything its explanation names.
// forecasting.ResolveWinChance builds it; every surface that shows a deal's
// chance reads it, so the number and its words are the same everywhere.
type Chance struct {
	// Percent is the chance every surface shows and weights by.
	Percent int
	// Source is "stage", "human" or "margince": where the deal's probability
	// comes from.
	Source string
	// Typed is the stage's typed chance.
	Typed int
	// InUse is the typed chance moved by the stage's recorded outcomes, before
	// any risk effect.
	InUse int
	// EvidenceN is how many closed entries moved it; nil where learning is off
	// or the deal's probability was set on the deal.
	EvidenceN *int
	WonShare  int
	// Effect is the odds ratio applied to InUse; nil when no signal moved it.
	Effect       *float64
	EffectSignal string
	// EffectDays is how long the effect's signal has held, where it has a length.
	EffectDays *int
	// EffectLearned is false when the quiet effect is the fixed halving
	// because too few closed deals back a learned one.
	EffectLearned  bool
	EffectSample   int
	SelfCorrecting bool
}

// Line is the one-line explanation of the chance, the same words on every
// surface that prints it in English.
func (c Chance) Line() string {
	var parts []string
	switch c.Source {
	case "human":
		parts = append(parts, fmt.Sprintf("Set by a colleague at %d%%", c.Percent))
	case "margince":
		parts = append(parts, fmt.Sprintf("Set by Margince at %d%%", c.Percent))
	default:
		parts = append(parts, c.stageLine()...)
	}
	if c.Effect != nil {
		parts = append(parts, c.effectLine())
	}
	if c.Source == "stage" && (c.Effect != nil || c.InUse != c.Typed) {
		parts = append(parts, fmt.Sprintf("%d%%", c.Percent))
	}
	return strings.Join(parts, " · ")
}

func (c Chance) stageLine() []string {
	parts := []string{fmt.Sprintf("Stage typed %d%%", c.Typed)}
	switch {
	case !c.SelfCorrecting:
		parts = append(parts, "learning from closed deals is off")
	case c.EvidenceN != nil && *c.EvidenceN > 0:
		parts = append(parts, fmt.Sprintf("%d deals from here closed, %d%% won", *c.EvidenceN, c.WonShare))
	}
	return parts
}

func (c Chance) effectLine() string {
	name := strings.ReplaceAll(c.EffectSignal, "_", " ")
	if c.EffectDays != nil {
		name += fmt.Sprintf(" %d days", *c.EffectDays)
	}
	if !c.EffectLearned {
		return name + " halves it (too few closed deals to learn from)"
	}
	verb := "lowers"
	if *c.Effect > 1 {
		verb = "raises"
	}
	return fmt.Sprintf("%s %s it (learned from %d closed deals)", name, verb, c.EffectSample)
}
