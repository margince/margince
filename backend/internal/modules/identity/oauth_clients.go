// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The admin surface over oauth_client: every client an AI tool signs users in
// through, whichever way it came to exist. It also registers clients by hand,
// for tools that cannot register themselves.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/bearer"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	maxAgentClientNameRunes = 120
	maxAgentClientRedirects = 10
	createdViaAdmin         = "admin"
	agentClientAuditEntity  = "oauth_client"
	agentClientFieldName    = "name"
	agentClientFieldURIs    = "redirect_uris"
	// agentClientTierSort names the cursor's leading key: admin rows and rows
	// with a live connection (tier 0) list before the rest (tier 1).
	agentClientTierSort = "tier"
)

var errNoSecretToRotate = fmt.Errorf("%w: this client has no secret to rotate; only a client an admin registered with a secret has one", apperrors.ErrConflict)

// AgentClient is one OAuth client as the admin list shows it. It carries no
// secret and no digest of one.
type AgentClient struct {
	ID             ids.UUID
	ClientID       string
	ClientName     string
	RedirectURIs   []string
	RegisteredBy   string
	Confidential   bool
	Disabled       bool
	CreatedAt      time.Time
	ConnectedUsers int
	LastUsedAt     *time.Time
}

// RegisterAgentClientInput is what an admin types to register a client.
type RegisterAgentClientInput struct {
	Name         string
	RedirectURIs []string
	Confidential bool
}

// InvalidAgentClientFieldError maps to 422 naming Field.
type InvalidAgentClientFieldError struct{ Field, Code, Message string }

func (e *InvalidAgentClientFieldError) Error() string { return e.Field + ": " + e.Message }

// agentClientConnectedUsers counts the users with a live GRANT through the
// client; it decides nothing about any passport.
const agentClientConnectedUsers = `(SELECT count(DISTINCT g.user_id) FROM oauth_grant g
	  WHERE g.client_id = c.client_id AND g.revoked_at IS NULL)`

// agentClientLastUsed is the newest use of any passport the client was ever
// issued, revoked ones included: it reports history, not a credential.
const agentClientLastUsed = `(SELECT max(p.last_used_at) FROM passport p JOIN oauth_grant g ON g.id = p.oauth_grant_id
	  WHERE g.client_id = c.client_id)`

// agentClientSelect is the projection the list and a single row both read, so
// a client counts its connected users the same way on either.
const agentClientSelect = `
	SELECT c.id, c.client_id, c.client_name, c.redirect_uris, c.created_via,
	       c.client_secret_hash IS NOT NULL, c.disabled_at IS NOT NULL, c.created_at,
	       ` + agentClientConnectedUsers + `, ` + agentClientLastUsed + `
	  FROM oauth_client c`

func scanAgentClient(row pgx.Row) (AgentClient, error) {
	var c AgentClient
	err := row.Scan(&c.ID, &c.ClientID, &c.ClientName, &c.RedirectURIs, &c.RegisteredBy,
		&c.Confidential, &c.Disabled, &c.CreatedAt, &c.ConnectedUsers, &c.LastUsedAt)
	return c, err
}

// listedClient carries the tier the list query sorted it by, so the cursor is
// minted from the same expression the next page filters on.
type listedClient struct {
	AgentClient
	tier int
}

