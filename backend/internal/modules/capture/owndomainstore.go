// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The administrator's view of the own-domain set (CAP-WIRE-2a).
//
// Every human role READS it — a rep who cannot find a thread deserves to see
// why — and only admin/ops change it, because the set decides whether
// correspondence is stored at all. Human-only throughout: an agent must not
// widen or narrow what the CRM may hold. What a connected mailbox contributes
// is a candidate; confirming one is a human act, and this is where it happens.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/idna"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditKeyOwnDomain is the audit-image key naming which domain a change was
// about — the field the trail is searched by.
const auditKeyOwnDomain = "own_email_domain"

// OwnDomain is one registered domain and how it got there.
type OwnDomain struct {
	Domain    string
	Source    string
	Verified  bool
	CreatedAt time.Time
}

// OwnDomainStore reads and writes the workspace's own-domain registry.
type OwnDomainStore struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
}

// NewOwnDomainStore builds the store over the app pool.
func NewOwnDomainStore(db *database.DB) *OwnDomainStore {
	return &OwnDomainStore{db: db}
}

// OwnDomainList is the registry plus what the installation's own company
// claims. The two are reported separately because only one of them is editable
// here: a company's own domains come from its profile and are changed there, so
// showing them as removable rows would offer an action this surface cannot
// perform.
type OwnDomainList struct {
	Domains       []OwnDomain
	AnchorDomains []string
}

