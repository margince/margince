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

	// ClientMetadataProduct identifies the fetch of a client's own metadata
	// document during OAuth consent. That request goes to a URL the CALLER
	// supplied, which makes it the one outbound call where the operator on the
	// other end most needs to know who is asking.
	ClientMetadataProduct = "margince-oauth"
	ClientMetadataHeader  = ClientMetadataProduct + "/" + version
)
