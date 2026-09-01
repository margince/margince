// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package outbound holds how this product identifies itself to a server it
// calls.
package outbound

// version is one number for one product. A remote operator reading two versions
// in their log would reasonably conclude they were seeing two pieces of
// software, and the component already tells them which part of this one called.
const version = "1.0"

// Every identity is a CONSTANT, and its header is a constant expression built
// from the same token. A variable — even an unexported one behind an accessor —
// could be rewritten by anything in the process, putting the advertised name
// and the policy matched against it back out of step. That is the defect this
// package exists to remove, and a const cannot reach it.
const (
	// SiteReadProduct is the crawler's token. It appears in site operators'
	// robots.txt files, so changing it silently stops their rules applying to
	// us — a Disallow written for the name we advertise is no longer matched.
	SiteReadProduct = "margince-siteread"
	SiteReadHeader  = SiteReadProduct + "/" + version

	// GeocodeProduct identifies address lookups. The upstream policy requires
	// an identifiable agent; an anonymous or spoofed one is how an installation
	// gets the whole service blocked.
	GeocodeProduct = "margince-geocode"
	GeocodeHeader  = GeocodeProduct + "/" + version

	// CertLogProduct identifies certificate-transparency queries. CT logs are
	// shared public infrastructure run on goodwill, so a recurring reader
	// identifies itself for the same reason the geocoder does: an operator who
	// can name the client can ask it to slow down instead of blocking it.
	//
	// It is a SEPARATE token from the site reader even though both read the
	// public web, because the two obey different rules. The site reader's name
	// is matched against robots.txt groups a site wrote; a CT log has no
	// robots.txt and no per-site policy, and folding the two would put the
	// crawler's advertised identity on a request no site ever ruled on.
	CertLogProduct = "margince-certlog"
	CertLogHeader  = CertLogProduct + "/" + version

	// WebhooksProduct identifies deliveries this product makes to a customer's
	// endpoint, so their logs can attribute a call they did not expect.
	WebhooksProduct = "margince-webhooks"
	WebhooksHeader  = WebhooksProduct + "/" + version

	// SelfCheckProduct identifies this installation asking whether its own
	// configured public address answers. The request goes to the operator's
	// own ingress, and it is the one call where a name in their log saves
	// them from investigating an unexplained hit on their health endpoint
	// every minute.
	SelfCheckProduct = "margince-selfcheck"
	SelfCheckHeader  = SelfCheckProduct + "/" + version

	// ClientMetadataProduct identifies the fetch of a client's own metadata
	// document during OAuth consent. That request goes to a URL the CALLER
	// supplied, which makes it the one outbound call where the operator on the
	// other end most needs to know who is asking.
	ClientMetadataProduct = "margince-oauth"
	ClientMetadataHeader  = ClientMetadataProduct + "/" + version

	// KeySetProduct identifies the fetch of an identity provider's public key
	// set. The request carries no credential — it is a read of a document
	// published for anyone — so the agent is the ONLY thing the provider's
	// operator has to attribute it by, and a key set is read on a schedule
	// rather than once.
	//
	// Named for the DOCUMENT rather than for what it is used for. Spelled
	// around the word "token" it reads as a secret to a scanner, and a
	// suppression on a public constant is a worse answer than a better name.
	KeySetProduct = "margince-keyset"
	KeySetHeader  = KeySetProduct + "/" + version

	// SearchProduct identifies web-search queries. The key names the ACCOUNT,
	// which is one customer's contract with the provider; the agent names the
	// software making the calls under it. An operator diagnosing a spike can
	// act on the second without cancelling the first.
	SearchProduct = "margince-search"
	SearchHeader  = SearchProduct + "/" + version

	// EnrichProduct identifies contact-enrichment lookups, for the same reason
	// as the search token and separately from it: the two run at different
	// rates against different vendors, and one being throttled must not be the
	// other's problem.
	EnrichProduct = "margince-enrich"
	EnrichHeader  = EnrichProduct + "/" + version

	// MirrorProduct identifies reads and writes against a customer's own CRM
	// while this product mirrors it. Their administrator sees the traffic in an
	// audit log beside their people's own, and "some Go program" is not an
	// answer to "what is writing to our records".
	MirrorProduct = "margince-mirror"
	MirrorHeader  = MirrorProduct + "/" + version

	// MailProduct is what an SMTP session announces when the sender's address
	// carries no domain to announce instead. A relay operator reads the EHLO
	// name to attribute mail; the library's default is `localhost`, which is
	// not merely anonymous but a claim, and a false one.
	MailProduct = "margince-mail"

	// MailboxProduct identifies this product in an IMAP ID exchange (RFC 2971).
	// A mailbox provider throttles per client, and a client they cannot name is
	// one they can only throttle by guessing.
	MailboxProduct = "margince-mailbox"
	MailboxVersion = version

	// SignInProduct identifies the Google sign-in authorization-code exchange —
	// a SEPARATE token from ClientMetadataProduct's OAuth-consent fetch, even
	// though both are OAuth calls to Google: this one carries the shared Gmail
	// app's client secret, so it is a credentialed call an operator diagnosing
	// abuse must be able to tell apart from an anonymous metadata read.
	SignInProduct = "margince-signin"
	SignInHeader  = SignInProduct + "/" + version
)
