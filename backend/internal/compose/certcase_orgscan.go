// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for the account scan (account_scan/org_scan).
//
// It certifies the shipped path: the request is built by orgscan's own
// builder and the reply is read by orgscan's own grounding. A case that
// rebuilt either would measure a copy, and a copy stays green through the
// change that breaks the original.
//
// The fixture names its exchanges by LABEL. Prepare mints the ids, so an id
// in the reply is an id the model was handed rather than one the corpus
// author could have written into the expected answer — and the quote check
// runs against the fixture's own message text, exactly as production runs it
// against the account's.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/orgbrief"
	"github.com/margince/margince/backend/internal/compose/orgscan"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// orgScanFixture is one account as the scan reads it.
type orgScanFixture struct {
	Name            string                  `json:"name"`
	Industry        string                  `json:"industry,omitempty"`
	SectionsOmitted []string                `json:"sections_omitted,omitempty"`
	Contacts        []orgScanNamedFixture   `json:"contacts,omitempty"`
	Deals           []orgBriefDealFixture   `json:"open_deals,omitempty"`
	Tasks           []orgScanNamedFixture   `json:"open_tasks,omitempty"`
	Messages        []orgScanMessageFixture `json:"messages"`
}

type orgScanNamedFixture struct {
	Label string `json:"label"`
	Name  string `json:"name"`
}

type orgScanMessageFixture struct {
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Subject   string `json:"subject,omitempty"`
	At        string `json:"at"`
	Text      string `json:"text"`
}

type orgScanCases struct{}

func (orgScanCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskAccountScan, Variant: "org_scan", Kind: ai.SiteKindOneShot}
}

// Prepare turns one account and the exchanges a correct scan must rest on
// into a runnable case, minting an id per labelled record.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (orgScanCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f orgScanFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("account_scan/org_scan: the fixture is not the shape this site takes: %w", err)
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"account_scan/org_scan: the expected answer is not a list of exchange labels the findings must cite: %w", err)
	}
	in, label, err := orgScanInput(f)
	if err != nil {
		return nil, fmt.Errorf("account_scan/org_scan: %w", err)
	}
	if err := refuseUngroundableBrief(want, label); err != nil {
		return nil, fmt.Errorf("account_scan/org_scan: %w", err)
	}
	return &orgScanCase{in: in, orgID: ids.New[ids.OrganizationKind](), label: label, expected: want}, nil
}

// orgScanInput builds the production input, minting one id per labelled
// record so no id in the reply can have come from the corpus.
func orgScanInput(f orgScanFixture) (orgscan.Input, map[string]string, error) {
	in := orgscan.Input{Account: orgbrief.Input{
		Name: f.Name, Industry: f.Industry, SectionsOmitted: f.SectionsOmitted,
	}}
	label := map[string]string{}
	for _, contact := range f.Contacts {
		if err := refuseUnnameable(contact.Label, "contact", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[contact.Label] = id
		in.Account.Contacts = append(in.Account.Contacts, orgbrief.NamedIn{ID: id, Name: contact.Name})
	}
	for _, deal := range f.Deals {
		if err := refuseUnnameable(deal.Label, "deal", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[deal.Label] = id
		in.Account.OpenDeals = append(in.Account.OpenDeals, orgbrief.DealIn{
			ID: id, Name: deal.Name, Stage: deal.Stage,
			AmountMinor: deal.AmountMinor, Currency: deal.Currency, Stalled: deal.Stalled,
		})
	}
	for _, task := range f.Tasks {
		if err := refuseUnnameable(task.Label, "task", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[task.Label] = id
		in.Account.OpenTasks = append(in.Account.OpenTasks, orgbrief.TaskIn{ID: id, Name: task.Name})
	}
	for _, message := range f.Messages {
		if err := refuseUnnameable(message.Label, "message", label); err != nil {
			return in, nil, err
		}
		at, err := time.Parse(time.RFC3339, message.At)
		if err != nil {
			return in, nil, fmt.Errorf("message %q is dated %q, which is not an RFC 3339 instant", message.Label, message.At)
		}
		id := ids.NewV7()
		label[message.Label] = id.String()
		in.Messages = append(in.Messages, orgscan.MessageIn{
			ID: id, Kind: message.Kind, Direction: message.Direction, Subject: message.Subject,
			At: at.UTC(), Text: message.Text,
		})
	}
	if len(in.Messages) == 0 {
		return in, nil, errors.New("the fixture carries no exchange, and the scan asks nothing of an account with none")
	}
	return in, label, nil
}

// orgScanCase certifies one reading of one account.
type orgScanCase struct {
	in       orgscan.Input
	orgID    ids.OrganizationID
	label    map[string]string
	expected []string
}

// Run issues the one request this site sends, through the production
// builder — the reply schema included, with this fixture's ids as the
// citation enum.
func (c *orgScanCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := orgscan.ScanRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("account_scan/org_scan: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate runs the production grounding and asks whether the surviving
// findings rest on the exchanges the scenario says a correct scan is about.
//
// A reply the parser refuses whole is invalid. One whose every finding was
// refused — a fabricated citation, a quote not in its message — is invalid
// too: the model produced something the gate could not let through. One
// that raised nothing is an abstention, which is a right answer for an
// account that needs nobody.
func (c *orgScanCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	kept, refused, err := orgscan.ParseFindings(trace.Output, c.orgID, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if len(kept) == 0 && len(refused) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: strings.Join(refused, "; ")}
	}
	if len(kept) == 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeAbstained, Detail: "no finding rested on an exchange of this account"}
	}
	cited := map[string]bool{}
	for _, finding := range kept {
		for _, evidence := range finding.Evidence {
			cited[evidence.EntityId.String()] = true
		}
	}
	var missing []string
	for _, name := range c.expected {
		if !cited[c.label[name]] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: "never cited: " + strings.Join(missing, ", ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}
