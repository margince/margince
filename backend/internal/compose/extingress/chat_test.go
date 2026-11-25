// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extingress_test

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/extingress"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
)

var received = time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)

func aChatRecord() extension.Record {
	return extension.Record{
		System: "member-chat", Key: "k-1", ThreadKey: "member-chat:t-1",
		Activity: extension.ActivityFields{
			Kind: extension.ActivityKindMessage, ChannelProvider: "member_chat", Body: "hi",
			OccurredAt: time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC), Direction: extension.DirectionInbound,
		},
		Counterparty: extension.Counterparty{
			DisplayName: "Chat Buyer",
			ChannelIdentity: extension.ChannelIdentity{
				Provider: "member_chat", ChannelUserID: "acct-1", DisplayName: "@chat",
			},
		},
	}
}

func aSource(pin extension.Posture) extingress.ChatSource {
	return extingress.ChatSource{
		CapturedBy: "connector:ext:memberchat", SourceSystem: "ext:memberchat:member-chat",
		Member: ids.NewV7(), Credential: extension.CredentialPerMember,
		Source: extension.IngressSource{System: "member-chat", PinnedPosture: pin}, Now: received,
	}
}

// A chat message is recognised by what it says, and anything else keeps the
// general mapping.
func TestOnlyAMessageOnADeclaredTransportNamedByAnAccountOnItIsChat(t *testing.T) {
	if !extingress.IsChatMessage(aChatRecord()) {
		t.Fatal("a message naming its counterparty by an account on its own transport was not read as chat")
	}
	for name, damage := range map[string]func(*extension.Record){
		"a note":                    func(r *extension.Record) { r.Activity.Kind = "note" },
		"another transport":         func(r *extension.Record) { r.Counterparty.ChannelIdentity.Provider = "other" },
		"no account":                func(r *extension.Record) { r.Counterparty.ChannelIdentity = extension.ChannelIdentity{} },
		"a message with a duration": func(r *extension.Record) { d := 5; r.Activity.DurationSeconds = &d },
	} {
		rec := aChatRecord()
		damage(&rec)
		if extingress.IsChatMessage(rec) {
			t.Errorf("%s was read as chat", name)
		}
	}
}

// The keys a unit stored its records under are the keys they keep.
func TestAUnitsOwnKeysAreKeptVerbatim(t *testing.T) {
	binding, msg, err := extingress.ChatInput(aSource(""), aChatRecord())
	if err != nil {
		t.Fatal(err)
	}
	if msg.Keys == nil || msg.Keys.SourceID != "k-1" || msg.Keys.ThreadKey != "member-chat:t-1" {
		t.Errorf("keys = %+v, want the record's own key and thread", msg.Keys)
	}
	if binding.SourceSystem != "ext:memberchat:member-chat" || binding.Provider != "member_chat" {
		t.Errorf("binding = %+v", binding)
	}
	if binding.Ownership != connector.ChatOwnedByWorkspace || binding.PinnedPosture != "" {
		t.Errorf("a source with no pin files as the company's, but the binding is %+v", binding)
	}
}

// A pin is what makes a source member-owned, and it travels as the pin.
func TestAPinnedSourceFilesAsTheMembersOwnCorrespondence(t *testing.T) {
	src := aSource(extension.PostureClassified)
	binding, _, err := extingress.ChatInput(src, aChatRecord())
	if err != nil {
		t.Fatal(err)
	}
	if binding.Ownership != connector.ChatOwnedByMember || binding.OwnerUserID != src.Member ||
		binding.PinnedPosture != connector.ChatPostureClassified {
		t.Errorf("binding = %+v, want member-owned by %s and pinned classified", binding, src.Member)
	}
}

