// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Wire codes a screen translates. The refusals are 422 field errors; the
// question is a 409 the same request answers once it carries `confirm: true`.
const (
	CodeOwnDomain          = "own_domain"
	CodeConsumerMailDomain = "consumer_mail_domain"
	CodeAlreadyShipped     = "already_on_shipped_list"
	CodeNeedsConfirmation  = "domain_needs_confirmation"
	detailKeyDomain        = "domain"
	detailKeyRole          = "role"
	detailKeyCompany       = "company_name"
)

// domainQuestion is the 409 that asks whether a value that is already something
// on file should be stored anyway.
func domainQuestion(value string, class DomainClass) *httperr.DetailedError {
	details := map[string]any{detailKeyDomain: value, detailKeyRole: string(class.Role)}
	if class.Company != "" {
		details[detailKeyCompany] = class.Company
	}
	return &httperr.DetailedError{
		Status:  http.StatusConflict,
		Code:    CodeNeedsConfirmation,
		Detail:  value + " is already on file; send confirm to store it anyway",
		Details: details,
	}
}

// checkConsumerListEntry refuses an `extra` entry for the workspace's own
// domain or one the shipped list holds, and asks before one that is a company's.
// A carve-out is not asked about: it takes a domain out of the list.
func checkConsumerListEntry(ctx context.Context, tx pgx.Tx, domain, kind string, confirmed bool) error {
	if kind != FreemailKindExtra {
		return nil
	}
	class, err := ClassifyDomainTx(ctx, tx, domain)
	if err != nil {
		return err
	}
	switch {
	case class.Role == DomainRoleOwn:
		return httperr.Validation(detailKeyDomain, CodeOwnDomain, domain+" is your own domain")
	case class.Role == DomainRoleConsumerShipped:
		return httperr.Validation(detailKeyDomain, CodeAlreadyShipped, domain+" is already on the shipped list")
	case class.Role == DomainRoleCompany && !confirmed:
		return domainQuestion(domain, class)
	}
	return nil
}

// checkIdentityClaim refuses a claim that would switch off capture for a whole
// consumer-mail domain, and asks before one that covers a contact or a company
// already on file.
func checkIdentityClaim(ctx context.Context, tx pgx.Tx, kind, value string, confirmed bool) error {
	var class DomainClass
	var err error
	if kind == IdentityKindAddress {
		class, err = ClassifyAddressTx(ctx, tx, value)
	} else {
		class, err = ClassifyDomainTx(ctx, tx, value)
	}
	if err != nil {
		return err
	}
	switch {
	case class.Role.IsConsumer():
		return httperr.Validation("value", CodeConsumerMailDomain, value+" is a consumer mail domain shared by many senders")
	case (class.Role == DomainRoleCompany || class.Role == DomainRoleContactAddress) && !confirmed:
		return domainQuestion(value, class)
	}
	return nil
}
