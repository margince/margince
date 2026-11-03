// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A change in what a company runs, as a signal on its account.
//
// This is the seam between the contacts module, which owns the company record and
// notices the change while writing it, and the signals module, which owns the
// row. Neither imports the other; the edge is spelled here, which is the same
// arrangement every other cross-module producer uses.
//
// The event fires on CHANGE, never on observation. A scheduled pass over a
// company that runs exactly what it ran last week must raise nothing — a rep
// opening the account should see "they moved to Microsoft 365 in March", not a
// weekly restatement of a mail provider that never moved.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/techprofile"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The signal vocabulary for a technical change: one kind, because a rep filters
// for "something about their stack moved" rather than for which field it was,
// and the summary already says which.
const (
	kindTechnicalChange = "technical_change"
	technicalChannel    = "derived"
	technicalSource     = "technical-lookup"
	// technicalPreviousKey names what the record held before, which is what
	// makes a move readable as a move rather than as an arrival.
	technicalPreviousKey = "previous"
)

// technicalChangeRecorder builds the recorder the contacts module calls inside
// its own write transaction, so the record change and the company event commit
// together or not at all.
func technicalChangeRecorder() contacts.TechnicalChangeRecorder {
	return func(ctx context.Context, tx pgx.Tx, change contacts.TechnicalChange, at time.Time) error {
		parts := technicalPartsOf(change)
		summary, ok := parts.worded(identity.BaseLanguageForRecord(ctx, tx))
		if !ok {
			// A change in a field nobody would act on. The record still holds
			// it; it just does not earn a line on the account's signal list.
			return nil
		}
		params, err := json.Marshal(parts)
		if err != nil {
			return fmt.Errorf("encoding a technical change summary's parts: %w", err)
		}
		_, err = signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind:          kindTechnicalChange,
			CompanyID:     change.CompanyID.UUID,
			Summary:       summary,
			SummaryParams: params,
			// Never `warn` or `urgent`: a company changing its own systems is
			// news about the account, not a problem with it.
			Severity: severityInfo,
			Channel:  technicalChannel,
			Source:   technicalSource,
			// The state it arrived at AND the state it left, so a company that
			// moves Google → Microsoft → Google → Microsoft raises an event
			// each time rather than the second Microsoft move colliding with
			// the first. A pass over an UNCHANGED company still raises
			// nothing, because it produces no change to file at all.
			Fingerprint: fingerprintOf(technicalSource, change.CompanyID.String(),
				change.Field, change.ValueKey, string(change.Kind), change.Previous),
			// The evidence is the public record that proved it — the MX host,
			// the certificate hostname, the matched marker — because "how do
			// you know?" is the first question this claim invites.
			Evidence: []signals.DerivedEvidence{{Snippet: change.Evidence}},
			Audit: map[string]any{
				paramKind:            string(change.Kind),
				extractionFieldKey:   change.Field,
				extractionValueKey:   change.Value,
				technicalPreviousKey: change.Previous,
			},
		}, at)
		if err != nil {
			return fmt.Errorf("filing a technical change signal: %w", err)
		}
		return nil
	}
}

// technicalSummaryCopy is one language's set of summary sentences. Every field
// is required — a partial set is a silent English fallback in miniature, and
// the census test refuses one. Each verb interpolates change.Value, and the
// two "moved" forms interpolate change.Previous after it; product and provider
// names are never translated, which is why the holes are all there is.
type technicalSummaryCopy struct {
	// mailMoved / mailSet / mailGone: the mail system moved, was seen for the
	// first time, or is no longer seen.
	mailMoved string
	mailSet   string
	mailGone  string
	// serviceGone / serviceNew: an operated service left the air, or arrived.
	serviceGone string
	serviceNew  string
	// hostingMoved / hostingSet / hostingGone: the hosting provider changed,
	// was first seen, or is no longer seen.
	hostingMoved string
	hostingSet   string
	hostingGone  string
	// technologyGone / technologyNew: a product left the stack, or joined it.
	technologyGone string
	technologyNew  string
}

