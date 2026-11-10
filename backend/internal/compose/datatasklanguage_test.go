// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryCoreRepairTitleReachesTheReaderInTheirLanguage(t *testing.T) {
	tasks := dataTaskCatalog(nil, func() time.Time { return time.Unix(0, 0) })
	if len(tasks) == 0 {
		t.Fatal("the repair catalog is empty")
	}
	for _, task := range tasks {
		for _, language := range []textlang.Lang{textlang.German, textlang.Vietnamese} {
			progress := []datatask.Progress{{TaskID: task.ID, Title: task.Title}}
			shown := dataTaskProgressForReader(systemtext.WithLocale(context.Background(), language), progress)
			want := systemtext.Copy(dataTaskTitleCode(task.ID), language)
			if len(shown) != 1 || shown[0].Title != want {
				t.Fatalf("%s in %s: %+v, want %q", task.ID, language, shown, want)
			}
			if progress[0].Title != task.Title {
				t.Fatal("reader projection changed the canonical title")
			}
		}
	}
}

func TestUnknownRepairTitlesKeepTheirSourceWording(t *testing.T) {
	progress := []datatask.Progress{{TaskID: "custom/repair", Title: "Operator supplied repair"}}
	shown := dataTaskProgressForReader(systemtext.WithLocale(context.Background(), textlang.Vietnamese), progress)
	if len(shown) != 1 || shown[0].Title != progress[0].Title {
		t.Fatalf("unknown repair title changed: %+v", shown)
	}
}
