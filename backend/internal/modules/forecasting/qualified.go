// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// ViabilityBasis is what a deal's qualification rested on when it was read,
// frozen beside it on each contribution. Nil while enforcement is off.
type ViabilityBasis struct {
	// PolicyVersion is the viability policy enforced for the read.
	PolicyVersion string
	// AssessedAt is when the input check last assessed the deal, nil if never.
	AssessedAt *time.Time
	// Disposition is the review standing: none, open_review, override or
	// unsupported.
	Disposition string
	// Complete is true when the deal was judged on its current evidence under
	// this policy. False is what makes a deal unknown.
	Complete bool
}

// The review standings a basis records, matching the migration's CHECK.
const (
	DispositionNone        = "none"
	DispositionOpenReview  = "open_review"
	DispositionOverride    = "override"
	DispositionUnsupported = "unsupported"
)

// QualifiedReadings are the readings restricted to deals whose customer still
// supports them. Nil on Readings while enforcement is off.
//
// Excluded and unknown are counted apart. An excluded deal was judged and
// found unsupported; an unknown one could not be judged. Counting it as
// excluded would report a gap in the mailbox as a gap in the pipeline.
type QualifiedReadings struct {
	// OpenMinor is the open money of the deals the qualified readings keep, the
	// qualified counterpart of Readings.OpenMinor.
	OpenMinor     int64
	EvidenceMinor int64
	BestCaseMinor int64
	WeightedMinor int64
	// The baseline open money left out, and how many deals carry it.
	ExcludedMinor int64
	UnknownMinor  int64
	ExcludedCount int
	UnknownCount  int
	// The part of the excluded money with no review behind it. Such a deal has a
	// weak close date or no next step, and the customer still talks about it.
	UnsupportedMinor int64
	UnsupportedCount int
	// AmountsPartial is true when an excluded or unknown deal had no amount
	// the reader can see: unpriced, no exchange rate, or masked. The two
	// subtotals are then lower than the money those deals carry.
	AmountsPartial bool
}

// The qualification modes a read reports beside its qualified block.
const (
	QualificationOff       = "off"
	QualificationEnforcing = "enforcing"
)

// QualificationMode says whether the qualified forecast was enforced for this
// read, apart from whether any deal fell in it.
func (r Readings) QualificationMode() string {
	if r.QualifiedPolicy != "" || r.Qualified != nil {
		return QualificationEnforcing
	}
	return QualificationOff
}

// ApplyQualifiedPolicy records the policy enforced for this read. While one is
// enforced the qualified block always exists, so a scope with no deals answers
// zero rather than reading as enforcement off.
func (r *Readings) ApplyQualifiedPolicy(policy string) {
	r.QualifiedPolicy = policy
	if policy != "" && r.Qualified == nil {
		r.Qualified = &QualifiedReadings{}
	}
}

// qualify adds one contribution to the qualified readings, starting them on
// the first qualified deal. The baseline weight is reused, never recomputed, so
// a qualified total is always a subset of the baseline it sits beside.
func qualify(out *Readings, c Contribution, base int64) error {
	if c.Qualification == viability.QualificationNotApplied {
		return nil
	}
	if out.Qualified == nil {
		out.Qualified = &QualifiedReadings{}
	}
	q := out.Qualified
	if !c.InOpen {
		return nil
	}
	adds := []struct {
		into *int64
		by   int64
		when bool
	}{
		{&q.OpenMinor, base, c.Qualification == viability.QualificationEligible},
		{&q.EvidenceMinor, base, c.InEvidence && c.Qualification == viability.QualificationEligible},
		{&q.BestCaseMinor, base, c.InBestCase && c.Qualification == viability.QualificationEligible},
		{&q.WeightedMinor, c.WeightedMinor, c.Qualification == viability.QualificationEligible},
		{&q.ExcludedMinor, base, excluded(c.Qualification)},
		{&q.UnsupportedMinor, base, c.Qualification == viability.QualificationUnsupported},
		{&q.UnknownMinor, base, c.Qualification == viability.QualificationUnknown},
	}
	for _, add := range adds {
		if !add.when {
			continue
		}
		sum, err := addMinor(*add.into, add.by)
		if err != nil {
			return err
		}
		*add.into = sum
	}
	switch c.Qualification {
	case viability.QualificationExcluded:
		q.ExcludedCount++
	case viability.QualificationUnsupported:
		q.ExcludedCount++
		q.UnsupportedCount++
	case viability.QualificationUnknown:
		q.UnknownCount++
	}
	if c.Qualification != viability.QualificationEligible && c.BaseMinor == nil {
		q.AmountsPartial = true
	}
	return nil
}

// excluded reports a deal the qualified readings leave out, under review or
// unsupported alike.
func excluded(q viability.Qualification) bool {
	return q == viability.QualificationExcluded || q == viability.QualificationUnsupported
}

// basisColumns spreads a basis over its four contribution columns, all NULL
// when there is none.
func basisColumns(b *ViabilityBasis) (policy *string, assessed *time.Time, disposition *string, complete *bool) {
	if b == nil {
		return nil, nil, nil, nil
	}
	return &b.PolicyVersion, b.AssessedAt, &b.Disposition, &b.Complete
}

// frozenBasis is the four basis columns as a row holds them, any of them NULL.
type frozenBasis struct {
	policy, disposition *string
	assessed            *time.Time
	complete            *bool
}

// read answers the basis a row froze, nil on a row frozen without one.
func (b frozenBasis) read() *ViabilityBasis {
	if b.policy == nil || b.disposition == nil || b.complete == nil {
		return nil
	}
	return &ViabilityBasis{PolicyVersion: *b.policy, AssessedAt: b.assessed, Disposition: *b.disposition, Complete: *b.complete}
}

// QualifiedOf rebuilds the qualified readings from frozen rows, the way
// Compute builds them from live deals. The caller asks only of rows frozen
// under an enforced policy.
func QualifiedOf(rows []Contribution) (QualifiedReadings, error) {
	var out Readings
	for _, c := range rows {
		base := int64(0)
		if c.BaseMinor != nil {
			base = *c.BaseMinor
		}
		if err := qualify(&out, c, base); err != nil {
			return QualifiedReadings{}, err
		}
	}
	if out.Qualified == nil {
		return QualifiedReadings{}, nil
	}
	return *out.Qualified, nil
}

// basisAgrees answers whether every row on both sides froze a basis under one
// policy. A qualified reading compared across rows frozen without one, or
// under two, is a definition change and not deals moving.
func basisAgrees(from, to snapshotSide) bool {
	policy := ""
	for _, side := range [][]Contribution{from.Contributions, to.Contributions} {
		for _, c := range side {
			if c.Basis == nil {
				return false
			}
			if policy == "" {
				policy = c.Basis.PolicyVersion
			}
			if c.Basis.PolicyVersion != policy {
				return false
			}
		}
	}
	return true
}