// A vocabulary is spelled one key per row rather than once per language.
//
// The frames above are prose and differ in shape, so each language states its
// own.
//
// A vocabulary entry is one concept in three languages, and a row shows all
// three. A missing translation is then visible on the line that should carry
// it, and the table does not repeat its structure per language.
type vocabularyEntry struct{ english, german, vietnamese string }

// in answers this language, falling back to English for one the product does
// not ship. The census holds every row complete, so the fallback is a floor
// rather than a path a shipped language takes.
func (e vocabularyEntry) in(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return e.german
	case textlang.Vietnamese:
		return e.vietnamese
	default:
		return e.english
	}
}

// serviceVocabulary names every service key techprofile classifies.
//
// One name serves both the sentence and the record, because a service name is
// a standalone noun phrase in either position. The census holds this complete
// against techprofile.ServiceKeys.
var serviceVocabulary = map[string]vocabularyEntry{
	techprofile.ServiceWebshop:        {"Webshop", "Webshop", "Cửa hàng trực tuyến"},
	techprofile.ServiceCustomerPortal: {"Customer portal", "Kundenportal", "Cổng khách hàng"},
	techprofile.ServiceCareers:        {"Careers page", "Karriereseite", "Trang tuyển dụng"},
	techprofile.ServiceAPI:            {"Public API", "Öffentliche API", "API công khai"},
	techprofile.ServiceVPN:            {"VPN access", "VPN-Zugang", "Truy cập VPN"},
	techprofile.ServiceMail:           {"Own mail infrastructure", "Eigene Mail-Infrastruktur", "Hạ tầng mail riêng"},
	techprofile.ServiceFileCloud:      {"File cloud", "Datei-Cloud", "Đám mây lưu trữ tệp"},
	techprofile.ServiceDevInfra: {
		"Development infrastructure", "Entwicklungs-Infrastruktur", "Hạ tầng phát triển",
	},
	techprofile.ServiceStatusPage: {"Status page", "Statusseite", "Trang trạng thái"},
	techprofile.ServiceSupport:    {"Support portal", "Support-Portal", "Cổng hỗ trợ"},
}

// mailSentenceVocabulary is the two mail fallbacks inside a sentence. A real
// provider's key is absent here, so its vendor name passes through
// untranslated.
var mailSentenceVocabulary = map[string]vocabularyEntry{
	techprofile.MailSelfHosted: {"their own mail server", "eigener Mailserver", "máy chủ mail riêng"},
	techprofile.MailOther:      {"another provider", "anderer Anbieter", "nhà cung cấp khác"},
}

// mailRecordVocabulary is the same two keys as a standalone record value.
// "Mail runs on their own mail server" needs the possessive, and a fact row
// reading "their own mail server" does not.
var mailRecordVocabulary = map[string]vocabularyEntry{
	techprofile.MailSelfHosted: {"Own mail server", "Eigener Mailserver", "Máy chủ mail riêng"},
	techprofile.MailOther:      {"Another provider", "Anderer Anbieter", "Nhà cung cấp khác"},
}

// securityVocabulary and securityEvidenceVocabulary are the email-security
// postures as the record carries them. The protocol names stay as they are,
// because SPF and DMARC are not words any language translates.
var securityVocabulary = map[string]vocabularyEntry{
	techprofile.SecuritySPF:             {"SPF published", "SPF veröffentlicht", "Đã công bố SPF"},
	techprofile.SecurityDMARCNone:       {"DMARC observing only", "DMARC nur beobachtend", "DMARC chỉ theo dõi"},
	techprofile.SecurityDMARCQuarantine: {"DMARC quarantining", "DMARC in Quarantäne", "DMARC cách ly"},
	techprofile.SecurityDMARCReject:     {"DMARC enforced", "DMARC durchgesetzt", "DMARC đã thực thi"},
	techprofile.SecurityDKIM:            {"DKIM configured", "DKIM eingerichtet", "Đã thiết lập DKIM"},
}

