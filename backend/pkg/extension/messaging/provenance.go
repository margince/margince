// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messaging

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Instrument is one law or decree a rule set states, and when it took effect.
//
// It documents the record and binds nothing. A decision records the ruleset
// version it was judged under; this says what that number meant. Two
// instruments with different commencement dates may both be live, as when a
// decree amends rather than replaces. So this is a list, and the pack states
// every instrument its obligations rest on.
type Instrument struct {
	// Name is the instrument as a regulator cites it, e.g.
	// "Decree 91/2020/ND-CP". Not translated: a citation is the same string
	// in every locale, and translating one would make it unfindable.
	Name string

	// EffectiveFrom is the instrument's own commencement date, never the date
	// this product learned about it. Zero means the pack did not state one,
	// which is legal and says less rather than saying "the epoch".
	EffectiveFrom time.Time

	// Provision is the part a citation names when it states none of its own.
	// It is written as the instrument prints it. Empty names none.
	Provision string

	// SourceURL is the official published text, over https. Empty states none.
	SourceURL string

	// ExpiresFrom is the first instant the instrument no longer binds. Zero
	// means it states no end. Once past, a citation of it is no longer shown.
	ExpiresFrom time.Time
}

// Validate refuses an instrument that names nothing, links to an unsafe
// source, or ends before it begins.
func (i Instrument) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return errors.New("an instrument carries no name — a commencement date with nothing to commence names no law")
	}
	if i.SourceURL != "" {
		if err := validateSourceURL(i.SourceURL); err != nil {
			return fmt.Errorf("instrument %q: %w", i.Name, err)
		}
	}
	if !i.ExpiresFrom.IsZero() && !i.EffectiveFrom.IsZero() && !i.ExpiresFrom.After(i.EffectiveFrom) {
		return fmt.Errorf("instrument %q expires on or before it takes effect", i.Name)
	}
	return nil
}

// Provenance is who reviewed a rule set against its sources, and when.
//
// Two dates that read alike and mean different things: ReviewDue passing says
// nobody has re-read the pack in time, and the pack still applies. An
// instrument's ExpiresFrom passing says the law itself ended. The first
// silences the pack's citations; the second silences only that instrument's.
type Provenance struct {
	// CreatedAt is when the rule set was first written.
	CreatedAt time.Time
	// SourceCheckedAt is when its instruments were last compared with their
	// published texts.
	SourceCheckedAt time.Time
	// ReviewedAt is when a named reviewer last approved it. Zero is a draft,
	// and a draft cannot cite.
	ReviewedAt time.Time
	// ReviewDue is when the next review is owed. Zero sets no due date.
	ReviewDue time.Time
	// Reviewer names who approved it, as the pack's maintainers sign.
	Reviewer string
}

// validate refuses a review that names nobody or falls due before it happened.
func (p Provenance) validate() error {
	if p.ReviewedAt.IsZero() {
		return nil
	}
	if strings.TrimSpace(p.Reviewer) == "" {
		return errors.New("the rule set records a review and no reviewer")
	}
	if !p.ReviewDue.IsZero() && !p.ReviewDue.After(p.ReviewedAt) {
		return errors.New("the rule set's next review falls due on or before its last one")
	}
	return nil
}

// Territory narrows a citation to where the sending legal entity is
// established, relative to the jurisdiction whose pack declares it. Closed,
// because the engine has to decide each one.
type Territory string

const (
	// AnySender cites whatever the sender's country.
	AnySender Territory = ""
	// SenderHere cites only a sender established in this jurisdiction.
	SenderHere Territory = "sender_here"
	// SenderElsewhere cites only a sender established outside it.
	SenderElsewhere Territory = "sender_elsewhere"
)

// Validate enforces membership in the closed set.
func (t Territory) Validate() error {
	switch t {
	case AnySender, SenderHere, SenderElsewhere:
		return nil
	}
	return fmt.Errorf("territory %q is not one the engine decides", string(t))
}

// Citation names the law behind one finding or refusal the engine reports.
//
// One code can rest on different law in two markets. So a citation belongs to
// the pack of the recipient it explains, and Territory narrows it further.
type Citation struct {
	// Code is the engine's finding or refusal code this explains, e.g.
	// "frequency_cap_reached".
	Code string
	// Territory is where the sender must be established for this to apply.
	Territory Territory
	// Instrument is the Name of one of the rule set's Instruments.
	Instrument string
	// Provision is the part of the instrument, e.g. "Art. 21(2)". Empty takes
	// the instrument's.
	Provision string
	// SourceURL is the official text of the provision, over https. Empty
	// takes the instrument's.
	SourceURL string
}

// validateCitations refuses a citation that names no declared law, has no
// provision or source to link, or comes from a pack nobody reviewed.
func (r Rules) validateCitations() error {
	if err := r.Provenance.validate(); err != nil {
		return err
	}
	if len(r.Citations) > 0 && r.Provenance.ReviewedAt.IsZero() {
		return errors.New("the rule set cites law and records no review — a draft rule cannot cite")
	}
	cited := map[string][]Territory{}
	for _, c := range r.Citations {
		if strings.TrimSpace(c.Code) == "" || strings.TrimSpace(c.Code) != c.Code {
			return fmt.Errorf("citation code %q is empty or padded", c.Code)
		}
		if err := c.Territory.Validate(); err != nil {
			return fmt.Errorf("citation %q: %w", c.Code, err)
		}
		resolved, err := r.ResolveCitation(c)
		if err != nil {
			return err
		}
		if err := validateSourceURL(resolved.SourceURL); err != nil {
			return fmt.Errorf("citation %q: %w", c.Code, err)
		}
		for _, other := range cited[c.Code] {
			if other == c.Territory || other == AnySender || c.Territory == AnySender {
				return fmt.Errorf("citation %q is declared twice for one sender — two laws for one finding is two answers", c.Code)
			}
		}
		cited[c.Code] = append(cited[c.Code], c.Territory)
	}
	return nil
}

// ResolveCitation fills a citation's empty provision and source from the
// instrument it names. It refuses a citation that names no single declared
// instrument, or that ends up with no provision.
func (r Rules) ResolveCitation(c Citation) (Citation, error) {
	var named []Instrument
	for _, in := range r.Instruments {
		if in.Name == c.Instrument {
			named = append(named, in)
		}
	}
	if len(named) != 1 {
		return Citation{}, fmt.Errorf("citation %q names instrument %q, which the rule set declares %d times — it must name exactly one",
			c.Code, c.Instrument, len(named))
	}
	if c.Provision == "" {
		c.Provision = named[0].Provision
	}
	if c.SourceURL == "" {
		c.SourceURL = named[0].SourceURL
	}
	if strings.TrimSpace(c.Provision) == "" {
		return Citation{}, fmt.Errorf("citation %q names no provision, and its instrument states none", c.Code)
	}
	return c, nil
}

// validateSourceURL accepts an absolute https address and nothing else. The
// link is shown to a rep, and another scheme is one a page could abuse.
func validateSourceURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("source %q is not an absolute https address", raw)
	}
	return nil
}

// digestLength is a SHA-256 written as lower-case hex.
const digestLength = 64

// validateDigest accepts no digest, or a SHA-256 in lower-case hex.
func (r Rules) validateDigest() error {
	if r.Digest == "" {
		return nil
	}
	if len(r.Digest) != digestLength || strings.Trim(r.Digest, "0123456789abcdef") != "" {
		return fmt.Errorf("digest %q is not a SHA-256 in lower-case hex", r.Digest)
	}
	return nil
}
