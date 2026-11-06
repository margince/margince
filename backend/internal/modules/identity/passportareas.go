// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The areas of work a passport may act in, beside its scopes. A NULL column
// admits every area, those added later included; a list admits only its own.
// The code a consent writes, the grant it becomes and every passport the grant
// mints carry the same value, as they carry the scopes.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const auditFieldAreas = "work_areas"

// consentAllAreas is what the consent screen posts for a passport set to all areas.
const consentAllAreas = "all"

// ReachableAreas lists the areas the signed-in caller's role reaches, which a
// passport's areas are chosen from. Compose answers it from the tool registry.
type ReachableAreas func(ctx context.Context) []string

// WithReachableAreas binds the answer the passport page and the consent screen
// offer areas from. Nil offers none, so neither shows a choice.
func (h Handlers) WithReachableAreas(reach ReachableAreas) Handlers {
	h.reachableAreas = reach
	return h
}

func (h Handlers) reachable(ctx context.Context) []crmcontracts.WorkArea {
	out := []crmcontracts.WorkArea{}
	if h.reachableAreas == nil {
		return out
	}
	for _, area := range h.reachableAreas(ctx) {
		out = append(out, crmcontracts.WorkArea(area))
	}
	return out
}

// admitAreas refuses a choice no screen offers: an empty list, an area a user
// cannot choose, or one area named twice.
func admitAreas(areas principal.WorkAreas) error {
	if !areas.Limited {
		return nil
	}
	if len(areas.Chosen) == 0 {
		return &InvalidPassportFieldError{Field: "areas", Code: "no_area_chosen", Message: "choose at least one area, or all areas"}
	}
	for i, area := range areas.Chosen {
		if !workarea.IsChoosable(area) {
			return &InvalidPassportFieldError{Field: "areas", Code: "unknown_area", Message: fmt.Sprintf("%q is not an area a passport can be limited to", area)}
		}
		if slices.Contains(areas.Chosen[:i], area) {
			return &InvalidPassportFieldError{Field: "areas", Code: "area_repeated", Message: fmt.Sprintf("%q is chosen twice", area)}
		}
	}
	return nil
}

// areasColumn is the stored form: NULL for all areas.
func areasColumn(areas principal.WorkAreas) []string {
	if !areas.Limited {
		return nil
	}
	return slices.Clone(areas.Chosen)
}

// areasFromColumn reads the stored form back.
func areasFromColumn(column []string) principal.WorkAreas {
	if column == nil {
		return principal.WorkAreas{}
	}
	return principal.WorkAreas{Limited: true, Chosen: column}
}

func areasFromWire(wire crmcontracts.PassportAreas) principal.WorkAreas {
	if wire.Mode != crmcontracts.PassportAreasModeCustom {
		return principal.WorkAreas{}
	}
	chosen := make([]string, 0, len(wire.Areas))
	for _, area := range wire.Areas {
		chosen = append(chosen, string(area))
	}
	return principal.WorkAreas{Limited: true, Chosen: chosen}
}

func areasToWire(areas principal.WorkAreas) crmcontracts.PassportAreas {
	wire := crmcontracts.PassportAreas{Mode: crmcontracts.PassportAreasModeAll, Areas: []crmcontracts.WorkArea{}}
	if !areas.Limited {
		return wire
	}
	wire.Mode = crmcontracts.PassportAreasModeCustom
	for _, area := range areas.Chosen {
		wire.Areas = append(wire.Areas, crmcontracts.WorkArea(area))
	}
	return wire
}

// parseConsentedAreas reads the consent form's areas field. An absent field is
// all areas, the screen's default; any refusal is terminal, as a scope's is.
func parseConsentedAreas(raw string) (principal.WorkAreas, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 || (len(fields) == 1 && fields[0] == consentAllAreas) {
		return principal.WorkAreas{}, nil
	}
	areas := principal.WorkAreas{Limited: true, Chosen: fields}
	if err := admitAreas(areas); err != nil {
		return principal.WorkAreas{}, fmt.Errorf("%w: %w", err, apperrors.ErrInvalidArgument)
	}
	return areas, nil
}

// SetPassportAreas changes where one of the caller's own passports may act. A
// connection's passport carries the change to its grant, so the credential a
// renewal mints keeps it, even while the connection is between credentials.
// Another user's passport reads as absent. Whether the passport still works is
// the liveness rule's question, asked at every call, and not asked twice here.
func (s *Service) SetPassportAreas(ctx context.Context, id Identity, passportID ids.PassportID, areas principal.WorkAreas) error {
	// Asked of the caller as bound, before actorCtx restamps it as human.
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := admitAreas(areas); err != nil {
		return err
	}
	ctx = actorCtx(ctx, id)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var grantID *ids.UUID
		err := tx.QueryRow(ctx, `SELECT oauth_grant_id FROM passport WHERE id = $1 AND on_behalf_of = $2`,
			passportID, id.UserID).Scan(&grantID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		column := areasColumn(areas)
		var before []string
		if grantID == nil {
			before, err = setOwnPassportAreasTx(ctx, tx, passportID, column)
		} else {
			before, err = setGrantAreasTx(ctx, tx, *grantID, column)
		}
		if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "passport", passportID.UUID,
			map[string]any{auditFieldAreas: before}, map[string]any{auditFieldAreas: column})
		return err
	})
}

// setOwnPassportAreasTx writes a passport a human minted, under its row lock,
// and answers the areas it held.
func setOwnPassportAreasTx(ctx context.Context, tx pgx.Tx, passportID ids.PassportID, column []string) ([]string, error) {
	var before []string
	if err := tx.QueryRow(ctx, `SELECT work_areas FROM passport WHERE id = $1 FOR UPDATE`, passportID).Scan(&before); err != nil {
		return nil, err
	}
	_, err := tx.Exec(ctx, `UPDATE passport SET work_areas = $2 WHERE id = $1`, passportID, column)
	return before, err
}

// setGrantAreasTx writes the grant and every live passport under it, so the
// next call and the next renewal read one value. The grant is locked first,
// the order revocation and renewal take.
func setGrantAreasTx(ctx context.Context, tx pgx.Tx, grantID ids.UUID, column []string) ([]string, error) {
	var before []string
	if err := tx.QueryRow(ctx, `SELECT work_areas FROM oauth_grant WHERE id = $1 FOR UPDATE`, grantID).Scan(&before); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE oauth_grant SET work_areas = $2 WHERE id = $1`, grantID, column); err != nil {
		return nil, err
	}
	_, err := tx.Exec(ctx,
		`UPDATE passport SET work_areas = $2 WHERE oauth_grant_id = $1 AND revoked_at IS NULL`, grantID, column)
	return before, err
}

// SetPassportAreas implements (PUT /passports/{id}/areas).
func (h Handlers) SetPassportAreas(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	identity, ok := identityFrom(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "a passport's areas are chosen by its signed-in human")
		return
	}
	var req crmcontracts.PassportAreas
	if !httperr.Decode(w, r, &req) {
		return
	}
	if !req.Mode.Valid() {
		httperr.Write(w, r, httperr.Validation("mode", "unknown_mode", "say all or custom"))
		return
	}
	areas := areasFromWire(req)
	err := h.svc.SetPassportAreas(r.Context(), identity, ids.From[ids.PassportKind](ids.UUID(id)), areas)
	if badField, ok := errors.AsType[*InvalidPassportFieldError](err); ok {
		httperr.Write(w, r, httperr.Validation(badField.Field, badField.Code, badField.Message))
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, areasToWire(areas))
}
