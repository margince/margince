// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gdocs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// docsBase is the Google Docs API; NewDocsAPI takes another for tests.
const docsBase = "https://docs.googleapis.com"

// ErrScopeRefused is Google refusing the read grant itself. Token refresh does
// not report scopes, so a read is the first place a narrowed grant shows.
var ErrScopeRefused = errors.New("gdocs: Google refused the documents.readonly grant")

// ErrDocumentUnreadable is one Doc this account may not open, or one that is
// gone. It says nothing about the connection, so the other Docs are still read.
var ErrDocumentUnreadable = errors.New("gdocs: this account cannot open the document")

// scopeRefusedReasons are the codes Google sends when the token lacks the
// scope, as against a Doc the account has no access to.
var scopeRefusedReasons = map[string]bool{
	"ACCESS_TOKEN_SCOPE_INSUFFICIENT": true,
	"insufficientPermissions":         true,
}

// Document is one Doc as read: its title, the revision read, and its text.
type Document struct {
	Title    string
	Revision string
	Text     string
}

// DocsAPI opens one Google Doc by its id.
type DocsAPI interface {
	Document(ctx context.Context, accessToken, fileID string) (Document, error)
}

type docsClient struct {
	client *http.Client
	base   string
}

// NewDocsAPI reads Docs over client and base, Google's when empty.
//
//nolint:ireturn // returns the DocsAPI seam by design, so tests substitute a stub
func NewDocsAPI(client *http.Client, base string) DocsAPI {
	if client == nil {
		client = googleconn.BoundedClient()
	}
	if base == "" {
		base = docsBase
	}
	return docsClient{client: client, base: base}
}

// Document reads every tab, because Google writes the transcript into a tab of
// the notes Doc as well as into a Doc of its own.
func (d docsClient) Document(ctx context.Context, accessToken, fileID string) (Document, error) {
	var raw rawDocument
	q := url.Values{"includeTabsContent": {"true"}}
	status, err := googleconn.Get(ctx, d.client, d.base, accessToken, "/v1/documents/"+url.PathEscape(fileID), q, &raw)
	if err != nil {
		return Document{}, classifyRead(status, err)
	}
	return Document{Title: strings.TrimSpace(raw.Title), Revision: raw.RevisionID, Text: raw.text()}, nil
}

// classifyRead separates the three refusals a read can meet. A throttle and
// every other failure keep googleconn's class, so the registry backs off or
// parks the connection as for any Google sync.
func classifyRead(status int, err error) error {
	if _, throttled := errors.AsType[*connector.RateLimitedError](err); throttled {
		return err
	}
	reason := connector.ProviderReason(err)
	switch {
	case status == http.StatusForbidden && scopeRefusedReasons[reason]:
		return fmt.Errorf("%w (%s)", ErrScopeRefused, reason)
	case status == http.StatusForbidden && googleconn.Misconfigured(err):
		return err
	case status == http.StatusForbidden || status == http.StatusNotFound:
		return fmt.Errorf("%w (status %d)", ErrDocumentUnreadable, status)
	default:
		return err
	}
}

// The part of documents.get the text is read from.
type rawDocument struct {
	Title      string   `json:"title"`
	RevisionID string   `json:"revisionId"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Body       rawBody  `json:"body"`
	Tabs       []rawTab `json:"tabs"`
}

type rawTab struct {
	DocumentTab struct {
		Body rawBody `json:"body"`
	} `json:"documentTab"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	ChildTabs []rawTab `json:"childTabs"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

type rawBody struct {
	Content []structuralElement `json:"content"`
}

type structuralElement struct {
	Paragraph       *paragraph `json:"paragraph"`
	Table           *table     `json:"table"`
	TableOfContents *rawBody   `json:"tableOfContents"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

type paragraph struct {
	Elements []paragraphElement `json:"elements"`
}

type paragraphElement struct {
	TextRun *struct {
		Content string `json:"content"`
	} `json:"textRun"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Person *struct {
		Properties struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"personProperties"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	} `json:"person"`
	RichLink *struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"richLinkProperties"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	} `json:"richLink"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

type table struct {
	Rows []struct {
		Cells []struct {
			Content []structuralElement `json:"content"`
		} `json:"tableCells"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	} `json:"tableRows"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

// text is the Doc's words in reading order. A Doc read with its tabs has an
// empty body, and one read without them has no tabs, so whichever is filled
// is the text.
func (d rawDocument) text() string {
	var b strings.Builder
	if len(d.Tabs) == 0 {
		writeElements(&b, d.Body.Content)
	}
	writeTabs(&b, d.Tabs)
	return b.String()
}

func writeTabs(b *strings.Builder, tabs []rawTab) {
	for _, tab := range tabs {
		writeElements(b, tab.DocumentTab.Body.Content)
		writeTabs(b, tab.ChildTabs)
	}
}

func writeElements(b *strings.Builder, elements []structuralElement) {
	for _, el := range elements {
		switch {
		case el.Paragraph != nil:
			writeParagraph(b, *el.Paragraph)
		case el.Table != nil:
			writeTable(b, *el.Table)
		case el.TableOfContents != nil:
			writeElements(b, el.TableOfContents.Content)
		}
	}
}

// writeParagraph writes a paragraph's runs. Google ends each paragraph's last
// run with its own newline, so no separator is added.
func writeParagraph(b *strings.Builder, p paragraph) {
	for _, el := range p.Elements {
		switch {
		case el.TextRun != nil:
			b.WriteString(el.TextRun.Content)
		case el.Person != nil:
			b.WriteString(personName(el.Person.Properties.Name, el.Person.Properties.Email))
		case el.RichLink != nil:
			b.WriteString(el.RichLink.Properties.Title)
		}
	}
}

func personName(name, email string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return email
}

// writeTable writes one line per row with the cells side by side, so a row of
// an action-item table stays one line the reading can cite.
func writeTable(b *strings.Builder, t table) {
	for _, row := range t.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, cell := range row.Cells {
			var inner strings.Builder
			writeElements(&inner, cell.Content)
			cells = append(cells, strings.Join(strings.Fields(inner.String()), " "))
		}
		b.WriteString(strings.Join(cells, " | "))
		b.WriteString("\n")
	}
}
