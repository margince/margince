// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a proposal lands in an initiative's goal and rule: it fills what the
// words named and leaves the rest as the reader had it.

import { type Goal, OBJECTIVES, type Rule } from "./reporting.initiativegoal";
import type { ReportingSelection } from "./reporting.model";
import type { SetupProposal } from "./reporting.setupproposal";

/**
 * The selection with the proposal's dimensions, metrics and charts over
 * it. The metrics and blocks arrays are the selection's own when the words
 * named none, so a caller can tell an unchanged list by identity.
 */
export function selectionWithProposal(
  selection: ReportingSelection,
  proposal: SetupProposal,
): ReportingSelection {
  const proposed = proposal.selection;
  return {
    ...selection,
    product_id: proposed.product_id ?? selection.product_id,
    industry: proposed.industry ?? selection.industry,
    country: proposed.country ?? selection.country,
    period: proposed.period ?? selection.period,
    metrics:
      proposed.metrics && proposed.metrics.length > 0
        ? proposed.metrics
        : selection.metrics,
    blocks:
      proposed.blocks && proposed.blocks.length > 0
        ? proposed.blocks
        : selection.blocks,
  };
}

export function goalWithProposal(goal: Goal, proposal: SetupProposal): Goal {
  const objective = OBJECTIVES.find((id) => id === proposal.objective?.metric);
  return {
    ...goal,
    name: goal.name.trim() === "" && proposal.name ? proposal.name : goal.name,
    objective: objective ?? goal.objective,
    target:
      proposal.objective?.target_value === undefined
        ? goal.target
        : String(proposal.objective.target_value),
    deadline: proposal.objective?.deadline ?? goal.deadline,
  };
}

export function ruleWithProposal(rule: Rule, proposal: SetupProposal): Rule {
  const proposed = proposal.selection;
  return {
    product: proposed.product_id ?? rule.product,
    industry: proposed.industry ?? rule.industry,
    country: proposed.country ?? rule.country,
  };
}