// ListAgentClients pages the live clients, admin rows and connected ones first.
// A client whose last connection ends between two page reads changes tier.
// It can then show twice or not at all, and the next full read lists it once.
func (s *Service) ListAgentClients(ctx context.Context, actor Identity, cursor *string, limitIn *int) ([]AgentClient, storekit.Page, error) {
	ctx, err := admit(ctx, actor, agentClientObject, principal.ActionRead)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	limit := storekit.ClampLimit(limitIn)
	after, err := decodeAgentClientCursor(cursor)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var clients []listedClient
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, client_id, client_name, redirect_uris, created_via, confidential, disabled,
			       created_at, connected_users, last_used_at, tier
			  FROM (SELECT l.*, CASE WHEN l.created_via = 'admin' OR l.connected_users > 0 THEN 0 ELSE 1 END AS tier
			          FROM (`+agentClientSelect+` WHERE c.deleted_at IS NULL)
			            AS l(id, client_id, client_name, redirect_uris, created_via, confidential,
			                 disabled, created_at, connected_users, last_used_at)) t
			 WHERE $1::int IS NULL OR t.tier > $1
			    OR (t.tier = $1 AND (t.created_at, t.id) < ($2::timestamptz, $3::uuid))
			 ORDER BY t.tier, t.created_at DESC, t.id DESC
			 LIMIT $4`, after.tier, after.createdAt, after.id, limit+1)
		if err != nil {
			return err
		}
		clients, err = pgx.CollectRows(rows, scanListedClient)
		return err
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return agentClientPage(clients, limit)
}

type agentClientCursor struct {
	tier      *int
	createdAt *time.Time
	id        *ids.UUID
}

func decodeAgentClientCursor(token *string) (agentClientCursor, error) {
	if token == nil || *token == "" {
		return agentClientCursor{}, nil
	}
	c, err := storekit.DecodeCursor(*token)
	if err != nil {
		return agentClientCursor{}, err
	}
	if c.SortField != agentClientTierSort || c.SortKey == nil {
		return agentClientCursor{}, &storekit.MalformedCursorError{}
	}
	tier, err := strconv.Atoi(*c.SortKey)
	if err != nil || (tier != 0 && tier != 1) {
		return agentClientCursor{}, &storekit.MalformedCursorError{}
	}
	return agentClientCursor{tier: &tier, createdAt: &c.CreatedAt, id: &c.ID}, nil
}

func scanListedClient(row pgx.CollectableRow) (listedClient, error) {
	var c listedClient
	err := row.Scan(&c.ID, &c.ClientID, &c.ClientName, &c.RedirectURIs, &c.RegisteredBy,
		&c.Confidential, &c.Disabled, &c.CreatedAt, &c.ConnectedUsers, &c.LastUsedAt, &c.tier)
	return c, err
}

func agentClientPage(listed []listedClient, limit int) ([]AgentClient, storekit.Page, error) {
	clients := make([]AgentClient, 0, min(len(listed), limit))
	for _, c := range listed[:min(len(listed), limit)] {
		clients = append(clients, c.AgentClient)
	}
	if len(listed) <= limit {
		return clients, storekit.Page{}, nil
	}
	last := listed[limit-1]
	key := strconv.Itoa(last.tier)
	next, err := storekit.EncodeOpaque(storekit.Cursor{
		CreatedAt: last.CreatedAt, ID: last.ID, SortField: agentClientTierSort, SortKey: &key,
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return clients, storekit.Page{NextCursor: next, HasMore: true}, nil
}

// RegisterAgentClient mints a client an admin types in, and its secret when it
// is confidential. The raw secret is returned here once and stored only as a
// digest; neither reaches the audit row.
func (s *Service) RegisterAgentClient(ctx context.Context, actor Identity, in RegisterAgentClientInput) (AgentClient, string, error) {
	ctx, err := admit(ctx, actor, agentClientObject, principal.ActionCreate)
	if err != nil {
		return AgentClient{}, "", err
	}
	name, uris, err := validAgentClientInput(in)
	if err != nil {
		return AgentClient{}, "", err
	}
	clientID, err := randomToken()
	if err != nil {
		return AgentClient{}, "", err
	}
	var secret string
	var hash *string
	if in.Confidential {
		raw, digest, err := bearer.Mint()
		if err != nil {
			return AgentClient{}, "", err
		}
		secret, hash = raw, &digest
	}
	var client AgentClient
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var id ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO oauth_client (client_id, client_name, redirect_uris, created_via, client_secret_hash)
			VALUES ($1, $2, $3, 'admin', $4) RETURNING id`, clientID, name, uris, hash).Scan(&id); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "create", agentClientAuditEntity, id, nil, map[string]any{
			"client_name": name, agentClientFieldURIs: uris, "confidential": in.Confidential,
		}); err != nil {
			return err
		}
		client, err = agentClientByID(ctx, tx, id)
		return err
	})
	if err != nil {
		return AgentClient{}, "", err
	}
	return client, secret, nil
}

