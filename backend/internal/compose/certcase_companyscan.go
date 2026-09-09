// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for the account scan (account_scan/company_scan).
//
// It certifies the shipped path: the request is built by companyscan's own
// builder and the reply is read by companyscan's own grounding. A case that
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
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// companyScanFixture is one account as the scan reads it.
type companyScanFixture struct {
	Name            string                  `json:"name"`
	Industry        string                  `json:"industry,omitempty"`
	SectionsOmitted []string                `json:"sections_omitted,omitempty"`
	Contacts        []companyScanNamedFixture   `json:"contacts,omitempty"`
	Deals           []companyBriefDealFixture   `json:"open_deals,omitempty"`
	Tasks           []companyScanNamedFixture   `json:"open_tasks,omitempty"`
	Messages        []companyScanMessageFixture `json:"messages"`
}

type companyScanNamedFixture struct {
	Label string `json:"label"`
	Name  string `json:"name"`
}

type companyScanMessageFixture struct {
	Label     string `json:"label"`
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	Subject   string `json:"subject,omitempty"`
	At        string `json:"at"`
	Text      string `json:"text"`
}

type companyScanCases struct{}

func (companyScanCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskAccountScan, Variant: "company_scan", Kind: ai.SiteKindOneShot}
}

// Prepare turns one account and the exchanges a correct scan must rest on
// into a runnable case, minting an id per labelled record.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (companyScanCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f companyScanFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("account_scan/company_scan: the fixture is not the shape this site takes: %w", err)
	}
	var want []string
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf(
			"account_scan/company_scan: the expected answer is not a list of exchange labels the findings must cite: %w", err)
	}
	in, label, err := companyScanInput(f)
	if err != nil {
		return nil, fmt.Errorf("account_scan/company_scan: %w", err)
	}
	// Only an exchange can be what a finding rests on, so only an exchange's
	// label is an expectation: a contact's name here would be one no reply
	// could ever cite.
	exchanges := map[string]string{}
	for _, message := range f.Messages {
		exchanges[message.Label] = label[message.Label]
	}
	if err := refuseUngroundableBrief(want, exchanges); err != nil {
		return nil, fmt.Errorf("account_scan/company_scan: %w", err)
	}
	return &companyScanCase{in: in, companyID: ids.New[ids.CompanyKind](), label: label, expected: want}, nil
}

// companyScanInput builds the production input, minting one id per labelled
// record so no id in the reply can have come from the corpus.
func companyScanInput(f companyScanFixture) (companyscan.Input, map[string]string, error) {
	in := companyscan.Input{Account: companybrief.Input{
		Name: f.Name, Industry: f.Industry, SectionsOmitted: f.SectionsOmitted,
	}}
	label := map[string]string{}
	for _, contact := range f.Contacts {
		if err := refuseUnnameable(contact.Label, "contact", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[contact.Label] = id
		in.Account.Contacts = append(in.Account.Contacts, companybrief.NamedIn{ID: id, Name: contact.Name})
	}
	for _, deal := range f.Deals {
		if err := refuseUnnameable(deal.Label, "deal", label); err != nil {
			return in, nil, err
		}
		id := ids.NewV7().String()
		label[deal.Label] = id
		in.Account.OpenDeals = append(in.Account.OpenDeals, companybrief.DealIn{
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
		in.Account.OpenTasks = append(in.Account.OpenTasks, companybrief.TaskIn{ID: id, Name: task.Name})
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
		in.Messages = append(in.Messages, companyscan.MessageIn{
			ID: id, Kind: message.Kind, Direction: message.Direction, Subject: message.Subject,
			At: at.UTC(), Text: message.Text,
		})
	}
	if len(in.Messages) == 0 {
		return in, nil, errors.New("the fixture carries no exchange, and the scan asks nothing of an account with none")
	}
	return in, label, nil
}

// companyScanCase certifies one reading of one account.
type companyScanCase struct {
	in       companyscan.Input
	companyID    ids.CompanyID
	label    map[string]string
	expected []string
}

// Run issues the one request this site sends, through the production
// builder — the reply schema included, with this fixture's ids as the
// citation enum.
func (c *companyScanCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := companyscan.ScanRequest(c.in, string(textlang.English))
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("account_scan/company_scan: %w", err)
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
func (c *companyScanCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	kept, refused, err := companyscan.ParseFindings(trace.Output, c.companyID, c.in)
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
