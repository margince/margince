// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/winprobability"
)

// Source names where a deal's win probability comes from.
type Source string

// The three sources. A deal follows its stage until a colleague or Margince
// sets its own probability; a stage move sets it back to the stage.
const (
	SourceStage    Source = "stage"
	SourceHuman    Source = "human"
	SourceMargince Source = "margince"
)

// The deal columns that hold a probability set on the deal. Both are NULL
// while the deal follows its stage; deal_win_probability_pairing refuses one
// half without the other.
const (
	winProbabilityOverrideColumn = "win_probability_override"
	winProbabilitySetByColumn    = "win_probability_set_by"
)

// WinProbabilitySQL is the deal's effective win probability in SQL: its own
// where set, else its stage's. A scalar subquery, so a caller needs no join.
// EffectiveWinProbability is the same rule for a caller holding both halves;
// winprobability_integration_test.go holds the two equal.
func WinProbabilitySQL(alias string) string {
	return winprobability.SQL(dealRef(alias))
}

// WinProbabilitySourceSQL is where WinProbabilitySQL's value comes from.
func WinProbabilitySourceSQL(alias string) string {
	return fmt.Sprintf("COALESCE(%s.win_probability_set_by, '%s')", dealRef(alias), SourceStage)
}

// StageWinProbabilitySQL is the probability of the deal's current stage alone.
func StageWinProbabilitySQL(alias string) string {
	return fmt.Sprintf("(SELECT wp_stage.win_probability FROM stage wp_stage WHERE wp_stage.id = %s.stage_id)", dealRef(alias))
}

// dealRef qualifies the deal table: an unaliased query names it by table.
func dealRef(alias string) string {
	if alias == "" {
		return "deal"
	}
	return alias
}

// EffectiveWinProbability is WinProbabilitySQL for a caller holding the stored
// override, who set it, and the stage's probability.
func EffectiveWinProbability(override *int, setBy *string, stage int) (int, Source) {
	if override == nil || setBy == nil {
		return stage, SourceStage
	}
	return *override, Source(*setBy)
}

// setWinProbability writes a deal's own probability for the human edit, the
// stage move (nil, back to the stage) and the close-date sweep. It writes
// nothing when the value and source already hold, so a
// repeated write leaves no audit row behind.
func setWinProbability(p *storekit.Patch, current *int, currentSource *string, override *int, setBy Source) {
	var nextSource *string
	if override != nil {
		source := string(setBy)
		nextSource = &source
	}
	if sameInt(current, override) && sameString(currentSource, nextSource) {
		return
	}
	p.Set(winProbabilityOverrideColumn, current, override)
	p.Set(winProbabilitySetByColumn, currentSource, nextSource)
}

// storedWinProbability is what a deal read says the two override columns
// hold: nothing while the deal follows its stage.
func storedWinProbability(d crmcontracts.Deal) (*int, *string) {
	if d.WinProbability == nil || d.WinProbabilitySource == nil ||
		*d.WinProbabilitySource == crmcontracts.DealWinProbabilitySource(SourceStage) {
		return nil, nil
	}
	override, source := *d.WinProbability, string(*d.WinProbabilitySource)
	return &override, &source
}

func sameInt(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// AdvancedDeal is a deal after a stage move, with the probability the move
// set. Probability is nil when the move changed no stage. The deal is
// embedded, so a caller reads its fields as on any deal read. Wire is how it
// reaches the contract, since Deal's own JSON would drop Probability.
type AdvancedDeal struct {
	crmcontracts.Deal
	Probability *ProbabilitySet
}

// ProbabilitySet is what a stage move did to the deal's probability: it now
// follows the new stage.
type ProbabilitySet struct {
	WinProbability int
	StageName      string
	// Note is the same fact as one English sentence, for a model or a log.
	Note string
}

func newProbabilitySet(winProbability int, stageName string) *ProbabilitySet {
	return &ProbabilitySet{
		WinProbability: winProbability, StageName: stageName,
		Note: fmt.Sprintf("Probability set to %d%% by the stage %s", winProbability, stageName),
	}
}

// Wire is the advance answer the contract names: the deal and, when the move
// changed the stage, the probability it set. The deal travels through its own
// JSON so its custom fields arrive as they do on every other deal read.
func (a AdvancedDeal) Wire() (crmcontracts.DealAdvanced, error) {
	var out crmcontracts.DealAdvanced
	raw, err := json.Marshal(a.Deal)
	if err != nil {
		return out, fmt.Errorf("deals: encoding the advanced deal: %w", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("deals: decoding the advanced deal: %w", err)
	}
	if a.Probability != nil {
		out.ProbabilitySet = &crmcontracts.DealProbabilitySet{
			WinProbability: a.Probability.WinProbability, StageName: a.Probability.StageName, Note: a.Probability.Note,
		}
	}
	return out, nil
}
