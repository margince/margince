// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BenchmarkRankingAWholeBacklog times the in-memory half of complete assembly.
// It ranks one first page over every admitted candidate. Each page would repeat
// this work if no lane were capped. The database half is TestDailyWorklistBacklog.
func BenchmarkRankingAWholeBacklog(b *testing.B) {
	reader := ids.NewV7()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	for _, mix := range []backlogMix{{name: "mixed", waiting: 25, risk: 15}, {name: "tasks_only"}} {
		for _, size := range []int{100, 1_000, 10_000, 100_000} {
			day, waiting := backlogDay(reader, size, mix)
			b.Run(fmt.Sprintf("%s/%d", mix.name, size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					page := (&Service{}).worklistFrom(ctx, day, scopeAll, "", worklistPage, waiting, worklistCursor{}, nil)
					if page.Summary.Total == 0 {
						b.Fatal("the backlog ranked to an empty day")
					}
				}
			})
		}
	}
}

// backlogMix is the share of the backlog, in percent, each non-task source
// holds; tasks take the rest.
type backlogMix struct {
	name          string
	waiting, risk int
}

// backlogDay renders size candidates through the lanes' own renderers: overdue
// tasks, customers waiting and quiet deals, each owned by the reader.
func backlogDay(reader ids.UUID, size int, mix backlogMix) (crmcontracts.Attention, waitingRead) {
	until := rankInstant.Truncate(24 * time.Hour).Add(24 * time.Hour)
	day := crmcontracts.Attention{AsOf: rankInstant}
	var waiting waitingRead
	waiting.read = true
	amount, currency := int64(500_000), "EUR"
	for i := range size {
		age := time.Duration(1+i%25) * 24 * time.Hour
		switch slot := i % 100; {
		case slot < mix.waiting:
			waiting.rows = append(waiting.rows, WaitingCustomer{
				ActivityID: ids.NewV7(), Subject: "Question about the offer", Since: rankInstant.Add(-age),
				FirstAskedAt: rankInstant.Add(-age), ContactID: ids.NewV7(),
			})
		case slot < mix.waiting+mix.risk:
			day.AtRisk = appendItem(day.AtRisk, riskItem(RiskyDeal{
				DealID: ids.NewV7(), Name: "Quiet deal", OwnerID: &reader, AmountMinor: &amount, Currency: &currency,
				QuietDays: 20 + i%40, LastTouchedAt: rankInstant.Add(-age),
			}))
		default:
			due := rankInstant.Add(-age)
			day.Planned = append(day.Planned, taskItem(Task{
				ID: ids.NewV7(), Subject: "Send the revised offer", DueAt: &due, AssigneeID: &reader,
				LinkType: subjectContact, LinkID: ids.NewV7(),
			}, rankInstant, until, time.UTC))
		}
	}
	return day, waiting
}

func appendItem(lane *[]crmcontracts.AttentionItem, row crmcontracts.AttentionItem) *[]crmcontracts.AttentionItem {
	if lane == nil {
		lane = &[]crmcontracts.AttentionItem{}
	}
	*lane = append(*lane, row)
	return lane
}
