// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What a machine did that a customer-facing reader should see.
//
// ACTOR TYPE IS NOT ENOUGH, and this file is the whole of that argument. A row
// whose actor_type is agent, system or connector tells you a machine wrote it.
// It does not tell you the write MEANS anything to the person reading the
// morning: a projection refresh, a maintenance sweep and a retention pass are
// all machine writes, and a receipt that showed them would turn internal churn
// into apparent value — the surface congratulating itself for keeping its own
// books.
//
// So admission is a CLOSED LIST, and membership costs three things: a sentence
// the reader can understand, a consequence where the action has one, and a
// policy on whether it can be taken back. An action with no answer to those is
// not ready to be shown, and is counted in not_shown rather than served.
//
// The list is deliberately short. It grows when somebody decides a new machine
// action is worth a rep's attention and writes its three answers, never by an
// action arriving in the ledger.

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// admittedAction is what one machine action means to a reader.
type admittedAction struct {
	// sentence is the i18n key the client draws. A key rather than prose: the
	// product ships three languages, and a sentence composed here reaches a
	// German reader in English.
	sentence string
	// consequence says what it means for the reader, where the action has
	// something to say. Empty where it does not — an invented consequence is
	// worse than none, because a reader cannot tell the two apart.
	consequence string
	// reversible says whether an undo is offered at all for this KIND of
	// action. A true here is a necessary condition and never a sufficient one:
	// compose/undoability still judges the individual row, and its answer wins.
	reversible bool
}

// admitted is the closed set, keyed by the audit action.
//
// Keyed by action alone rather than by (action, entity_type): the audit
// vocabulary's verbs already carry their meaning — an `advance_stage` is a stage
// move whatever it moved — and a key per pair would multiply the list by seven
// while saying the same thing seven times.
var admitted = map[string]admittedAction{
	"advance_stage": {
		sentence:    "magic.action.advance_stage",
		consequence: "magic.consequence.stage_moved",
		reversible:  true,
	},
	"promote": {
		sentence:    "magic.action.promote",
		consequence: "magic.consequence.lead_promoted",
		reversible:  true,
	},
	"update": {
		sentence:    "magic.action.update",
		consequence: "",
		reversible:  true,
	},
	"assign": {
		sentence:    "magic.action.assign",
		consequence: "magic.consequence.owner_changed",
		reversible:  true,
	},
	"activity_relink": {
		sentence:    "magic.action.activity_relink",
		consequence: "magic.consequence.record_relinked",
		reversible:  true,
	},
	"send_email": {
		sentence:    "magic.action.send_email",
		consequence: "magic.consequence.message_sent",
		// A sent message cannot be unsent. Saying so is the point: a control
		// that looked reversible here would promise something the world does
		// not allow.
		reversible: false,
	},
	"schedule": {
		sentence:    "magic.action.schedule",
		consequence: "magic.consequence.meeting_booked",
		reversible:  false,
	},
	"disqualify": {
		sentence:    "magic.action.disqualify",
		consequence: "magic.consequence.lead_disqualified",
		reversible:  true,
	},
}

// meaningOf answers what an action means to a reader, and whether it means
// anything at all.
func meaningOf(action string) (admittedAction, bool) {
	meaning, ok := admitted[action]
	return meaning, ok
}

// machineActors are the actor types this surface reports.
//
// Human actors are absent deliberately. This reports what ran WITHOUT being
// asked; a person's own change is their own, and handing it back to them as
// machinery would be a lie about who did it.
func machineActors() []string {
	return []string{
		string(crmcontracts.MagicActorAgent),
		string(crmcontracts.MagicActorSystem),
		string(crmcontracts.MagicActorConnector),
	}
}
