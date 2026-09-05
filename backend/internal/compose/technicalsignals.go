// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A change in what a company runs, as a signal on its account.
//
// This is the seam between the people module, which owns the company record and
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
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
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

// technicalChangeRecorder builds the recorder the people module calls inside
// its own write transaction, so the record change and the company event commit
// together or not at all.
func technicalChangeRecorder() people.TechnicalChangeRecorder {
	return func(ctx context.Context, tx pgx.Tx, change people.TechnicalChange, at time.Time) error {
		summary, ok := technicalChangeSummary(technicalSummaryLanguage(ctx, tx), change)
		if !ok {
			// A change in a field nobody would act on. The record still holds
			// it; it just does not earn a line on the account's signal list.
			return nil
		}
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind:           kindTechnicalChange,
			OrganizationID: change.OrganizationID.UUID,
			Summary:        summary,
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
			Fingerprint: fingerprintOf(technicalSource, change.OrganizationID.String(),
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

// technicalSummaryLanguage resolves the installation's base language inside the
// transaction the recorder already holds — the summary is shared-record text,
// and shared-record text follows the base language, not the language of
// whatever produced the change.
//
// It never fails the caller, for the reason identity.BaseLanguageForPrompt
// documents: refusing to file a real change on the record because a settings
// read failed trades a fact for a formatting preference. The failure is logged
// because this returns a string and nothing else, so the caller cannot notice
// a degraded resolve and say so itself.
func technicalSummaryLanguage(ctx context.Context, tx pgx.Tx) textlang.Lang {
	lang, err := identity.BaseLanguageOf(ctx, tx)
	if err != nil {
		slog.WarnContext(ctx, "the installation's base language could not be read; this technical-change summary is English",
			"reason", err)
		return textlang.English
	}
	return textlang.Lang(lang)
}

// technicalSummaryCopy is one language's set of summary sentences. Every field
// is required — a partial set is a silent English fallback in miniature, and
// the census test refuses one. Each verb interpolates change.Value, and the
// two "moved" forms interpolate change.Previous after it; product and provider
// names are never translated, which is why the holes are all there is.
type technicalSummaryCopy struct {
	// mailMoved / mailSet: the mail system moved, or was seen for the first time.
	mailMoved string
	mailSet   string
	// serviceGone / serviceNew: an operated service left the air, or arrived.
	serviceGone string
	serviceNew  string
	// hostingMoved / hostingSet: the hosting provider changed, or was first seen.
	hostingMoved string
	hostingSet   string
	// technologyGone / technologyNew: a product left the stack, or joined it.
	technologyGone string
	technologyNew  string
	// serviceNames names every service key techprofile can classify, because
	// techprofile's own labels are the record's, written in one language, and
	// a sentence in the reader's language must not carry a name in another.
	// The census test holds this map complete against techprofile.ServiceKeys.
	serviceNames map[string]string
	// mailNames covers the two mail fallback labels techprofile writes in
	// prose ("own mail server", "another provider"); a real provider's key is
	// absent here on purpose, so its vendor name passes through untranslated.
	mailNames map[string]string
}

// technicalSummaryByLang is the census, keyed by textlang.Lang so the test can
// walk textlang.Shipped and ask this map directly.
var technicalSummaryByLang = map[textlang.Lang]technicalSummaryCopy{
	textlang.English: {
		mailMoved:      "Mail now runs on %s (was %s)",
		mailSet:        "Mail runs on %s",
		serviceGone:    "%s went offline",
		serviceNew:     "%s is newly online",
		hostingMoved:   "Hosting moved to %s (was %s)",
		hostingSet:     "Hosted at %s",
		technologyGone: "No longer uses %s",
		technologyNew:  "Now uses %s",
		serviceNames: map[string]string{
			techprofile.ServiceWebshop:        "Webshop",
			techprofile.ServiceCustomerPortal: "Customer portal",
			techprofile.ServiceCareers:        "Careers page",
			techprofile.ServiceAPI:            "Public API",
			techprofile.ServiceVPN:            "VPN access",
			techprofile.ServiceMail:           "Own mail infrastructure",
			techprofile.ServiceFileCloud:      "File cloud",
			techprofile.ServiceDevInfra:       "Development infrastructure",
			techprofile.ServiceStatusPage:     "Status page",
			techprofile.ServiceSupport:        "Support portal",
		},
		mailNames: map[string]string{
			techprofile.MailSelfHosted: "their own mail server",
			techprofile.MailOther:      "another provider",
		},
	},
	textlang.German: {
		mailMoved:      "Mail läuft jetzt über %s (vorher %s)",
		mailSet:        "Mail läuft über %s",
		serviceGone:    "%s ist offline gegangen",
		serviceNew:     "%s ist neu online",
		hostingMoved:   "Hosting jetzt bei %s (vorher %s)",
		hostingSet:     "Hosting bei %s",
		technologyGone: "%s wird nicht mehr eingesetzt",
		technologyNew:  "Setzt jetzt %s ein",
		serviceNames: map[string]string{
			techprofile.ServiceWebshop:        "Webshop",
			techprofile.ServiceCustomerPortal: "Kundenportal",
			techprofile.ServiceCareers:        "Karriereseite",
			techprofile.ServiceAPI:            "Öffentliche API",
			techprofile.ServiceVPN:            "VPN-Zugang",
			techprofile.ServiceMail:           "Eigene Mail-Infrastruktur",
			techprofile.ServiceFileCloud:      "Datei-Cloud",
			techprofile.ServiceDevInfra:       "Entwicklungs-Infrastruktur",
			techprofile.ServiceStatusPage:     "Statusseite",
			techprofile.ServiceSupport:        "Support-Portal",
		},
		mailNames: map[string]string{
			techprofile.MailSelfHosted: "eigener Mailserver",
			techprofile.MailOther:      "anderer Anbieter",
		},
	},
	textlang.Vietnamese: {
		mailMoved:      "Mail hiện chạy trên %s (trước đây là %s)",
		mailSet:        "Mail chạy trên %s",
		serviceGone:    "%s đã ngừng hoạt động",
		serviceNew:     "%s vừa xuất hiện trực tuyến",
		hostingMoved:   "Hosting đã chuyển sang %s (trước đây là %s)",
		hostingSet:     "Hosting tại %s",
		technologyGone: "Không còn dùng %s",
		technologyNew:  "Hiện đang dùng %s",
		serviceNames: map[string]string{
			techprofile.ServiceWebshop:        "Cửa hàng trực tuyến",
			techprofile.ServiceCustomerPortal: "Cổng khách hàng",
			techprofile.ServiceCareers:        "Trang tuyển dụng",
			techprofile.ServiceAPI:            "API công khai",
			techprofile.ServiceVPN:            "Truy cập VPN",
			techprofile.ServiceMail:           "Hạ tầng mail riêng",
			techprofile.ServiceFileCloud:      "Đám mây lưu trữ tệp",
			techprofile.ServiceDevInfra:       "Hạ tầng phát triển",
			techprofile.ServiceStatusPage:     "Trang trạng thái",
			techprofile.ServiceSupport:        "Cổng hỗ trợ",
		},
		mailNames: map[string]string{
			techprofile.MailSelfHosted: "máy chủ mail riêng",
			techprofile.MailOther:      "nhà cung cấp khác",
		},
	},
}

// technicalChangeSummary is the sentence a rep reads on the account.
//
// Written per field rather than from a template, because the four fields are
// four different pieces of news: a mail system moving is an IT decision worth a
// call, a careers page appearing is a hiring signal, and a shared phrasing
// would flatten both into "a technical signal changed".
func technicalChangeSummary(lang textlang.Lang, change people.TechnicalChange) (string, bool) {
	said, ok := technicalSummaryByLang[lang]
	if !ok {
		// Unreachable for a language the product ships — the census holds that
		// — but the language comes off a settings row, and answering English
		// beats answering nothing.
		said = technicalSummaryByLang[textlang.English]
	}
	switch change.Field {
	case people.FactMailProvider:
		// The fallback mail labels are prose and follow the language; a real
		// provider's key misses the map and its vendor name passes through.
		value := nameOr(said.mailNames, change.ValueKey, change.Value)
		if change.Kind == people.TechnicalMoved {
			return fmt.Sprintf(said.mailMoved, value, nameOr(said.mailNames, change.PreviousKey, change.Previous)), true
		}
		return fmt.Sprintf(said.mailSet, value), true
	case people.FactOperatedService:
		value := nameOr(said.serviceNames, change.ValueKey, change.Value)
		if change.Kind == people.TechnicalGone {
			return fmt.Sprintf(said.serviceGone, value), true
		}
		return fmt.Sprintf(said.serviceNew, value), true
	case people.FactHostingProvider:
		if change.Kind == people.TechnicalMoved {
			return fmt.Sprintf(said.hostingMoved, change.Value, change.Previous), true
		}
		return fmt.Sprintf(said.hostingSet, change.Value), true
	case people.FactTechnology:
		if change.Kind == people.TechnicalGone {
			return fmt.Sprintf(said.technologyGone, change.Value), true
		}
		return fmt.Sprintf(said.technologyNew, change.Value), true
	default:
		// email_security lands here on purpose. A DMARC policy tightening is a
		// real fact about the company and belongs on the record, but it is not
		// a reason to pick up the phone, and every account publishing one would
		// put a line on every signal list for nobody to act on.
		return "", false
	}
}

// nameOr is the display name a language keeps for a key, and the record's own
// label when it keeps none — a vendor name, which no language translates.
func nameOr(names map[string]string, key, recorded string) string {
	if name, ok := names[key]; ok {
		return name
	}
	return recorded
}
