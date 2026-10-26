// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// narrativeLanguageProbe keeps the system prompt each rep's sentence was asked
// with, then refuses, so the review stays deterministic.
type narrativeLanguageProbe struct{ systems map[ids.UUID]string }

func (p *narrativeLanguageProbe) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if actor, ok := principal.Actor(ctx); ok {
		p.systems[actor.UserID] = req.System
	}
	return model.Response{}, context.Canceled
}

// A personal review is read by its owner alone, so its sentence is asked for
// in the owner's saved language. With none saved, it uses the installation's.
func TestTheWeeklySentenceIsAskedInItsOwnersLanguage(t *testing.T) {
	for _, test := range []struct {
		name   string
		locale any
		want   string
	}{
		{"a saved language", "vi", "vi"},
		{"no saved language", nil, "en"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := integration.Setup(t)
			seedManagerRoles(t, e, e.Rep1, e.Rep2, e.Rep3)
			seedMeasuredNarrativeSources(t, e)
			e.WsExec(t, `UPDATE app_user SET locale = $1 WHERE id = $2`, test.locale, e.Rep1)
			probe := &narrativeLanguageProbe{systems: map[ids.UUID]string{}}
			w := teamSnapshotWorker(e)
			w.narrator = probe
			if err := w.measureWorkspace(e.Admin(), e.WS, teamJobClock); err != nil {
				t.Fatal(err)
			}
			system, asked := probe.systems[e.Rep1]
			if !asked {
				t.Fatal("the rep's sentence was never asked for")
			}
			if !strings.Contains(system, promptlang.Rule(test.want)) {
				t.Fatalf("the sentence is not asked in %q:\n%s", test.want, system)
			}
		})
	}
}
