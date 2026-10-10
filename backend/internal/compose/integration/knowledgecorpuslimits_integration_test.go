// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A corpus's name, topic and description are held to the contract's caps, and
// its topic is never blank: the palette routes a question by it.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestACorpusIsRefusedPastTheContractsCapsAndWithoutATopic(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	tooLongDescription := strings.Repeat("d", 2001)
	atTheCap := strings.Repeat("é", 200)

	for name, tc := range map[string]struct {
		in    knowledge.NewCorpus
		field string
	}{
		"a 201-character name":         {knowledge.NewCorpus{Name: strings.Repeat("n", 201), TopicStatement: "t"}, "name"},
		"a 501-character topic":        {knowledge.NewCorpus{Name: "n", TopicStatement: strings.Repeat("t", 501)}, "topic_statement"},
		"a 2001-character description": {knowledge.NewCorpus{Name: "n", TopicStatement: "t", Description: &tooLongDescription}, "description"},
		"a blank topic":                {knowledge.NewCorpus{Name: "n", TopicStatement: "  "}, "topic_statement"},
		"an invisible topic":           {knowledge.NewCorpus{Name: "n", TopicStatement: "\u200b"}, "topic_statement"},
		"a 200-character wide name":    {knowledge.NewCorpus{Name: atTheCap, TopicStatement: "t"}, ""},
	} {
		_, err := store.CreateCorpus(ctx, tc.in)
		assertCorpusRefusal(t, name, err, tc.field)
	}
}

func TestEditingACorpusIsHeldToTheSameCapsAndKeepsATopic(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)
	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	long, blank, tooLongDescription := strings.Repeat("x", 501), " ", strings.Repeat("d", 2001)
	longName := strings.Repeat("n", 201)

	for name, tc := range map[string]struct {
		patch knowledge.UpdateCorpus
		field string
	}{
		"a 201-character name":         {knowledge.UpdateCorpus{Name: &longName}, "name"},
		"a 501-character topic":        {knowledge.UpdateCorpus{TopicStatement: &long}, "topic_statement"},
		"a blank topic":                {knowledge.UpdateCorpus{TopicStatement: &blank}, "topic_statement"},
		"a 2001-character description": {knowledge.UpdateCorpus{Description: &tooLongDescription}, "description"},
	} {
		_, err := store.EditCorpus(ctx, ids.UUID(made.Id), tc.patch)
		assertCorpusRefusal(t, name, err, tc.field)
	}
}

func assertCorpusRefusal(t *testing.T, name string, err error, field string) {
	t.Helper()
	var refused *httperr.DetailedError
	switch {
	case field == "" && err != nil:
		t.Errorf("%s: refused with %v", name, err)
	case field != "" && (!errors.As(err, &refused) || refused.Fields[0].Field != field):
		t.Errorf("%s: %v, want a refusal naming %s", name, err, field)
	}
}
