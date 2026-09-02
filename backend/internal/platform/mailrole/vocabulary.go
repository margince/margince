// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailrole

// roleTokens are the words that name a FUNCTION rather than a person. A local
// part containing one of them, as a whole word, is a mailbox an organization
// answers.
//
// German and English both appear because this product's installations
// correspond in both, and `buchhaltung@` is exactly as much a department as
// `accounting@`. A word goes in only when it names a function in EVERY use: a
// person may be called Bill, so `bill` stays out while `billing` goes in.
//
// Machine local parts — `noreply@`, `mailerdaemon@`, `notification@` — are NOT
// here. capture/transactional.go's machineMarkers owns that vocabulary and
// recordWorthy asks it before this list, so repeating them would be the second
// spelling this package exists to prevent. The two lists answer different
// questions: that one asks whether a reply reaches anybody, this one asks
// whether a person is named.
//
// Words that name a function AND are ordinary business vocabulary stay OUT,
// however role-like they look: `team`, `group`, `partner`, `business`, `media`,
// `general`, `personal`, `security`, `legal`, `finance`, `payments`, `orders`,
// `service`, `contact`, `help`, `news`, `press`, `admin`, `alerts`. Each is a
// real word other lists in this tree legitimately hold — an RBAC object, a
// refusal phrase, an org-name stopword — and matching them here would refuse
// contacts on a word a person's address may honestly contain. The AI verdict
// owns the addresses this list cannot reach; a deterministic list that guessed
// would be worse than one that abstains.
//
// This list is the single spelling of the role vocabulary. A second copy
// anywhere in the tree is two answers to one question, which is how one door
// named a contact "Billing" while another refused to.
//
// Held by: TestOnlyOnePackageDeclaresRoleMailboxes (backend/gates/rolemailboxonelist_test.go)
var roleTokens = map[string]struct{}{
	// Reaching the organization. `mail` and `post` are here because a mailbox
	// called after the medium names nobody — `mail@petereich.com` was the case
	// that first put a contact called "Mail" in this tree.
	"info": {}, "kontakt": {}, "hello": {}, "hallo": {}, "enquiries": {},
	"enquiry": {}, "inquiries": {}, "office": {}, "welcome": {}, "postmaster": {},
	"mail": {}, "post": {},

	// Answering a customer
	"support": {}, "helpdesk": {}, "servicedesk": {}, "customercare": {},
	"hotline": {}, "kundenservice": {},

	// Money
	"billing": {}, "invoice": {}, "invoices": {}, "invoicing": {},
	"accounting": {}, "buchhaltung": {}, "rechnung": {}, "rechnungen": {},
	"accountsreceivable": {}, "accountspayable": {}, "dunning": {}, "mahnung": {},

	// Selling and marketing
	"sales": {}, "vertrieb": {}, "marketing": {},

	// Publishing at people
	"newsletter": {}, "presse": {}, "events": {}, "veranstaltung": {},

	// People and hiring. `jobs`, `careers`, `karriere` and `bewerbung` are
	// deliberately absent: platform/techprofile already reads those four words
	// as WEBSITE PATH labels, and one vocabulary answering two questions is how
	// a list starts disagreeing with itself. A hiring mailbox reaches this list
	// through `recruiting@`, which names no page.
	"recruiting": {}, "recruitment": {}, "hr": {},

	// Running the thing
	"webmaster": {}, "hostmaster": {}, "abuse": {}, "datenschutz": {},
	"compliance": {}, "dpo": {},

	// Operations
	"bestellung": {}, "reservations": {}, "bookings": {}, "versand": {},
}

// roleQualifiers modify a department without naming a person: a region, a size,
// a word like "team". They matter only for DisplayName, where "APAC Billing"
// and "Support Team" must read as departments rather than as people.
//
// They are deliberately NOT matched in a local part on their own: `apac@` says
// nothing about whether a person answers it, and refusing it would lose a real
// mailbox on no evidence.
var roleQualifiers = map[string]struct{}{
	"apac": {}, "emea": {}, "amer": {}, "americas": {}, "asia": {},
	"europe": {}, "global": {}, "international": {}, "regional": {},
	"team": {}, "dept": {}, "department": {}, "abteilung": {}, "group": {},
	"desk": {}, "center": {}, "centre": {}, "hub": {}, "the": {}, "und": {},
	"and": {}, "de": {}, "uk": {}, "us": {}, "eu": {},
}

// helpdeskVendors host their customers' support queues. Mail from one names the
// vendor's routing address, never a person: a ticket reply arrives from
// `support+<ticket-id>@<tenant>.zendesk.com` however the human behind it signed
// it.
//
// Matched on the registrable domain and its subdomains, so a lookalike
// registration cannot borrow the rule.
var helpdeskVendors = map[string]struct{}{
	"zendesk.com":       {},
	"freshdesk.com":     {},
	"freshservice.com":  {},
	"helpscout.net":     {},
	"helpscoutapp.com":  {},
	"intercom-mail.com": {},
	"intercom.io":       {},
	"zohodesk.com":      {},
	"jitbit.com":        {},
	"kayako.com":        {},
	"groovehq.com":      {},
	"frontapp.com":      {},
	"helpshift.com":     {},
	"gorgias.help":      {},
	"reamaze.com":       {},
}
