// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// pageProbeProvider answers a search with a fixed page and a read by id.
type pageProbeProvider struct {
	queryProbeProvider
	page []datasource.Record
}

func (p *pageProbeProvider) Search(context.Context, datasource.SearchQuery) (datasource.SearchResult, error) {
	return datasource.SearchResult{Records: p.page}, nil
}

// recordToolsNamedBy registers the record-serving tools the way compose does:
// the namer arrives as a registry option, not as a per-tool argument.
func recordToolsNamedBy(p datasource.SystemOfRecordProvider, name SeatNamer) *Registry {
	r := NewRegistry(nil, nil, WithSeatNamer(name))
	RegisterCoreTools(r, p, nil, nil, nil, nil, nil, nil)
	RegisterListTool(r, p, noFilterVocabulary{})
	return r
}

type servedOwner struct {
	ID    ids.UUID `json:"id"`
	Name  string   `json:"name"`
	IsYou bool     `json:"is_you"`
}

type servedRow struct {
	ID    ids.UUID     `json:"id"`
	Owner *servedOwner `json:"owner"`
}

// handle calls one registered tool's handler as the given seat.
func handle(t *testing.T, r *Registry, tool string, seat ids.UUID, args string) json.RawMessage {
	t.Helper()
	raw, err := r.tools[tool].Handle(humanCtx(seat), json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return raw
}

func rowsOf(t *testing.T, raw json.RawMessage) map[ids.UUID]*servedOwner {
	t.Helper()
	var page struct {
		Records []servedRow `json:"records"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("decoding a page: %v", err)
	}
	owners := make(map[ids.UUID]*servedOwner, len(page.Records))
	for _, row := range page.Records {
		owners[row.ID] = row.Owner
	}
	return owners
}

// A company found by name is as likely to be a colleague's as one found by a
// plan: search_records and list_records rows say whose it is, named, and
// whether it is the caller's — and an unowned company claims nobody.
func TestASearchedOrListedRecordSaysWhoseItIs(t *testing.T) {
	me, sofia := ids.NewV7(), ids.NewV7()
	theirs := ownedRecord(datasource.EntityCompany, sofia)
	mine := ownedRecord(datasource.EntityCompany, me)
	nobodys := ownedRecord(datasource.EntityCompany, ids.UUID{})
	r := recordToolsNamedBy(&pageProbeProvider{page: []datasource.Record{theirs, mine, nobodys}},
		func(context.Context, []ids.UUID) (map[ids.UUID]string, error) {
			return map[ids.UUID]string{sofia: "Sofia Meier", me: "Lars"}, nil
		})

	for tool, args := range map[string]string{
		"search_records": `{"q":"Acme"}`,
		"list_records":   `{"record_type":"company"}`,
	} {
		owners := rowsOf(t, handle(t, r, tool, me, args))
		if got := owners[theirs.Ref.ID]; got == nil || got.Name != "Sofia Meier" || got.IsYou {
			t.Errorf("%s: a colleague's company is not named as hers: %+v", tool, got)
		}
		if got := owners[mine.Ref.ID]; got == nil || !got.IsYou {
			t.Errorf("%s: the caller's own company is not marked as theirs: %+v", tool, got)
		}
		if got := owners[nobodys.Ref.ID]; got != nil {
			t.Errorf("%s: an unowned company claims an owner: %+v", tool, got)
		}
	}
}

func TestAReadRecordSaysWhoseItIs(t *testing.T) {
	me, sofia := ids.NewV7(), ids.NewV7()
	theirs := ownedRecord(datasource.EntityCompany, sofia)
	nobodys := ownedRecord(datasource.EntityCompany, ids.UUID{})
	provider := &pageProbeProvider{queryProbeProvider: queryProbeProvider{records: map[ids.UUID]datasource.Record{
		theirs.Ref.ID: theirs, nobodys.Ref.ID: nobodys,
	}}}
	r := recordToolsNamedBy(provider, func(context.Context, []ids.UUID) (map[ids.UUID]string, error) {
		return map[ids.UUID]string{sofia: "Sofia Meier"}, nil
	})

	var read servedRow
	raw := handle(t, r, "read_record", me, `{"record_type":"company","id":"`+theirs.Ref.ID.String()+`"}`)
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatalf("decoding the read: %v", err)
	}
	if read.Owner == nil || read.Owner.Name != "Sofia Meier" || read.Owner.IsYou || read.Owner.ID != sofia {
		t.Errorf("a read of a colleague's company does not name her as its owner: %s", raw)
	}

	read = servedRow{}
	raw = handle(t, r, "read_record", me, `{"record_type":"company","id":"`+nobodys.Ref.ID.String()+`"}`)
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatalf("decoding the read: %v", err)
	}
	if read.Owner != nil {
		t.Errorf("a read of an unowned company claims an owner: %s", raw)
	}
}

// The owner only changes what a model says if the copy tells it what to do with
// one, on every tool that serves it.
func TestEveryRecordToolTellsTheModelToSayWhoseItIs(t *testing.T) {
	for tool, described := range map[string]toolCopy{
		"search_records": searchRecordsCopy, "list_records": listRecordsCopy,
		"read_record": readRecordCopy, "query_workspace": queryWorkspaceCopy,
	} {
		if !strings.Contains(described.render(), sayWhoseItIs) {
			t.Errorf("%s serves an owner but never says what to do when it is not the caller", tool)
		}
	}
}
