// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

// Which email a reply prefix answers, and what a wrong answer costs: every
// draft refused for its prefix is a second model call for the same click.

import (
	"context"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func mail(direction crmcontracts.ActivityDirection, occurred time.Time, subject string) crmcontracts.Activity {
	return crmcontracts.Activity{
		Id:         openapi_types.UUID(ids.NewV7()),
		Kind:       crmcontracts.ActivityKindEmail,
		Direction:  &direction,
		Subject:    strPtr(subject),
		OccurredAt: occurred,
	}
}

func task(occurred time.Time, subject string) crmcontracts.Activity {
	return crmcontracts.Activity{
		Id:         openapi_types.UUID(ids.NewV7()),
		Kind:       crmcontracts.ActivityKindTask,
		Subject:    strPtr(subject),
		OccurredAt: occurred,
	}
}

// renewalThread is a contact whose newest email is theirs, with a task logged
// after it: the record the reported double call was drafted from.
func renewalThread() Input {
	view := viewWith(
		task(draftedAt.Add(-time.Hour), "Call Alice back about the retrofit"),
		mail(crmcontracts.ActivityDirectionInbound, draftedAt.Add(-9*24*time.Hour), "Re: Renewal terms for Demo GmbH"),
		mail(crmcontracts.ActivityDirectionOutbound, draftedAt.Add(-12*24*time.Hour), "Renewal terms for Demo GmbH"),
	)
	view.Contact.FullName = "Alice Müller"
	return FromView(view, Request{Envelope: envelopeAt(textlang.German, convstate.BandWeeks)})
}

// A model answering their newest email with "Re:" is right, and a task logged
// since does not make it wrong. Refusing it spent a correction call on every
// such click, and the retry it bought was told to rewrite a body nothing was
// wrong with.
func TestAReplyToTheirNewestEmailIsServedOnTheFirstCall(t *testing.T) {
	const answer = `{"subject":"Re: Renewal terms for Demo GmbH",` +
		`"body":"Guten Tag Alice Müller,\n\nvielen Dank für Ihre Rückmeldung zu den Verlängerungskonditionen für die Demo GmbH.\n\n` +
		`Haben Sie Zeit für ein kurzes Telefonat, um die Details zu besprechen?","reasoning":[]}`
	lane := &sequencedLane{answers: []string{answer, answer}}

	draft, by, err := Write(context.Background(), lane, renewalThread(), draftvoice.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if lane.calls != 1 {
		t.Errorf("one click made %d model calls, want 1", lane.calls)
	}
	if by != crmcontracts.WrittenByModel || !strings.HasPrefix(draft.Subject, "Re:") {
		t.Errorf("served %q by %s, want the model's reply subject", draft.Subject, by)
	}
}

func TestOnlyTheNewestEmailDecidesTheThread(t *testing.T) {
	at := func(hours int) time.Time { return draftedAt.Add(-time.Duration(hours) * time.Hour) }
	cases := map[string]struct {
		acts []crmcontracts.Activity
		want bool
	}{
		"their email is newest": {
			[]crmcontracts.Activity{mail(crmcontracts.ActivityDirectionInbound, at(2), "Pricing")}, true,
		},
		"a task after their email": {
			[]crmcontracts.Activity{task(at(1), "Call back"), mail(crmcontracts.ActivityDirectionInbound, at(2), "Pricing")}, true,
		},
		"our email after theirs": {
			[]crmcontracts.Activity{
				mail(crmcontracts.ActivityDirectionOutbound, at(1), "Pricing"),
				mail(crmcontracts.ActivityDirectionInbound, at(2), "Pricing"),
			}, false,
		},
		"a task after our email": {
			[]crmcontracts.Activity{task(at(1), "Call back"), mail(crmcontracts.ActivityDirectionOutbound, at(2), "Pricing")}, false,
		},
		"no email at all": {
			[]crmcontracts.Activity{task(at(1), "Call back")}, false,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			in := FromView(viewWith(c.acts...), Request{Envelope: envelopeAt(textlang.English, convstate.BandFresh)})
			if got := in.Threaded(); got != c.want {
				t.Errorf("Threaded() = %v, want %v", got, c.want)
			}
		})
	}
}
