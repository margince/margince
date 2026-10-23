// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRecordInformationRemainsReadableWithoutBecomingPersonalWork(t *testing.T) {
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil, nil,
		&stubDecay{rows: []QuietRelationship{{ContactID: ids.NewV7(), Name: "Quiet contact", QuietDays: 90, LastAt: readInstant.AddDate(0, 0, -90), HasOpenDeal: true}}},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).
		WithDomainQuestions(&stubDomainQuestions{rows: []DomainQuestion{
			{Domain: "probe.test", AskedAt: readInstant.AddDate(0, -4, 0)},
			{Domain: "business.example", AskedAt: readInstant},
		}})
	information, err := svc.Assemble(pageReader())
	if err != nil {
		t.Fatal(err)
	}
	if information.RelationshipDecay == nil || len(*information.RelationshipDecay) != 1 || information.DomainQuestions == nil || len(*information.DomainQuestions) != 2 {
		t.Fatalf("optional record information was lost: %+v", information)
	}
	work, err := svc.Worklist(pageReader(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(work.Queue) != 0 || work.Summary.Total != 0 || work.Focus == nil || len(work.Focus.Items) != 0 {
		t.Fatalf("mailbox context or silence became personal duty: %+v", work)
	}
}
