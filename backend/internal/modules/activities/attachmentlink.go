// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A document that lives elsewhere, listed with a record's files by a user:
// the address and what to call it, no bytes. It is the same row shape as a link
// a mail carried, so every reader that refuses withheld bytes refuses it too.

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// linkSource is the source of a link a user added, as upload is of a file.
const linkSource = "link"

// maxLinkAddressLength mirrors the length bound of the attachment_link_shape CHECK.
const maxLinkAddressLength = 2048

// maxLinkTitleRunes is the bound the attachment table holds every title to.
const maxLinkTitleRunes = 120

const fieldURL = "url"

// addableLinkCategories are what a user may say a linked document is. The
// provenance values and `link` describe how something arrived, which a user
// adding an address cannot claim.
var addableLinkCategories = map[string]struct{}{
	"contract": {}, "offer": {}, "legal": {}, "other": {},
}

// LinkInput is one link a user adds to a record.
type LinkInput struct {
	EntityType string
	EntityID   ids.UUID
	URL        string
	Title      string
	// Category is empty for none, as on an upload.
	Category   string
	ContractID *ids.UUID
}

// LinkAddress is the address a link row stores: trimmed, http or https, with a
// host and no user info, which can make the visible host differ from where the
// address leads. The preview and the write both read an address through it.
func LinkAddress(raw string) (string, error) {
	address := strings.TrimSpace(raw)
	parsed, err := url.Parse(address)
	switch {
	case address == "":
		return "", linkFault("required", "give the address of the document")
	case err != nil, parsed.Hostname() == "":
		return "", linkFault("invalid_url", "that is not a web address; paste the whole address, starting with https://")
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "", linkFault("invalid_url", "only http and https addresses can be listed")
	case parsed.User != nil:
		return "", linkFault("invalid_url", "an address with a user name in it cannot be listed; remove the part before @")
	case len(address) > maxLinkAddressLength:
		return "", &values.ParseError{Field: fieldURL, Code: "too_long", Max: maxLinkAddressLength,
			Message: fmt.Sprintf("the address is longer than %d characters", maxLinkAddressLength)}
	}
	// The CHECK on the column reads the scheme case-sensitively.
	return parsed.Scheme + address[len(parsed.Scheme):], nil
}

func linkFault(code, message string) error {
	return &values.ParseError{Field: fieldURL, Code: code, Message: message}
}

// linkTitle cleans a typed or fetched title to one line, or answers "" for none.
func linkTitle(raw string) (string, error) {
	title := strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw)), " ")
	if utf8.RuneCountInString(title) > maxLinkTitleRunes {
		return "", &values.ParseError{Field: "title", Code: "too_long", Max: maxLinkTitleRunes,
			Message: fmt.Sprintf("the title is longer than %d characters", maxLinkTitleRunes)}
	}
	return title, nil
}

func refuseUnaddableCategory(category string) error {
	if _, ok := addableLinkCategories[category]; ok || category == "" {
		return nil
	}
	return &values.ParseError{Field: fieldCategory, Code: "category_not_assertable",
		Message: "a link is filed by what it is: contract, offer, legal or other"}
}

// AddAttachmentLink lists a link on a record under the upload's authority:
// Update on the parent's type and a live parent the caller can see.
func (s *Store) AddAttachmentLink(ctx context.Context, in LinkInput) (crmcontracts.Attachment, error) {
	if err := httperr.RequireBodyID(fieldEntityID, in.EntityID); err != nil {
		return crmcontracts.Attachment{}, err
	}
	address, err := LinkAddress(in.URL)
	if err != nil {
		return crmcontracts.Attachment{}, err
	}
	title, err := linkTitle(in.Title)
	if err != nil {
		return crmcontracts.Attachment{}, err
	}
	if err := refuseUnaddableCategory(in.Category); err != nil {
		return crmcontracts.Attachment{}, err
	}
	if err := requireAttachmentWrite(ctx, in.EntityType); err != nil {
		return crmcontracts.Attachment{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Attachment{}, err
	}
	in.URL, in.Title = address, title
	var out crmcontracts.Attachment
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureAttachmentParentWritableLive(ctx, tx, in.EntityType, in.EntityID); err != nil {
			return err
		}
		if err := ensureContractFileable(ctx, tx, in.ContractID); err != nil {
			return err
		}
		if err := refuseListedLink(ctx, tx, in); err != nil {
			return err
		}
		id, err := insertAttachmentLink(ctx, tx, in, by)
		if err != nil {
			return err
		}
		if out, err = readAttachment(ctx, tx, id); err != nil {
			return err
		}
		return announceAddedLink(ctx, tx, id, in)
	})
	return out, err
}

// refuseListedLink stops the same address being listed twice on one record,
// where it would read as two documents.
func refuseListedLink(ctx context.Context, tx pgx.Tx, in LinkInput) error {
	var listed bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM attachment
		 WHERE entity_type = $1 AND entity_id = $2 AND url = $3 AND archived_at IS NULL)`,
		in.EntityType, in.EntityID, in.URL).Scan(&listed); err != nil {
		return fmt.Errorf("activities: looking for the link on the record: %w", err)
	}
	if listed {
		return linkFault("link_already_listed", "this address is already listed on the record")
	}
	return nil
}

func insertAttachmentLink(ctx context.Context, tx pgx.Tx, in LinkInput, by string) (ids.UUID, error) {
	rollUp, hasAccount, err := accountRollUp(ctx, tx, in.EntityType, in.EntityID)
	if err != nil {
		return ids.UUID{}, err
	}
	var account *ids.UUID
	if hasAccount {
		account = &rollUp
	}
	id := ids.NewV7()
	if _, err := tx.Exec(ctx, `
		INSERT INTO attachment (id, entity_type, entity_id, filename, storage_key, source,
			captured_by, company_id, contract_id, bytes_withheld, url, title, category)
		VALUES ($1, $2, $3, $4, '', $5, $6, $7, $8, true, $9, $10, $11)`,
		id, in.EntityType, in.EntityID, linkHostOf(in.URL), linkSource, by,
		account, in.ContractID, in.URL, nullIfEmpty(in.Title), storedCategory(in.Category)); err != nil {
		return ids.UUID{}, fmt.Errorf("activities: listing a link: %w", err)
	}
	return id, nil
}

// storedCategory files a link with no category as `other`, the column's
// default, as an upload is.
func storedCategory(category string) string {
	if category == "" {
		return "other"
	}
	return category
}

// announceAddedLink writes the audit and outbox rows of the new link. The
// address and title stay in the audit row: what a user typed can be the
// sensitive part, and the event travels to every subscriber.
func announceAddedLink(ctx context.Context, tx pgx.Tx, id ids.UUID, in LinkInput) error {
	auditID, err := storekit.Audit(ctx, tx, "create", "attachment", id, nil, map[string]any{
		fieldEntityType: in.EntityType,
		fieldEntityID:   in.EntityID.String(),
		fieldURL:        in.URL,
		"title":         in.Title,
		fieldCategory:   in.Category,
	})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id, crmcontracts.PublicEventAttachmentCreated{
		ParentType: in.EntityType,
		ParentId:   openapi_types.UUID(in.EntityID),
	})
}
