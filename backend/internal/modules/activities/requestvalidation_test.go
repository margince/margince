// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOnlyTaskCreationCanAcceptARequest(t *testing.T) {
	id := openapi_types.UUID(ids.NewV7())
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "note", RequestActivityId: &id, Source: "manual"})
	var fault *RequestAcceptanceFieldError
	if !errors.As(err, &fault) {
		t.Fatalf("note acceptance = %v, want a correctable field refusal", err)
	}
	field, _, _ := fault.FieldFault()
	if field != "request_activity_id" {
		t.Fatal("refusal does not name the invalid field")
	}
}

func TestARecencyCursorCannotResumeRequestPriorityOrder(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	cursor := "anything"
	for _, input := range []ListActivitiesInput{
		{RequestReviewAsOf: &at, Cursor: &cursor},
		{RequestCurrentAsOf: &at, Cursor: &cursor},
	} {
		_, _, _, _, err := listActivitiesFilter(unscopedCtx(), input)
		if !errors.Is(err, errRequestReviewWithCursor) {
			t.Fatalf("request cursor = %v", err)
		}
	}
}

// Automatic work, the reply-owed admission and the review list each read an
// asks_us verdict. Each applies the one rule for when it may count.
func TestEveryReaderOfAnOwedVerdictAsksWhetherItStands(t *testing.T) {
	stands := storekit.OwedVerdictStandsClause("a")
	for name, sql := range map[string]string{
		"AutomaticRequestIntentClause": storekit.AutomaticRequestIntentClause("a"),
		"capturedReplyAdmissionSQL":    capturedReplyAdmissionSQL("false", "false"),
		"reviewableRequestSQL":         reviewableRequestSQL("$1"),
		"confirmedRequestSQL":          confirmedRequestSQL,
	} {
		if !strings.Contains(sql, stands) {
			t.Errorf("%s admits an asks_us verdict without OwedVerdictStandsClause", name)
		}
	}
}

// asksUsComparison is SQL that tests an owed verdict against asks_us, spelled
// as a literal or through the OwedVerdictAsksUs constant.
var asksUsComparison = regexp.MustCompile("owed_verdict[^`\\n]{0,24}?(=|<>|DISTINCT FROM|IN \\()\\s*'(asks_us'|`\\s*\\+\\s*OwedVerdictAsksUs)")

// The census behind the test above. Every Go source under internal/ compares
// an owed verdict to asks_us only through storekit.AsksUsClause. A new reader
// then cannot skip whether the verdict stands.
func TestOnlyAsksUsClauseComparesAVerdictToAsksUs(t *testing.T) {
	const owner = "platform/database/storekit/requestintent.go"
	root := filepath.Join("..", "..")
	scanned := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		matches := asksUsComparison.FindAllIndex(source, -1)
		if filepath.ToSlash(rel) == owner {
			if len(matches) != 1 {
				t.Errorf("%s holds %d asks_us comparisons, want the one in AsksUsClause", owner, len(matches))
			}
			return nil
		}
		for _, at := range matches {
			t.Errorf("%s:%d compares an owed verdict to asks_us; read it through storekit.AsksUsClause",
				rel, bytes.Count(source[:at[0]], []byte("\n"))+1)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 1000 {
		t.Fatalf("scanned %d Go files under internal/, want the whole tree", scanned)
	}
}
