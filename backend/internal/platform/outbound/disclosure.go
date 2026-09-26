// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package outbound

// What each outbound identity sends, and to whom.
//
// docs/reference/ai-egress.md is cited by the works-agreement template and by
// the DPIA as the complete answer to "what leaves this machine". It was
// generated from the AI routing table alone, so it answered only for model and
// embedding providers — and a document called complete that is not is worse
// than none, because the operator who writes their processing record from it
// will not find out from anything in here.
//
// Declared BESIDE the identity constants rather than in a page or a list of its
// own. An identity exists because a call goes out under it, so the two facts
// have one home: adding a token without saying what it discloses fails
// outboundidentityusers_test.go's sibling check, and a page kept anywhere else
// is a second list that goes stale the first time somebody adds a caller.
//
// The DATA CATEGORY is a judgement and cannot be derived — a URL is a string to
// the compiler whether it names a company's public homepage or a contact's. So it
// is written here, where the reviewer adding the caller is looking, rather than
// inferred somewhere the reviewer never reads.

// Disclosure is one outbound identity's answer: where it calls, what it can
// carry, and what an operator does to stop it.
type Disclosure struct {
	// Product is the identity constant this describes, which is the join to
	// the header the call actually advertises.
	Product string
	// Endpoint is the host or service reached, as an operator would recognise
	// it. Not a URL: several of these are configurable, and the point is which
	// party receives the data rather than which path.
	Endpoint string
	// Personal says whether the request can carry personal data. False is a
	// claim, not an absence — it is what lets the generated page separate the
	// disclosures a processing record has to name from the rest.
	Personal bool
	// Category names what the receiving party can see, in the words a reader of
	// a works agreement would use. Empty only where Personal is false.
	Category string
	// Control is what an administrator sets to prevent the call, or "" where
	// the call is not optional.
	Control string
}

// Disclosures is every outbound identity and what it sends.
//
// One entry per Product constant above, and the page is generated from it: an
// identity with no entry is a call this installation makes and cannot answer
// for.
//
// Held by: TestEveryOutboundIdentitySaysWhatItDiscloses
// (backend/gates/outbounddisclosures_test.go)
func Disclosures() []Disclosure {
	return []Disclosure{
		{Product: CalendarProduct, Endpoint: "Google Calendar or Microsoft Graph", Personal: true, Category: "attendee addresses, meeting title, description, time, location and a guest management link; calendar queries identify the host and requested time window", Control: "the host connects a calendar and authorizes scheduling; disconnecting its credential prevents further calls"},
		{
			Product: SearchProduct, Endpoint: "Brave Search API", Personal: true,
			Category: "a contact's name and employer, as a search query",
			Control:  "MARGINCE_BRAVE_API_KEY unset (the default) leaves web search disabled",
		},
		{
			Product: EnrichProduct, Endpoint: "the contact-enrichment provider bound for this installation", Personal: true,
			Category: "a contact's name, employer and public profile identifiers",
			Control:  "the enrichment provider's own credential, unset by default",
		},
		{
			Product: GeocodeProduct, Endpoint: "Nominatim / OpenStreetMap", Personal: true,
			Category: "a postal address, which on a contact record is somebody's",
			Control:  "the geocoding provider setting",
		},
		{
			Product: SiteReadProduct, Endpoint: "any site a captured or entered URL names", Personal: true,
			Category: "the URL itself, which can name a contact's own page",
			Control:  "the site-read rollout setting",
		},
		{
			Product: CertLogProduct, Endpoint: "certificate-transparency logs (crt.sh)", Personal: false,
			Control: "the domain-discovery setting",
		},
		{
			Product: ModelCatalogueProduct, Endpoint: "a model vendor's public catalogue", Personal: false,
			Control: "asked once per installation during setup",
		},
		{
			Product: WebhooksProduct, Endpoint: "the customer's own webhook endpoint", Personal: true,
			Category: "whatever the subscribed event carries, which is the customer's own record data",
			Control:  "the subscription itself; deleting it stops the delivery",
		},
		{
			Product: MailProduct, Endpoint: "the configured SMTP relay", Personal: true,
			Category: "the message and its recipients",
			Control:  "the outbound mail channel's own configuration",
		},
		{
			Product: MailboxProduct, Endpoint: "the mailbox provider (IMAP)", Personal: true,
			Category: "the mailbox credential and the folders it reads",
			Control:  "the mailbox connection; removing it stops the sync",
		},
		{
			Product: SignInProduct, Endpoint: "Google's OAuth token endpoint", Personal: true,
			Category: "the signing-in user's authorization code",
			Control:  "the Google sign-in method",
		},
		{
			Product: ClientMetadataProduct, Endpoint: "an OAuth client's published metadata document", Personal: false,
			Control: "the OAuth client registration that names it",
		},
		{
			Product: KeySetProduct, Endpoint: "the identity provider's published key set", Personal: false,
			Control: "the identity provider binding",
		},
		{
			Product: SelfCheckProduct, Endpoint: "this installation's own public address", Personal: false,
			Control: "not optional: it is how the installation learns whether it is reachable",
		},
	}
}
