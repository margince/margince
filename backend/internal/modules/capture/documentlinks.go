// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which links in a mail body count as documents sent: a share link to a Google
// Doc, Sheet, Slides deck, Form or Drive file. This is a capture rule about what
// mail bodies contain; the row it becomes belongs to the activities module.

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// documentHosts are the hosts whose links are documents. A host is added here
// when it is seen in real mail and the team names it.
var documentHosts = map[string]struct{}{
	"docs.google.com":  {},
	"drive.google.com": {},
}

// maxDocumentLinksPerMessage bounds the link rows one message writes.
const maxDocumentLinksPerMessage = 25

const (
	actionLinksDropped = "capture_links_dropped"
	reasonLinksOverCap = "document_links_over_cap"
	fieldDroppedLinks  = "dropped_links"
)

// maxDocumentLinkLength mirrors the length bound of the attachment_link_shape
// CHECK; a longer address is skipped so it cannot abort the capture transaction.
const maxDocumentLinkLength = 2048

func defaultPort(scheme string) string {
	if scheme == "http" {
		return "80"
	}
	return "443"
}

// documentLinkKey is the canonical address of a document link: the lowercased
// host and the URL without its fragment. It answers false for any other address.
// The host is matched by equality on the parsed hostname, never by a substring.
// An address with user info is refused, because its visible host can differ from
// where it leads.
func documentLinkKey(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User != nil {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if _, known := documentHosts[host]; !known {
		return "", false
	}
	parsed.Host = host
	if port := parsed.Port(); port != "" && port != defaultPort(scheme) {
		parsed.Host = host + ":" + port
	}
	parsed.Scheme = scheme
	parsed.Fragment = ""
	if canonical := parsed.String(); len(canonical) <= maxDocumentLinkLength {
		return canonical, true
	}
	return "", false
}

// documentLinksOf filters a mail's links to document hosts, drops repeats of one
// canonical address, and stops at maxDocumentLinksPerMessage. over counts the
// links left out by the cap.
func documentLinksOf(envelope *MailEnvelope) (links []MailLink, over int) {
	if envelope == nil {
		return nil, 0
	}
	seen := map[string]bool{}
	for _, link := range envelope.Links {
		key, ok := documentLinkKey(link.URL)
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		if len(links) >= maxDocumentLinksPerMessage {
			over++
			continue
		}
		links = append(links, MailLink{URL: key, Text: link.Text})
	}
	return links, over
}

// recordLinks writes a row for each document link of a new mail, in the capture
// transaction. A private message records them too: the row hangs off the mail,
// so the mail's audience decides who sees it. No keeper wired means no rows,
// the same as files.
func (s *Sink) recordLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields,
) error {
	if s.files == nil {
		return nil
	}
	over, err := RecordDocumentLinksTx(ctx, tx, s.files, activityID, fileSourceFor(rec, fields), fields.Envelope)
	if err != nil {
		return err
	}
	if over > 0 {
		return s.logBreadcrumbTx(ctx, tx, actionLinksDropped, rec, reasonLinksOverCap,
			map[string]any{fieldDroppedLinks: over})
	}
	return nil
}

// RecordDocumentLinksTx writes the document links of one mail through keeper
// and returns how many the per-message cap left out. Live capture and the
// repair of stored mail both call it to decide which links count.
func RecordDocumentLinksTx(
	ctx context.Context, tx pgx.Tx, keeper FileKeeper, activityID ids.ActivityID,
	from FileSource, envelope *MailEnvelope,
) (over int, err error) {
	links, over := documentLinksOf(envelope)
	if len(links) == 0 {
		return over, nil
	}
	if err := keeper.RecordLinks(ctx, tx, activityID, from, links); err != nil {
		return over, fmt.Errorf("capture: recording the document links a message carried: %w", err)
	}
	return over, nil
}