// List returns the registry and the company's claimed domains.
func (s *OwnDomainStore) List(ctx context.Context) (OwnDomainList, error) {
	var out OwnDomainList
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionRead); err != nil {
		return OwnDomainList{}, err
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT domain, source, verified, created_at
			  FROM workspace_email_domain ORDER BY domain`)
		if err != nil {
			return fmt.Errorf("capture: listing own domains: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d OwnDomain
			if err := rows.Scan(&d.Domain, &d.Source, &d.Verified, &d.CreatedAt); err != nil {
				return fmt.Errorf("capture: listing own domains: %w", err)
			}
			out.Domains = append(out.Domains, d)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("capture: listing own domains: %w", err)
		}

		claimed, err := queryDomains(ctx, tx, anchorDomains)
		if err != nil {
			return err
		}
		out.AnchorDomains = claimed.Domains()
		return nil
	})
	return out, err
}

// Colleagues answers which addresses belong to the workspace itself, so a
// caller composing a reply can tell a co-worker without a seat from a
// counterparty. It reads the TRUSTED set — domains an administrator vouched
// for plus the company's own — and not the mailbox-seeded candidates: a
// contractor who connects a mailbox at a customer must not turn that whole
// customer into colleagues nobody may answer. A workspace with nothing
// registered has no colleagues this can name.
//
// Gated as a person read, not a settings read: whether somebody is a
// colleague is a fact about them, asked by a caller who already holds their
// address, and the same grant that let them reach the address answers it. A
// rep composing a reply holds no settings authority and must not need one.
func (s *OwnDomainStore) Colleagues(ctx context.Context) (InternalDomains, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return InternalDomains{}, err
	}
	var own InternalDomains
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		own, err = trustedOwnDomainsTx(ctx, tx)
		return err
	})
	return own, err
}

// Add registers a domain as the workspace's own, verified.
//
// An administrator entering a domain IS the human vouching for it — there is no
// second confirmation step, because there is no one else to ask. Idempotent on
// the folded domain: adding one a mailbox already contributed confirms it
// rather than failing.
func (s *OwnDomainStore) Add(ctx context.Context, raw string) (OwnDomain, error) {
	// Authorization before input: a caller with no grant is told they may not,
	// not what is wrong with a value they were never allowed to submit. Outside
	// the transaction, like every sibling store — a refused caller must not open
	// one and set the workspace GUC first.
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return OwnDomain{}, err
	}
	domain, err := ValidOwnDomain(raw)
	if err != nil {
		return OwnDomain{}, err
	}
	var out OwnDomain
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Serialize this domain's registration before the row is read. The
		// prior state chooses which audit door this write takes, and the CTE
		// below does NOT close that window on its own: under READ COMMITTED the
		// ON CONFLICT re-check resolves against a row a mailbox sync committed
		// after the CTE's snapshot was taken, so the two would disagree and the
		// row would claim there was nothing before. Every writer of the table
		// takes it — the sync's seed and the removal as well as this.
		if err := lockOwnDomain(ctx, tx, domain); err != nil {
			return err
		}
		// The prior state, so the trail distinguishes "an admin registered a new
		// domain" from "an admin confirmed a candidate a mailbox had seen".
		//
		// Read by the upsert itself, in a CTE: the statement that replaces the
		// row is the only thing that can say what it replaced, and under the
		// lock its snapshot is the same state the upsert acts on.
		var priorSource *string
		var priorVerified *bool
		if err := tx.QueryRow(ctx, `
			WITH was AS (
			  SELECT source, verified FROM workspace_email_domain WHERE domain = $1
			)
			INSERT INTO workspace_email_domain (domain, source, verified)
			VALUES ($1, 'admin', true)
			ON CONFLICT (domain)
			  DO UPDATE SET source = 'admin', verified = true
			RETURNING domain, source, verified, created_at,
			          (SELECT was.source FROM was), (SELECT was.verified FROM was)`, domain).
			Scan(&out.Domain, &out.Source, &out.Verified, &out.CreatedAt, &priorSource, &priorVerified); err != nil {
			return fmt.Errorf("capture: registering own domain: %w", err)
		}
		var beforeImage map[string]any
		if priorSource != nil && priorVerified != nil {
			beforeImage = map[string]any{
				auditKeyOwnDomain: domain, "source": *priorSource, "verified": *priorVerified,
			}
		}
		// Audit-only, like the capture-settings write beside it: this is
		// workspace configuration, not a domain record, and the closed event
		// catalog carries no type for it. The audit row is the durable answer to
		// "who put this domain in", which is the question that will be asked.
		after := map[string]any{auditKeyOwnDomain: domain, "source": "admin", "verified": true}
		if beforeImage == nil {
			// A domain nobody had seen: the list gained an entry, and there is
			// no prior source or verification for the row to name.
			_, err := storekit.AuditEvent(ctx, tx, "update", captureSettingsObject,
				storekit.MustWorkspace(ctx), after)
			return err
		}
		// A candidate a mailbox had already seen, now confirmed: source and
		// verified moved, and the row says what they moved from.
		_, err := storekit.Audit(ctx, tx, "update", captureSettingsObject,
			storekit.MustWorkspace(ctx), beforeImage, after)
		return err
	})
	return out, err
}

// Remove stops treating a domain as the workspace's own.
//
// Removing one the company itself claims does nothing on its own: that claim
// lives on the company profile and is changed there, and the list reports the
// two apart so the surface can say which is which.
func (s *OwnDomainStore) Remove(ctx context.Context, raw string) error {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return err
	}
	// After the gate: "@" and " " both normalize to nothing, and answering 204
	// for them without checking the grant would leave a hole the size of one
	// unusual path segment.
	domain := normalizeDomain(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
	if domain == "" {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The same lock the registration takes: a removal committing while a
		// registration reads the prior state would leave that row naming a
		// candidate the list no longer held.
		if err := lockOwnDomain(ctx, tx, domain); err != nil {
			return err
		}
		// The domain IS the key now (ADR-0091 §8 phase D): one installation, one
		// list, and the unique index the insert above conflicts on says the same.
		tag, err := tx.Exec(ctx,
			`DELETE FROM workspace_email_domain WHERE domain = $1`, domain)
		if err != nil {
			return fmt.Errorf("capture: removing own domain: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		// "archive", not a delete verb: the audit vocabulary is closed (0018)
		// and withdrawing a registry entry IS retiring it — the same reading
		// the consumer-mail list settled on.
		_, err = storekit.Audit(ctx, tx, "archive", captureSettingsObject,
			storekit.MustWorkspace(ctx),
			map[string]any{auditKeyOwnDomain: domain}, nil)
		return err
	})
}

// lockOwnDomain serializes every writer of one own-domain entry, so the state
// a registration replaces is read under the same lock that replaces it.
func lockOwnDomain(ctx context.Context, tx pgx.Tx, domain string) error {
	return storekit.LockWriteIdentity(ctx, tx, ownDomainWriteIdentity, domain)
}

// ownDomainWriteIdentity names the write identity an own-domain entry's writers
// serialize on.
const ownDomainWriteIdentity = "workspace_email_domain"

// ValidOwnDomain vets a domain and returns its stored form.
//
// Exported so the handler can answer 422 naming what is wrong, the way the
// consumer-mail surface does; the store calls it too, as its own floor, so a
// non-HTTP caller cannot write a row the matcher will never read.
//
// The value decides whether mail is stored, so a mistyped one is refused with
// what to do about it rather than folded into something that silently matches
// nothing.
func ValidOwnDomain(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@")))
	domain = strings.TrimSuffix(domain, ".")
	switch {
	case domain == "":
		return "", errors.New("give a domain, for example acme.com")
	case strings.ContainsAny(domain, "@/ "):
		return "", errors.New("give a bare domain — no address, scheme or path")
	case !strings.Contains(domain, "."):
		return "", fmt.Errorf("%s is not a domain", domain)
	case net.ParseIP(domain) != nil:
		// Named before the public-suffix test below, which would otherwise tell
		// somebody who typed an address that it "is a public suffix" — true of
		// the lookup and useless to the reader.
		return "", fmt.Errorf("%s is an IP address; give the domain your mail is addressed to", domain)
	case len(domain) > 253:
		return "", errors.New("that domain is too long")
	}
	// normalizeDomain deliberately keeps a domain IDNA cannot read, because its
	// original caller must not turn a parse failure into "internal". Here the
	// opposite is right: a domain nothing can match is a typo, and storing it
	// would look like protection that never fires.
	ascii, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return "", fmt.Errorf("%s is not a usable domain name", domain)
	}
	// A public suffix is refused here so the person typing it is told why.
	// NewInternalDomains drops one anyway, whichever writer it came from — this
	// is the readable error, not the guarantee.
	if !ownableDomain(ascii) {
		return "", fmt.Errorf("%s is a public suffix, not a company's domain — give the domain you register mail under", domain)
	}
	return ascii, nil
}

// ColleagueDomainsTx is the vouched-for own-domain set, read inside a
// transaction the caller already holds.
//
// It exists beside Colleagues rather than replacing it because the two differ
// in exactly the way that matters to a caller comparing several reads: this one
// borrows the caller's transaction, so a set of counts meant to differ by one
// rule sees ONE snapshot of the domains. Colleagues opens its own, which is
// right for a single question asked once.
//
// Gated as an activity read, not a person read. The caller is judging
// correspondence — which messages are a colleague's — and the grant that let
// them read the message is the grant that answers it. Requiring person:read
// would refuse a reader who may see the mail but not the contact, which is a
// narrowing that has nothing to do with the question.
//
// Returns the normalized domains rather than the set, because the caller tests
// them in SQL: a predicate cannot run before a scan cap, and a colleague rule
// applied after one lets internal threads fill the scan.
func (s *OwnDomainStore) ColleagueDomainsTx(ctx context.Context, tx pgx.Tx) ([]string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	own, err := trustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	return own.Domains(), nil
}