var securityEvidenceVocabulary = map[string]vocabularyEntry{
	techprofile.SecuritySPF:       {"SPF record (v=spf1)", "SPF-Eintrag (v=spf1)", "Bản ghi SPF (v=spf1)"},
	techprofile.SecurityDMARCNone: {"DMARC record: p=none", "DMARC-Eintrag: p=none", "Bản ghi DMARC: p=none"},
	techprofile.SecurityDMARCQuarantine: {
		"DMARC record: p=quarantine", "DMARC-Eintrag: p=quarantine", "Bản ghi DMARC: p=quarantine",
	},
	techprofile.SecurityDMARCReject: {
		"DMARC record: p=reject", "DMARC-Eintrag: p=reject", "Bản ghi DMARC: p=reject",
	},
	techprofile.SecurityDKIM: {
		"DKIM selector answered", "DKIM-Selector beantwortet", "Bộ chọn DKIM đã phản hồi",
	},
}

// technicalSummaryByLang is the census, keyed by textlang.Lang so the test can
// walk textlang.Shipped and ask this map directly.
var technicalSummaryByLang = map[textlang.Lang]technicalSummaryCopy{
	textlang.English: {
		mailMoved:      "Mail now runs on %s (was %s)",
		mailSet:        "Mail runs on %s",
		mailGone:       "Mail no longer runs on %s",
		serviceGone:    "%s went offline",
		serviceNew:     "%s is newly online",
		hostingMoved:   "Hosting moved to %s (was %s)",
		hostingSet:     "Hosted at %s",
		hostingGone:    "No longer hosted at %s",
		technologyGone: "No longer uses %s",
		technologyNew:  "Now uses %s",
	},
	textlang.German: {
		mailMoved:      "Mail läuft jetzt über %s (vorher %s)",
		mailSet:        "Mail läuft über %s",
		mailGone:       "Mail läuft nicht mehr über %s",
		serviceGone:    "%s ist offline gegangen",
		serviceNew:     "%s ist neu online",
		hostingMoved:   "Hosting jetzt bei %s (vorher %s)",
		hostingSet:     "Hosting bei %s",
		hostingGone:    "Hosting nicht mehr bei %s",
		technologyGone: "%s wird nicht mehr eingesetzt",
		technologyNew:  "Setzt jetzt %s ein",
	},
	textlang.Vietnamese: {
		mailMoved:      "Mail hiện chạy trên %s (trước đây là %s)",
		mailSet:        "Mail chạy trên %s",
		mailGone:       "Mail không còn chạy trên %s",
		serviceGone:    "%s đã ngừng hoạt động",
		serviceNew:     "%s vừa xuất hiện trực tuyến",
		hostingMoved:   "Hosting đã chuyển sang %s (trước đây là %s)",
		hostingSet:     "Hosting tại %s",
		hostingGone:    "Không còn hosting tại %s",
		technologyGone: "Không còn dùng %s",
		technologyNew:  "Hiện đang dùng %s",
	},
}

type technicalChangeParts struct {
	Field       string                       `json:"field"`
	Kind        contacts.TechnicalChangeKind `json:"kind"`
	Value       string                       `json:"value"`
	ValueKey    string                       `json:"value_key,omitempty"`
	Previous    string                       `json:"previous,omitempty"`
	PreviousKey string                       `json:"previous_key,omitempty"`
}

func technicalPartsOf(change contacts.TechnicalChange) technicalChangeParts {
	return technicalChangeParts{
		Field: change.Field, Kind: change.Kind, Value: change.Value, ValueKey: change.ValueKey,
		Previous: change.Previous, PreviousKey: change.PreviousKey,
	}
}

