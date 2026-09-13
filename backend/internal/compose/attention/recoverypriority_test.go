// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDatedDealRecoveryDoesNotDependOnPortfolioValue(t *testing.T) {
	for _, priced := range []bool{false, true} {
		t.Run(map[bool]string{false: "unpriced", true: "only priced deal"}[priced], func(t *testing.T) {
			value := int64(8900000)
			provisional := true
			omitted := "omitted"
			closes := rankInstant.AddDate(0, 0, 14)
			risk := RiskyDeal{DealID: ids.NewV7(), QuietDays: 71, ExpectedCloseDate: &closes,
				CloseDateProvisional: &provisional, ForecastCategory: &omitted}
			if priced {
				risk.AmountMinor = &value
			}
			row := classifyRisk(riskItem(risk), rankInstant, materialBar{minor: value, known: priced}, dayMoney{})
			if row.item.Level != levelMaterialRisk {
				t.Fatalf("dated recovery level = %d", row.item.Level)
			}
			lead := classifyLead(OwedLead{ID: ids.NewV7(), Name: "Prospect"}, rankInstant)
			if !less(row, lead) {
				t.Fatal("generic prospecting outranked a recovery due within two weeks")
			}
			if row.item.Deal == nil || row.item.Deal.CloseDateProvisional == nil || !*row.item.Deal.CloseDateProvisional || *row.item.Deal.ForecastCategory != omitted {
				t.Fatal("the recovery row lost its provisional date or forecast category")
			}
			breached := classifyLead(OwedLead{ID: ids.NewV7(), State: "breached", DeadlineAt: rankInstant.Add(-time.Hour)}, rankInstant)
			if !less(breached, row) {
				t.Fatal("deal recovery displaced an actual breached response")
			}
		})
	}
}

func TestProspectingLabelsDoNotOverrideExistingWorkFacts(t *testing.T) {
	value := int64(100)
	risk := classifyRisk(riskItem(RiskyDeal{DealID: ids.NewV7(), QuietDays: 40, AmountMinor: &value}), rankInstant,
		materialBar{minor: value, known: true}, dayMoney{})
	lead := classifyLead(OwedLead{ID: ids.NewV7()}, rankInstant)
	if !less(risk, lead) {
		t.Fatal("a generic lead category outranked the known value of a quiet deal")
	}
	future := rankInstant.AddDate(0, 0, 7)
	task := classifyTask(item("future", "task", withDue(future)), rankInstant)
	if task.item.Level != levelAgreed {
		t.Fatal("future task was called urgent")
	}
}

func TestOrdinaryFollowupForAnOwedLeadKeepsItsDeadlineAndAction(t *testing.T) {
	leadID := ids.NewV7()
	dueAt := rankInstant.AddDate(0, 0, -2)
	lead := classifyLead(OwedLead{ID: leadID}, rankInstant)
	activity := taskItem(Task{ID: ids.NewV7(), Subject: "Confirm discovery agenda", LinkType: "lead", LinkID: leadID, DueAt: &dueAt}, rankInstant, rankInstant.AddDate(0, 0, 7), time.UTC)
	task := classifyTask(activity, rankInstant)
	rows := dropEscalationTasksAlreadyOwed([]ranked{lead, task})
	if len(rows) != 2 {
		t.Fatal("a real lead follow-up was removed as an SLA duplicate")
	}
	if rows[1].item.DueAt == nil || !rows[1].item.DueAt.Equal(dueAt) {
		t.Fatal("follow-up deadline lost")
	}
	if rows[1].item.Source != crmcontracts.WorklistItemSourceTask {
		t.Fatal("follow-up no longer resolves its task action")
	}
}

func TestDisablingTheResponseTargetDoesNotEraseAnOutstandingTaskDeadline(t *testing.T) {
	leadID := ids.NewV7()
	dueAt := rankInstant.Add(-time.Hour)
	lead := classifyLead(OwedLead{ID: leadID}, rankInstant)
	task := taskItem(Task{ID: ids.NewV7(), LeadResponseEscalation: true, Subject: "Response follow-up", LinkType: "lead", LinkID: leadID, DueAt: &dueAt}, rankInstant, rankInstant.AddDate(0, 0, 7), time.UTC)
	rows := dropEscalationTasksAlreadyOwed([]ranked{lead, classifyTask(task, rankInstant)})
	if len(rows) != 2 || rows[1].item.DueAt == nil || !rows[1].item.DueAt.Equal(dueAt) {
		t.Fatal("turning off a policy erased the outstanding activity's deadline")
	}
}
