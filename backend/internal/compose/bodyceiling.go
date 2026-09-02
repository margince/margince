// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"mime"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// Which routes carry FILES, and may therefore read a body wider than the JSON
// bound every other route rides.
//
// Declared here because it is route knowledge, and routes are composed here.
// The list is short on purpose and is the whole grant: a route absent from it
// cannot obtain the wider bound by any means, including claiming to be
// multipart. That is the property being protected — several handlers in this
// tree decode `r.Body` with no bound of their own, and two of those routes are
// unauthenticated (DCR and login). Widening on the sender's Content-Type alone
// would have handed them the file bound each for the price of a header.
//
// The operator sets the NUMBERS (OPS-CFG-12, `uploads` in margince.yaml); this
// function decides which route gets which one. Nobody chooses the list at run
// time, which is what keeps the paragraph above true.
//
// Paths are as the generated router mounts them, under the /v1 prefix the
// chassis sees. TestEveryUploadRouteIsDeclared derives the expected set from
// the contract, so a new upload route cannot be added without appearing here.
func uploadCeilings(limits deployconfig.UploadLimits) map[string]int64 {
	return map[string]int64{
		"/v1/attachments":             limits.Attachment,     // uploadAttachment
		"/v1/imports/sources":         limits.CSVImport,      // uploadImportSource
		"/v1/me/linkedin-connections": limits.LinkedInImport, // importLinkedInConnections
		// A .vcf is a text file of a few hundred bytes per card, so it rides
		// the CSV import's ceiling rather than earning an operator dial of its
		// own — the two are the same kind of upload, a delimited address book.
		"/v1/people/vcard-import":              limits.CSVImport,         // importVCards
		"/v1/knowledge/corpora/{id}/documents": limits.KnowledgeDocument, // uploadCorpusDocument
		// A company mark, and the only ceiling here an operator cannot move.
		// The others bound what a workspace ACCUMULATES — attachments, imports,
		// corpora — where an installation's appetite is its own business. This
		// one bounds a single square image that is re-encoded to 256px before
		// anything is stored, so a larger upload buys the sender nothing but
		// decode work: there is no deployment for which a different number is
		// the right one, and a dial nobody can have a reason to turn is a
		// question asked of every operator for no answer.
		"/v1/company/logo": companyLogoUploadBytes, // uploadCompanyLogo
	}
}

// What a company mark may arrive as. Generous for what it holds — a 5 MB
// source is a photograph, not a logo — because the cost of refusing a person's
// own file is that they go and find image software, while the cost of
// accepting it is one decode of an image this server immediately shrinks.
const companyLogoUploadBytes = 5_000_000

// bodyCeilingFor is the chassis's BodyCeiling for this composition.
//
// THREE conditions, all required, because each closes a different door: the
// method, so a GET cannot carry a wide body; the route, so only a declared
// upload route can; and the media type, so an upload route handed JSON still
// rides the tight bound. A request failing any of them gets the JSON ceiling.
//
// The path is matched exactly as the router mounts it, with no normalization of
// our own. A trailing slash or an un-cleaned segment is a path the router does
// not serve, and guessing at how it would resolve one is how a grant ends up
// wider than the route list it was written from.
func bodyCeilingFor(ceilings map[string]int64) httpserver.BodyCeiling {
	return func(r *http.Request) int64 {
		if r.Method != http.MethodPost {
			return httperr.MaxBodyBytes
		}
		ceiling, declared := ceilings[r.URL.Path]
		if !declared {
			return httperr.MaxBodyBytes
		}
		// Compared as a parsed MEDIA TYPE, not as a prefix. A prefix match also
		// accepts `multipart/form-dataX`, which `ParseMultipartForm` then rejects —
		// so the chassis and the parser would disagree about what a multipart body
		// is, and the sender would pick which one won.
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			return httperr.MaxBodyBytes
		}
		return ceiling
	}
}
