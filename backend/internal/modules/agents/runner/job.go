// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// What a run is GIVEN: the goal, its seed grounding, the budget that bounds it,
// and the catalog entry's allowlist it inherits. Authority is deliberately not
// here — it rides the context principal, the same way every other surface
// carries it.

// Budget bounds one run (architecture/07 §4). Both are HARD per-run
// ceilings, deliberately independent of workspace-level budgets: one
// unattended run can never claim the whole workspace budget (RT-AI-H5).
type Budget struct {
	MaxSteps        int
	MaxOutputTokens int
}

// The §4 RATIFY defaults: 40 reason-act cycles sized to one deal-bundle
// pass, 50k output tokens per run.
const (
	DefaultMaxSteps        = 40
	DefaultMaxOutputTokens = 50_000
)

func (b Budget) withDefaults() Budget {
	if b.MaxSteps <= 0 {
		b.MaxSteps = DefaultMaxSteps
	}
	if b.MaxOutputTokens <= 0 {
		b.MaxOutputTokens = DefaultMaxOutputTokens
	}
	return b
}

// Job is one runner invocation: a goal over seed grounding under a
// budget. Authority is NOT here — it rides the context principal, the
// same way every other surface carries it.
type Job struct {
	Goal       string
	TriggerRef string
	Grounding  []Grounding
	Budget     Budget
	// Tools is the catalog entry's allowlist, carried from AgentSpec.Tools.
	//
	// EMPTY IS REFUSED. Run and Resume degrade a job carrying none before any
	// model call, because the only other reading of an empty list is "offer
	// everything the passport admits" — the whole catalog in every step of the
	// window. Every job names the tools its goal needs; the certification lane
	// builds its jobs from the same catalog entries production runs.
	Tools []string
	// LanguageRule is the RENDERED "write in this language" block for the run's
	// final summary, already text rather than a language code.
	//
	// Rendered by compose and passed in, because that block is spelled in
	// compose/promptlang and a module may not import compose. Passing a code
	// instead would mean this package rendering its own copy of the rule, which
	// is the second spelling that package exists to prevent.
	//
	// Held by: TestOnlyPromptlangSpellsTheLanguageRule (backend/gates/promptlanguage_test.go)
	//
	// EMPTY MEANS NO RULE. Production always renders one — English when an
	// installation has chosen none — and the certification lane passes the
	// English rule, as every other certified site does, so two installations'
	// scores stay comparable while the prompt stays the one a run sends.
	LanguageRule string
}

// Grounding is one provenance-stamped seed context item (§3): T2
// content is spotlighted as data-not-instructions before it enters the
// prompt.
type Grounding struct {
	SourceID  string
	TrustTier string // "T0" | "T1" | "T2"
	Content   string
}
