// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import "github.com/margince/margince/backend/internal/shared/kernel/promptref"

// scanRefs names every record the scan payload shows. The messages a finding
// may cite are m1, m2, … in the order they are listed. The account's own
// records take a prefix per kind. Any other record id in the payload is r1,
// r2, … so none reaches the model. ScanRequest and ParseFindings both build it
// from the same input.
type scanRefs struct {
	messages *promptref.Refs
	tables   []*promptref.Refs
}

func refsOf(in Input) scanRefs {
	messages := promptref.New("m")
	for _, message := range in.Messages {
		messages.For(message.ID.String())
	}
	account := in.Account
	company, contacts, deals, tasks := promptref.New("c"), promptref.New("p"), promptref.New("d"), promptref.New("t")
	if account.ID != "" {
		company.For(account.ID)
	}
	for _, contact := range account.Contacts {
		contacts.For(contact.ID)
	}
	for _, deal := range account.OpenDeals {
		deals.For(deal.ID)
	}
	for _, task := range account.OpenTasks {
		tasks.For(task.ID)
	}
	activities, promises := promptref.New("a"), promptref.New("k")
	for _, commitment := range account.OpenCommitments {
		promises.For(commitment.ID)
		activityRef(messages, activities, commitment.ActivityID)
	}
	for _, act := range account.Recent {
		activityRef(messages, activities, act.ID)
	}
	if account.LastContact != nil {
		activityRef(messages, activities, account.LastContact.ID)
	}
	refs := scanRefs{messages: messages, tables: []*promptref.Refs{messages, company, contacts, deals, tasks, activities, promises}}
	others := promptref.Mint(promptref.Rewrite(encodeInput(in), refs.tables...), nil, map[string]string{"": "r"})
	if table, ok := others[""]; ok {
		refs.tables = append(refs.tables, table)
	}
	return refs
}

// activityRef names an activity by its message ref when the scan shows it as
// a message, so one record never carries two refs.
func activityRef(messages, activities *promptref.Refs, id string) {
	if _, isMessage := messages.RefOf(id); !isMessage && id != "" {
		activities.For(id)
	}
}

// payload is the input as the model reads it, every record named by its ref.
func (r scanRefs) payload(in Input) string {
	return promptref.Rewrite(encodeInput(in), r.tables...)
}

// namedIn reports whether text spells any ref the payload shows.
func (r scanRefs) namedIn(text string) bool {
	for _, table := range r.tables {
		if _, named := table.NamedIn(text); named {
			return true
		}
	}
	return false
}
