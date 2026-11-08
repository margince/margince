// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aitest_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestRefWhereFindsTheRecordByAFieldWhereverTheRequestShowsIt(t *testing.T) {
	t.Parallel()
	req := model.Request{Messages: []model.Message{{
		Content: `<span>{"deal":{"id":"d1"},"timeline":[{"id":"a1","subject":"Kickoff"},{"id":"a2","subject":"Scope"}]}</span>` +
			"\n\nVoice {not json}\n\n" + `{"products":[{"id":"p1","label":"Linden press line"}]}`,
	}}}
	if got, ok := aitest.RefWhere(req, "subject", "Scope"); !ok || got != "a2" {
		t.Errorf("RefWhere(subject, Scope) = %q, %v; want a2", got, ok)
	}
	if got, ok := aitest.RefBeside(req, "Linden press line"); !ok || got != "p1" {
		t.Errorf("RefBeside past prose = %q, %v; want p1", got, ok)
	}
	if got, ok := aitest.RefWhere(req, "subject", "Renewal"); ok {
		t.Errorf("RefWhere found %q for a record the request does not show", got)
	}
}
