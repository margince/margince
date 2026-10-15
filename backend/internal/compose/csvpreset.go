// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A run may name the record its activities are filed under, so a file started
// from a contact's page needs no links column. The choice rides on the run's
// mapping, which the test and the commit both read.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// presetLinksFrom reads a request's preset links into the run's own shape. The
// wire's enum holds one type today; a zero id names no record.
func presetLinksFrom(sent *[]crmcontracts.ImportPresetLink) ([]migration.PresetLink, error) {
	if sent == nil {
		return nil, nil
	}
	out := make([]migration.PresetLink, 0, len(*sent))
	for _, link := range *sent {
		if !link.Type.Valid() || ids.UUID(link.Id).IsZero() {
			return nil, httperr.Validation("preset_links", "invalid",
				"A preset link names a contact by its id.")
		}
		out = append(out, migration.PresetLink{Type: string(link.Type), ID: link.Id.String()})
	}
	return out, nil
}

// presetLinksOnWire spells the run's preset links as the contract's.
func presetLinksOnWire(links []migration.PresetLink) *[]crmcontracts.ImportPresetLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]crmcontracts.ImportPresetLink, 0, len(links))
	for _, link := range links {
		id, err := ids.Parse(link.ID)
		if err != nil {
			continue
		}
		out = append(out, crmcontracts.ImportPresetLink{
			Type: crmcontracts.ImportPresetLinkType(link.Type), Id: openapi_types.UUID(id),
		})
	}
	return &out
}

// refuseNonActivityPreset holds a preset to the one kind of run it fits: a
// link on a company or a lead row would mean nothing.
func refuseNonActivityPreset(m migration.RunMapping) error {
	if len(m.PresetLinks) == 0 {
		return nil
	}
	for _, part := range m.PartList() {
		if part.Object != migration.ObjectActivity {
			return httperr.Validation("preset_links", "activities_only",
				fmt.Sprintf("A preset link files activities, but %s rows are in this run.", part.Object))
		}
	}
	return nil
}

// presetsAreVisible asks, with the caller's own row scope, whether each preset
// record may be read. A record the caller cannot see answers not-found like one
// that does not exist, so a preset discloses nothing.
func (h importHandlers) presetsAreVisible(ctx context.Context, links []migration.PresetLink) error {
	for _, link := range links {
		id, err := ids.Parse(link.ID)
		if err != nil {
			return httperr.Validation("preset_links", "invalid", "A preset link names a contact by its id.")
		}
		err = database.WithWorkspaceTx(ctx, h.db.Pool(), func(tx pgx.Tx) error {
			return auth.EnsureVisibleLive(ctx, tx, link.Type, id)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// presetSet is a run's preset records and what they resolved to. A preset is
// asked about once per writer: the answer is the same for every row.
type presetSet struct {
	links   []migration.PresetLink
	found   []activities.ActivityLinkInput
	checked bool
	missing *migration.Reason
}

// types are the record types the preset files an activity under.
func (p presetSet) types() []string {
	out := make([]string, len(p.links))
	for i, link := range p.links {
		out[i] = link.Type
	}
	return out
}

// resolvePreset answers the links every row carries, or the reason none can.
// A record that has gone out of the importer's row scope refuses each row, as
// a links cell naming it would.
func (w *csvWriters) resolvePreset(ctx context.Context) ([]activities.ActivityLinkInput, *migration.Reason, error) {
	if w.preset.checked {
		return w.preset.found, w.preset.missing, nil
	}
	for _, link := range w.preset.links {
		answer, err := w.links.visibleByID(ctx, link.Type, link.ID)
		if err != nil {
			return nil, nil, err
		}
		if !answer.Found {
			why := linkNotFoundWhy(link.Type, link.ID)
			w.preset.found, w.preset.missing = nil, &why
			break
		}
		w.preset.found = append(w.preset.found, activities.ActivityLinkInput{EntityType: link.Type, EntityID: answer.ID})
	}
	w.preset.checked = true
	return w.preset.found, w.preset.missing, nil
}
