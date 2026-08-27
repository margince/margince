// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Org display-name derivation (ADR-0072/A118, PO-F-2a). The capture
// auto-create path must not name an organization by its raw mail eSLD —
// "gitex.com" reads as a defect where "Gitex" reads as a name. This derives a
// readable, honest name from the domain's registrable label alone; it invents
// nothing beyond capitalizing the label the domain already states. A richer
// source (a dossier's site-stated name, a corroborated signature) may later
// overwrite it, gated by organization.name_source (nameSourceDomain marks a
// name still provisional).

import "github.com/margince/margince/backend/internal/platform/freemail"

// The organization.name_source provenance values (0120). Ordered weakest to
// strongest: a stronger source may overwrite a weaker one, never the reverse,
// and never 'human'.
const (
	nameSourceHuman     = "human"
	nameSourceDossier   = "dossier"
	nameSourceSignature = "signature"
	nameSourceDomain    = "domain"
)

// DisplayNameFromDomain turns a mail domain into a readable organization name.
//
// It delegates: the derivation is a public-suffix walk, and the SAME walk
// already answers "is this a mailbox host" for the same caller at the same
// moment. Keeping the two in one package is what stops a second door growing
// its own copy of each — the agent tool `qualify_lead` had done exactly that,
// with a 15-domain list beside a split on the first dot that named
// "eu.docusign.net" as "Eu".
//
// The result is stamped with name_source='domain' by the caller: provisional,
// overwritable.
func DisplayNameFromDomain(domain string) string {
	return freemail.DisplayName(domain)
}
