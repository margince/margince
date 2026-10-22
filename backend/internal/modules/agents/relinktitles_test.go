// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestABatchRelinkTitleNamesTheDestination(t *testing.T) {
	activity, project := ids.NewV7(), ids.NewV7()
	call := NewRelinkActivitiesCall(oneRecord(datasource.EntityProject, project, `{"name":"FBA WirkCheck"}`, 3),
		nil, RelinkActivitiesCommand{ActivityIDs: []ids.UUID{activity}, EntityType: "project", EntityID: project})
	info, err := StageSubject(context.Background(), call)
	if err != nil {
		t.Fatalf("staging answered %v", err)
	}
	if !strings.Contains(info.Summary, `"FBA WirkCheck"`) || strings.Contains(info.Summary, project.String()) {
		t.Errorf("summary %q names the project by id instead of its name", info.Summary)
	}
}

// The single form binds to the activity, so it names the activity and reads
// nothing else. Its decider may not be able to see the destination.
func TestASingleRelinkTitleNamesTheActivityAndReadsNoDestination(t *testing.T) {
	activity, project := ids.NewV7(), ids.NewV7()
	provider := oneRecord(datasource.EntityActivity, activity, `{"kind":"email","subject":"Kickoff Migration"}`, 3)
	call := NewRelinkActivityCall(provider, nil,
		RelinkActivityCommand{ActivityID: activity, EntityType: "project", EntityID: project})
	info, err := StageSubject(context.Background(), call)
	if err != nil {
		t.Fatalf("staging answered %v", err)
	}
	if !strings.Contains(info.Summary, `"Kickoff Migration"`) || strings.Contains(info.Summary, activity.String()) {
		t.Errorf("summary %q names the activity by id instead of its subject", info.Summary)
	}
	if n := provider.readsOf[datasource.EntityRef{Type: datasource.EntityProject, ID: project}]; n != 0 {
		t.Errorf("the destination was read %d times; its name must not reach this card", n)
	}
}
