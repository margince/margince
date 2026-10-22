// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// withRecord adds one authoritative row to a tallying provider.
func (p *tallyingProvider) withRecord(entity datasource.EntityType, id ids.UUID, fields string) *tallyingProvider {
	ref := datasource.EntityRef{Type: entity, ID: id}
	p.records[ref] = nativeRecord(datasource.Record{Ref: ref, Fields: json.RawMessage(fields), Version: 1})
	return p
}

// failingDestination answers every read of a project with a fault that is not
// a visibility answer.
type failingDestination struct{ *tallyingProvider }

func (p failingDestination) Read(ctx context.Context, ref datasource.EntityRef) (datasource.Record, error) {
	if ref.Type == datasource.EntityProject {
		return datasource.Record{}, errors.New("the store is unreachable")
	}
	return p.tallyingProvider.Read(ctx, ref)
}

func TestARelinkTitleNamesTheActivityAndTheDestination(t *testing.T) {
	activity, project := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name string
		call GovernedCall
	}{
		{"relink_activity", NewRelinkActivityCall(
			oneRecord(datasource.EntityActivity, activity, `{"kind":"email","subject":"Kickoff Migration"}`, 3).
				withRecord(datasource.EntityProject, project, `{"name":"FBA WirkCheck"}`),
			nil, RelinkActivityCommand{ActivityID: activity, EntityType: "project", EntityID: project})},
		{"relink_activities", NewRelinkActivitiesCall(
			oneRecord(datasource.EntityProject, project, `{"name":"FBA WirkCheck"}`, 3),
			nil, RelinkActivitiesCommand{ActivityIDs: []ids.UUID{activity}, EntityType: "project", EntityID: project})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := StageSubject(context.Background(), c.call)
			if err != nil {
				t.Fatalf("staging answered %v", err)
			}
			if !strings.Contains(info.Summary, `"FBA WirkCheck"`) {
				t.Errorf("summary %q does not name the project", info.Summary)
			}
			for _, id := range []ids.UUID{activity, project} {
				if strings.Contains(info.Summary, id.String()) {
					t.Errorf("summary %q shows a record id where its name belongs", info.Summary)
				}
			}
		})
	}
	t.Run("the single form names the activity by its subject", func(t *testing.T) {
		info, err := StageSubject(context.Background(), cases[0].call)
		if err != nil || !strings.Contains(info.Summary, `"Kickoff Migration"`) {
			t.Errorf("summary %q (err %v) does not name the activity", info.Summary, err)
		}
	})
}

func TestASingleRelinkToAnUnreadableDestinationStillStages(t *testing.T) {
	activity, project := ids.NewV7(), ids.NewV7()
	cmd := RelinkActivityCommand{ActivityID: activity, EntityType: "project", EntityID: project}
	provider := oneRecord(datasource.EntityActivity, activity, `{"kind":"email","subject":"Kickoff"}`, 3)

	info, err := StageSubject(context.Background(), NewRelinkActivityCall(provider, nil, cmd))
	if err != nil {
		t.Fatalf("a destination the caller cannot see refused the staging: %v", err)
	}
	if !strings.Contains(info.Summary, project.String()) {
		t.Errorf("summary %q names nothing for a destination it could not read", info.Summary)
	}

	_, err = StageSubject(context.Background(), NewRelinkActivityCall(failingDestination{provider}, nil, cmd))
	if err == nil {
		t.Error("a destination read that failed for another reason staged anyway")
	}
}
