// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The reasons the import writers give for a row that did not land as asked.
// One place, so the test run and the commit say a refusal in the same words
// and carry the same code.

import (
	"github.com/margince/margince/backend/internal/modules/migration"
)

// The three "held" reasons name the row's own value and nothing about the
// incumbent. They never say who holds it, nor whether the caller could see them.
// duplicateEmailWhy is a merge candidate the run discloses and does not resolve.
var (
	boundArchivedWhy        = migration.Why(migration.ReasonRecordArchived)
	linkedInClaimedWhy      = migration.Why(migration.ReasonLinkedinHeld)
	domainClaimedWhy        = migration.Why(migration.ReasonDomainHeld)
	contactEmailClaimedWhy  = migration.Why(migration.ReasonContactEmailHeld)
	duplicateEmailWhy       = migration.Why(migration.ReasonDuplicateEmail)
	naturalKeyTakenWhy      = migration.Why(migration.ReasonNaturalKeyTaken)
	ownerRefusedWhy         = migration.Why(migration.ReasonOwnerRefused)
	unknownOwnerWhy         = migration.WhyCount(migration.ReasonOwnerUnknown, "rows", 1)
	newTagWhy               = migration.Why(migration.ReasonTagNew)
	addressNotSplitWhy      = migration.Why(migration.ReasonAddressNotSplit)
	createdTagWhy           = migration.Why(migration.ReasonTagCreated)
	contactUnnamedWhy       = migration.Why(migration.ReasonContactUnnamed)
	companyNameEmptyWhy     = migration.Why(migration.ReasonCompanyNameEmpty)
	employerColumnEmptyWhy  = migration.Why(migration.ReasonEmployerColumnEmpty)
	companyCollisionWhy     = migration.Why(migration.ReasonCompanyDuplicateFound)
	contactCollisionWhy     = migration.Why(migration.ReasonContactDuplicateFound)
	companyDuplicateSkipWhy = migration.Why(migration.ReasonCompanyDuplicateSkipped)
	contactDuplicateSkipWhy = migration.Why(migration.ReasonContactDuplicateSkipped)
)

// skippedFor is the result of a row skipped for why.
func skippedFor(why migration.Reason) migration.EnsureResult {
	return migration.EnsureResult{Skipped: true, SkipReason: why.English(), SkipWhy: why}
}

// notLinkedFor is the result of an edge left unapplied for why.
func notLinkedFor(why migration.Reason) migration.AssocResult {
	return migration.AssocResult{Reason: why.English(), Why: why}
}

// skippedWithProblem is skippedFor for a refusal that is also a report issue.
func skippedWithProblem(problem *migration.SkippedRow) migration.EnsureResult {
	res := skippedFor(problem.Why())
	res.SkipReason, res.Problem = problem.Reason, problem
	return res
}
