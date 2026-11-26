// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

var (
	outsiderMail   = langcopytest.Outsider("a contact or a buyer reads this, sent on a member's behalf")
	controllerMail = langcopytest.Controller("the data controller writes this to a data subject, and in law says we")
	readersVoice   = langcopytest.OwnVoice("a consent statement and its answers are the data subject's own sentence, " +
		"so it may say tôi, chúng tôi and của mình")
)

// mailReaders names, by field prefix, the mail a member does not read. Every
// other field is member mail, held to every rule.
var mailReaders = []struct {
	prefix   string
	register langcopytest.Register
}{
	{"Room", outsiderMail},
	{"UnsubscribeLabel", outsiderMail},
	{"ManagePreferencesLabel", outsiderMail},
	{"SignOffClosing", outsiderMail},
	{"PoweredBy", outsiderMail},
	{"Confirm", controllerMail},
	{"ConfirmMarketing", readersVoice},
	{"ConfirmSubscription", readersVoice},
	{"Return", controllerMail},
	{"ReturnAction", readersVoice},
	{"ReturnConsentWording", readersVoice},
	{"NoticeAction", controllerMail},
	{"NoticePreview", controllerMail},
	{"NoticeSubject", controllerMail},
	{"NoticeBody", controllerMail},
	{"NoticeIgnore", controllerMail},
	{"OptOutAck", controllerMail},
	{"RequestAck", controllerMail},
	{"RestrictionLift", controllerMail},
	{"Disclosure", controllerMail},
	{"Advertis", controllerMail},
	{"Controller", controllerMail},
	{"Frequency", controllerMail},
	{"ZoneOfCountry", controllerMail},
	{"ConsentPurposeLabel", controllerMail},
	{"BusinessPurpose", controllerMail},
	{"MarketingPurpose", controllerMail},
	{"TransactionalPurpose", controllerMail},
}

func TestTheMailCopyFollowsTheEnglishAndVietnameseStylePages(t *testing.T) {
	read := map[string]bool{}
	for field, text := range langcopytest.Sentences(t, For(string(English)), "Language") {
		if register, prefix := readerOf(field); prefix != "" {
			read[prefix] = true
			register.EnglishText(t, field, text)
			continue
		}
		langcopytest.EnglishText(t, field, text)
	}
	for field, text := range langcopytest.Sentences(t, For(string(Vietnamese)), "Language") {
		if register, prefix := readerOf(field); prefix != "" {
			register.VietnameseText(t, field, text)
			continue
		}
		langcopytest.VietnameseText(t, field, text)
	}
	for _, reader := range mailReaders {
		if !read[reader.prefix] {
			t.Errorf("%s names no field of Copy; delete it or correct the prefix", reader.prefix)
		}
	}
}

// readerOf answers the register of the longest prefix that names a field, and
// that prefix, or "" for member mail.
func readerOf(field string) (langcopytest.Register, string) {
	var register langcopytest.Register
	matched := ""
	for _, reader := range mailReaders {
		if strings.HasPrefix(field, reader.prefix) && len(reader.prefix) > len(matched) {
			register, matched = reader.register, reader.prefix
		}
	}
	return register, matched
}
