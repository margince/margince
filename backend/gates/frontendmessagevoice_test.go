// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var updateMessageVoice = flag.Bool("update-messagevoice", false, "rewrite the frontend's author-side cases from the activity engine")

type messageVoiceCase struct {
	Kind      crmcontracts.ActivityKind        `json:"kind"`
	Direction *crmcontracts.ActivityDirection  `json:"direction"`
	Author    *crmcontracts.ActivityAuthorKind `json:"author"`
	Side      *string                          `json:"side"`
}

func TestTheFrontendReadsTheSameAuthorSideAsTheActivityEngine(t *testing.T) {
	t.Parallel()
	schemas := crmYAMLSchemas(t)
	directions := schemas["Activity"].Properties["direction"].Enum
	authors := schemas["ActivityAuthor"].Properties["kind"].Enum
	kinds := schemas["Activity"].Properties["kind"].Enum
	if len(directions) == 0 || len(authors) == 0 || len(kinds) == 0 {
		t.Fatal("the contract's direction, author or kind census is empty")
	}
	var rows []messageVoiceCase
	for _, kind := range kinds {
		rows = append(rows, messageVoiceCases(crmcontracts.ActivityKind(kind), directions, authors)...)
	}
	rendered, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rendered = append(rendered, '\n')
	name := filepath.Join("..", "frontend", "src", "lib", "messagevoice.contract.json")
	if *updateMessageVoice {
		if err := os.WriteFile(name, rendered, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	committed, err := gatekit.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, rendered) {
		t.Fatal("author-side cases are stale; run go test ./gates -run TestTheFrontendReadsTheSameAuthorSide -update-messagevoice")
	}
}

// messageVoiceCases is every direction and author of one kind, with the side
// the activity engine reads; a row with no direction is listed once.
func messageVoiceCases(kind crmcontracts.ActivityKind, directions, authors []string) []messageVoiceCase {
	var rows []messageVoiceCase
	for _, direction := range append([]string{""}, directions...) {
		if direction == "" && len(rows) > 0 {
			continue
		}
		for _, author := range append([]string{""}, authors...) {
			row := crmcontracts.Activity{Kind: kind}
			test := messageVoiceCase{Kind: kind}
			if direction != "" {
				value := crmcontracts.ActivityDirection(direction)
				row.Direction, test.Direction = &value, &value
			}
			if author != "" {
				value := crmcontracts.ActivityAuthorKind(author)
				row.Voice = &crmcontracts.ActivityVoice{Author: &crmcontracts.ActivityAuthor{Kind: value}}
				test.Author = &value
			}
			if ours, known := activities.WroteItOurSide(row); known {
				side := "theirs"
				if ours {
					side = "ours"
				}
				test.Side = &side
			}
			rows = append(rows, test)
		}
	}
	return rows
}
