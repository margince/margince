// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The sentences the approval card shows for a preview's effects and for a
// preview it cannot explain, mirrored from the frontend catalogs. The gate in
// backend/gates/approvalpreviewwords_test.go holds each to the card's own.

import (
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// previewStateUnavailable is the state the card reads every unknown one as.
const previewStateUnavailable = "unavailable"

// EffectSentence is one preview effect as the card words it in lang. An effect
// the card has no sentence for reads as its code, as it does on the card.
func EffectSentence(lang textlang.Lang, effect string) string {
	return catalogSentence(effectSentencesByLang, lang, effect, effect)
}

// PreviewStateSentence is the card's sentence for a preview in state. A state
// the card has no sentence for reads as unavailable.
func PreviewStateSentence(lang textlang.Lang, state string) string {
	unavailable := catalogSentence(previewStateSentencesByLang, lang, previewStateUnavailable, "")
	return catalogSentence(previewStateSentencesByLang, lang, state, unavailable)
}

// PreviewEffects lists every effect the card has a sentence for, sorted, for
// the gate that holds them to the frontend's.
func PreviewEffects() []string {
	return slices.Sorted(maps.Keys(effectSentencesByLang[textlang.English]))
}

// PreviewStates lists every state the card has a sentence for, sorted.
func PreviewStates() []string {
	return slices.Sorted(maps.Keys(previewStateSentencesByLang[textlang.English]))
}

func catalogSentence(catalog map[textlang.Lang]map[string]string, lang textlang.Lang, code, fallback string) string {
	sentences, ok := catalog[lang]
	if !ok {
		sentences = catalog[textlang.English]
	}
	if sentence, ok := sentences[code]; ok {
		return sentence
	}
	return fallback
}

var effectSentencesByLang = map[textlang.Lang]map[string]string{
	textlang.English: {
		"append_recipient_unsubscribe":          "Sending adds a recipient-specific unsubscribe link when the recipient has an applicable preference record. The private link is created at execution, not during review.",
		"apply_company_read":                    "Applies the displayed company fields and evidence. Human-owned evidence remains unchanged; existing column values are retained where shown.",
		"archive_activity_preserve_content":     "Archive this activity and remove it from active work. Its content remains stored; previously sent messages are not recalled.",
		"archive_company_detach_scope":          "Archive this company and its listed domains, classifications, partner program and relationships. Remove its list memberships and tags. Contacts, deals and conversation history remain.",
		"archive_contact_detach_scope":          "Archive this contact and retire the listed email addresses, phone numbers, channel bindings and relationships. Remove the listed memberships and tags. Conversation history remains.",
		"archive_deal_detach_scope":             "Archive this deal and remove the shown stakeholder links, list memberships and tag assignments. Contacts, lists and tags remain stored.",
		"archive_lead_remove_tags":              "Archives this lead and removes all its tags. The record remains available in archived records.",
		"archive_offer_preserve_lines":          "Archive this offer and hide it from the active offer list. Its line items remain stored.",
		"archive_product_preserve_snapshots":    "Archive this product and remove it from the active catalog. Existing offer line snapshots remain unchanged.",
		"archive_project_detach_scope":          "Archive this project and remove the listed stakeholder links, list memberships and tags. Activities, deals and company links remain.",
		"archive_relationship_no_primary":       "Leave the contact without a primary employer. No employer is chosen automatically.",
		"archive_relationship_preserve_records": "Archive this relationship link. The linked records remain stored.",
		"archive_relationship_promote_employer": "Make the shown remaining employment the contact’s primary employer.",
		"archive_template_preserve_offers":      "Archive this template and remove it from template selection, including its use as a default. Existing offers keep their template reference.",
		"assign_offer_number":                   "Assign the offer number when the draft is created.",
		"assign_project_key":                    "Assign a project key when the project is created.",
		"attach_task_recipients":                "Link these recipients to the existing task.",
		"authorize_calendar_invitation":         "Authorizes the proposing host to reserve the slot and queue this calendar invitation to the displayed address. A private reschedule/cancel link is appended to the invitation. Video and reminder settings are shown above. Current calendar access, availability and recipient permissions are checked when executed. Provider confirmation does not prove the guest accepted or received the invitation.",
		"authorize_channel_reply":               "Authorizes the proposing caller to reply once on this conversation to the displayed channel account, with the full message and listed files. Approval does not send immediately. Current channel availability, recipient permissions and file checks still apply.",
		"authorize_message_send":                "Authorizes the proposing caller to send this message once. Approval does not send it immediately or add your signature. Sending still requires current permissions and consent checks.",
		"authorize_website_read":                "Authorizes the proposing caller to read this website using configured external services. Reads may use AI and incur usage charges.",
		"classify_correspondence":               "Classify linked correspondence as business records.",
		"create_activity_record":                "File the activity without sending a message or calendar invitation",
		"create_company_record":                 "Authorizes the proposing caller to create this company once with the displayed values. Current permissions and duplicate identities are checked again.",
		"create_contact_employer":               "Create the missing company and link this contact as an employee.",
		"create_contact_record":                 "Authorizes the proposing caller to create this contact once with the displayed values. Current permissions and duplicate identities are checked again.",
		"create_deal_record":                    "Create the deal in the shown pipeline and stage, with its initial stage history.",
		"create_lead_record":                    "Authorizes the proposing caller to create this lead once with the displayed values. Creation checks current permissions and duplicate identities again.",
		"create_offer_draft":                    "Create a draft offer with the shown terms, line items and totals. No offer is sent.",
		"create_offer_template":                 "Create an offer template with the shown header and footer.",
		"create_product_record":                 "Create the catalog product with the shown price and billing terms. Existing offer lines keep their prices.",
		"create_project_record":                 "Create the project, link the shown company as its customer, and record the initial phase.",
		"create_workspace_visible":              "Makes the new record available to colleagues who have permission to see it.",
		"fill_empty_lead_fields":                "Fills only the empty lead fields shown above. Existing values and the email address stay unchanged; no second lead is created.",
		"geocode_if_enabled":                    "If address geocoding is enabled, queues a coordinate lookup for the displayed address.",
		"import_close_deal":                     "Close the deal using the imported history",
		"marketing_confirmation_unavailable":    "The current contact or consent details prevent a marketing confirmation request. Booking can continue without sending it or granting marketing consent.",
		"member_connectors_reconnect":           "Connected accounts must be reconnected under the surviving member.",
		"member_crm_private_records_move":       "Owned CRM records, including private contacts and companies, become visible to the receiving colleague.",
		"member_email_reserved":                 "Removal cannot be undone here. The original email stays reserved; use Deactivate if this colleague may return.",
		"member_original_attribution":           "Original names remain in history.",
		"member_private_alerts_archived":        "Personal alerts are archived.",
		"member_private_audiences_retained":     "Mail and activities keep their own audience. Activity already shared with a transferred record’s readers may become discoverable to the recipient.",
		"member_source_signed_out":              "The removed identity is signed out everywhere.",
		"member_survivor_access":                "The survivor keeps its roles, teams and sign-in settings, and retains the merged identity’s personal access and correspondence.",
		"member_survivor_inactive":              "The surviving member is deactivated. Reactivate it to restore sign-in and team assignment.",
		"merge_company_preserve_receipts":       "Partner-program and tax-verification records move only when the survivor has none. Otherwise the source copies remain with the archived company.",
		"merge_company_private_readings":        "Each reader keeps their existing survivor summary when both companies have one. Suggestions dismissed on the removed company may be offered again on the survivor.",
		"merge_company_reconcile_lookups":       "Website lookups move to the survivor; duplicate active lookups are canceled. Background company scans for the removed company stop.",
		"merge_records_release_source":          "Moves the displayed records and values to the survivor and archives the source. Existing survivor values take precedence where shown.",
		"merge_tags_release_source":             "Replace the source tag on the shown records with the surviving tag. Where both tags are present, remove only the duplicate source assignment. Archive the source tag and release its name for reuse; it is not retained as an alias.",
		"partner_margin_tier_unset":             "Margin tier: not set. A human sets it on the company’s Partner tab.",
		"preserve_existing_consent":             "The existing consent decision stays unchanged; no new consent proof is recorded.",
		"promote_create_contact":                "Creates a contact with the displayed details.",
		"promote_fill_company_industry":         "Sets the displayed industry on the linked company, which has none yet.",
		"promote_fill_contact":                  "Adds the displayed missing details to the existing contact. Keeps its existing name, title and primary phone.",
		"promote_lead_archive_source":           "Qualifies and archives this lead, linking it to the contact. Keeps the original consent evidence on the lead.",
		"promote_move_consent_links":            "Moves the listed unsubscribe links and communication bases to the contact. Their scope, expiry and revocation remain unchanged.",
		"promote_open_deal_seat_contact":        "Opens the displayed deal and adds the contact as a stakeholder. The lead links to this deal.",
		"promote_propose_company":               "Creates no company from the typed name. The contact shows the name as a company to link or create.",
		"promote_queue_duplicate_review":        "Creates a separate contact and queues the possible duplicate for review.",
		"queue_duplicate_review":                "Creates a duplicate candidate for review alongside the new record.",
		"queue_vat_check":                       "Request a VAT-number check if verification is configured.",
		"rearm_held_message":                    "Schedules the displayed held message to be retried after the delay shown. Current permissions, recipients and communication rules are checked again before sending.",
		"record_confirmation_only":              "Record this confirmation and complete the waiting automation run. No record update or message send is authorized.",
		"record_contact_acquisition":            "Records the displayed acquisition history. This does not grant permission to send messages.",
		"record_meeting_only":                   "Authorizes recording a meeting on the linked records. No calendar invitation or email is sent. Supplied attendee addresses are not stored on the meeting. The host must still have a free slot when it is recorded.",
		"record_meeting_with_consent":           "Authorizes recording the meeting and capturing the displayed transactional consent before reserving the slot. An existing consent decision is preserved. Consent may be recorded even if the slot is no longer available. No calendar invitation is sent.",
		"relink_preserve_other_links":           "Other record links remain. Participants tied to replaced contact links move to the destination contact; matching participant rows merge.",
		"relink_project_retention":              "Filing under this project records a commercial correspondence retention basis. Moving the activity later does not remove that basis.",
		"request_marketing_confirmation":        "Authorizes an attempt to request marketing confirmation at the displayed address. Delivery depends on the configured mail service and current consent checks. This does not subscribe the contact. The submitted marketing wording is not stored as proof; confirmation uses the wording on the confirmation page.",
		"resolve_contact_employer":              "Resolves the employer from the company or work address, reuses or creates the company, and links employment. Uncertain domains wait for review; eligible enrichment follows company settings.",
		"restore_lead_archive_contact":          "Restores the lead as Engaged and removes its qualification links. Archives the contact, its email addresses, phone numbers, channel identities and relationships; removes its tags and list memberships. Activities stay where they are.",
		"restore_lead_preserve_contact":         "Restores the lead as Engaged and removes its qualification links. Keeps the existing contact and its history; removes the link back to this lead. Activities stay where they are.",
		"reuse_contact_employer":                "Link this contact to the existing company.",
		"reuse_existing_record":                 "The source key already identifies this record. The call returns it without creating another record.",
		"send_without_human_signature":          "Sends without a human display name or personal signature. Approval does not make the reviewer the author.",
		"set_template_locale_default":           "Set this template as the default for the shown language.",
		"start_project_delivery":                "Start delivery on eligible linked projects.",
	},
	textlang.German: {
		"append_recipient_unsubscribe":          "Beim Versand wird ein empfängerbezogener Abmeldelink ergänzt, wenn ein entsprechender Präferenzdatensatz besteht. Der private Link wird erst bei der Ausführung erstellt, nicht bei der Prüfung.",
		"apply_company_read":                    "Übernimmt die angezeigten Unternehmensfelder und Belege. Menschlich bestätigte Belege bleiben unverändert; vorhandene Feldwerte bleiben wie angezeigt erhalten.",
		"archive_activity_preserve_content":     "Diese Aktivität archivieren und aus der aktiven Arbeit entfernen. Der Inhalt bleibt gespeichert; bereits gesendete Nachrichten werden nicht zurückgerufen.",
		"archive_company_detach_scope":          "Dieses Unternehmen mit den aufgeführten Domains, Klassifizierungen, dem Partnerprogramm und den Beziehungen archivieren. Listenzugehörigkeiten und Tags entfernen. Kontakte, Deals und Gesprächsverlauf bleiben erhalten.",
		"archive_contact_detach_scope":          "Diesen Kontakt und die aufgeführten E-Mail-Adressen, Telefonnummern, Kanalverknüpfungen und Beziehungen archivieren. Die aufgeführten Listenmitgliedschaften und Tags entfernen. Der Gesprächsverlauf bleibt erhalten.",
		"archive_deal_detach_scope":             "Diesen Deal archivieren und die gezeigten Stakeholder-Verknüpfungen, Listenzugehörigkeiten und Tag-Zuordnungen entfernen. Kontakte, Listen und Tags bleiben gespeichert.",
		"archive_lead_remove_tags":              "Archiviert diesen Lead und entfernt alle seine Tags. Der Datensatz bleibt unter den archivierten Datensätzen verfügbar.",
		"archive_offer_preserve_lines":          "Dieses Angebot archivieren und aus der aktiven Angebotsliste ausblenden. Seine Positionen bleiben gespeichert.",
		"archive_product_preserve_snapshots":    "Dieses Produkt archivieren und aus dem aktiven Katalog entfernen. Gespeicherte Angebotspositionen bleiben unverändert.",
		"archive_project_detach_scope":          "Dieses Projekt archivieren und die aufgeführten Beteiligten-Verknüpfungen, Listenmitgliedschaften und Tags entfernen. Aktivitäten, Deals und Firmenverknüpfungen bleiben erhalten.",
		"archive_relationship_no_primary":       "Der Kontakt bleibt ohne primären Arbeitgeber. Es wird kein Arbeitgeber automatisch ausgewählt.",
		"archive_relationship_preserve_records": "Diese Verknüpfung archivieren. Die verknüpften Datensätze bleiben gespeichert.",
		"archive_relationship_promote_employer": "Die gezeigte verbleibende Anstellung als primären Arbeitgeber des Kontakts festlegen.",
		"archive_template_preserve_offers":      "Diese Vorlage archivieren und aus der Vorlagenauswahl entfernen, auch als Standardvorlage. Bestehende Angebote behalten ihren Vorlagenverweis.",
		"assign_offer_number":                   "Die Angebotsnummer wird beim Erstellen des Entwurfs vergeben.",
		"assign_project_key":                    "Bei der Erstellung einen Projektschlüssel vergeben.",
		"attach_task_recipients":                "Diese Kontakte mit der bestehenden Aufgabe verknüpfen.",
		"authorize_calendar_invitation":         "Autorisiert die vorschlagende organisierende Person, den Zeitraum zu reservieren und diese Kalendereinladung an die angezeigte Adresse zum Versand einzureihen. Der Einladung wird ein privater Link zum Verschieben oder Absagen hinzugefügt. Video- und Erinnerungseinstellungen stehen oben. Kalenderzugriff, Verfügbarkeit und Empfangsberechtigungen werden bei der Ausführung geprüft. Eine Bestätigung des Anbieters belegt weder die Annahme noch den Empfang durch den Gast.",
		"authorize_channel_reply":               "Autorisiert den vorschlagenden Aufrufer, einmal in dieser Unterhaltung mit dem vollständigen Text und den aufgelisteten Dateien an das angezeigte Kanalkonto zu antworten. Die Freigabe versendet noch nichts. Kanalverfügbarkeit, Empfangsberechtigungen und Dateien werden bei der Ausführung erneut geprüft.",
		"authorize_message_send":                "Erlaubt dem vorschlagenden Aufrufer, diese Nachricht einmal zu senden. Die Freigabe sendet sie nicht sofort und fügt deine Signatur nicht hinzu. Beim Senden werden die aktuellen Berechtigungen und Einwilligungen erneut geprüft.",
		"authorize_website_read":                "Erlaubt dem vorschlagenden Aufrufer, diese Website mit den konfigurierten externen Diensten zu lesen. Dabei können KI und nutzungsabhängige Gebühren anfallen.",
		"classify_correspondence":               "Verknüpfte Korrespondenz als Geschäftsbriefe einordnen.",
		"create_activity_record":                "Aktivität ablegen, ohne Nachricht oder Kalendereinladung zu senden",
		"create_company_record":                 "Erlaubt dem vorschlagenden Aufrufer, dieses Unternehmen einmal mit den angezeigten Werten anzulegen. Aktuelle Berechtigungen und doppelte Identitäten werden erneut geprüft.",
		"create_contact_employer":               "Das fehlende Unternehmen erstellen und als Arbeitgeber dieses Kontakts verknüpfen.",
		"create_contact_record":                 "Erlaubt dem vorschlagenden Aufrufer, diesen Kontakt einmal mit den angezeigten Werten anzulegen. Aktuelle Berechtigungen und doppelte Identitäten werden erneut geprüft.",
		"create_deal_record":                    "Den Deal in der angezeigten Pipeline und Phase mit dem ersten Eintrag im Phasenverlauf erstellen.",
		"create_lead_record":                    "Erlaubt dem vorschlagenden Aufrufer, diesen Lead einmal mit den angezeigten Werten anzulegen. Beim Anlegen werden Berechtigungen und doppelte Identitäten erneut geprüft.",
		"create_offer_draft":                    "Einen Angebotsentwurf mit den gezeigten Bedingungen, Positionen und Summen erstellen. Es wird kein Angebot versendet.",
		"create_offer_template":                 "Eine Angebotsvorlage mit der angezeigten Kopf- und Fußzeile erstellen.",
		"create_product_record":                 "Das Katalogprodukt mit dem angezeigten Preis und den Abrechnungsbedingungen erstellen. Bestehende Angebotspositionen behalten ihre Preise.",
		"create_project_record":                 "Das Projekt erstellen, das angezeigte Unternehmen als Kunden verknüpfen und die Anfangsphase dokumentieren.",
		"create_workspace_visible":              "Der neue Datensatz ist mit passenden Leserechten sichtbar.",
		"fill_empty_lead_fields":                "Ergänzt nur die oben gezeigten leeren Lead-Felder. Vorhandene Werte und die E-Mail-Adresse bleiben erhalten; es wird kein zweiter Lead angelegt.",
		"geocode_if_enabled":                    "Wenn die Adress-Geokodierung aktiviert ist, wird eine Koordinatenabfrage für die angezeigte Adresse eingeplant.", //nolint:misspell // "Adress-" is German, as the card's catalog spells it.
		"import_close_deal":                     "Deal anhand des importierten Verlaufs abschließen",
		"marketing_confirmation_unavailable":    "Die aktuellen Kontakt- oder Einwilligungsdaten verhindern eine Marketingbestätigung. Die Buchung kann ohne Versand und ohne Marketingeinwilligung fortgesetzt werden.",
		"member_connectors_reconnect":           "Verbundene Konten müssen beim verbleibenden Mitglied neu verbunden werden.",
		"member_crm_private_records_move":       "Zugewiesene CRM-Datensätze, auch private Kontakte und Unternehmen, werden für das empfangende Mitglied sichtbar.",
		"member_email_reserved":                 "Das Entfernen lässt sich hier nicht rückgängig machen. Die ursprüngliche E-Mail-Adresse bleibt reserviert. Falls das Mitglied zurückkehren könnte, Deaktivieren verwenden.",
		"member_original_attribution":           "Die ursprünglichen Namen bleiben im Verlauf erhalten.",
		"member_private_alerts_archived":        "Persönliche Hinweise werden archiviert.",
		"member_private_audiences_retained":     "E-Mails und Aktivitäten behalten ihre Freigaben. Bereits freigegebene Aktivitäten können mit übertragenen Datensätzen für das neu zuständige Mitglied sichtbar werden.",
		"member_source_signed_out":              "Das entfernte Konto wird überall abgemeldet.",
		"member_survivor_access":                "Das verbleibende Mitglied behält Rollen, Teams und Anmeldeeinstellungen und erhält den persönlichen Zugriff und die Korrespondenz des zusammengeführten Kontos.",
		"member_survivor_inactive":              "Das verbleibende Mitglied ist deaktiviert. Für Anmeldung und Teamzuordnung wieder aktivieren.",
		"merge_company_preserve_receipts":       "Partnerprogramm- und Steuerprüfungsnachweise werden nur übernommen, wenn das Ziel keine hat. Andernfalls bleiben die Quellkopien beim archivierten Unternehmen.",
		"merge_company_private_readings":        "Hat eine Person Zusammenfassungen für beide Unternehmen, bleibt ihre Zusammenfassung des Ziels erhalten. Für die Quelle ausgeblendete Vorschläge können beim Ziel erneut erscheinen.",
		"merge_company_reconcile_lookups":       "Website-Abfragen wechseln zum Ziel; doppelte laufende Abfragen werden abgebrochen. Hintergrundprüfungen für das entfernte Unternehmen enden.",
		"merge_records_release_source":          "Übernimmt die angezeigten Datensätze und Werte in das Ziel und archiviert die Quelle. Vorhandene Zielwerte haben an den angezeigten Stellen Vorrang.",
		"merge_tags_release_source":             "Das Quell-Tag auf den gezeigten Datensätzen durch das verbleibende Tag ersetzen. Wenn beide Tags vorhanden sind, nur die doppelte Quell-Zuordnung entfernen. Das Quell-Tag archivieren und seinen Namen zur erneuten Verwendung freigeben; er bleibt nicht als Alias erhalten.",
		"partner_margin_tier_unset":             "Margenstufe: nicht gesetzt. Ein Mensch legt sie im Partner-Tab des Unternehmens fest.",
		"preserve_existing_consent":             "Die bestehende Einwilligungsentscheidung bleibt unverändert; es wird kein neuer Nachweis erfasst.",
		"promote_create_contact":                "Erstellt einen Kontakt mit den angezeigten Angaben.",
		"promote_fill_company_industry":         "Trägt die angezeigte Branche beim verknüpften Unternehmen ein, das noch keine hat.",
		"promote_fill_contact":                  "Ergänzt die angezeigten fehlenden Angaben am bestehenden Kontakt. Vorhandener Name, Position und Hauptnummer bleiben erhalten.",
		"promote_lead_archive_source":           "Qualifiziert und archiviert diesen Lead und verknüpft ihn mit dem Kontakt. Die ursprünglichen Einwilligungsbelege bleiben am Lead.",
		"promote_move_consent_links":            "Verschiebt die aufgeführten Abmeldelinks und Kommunikationsgrundlagen zum Kontakt. Geltungsbereich, Ablauf und Widerruf bleiben unverändert.",
		"promote_open_deal_seat_contact":        "Eröffnet den angezeigten Deal und fügt den Kontakt als Beteiligten hinzu. Der Lead wird mit diesem Deal verknüpft.",
		"promote_propose_company":               "Legt aus dem eingetippten Namen kein Unternehmen an. Der Kontakt zeigt den Namen als Unternehmen zum Verknüpfen oder Anlegen.",
		"promote_queue_duplicate_review":        "Erstellt einen separaten Kontakt und stellt die mögliche Dublette zur Prüfung bereit.",
		"queue_duplicate_review":                "Legt neben dem neuen Datensatz einen Dublettenvorschlag zur Prüfung an.",
		"queue_vat_check":                       "Prüfung der Umsatzsteuer-ID anfordern, sofern eingerichtet.",
		"rearm_held_message":                    "Plant einen erneuten Versuch der angehaltenen Nachricht nach der angegebenen Wartezeit. Berechtigungen, Empfangende und Kommunikationsregeln werden vor dem Versand erneut geprüft.",
		"record_confirmation_only":              "Diese Bestätigung erfassen und den wartenden Automatisierungslauf abschließen. Es wird keine Datensatzänderung und kein Nachrichtenversand freigegeben.",
		"record_contact_acquisition":            "Erfasst die angezeigten Angaben zur Herkunft. Das erlaubt noch keinen Nachrichtenversand.",
		"record_meeting_only":                   "Erlaubt das Erfassen eines Termins bei den verknüpften Einträgen. Es wird keine Kalendereinladung oder E-Mail versendet. Angegebene Adressen der Teilnehmenden werden nicht am Termin gespeichert. Der Zeitraum muss beim Erfassen für die organisierende Person noch verfügbar sein.",
		"record_meeting_with_consent":           "Autorisiert das Erfassen des Termins und der angezeigten transaktionalen Einwilligung vor der Zeitreservierung. Eine bestehende Entscheidung bleibt erhalten. Die Einwilligung kann erfasst werden, auch wenn der Zeitraum nicht mehr frei ist. Es wird keine Kalendereinladung versendet.",
		"relink_preserve_other_links":           "Andere Verknüpfungen bleiben bestehen. Beteiligte ersetzter Kontaktverknüpfungen wechseln zum Zielkontakt; gleiche Einträge werden zusammengeführt.",
		"relink_project_retention":              "Die Ablage unter diesem Projekt begründet die Aufbewahrung als Geschäftskorrespondenz. Eine spätere Verschiebung hebt diesen Grund nicht auf.",
		"request_marketing_confirmation":        "Autorisiert den Versuch, eine Marketingbestätigung an die angezeigte Adresse anzufordern. Die Zustellung hängt vom konfigurierten Maildienst und aktuellen Einwilligungsprüfungen ab. Der Kontakt wird dadurch nicht angemeldet. Der eingereichte Marketingtext wird nicht als Nachweis gespeichert; maßgeblich ist der Text auf der Bestätigungsseite.",
		"resolve_contact_employer":              "Ermittelt den Arbeitgeber aus dem Unternehmen oder der geschäftlichen Adresse, verwendet ein vorhandenes Unternehmen oder legt es an und verknüpft die Beschäftigung. Unklare Domains warten auf Prüfung; die Anreicherung folgt den Einstellungen des Unternehmens.",
		"restore_lead_archive_contact":          "Stellt den Lead als Engagiert wieder her und entfernt seine Qualifizierungsverknüpfungen. Archiviert den Kontakt, seine E-Mail-Adressen, Telefonnummern, Kanalidentitäten und Beziehungen; entfernt seine Tags und Listenzugehörigkeiten. Aktivitäten bleiben unverändert zugeordnet.",
		"restore_lead_preserve_contact":         "Stellt den Lead als Engagiert wieder her und entfernt seine Qualifizierungsverknüpfungen. Behält den bestehenden Kontakt und seine Historie; entfernt die Rückverknüpfung zu diesem Lead. Aktivitäten bleiben unverändert zugeordnet.",
		"reuse_contact_employer":                "Diesen Kontakt mit dem bestehenden Unternehmen verknüpfen.",
		"reuse_existing_record":                 "Der Quellschlüssel bezeichnet bereits diesen Datensatz. Der Aufruf gibt ihn zurück, ohne einen weiteren anzulegen.",
		"send_without_human_signature":          "Versand ohne menschlichen Anzeigenamen oder persönliche Signatur. Die Freigabe macht die prüfende Person nicht zur absendenden Person.",
		"set_template_locale_default":           "Diese Vorlage als Standard für die angezeigte Sprache festlegen.",
		"start_project_delivery":                "Bei berechtigten verknüpften Projekten die Umsetzung starten.",
	},
	textlang.Vietnamese: {
		"append_recipient_unsubscribe":          "Khi gửi, hệ thống thêm liên kết hủy đăng ký riêng nếu người nhận có bản ghi tùy chọn phù hợp. Liên kết riêng được tạo khi thực hiện, không phải khi xem xét.",
		"apply_company_read":                    "Áp dụng các trường và bằng chứng công ty được hiển thị. Giữ nguyên bằng chứng do người dùng xác nhận và các giá trị hiện có như đã hiển thị.",
		"archive_activity_preserve_content":     "Lưu trữ hoạt động này và loại khỏi công việc đang hoạt động. Nội dung vẫn được lưu; tin nhắn đã gửi không bị thu hồi.",
		"archive_company_detach_scope":          "Lưu trữ công ty này cùng các tên miền, phân loại, chương trình đối tác và mối quan hệ được liệt kê. Xóa tư cách thành viên danh sách và thẻ. Giữ lại liên hệ, cơ hội bán hàng và lịch sử hội thoại.",
		"archive_contact_detach_scope":          "Lưu trữ liên hệ này cùng các địa chỉ email, số điện thoại, liên kết kênh và mối quan hệ được liệt kê. Xóa tư cách thành viên danh sách và thẻ được liệt kê. Lịch sử hội thoại được giữ nguyên.",
		"archive_deal_detach_scope":             "Lưu trữ deal này và xóa các liên kết bên liên quan, tư cách thành viên danh sách và gán nhãn được hiển thị. Liên hệ, danh sách và nhãn vẫn được lưu.",
		"archive_lead_remove_tags":              "Lưu trữ lead này và xóa tất cả thẻ của lead. Bạn vẫn có thể xem bản ghi trong danh sách đã lưu trữ.",
		"archive_offer_preserve_lines":          "Lưu trữ báo giá này và ẩn khỏi danh sách báo giá đang hoạt động. Các hạng mục vẫn được lưu.",
		"archive_product_preserve_snapshots":    "Lưu trữ sản phẩm này và loại khỏi danh mục đang hoạt động. Các bản chụp hạng mục báo giá hiện có không thay đổi.",
		"archive_project_detach_scope":          "Lưu trữ dự án này và xóa các liên kết bên liên quan, tư cách thành viên danh sách và thẻ được liệt kê. Hoạt động, giao dịch và liên kết công ty được giữ nguyên.",
		"archive_relationship_no_primary":       "Để liên hệ không có nhà tuyển dụng chính. Không tự động chọn nhà tuyển dụng.",
		"archive_relationship_preserve_records": "Lưu trữ liên kết quan hệ này. Các bản ghi được liên kết vẫn được lưu.",
		"archive_relationship_promote_employer": "Đặt công ty của quan hệ công tác còn lại được hiển thị làm nhà tuyển dụng chính của liên hệ.",
		"archive_template_preserve_offers":      "Lưu trữ mẫu này và loại khỏi danh sách chọn mẫu, kể cả vai trò mẫu mặc định. Các báo giá hiện có giữ tham chiếu tới mẫu.",
		"assign_offer_number":                   "Cấp số báo giá khi tạo bản nháp.",
		"assign_project_key":                    "Cấp mã dự án khi tạo dự án.",
		"attach_task_recipients":                "Liên kết các liên hệ nhận với công việc hiện có.",
		"authorize_calendar_invitation":         "Cho phép người chủ trì đề xuất giữ khung giờ và xếp lời mời lịch này vào hàng đợi gửi tới địa chỉ hiển thị. Một liên kết riêng để đổi lịch hoặc hủy được thêm vào lời mời. Cài đặt video và nhắc nhở được hiển thị ở trên. Quyền truy cập lịch, thời gian trống và quyền gửi tới người nhận được kiểm tra khi thực hiện. Xác nhận từ nhà cung cấp không chứng minh khách đã chấp nhận hoặc nhận được lời mời.",
		"authorize_channel_reply":               "Cho phép người đề xuất trả lời một lần trong cuộc hội thoại này tới tài khoản kênh hiển thị, với toàn bộ nội dung và các tệp đã liệt kê. Phê duyệt không gửi ngay. Vẫn phải kiểm tra khả năng gửi của kênh, quyền gửi tới người nhận và các tệp tại thời điểm thực hiện.",
		"authorize_message_send":                "Cho phép bên đề xuất gửi tin nhắn này một lần. Phê duyệt không gửi ngay và không thêm chữ ký của bạn. Khi gửi, hệ thống vẫn kiểm tra quyền truy cập và sự đồng ý hiện tại.",
		"authorize_website_read":                "Cho phép bên đề xuất đọc trang web này bằng các dịch vụ bên ngoài đã cấu hình. Việc đọc có thể sử dụng AI và phát sinh phí sử dụng.",
		"classify_correspondence":               "Phân loại thư từ liên quan là hồ sơ kinh doanh.",
		"create_activity_record":                "Lưu hoạt động mà không gửi tin nhắn hoặc lời mời lịch",
		"create_company_record":                 "Cho phép bên đề xuất tạo công ty này một lần với các giá trị hiển thị. Quyền hiện tại và danh tính trùng được kiểm tra lại.",
		"create_contact_employer":               "Tạo công ty còn thiếu và liên kết liên hệ này làm nhân viên.",
		"create_contact_record":                 "Cho phép bên đề xuất tạo bản ghi liên hệ này một lần với các giá trị hiển thị. Quyền hiện tại và danh tính trùng được kiểm tra lại.",
		"create_deal_record":                    "Tạo cơ hội trong quy trình và giai đoạn được hiển thị, kèm lịch sử giai đoạn ban đầu.",
		"create_lead_record":                    "Cho phép bên đề xuất tạo lead này một lần với các giá trị hiển thị. Khi tạo, quyền và danh tính trùng được kiểm tra lại.",
		"create_offer_draft":                    "Tạo bản nháp báo giá với các điều khoản, hạng mục và tổng tiền được hiển thị. Chưa gửi báo giá.",
		"create_offer_template":                 "Tạo mẫu báo giá với nội dung đầu trang và chân trang được hiển thị.",
		"create_product_record":                 "Tạo sản phẩm trong danh mục với giá và điều khoản thanh toán được hiển thị. Các dòng báo giá hiện có giữ nguyên giá.",
		"create_project_record":                 "Tạo dự án, liên kết công ty được hiển thị làm khách hàng và ghi lại giai đoạn ban đầu.",
		"create_workspace_visible":              "Bản ghi mới hiển thị trong tổ chức theo quyền truy cập bản ghi.",
		"fill_empty_lead_fields":                "Chỉ điền các trường trống của lead được hiển thị ở trên. Giá trị hiện có và địa chỉ email được giữ nguyên; không tạo hồ sơ thứ hai.",
		"geocode_if_enabled":                    "Nếu tính năng mã hóa địa lý được bật, hệ thống sẽ xếp hàng tra cứu tọa độ cho địa chỉ hiển thị.",
		"import_close_deal":                     "Đóng cơ hội theo lịch sử đã nhập",
		"marketing_confirmation_unavailable":    "Thông tin liên hệ hoặc sự đồng ý hiện tại ngăn yêu cầu xác nhận tiếp thị. Vẫn có thể đặt lịch mà không gửi yêu cầu hoặc cấp sự đồng ý tiếp thị.",
		"member_connectors_reconnect":           "Cần kết nối lại các dịch vụ dưới thành viên giữ lại.",
		"member_crm_private_records_move":       "Đồng nghiệp nhận có thể xem các hồ sơ CRM được bàn giao, kể cả liên hệ và công ty riêng tư.",
		"member_email_reserved":                 "Không thể hoàn tác việc xóa tại đây. Email ban đầu vẫn được giữ riêng. Dùng Tắt nếu đồng nghiệp này có thể quay lại.",
		"member_original_attribution":           "Lịch sử giữ nguyên tên thành viên thực hiện ban đầu.",
		"member_private_alerts_archived":        "Các cảnh báo cá nhân được chuyển vào kho lưu trữ.",
		"member_private_audiences_retained":     "Email và hoạt động giữ nguyên phạm vi chia sẻ. Thành viên nhận có thể tìm thấy hoạt động đã chia sẻ với các thành viên có quyền xem hồ sơ được bàn giao.",
		"member_source_signed_out":              "Tài khoản bị xóa sẽ đăng xuất ở mọi nơi.",
		"member_survivor_access":                "Thành viên giữ lại giữ nguyên vai trò, nhóm và thiết lập đăng nhập, đồng thời kế thừa quyền truy cập cá nhân và thư từ của tài khoản được gộp.",
		"member_survivor_inactive":              "Thành viên giữ lại đang bị tắt. Bật lại để đăng nhập và thêm vào nhóm.",
		"merge_company_preserve_receipts":       "Hồ sơ chương trình đối tác và xác minh thuế chỉ được chuyển khi công ty giữ lại chưa có. Nếu đã có, bản nguồn vẫn thuộc công ty được lưu trữ.",
		"merge_company_private_readings":        "Nếu một người có bản tóm tắt cho cả hai công ty, bản của công ty giữ lại được giữ nguyên. Đề xuất đã bỏ qua trên công ty bị loại bỏ có thể xuất hiện lại trên công ty giữ lại.",
		"merge_company_reconcile_lookups":       "Các lượt tra cứu trang web được chuyển sang công ty giữ lại; lượt đang chạy trùng lặp bị hủy. Việc kiểm tra nền cho công ty bị loại bỏ dừng lại.",
		"merge_records_release_source":          "Chuyển các hồ sơ và giá trị được hiển thị sang hồ sơ giữ lại rồi lưu trữ hồ sơ nguồn. Giá trị hiện có của hồ sơ giữ lại được ưu tiên ở những mục được chỉ rõ.",
		"merge_tags_release_source":             "Thay nhãn nguồn trên các bản ghi được hiển thị bằng nhãn giữ lại. Nếu đã có cả hai nhãn, chỉ xóa gán nhãn nguồn trùng lặp. Lưu trữ nhãn nguồn và cho phép dùng lại tên; tên không được giữ làm bí danh.",
		"partner_margin_tier_unset":             "Mức hoa hồng đối tác: chưa đặt. Một thành viên đặt mức này ở tab Đối tác của công ty.",
		"preserve_existing_consent":             "Giữ nguyên quyết định đồng ý hiện có; không ghi bằng chứng đồng ý mới.",
		"promote_create_contact":                "Tạo liên hệ với thông tin đã hiển thị.",
		"promote_fill_company_industry":         "Ghi ngành đã hiển thị vào công ty được liên kết, vì công ty chưa có ngành.",
		"promote_fill_contact":                  "Bổ sung thông tin còn thiếu đã hiển thị vào liên hệ hiện có. Giữ tên, chức danh và số điện thoại chính hiện có.",
		"promote_lead_archive_source":           "Xác định lead đủ điều kiện, lưu trữ lead và liên kết với liên hệ. Giữ bằng chứng đồng ý ban đầu ở lead.",
		"promote_move_consent_links":            "Chuyển các liên kết hủy đăng ký và căn cứ liên lạc đã liệt kê sang liên hệ. Giữ nguyên phạm vi, thời hạn và trạng thái thu hồi.",
		"promote_open_deal_seat_contact":        "Mở cơ hội đã hiển thị và thêm liên hệ làm bên liên quan. Lead được liên kết với cơ hội này.",
		"promote_propose_company":               "Không tạo công ty từ tên đã nhập. Liên hệ hiển thị tên này như một công ty cần liên kết hoặc tạo.",
		"promote_queue_duplicate_review":        "Tạo một liên hệ riêng và đưa cặp có thể trùng vào danh sách xem xét.",
		"queue_duplicate_review":                "Tạo đề xuất trùng lặp để kiểm tra cùng với bản ghi mới.",
		"queue_vat_check":                       "Yêu cầu kiểm tra mã số VAT nếu dịch vụ xác minh được cấu hình.",
		"rearm_held_message":                    "Lên lịch thử gửi lại tin nhắn đang giữ sau khoảng chờ hiển thị. Quyền, người nhận và quy tắc liên lạc hiện tại được kiểm tra lại trước khi gửi.",
		"record_confirmation_only":              "Ghi nhận xác nhận này và hoàn tất lượt tự động hóa đang chờ. Không cho phép cập nhật bản ghi hoặc gửi tin nhắn.",
		"record_contact_acquisition":            "Ghi lại thông tin nguồn thu thập hiển thị. Việc này không cấp quyền gửi tin nhắn.",
		"record_meeting_only":                   "Cho phép ghi lại cuộc họp trên các bản ghi được liên kết. Không gửi lời mời lịch hoặc email. Địa chỉ người tham dự được cung cấp không được lưu trong cuộc họp. Khung giờ của người chủ trì phải còn trống khi ghi lại.",
		"record_meeting_with_consent":           "Cho phép ghi cuộc hẹn và sự đồng ý giao dịch hiển thị trước khi giữ khung giờ. Quyết định đồng ý hiện có được giữ nguyên. Sự đồng ý có thể được ghi ngay cả khi khung giờ không còn trống. Không gửi lời mời lịch.",
		"relink_preserve_other_links":           "Các liên kết khác được giữ nguyên. Người tham gia thuộc liên kết liên hệ bị thay thế được chuyển sang liên hệ đích; các mục trùng khớp được gộp.",
		"relink_project_retention":              "Gắn hoạt động với dự án này tạo căn cứ lưu giữ thư từ thương mại. Chuyển hoạt động sau đó không xóa căn cứ này.",
		"request_marketing_confirmation":        "Cho phép thử gửi yêu cầu xác nhận tiếp thị tới địa chỉ hiển thị. Việc gửi phụ thuộc vào dịch vụ thư đã cấu hình và kiểm tra sự đồng ý hiện tại. Hành động này không đăng ký liên hệ. Nội dung tiếp thị đã gửi không được lưu làm bằng chứng; xác nhận sử dụng nội dung trên trang xác nhận.",
		"resolve_contact_employer":              "Xác định nơi làm việc từ công ty hoặc địa chỉ công việc, dùng lại hoặc tạo công ty rồi liên kết việc làm. Tên miền chưa rõ sẽ chờ xem xét; việc bổ sung thông tin tuân theo cài đặt của công ty.",
		"restore_lead_archive_contact":          "Khôi phục lead về trạng thái Đã tương tác và xóa các liên kết xác định đủ điều kiện. Lưu trữ liên hệ, địa chỉ email, số điện thoại, danh tính kênh và các mối quan hệ; xóa thẻ và tư cách thành viên danh sách. Hoạt động giữ nguyên liên kết.",
		"restore_lead_preserve_contact":         "Khôi phục lead về trạng thái Đã tương tác và xóa các liên kết xác định đủ điều kiện. Giữ liên hệ hiện có và lịch sử; xóa liên kết ngược đến lead này. Hoạt động giữ nguyên liên kết.",
		"reuse_contact_employer":                "Liên kết liên hệ này với công ty hiện có.",
		"reuse_existing_record":                 "Khóa nguồn đã xác định bản ghi này. Lệnh trả về bản ghi đó mà không tạo thêm bản ghi.",
		"send_without_human_signature":          "Gửi không kèm tên hiển thị của cá nhân hoặc chữ ký cá nhân. Việc phê duyệt không biến người xem xét thành tác giả.",
		"set_template_locale_default":           "Đặt mẫu này làm mặc định cho ngôn ngữ được hiển thị.",
		"start_project_delivery":                "Bắt đầu triển khai các dự án liên quan đủ điều kiện.",
	},
}

var previewStateSentencesByLang = map[textlang.Lang]map[string]string{
	textlang.English: {
		"stale":             "This record changed after the proposal. Request a new proposal before approving.",
		"incomplete":        "The full change cannot be shown. Request a new proposal before approving.",
		"needs_wider_sight": "This change touches records you cannot open. A colleague who can see them must decide it.",
		"unavailable":       "Approval context is unavailable. Reload or request a new proposal.",
	},
	textlang.German: {
		"stale":             "Dieser Eintrag wurde nach dem Vorschlag geändert. Fordere vor der Freigabe einen neuen Vorschlag an.",
		"incomplete":        "Die vollständige Änderung kann nicht angezeigt werden. Fordere vor der Freigabe einen neuen Vorschlag an.",
		"needs_wider_sight": "Diese Änderung betrifft Datensätze, die du nicht öffnen kannst. Entscheiden muss jemand, der sie sehen kann.",
		"unavailable":       "Der Freigabekontext ist nicht verfügbar. Lade die Seite neu oder fordere einen neuen Vorschlag an.",
	},
	textlang.Vietnamese: {
		"stale":             "Hồ sơ đã thay đổi sau khi đề xuất được tạo. Yêu cầu đề xuất mới trước khi duyệt.",
		"incomplete":        "Không thể hiển thị đầy đủ thay đổi. Yêu cầu đề xuất mới trước khi duyệt.",
		"needs_wider_sight": "Thay đổi này chạm đến các bản ghi bạn không mở được. Một đồng nghiệp xem được chúng phải quyết định.",
		"unavailable":       "Ngữ cảnh phê duyệt không có sẵn. Tải lại hoặc yêu cầu đề xuất mới.",
	},
}