func TestADayAndAnUnknownTimeFileAsADayAndAnUnknownOneTakesTheReceiptTime(t *testing.T) {
	day := aChatRecord()
	day.OccurredPrecision = extension.PrecisionDay
	_, msg, err := extingress.ChatInput(aSource(""), day)
	if err != nil || !msg.OccurredDay || !msg.OccurredAt.Equal(day.Activity.OccurredAt) {
		t.Fatalf("a day record = %+v, %v; want the day flag and its own time", msg, err)
	}
	unknown := aChatRecord()
	unknown.OccurredPrecision = extension.PrecisionUnknown
	unknown.Activity.OccurredAt = time.Time{}
	_, msg, err = extingress.ChatInput(aSource(""), unknown)
	if err != nil || !msg.OccurredDay || !msg.OccurredAt.Equal(received) {
		t.Fatalf("an unknown-time record = %+v, %v; want a day dated at the receipt time", msg, err)
	}
	_, msg, err = extingress.ChatInput(aSource(""), aChatRecord())
	if err != nil || msg.OccurredDay {
		t.Fatalf("an instant record = %+v, %v; want no day flag", msg, err)
	}
}

func TestAChosenSubjectBecomesTheCoresReference(t *testing.T) {
	id := ids.NewV7()
	for kind, want := range map[extension.SubjectKind]datasource.EntityType{
		extension.SubjectContact: datasource.EntityContact, extension.SubjectLead: datasource.EntityLead,
	} {
		rec := aChatRecord()
		rec.ChosenSubject = &extension.ChosenSubject{Kind: kind, ID: id.String()}
		_, msg, err := extingress.ChatInput(aSource(""), rec)
		if err != nil || msg.ChosenSubject == nil || msg.ChosenSubject.Type != want || msg.ChosenSubject.ID != id {
			t.Errorf("%s subject = %+v, %v; want %s %s", kind, msg.ChosenSubject, err, want, id)
		}
	}
}

// The core types the bytes, whatever a unit claims.
func TestAFilesTypeComesFromItsBytes(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n0000")
	parts := extingress.PartsOf([]extension.InboundFile{{Ordinal: 3, Filename: "a/b.pdf", ContentType: "application/pdf", Body: png}})
	if len(parts) != 1 || parts[0].ContentType != "image/png" || parts[0].DeclaredType != "application/pdf" || parts[0].Filename != "ab.pdf" {
		t.Fatalf("parts = %+v, want the sniffed type, the claim kept as declared and a safe name", parts)
	}
}

// The unit-facing vocabularies are restated on the published surface, so they
// are held equal to the core's.
func TestThePublishedVocabulariesAreTheCores(t *testing.T) {
	for _, pair := range [][2]string{
		{string(extension.PostureClassified), capture.PostureClassified},
		{string(extension.PostureHeld), capture.PostureHeld},
		{string(extension.PostureClassified), connector.ChatPostureClassified},
		{string(extension.PostureHeld), connector.ChatPostureHeld},
		{string(extension.SubjectContact), string(datasource.EntityContact)},
		{string(extension.SubjectLead), string(datasource.EntityLead)},
	} {
		if pair[0] != pair[1] {
			t.Errorf("published %q, core %q", pair[0], pair[1])
		}
	}
}

// Ownership comes from the channel's declared credential model, and a pin only
// sets posture. A pin on a bot channel, or with no member to bind, is refused.
func TestAPinNeverMakesABotChannelMemberOwned(t *testing.T) {
	bot := aSource(extension.PostureClassified)
	bot.Credential = extension.CredentialWorkspaceBot
	if _, _, err := extingress.ChatInput(bot, aChatRecord()); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a pinned source on a workspace bot channel = %v, want ErrInvalid", err)
	}
	nobody := aSource(extension.PostureHeld)
	nobody.Member = ids.UUID{}
	if _, _, err := extingress.ChatInput(nobody, aChatRecord()); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a pinned source with no member to bind = %v, want ErrInvalid", err)
	}
	source := []extension.IngressSource{{System: "member-chat", PinnedPosture: extension.PostureHeld}}
	if err := extingress.RefusePinWithoutMemberChannel(source, []extension.Channel{{Provider: "bot", CredentialModel: extension.CredentialWorkspaceBot}}); err == nil {
		t.Error("boot admitted a pinned source whose unit declares only a bot channel")
	}
	if err := extingress.RefusePinWithoutMemberChannel(source, []extension.Channel{{Provider: "mine", CredentialModel: extension.CredentialPerMember}}); err != nil {
		t.Errorf("boot refused a pinned source with a per-member channel: %v", err)
	}
}
