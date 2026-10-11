// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The jurisdiction page: which pack applies, what it puts in force, and the
// documents it ships.
//
// Composed here because the answer spans the extension registry (which unit
// ships the pack), privacy (the floor policy), the messaging and
// subject-rights registries, and identity (the stored selection), and no
// module may import the others. One assembler reads them all, so a later agent
// tool asks the same function and cannot give a second answer.

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/compliancedocs"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	"github.com/margince/margince/backend/pkg/extension"
)

// requestKinds are the kinds the page lists a deadline for, in queue order.
var requestKinds = []subjectrightsrules.RequestKind{"access", "rectify", "erasure"}

// GetInstallationJurisdiction answers the page.
func (h installationSettingsHandlers) GetInstallationJurisdiction(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "GetInstallationJurisdiction")
		return
	}
	var out crmcontracts.InstallationJurisdiction
	err := h.readJurisdiction(r.Context(), func(ctx context.Context, tx pgx.Tx, code jurisdiction.Code) error {
		var err error
		out, err = assembleJurisdiction(ctx, tx, code)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DownloadJurisdictionDocument serves one of the selected pack's documents.
// An id the selected pack does not ship is 404, whichever pack ships it.
func (h installationSettingsHandlers) DownloadJurisdictionDocument(w http.ResponseWriter, r *http.Request, documentID string) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "DownloadJurisdictionDocument")
		return
	}
	var doc *compliancedocs.Document
	err := h.readJurisdiction(r.Context(), func(_ context.Context, _ pgx.Tx, code jurisdiction.Code) error {
		for _, d := range compliancedocs.For(code) {
			if d.ID == documentID {
				doc = &d
				return nil
			}
		}
		return apperrors.ErrNotFound
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.StreamObject(w, r, httperr.StreamedObject{
		Download: httperr.Download{ContentType: doc.MediaType, Filename: documentFilename(*doc), Size: int64(len(doc.Body))},
		Body:     io.NopCloser(bytes.NewReader(doc.Body)),
	}, "jurisdiction document "+doc.ID)
}

// readJurisdiction runs fn with the selected code, behind the page's gate: a
// human holding installation_settings read. The country is read through the
// GATED reader, never the machinery one the floor and the deadlines use.
func (h installationSettingsHandlers) readJurisdiction(ctx context.Context, fn func(context.Context, pgx.Tx, jurisdiction.Code) error) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "installation_settings", principal.ActionRead); err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		code, err := gatedCountry(ctx, tx)
		if err != nil {
			return err
		}
		return fn(ctx, tx, code)
	})
}

func gatedCountry(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	code, err := settings.GetTx(ctx, tx, identity.Country)
	return jurisdiction.Code(code), err
}

// assembleJurisdiction is the one reader of what a selection puts in force.
func assembleJurisdiction(ctx context.Context, tx pgx.Tx, code jurisdiction.Code) (crmcontracts.InstallationJurisdiction, error) {
	policy, err := privacy.ResolveFloorPolicy(ctx, tx, gatedCountry)
	if err != nil {
		return crmcontracts.InstallationJurisdiction{}, err
	}
	units := jurisdictionUnits()
	out := crmcontracts.InstallationJurisdiction{
		Available: []crmcontracts.JurisdictionUnit{},
		Floor:     floorContract(privacy.StatutoryFloorPosture(policy)),
		Deadlines: deadlinesFor(code),
		Documents: []crmcontracts.JurisdictionDocument{},
	}
	for _, code := range compiledInCodes() {
		out.Available = append(out.Available, units[jurisdiction.Code(code)])
	}
	if code == "" {
		return out, nil
	}
	selected := string(code)
	out.Selected = &selected
	if unit, ok := units[code]; ok {
		out.Unit = &unit
	}
	out.RetentionClasses = retentionClassesOf(code)
	out.Messaging = messagingOf(code)
	for _, d := range compliancedocs.For(code) {
		out.Documents = append(out.Documents, crmcontracts.JurisdictionDocument{
			Id: d.ID, Title: d.Title, Language: d.Language, MediaType: d.MediaType, SizeBytes: len(d.Body),
			Audience: crmcontracts.JurisdictionDocumentAudience(d.Audience), Kind: crmcontracts.JurisdictionDocumentKind(d.Kind),
		})
	}
	return out, nil
}