// worded is the sentence a rep reads on the account.
//
// Written per field rather than from a template, because the four fields are
// four different pieces of news: a mail system moving is an IT decision worth a
// call, a careers page appearing is a hiring signal, and a shared phrasing
// would flatten both into "a technical signal changed".
func (parts technicalChangeParts) worded(lang textlang.Lang) (string, bool) {
	said := technicalCopyFor(lang)

	switch parts.Field {
	case contacts.FactMailProvider:
		// The fallback mail labels are prose and follow the language; a real
		// provider's key misses the map and its vendor name passes through.
		value := nameOr(mailSentenceVocabulary, lang, parts.ValueKey, parts.Value)
		switch parts.Kind {
		case contacts.TechnicalMoved:
			previous := nameOr(mailSentenceVocabulary, lang, parts.PreviousKey, parts.Previous)
			return fmt.Sprintf(said.mailMoved, value, previous), true
		case contacts.TechnicalGone:
			return fmt.Sprintf(said.mailGone, value), true
		}
		return fmt.Sprintf(said.mailSet, value), true
	case contacts.FactOperatedService:
		value := nameOr(serviceVocabulary, lang, parts.ValueKey, parts.Value)
		if parts.Kind == contacts.TechnicalGone {
			return fmt.Sprintf(said.serviceGone, value), true
		}
		return fmt.Sprintf(said.serviceNew, value), true
	case contacts.FactHostingProvider:
		switch parts.Kind {
		case contacts.TechnicalMoved:
			return fmt.Sprintf(said.hostingMoved, parts.Value, parts.Previous), true
		case contacts.TechnicalGone:
			return fmt.Sprintf(said.hostingGone, parts.Value), true
		}
		return fmt.Sprintf(said.hostingSet, parts.Value), true
	case contacts.FactTechnology:
		if parts.Kind == contacts.TechnicalGone {
			return fmt.Sprintf(said.technologyGone, parts.Value), true
		}
		return fmt.Sprintf(said.technologyNew, parts.Value), true
	default:
		// email_security lands here on purpose. A DMARC policy tightening is a
		// real fact about the company and belongs on the record, but it is not
		// a reason to pick up the phone, and every account publishing one would
		// put a line on every signal list for nobody to act on.
		return "", false
	}
}

func (parts technicalChangeParts) complete() bool {
	switch parts.Kind {
	case contacts.TechnicalMoved:
		return parts.Value != "" && parts.Previous != ""
	case contacts.TechnicalAppeared, contacts.TechnicalGone:
		return parts.Value != ""
	default:
		return false
	}
}

// nameOr is the display name a language keeps for a key, and the record's own
// label when it keeps none — a vendor name, which no language translates.
func nameOr(vocabulary map[string]vocabularyEntry, lang textlang.Lang, key, recorded string) string {
	if entry, ok := vocabulary[key]; ok {
		return entry.in(lang)
	}
	return recorded
}

// technicalCopyFor is this language's copy, English when the language is one
// the product does not ship.
//
// Unreachable for a shipped language, because the census holds the map
// complete. The language comes off a settings row, and answering English beats
// answering nothing.
func technicalCopyFor(lang textlang.Lang) technicalSummaryCopy {
	said, ok := technicalSummaryByLang[lang]
	if !ok {
		return technicalSummaryByLang[textlang.English]
	}
	return said
}

// recordName is what a record calls one enumerated signal, in this language.
//
// Keyed by field as well as key, so a service and a mail posture that ever
// share a key cannot borrow each other's name.
func recordName(signal techprofile.Signal, lang textlang.Lang) (string, bool) {
	switch signal.Field {
	case techprofile.FieldOperatedService:
		return named(serviceVocabulary, lang, signal.Key)
	case techprofile.FieldMailProvider:
		return named(mailRecordVocabulary, lang, signal.Key)
	case techprofile.FieldEmailSecurity:
		return named(securityVocabulary, lang, signal.Key)
	default:
		return "", false
	}
}

// named answers one vocabulary row in this language, and false for a key the
// vocabulary does not carry.
func named(vocabulary map[string]vocabularyEntry, lang textlang.Lang, key string) (string, bool) {
	entry, ok := vocabulary[key]
	if !ok {
		return "", false
	}
	return entry.in(lang), true
}
