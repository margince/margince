// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealstatus composes deal assessments and chooses the current next step.
// Both the deal page and Worklist use its deterministic rules and owning reads.
// Worklist batches those facts before selection; it never requires a saved card.
//
// Saved model assessments retain their written date and citation access checks.
// Current actions have a separate source/policy basis, checked again when accepted.
// Receipt-bearing task writes require Serializable isolation so concurrent acceptance
// cannot create duplicate work from the same absent-task observation.
package dealstatus