// jurisdictionUnits maps each compiled-in pack to the unit that ships it. A
// core-registered pack has no unit and is listed under its code alone.
func jurisdictionUnits() map[jurisdiction.Code]crmcontracts.JurisdictionUnit {
	units := map[jurisdiction.Code]crmcontracts.JurisdictionUnit{}
	for _, p := range jurisdiction.Applicable() {
		units[p.Code()] = crmcontracts.JurisdictionUnit{Code: string(p.Code())}
	}
	for _, e := range ComposedExtensions() {
		for _, p := range e.Jurisdictions {
			units[p.Code()] = unitContract(p.Code(), e)
		}
	}
	return units
}

func unitContract(code jurisdiction.Code, e extension.Extension) crmcontracts.JurisdictionUnit {
	return crmcontracts.JurisdictionUnit{Code: string(code), UnitName: string(e.Name), UnitVersion: string(e.Version), Description: string(e.Description)}
}

func retentionClassesOf(code jurisdiction.Code) *[]crmcontracts.JurisdictionRetentionClass {
	classes := []crmcontracts.JurisdictionRetentionClass{}
	pack, ok := jurisdiction.For(code)
	if !ok || pack.Retention() == nil {
		return &classes
	}
	for _, c := range pack.Retention().Classes() {
		classes = append(classes, crmcontracts.JurisdictionRetentionClass{
			Name: string(c.Name), Keep: c.Keep.String(), FromYearEnd: c.Anchor == jurisdiction.AnchorCalendarYearEnd,
		})
	}
	return &classes
}

func messagingOf(code jurisdiction.Code) *crmcontracts.JurisdictionMessagingRules {
	rules, ok := messagingrules.For(code)
	if !ok {
		return nil
	}
	out := &crmcontracts.JurisdictionMessagingRules{Version: rules.Version, Instruments: []crmcontracts.JurisdictionInstrument{}}
	for _, in := range rules.Instruments {
		instrument := crmcontracts.JurisdictionInstrument{Name: in.Name}
		if !in.EffectiveFrom.IsZero() {
			instrument.EffectiveFrom = &openapi_types.Date{Time: in.EffectiveFrom}
		}
		out.Instruments = append(out.Instruments, instrument)
	}
	return out
}

// deadlinesFor lists the deadline per kind the selection gives, the same
// lookup the request writers make: the pack's own where it declares one, the
// GDPR month otherwise.
func deadlinesFor(code jurisdiction.Code) []crmcontracts.JurisdictionDeadline {
	out := make([]crmcontracts.JurisdictionDeadline, 0, len(requestKinds))
	for _, kind := range requestKinds {
		deadline, rule := subjectrightsrules.Resolve(code, kind)
		row := crmcontracts.JurisdictionDeadline{Kind: crmcontracts.JurisdictionDeadlineKind(kind), Rule: rule}
		if deadline.Hours > 0 {
			hours := deadline.Hours
			row.Hours = &hours
		} else {
			period := deadline.Period.String()
			row.Period = &period
		}
		out = append(out, row)
	}
	return out
}

func floorContract(p privacy.FloorPosture) crmcontracts.RetentionFloorPolicy {
	out := crmcontracts.RetentionFloorPolicy{Known: p.Known, FromYearEnd: p.FromYearEnd, Packs: p.Packs}
	if out.Packs == nil {
		out.Packs = []string{}
	}
	if p.Known {
		class, keep := p.Class, p.Keep.String()
		out.Class, out.Keep = &class, &keep
	}
	if c := p.Carried; c != nil {
		out.Carried = &crmcontracts.CarriedRetentionFloor{
			Class: c.Class, Keep: c.Keep.String(), FromYearEnd: c.FromYearEnd,
			Before: c.Before.UTC(), BindsUntil: c.BindsUntil.UTC().Truncate(time.Second),
		}
	}
	return out
}

// documentFilename is the name a download saves under.
func documentFilename(d compliancedocs.Document) string {
	if d.MediaType == "application/pdf" {
		return d.ID + ".pdf"
	}
	return d.ID + ".md"
}