func validAgentClientInput(in RegisterAgentClientInput) (string, []string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > maxAgentClientNameRunes {
		return "", nil, &InvalidAgentClientFieldError{
			Field: agentClientFieldName, Code: "invalid_name",
			Message: fmt.Sprintf("give the client a name of 1 to %d characters", maxAgentClientNameRunes),
		}
	}
	var uris []string
	for _, raw := range in.RedirectURIs {
		uri := strings.TrimSpace(raw)
		if !validRedirectURI(uri) {
			return "", nil, &InvalidAgentClientFieldError{
				Field: agentClientFieldURIs, Code: "invalid_redirect_uri",
				Message: fmt.Sprintf("%q: a redirect URI must be https, or http on localhost, with no fragment", uri),
			}
		}
		if !slices.Contains(uris, uri) {
			uris = append(uris, uri)
		}
	}
	if len(uris) == 0 || len(uris) > maxAgentClientRedirects {
		return "", nil, &InvalidAgentClientFieldError{
			Field: agentClientFieldURIs, Code: "redirect_uri_count",
			Message: fmt.Sprintf("give 1 to %d redirect URIs", maxAgentClientRedirects),
		}
	}
	return name, uris, nil
}

// RotateAgentClientSecret replaces a confidential client's secret. The old one
// stops at once; grants and refresh tokens stay, renewing with the new secret.
func (s *Service) RotateAgentClientSecret(ctx context.Context, actor Identity, id ids.UUID) (string, error) {
	ctx, err := admit(ctx, actor, agentClientObject, principal.ActionUpdate)
	if err != nil {
		return "", err
	}
	raw, digest, err := bearer.Mint()
	if err != nil {
		return "", err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var confidential bool
		err := tx.QueryRow(ctx, `
			SELECT client_secret_hash IS NOT NULL FROM oauth_client
			 WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&confidential)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !confidential {
			return errNoSecretToRotate
		}
		if _, err := tx.Exec(ctx, `UPDATE oauth_client SET client_secret_hash = $2 WHERE id = $1`, id, digest); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", agentClientAuditEntity, id,
			map[string]any{"client_secret": "set"}, map[string]any{"client_secret": "replaced"})
		return err
	})
	if err != nil {
		return "", err
	}
	return raw, nil
}

// SetAgentClientDisabled disables or enables a client. Disabled, it is refused
// on every path at once, live access tokens included (liveClientPredicate).
func (s *Service) SetAgentClientDisabled(ctx context.Context, actor Identity, id ids.UUID, disabled bool) (AgentClient, error) {
	ctx, err := admit(ctx, actor, agentClientObject, principal.ActionUpdate)
	if err != nil {
		return AgentClient{}, err
	}
	var client AgentClient
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var was bool
		err := tx.QueryRow(ctx, `
			SELECT disabled_at IS NOT NULL FROM oauth_client
			 WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&was)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if was != disabled {
			if _, err := tx.Exec(ctx, `
				UPDATE oauth_client SET disabled_at = CASE WHEN $2 THEN now() END WHERE id = $1`,
				id, disabled); err != nil {
				return err
			}
			if err := auditAgentClientDisabled(ctx, tx, id, disabled); err != nil {
				return err
			}
		}
		client, err = agentClientByID(ctx, tx, id)
		return err
	})
	return client, err
}

// DeleteAgentClient ends every connection the client holds and removes it
// from the list.
func (s *Service) DeleteAgentClient(ctx context.Context, actor Identity, id ids.UUID) error {
	ctx, err := admit(ctx, actor, agentClientObject, principal.ActionDelete)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error { return revokeClientTx(ctx, tx, id) })
}

func agentClientByID(ctx context.Context, tx pgx.Tx, id ids.UUID) (AgentClient, error) {
	client, err := scanAgentClient(tx.QueryRow(ctx, agentClientSelect+` WHERE c.id = $1 AND c.deleted_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentClient{}, apperrors.ErrNotFound
	}
	return client, err
}

func auditAgentClientDisabled(ctx context.Context, tx pgx.Tx, id ids.UUID, disabled bool) error {
	before, after := map[string]any{"disabled": !disabled}, map[string]any{"disabled": disabled}
	var err error
	if disabled {
		_, err = storekit.Audit(ctx, tx, "pause", agentClientAuditEntity, id, before, after)
	} else {
		_, err = storekit.Audit(ctx, tx, "resume", agentClientAuditEntity, id, before, after)
	}
	return err
}
