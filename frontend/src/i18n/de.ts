import type { MessageKey } from "./en";

// German catalog, the A24 default locale. `satisfies` forces exact key
// parity with en at compile time; i18n.test.ts re-checks it at runtime so a
// build without typechecking still fails loudly. Every value is translated from
// en.ts and written to docs/reference/ui-copy-style-de.md.
export const de = {
  "aiAdmin.allowance": "Monatliches KI-Kontingent",
  "aiAdmin.pool":
    "Gemeinsamer Pool des Unternehmens. Kein individuelles Kontingent und keine Ausgabengrenze in Dollar.",
  "aiAdmin.consumption": "{spent} von {total} Tokens verbraucht · {pct} %",
  "aiAdmin.remaining": "{tokens} Tokens verbleibend",
  "aiAdmin.reset": "Wird am {date} UTC zurückgesetzt",
  "aiAdmin.fixed":
    "Ein festes Kontingent für das Unternehmen ersetzt die Berechnung pro Nutzerkonto.",
  "aiAdmin.formula":
    "Aktive Nutzende mit vollem Platz: {users} × {tokens} Tokens pro Nutzerkonto und Monat.",
  "aiAdmin.floor":
    "Ohne berechtigte Nutzende wird das Kontingent für 1 Nutzerkonto berechnet.",
  "aiAdmin.normal": "Im normalen Kontingentbereich",
  "aiAdmin.degraded":
    "Schwelle von 80 % erreicht: Routing auf niedrigere Modellstufen ist aktiv",
  "aiAdmin.queued":
    "Kontingent erreicht: KI im Hintergrund wird zurückgestellt",
  "aiAdmin.policy":
    "Ab 80 % wechselt das Routing auf niedrigere Modellstufen; das Modell kann dasselbe bleiben. Ab 100 % wartet KI-Arbeit im Hintergrund, und interaktive KI nutzt die niedrigste Modellstufe. Die Suchindexierung läuft weiter und zählt zur Nutzung.",
  "aiAdmin.saved": "Kontingent gespeichert",
  "aiAdmin.recovery":
    "Berechtigte Website-Lesevorgänge, Unternehmensscans und Aufbauvorgänge von Stilprofilen werden beim nächsten Abgleich ausführbar, in der Regel innerhalb einer Minute. Ob sie abgeschlossen werden, hängt von der Worker-Kapazität, den aktuellen Berechtigungen und der Verfügbarkeit des Anbieters ab. Andere Hintergrundläufe behalten ihren normalen Zeitplan.",
  "aiAdmin.edit": "Kontingent bearbeiten",
  "aiAdmin.perUser": "Tokens pro vollem Platz und Monat",
  "aiAdmin.range": "Ganze Tokens, von 1 bis 1.000.000.000.000.",
  "aiAdmin.company": "Fester Gesamtwert für das Unternehmen (optional)",
  "aiAdmin.overrideHint":
    "Leer lassen, um die Berechnung pro Nutzerkonto zu verwenden. Der Wert pro Nutzerkonto bleibt erhalten.",
  "aiAdmin.routingStale":
    "Modellzuordnungen wurden während der Bearbeitung geändert",
  "aiAdmin.stale": "Dieses Kontingent wurde während der Bearbeitung geändert",
  "aiAdmin.staleHelp":
    "Brich ab und öffne den Editor erneut, um die aktuellen Einstellungen zu verwenden.",
  "aiAdmin.failed": "Änderung nicht übernommen",
  "aiAdmin.preview": "Auswirkungen vorab ansehen",
  "aiAdmin.previewHint":
    "Vorschau auf Basis der aktuellen Bedingungen. Beim Speichern werden Einstellungen und Nutzung erneut geprüft; die Vorschau reserviert keine Kapazität und ruft kein Modell auf.",
  "aiAdmin.features": "KI nach Tätigkeit",
  "aiAdmin.featuresWithheld":
    "Nur Nutzende mit Leserecht für die KI-Diagnose und für das KI-Kontingent sehen, welche Funktionen gerade aktiv sind.",
  "aiAdmin.save": "Kontingent speichern",
  "aiAdmin.cancel": "Abbrechen",
  "aiAdmin.prospective":
    "Von der aktuellen Richtlinie gewähltes Modell. Tatsächliche Aufrufe können ausweichen oder fehlschlagen. Anbieterstatus und tatsächlich verwendetes Modell sind hier nicht zu sehen.",
  "aiAdmin.calls": "Tatsächliche Modellaufrufe ansehen",
  "aiAdmin.website": "Website-Lesevorgänge",
  "aiAdmin.scans": "Unternehmensscans",
  "aiAdmin.voice": "Aufbau von Stilprofilen",
  "aiAdmin.waiting": "Erfasste Arbeit, die auf das Kontingent wartet",
  "aiAdmin.coverage":
    "Die Zahlen umfassen nur dauerhaft gespeicherte Website-Lesevorgänge, Unternehmensscans und Aufbauvorgänge von Stilprofilen. Nicht jeder geplante KI-Lauf wird gezählt, und die Zahlen garantieren nicht, dass eine Anfrage noch ausgeführt werden darf.",
  "aiAdmin.unavailable": "Nicht verfügbar",
  "aiAdmin.impact.blocked": "Wartet auf Kontingent",
  "aiAdmin.impact.model": "Anderes Modell gewählt",
  "aiAdmin.impact.decision": "Entscheidungsmodell geändert",
  "aiAdmin.impact.fallback": "Ausweichkette geändert",
  "aiAdmin.impact.unconfigured": "Kein Modell konfiguriert",
  "aiAdmin.impact.exempt": "Läuft über das Kontingent hinaus weiter",
  "aiAdmin.impact.same": "Gleiche Modellauswahl",
  "aiAdmin.activity": "Tätigkeit",
  "aiAdmin.model": "Von der Richtlinie gewähltes Modell",
  "aiAdmin.cloud": "Cloud-Anbieter",
  "aiAdmin.endpoint": "Konfigurierter Endpunkt; Standort nicht geprüft",
  "aiAdmin.editBinding": "Gemeinsame Zuordnung bearbeiten",
  "aiAdmin.decisionFirst":
    "Zuerst Entscheidungsmodell ({provider} · {model} · {processing}) → dann {ladder}",
  "aiAdmin.decisionSkip.unbound":
    "Entscheidungsmodell nicht genutzt: keines zugeordnet.",
  "aiAdmin.decisionSkip.uncertified":
    "Entscheidungsmodell nicht genutzt: für diese Aktivität nicht zertifiziert.",
  "aiAdmin.decisionSkip.local_only":
    "Entscheidungsmodell nicht genutzt: diese Aktivität nimmt nur einen lokalen Entscheidungsanbieter.",
  "aiAdmin.effect": "Auswirkung",
  "aiAdmin.advanced": "Erweitert: gemeinsame Modellzuordnungen",
  "aiAdmin.unused":
    "Von den aktuell ausgelieferten Tätigkeiten nicht genutzt: {tiers}",
  "aiAdmin.inputRate": "Eingabe {input} pro Million Tokens",
  "aiAdmin.rates": "Eingabe {input} · Ausgabe {output} pro Million Tokens",

  "brief.weekly.tasksCompleted": "Aufgaben erledigt",
  "teamweekly.noPriority": "Die erfassten Kennzahlen zeigen keine Priorität",
  "teamweekly.basis":
    "Mitgliederübersichten der erfassten Woche. Aktuelle Pläne und Worklists zeigen die heutigen Zuständigkeiten.",
  "brief.weekly.noMeetings": "Keine erfasst",
  "brief.weekly.noLeads": "Keine eingegangen",
  "brief.weekly.noCommitments": "Keine fällig",
  "brief.weekly.basis":
    "Erfasste CRM-Arbeit dieser abgeschlossenen Woche. Fehlende Datensätze belegen keine Inaktivität.",
  "home.receipt.date": "Abschlussdatum: {before} → {after}",
  "home.receipt.undated": "Kein Datum",
  "home.receipt.confidence": "Forecast-Konfidenz aktualisiert",
  "home.receipt.forecast": "Forecast-Kategorie geändert",
  "home.task.yours": "Deine Aufgabe: {action}",
  "home.change.unknown": "Unbekannt, wer die Änderung vorgenommen hat.",
  "home.change.stageUnknown": "Unbekannte Phase",
  "home.change.unidentified": "Unbekannt",
  "home.change.by": "Geändert von: {actor}",
  "brief.focus.inQueue": "In der Worklist",
  "brief.focus.position": "{at} von {count}",
  "worklist.bandCount_one": "{count} Eintrag",
  "worklist.bandCount_other": "{count} Einträge",
  "brief.focus.context": "Details anzeigen",
  "brief.focus.back": "Zurück zum Fokus",
  "brief.queue.back": "Zurück zur Worklist",
  "brief.queue.title": "Worklist",
  "brief.queue.show": "Worklist einblenden",
  "brief.queue.hide": "Worklist ausblenden",
  "brief.focus.urgentRemaining_one":
    "{count} weiterer dringender Eintrag in der Worklist",
  "brief.focus.urgentRemaining_other":
    "{count} weitere dringende Einträge in der Worklist",
  "brief.focus.remaining": "Weitere Prioritäten in der Worklist: {count}",
  "brief.team.planUnavailable":
    "Für {name} ist kein aktueller Plan für dich sichtbar.",
  "brief.team.noCommitments": "{name} hat in diesem Plan keine Zusagen.",
  "brief.team.outcomes":
    "Gewonnen: {won} · Verloren: {lost} · Deals bewegt: {moved} · Leads zugewiesen: {leads}",
  "brief.schedule.unavailable": "Der Kalender wurde nicht geladen.",
  "brief.schedule.more":
    "Lade weitere Agenda-Einträge, um die übrigen Termine zu sehen.",
  "brief.readings.riskPartial": "Nur bekannter Wert · nicht alles geprüft",
  "brief.readings.unpricedCount_one": "1 Deal ohne Preis · nicht enthalten",
  "brief.readings.unpricedCount_other":
    "{count} Deals ohne Preis · nicht enthalten",
  "brief.coverage.source.generic": "Weitere Arbeit",
  "brief.coverage.source.weekly_commitment": "Wochenzusagen",
  "brief.coverage.source.batch": "Gruppierte Arbeit",
  "brief.coverage.source.introduction_request": "Vorstellungsanfragen",
  "brief.coverage.source.automation_run": "Automatisierungsfehler",
  "brief.coverage.source.undelivered": "Nicht gesendete E-Mails",
  "brief.coverage.source.bounce": "Unzustellbare E-Mails",
  "brief.coverage.source.ai_work_health": "Automatisierungsprüfungen",
  "brief.coverage.source.capture_health": "Postfachverbindungen",
  "brief.coverage.source.failed_approval":
    "Fehlgeschlagene freigegebene Aktionen",
  "brief.coverage.source.relationship_decay": "Ruhende Beziehungen",
  "brief.coverage.source.meeting_outcome": "Termin-Nachbereitung",
  "brief.coverage.source.meeting": "Anstehende Termine",
  "brief.coverage.source.deal_at_risk": "Markierte Deals",
  "brief.coverage.source.lead_response": "Zugewiesene Leads",
  "brief.coverage.source.customer_waiting": "Unbeantwortete Nachrichten",
  "brief.coverage.source.conversation_claim": "Kundenzusagen",
  "brief.coverage.source.brief_item": "Deal-Updates",
  "brief.coverage.source.dedupe_candidate": "Mögliche Duplikate",
  "brief.team.commitmentRate": "Erledigte fällige Zusagen: {done} von {total}.",
  "brief.team.meetingRate":
    "Termine mit erfasstem nächsten Schritt: {done} von {total}.",
  "brief.digest.messages": "Synchronisierte E-Mails",
  "brief.team.week": "Woche ab",
  "brief.week.lostLabel": "Verloren",
  "brief.feed.refreshFailed":
    "Der Morgenbericht wurde nicht aktualisiert. Angezeigt wird die zuletzt geladene Arbeit.",
  "brief.team.saveResponse": "Antwort speichern",
  "brief.team.response": "Antwort auf die Hilfeanfrage",
  "brief.team.planFor": "Aktueller Plan · {name}",
  "brief.team.plan": "Aktuellen Plan prüfen",
  "brief.approval.approve": "Freigeben",
  "brief.approval.email": "E-Mail freigeben",
  "brief.coverage.source.approval": "Vorschläge",
  "brief.coverage.source.task": "Aufgaben",
  "brief.coverage.source.dsr": "Datenschutzanfragen",
  "brief.coverage.source.notice_case": "Datenschutzhinweise",
  "brief.coverage.source.notice": "Hinweise",
  "brief.weekly.learnings.caveat":
    "Diese Beobachtungen beschreiben Zusammenhänge in erfasster Arbeit und belegen nicht, was das Ergebnis verursacht hat.",
  "brief.plan.open": "Wochenplan öffnen",
  "worklist.source.weekly_commitment": "Wochenzusage",
  "worklist.untitled.weekly_commitment": "Wochenzusage",
  "brief.plan.select": "Deal, Lead, Kontakt, Unternehmen oder Projekt suchen",
  "brief.plan.period": "Aktueller Plan · Woche vom {date}",
  "brief.forecast.period":
    "Forecast-Zeitraum: {start} bis {end}. Mit diesem Wochenrückblick gespeichert.",
  "brief.team.none":
    "Keine Teams verfügbar. Lege ein Team in den Einstellungen an.",
  "brief.week.supporting": "Kennzahlen, Forecast und Beobachtungen",
  "brief.coverage.retry": "Morgenbericht aktualisieren",
  "brief.reply.owed": "Kontakt wartet auf deine Antwort",
  "brief.createdAt": "Morgenbericht erstellt am {when}",
  "brief.updatedAt": "Agenda aktualisiert am {when}",
  "brief.changes.superseded":
    "Dieser Deal wurde erneut geändert. Öffne den Deal, um seinen aktuellen Stand zu prüfen.",
  "brief.changes.title": "Für dich vorgenommene Änderungen",
  "brief.changes.accept": "Annehmen",
  "brief.changes.accepted": "Angenommen",
  "brief.changes.undone": "Rückgängig gemacht",
  "brief.changes.empty":
    "In den letzten 24 Stunden wurden keine Änderungen in deinem Auftrag vorgenommen.",
  "brief.updates.title": "Updates",
  "brief.task.undated": "Kein Fälligkeitsdatum",
  "brief.readings.summary": "Arbeitsübersicht",
  "brief.readings.unpriced": "Ohne Preis",
  "brief.readings.noDealWork": "Keine",
  "brief.readings.noDealWorkWhy": "Heute kein Deal markiert",
  "brief.readings.unavailable": "Nicht gezählt",
  "brief.readings.unavailable.urgent": "Quellen nicht verfügbar",
  "brief.readings.unavailable.meetings": "Kalender nicht verfügbar",
  "brief.readings.unavailable.leads": "Lead-Quelle nicht verfügbar",
  "brief.readings.unavailable.decisions": "Quelle nicht verfügbar",
  "brief.feed.incomplete":
    "Keine Einträge geladen. Ein Teil der Arbeit konnte nicht geprüft werden.",
  "brief.feed.fullWorklist": "Vollständige Worklist öffnen",
  "brief.week.workRecorded": "Diese Woche wurde Arbeit erledigt.",
  "brief.week.leads_one": "{count} Lead zugewiesen.",
  "brief.week.leads_other": "{count} Leads zugewiesen.",
  "brief.week.responses_one":
    "{count} Lead innerhalb der Zielzeit beantwortet.",
  "brief.week.responses_other":
    "{count} Leads innerhalb der Zielzeit beantwortet.",
  "brief.week.lost_one": "{count} Deal verloren.",
  "brief.week.lost_other": "{count} Deals verloren.",
  "brief.row.details": "Details",
  "brief.glance.introTeam": "Der Tag deines Teams im Überblick.",
  "teamweekly.focus.deals_at_risk": "Rettung gefährdeter Deals",
  "teamweekly.headline.partial":
    "Unvollständige Abdeckung. Diese Zahlen betreffen die unten gezählten Mitglieder.",
  "teamweekly.headline.unmeasured":
    "Für diese Woche liegen keine Snapshots der Mitglieder vor. Die Leistung wird nicht gemessen.",
  "worklist.lead.noTarget": "keine Zielzeit für die Antwort festgelegt",
  "worklist.lead.lastTouch": "letzte Aktivität {date}",
  "worklist.deal.omitted": "nicht im Forecast",
  "worklist.deal.provisional":
    "prognostizierter Abschluss {date} (unbestätigt)",
  "brief.readings.riskBasis":
    "Erwarteter Wert · Deals ohne Preis nicht enthalten",
  "brief.readings.risk": "Gefährdeter Umsatz",
  "brief.glance.introTeamWeekly": "Prüfe die erfasste Woche deines Teams.",
  "brief.feed.teamTitle": "Prioritäten im Team",
  "theme.toDark": "Dunkles Design",
  "theme.toLight": "Helles Design",
  // Die drei Auswahlmöglichkeiten fürs Erscheinungsbild: das Kontomenü zeigt
  // alle gleichzeitig, also heißt jede so, wie sie IST, und nicht danach, was
  // ein Klick tut. Die beiden Labels darüber bleiben die Namen des reinen
  // Icon-Schalters auf Anmeldung und Onboarding. "System" benennt das Gerät,
  // dessen Einstellung übernommen wird, kein drittes Aussehen.
  "theme.light": "Hell",
  "theme.dark": "Dunkel",
  "theme.system": "System",

  "trust.accept": "Annehmen",
  "trust.edit": "Bearbeiten",
  "trust.dismiss": "Ausblenden",
  "trust.save": "Speichern",
  "trust.typedByYou": "Von dir eingetragen",
  "trust.typedByHuman": "Von einer Person eingetragen",
  "trust.typedByBuyer": "Von der Käuferseite eingetragen",
  "trust.typedByPrefix": "Eingetragen von",
  "trust.loggedInByVia": "In {via} erfasst von {name}",
  "trust.loggedInBy": "Erfasst von {name}",
  "trust.sourceUnknown": "Quelle nicht erfasst",
  "trust.agentTag": "Automatisiert durch {agent}",
  "trust.agentUnnamed": "Automatisiert durch einen Agenten",
  "trust.systemTag": "Systemaufgabe {job}",
  "trust.systemUnnamed": "Systemaufgabe",
  "trust.connectorTag": "Über {connector}",
  "trust.dismissed": "Vorschlag ausgeblendet",
  "trust.stagedProposal": "Vorgeschlagener Wert",
  "trust.resolvedValue": "Übernommener Wert",
  "trust.editValue": "{description} bearbeiten",
  "trust.evidenceFrom": "Beleg aus {source}",
  "trust.evidenceLine_one": "Zeile {lines}",
  "trust.evidenceLine_other": "Zeilen {lines}",

  "history.created": "(angelegt)",
  "history.oldValue": "Vorheriger Wert",
  "history.newValue": "Neuer Wert",
  "history.cleared": "(geleert)",
  "history.passport": "Agent-Passport",
  "history.empty": "Keine Änderungen erfasst",
  "history.fieldEmpty":
    "Beim Anlegen festgelegt und nie geändert. Das Audit-Log verzeichnet keine Bearbeitungen.",
  "history.filterEmpty": "Keine Änderungen entsprechen diesem Filter.",
  "history.clearFilter": "Filter zurücksetzen",
  "history.allFields": "Alle Felder",
  "history.actorAll": "Alle",
  "history.actorHuman": "Mensch",
  "history.actorAgent": "Agent",
  "history.tabChanges": "Nach Änderung",
  "history.tabFields": "Nach Feld",
  "history.undo.action": "Rückgängig machen",
  "history.undo.redo": "Erneut anwenden",
  "history.undo.busy": "Änderung wird rückgängig gemacht…",
  "history.undo.confirmTitle": "Diese Änderung rückgängig machen?",
  "history.undo.confirmEdgeBody":
    "Damit ändert sich die Verknüpfung mit {other}. Beide Datensätze bleiben bestehen, nur die Verknüpfung zwischen ihnen ändert sich.",
  "history.undo.confirmBody_one":
    "{count} Feld wird auf seinen Wert vor dieser Änderung zurückgesetzt:",
  "history.undo.confirmBody_other":
    "{count} Felder werden auf ihren Wert vor dieser Änderung zurückgesetzt:",
  "history.undo.versionSkew":
    "Der Datensatz wurde geändert, während er geöffnet war. Der Verlauf wurde neu geladen. Prüfe die Änderung erneut, bevor du sie rückgängig machst.",
  "history.undo.noBeforeImage":
    "Diese Änderung hat die vorherigen Werte nicht erfasst und lässt sich daher nicht rückgängig machen.",
  "history.undo.notReplayable":
    "Diese Art von Änderung lässt sich nicht rückgängig machen.",
  "history.undo.unsupportedRecordType":
    "Änderungen an diesem Datensatztyp lassen sich nicht rückgängig machen.",
  "history.undo.superseded":
    "Diese Felder wurden seitdem geändert, daher lässt sich diese Änderung nicht rückgängig machen.",
  "history.undo.behindErasureBoundary":
    "Diese Änderung liegt vor einer Löschung, und ihre Werte wurden endgültig gelöscht.",
  "history.undo.alreadyUndone":
    "Diese Änderung wurde bereits rückgängig gemacht.",
  "history.undo.notRestorableByThisPath":
    "Änderungen an diesen Feldern lassen sich nicht rückgängig machen.",
  "history.undo.recordArchived":
    "Der Datensatz ist archiviert und lässt keine Änderungen zu, daher lässt sich diese Änderung nicht rückgängig machen.",
  "history.undo.nullUnwritable":
    "Diese Änderung lässt sich nicht rückgängig machen, weil dabei ein Pflichtfeld leer würde.",
  "history.undo.notWritableByCaller":
    "Du hast keine Berechtigung, diese Felder zu ändern.",
  "history.undo.edgeRelinkUnsupported":
    "Eine entfernte Verknüpfung lässt sich nicht rückgängig machen. Lege sie auf diesem Datensatz erneut an.",
  "history.reversal.collapsed":
    "Änderung von {actor}, rückgängig gemacht von {undoer}",
  "history.reversal.collapsedSelf":
    "{actor} hat die eigene Änderung rückgängig gemacht",
  "history.reversal.partly":
    "Änderung von {actor}, teilweise rückgängig gemacht von {undoer}",
  "history.reversal.partlySelf":
    "{actor} hat die eigene Änderung teilweise rückgängig gemacht",
  "history.reversal.net": "Ergebnis: unverändert",
  "history.reversal.stillChanged": "Weiterhin geändert",
  "history.reversal.expand": "Beide Änderungen anzeigen",
  "history.reversal.collapse": "Ausblenden",
  "history.reversal.undoneBy": "Rückgängig gemacht von {undoer}",
  "history.reversal.unpaired": "Macht eine frühere Änderung rückgängig",
  "history.edge.marker": "Verknüpfung",
  "history.field.address": "Adresse",
  "history.field.admission": "Eingangsprüfung der Domain",
  "history.field.admission_reason": "Grund der Eingangsprüfung",
  "history.field.admission_source": "Quelle der Eingangsprüfung",
  "history.field.amount_minor": "Wert",
  "history.field.bounce": "Unzustellbarkeit",
  "history.field.capture_question": "Erfassungsfrage",
  "history.field.channel_identity": "Kanalkonto",
  "history.field.channel_username": "Nutzername im Kanal",
  "history.field.cohort_linked": "Verknüpfte Nachrichten",
  "history.field.cohort_promoted": "Zugeordnete Nachrichten",
  "history.field.corrected": "Ergebnis korrigiert",
  "history.field.disposition": "Einstufung",
  "history.field.domain": "Domain",
  "history.field.expected_arr_minor": "Erwarteter ARR",
  "history.field.assignee_id": "Zugewiesen an",
  "history.field.body": "Notizen",
  "history.field.emails": "E-Mail-Adressen",
  "history.field.nudge_dismissal": "Hinweis ausgeblendet",
  "history.field.phones": "Telefonnummern",
  "history.field.meeting_status": "Ergebnis des Termins",
  "history.field.candidate_company_key": "Zugeordnetes Unternehmen",
  "history.field.communication_basis": "Rechtsgrundlage",
  "history.field.company_name": "Unternehmensname",
  "history.field.confirm_submission": "Art der Bestätigung",
  "history.field.currency": "Währung",
  "history.field.decided_by_level": "Entschieden von",
  "history.field.description": "Beschreibung",
  "history.field.display_name": "Name",
  "history.field.domains": "Domains",
  "history.field.due_at": "Fällig",
  "history.field.email": "E-Mail",
  "history.field.ended_at": "Beendet",
  "history.field.expected_close_date": "Voraussichtlicher Abschluss",
  "history.field.first_name": "Vorname",
  "history.field.forecast_category": "Forecast-Kategorie",
  "history.field.full_name": "Name",
  "history.field.fx_rate_date": "Datum des Wechselkurses",
  "history.field.fx_rate_to_base": "Wechselkurs",
  "history.field.industry": "Branche",
  "history.field.is_done": "Erledigt",
  "history.field.last_name": "Nachname",
  "history.field.legal_name": "Rechtlicher Name",
  "history.field.lifecycle": "Lebenszyklus",
  "history.field.lifted_by": "Aufgehoben von",
  "history.field.lifted_by_level": "Aufgehoben auf Ebene",
  "history.field.lifted_suppression": "Aufgehobene Sperre",
  "history.field.linkedin_url": "LinkedIn-URL",
  "history.field.lost_reason": "Verlustgrund",
  "history.field.name": "Name",
  "history.field.note": "Notiz",
  "history.field.occurred_at": "Zeitpunkt",
  "history.field.company_id": "Unternehmen",
  "history.field.owner_id": "Zuständig",
  "history.field.provider_claims_received": "Erhaltene Anbieteraussagen",
  "history.field.reachability": "Erreichbar",
  "history.field.reply_verdict": "Ergebnis der Antwort",
  "history.field.reply_verdict_by": "Ergebnis der Antwort von",
  "history.field.research_claims_accepted": "Übernommene Rechercheaussagen",
  "history.field.scope": "Umfang",
  "history.field.stopped": "Gestoppt",
  "history.field.stops_carried": "Übernommene Kontaktsperren",
  "history.field.submission_decision": "Entscheidung zur Einreichung",
  "history.field.vat_checked_at": "USt-ID geprüft",
  "history.field.vat_consultation_number": "Abfragenummer der USt-ID-Prüfung",
  "history.field.vat_number": "USt-ID",
  "history.field.vat_registered_address":
    "Im USt-Register hinterlegte Anschrift",
  "history.field.vat_registered_name": "Im USt-Register hinterlegter Name",
  "history.field.vat_requested": "USt-ID-Prüfung angefordert",
  "history.field.vat_status": "USt-Status",
  "history.field.visibility": "Sichtbarkeit",
  "history.field.parent_company_id": "Muttergesellschaft",
  "history.field.partner_attribution": "Partnerzuordnung",
  "history.field.partner_company_id": "Partner",
  "history.field.project_id": "Projekt",
  "history.field.qualifying_event": "Qualifizierendes Ereignis",
  "history.field.reason": "Grund",
  "history.field.recorded_at_level": "Erfasst auf Ebene",
  "history.field.relationship_types": "Beziehungsarten",
  "history.field.remind_at": "Erinnerung",
  "history.field.resolved_category": "Nachrichtenkategorie",
  "history.field.score": "Score",
  "history.field.score_override_reason": "Grund für die Score-Anpassung",
  "history.field.size_band": "Größe",
  "history.field.social": "Social-Profile",
  "history.field.source": "Quelle",
  "history.field.started_at": "Beginn",
  "history.field.status": "Status",
  "history.field.subject": "Betreff",
  "history.field.submission_id": "Einreichung",
  "history.field.suppression_kind": "Art der Sperre",
  "history.field.target_end_date": "Geplantes Ende",
  "history.field.title": "Position",
  "history.field.wait_until": "Wartet bis",
  "history.field.commercial_motion": "Geschäftsart",
  "history.field.priority": "Priorität",
  "history.field.acquisition_source": "Akquisequelle",
  "history.emptyList": "Nichts festgelegt",

  "confidence.high": "hoch",
  "confidence.med": "mittel",
  "confidence.low": "niedrig",

  "autonomy.auto": "automatisch",
  "autonomy.confirm": "erst Freigabe",

  "nav.brief": "Startseite",
  "nav.contacts": "Kontakte",
  "nav.companies": "Unternehmen",
  "nav.leads": "Leads",
  "nav.deals": "Deals",
  "nav.analytics": "Analysen",
  "nav.settings": "Einstellungen",
  "nav.automations": "Automatisierungen",
  "nav.group.records": "Datensätze",
  "nav.group.work": "Arbeit",
  "nav.group.intelligence": "Auswertung",
  "nav.offers": "Angebot",
  "nav.share": "Teilen",
  "nav.search": "Suchergebnisse",
  "nav.tags": "Tags",

  "shell.railAria": "Hauptnavigation",
  "shell.skipToContent": "Zum Inhalt springen",
  "shell.logoAria": "Margince",
  "shell.companyLogoAria": "Startseite von {company}, läuft mit Margince",
  "shell.poweredBy": "Läuft mit Margince",
  "shell.poweredByPrefix": "Läuft mit",
  "shell.beta": "Beta",
  "shell.searchEverything": "Suchen oder Margince fragen",
  "shell.breadcrumbAria": "Navigationspfad",
  "shell.license.none": "Keine Lizenz",
  "shell.license.refused": "Lizenz abgelehnt",
  "shell.signOutAria": "Abmelden",
  "shell.signOutErr": "Abmeldung fehlgeschlagen",
  "shell.collapse": "Seitenleiste einklappen",
  "shell.expand": "Seitenleiste ausklappen",
  "shell.accountAria": "Nutzerkonto",
  "shell.theme": "Design",
  "shell.more": "Mehr",
  "shell.unknownPage": "Nicht gefunden",
  "shell.closeMenu": "Schließen",
  "shell.agent.scope": "Margince liest nur, was du sehen kannst.",
  "shell.capture.importing": "Postfachverlauf wird importiert",
  "shell.capture.share": "{percent} · {scanned} von {total} Nachrichten",
  "shell.capture.count": "Bisher gelesene Nachrichten: {scanned}",
  "shell.capture.open": "Import öffnen",
  "shell.navBackApp": "Zurück zur App",
  "shell.navBack": "Zurück",
  "shell.navBackTo": "Zurück zu {name}",
  "shell.sectionSwitch": "{name}: Bereich wechseln",
  "attention.selected": "{n} ausgewählt",
  "locale.name.en": "English",
  "locale.name.de": "Deutsch",
  "locale.name.vi": "Tiếng Việt",
  "locale.switchLabel": "Sprache",

  "screen.pending": "Diese Ansicht ist noch nicht verfügbar.",

  "ext.notFound":
    "In dieser Installation ist keine Erweiterung namens „{name}“ aktiviert.",

  "search.placeholder":
    "Kontakte, Unternehmen, Deals, Projekte, Produkte, Aktivitäten, Leads durchsuchen…",
  "search.prompt": "Gib einen Suchbegriff ein.",
  "search.empty": "Keine Treffer für „{q}“.",
  "search.group.contact": "Kontakte",
  "search.group.company": "Unternehmen",
  "search.group.deal": "Deals",
  "search.group.project": "Projekte",
  "search.group.product": "Produkte",
  "search.group.offerTemplate": "Angebotsvorlagen",
  "search.group.activity": "Aktivitäten",
  "search.group.lead": "Leads",
  "search.group.tag": "Tags",
  "search.kind.contact": "Kontakt",
  "search.kind.company": "Unternehmen",
  "search.kind.deal": "Deal",
  "search.kind.project": "Projekt",
  "search.kind.product": "Produkt",
  "search.kind.offerTemplate": "Angebotsvorlage",
  "search.kind.activity": "Aktivität",
  "search.kind.lead": "Lead",
  "search.kind.tag": "Tag",
  "search.filter.label": "Nur anzeigen",
  "search.filter.all": "Alle",
  "search.pending": "Wird gesucht…",
  "search.tag.carriedBy_one": "{count} Datensatz mit diesem Tag",
  "search.tag.carriedBy_other": "{count} Datensätze mit diesem Tag",
  "search.tier.mirrored": "Aus einem verbundenen System",
  "search.tier.unverified": "Nicht verifiziert",

  "context.recentTouches": "Letzte Aktivitäten",
  "context.openTasks": "Offene Aufgaben",
  "context.relatedContacts": "Zugehörige Kontakte",
  "context.relatedCompanies": "Zugehörige Unternehmen",
  "context.relatedProjects": "Zugehörige Projekte",
  "context.whoKnows": "Verbindungen im Team",
  "context.relatedDeals": "Zugehörige Deals",
  "context.title": "Zugehörige Datensätze",
  "context.empty": "Keine zugehörigen Datensätze.",

  "palette.aria": "Befehlspalette",
  "palette.placeholder": "Suchen oder Margince fragen",
  "palette.empty": "Keine Treffer.",
  "palette.typeScreen": "Ansicht",
  "palette.typeAction": "Aktion",
  "palette.typeRecord": "Datensatz",
  "palette.seeAll": "Alle Ergebnisse für „{query}“ anzeigen",
  "palette.searching": "Datensätze werden durchsucht…",
  "palette.searchFailedTitle": "Suche fehlgeschlagen",
  "palette.searchFailed": "Die Befehle oben funktionieren weiterhin.",
  "action.newDeal": "Neuer Deal",
  "action.booking": "Buchungsseite",

  "common.undo": "Rückgängig machen",
  "common.close": "Schließen",
  "clipboard.copyFailedTitle": "Zugriff auf die Zwischenablage verweigert",

  "explain.open": "Diese Zahl erklären",
  "explain.mayHaveMoved":
    "Dieser Link hält nicht fest, wann die Zahl berechnet wurde, daher wurden diese Werte jetzt neu berechnet. Hat sich seitdem ein Wechselkurs geändert, weichen sie möglicherweise von der angeklickten Zahl ab.",
  "explain.title": "So setzt sich diese Zahl zusammen",
  "explain.rate": "Kurs {rate} am {date}",
  "explain.cell": "{figure} erklären",
  "explain.excluded_one":
    "Bei 1 Datensatz ist ein Feld für deine Rolle ausgeblendet, deshalb fehlt er in dieser Zahl und in den Zeilen darunter.",
  "explain.excluded_other":
    "Bei {count} Datensätzen ist ein Feld für deine Rolle ausgeblendet, deshalb fehlen sie in dieser Zahl und in den Zeilen darunter.",

  "board.count": "Deals: {count}",
  "board.weighted": "gewichtet {value}",
  "board.mixedCurrencies": "mehrere Währungen, keine Gesamtsumme",
  "dealfiles.hidden": "An diesem Deal ausgeblendet",
  "dealfiles.unhidden": "Wieder an diesem Deal sichtbar",
  "deal.stalled": "stockt",
  "deal.stalledBadge": "Stockt",
  "deal.archived": "Archiviert",
  "deal.singleThreaded": "Nur ein Kontakt",
  "deal.staged": "Vorgemerkt",
  "deal.closes": "Abschluss {date}",
  "deal.undated": "kein Abschlussdatum",
  "deal.lastMail": "Letzte E-Mail",
  "deal.mail.title": "Bisherige E-Mails",
  "deal.mail.sent": "Gesendet {ago}",
  "deal.mail.received": "Erhalten {ago}",
  "deal.mail.none": "Noch keine E-Mail zu diesem Deal",
  "deal.mail.viewAll": "Alle Aktivitäten anzeigen",
  "deal.closesProvisional":
    "vorläufiges Abschlussdatum, von keinem Menschen bestätigt",
  "record.notShown": "Nicht angezeigt",
  "reading.restricted": "Eingeschränkt",
  "reading.loading": "Wird geladen",
  "format.notForecast": "Nicht prognostiziert",
  "record.timelineLoading": "Verlauf wird geladen…",
  "record.chronologyLoading": "Änderungsverlauf wird geladen…",
  "record.timeline": "Verlauf",
  "record.edit": "Bearbeiten",

  "record.fieldRequired": "Dieses Feld ist erforderlich.",
  "record.registration": "Registrierung",
  "record.leadProfileReadOnly":
    "LinkedIn kann bei einem Lead nicht geändert werden.",
  "record.leadStatusAction": "Ändere den Status über die Lead-Statussteuerung.",
  "record.leadScoreAction":
    "Ändere den Score über die Score-Korrektur mit Begründung.",
  "record.openProfile": "Profil öffnen",
  "record.fieldsFailed": "Eigene Felder wurden nicht geladen.",
  "record.fieldsLoading": "Eigene Felder werden geladen…",
  "record.fieldsRetry": "Erneut versuchen",

  "record.companyRoutingKey": "Zuordnungsschlüssel des Unternehmens",
  "record.finishFieldEdit":
    "Speichere oder verwirf die aktuelle Änderung, bevor du die Details schließt.",
  "record.save": "Speichern",
  "record.saveDone": "„{name}“ gespeichert",
  "record.archiveDone": "„{name}“ archiviert",
  "record.archive": "Archivieren",
  "record.disqualify": "Disqualifizieren",
  "record.archiveConfirm":
    "Diesen Datensatz archivieren? Das lässt sich nicht rückgängig machen.",
  "record.archived": "Archiviert",
  "record.archivedReadOnly":
    "Dieses Unternehmen ist archiviert und lässt keine Änderungen zu.",
  "record.notYoursToChange":
    "Du kannst dieses Unternehmen nicht bearbeiten. Frage die zuständige Person, ob sie es mit dir teilt, oder einen Admin nach Bearbeitungsrechten.",
  "record.logActivityRefused":
    "Du hast keine Berechtigung, Aktivitäten zu diesem Datensatz zu erfassen.",
  "record.share": "Teilen",
  "record.moreActions": "Weitere Aktionen",
  "record.fullHistory": "Vollständiger Verlauf",

  "share.title": "Diesen Datensatz teilen",
  "share.ceiling.pre": "Das Teilen ändert die Sichtbarkeit für ",
  "share.ceiling.recordEmphasis": "genau diesen einen Datensatz",
  "share.ceiling.mid":
    ". Am übrigen Zugriffsbereich einer Person ändert sich nichts. Geteilter Zugriff reicht höchstens so weit wie dein eigener, ",
  "share.ceiling.noWider": "nicht weiter",
  "share.ceiling.post": ".",
  "share.unknownRecord": "Dieser Datensatz kann nicht geteilt werden.",
  "share.grantAccess": "Zugriff gewähren",
  "share.subject": "Person oder Team",
  "share.holdsRead": "Hat Lesezugriff",
  "share.holdsWrite": "Hat Schreibzugriff",
  "share.kindPerson": "Person",
  "share.kindTeam": "Team",
  "share.access": "Zugriffsrecht",
  "share.access.read": "Lesezugriff",
  "share.access.write": "Schreibzugriff",
  "share.access.readNote":
    "Kann diesen Datensatz öffnen und lesen, aber nicht bearbeiten oder senden.",
  "share.access.writeNote":
    "Kann diesen Datensatz öffnen, bearbeiten und ergänzen, aber weder die Zuständigkeit noch das Teilen ändern.",
  "share.expiry": "Ablauf",
  "share.expiry.none": "Kein Ablauf (bis zum Widerruf)",
  "share.expiry.day": "Läuft in 24 Stunden ab",
  "share.expiry.week": "Läuft in 7 Tagen ab",
  "share.expiry.month": "Läuft in 30 Tagen ab",
  "share.expiryConsequence_one":
    "Der Zugriff wird in {days} Tag automatisch widerrufen. Du kannst ihn früher widerrufen.",
  "share.expiryConsequence_other":
    "Der Zugriff wird in {days} Tagen automatisch widerrufen. Du kannst ihn früher widerrufen.",
  "share.expiryConsequenceNone":
    "Der Zugriff bleibt bestehen, bis du ihn widerrufst.",
  "share.reason": "Grund",
  "share.grant": "Zugriff gewähren",
  "share.update": "Zugriff ändern",
  "share.unchanged":
    "Nichts geändert. {name} hatte bereits {access} auf diesen Datensatz.",
  "share.downgradeTitle": "Zugriff reduzieren?",
  "share.downgradeBody":
    "{name} hat {from} auf diesen Datensatz und behält nur noch {to}. Die Änderung wird im Audit-Log festgehalten.",
  "share.downgradeConfirm": "Auf {to} reduzieren",
  "share.seatCeiling":
    "Ein Leseplatz kann keinen Schreibzugriff haben. Mache den Platz zuerst zu einem vollen Platz, oder gewähre Lesezugriff.",
  "share.whoHasAccess": "Geteilt mit",
  "share.grantedBy": "gewährt von",
  "share.revoke": "Widerrufen",
  "share.revokeConfirm":
    "Diesen Zugriff widerrufen? Er endet mit der nächsten Anfrage und lässt sich nicht rückgängig machen.",
  "share.approvalRequired":
    "Dieser geteilte Zugriff wird erst nach einer Freigabe wirksam. Er ist noch nicht angewendet.",
  "share.teamMembers_one": "Team · {count} Mitglied",
  "share.teamMembers_other": "Team · {count} Mitglieder",
  "share.rosterLoading": "Personen und Teams werden geladen…",
  "share.rosterErrorUsers":
    "Personen wurden nicht geladen. Unten stehen die Teams.",
  "share.rosterErrorTeams":
    "Die Teams wurden nicht geladen. Unten stehen die Personen.",
  "share.rosterErrorBoth": "Personen und Teams wurden nicht geladen.",
  "share.rosterEmpty": "Keine Personen oder Teams zum Teilen.",

  "edit.versionSkew":
    "Dieser Datensatz wurde seit dem Öffnen geändert. Lade die Seite neu und versuche es erneut.",

  "merge.contact": "Kontakt zusammenführen",
  "merge.company": "Unternehmen zusammenführen",
  "merge.searchPlaceholder": "Suchen…",
  "merge.pickTarget": "Beizubehaltenden Datensatz auswählen",
  "merge.confirm":
    "{source} mit {target} zusammenführen? {source} wird dabei archiviert.",
  "merge.submit": "Zusammenführen",

  "tab.overview": "Übersicht",
  "tab.relationships": "Personen und Unternehmen",
  "tab.partner": "Partner",
  "tab.rollup": "Gesamtsumme",
  "tab.history": "Verlauf",

  "rollup.weightedPipeline": "Gewichteter Deal-Wert",
  "rollup.closedWon": "In diesem Quartal gewonnen",
  "rollup.activity30d": "Aktivität, 30 Tage",
  "rollup.accounts": "Einbezogene Unternehmen",
  "rollup.excluded_one": "{count} verborgenes Unternehmen ausgeschlossen",
  "rollup.excluded_other": "{count} verborgene Unternehmen ausgeschlossen",
  "rollup.fxUnavailable":
    "Ein Wechselkurs fehlt, daher lässt sich die Summe nicht berechnen.",
  "rollup.computedAt": "Berechnet am {when}",

  "nav.partners": "Partner",
  "deal.partnerSourced": "über",
  "deal.partnerInfluenced": "unterstützt von",
  "deal.partnerAttribution": "Partner-Attribution",
  "deal.attributionUnset": "Nicht festgelegt (zählt als vermittelt)",
  "deal.attributionSourced": "Deal vermittelt (mit Provision)",
  "deal.attributionInfluenced": "Bestehenden Deal beeinflusst (ohne Provision)",
  "partnerDeals.panelTitle": "Partner-Deals",
  "partnerDeals.none": "Noch keine Partner-Deals",
  "partnerDeals.column.deal": "Deal",
  "partnerDeals.column.customer": "Kunde",
  "partnerDeals.column.attribution": "Attribution",
  "partnerDeals.column.amount": "Deal-Wert",
  "partnerDeals.column.status": "Status",
  "commission.panelTitle": "Provision",
  "commission.none": "Noch keine Provision",
  "commission.column.deal": "Deal",
  "commission.column.amount": "Verdient",
  "commission.column.rate": "Satz",
  "commission.column.basis": "Deal-Wert",
  "commission.column.status": "Status",
  "commission.status.accrued": "Aufgelaufen",
  "commission.status.approved": "Freigegeben",
  "commission.status.paid": "Ausgezahlt",
  "commission.status.void": "Storniert",
  "commission.outstanding": "Offen",
  "commission.outstandingDetail_one":
    "1 Eintrag · aufgelaufen oder freigegeben",
  "commission.outstandingDetail_other":
    "{count} Einträge · aufgelaufen oder freigegeben",
  "commission.column.actions": "Aktionen",
  "commission.decide.withheld": "Keine Berechtigung zur Entscheidung",
  "commission.decide.approve": "Freigeben",
  "commission.decide.pay": "Als ausgezahlt markieren",
  "commission.decide.void": "Stornieren",
  "commission.decide.approveConfirm":
    "Die Freigabe hält fest, dass diese Provision vereinbart ist. Ausgezahlt wird dadurch nichts: Wickle die Zahlung in deinem Finanzsystem ab und markiere sie danach hier als ausgezahlt.",
  "commission.decide.payConfirm":
    "Markiere sie erst als ausgezahlt, wenn dein Finanzsystem sie ausgezahlt hat. Margince erfasst die Zahlung und bewegt kein Geld.",
  "commission.decide.voidConfirm":
    "Eine Stornierung legt neben diesem Eintrag eine Gegenbuchung an. Nichts wird gelöscht, der ursprüngliche Eintrag bleibt lesbar.",
  "commission.decide.reasonLabel": "Stornogrund",
  "commission.decide.reasonRequired":
    "Gib einen Grund ein. Er erklärt dem Partner später die Stornierung.",
  "commission.decide.approved": "Provision freigegeben",
  "commission.decide.paid": "Provision als ausgezahlt markiert",
  "commission.decide.voided": "Provision storniert",
  "commission.decide.settledElsewhere":
    "Die Zahlung erfolgt in deinem Finanzsystem. Margince erfasst das Ergebnis.",
  "partner.setup": "Als Partner einrichten",
  "partner.edit": "Partner bearbeiten",
  "partner.none": "Noch kein Partner",
  "partner.company": "Unternehmen",
  "partner.role": "Partnerrolle",
  "partner.roleAll": "Alle Rollen",
  "partner.certStatus": "Zertifizierungsstatus",
  "partner.certStatusAll": "Alle Status",
  "partner.marginTier": "Margenstaffel",
  "partner.stage": "Beziehungsphase",
  "partner.nextStep": "Nächster Schritt",
  "partner.nextStepDue": "Nächster Schritt fällig",
  "partner.servedSegments": "Betreute Segmente",
  "partner.servedSegmentsHint": "durch Kommas getrennt",
  "partner.role.hosting": "Hosting",
  "partner.role.consulting": "Beratung",
  "partner.role.strategic": "Strategisch",
  "partner.cert.applied": "Beantragt",
  "partner.cert.certified": "Zertifiziert",
  "partner.cert.suspended": "Ausgesetzt",
  "partner.marginTier.tier1": "Intro (15 %)",
  "partner.marginTier.tier2": "Aktive Zusammenarbeit (20 %)",
  "partner.marginTier.tier3": "Abschluss durch Partner (25 %)",
  "partner.stage.research": "Recherche",
  "partner.stage.identified": "Identifiziert",
  "partner.stage.contacted": "Kontaktiert",
  "partner.stage.inConversation": "Im Gespräch",
  "partner.stage.fitConfirmed": "Passung bestätigt",
  "partner.stage.agreementPending": "Vertrag ausstehend",
  "partner.stage.active": "Aktiv",
  "partner.stage.activeReferring": "Aktiv, empfiehlt weiter",
  "partner.stage.dormant": "Ruhend",
  "partner.stage.noFit": "Keine Passung",

  "rel.add": "Beziehung hinzufügen",
  "rel.seatOnDeal": "Zu einem Deal hinzufügen",
  "rel.addStakeholder": "Stakeholder hinzufügen",
  "rel.dealStakeholders": "Stakeholder",
  "rel.dealStakeholdersEmpty": "Keine Stakeholder zu diesem Deal",
  "rel.kind": "Art",
  "rel.saveDone": "Beziehung gespeichert",
  "rel.role": "Rolle",
  "rel.startedAt": "Beginn",
  "rel.endedAt": "Ende",
  "rel.current": "Aktuell",
  "rel.endedOn": "Bis {when}",
  "rel.remove": "Entfernen",
  "rel.removeConfirm":
    "Diese Beziehung entfernen? Das lässt sich nicht rückgängig machen.",
  "rel.empty": "Noch keine Beziehungen",
  "rel.counterparty": "Verknüpft mit",
  "rel.dates": "Zeitraum",
  "rel.addConfirm": "Verknüpfung „{kind}“ zu {target} hinzufügen.",
  "rel.kind.employment": "Anstellung",
  "rel.kind.dealStakeholder": "Deal-Stakeholder",
  "rel.kind.projectStakeholder": "Projekt-Stakeholder",
  "rel.kind.projectCompany": "Unternehmen im Projekt",
  "rel.kind.partnerOf": "Partner von",
  "rel.kind.referredBy": "Empfohlen von",
  "rel.kind.coSellWith": "Co-Selling mit",
  "rel.kind.worksWith": "Arbeitet mit",
  "rel.kind.billingContact": "Rechnungskontakt",

  "common.error": "Diese Ansicht wurde nicht geladen. Lade die Seite neu.",
  "common.errorNoCause":
    "Die Anfrage ist fehlgeschlagen. Keine Ursache gemeldet.",
  "common.assistantUnavailable":
    "Der Assistent hat nicht geantwortet, daher wurde kein Entwurf erstellt. Gib die Angaben von Hand ein oder lass einen Admin das Modell in den Einstellungen unter KI prüfen.",
  "common.gatewayUnavailable":
    "Der Server hat die Anfrage nicht rechtzeitig abgeschlossen und verarbeitet sie möglicherweise noch. Warte, bevor du es erneut versuchst, sonst kann die Arbeit zweimal laufen.",
  "common.permissionDenied":
    "Du hast keine Berechtigung für diese Aktion. Lass deinen Zugriff von einem Admin oder der Person erweitern, die diesen Datensatz mit dir geteilt hat.",
  "common.seatReadOnly":
    "Dieser Platz hat nur Lesezugriff, daher wurde die Anfrage abgelehnt. Lass den Platz von einem Admin hochstufen.",
  "common.retry": "Erneut versuchen",
  "common.empty": "Hier ist noch nichts.",
  "common.saving": "Wird gespeichert…",
  "common.loading": "Wird geladen…",
  "ref.nameLoadFailed": "Name wurde nicht geladen",
  "ref.notInRoster":
    "Aktuell zugewiesen (nicht mehr in der Liste der Nutzenden)",
  "picker.noMatch": "Kein Treffer",

  // "Funktioniert nicht mehr", nicht "Fehler aufgetreten": die Ansicht ist
  // stehengeblieben, und das ist die Beobachtung, die der Lesende selbst
  // machen kann. Kein Wort über den Fehler.
  "app.errorTitle": "Diese Ansicht funktioniert nicht mehr",
  "app.errorBody":
    "Versuche es erneut. Wenn es wieder fehlschlägt, lade die Seite neu.",
  "app.errorRetry": "Erneut versuchen",

  // Die Grenze um EINE Karte: sagt weniger als die App-Grenze, weil sie
  // weniger genommen hat — Seite und Navigation stehen noch.
  "card.errorTitle": "Diese Karte funktioniert nicht mehr",
  "card.errorRetry": "Erneut versuchen",

  // Das neunteilige Zustandsvokabular (design-system/surfacestate.tsx):
  // gehört dem ZUSTAND, nicht einer einzelnen Fläche.
  "state.withheld": "Für deine Rolle ausgeblendet",
  "state.unavailable":
    "Einige Daten wurden nicht geladen, daher ist dieser Abschnitt möglicherweise unvollständig.",
  "state.failed": "Dieser Abschnitt wurde nicht geladen.",
  "state.loading": "Dieser Abschnitt wird geladen…",
  "state.retry": "Erneut versuchen",
  "state.stale": "Zuletzt bekannte Werte, nicht aktualisiert",
  "state.staleAsOf": "Zuletzt bekannte Werte, Stand {when}",
  "state.partial": "Ein Teil der Liste wird angezeigt",
  "state.partialCount": "Weitere nicht angezeigt: {count}",
  "filePreview.download": "Herunterladen",
  "filePreview.print": "Drucken",
  "filePreview.loading": "Datei wird geladen…",
  "filePreview.failedTitle": "Keine Vorschau für diese Datei",
  "filePreview.failed":
    "Lade die Datei herunter, um sie in einer anderen Anwendung zu öffnen.",

  "list.headActions": "Weitere Aktionen",
  "list.search": "Suchen",
  "list.showArchived": "Archivierte anzeigen",
  "list.loadMore": "Mehr laden",
  "list.viewAll": "Alle",
  "list.viewHot": "Heiß",

  "table.range": "{unit} {first} bis {last} von {count}",
  "table.pagination": "Seiten",
  "table.page": "Seite {number}",
  "table.prev": "Zurück",
  "table.next": "Weiter",
  "table.rowsPerPage": "Zeilen pro Seite",
  "table.perPage": "{count} pro Seite",
  "table.sortedBy": "sortiert nach {column}",
  "table.shownColumns": "Sichtbare Spalten",
  "table.display": "Darstellung",
  "table.density": "Dichte",
  "table.compact": "Kompakt",
  "table.sort": "Sortieren",
  "table.sortNamed": "Sortierung: {column}",
  "table.sortMenu": "Sortieren nach",
  "table.sortDefault": "Standardreihenfolge",
  "table.sortAscending": "aufsteigend",
  "table.sortDescending": "absteigend",
  "table.sortBy": "Nach {column} sortieren",
  "table.noMatches": "Keine {unit} passen zu diesen Filtern.",
  "table.clearFilters": "Filter zurücksetzen",
  "table.none": "Noch keine {unit}.",
  "table.actions": "Aktionen",
  "table.rangeLoaded": "Geladene {unit} {first} bis {last} von {count}",
  "unit.contacts": "Kontakte",
  "unit.companies": "Unternehmen",
  "unit.deals": "Deals",
  "unit.leads": "Leads",
  "unit.partners": "Partner",
  "unit.products": "Produkte",
  "unit.offerTemplates": "Angebotsvorlagen",
  "table.filter": "Filter",
  "table.filterSearch": "Attribute durchsuchen",
  "table.addFilter": "Filter hinzufügen",
  "table.filterIs": "ist",
  "table.filterCondition": "Bedingung",
  "table.filterMore": "Weitere Aktionen für den Filter {filter}",
  "table.deleteFilter": "Filter löschen",
  "table.filterValueSearch": "Werte für {filter} durchsuchen",
  "table.filterTypeToSearch": "Zum Suchen tippen",
  "table.filterSearching": "Wird gesucht…",
  "table.filterSearchFailed": "Suche fehlgeschlagen. Versuche es erneut.",
  "table.filterNoMatches": "Keine Treffer.",
  "table.filterBack": "Zurück zu allen Filtern",

  "contacts.name": "Name",
  "contacts.email": "E-Mail",
  "list.owner": "Zuständig",
  "list.unowned": "Nicht zugewiesen",
  "list.created": "Erstellt",
  "list.lastActivity": "Letzte Aktivität",
  "list.filterOwnerMe": "In deiner Zuständigkeit",
  "list.filterOwnerAll": "Alle Zuständigen",
  "list.filterOwnerUnassigned": "Nicht zugewiesen",
  "views.save": "Ansicht speichern",
  "views.saveConfirm": "Speichern",
  "views.saveTitle": "Diese Ansicht speichern",
  "views.name": "Name",
  "views.rail": "Gespeicherte Ansichten",
  "views.manage": "Ansichten verwalten",
  "views.none": "Keine gespeicherten Ansichten",
  "views.rename": "Umbenennen",
  "views.renameNamed": "{name} umbenennen",
  "views.delete": "Löschen",
  "views.deleteNamed": "{name} löschen",
  "views.deleteAsk":
    "Das Löschen von {name} entfernt den Tab. Die Datensätze darin bleiben unverändert.",
  "views.deleteConfirm": "Ansicht löschen",
  "list.viewMine": "Meine",
  "list.viewCustomers": "Kunden",
  "list.viewProspects": "Interessenten",
  "company.filterLifecycleAll": "Jede Phase",
  "company.filterRelTypeAll": "Jeder Typ",
  "company.filterSizeBandAll": "Jede Größe",
  "consent.confirmRecipient": "An {address} senden",
  "consent.guardFailed":
    "Kommunikationsberechtigungen konnten nicht geladen werden.",
  "consent.permissionScope":
    "Berechtigungen gelten für den genannten Zweck. Kontomitteilungen und Servicehinweise berechtigen nicht zu Vertrieb oder Marketing; jede Nachricht wird vor dem Versand geprüft.",
  "consent.manage": "Einwilligungen und Nachweisverlauf verwalten",
  "contact.consent": "Einwilligung",
  "consent.grant": "Einwilligung erfassen",
  "consent.operatorWording":
    "Von einem Teammitglied im CRM erfasst, das versichert hat, dass dieser Kontakt außerhalb des Produkts seine Einwilligung für {label} erteilt hat. Dem Kontakt wurde hier kein Wortlaut angezeigt.",
  "consent.withdraw": "Widerrufen",
  "consent.doiBySubject":
    "Nur dieser Kontakt kann diesen Zweck bestätigen, über einen Link, der an seine hinterlegte Adresse gesendet wird. Nutze dafür „Bestätigung der Angaben anfordern“ unter Kommunikationsberechtigungen.",
  "consent.askToConfirm": "Bestätigung der Angaben anfordern",
  "consent.askToConfirmWhat":
    "Sendet diesem Kontakt per E-Mail einen privaten Link. Darüber sieht er, was über ihn gespeichert ist, kann es korrigieren und angeben, ob er von deinem Unternehmen hören möchte. Der Link geht an seine eigene hinterlegte Adresse; an eine andere kannst du ihn nicht senden.",
  "consent.askQueued": "Link für {address} eingereiht.",
  "consent.askNotDelivered":
    "Der Link wurde für {address} erstellt, aber diese Installation versendet keine E-Mails, deshalb wurde er nicht gesendet.",
  "consent.askExpires": "Der Link gilt bis",
  "consent.noRecord": "kein Eintrag",
  "consent.noPurposes":
    "Dieses Unternehmen erfasst noch keine Einwilligungszwecke.",
  "consent.defaultDeny":
    "Dieser Verlauf hält die Einwilligung je Zweck fest. Ob Kommunikation erlaubt ist, kann auch von anderen erfassten Grundlagen abhängen; die Nachricht wird vor dem Versand erneut geprüft.",
  "consent.basis": "Grundlage: {basis}",
  "consent.proofLog": "Nachweisprotokoll",
  "consent.proofEmpty":
    "Für diesen Zweck ist keine Einwilligungsentscheidung erfasst. Ein leeres Protokoll bedeutet, dass kein Einwilligungsereignis erfasst wurde, nicht, dass eines fehlt.",
  "consent.sourceUnknown": "Quelle nicht erfasst",
  "consent.actorHuman": "Mensch",
  "consent.actorAgent": "Agent",
  "consent.actorSystem": "System",
  "consent.actorConnector": "Connector",
  "consent.actorUnknown": "Akteur nicht erfasst",
  "consent.purposesUnavailable":
    "Der Katalog der Einwilligungszwecke wurde nicht geladen, deshalb lassen sich die Double-Opt-in-Anforderungen nicht anzeigen.",

  "company.reject": "Kein Unternehmen",
  "company.rejectConfirm":
    "Damit wird „{name}“ archiviert und {domain} als Unternehmen gesperrt, sodass spätere Nachrichten von dieser Domain es nicht erneut anlegen. Ein Admin kann die Domain in den Einstellungen unter Erfassung wieder freigeben.",
  "company.rejectReasonLabel": "Grund, warum das kein Unternehmen ist",
  "company.rejectReasonHint":
    "Ein Grund, mit dem jemand, der gesperrte Domains prüft, arbeiten kann. Die Sperre besteht über den Datensatz hinaus.",
  "company.rejectDone":
    "„{name}“ archiviert und {domain} als Unternehmen gesperrt",
  "company.name": "Unternehmen",
  "company.brief.title": "Unternehmensbericht",
  "company.description": "Beschreibung",
  "company.website": "Website",
  "company.contactCount": "Kontakte",
  "company.openDealCount": "Offene Deals",
  // Nur dort angeboten, wo es noch kein Partnerprogramm gibt: der Tab mit dem
  // Formular erscheint erst, wenn eines besteht — so entsteht das erste.
  // Wo der Account bei uns steht, und was er für uns ist — die zwei Fragen,
  // die die abgelöste Einstufung mit einem Wert beantworten wollte.
  "company.lifecycle": "Lebenszyklus",
  "company.relationshipTypes": "Beziehungstyp",
  "company.sizeBand": "Unternehmensgröße",
  "company.lifecycle.unknown": "Nicht bewertet",
  "company.lifecycle.target": "Zielunternehmen",
  "company.lifecycle.prospect": "Interessent",
  "company.lifecycle.opportunity": "Chance",
  "company.lifecycle.customer": "Kunde",
  "company.lifecycle.former_customer": "Ehemaliger Kunde",
  "company.lifecycle.disqualified": "Disqualifiziert",
  "company.relType.customer": "Kunde",
  "company.relType.partner": "Partner",
  "company.relType.supplier": "Lieferant",
  "company.relType.investor": "Investor",
  "company.relType.portfolio_company": "Portfoliounternehmen",
  "company.relType.competitor": "Wettbewerber",
  "company.relType.other": "Sonstiges",
  // Warum ein Fakt seinem eigenen Feld widerspricht. Der Fakt bleibt mit
  // seinem Beleg sichtbar — ein Mensch erkennt es, Ausblenden wäre schlechter.
  "co.factSuspect.phoneShapedLocation": "Sieht wie eine Telefonnummer aus",
  "co.factSuspect.notAPhone": "Sieht nicht wie eine Telefonnummer aus",
  "co.factSuspect.notAYear": "Sieht nicht wie eine Jahreszahl aus",
  "co.factSuspect.notAnEmail": "Sieht nicht wie eine E-Mail-Adresse aus",
  "co.factSuspect.notASize": "Sieht nicht wie eine Beschäftigtenzahl aus",
  // Die drei Aussagen, mit denen die Übersicht beginnt, und was das
  // Ausführen eines Vorschlags bedeutet.
  "co.strip.title": "Unternehmensstatus",
  "co.strip.convertedAsOf": "{count} umgerechnet, Kurse vom {date}",
  "co.strip.noOpenDeals": "Keine",
  "co.strip.pipeline": "Offene Deals",
  "co.description.label": "Beschreibung",

  "co.strip.netInvoiced": "Umsatz · 12 Monate",
  "co.strip.notAssessed": "Nicht bewertet",
  "co.strip.lifetimeOf": "{amount} insgesamt",
  "stat.evidence": "Beleg",
  "stat.evidence.rests": "Worauf das beruht",
  "stat.open": "Öffnen",
  "co.strip.fin.neverInvoiced": "Nicht fakturiert",
  "co.strip.fin.noConnection": "Buchhaltung nicht verbunden",
  "co.strip.fin.unmapped": "Kunde nicht zugeordnet",
  "co.strip.fin.syncing": "Wird synchronisiert",
  "co.strip.fin.staleFigure": "Synchronisierung veraltet",
  "co.strip.fin.notCurrent": "Nicht aktuell",
  "co.strip.fin.errorFigure": "Letzte Synchronisierung fehlgeschlagen",
  "co.strip.fin.error": "Nicht verfügbar",
  "co.strip.fin.errorWhy": "Laden fehlgeschlagen",
  "co.strip.fin.noFigure": "Kein Wert",
  "co.strip.fin.loading": "Wird geladen…",
  "co.strip.unpriced": "Kein Betrag",
  "co.strip.pricedPartly": "{priced} von {total} mit Betrag",
  "co.strip.health": "Beziehung",
  "co.strip.healthOneSided": "Einseitig",
  "co.strip.healthBalanced": "Ausgeglichen",
  "co.strip.replyShare": "{percent} % eingehend",
  "co.strip.healthActive": "Aktiv",
  "co.strip.lastTouch": "Letzter Kontakt",
  "co.strip.lastTouch.today": "Heute",
  "co.strip.lastTouch.ago": "vor {count} T",
  "co.strip.lastTouch.theirs": "Eingehend",
  "co.strip.lastTouch.ours": "Ausgehend",
  "co.strip.lastTouch.never": "Keiner",
  "co.strip.nextMeeting": "Nächster Termin",
  "co.strip.next.none": "Kein Termin geplant",
  "co.360.thread": "Aktivitäten",
  "co.360.threadCount": "Aktivitäten · {count}",
  "co.360.fullHistory": "Vollständiger Verlauf",
  "co.strip.healthQuiet": "Ruhig",
  "co.strip.noInboundEver": "Keine eingehenden Nachrichten",
  "co.strip.unanswered": "Unbeantwortet",
  "co.strip.unansweredDetail": "Tage ohne Antwort: {days}",
  "co.strip.engagement.never_contacted": "Nie kontaktiert",
  "co.strip.engagement.active": "Aktiv",
  "co.strip.engagement.waiting_on_them": "Wartet auf die Gegenseite",
  "co.strip.engagement.waiting_on_us": "Wartet auf dein Team",
  "co.strip.engagement.dormant": "Ruhig",
  "co.strip.openDeals": "{count} offen",
  "co.strip.stalled": "{count} stockend",
  "co.suggest.act.draftReply": "Entwurf erstellen",
  "co.suggest.act.openDeal": "Deal öffnen",
  "co.suggest.act.addTask": "Nächsten Schritt hinzufügen",
  // Ein Verlauf als ein Ereignis sagt zuerst, WAS er ist.
  "timeline.group.thread_other": "{count} Nachrichten",
  "timeline.group.thread_one": "{count} Nachricht",
  "timeline.group.bulk_other": "an {count} Personen gesendet",
  "timeline.group.bulk_one": "an {count} Person gesendet",
  "timeline.group.expand": "Öffnen",
  "timeline.group.collapse": "Schließen",
  "timeline.group.openThread": "Vollständigen Thread anzeigen",
  "timeline.group.mayContinue": "Möglicherweise gibt es frühere Nachrichten",
  "timeline.group.kind": "Thread",
  "timeline.group.earlier_other": "{count} frühere Nachrichten anzeigen",
  "timeline.group.earlier_one": "{count} frühere Nachricht anzeigen",
  "timeline.group.hideEarlier": "Frühere Nachrichten ausblenden",
  "timeline.thread.wrote": "schrieb",
  "timeline.thread.you": "Von dir",
  "timeline.thread.we": "Von deinem Team",
  "timeline.thread.them": "Die Gegenseite",
  "timeline.thread.sentTo": "an {who} gesendet",
  "timeline.thread.sent": "gesendet",
  "timeline.filters.kind": "Aktivitätsart",
  "timeline.filters.kind.all": "Alle Arten",
  "timeline.filters.kind.email": "E-Mail",
  "timeline.filters.kind.message": "Nachrichten",
  "timeline.filters.kind.call": "Anrufe",
  "timeline.filters.kind.meeting": "Termine",
  "timeline.filters.kind.note": "Notizen",
  "timeline.filters.kind.task": "Aufgaben",
  "timeline.filters.search": "Diesen Verlauf durchsuchen",
  "timeline.filters.from": "Von",
  "timeline.filters.to": "Bis",
  "timeline.filters.searchOmitsLimited":
    "Die Suche schließt Threads aus, deren Inhalt du nicht öffnen kannst.",
  "tab.contacts": "Kontakte",
  "tab.deals": "Deals",
  "tab.dealRoom": "Deal Room",
  "tab.dealsProjects": "Deals und Projekte",
  "tab.tasks": "Aufgaben",
  "tab.timeline": "Verlauf",
  "tab.finance": "Finanzen",
  "tab.network": "Netzwerk",
  "tab.documents": "Dokumente",
  "tab.profile": "Profil",
  "tab.meetings": "Termine",
  "tab.research": "Daten und Werkzeuge",
  // Das Briefing nach den Fragen, die es beantwortet, und die Art jeder
  // Aussage — eine Einschätzung darf nicht wie ein Fakt wirken.
  "co.brief.nature.fact": "Fakt",
  "co.brief.nature.assessment": "Bewertung",
  "co.brief.nature.recommendation": "Vorschlag",
  "co.details.title": "Details",
  "co.health.dim.relationship": "Beziehung",
  "co.health.dim.commercial": "Geschäftlich",
  "co.health.dim.payment": "Zahlung",
  "co.health.means.relationship":
    "Ob Kontakte bei diesem Unternehmen noch in Verbindung stehen: wer geschrieben hat, wie lange das her ist und welche Seite den Anfang gemacht hat.",
  "co.health.means.commercial":
    "Ob offene Deals vorankommen: ihre Phasen und wie lange jeder schon ruht.",
  "co.health.means.payment":
    "Ob Rechnungen pünktlich bezahlt werden: was gerade überfällig ist und wie spät dieses Unternehmen üblicherweise zahlt.",
  "co.health.rating.atRisk": "Gefährdet",
  "co.health.rating.good": "Gut",
  "co.health.rating.strong": "Stark",
  "co.health.payment.overdue": "Die Zahlung ist überfällig.",
  "co.health.payment.late": "Zahlt in der Regel {days} Tage nach Fälligkeit.",
  "co.health.payment.onTime": "Zahlt pünktlich.",
  "company.partnerSetUp": "Partnerprogramm einrichten",
  "signal.kind.stalled_deal": "Deal stockt",
  "signal.kind.champion_left": "Champion ausgeschieden",
  "signal.kind.reengagement": "Wieder aktiv",
  "signal.kind.buying_intent": "Kaufinteresse",
  "signal.kind.risk": "Risiko",
  "signal.kind.other": "Sonstiges",
  "signal.kind.contract_ended": "Vertrag läuft aus",
  "signal.kind.new_opportunity": "Neue Deal-Chance",
  "signal.kind.commitment_made": "Zusage gegeben",
  "signal.kind.ghosted_thread": "Keine Antwort",
  "signal.kind.project_gone_quiet": "Projekt verstummt",
  "signal.kind.funding": "Finanzierung",
  "signal.kind.leadership_change": "Führungswechsel",
  "signal.kind.expansion": "Expansion",
  "signal.kind.product_launch": "Produkteinführung",
  "co.routeIn.band.strong": "in regelmäßigem Kontakt",
  "co.routeIn.band.some": "gelegentlich in Kontakt",
  "co.routeIn.band.faint": "kaum in Kontakt",
  "co.routeIn.bandBadge.strong": "In regelmäßigem Kontakt",
  "co.routeIn.bandBadge.some": "Gelegentlich in Kontakt",
  "co.routeIn.bandBadge.faint": "Kaum in Kontakt",
  "co.routeIn.bandBadge.unknown": "Kontakt erfasst, noch kein Muster",
  "record.profile": "Profil",
  "record.context": "Kontext",
  "record.restsOn": "Quellen",
  "record.restsOn.source_one": "Quelle",
  "record.restsOn.source_other": "Quellen",
  "record.tabs": "Tabs des Datensatzes",
  "record.panel.showDetails": "Details einblenden",
  "record.panel.hideDetails": "Details ausblenden",
  "room.editorial":
    "Dokumente und Kommentare sind sofort für die Käuferseite sichtbar.",
  "room.readOnly": "Du kannst diesen Raum lesen, aber nicht ändern.",
  "room.finished":
    "Dieser Raum ist beendet. Seine geteilten Inhalte bleiben als Aufzeichnung erhalten.",
  "room.card.title": "Deal Room",
  "room.card.contacts": "{invited} eingeladen · {active} angemeldet",
  "room.card.lastSeen": "Zuletzt von der Käuferseite gesehen: {when}",
  "room.create.sub":
    "Eine Seite, auf der die Käuferseite geteilte Dokumente per Link öffnet und bespricht.",
  "room.create.open": "Deal Room eröffnen",
  "room.create.confirm": "Eröffnen",
  "room.create.titleLabel": "Titel des Raums",
  "room.create.titleHint":
    "Die Überschrift, die die Käuferseite sieht. Du kannst sie später ändern.",
  "room.create.defaultTitle": "{deal}",
  "roompage.none":
    "Dieser Deal hat noch keinen Deal Room. Eröffne einen auf der Deal-Seite.",
  "roompage.backToDeal": "← Zurück zum Deal",
  "roompage.accessMenu": "Zugang zum Raum",
  "roompage.manage": "Raum verwalten",
  "roompage.pause": "Pausieren",
  "roompage.pauseHint":
    "Die Käuferseite behält ihre Links, sieht aber eine Pausenseite, bis du fortsetzt.",
  "roompage.resume": "Fortsetzen",
  "roompage.close": "Raum schließen",
  "roompage.closeHint":
    "Die Käuferseite kann weiter lesen. Neues kann nicht hinzugefügt werden.",
  "roompage.setExpiry": "Enddatum festlegen",
  "roompage.setExpiryHint": "Der Zugang endet an diesem Tag.",
  "roompage.closeTitle": "Diesen Deal Room schließen?",
  "roompage.closeBody":
    "Die Käuferseite kann den Raum weiter lesen. Danach wird kein Dokument, kein Kommentar und keine Entscheidung mehr angenommen. Du kannst weiterhin Zugänge entziehen und Links ausstellen.",
  "roompage.expiryLabel": "Zugang endet am",
  "roompage.expiryHint": "Leer lassen, wenn es kein Enddatum geben soll.",
  "roompage.banner.paused":
    "Pausiert. Die Käuferseite sieht eine Pausenseite, bis du fortsetzt.",
  "roompage.banner.closed":
    "Geschlossen. Die Käuferseite kann den Raum weiter lesen; Neues wird nicht angenommen.",
  "roompage.banner.expired":
    "Abgelaufen. Die Links der Käuferseite funktionieren nicht mehr.",
  "roompage.banner.archived": "Archiviert. Niemand kann diesen Raum betreten.",
  "roompage.banner.liveUntil": "Live. Der Zugang endet am {when}.",
  "roompage.text.title": "Titel und Begrüßung",
  "roompage.text.titleLabel": "Titel des Raums",
  "roompage.text.welcomeLabel": "Begrüßungstext",
  "roompage.viewAsBuyer": "Als Käuferseite ansehen",
  "roompage.previewArchived":
    "Für einen archivierten Raum gibt es keine Vorschau.",
  "roompage.previewNotYours":
    "Dein Zugriff auf diesen Deal umfasst nicht die Vorschau für die Käuferseite.",
  "access.title": "Zugang",
  "access.invite": "Einladen",
  "access.empty": "Noch niemand eingeladen.",
  "access.cap.view": "Nur lesen",
  "access.cap.viewHint": "Kann die Dokumente und Kommentare lesen.",
  "access.cap.comment": "Lesen und kommentieren",
  "access.cap.commentHint": "Kann außerdem Fragen stellen und antworten.",
  "access.state.invited": "eingeladen",
  "access.state.active": "angemeldet",
  "access.state.revoked": "entzogen",
  "access.state.revokedBadge": "Entzogen",
  "access.lastSeen": "zuletzt gesehen {when}",
  "access.downloads_one": "{count} Dokument heruntergeladen",
  "access.downloads_other": "{count} Dokumente heruntergeladen",
  "access.linkRequested":
    "Hat am {when} einen neuen Link angefordert. Stelle einen aus und sende ihn selbst.",
  "access.rowActions": "Aktionen für {name}",
  "access.issueLink": "Neuen Link ausstellen",
  "access.changeCapability": "Berechtigungen ändern",
  "access.revoke": "Zugang entziehen",
  "access.inviteTitle": "In den Deal Room einladen",
  "access.inviteConfirm": "Einladen",
  "access.done": "Fertig",
  "access.save": "Speichern",
  "access.nameLabel": "Name",
  "access.emailLabel": "E-Mail",
  "access.capabilityLegend": "Berechtigungen",
  "access.inviteNote":
    "Der Link wird dir zum Kopieren angezeigt. Ist ein Mail-Relay eingerichtet, wird er zusätzlich per E-Mail gesendet, die Zustellung ist aber nicht garantiert.",
  "access.issued.title": "Link für {name}",
  "access.issued.mailed":
    "An {email} gesendet. Du kannst ihn unten auch kopieren.",
  "access.issued.notMailed":
    "Der Link wurde nicht per E-Mail gesendet. Kopiere ihn und sende ihn selbst.",
  "access.issued.mailedTitle": "Einladungs-E-Mail eingereiht",
  "access.issued.notMailedTitle": "Einladung nicht per E-Mail gesendet",
  "access.issued.linkLabel": "Einladungslink",
  "access.issued.copy": "Link kopieren",
  "access.issued.copied": "Kopiert",
  "access.issued.copyFailed": "Markiere den Link und kopiere ihn von Hand.",
  "access.issued.oneTime":
    "Persönlicher Einmallink. Er funktioniert einmal, auf einem Gerät. Jede Person braucht eine eigene Einladung.",
  "access.issueLinkTitle": "Neuen Link für {name} ausstellen",
  "access.issueLinkBody":
    "Der bisherige Link funktioniert dann nicht mehr. Der neue Link wird dir zum Kopieren angezeigt.",
  "access.revokeTitle": "Zugang für {name} entziehen?",
  "access.neverSignedIn": "nie angemeldet",
  "access.revokeBody":
    "Die Sitzung endet, und der Link funktioniert nicht mehr. Kommentare bleiben sichtbar und zugeordnet. Eine Anfrage nach einem neuen Link stellt den Zugang nicht wieder her.",
  "access.changeCapabilityTitle": "Berechtigungen für {name}",
  "contactdealrooms.title": "Deal Rooms",
  "contactdealrooms.open": "Öffnen",
  "contactdealrooms.seatGone":
    "Diese Adresse hat in diesem Deal Room keinen Platz mehr.",
  "contactdealrooms.cut":
    "Nur die ersten Deal Rooms werden angezeigt. Dieser Kontakt ist in weiteren Deal Rooms.",
  "contactdealrooms.revokeTitle": "Zugang zu {room} entziehen?",
  "room.state.draft": "Entwurf",
  "room.state.building": "Wird erstellt",
  "room.state.ready": "Bereit",
  "room.state.publishing": "Wird veröffentlicht",
  "room.state.live": "Live",
  "room.state.paused": "Pausiert",
  "room.state.closed": "Geschlossen",
  "room.state.expired": "Abgelaufen",
  "room.state.archived": "Archiviert",
  "co.pulse.owner": "Zuständig",
  "co.pulse.sizeBand": "{band} Mitarbeitende",
  "co.pulse.strongestLead": "Bester Zugang",
  "co.pulse.strengthTail_one": ", der einzige Kontakt hier",
  "co.pulse.strengthTail_other": ", unter {count} Kontakten hier",
  "co.pulse.unowned": "Nicht zugewiesen",
  "co.since.first": "Du hast dieses Unternehmen noch nie geöffnet.",
  "co.partial":
    "Einige Bereiche wurden nicht geladen, daher ist diese Seite möglicherweise unvollständig.",
  "evidence.explain": "Herkunft von „{value}“",
  "evidence.fullHistory": "Vollständiger Verlauf",
  "co.section.unavailable":
    "Einige Daten wurden nicht geladen, daher ist dieser Bereich möglicherweise unvollständig.",
  "billing.title": "Rechnungskontakte",
  "billing.contactTitle": "Rechnungsrollen",
  "billing.none":
    "Noch keine Rechnungskontakte. Füge den Kontakt hinzu, an den Rechnungen adressiert werden.",
  "billing.noEmail": "Keine E-Mail hinterlegt",
  "billing.role.recipient": "Rechnungsempfang",
  "billing.role.approver": "Rechnungsfreigabe",
  "billing.role.accountsPayable": "Kreditorenbuchhaltung",
  "billing.add": "Kontakt hinzufügen",
  "billing.change": "Ändern",
  "billing.remove": "Entfernen",
  "billing.changeOne": "Rolle von {who} ändern",
  "billing.removeOne": "{who} von den Rechnungen dieses Unternehmens entfernen",
  "billing.addTitle": "Rechnungskontakt hinzufügen",
  "billing.changeTitle": "Rechnungsrolle ändern",
  "billing.who": "Kontakt",
  "billing.findContact": "Kontakt suchen",
  "billing.role": "Rolle",
  "billing.roleNote":
    "Ein Kontakt kann jede Rolle für dieses Unternehmen einmal innehaben.",
  "billing.saveAdd": "Kontakt hinzufügen",
  "billing.saveChange": "Änderung speichern",
  "billing.versionUnresolved":
    "Der Rechnungskontakt konnte nicht erneut gelesen werden, daher wurde die Änderung nicht gesendet. Lade die Seite neu und versuche es erneut.",
  "finance.title": "Finanzen",
  "finance.titleHistorical": "Finanzen · historisch",
  "finance.none": "Nichts erfasst.",
  "finance.loading": "Rechnungen werden geladen…",
  "finance.syncing":
    "Synchronisierung mit dem Buchhaltungssystem läuft. Die Zahlen erscheinen nach der ersten Synchronisierung.",
  "finance.noConnection":
    "Kein Buchhaltungssystem verbunden. Rechnungen und Zahlungsverhalten dieses Kunden erscheinen, sobald eines verbunden ist.",
  "finance.unmapped":
    "Verbunden, aber dieses Unternehmen ist noch keinem Kunden im Buchhaltungssystem zugeordnet.",
  "finance.netInvoiced": "Netto fakturiert · 12 Monate",
  "finance.coveragePeriod": "Ausgestellte Rechnungen vom {from} bis {to}",
  "finance.overdueRelationshipEnded":
    "Ausgeblendet, weil die Geschäftsbeziehung beendet ist.",
  "finance.overdue": "Überfällig",
  "finance.behaviour": "Zahlungsverhalten",
  "finance.behaviourShape":
    "Verzugstage je beglichener Rechnung, älteste zuerst",
  "finance.shareOfOpen": "{percent} % des offenen Saldos",
  "finance.overdueShareLabel": "Überfälliger Anteil am offenen Saldo",
  "finance.legendOverdue": "Überfällig {amount}",
  "finance.legendOpen": "Offen {amount}",
  "finance.medianAfterDue": "In der Regel {days} Tage nach Fälligkeit",
  "finance.medianEarly": "In der Regel {days} Tage vor Fälligkeit",
  "finance.col.invoice": "Rechnung",
  "finance.paidOn": "bezahlt {when}",
  "finance.col.dates": "Ausgestellt → fällig",
  "finance.recentInvoices": "Letzte Rechnungen",
  "finance.paidDaysLate_one": "1 Tag zu spät bezahlt",
  "finance.paidDaysLate_other": "{days} Tage zu spät bezahlt",
  "finance.overdueDays_one": "{days} Tag überfällig",
  "finance.overdueDays_other": "{days} Tage überfällig",
  "finance.col.amount": "Betrag",
  "finance.col.status": "Status",
  "finance.unnumbered": "Ohne Nummer",
  "finance.moreInvoices": "Weitere Rechnungen im Buchhaltungssystem",
  "finance.syncedFrom": "Aus {provider} · synchronisiert {when}",
  "finance.fromNeverSynced": "Aus {provider} · noch nicht synchronisiert",
  "finance.status.draft": "Entwurf",
  "finance.status.open": "Offen",
  "finance.status.partiallyPaid": "Teilweise bezahlt",
  "finance.status.paid": "Bezahlt",
  "finance.status.overdue": "Überfällig",
  "finance.status.disputed": "Strittig",
  "finance.status.credited": "Gutgeschrieben",
  "finance.status.void": "Storniert",
  "commercial.closes": "Abschluss {when}",
  "contracts.title": "Vertr\u00e4ge",
  "contracts.empty": "Noch keine Verträge",
  "contracts.noneActive": "Heute ist kein Vertrag aktiv",
  "contracts.filter.all": "Alle",
  "contracts.filter.active": "Aktiv",
  "contracts.status.draft": "Entwurf",
  "contracts.status.active": "Aktiv",
  "contracts.status.expired": "Ausgelaufen",
  "contracts.status.cancelled": "Gek\u00fcndigt",
  "contracts.status.superseded": "Ersetzt",
  "contracts.endsOn": "Endet am {when}",
  "contracts.renewsOn": "Verl\u00e4ngert sich am {when}",
  "contracts.endedPendingStatus": "Laufzeit beendet, Status ausstehend",
  "contracts.form.title": "Vertrag erfassen",
  "contracts.form.name": "Titel",
  "contracts.form.number": "Vertragsnummer",
  "contracts.form.value": "Wert",
  "contracts.form.basis": "Dieser Wert ist",
  "contracts.form.arr": "Jährlich wiederkehrender Umsatz",
  "contracts.form.arrMonthly": "Monatlicher Gegenwert:",
  "contracts.terms.net": "{days} Tage netto",
  "contracts.terms.onReceipt": "Sofort f\u00e4llig",
  "contracts.terms.arr": "{amount}/Jahr",
  "contracts.terms.monthly": "({amount}/Monat)",
  "contracts.basis.total": "der Gesamtwert der gesamten Laufzeit",
  "contracts.basis.annual": "12 Monate eines unbefristeten Vertrags",
  "contracts.form.startsOn": "Beginnt",
  "contracts.form.endsOn": "Endet",
  "contracts.form.endsOnHint": "Für einen unbefristeten Vertrag leer lassen.",
  "contracts.form.renewalOn": "Verl\u00e4ngert sich",
  "contracts.form.noticeDays": "K\u00fcndigungsfrist (Tage)",
  "contracts.form.noticeDaysHint":
    "Frist für eine Kündigung. Die Verlängerungswarnung erscheint vor dieser Frist, nicht vor dem Verlängerungsdatum.",
  "contracts.form.paymentTerms": "Zahlungsziel (Tage)",
  "contracts.form.paymentTermsHint":
    "Tage, die der Kunde zum Zahlen hat. 0 bedeutet sofort fällig; leer lassen, wenn kein Zahlungsziel vereinbart ist.",
  "contracts.form.signedOn": "Unterschrieben",
  "contracts.form.signedOnHint":
    "Nur, wenn jemand die Unterschrift bestätigt. Nie aus dem Abschlussdatum des Deals übernommen.",
  "contracts.form.save": "Vertrag erfassen",
  "contracts.form.errNoName": "Gib einen Vertragstitel ein.",
  "contracts.form.errTermOrder":
    "Eine Laufzeit kann nicht vor ihrem Beginn enden.",
  "contracts.add": "Vertrag hinzuf\u00fcgen",
  "contracts.rowMenu": "Vertragsaktionen",
  "contracts.renew.title": "Vertrag verlängern",
  "contracts.renew.hint":
    "Legt einen neuen Vertrag mit eigenen Bedingungen an und markiert diesen als ersetzt. Übernommen wird nur die Vertragspartei.",
  "contracts.renew.deal": "Deal",
  "contracts.renew.dealHint":
    "Der Deal, mit dem diese Laufzeit gewonnen wurde, falls vorhanden. Nie der Deal des vorherigen Vertrags.",
  "contracts.renew.dealNone": "Kein Deal",
  "contracts.renew.dealWithheldCompany":
    "Du kannst das Unternehmen dieses Vertrags nicht öffnen, daher lassen sich seine Deals nicht auflisten. Die Verlängerung behält dieselbe Vertragspartei und erfasst keinen Deal.",
  "contracts.renew.dealWithheldTitle": "Kein Deal verfügbar",
  "contracts.renew.submit": "Verlängern",
  "contracts.deal": "Deal",
  "contracts.statusChange.title": "Status ändern",
  "contracts.statusChange.label": "Neuer Status",
  "contracts.statusChange.submit": "Status ändern",
  "contracts.statusChange.errSame": "Der Vertrag hat bereits diesen Status.",
  "contracts.cancel.title": "Kündigung erfassen",
  "contracts.cancel.hint":
    "Der Kunde bleibt bis zum Wirksamkeitsdatum unter Vertrag. Erfasst wird die Kündigung, der Status ändert sich nicht.",
  "contracts.cancel.noticeOn": "Gekündigt am",
  "contracts.cancel.effectiveOn": "Wirksam ab",
  "contracts.cancel.effectiveOnHint":
    "Nicht nach Ende der Laufzeit und nicht vor dem Kündigungsdatum.",
  "contracts.cancel.submit": "Kündigung erfassen",
  "contracts.cancel.menuLabel": "Vertrag kündigen",
  "contracts.cancel.errIncomplete": "Gib beide Daten ein.",
  "contracts.cancel.errOrder":
    "Eine Kündigung kann nicht wirksam werden, bevor sie erklärt wurde.",
  "contracts.cancel.errTermEnd":
    "Eine Kündigung kann nicht nach Ende der Laufzeit wirksam werden.",
  "contracts.value.perYear": "pro Jahr",
  "contracts.value.total": "für die gesamte Laufzeit",
  "contracts.files": "Dateien",
  "contracts.noTerm": "Keine Daten erfasst",
  "contracts.openStart": "Offener Beginn",
  "contracts.openEnd": "Unbefristet",
  "contracts.edit": "Bearbeiten",
  "contracts.archive": "Archivieren",
  "contracts.archive.title": "Diesen Vertrag archivieren?",
  "contracts.archive.body":
    "„{title}“ verschwindet aus den Listen und den Summen des Unternehmens. Der Datensatz und sein Verlauf bleiben erhalten; gelöscht wird nichts.",
  "contracts.archive.confirm": "Archivieren",
  "contracts.form.editTitle": "Vertrag bearbeiten",
  "contracts.form.saveEdit": "\u00c4nderungen speichern",
  "contracts.form.file": "Unterschriebenes Dokument",
  "contracts.form.fileHint":
    "Lege das unterschriebene PDF hier ab oder klicke, um es auszuwählen. Es wird diesem Vertrag zugeordnet und erscheint in den Dokumenten des Unternehmens.",
  "contracts.form.fileEmpty": "Datei hier ablegen oder zum Auswählen klicken",
  "contracts.form.fileAdd":
    "Weitere Datei hier ablegen oder zum Auswählen klicken",
  "contracts.perYear": "{amount}/Jahr",
  "contracts.state.title": "Unter Vertrag · {count} aktiv",
  "contracts.state.none": "Kein Vertrag erfasst",
  "contracts.state.renewsOn": "Verlängert sich am {when}",
  "contracts.state.endsOn": "Gekündigt, endet am {when}",
  "contracts.state.partial": "{priced} von {total} mit Betrag",
  "commercial.lastOffer": "Letztes Angebot · {deal}",
  "commercial.offerUnnumbered": "Angebot",
  "commercial.validUntil": "gültig bis {when}",
  "commercial.offer.draft": "Entwurf",
  "commercial.offer.sent": "Gesendet",
  "commercial.offer.accepted": "Angenommen",
  "commercial.offer.rejected": "Abgelehnt",
  "commercial.offer.expired": "Abgelaufen",
  "commercial.offer.superseded": "Ersetzt",
  "co.section.restricted": "Für deine Rolle ausgeblendet",
  "co.next.title": "Aufgaben",
  "co.next.empty": "Keine offenen Aufgaben zu diesem Unternehmen.",
  "co.next.overdue": "\u00dcberfällig",
  "co.next.due": "Fällig {when}",
  "co.next.undated": "Kein Fälligkeitsdatum",
  "co.work.noDeals": "Keine offenen Deals.",
  "co.work.closes": "Abschluss {date}",
  "co.brief.by.model": "Von Margince geschrieben",
  "co.brief.by.deterministic": "Aus CRM-Datensätzen zusammengestellt",
  "co.brief.generatedAt": "Stand {when}",
  "co.growthFit.title": "Wachstumspotenzial",
  "co.growthFit.unavailable":
    "Die Bewertung wurde nicht geladen. Der Unternehmensdatensatz ist unverändert.",
  "co.growthFit.assembling":
    "Dieses Unternehmen wird bewertet. Die erste Bewertung liest den gesamten Datensatz und kann eine Minute dauern.",
  "co.growthFit.reassess": "Neu bewerten",
  "co.growthFit.reassessing": "Wird bewertet…",
  "co.growthFit.band.strong": "Passt gut",
  "co.growthFit.dim.industryFit": "Branchenpassung",
  "co.growthFit.dim.companySize": "Unternehmensgröße",
  "co.growthFit.dim.transformationNeed": "Transformationsbedarf",
  "co.growthFit.dim.access": "Zugang",
  "co.growthFit.band.moderate": "Passt teilweise",
  "co.growthFit.band.weak": "Passt kaum",
  "co.growthFit.band.unknown": "Zu wenig für eine Bewertung",
  "co.growthFit.completeness": "{present} von {expected} Angaben erfasst",
  "co.growthFit.missing": "Fehlt noch",
  "co.growthFit.capped": "Zurückgehalten: {reason}.",
  "co.growthFit.nextStep": "Als Nächstes: {step}.",
  "co.growthFit.positive": "Förderliche Faktoren",
  "co.growthFit.negative": "Hemmende Faktoren",
  "co.growthFit.whitespace": "Verkaufspotenzial",
  "co.growthFit.objections": "Wahrscheinliche Einwände",
  "co.growthFit.angle": "Vorgeschlagener Ansatz",
  "co.dossier.title": "Unternehmensüberblick",
  "co.dossier.unavailable":
    "Der Überblick wurde nicht geladen. Der Unternehmensdatensatz ist unverändert.",
  "co.dossier.empty":
    "Zu diesem Unternehmen ist noch nichts erfasst. Lies seine Website oder vervollständige das Profil unten.",
  "co.dossier.stale": "Zuletzt vor über einem Monat gelesen",
  "co.dossier.rewrite": "Neu erzeugen",
  "co.dossier.rewriting": "Wird neu erzeugt…",
  "co.dossier.section.summary": "Zusammenfassung",
  "co.dossier.section.products_services": "Produkte und Dienstleistungen",
  "co.dossier.section.markets": "Märkte",
  "co.dossier.section.buying_center": "Buying Center",
  "co.dossier.section.differentiation": "Differenzierung",
  "co.dossier.section.firmographics": "Größe, Alter und Registrierung",
  "co.evidence.unavailable":
    "Die Quelle wurde nicht geladen. Der Datensatz ist unverändert.",
  "co.evidence.producedBy": "erfasst von {who}",
  "co.evidence.retrievedAt": "Gelesen am {when}",
  "co.evidence.verifiedAt": "Von einer Person bestätigt am {when}",
  "co.evidence.confidence": "Modellsicherheit {percent} %",
  "co.evidence.gaps": "Nicht erfasst: {fields}.",
  "co.evidence.kind.site_read": "Von der Unternehmenswebsite gelesen",
  "co.evidence.kind.connector": "Aus einem verbundenen System",
  "co.evidence.kind.human": "Von einer Person eingegeben",
  "co.evidence.kind.migration": "Importiert",
  "co.evidence.kind.rule": "Abgeleitet",
  "co.brief.cite.deal": "Deal",
  "co.brief.cite.activity": "Aktivität",
  "co.brief.cite.contact": "Kontakt",
  "co.brief.cite.company": "Unternehmen",
  "co.brief.cite.fact": "Fakt",
  "co.brief.cite.profile_field": "Profilfeld",
  "co.brief.cite.deal.many": "{count} Deals",
  "co.brief.cite.activity.many": "{count} Aktivitäten",
  "co.brief.cite.contact.many": "{count} Kontakte",
  "co.brief.cite.company.many": "{count} Unternehmen",
  "co.brief.cite.fact.many": "{count} Fakten",
  "co.brief.cite.profile_field.many": "{count} Profilfelder",
  "approval.kind.advance_deal": "Deal voranbringen",
  "approval.kind.promote_lead": "Lead qualifizieren",
  "approval.kind.close_date_correction": "Abschlussdatum korrigieren",
  "approval.kind.deal_follow_up": "Follow-up zum Deal anlegen",
  "approval.kind.archive_record": "Datensatz archivieren",
  "approval.kind.merge_records": "Datensätze zusammenführen",
  "approval.kind.merge_tags": "Tags zusammenführen",
  "approval.kind.update_record": "Datensatz aktualisieren",
  "approval.kind.create_record": "Datensatz anlegen",
  "approval.kind.send_email": "E-Mail senden",
  "approval.kind.communication_review": "Abgelehnte E-Mail prüfen",
  "approval.kind.held_draft": "Entworfene E-Mail prüfen",
  "approval.kind.book_meeting": "Termin buchen",
  "approval.kind.volume_release": "Agent weiterarbeiten lassen",
  "approval.kind.coldstart": "Neues Unternehmen ergänzen",
  "approval.kind.enrich": "Aus dem Web anreichern",
  "approval.kind.deepread": "Unternehmenswebsite lesen",
  "approval.kind.linkedin_match": "LinkedIn-Treffer",
  "approval.kind.site_lead": "Kontakt von der Website anlegen",
  "approval.kind.capture_counterparty": "Kontakt aus E-Mail anlegen",
  "approval.kind.company_name_promotion": "Unternehmen umbenennen",
  "approval.kind.vcard_create": "Kontakt aus Visitenkarte anlegen",
  "approval.kind.lifecycle_change": "Lebenszyklusphase",
  "approval.kind.stage_progression": "Deal-Phase voranbringen",
  "approval.field.because": "Grund",
  "approval.field.from_stage": "Von",
  "approval.field.to_stage": "Nach",
  "approval.kind.transcript_proposal":
    "Nächsten Schritt aus Transkript anlegen",
  "approval.kind.fx_rate_proposal": "Wechselkurse aktualisieren",
  "approval.kind.ai_model_rate_proposal": "Modellpreise aktualisieren",
  "approval.kind.disqualify_lead": "Lead disqualifizieren",
  "approval.kind.demote_lead": "Lead-Qualifizierung zurücknehmen",
  "approval.kind.advance_project_phase": "Projektphase voranbringen",
  "approval.kind.assign_owner": "Zuständigkeit zuweisen",
  "approval.kind.commit_import": "Import übernehmen",
  "approval.kind.emit_flow_event": "Automatisierungsschritt erfassen",
  "approval.kind.relink_activity": "Aktivität neu zuordnen",
  "approval.kind.relink_thread": "Thread neu zuordnen",
  "approval.kind.relink_activities": "Aktivitäten neu zuordnen",
  "approval.kind.scheduled_send_held": "Zurückgehaltene Nachricht freigeben",
  "approval.kind.send_company_email": "E-Mail an Unternehmen",
  "approval.kind.send_message": "Nachricht senden",
  "approval.field.basis": "Grund",
  "approval.field.step": "Schritt",
  "approval.field.intent": "Grund für den Entwurf",
  "approval.field.evidence_snippet": "Auszug der Seite",
  "approval.field.previous_close_date": "Aktuelles Abschlussdatum",
  "approval.field.expected_close_date": "Vorgeschlagenes Datum",
  "approval.field.due_date": "Fällig",
  "approval.field.scheduled_at": "Geplant für",
  "approval.field.flags": "Probleme",
  "approval.field.closeDateFlag.overdue": "Datum verstrichen",
  "approval.field.closeDateFlag.missing": "kein Datum gesetzt",
  "approval.field.closeDateFlag.unrealistic_soon": "unrealistisch früh",
  "approval.field.closeDateFlag.unrealistic_stale":
    "kein Fortschritt in letzter Zeit",
  "approval.field.name": "Name",
  "approval.field.role": "Rolle",
  "approval.field.email": "E-Mail",
  "approval.field.domain": "Domain",
  "approval.field.company": "Unternehmen",
  "approval.field.title": "Position",
  "approval.field.leadNameNow": "Aktueller Name",
  "approval.field.capturedName": "Name in der Nachricht",
  "approval.field.leadCompanyNow": "Aktuelles Unternehmen",
  "approval.field.capturedCompany": "Unternehmen in der Nachricht",
  "approval.field.leadTitleNow": "Aktuelle Position",
  "approval.field.capturedTitle": "Position in der Nachricht",
  "approval.field.phone": "Telefon",
  "approval.field.url": "Website",
  "approval.field.address": "Adresse",
  "approval.field.published_email": "E-Mail auf der Seite",
  "approval.field.connection_name": "Name auf LinkedIn",
  "approval.field.connection_company": "Unternehmen auf LinkedIn",
  "approval.field.contact_name": "Kontakt im CRM",
  "approval.field.owner": "Zuständig",
  "approval.field.to": "An",
  "approval.field.currency": "Währung",
  "approval.field.rate": "Neuer Kurs",
  "approval.field.prior_rate": "Aktueller Kurs",
  "approval.field.provider": "Anbieter",
  "approval.field.model": "Modell",
  "approval.field.input_per_mtok": "Eingabe, pro Million Tokens",
  "approval.field.output_per_mtok": "Ausgabe, pro Million Tokens",
  "approval.field.tool": "Tool",
  "approval.field.observed": "Verbraucht",
  "approval.field.limit": "Limit",
  "approval.field.allowance": "Angefragt",
  "co.assistant.title": "Fragen zu diesem Unternehmen",
  "co.assistant.aiTag": "KI-gestützt",
  "co.decisions.open": "{count} offene prüfen",
  "co.decisions.title": "Offene Freigaben",
  "co.decisions.group": "{count} × {kind}",
  "co.decisions.empty": "Keine offenen Freigaben zu diesem Unternehmen.",
  "co.ask.title": "Margince fragen",
  "co.ask.q.whats_open": "Was ist hier offen?",
  "co.ask.q.meeting_prep": "Auf einen Termin vorbereiten",
  "co.ask.q.whats_changed": "Was hat sich zuletzt geändert?",
  "co.ask.nothing":
    "Keiner der Datensätze, auf die du Zugriff hast, beantwortet diese Frage.",
  "co.ask.failed": "Die Frage wurde nicht beantwortet. Versuche es erneut.",
  "co.suggest.title": "Margince schlägt vor",
  "co.suggest.kind.no_reply": "Keine Antwort",
  "co.suggest.kind.stalled_deal": "Stockender Deal",
  "co.suggest.kind.no_next_step": "Kein nächster Schritt",
  "co.suggest.kind.lifecycle_conflict": "Widerspruch im Lebenszyklus",
  "co.suggest.kind.commitment_unmet": "Zusage nicht eingehalten",
  "co.suggest.kind.question_unanswered": "Frage unbeantwortet",
  "co.suggest.kind.risk_raised": "Risiko angesprochen",
  "co.suggest.kind.need_raised": "Bedarf angesprochen",
  "co.suggest.more": "Weitere nicht angezeigt: {count}.",
  "co.suggest.basedOn": "Grundlage",
  "co.cite.open": "Datensatz öffnen",
  "co.suggest.dismiss": "Nicht jetzt",
  "co.suggest.byline": "Margince schlägt vor",
  "co.suggest.dismissFailed":
    "Der Vorschlag wurde nicht ausgeblendet. Versuche es erneut.",
  "co.suggest.addTaskFailed":
    "Der nächste Schritt wurde nicht gespeichert. Versuche es erneut.",
  "co.suggest.viewTasks": "Aufgaben ansehen",
  "co.suggest.commitment.overdueCount": "{count} überfällig",
  "co.suggest.commitment.openCount": "{count} offen",
  "co.suggest.commitment.overdueAtLeast": "{count}+ überfällig",
  "co.suggest.commitment.openAtLeast": "{count}+ offen",
  "co.deals.title": "Deals",
  "co.deals.empty": "Keine offenen Deals zu diesem Unternehmen.",
  "co.deals.wonLifetime": "Bisher gewonnen",
  "co.deals.lostCount": "{count} verloren",
  "co.deals.noStage": "Keine Phase",
  "co.rail.all": "Alle {count}",
  "co.rail.add": "Hinzufügen",
  "co.rail.allUncounted": "Alle",
  "co.rail.deals.title": "Aktive Deals",
  "co.rail.deals.empty": "Noch keine Deals zu diesem Unternehmen.",
  "co.rail.deals.emptyClosedOnly": "Keine offenen Deals, nur abgeschlossene.",
  "co.rail.deals.noCloseDate": "kein Abschlussdatum",
  "co.rail.deals.attentionOverdue": "Überfällig",
  "co.rail.deals.attentionCommitment": "Zusage der Käuferseite fällig",
  "co.rail.contacts.title": "Wichtige Kontakte",
  "co.rail.contacts.empty": "Noch keine Kontakte.",
  "co.rail.contacts.add": "Kontakt hinzufügen",
  "co.rail.contacts.inTouch": "Bereits in Kontakt",
  "co.rail.projects.title": "Projekte",
  "co.rail.projects.empty": "Noch keine Projekte.",

  "co.commercial.title": "Geschäftliches",
  "co.commercial.lostFigure": "Verlorene Deals",
  "co.commercial.allDeals": "Alle Deals",
  "co.commercial.truncated":
    "Mehr offene Deals, als hier Platz haben. Öffne „Alle Deals“, um den Rest zu sehen.",
  "linkedinImport.title": "LinkedIn-Verbindungen",
  "linkedinImport.sub":
    "Importiere deinen LinkedIn-Export, um zu sehen, wen dein Team bereits kennt.",
  "linkedinImport.profileLabel": "URL deines LinkedIn-Profils",
  "linkedinImport.profilePlaceholder": "https://www.linkedin.com/in/…",
  "linkedinImport.saveProfile": "Profil speichern",
  "linkedinImport.saveFailed":
    "Die Profil-URL wurde nicht gespeichert. Versuche es erneut.",
  "linkedinImport.profileReadFailed":
    "Deine Profil-URL konnte nicht geladen werden.",
  "linkedinImport.importFailed":
    "Der Export wurde nicht importiert. Versuche es erneut.",
  "linkedinImport.editProfile": "Bearbeiten",
  "linkedinImport.editProfileTitle": "Dein LinkedIn-Profil",
  "linkedinImport.profileNotSet": "Noch nicht erfasst",
  "linkedinImport.connectedNote":
    "Verbunden. Importierte Verbindungen werden diesem Profil zugeordnet, sodass das CRM zeigt, wer im Team jemanden kennt.",
  "linkedinImport.notConnectedNote":
    "Wenn du deine Profil-URL speicherst, werden importierte Verbindungen dir namentlich zugeordnet.",
  "linkedinImport.whichFile":
    "LinkedIn stellt die Datei Connections.csv unter „Einstellungen“, dann „Datenschutz“ bereit, wo sich eine Kopie der eigenen Daten anfordern lässt. Lade diese Datei aus dem Archiv hoch. Hochgeladene Verbindungen werden nie zu Kontakten: Suche, Listen und Kontaktseiten zeigen sie nicht, und niemand kann ihnen schreiben oder E-Mails senden.",
  "linkedinImport.choose": "Connections.csv ausw\u00e4hlen",
  "linkedinImport.importLabel": "Export der Verbindungen",
  "linkedinImport.noMatchesYet":
    "Noch keine Treffer, was bei einem neuen Unternehmen normal ist. Verbindungen werden mit Kontakten abgeglichen, sobald E-Mails gelesen werden, und der Abgleich läuft stündlich erneut.",
  "linkedinImport.working": "Export wird gelesen…",
  "linkedinImport.imported": "Verbindungen importiert",
  "linkedinImport.confirmed": "Einem Kontakt zugeordnet",
  "linkedinImport.suggested": "Wartet auf Bestätigung",

  // Die Prüfliste und die Reichweiten-Tabelle (ADR-0078 §2.1b).
  "linkedinReach.title": "Reichweite im Netzwerk",
  "linkedinReach.sub":
    "Erfasste Unternehmen, bei denen du jemanden kennst, die meisten Verbindungen zuerst.",
  "linkedinReach.readFailed":
    "Die Reichweite im Netzwerk konnte nicht geladen werden.",
  "linkedinReach.empty":
    "Noch keine deiner Verbindungen arbeitet bei einem erfassten Unternehmen.",
  "linkedinReach.allUnresolved":
    "Alle deine Verbindungen arbeiten bei Unternehmen, die noch nicht erfasst sind. Verbindungen: {unresolved}.",
  "linkedinReach.accountsLabel": "Unternehmen in deiner Reichweite",
  "linkedinReach.account": "Unternehmen",
  "linkedinReach.connections": "Verbindungen",
  "linkedinReach.onFile": "Bereits erfasst",
  "linkedinReach.onFileOf": "{onFile} von {total}",
  "linkedinReach.footnote":
    "{shown} von {total} Unternehmen angezeigt. Verbindungen bei noch nicht erfassten Unternehmen: {unresolved}.",
  "linkedinImport.skipped": "Übersprungene Zeilen (kein verwertbarer Name)",
  "co.signals.title": "Signale",
  "co.signals.emptyDetail":
    "Margince durchsucht Termine, E-Mails und Rechnungen nach Zusagen, Blockern und Risiken. Dafür ist mindestens eine dieser Quellen nötig.",
  "co.signals.empty": "Keine offenen Signale zu diesem Unternehmen.",
  "co.signals.openProject": "Projekt öffnen",
  "co.signals.openSource": "Meldung lesen",
  "chronology.label": "Verlaufsfilter",
  "chronology.activities": "Aktivitäten",
  "chronology.changes": "Änderungen",
  "filter.label": "Filter",
  "chronology.all": "Alles",
  "chronology.conversations": "Threads",
  "chronology.conversationsEmpty": "Noch keine Threads.",
  "convo.yourMove": "Antwort nötig",
  "convo.waitingOnThem": "Wartet auf Antwort der Gegenseite",
  "chronology.changesEmpty":
    "Seit dem Anlegen dieses Datensatzes wurde kein Feld geändert.",
  "chronology.allEmpty": "Noch keine Aktivität zu diesem Datensatz.",
  "chronology.truncated":
    "Ältere Einträge werden nicht angezeigt, weil es zu viele sind, um sie gemeinsam zu sortieren. Wähle Aktivitäten oder Änderungen, um weiter zurückzublicken.",
  "chronology.truncatedActivities":
    "Nur die neuesten Aktivitäten werden angezeigt.",
  "timeline.sentTo": "Gesendet an {who}",
  "timeline.receivedFrom": "Von {who}",
  "timeline.withWhom": "Mit {who}",
  "timeline.fieldUpdated": "Feld aktualisiert",
  "timeline.sent": "Gesendet",
  "timeline.received": "Empfangen",
  "timeline.kind.email": "E-Mail",
  "timeline.kind.meeting": "Termin",
  "timeline.kind.note": "Notiz",
  "timeline.kind.call": "Anruf",
  "timeline.kind.task": "Aufgabe",
  "timeline.kind.message": "Nachricht",
  "timeline.kind.change": "Datensatz",
  "timeline.withheld": "Inhalt nur für Teilnehmende",
  "compose.deadRecipients":
    "E-Mails an {addresses} kommen nicht an: Die letzte Zustellung wurde abgelehnt, und seitdem war keine erfolgreich. Trotzdem senden oder eine andere Adresse verwenden.",
  "compose.threadShare": "Mit dem Unternehmen teilen",
  "compose.threadMakePrivate": "Auf privat setzen",
  "compose.threadScope": "Gilt für den gesamten Thread.",
  "compose.threadStillHeld_one":
    "{count} weiteres Nutzerkonto hat diesen Thread nicht geteilt",
  "compose.threadStillHeld_other":
    "{count} weitere Nutzerkonten haben diesen Thread nicht geteilt",
  "compose.reason.posture": "Durch deine Einstellung zurückgehalten",
  "compose.reason.workspaceFloor": "Durch das Unternehmen zurückgehalten",
  "compose.reason.noRecord": "Zurückgehalten: kein Datensatz",
  "compose.reason.pendingVerdict": "Bis zur Einstufung zurückgehalten",
  "compose.reason.manual": "Privat gehalten",
  "compose.reason.verdict": "Durch Einstufung zurückgehalten",
  "compose.reason.counterparty": "Zurückgehalten: E-Mail mit der Gegenseite",
  "compose.reason.explicitlyConfidential": "Als vertraulich markiert",
  "compose.reason.noCounterparty": "Zurückgehalten: keine Gegenseite",
  "compose.audience": "Sichtbarkeit ändern",
  "compose.audienceTitle": "Wer darf diese Nachricht lesen?",
  "compose.audienceLegend": "Sichtbarkeit der Nachricht",
  "email.aMessage": "Eine Nachricht",
  "email.noSubject": "Kein Betreff",
  "email.withheldSubject": "Nicht für dich freigegeben",
  "email.receivedFrom": "Erhalten von {who}",
  "email.received": "Erhalten",
  "email.sentTo": "Gesendet an {who}",
  "email.sent": "Gesendet",
  "email.outgoingTo": "Ausgehend an {who}",
  "email.outgoing": "Ausgehend",
  "email.sendingTo": "Wird an {who} gesendet",
  "email.sending": "Wird gesendet",
  "email.notSentTo": "Nicht an {who} gesendet",
  "email.notSent": "Nicht gesendet",
  "email.bouncedFrom": "Hat {who} nicht erreicht",
  "email.bounced": "Nicht angekommen",
  "email.access.sentence.workspace": "Alle im Unternehmen können das lesen.",
  "email.access.sentence.team":
    "Alle, die die zugeordneten Datensätze öffnen können, können das lesen.",
  "email.access.sentence.participants":
    "Nur die Beteiligten dieser Nachricht können sie lesen.",
  "email.access.sentence.selected":
    "Nur die unten genannten Personen können das lesen.",
  "visibility.team": "Team",
  "visibility.workspace": "Geteilt",
  "visibility.participants": "Beteiligte",
  "visibility.selected": "Ausgewählte",
  "visibility.private": "Nur du",
  "visibility.withheld": "Zurückgehalten",
  "email.access.unnamedMember": "Ehemaliges Nutzerkonto",
  "email.move.needsReply": "Antwort nötig",
  "email.move.waitingForThem": "Wartet auf Antwort der Gegenseite",
  "email.detail.loading": "Nachricht wird geladen…",
  "email.detail.none": "Diese Nachricht",
  "email.detail.attachments_one": "{count} Anhang",
  "email.detail.attachments_other": "{count} Anhänge",
  "email.detail.showQuoted": "Zitierten Verlauf anzeigen",
  "email.detail.withheldReason":
    "Diese Nachricht ist nicht für dich freigegeben",
  "email.detail.from": "Von",
  "email.detail.to": "An",
  "email.detail.cc": "Cc",
  "email.detail.filedUnder": "Abgelegt unter",
  "email.detail.when": "Gesendet",
  "email.detail.bccWithheld":
    "Einige Empfangende stehen in Blindkopie und werden nicht angezeigt.",
  "compose.audienceWorkspace": "Alle im Unternehmen",
  "compose.audienceWorkspaceHint":
    "Alle, die die zugeordneten Datensätze öffnen können, können diese Nachricht lesen.",
  "compose.audienceParticipants": "Nur Beteiligte",
  "compose.audienceParticipantsHint":
    "Nur die an dieser Nachricht beteiligten Personen können Betreff und Text lesen. Andere sehen nur, dass an diesem Tag eine Nachricht ausgetauscht wurde.",
  "compose.audienceSelected": "Benannte Personen",
  "compose.audienceSelectedHint":
    "Nur die Personen und Teams, die du benennst, und alle, die bereits an der Nachricht beteiligt sind.",
  "compose.audienceMembersLegend": "Wer sie lesen darf",
  "compose.audienceMembersLoading": "Nutzende werden geladen…",
  "compose.audienceConfirm": "Sichtbarkeit speichern",
  "compose.audienceNote":
    "Gilt nur für diese Nachricht, nicht für den Thread oder den Kontakt.",
  "timeline.textMore": "Mehr anzeigen",
  "timeline.textLess": "Weniger anzeigen",
  "timeline.tailMore": "Signatur und zitierten Text anzeigen",
  "timeline.tailLess": "Signatur und zitierten Text ausblenden",
  "co.profileField.display_name": "Unternehmensname",
  "co.profileField.offer_summary": "Angebot",
  "co.profileField.icp": "Ideales Kundenprofil",
  "co.profileField.buying_center": "Buying Center",
  "co.profileField.value_proposition": "Nutzenversprechen",
  "co.profileField.usp": "Differenzierung",
  "co.profileField.customer_pains": "Kundenprobleme",
  "co.profileField.desired_outcomes": "Gewünschte Ergebnisse",
  "co.profileField.buying_intents": "Kaufauslöser",
  "co.profileField.common_objections": "Häufige Einwände",
  "co.profileField.sales_motion": "Vertriebsprozess",
  "co.profileField.legal_name": "Eingetragener Name",
  "co.profileField.registered_address": "Eingetragene Anschrift",
  "co.profileField.register_vat": "Register / USt-ID",
  "co.profileField.legal_form": "Rechtsform",
  "co.profileField.register_court": "Registergericht",
  "co.profileField.register_number": "Registernummer",
  "co.profileField.industry": "Branche",
  "co.profileField.history": "Geschichte",
  "co.narrative.title": "Beschreibung",
  "co.narrative.add": "Hinzufügen",
  "co.contacts.engagement": "Engagement",
  "co.contacts.lastInteraction": "Letzter Austausch",
  "co.contacts.strength": "Beziehung",
  "co.contacts.neverInTouch": "Noch kein Austausch",
  "co.contacts.theyWrote": "Der Kontakt hat geschrieben",
  "co.contacts.weWrote": "Dein Team hat geschrieben",
  "co.contacts.filter.status": "Engagement",
  "co.contacts.filter.statusAll": "Beliebiges Engagement",
  "co.contacts.band.wayIn": "Bester Zugang",
  "co.contacts.band.noWayIn": "Keiner",
  "co.contacts.band.noWayInWhy": "Angeschrieben · keine Antworten",
  "co.contacts.band.committee": "Buying Center",
  "co.contacts.band.missing": "Ohne {role}",
  "co.contacts.band.committeeComplete": "Vollständig",
  "co.contacts.band.committeeUnread": "Eingeschränkt",
  "co.contacts.band.committeeUnreadWhy": "Deals nicht sichtbar",
  "co.contacts.band.seatsHeld": "{count} im Team",
  "co.contacts.band.someHidden": "{count} ausgeblendet",
  "co.contacts.band.coverage": "Abdeckung",
  "co.contacts.band.reachable": "{count} von {total}",
  "co.contacts.band.untried":
    "Nicht kontaktiert: {count} · Wartet auf deine Antwort: {waiting}",
  "co.contacts.board.nobodyHolds": "Niemand hat diese Rolle",
  "co.contacts.band.noOpenDeal": "Kein offener Deal",
  "co.contacts.band.noOpenDealWhy": "Rollen werden pro Deal vergeben",
  "co.contacts.band.committeePartial": "Teilweise ausgeblendet",
  "co.contacts.board.otherRoles": "Weitere Rollen",
  "co.contacts.band.unavailable": "Nicht verfügbar",
  "co.contacts.band.unavailableWhy":
    "Laden fehlgeschlagen · Liste nicht betroffen",
  "co.contacts.view": "Ansicht des Buying Centers",
  "co.contacts.view.board": "Board",
  "co.contacts.view.map": "Karte",
  "co.contacts.map.region": "Kontaktkarte für dieses Unternehmen",
  "co.contacts.map.bestRoute": "Bester Zugang",
  "co.contacts.map.alternatives": "Alternativen",
  "co.contacts.map.noRoute": "Kein Weg erfasst",
  "co.contacts.map.more": "{count} weitere anzeigen",
  "co.contacts.map.clear": "Auswahl aufheben",
  "co.contacts.map.emptyTitle": "Noch kein Weg erfasst",
  "co.contacts.map.emptyBody":
    "Vergib Rollen im Kaufprozess oder importiere die bisherigen Interaktionen dieses Unternehmens.",
  "co.contacts.map.nothingSelected":
    "Wähle einen Kontakt, um den besten Weg zu diesem Kontakt zu sehen.",
  "co.contacts.map.ourSide": "Dein Team",
  "co.contacts.map.account": "Unternehmen",
  "co.contacts.map.missing": "{role} fehlt",
  "co.contacts.map.awaiting": "wartet auf Antwort",
  "co.contacts.map.owed": "Antwort ausstehend",
  "co.contacts.map.replied": "hat geantwortet",
  "co.contacts.map.never": "nie kontaktiert",
  "co.contacts.map.onDeal": "am Deal beteiligt",
  "co.contacts.map.routesWithheld":
    "Wege zu diesem Kontakt sind für dich ausgeblendet",
  "co.contacts.map.assignHint": "Für diesen Deal ist niemand zuständig",
  "co.contacts.map.scope":
    "{count} im Buying Center · nur der ausgewählte Deal.",
  "co.contacts.map.scopePartial":
    "{count} im Buying Center · {hidden} weitere, die du nicht sehen kannst.",
  "co.contacts.board.readFromMessages":
    "Aus den Nachrichten des Kontakts gelesen",
  "co.intro.title": "Vorstellung anfragen",
  "co.intro.who": "{colleague} wird gebeten, dich {contact} vorzustellen.",
  "co.intro.write": "Nachricht entwerfen",
  "co.intro.writing": "Wird geschrieben…",
  "co.intro.fromTemplate":
    "Aus einer Vorlage geschrieben. In dieser Installation ist kein Modell eingerichtet.",
  "co.intro.subject": "Betreff",
  "co.intro.body": "Nachricht",
  "co.intro.basedOn": "Grundlage",
  "co.intro.copy": "Kopieren",
  "co.intro.copyFailed":
    "Kopieren fehlgeschlagen. Markiere die Nachricht und kopiere sie von Hand.",
  "co.intro.copied": "Kopiert",
  "co.intro.openMail": "In E-Mail-Programm öffnen",
  "co.map.askIntro": "Vorstellung anfragen",
  "co.contacts.board.suggest": "Rollen vorschlagen",
  "co.contacts.board.suggesting": "Nachrichten werden gelesen…",
  "co.contacts.board.suggestNoDeal":
    "Rollen werden pro Deal vergeben, und dieses Unternehmen hat keinen offenen Deal.",
  "co.contacts.board.suggestWrote_one":
    "{count} Rolle aus Nachrichten zugewiesen",
  "co.contacts.board.suggestWrote_other":
    "{count} Rollen aus Nachrichten zugewiesen",
  "co.contacts.board.suggestUnavailable":
    "Rollenvorschläge brauchen ein Modell, und in dieser Installation ist keines eingerichtet.",
  "co.contacts.board.suggestNothing":
    "Aus den Nachrichten geht nicht hervor, wer kauft.",
  "co.contacts.board.suggestRefused_one":
    "Nichts war eindeutig genug zum Erfassen. {count} Vorschlag wurde wegen schwacher Belege verworfen.",
  "co.contacts.board.suggestRefused_other":
    "Nichts war eindeutig genug zum Erfassen. {count} Vorschläge wurden wegen schwacher Belege verworfen.",
  "co.contacts.board.confirm": "Bestätigen",
  "co.contacts.board.confirming": "Wird bestätigt…",
  "co.contacts.board.change": "Rolle ändern",
  "co.reach.waiting": "Antwort nötig",
  "co.reach.answered": "Beantwortet",
  "co.reach.silent": "Keine Antwort",
  "co.reach.lapsed": "Verstummt",
  "co.reach.untried": "Nicht kontaktiert",
  "co.role.champion": "Champion",
  "co.role.economic_buyer": "Economic Buyer",
  "co.role.blocker": "Blocker",
  "co.role.influencer": "Influencer",
  "co.role.user": "Endnutzende",
  "co.roleLabel.champion": "Champion",
  "co.roleLabel.economic_buyer": "Economic Buyer",
  "co.roleLabel.blocker": "Blocker",
  "co.roleLabel.influencer": "Influencer",
  "co.roleLabel.user": "Endnutzende",
  "co.evidence.extractedUnconfirmed": "Von KI extrahiert · nicht bestätigt",
  "co.evidence.previous": "Vorherige Aussage",
  "co.evidence.next": "Nächste Aussage",
  "co.evidence.title": "Quelle",
  "co.prep.withheld":
    "Teile dieses Unternehmens sind für dich ausgeblendet, daher ist diese Zusammenfassung unvollständig.",
  "co.read.newActivity_one": "1 neuer Eintrag seit deinem letzten Besuch.",
  "co.read.newActivity_other":
    "{count} neue Einträge seit deinem letzten Besuch.",
  "co.factField.founded_year": "Gegründet",
  "co.factField.employee_range": "Mitarbeitende",
  "co.factField.phone": "Telefon",
  "co.factField.contact_email": "Kontakt-E-Mail",
  "co.factField.location": "Standort",
  "co.factField.service": "Dienstleistung",
  "co.factField.product": "Produkt",
  "co.factField.capability": "Fähigkeit",
  "co.factField.served_industry": "Bedient",
  "co.factField.company_size": "Größe",
  "co.factField.geography": "Region",
  "co.factField.language": "Sprache",
  "co.factField.certification": "Zertifizierung",
  "co.factField.partner": "Partner",
  "co.factField.named_customer": "Referenzkunde",
  "co.factField.technology": "Technologie",
  "co.factField.mail_provider": "E-Mail-System",
  "co.factField.email_security": "E-Mail-Authentifizierung",
  "co.factField.hosting_provider": "Hosting",
  "co.factField.operated_service": "Betriebener Dienst",
  "co.vat.markVerdict": "USt-ID: {verdict}",
  "co.vat.markUnchecked": "USt-ID: noch nicht beim Register geprüft",
  "co.vat.markUnreadable":
    "USt-ID: Das Prüfergebnis wurde nicht geladen. Zum erneuten Versuch auswählen.",
  "co.vat.numberMoved":
    "Die Nummer in diesem Datensatz hat sich nach dieser Prüfung geändert. Prüfe die neue Nummer beim Register.",
  "co.vat.verdict": "Ergebnis des Registers",
  "co.vat.number": "Abgefragte Nummer",
  "co.vat.registeredName": "Eingetragen auf",
  "co.vat.registeredAddress": "Eingetragene Anschrift",
  "co.vat.checkedAt": "Abgefragt am",
  "co.vat.receipt": "Abfragenummer",
  "co.vat.status.valid": "Gültig",
  "co.vat.status.invalid": "Nicht gültig",
  "co.vat.noReceipt":
    "Keine vergeben. Das Register vergibt eine Abfragenummer nur für Prüfungen unter der eigenen USt-ID deines Unternehmens. Trage sie in den Einstellungen ein, damit spätere Prüfungen einen Nachweis enthalten, den eine Steuerbehörde akzeptiert.",
  "co.vat.never":
    "Die USt-ID dieses Unternehmens wurde noch nicht geprüft. Das geschieht automatisch, sobald die Nummer aus dem Impressum des Unternehmens gelesen wird, oder du prüfst sie jetzt.",
  "co.vat.askNow": "Beim Register prüfen",
  "co.vat.askAgain": "Erneut prüfen",
  "co.vat.askingBusy": "Wird beim Register geprüft…",
  "co.vat.asking":
    "Wird beim Register geprüft. Das Ergebnis erscheint hier, sobald das Register antwortet.",
  "co.tech.title": "Technologie",
  "co.tech.sub":
    "Was dieses Unternehmen öffentlich einsetzt, gelesen aus DNS-Einträgen, Zertifikaten und Startseite.",
  "co.tech.mail": "E-Mail",
  "co.tech.web": "Website-Technologie",
  "co.tech.services": "Dienste",
  "co.tech.hosting": "Hosting",
  "co.tech.empty":
    "Noch keine technischen Daten. Diese werden automatisch ergänzt und aktualisiert, sobald die Website des Unternehmens gelesen wird.",
  "co.tech.laneFailed":
    "{lane} hat nicht geantwortet. Das vorherige Ergebnis ist unverändert.",
  "co.tech.laneRefused": "Die Website hat das Lesen blockiert.",
  "co.tech.lane.dns": "DNS",
  "co.tech.lane.certlog": "Zertifikate",
  "co.tech.lane.homepage": "Startseite",
  "signal.kind.technical_change": "Technologiewechsel",
  "co.factField.quantified_outcome": "Ergebnis",
  "co.facts.title": "Unternehmensfakten",
  "co.facts.empty":
    "Noch keine Fakten. Lies die Website oder ergänze, was du weißt.",
  "co.facts.add": "Fakt hinzufügen",
  "co.facts.addField": "Art des Fakts",
  "co.facts.addValue": "Wert",
  "co.facts.addSave": "Fakt speichern",
  "co.facts.addCancel": "Abbrechen",
  "co.facts.addIncomplete": "Wähle eine Art des Fakts und gib einen Wert ein.",
  "co.facts.remove": "{value} entfernen",
  "co.facts.removeTitle": "Diesen Fakt entfernen?",
  "co.facts.removeConfirm": "Entfernen",
  "co.facts.removeAsk":
    "{field} ist als „{value}“ erfasst. Durch das Entfernen gilt das nicht mehr als Fakt über das Unternehmen. Ein späteres Lesen der Website kann es wieder hinzufügen.",
  "co.facts.showAll": "Alle {count} anzeigen",
  "co.facts.showLess": "Weniger anzeigen",
  "co.project.new": "Neues Projekt",
  "co.deal.new": "Neuer Deal",
  "co.recent.title": "Letzte Aktivitäten",
  "co.recent.emptyDetail":
    "Gesendete E-Mails, erfasste Anrufe und Termine erscheinen hier, jeweils mit Angabe, wer auf welcher Seite was getan hat.",
  "co.recent.empty": "Noch keine Aktivitäten erfasst.",
  "co.recent.kind.email": "E-Mail",
  "co.recent.kind.call": "Anruf",
  "co.recent.kind.meeting": "Termin",
  "co.recent.kind.note": "Notiz",
  "co.recent.kind.task": "Aufgabe",
  "co.recent.kind.message": "Nachricht",
  "co.recent.dir.theyWrote": "die Gegenseite hat geschrieben",
  "co.recent.dir.weSent": "dein Team hat gesendet",
  "co.recent.dir.theyCalled": "die Gegenseite hat angerufen",
  "co.recent.dir.weCalled": "dein Team hat angerufen",
  "co.recent.dir.both": "beide Seiten",
  "co.recent.minutes": "{count} min",
  "co.recent.re": "zu einem Deal",
  "co.recent.reNamed": "zu {name}",
  "tagAdmin.title": "Tags",
  "tagAdmin.sub":
    "Tags, unter denen dieses Unternehmen Datensätze ablegt. Alle können ein Tag vergeben. Nur Admins und Operations können Tags anlegen, umbenennen oder stilllegen.",
  "tagAdmin.listLabel": "Tag-Liste",
  "tagAdmin.empty": "Noch keine Tags. Lege das erste Tag an.",
  "import.contextTag": "Tag für diesen Import",
  "import.contextTagChosen":
    "Datensätze, die dieser Import anlegt, werden unter {name} abgelegt.",
  "import.contextTagChosenUnnamed":
    "Datensätze, die dieser Import anlegt, werden unter dem für diesen Lauf gewählten Tag abgelegt.",
  "import.contextTagHint":
    "Wird auf angelegte Datensätze angewendet, damit der Stapel auffindbar bleibt. Aktualisierte Datensätze behalten ihre Tags.",
  "import.contextTagNone": "Kein Tag",
  "tagAdmin.add": "Tag anlegen",
  "tagAdmin.addTitle": "Tag anlegen",
  "tagAdmin.editTitle": "Tag bearbeiten",
  "tagAdmin.nameLabel": "Name",
  "tagAdmin.colorLabel": "Farbe",
  "tagAdmin.colorNone": "Keine Farbe",
  "tagAdmin.color.teal": "Petrol",
  "tagAdmin.color.amber": "Bernstein",
  "tagAdmin.color.rose": "Rosé",
  "tagAdmin.color.slate": "Schiefer",
  "tagAdmin.color.sky": "Himmelblau",
  "tagAdmin.color.violet": "Violett",
  "tagAdmin.color.lime": "Limette",
  "tagAdmin.color.orange": "Orange",
  "tagAdmin.create": "Anlegen",
  "tagAdmin.save": "Speichern",
  "tagAdmin.edit": "Bearbeiten",
  "tagAdmin.merge": "Zusammenführen",
  "tagAdmin.archive": "Stilllegen",
  "tagAdmin.restore": "Wiederherstellen",
  "tagAdmin.usage": "Datensätze: {count}",
  "tagAdmin.usagePending": "Wird gezählt…",
  "tagAdmin.nearMatchTitle": "Ähnliches Tag vorhanden",
  "tagAdmin.nearMatch":
    "{names}. Vergib das vorhandene Tag, es sei denn, dieses meint etwas anderes.",
  "tagAdmin.mergeTitle": "{name} mit einem anderen Tag zusammenführen",
  "tagAdmin.mergeIntoLabel": "Dieses Tag behalten",
  "tagAdmin.mergeIntoNone": "Tag auswählen",
  "tagAdmin.mergeConfirm": "Zusammenführen",
  "tagAdmin.mergeWarningTitle": "Das lässt sich nicht rückgängig machen",
  "tagAdmin.mergeWarning":
    "Datensätze mit dem Tag {name} erhalten stattdessen das andere Tag, und der Name wird wieder frei.",
  "tagAdmin.mergedTitle": "Zusammengeführt",
  "tagAdmin.mergedBody":
    "Zum behaltenen Tag verschobene Datensätze: {moved}. Entfernte Duplikate, wo ein Datensatz bereits beide Tags hatte: {collapsed}.",
  "tagAdmin.countUsage": "Datensätze zählen",
  "tagAdmin.noVersion":
    "Dieses Tag wurde ohne Version geladen und kann nicht gespeichert werden. Lade die Seite neu und versuche es erneut.",
  "tagAdmin.withheld":
    "Du hast keinen Zugriff auf die Tags dieses Unternehmens.",
  "tagAdmin.truncatedTitle": "Liste gekürzt",
  "tagAdmin.truncated":
    "Tags jenseits des Limits werden nicht angezeigt und lassen sich weder bearbeiten noch als Ziel einer Zusammenführung wählen.",
  "tagAdmin.usageFailed": "Zählung nicht verfügbar",
  "tagAdmin.changeFailed": "Das Tag wurde nicht geändert. Versuche es erneut.",
  "tagAdmin.done": "Fertig",
  "tags.archived": "archiviert",
  "tags.columnHeader": "Tags",
  "tags.filterAll": "Beliebiges Tag",
  "tags.moreUncounted": "weitere",
  "tags.moreUncountedTip":
    "Darunter {names}. Öffne den Datensatz für alle Tags.",
  "tags.columnHeaderPartial": "Tags (unvollständig)",
  "tags.loading": "Tags werden geladen…",
  "tags.panelTitle": "Tags",
  "tags.add": "Tag hinzufügen",
  "tags.more": "+{count} weitere",
  "tags.showLess": "Weniger anzeigen",
  "tags.removeTag": "{name} entfernen",
  "tags.removeTitle": "{name} von diesem Datensatz entfernen?",
  "tags.addedBy": "Hinzugefügt von {who} · {when}",
  "tags.addedByUndated": "Hinzugefügt von {who}",
  "tags.addedOn": "Hinzugefügt am {when}",
  "tags.visibleWorkspaceWide":
    "Tag-Namen sind im gesamten Unternehmen sichtbar.",
  "tags.removeFromRecord": "Von diesem Datensatz entfernen",
  "tags.withheld": "Für deine Rolle ausgeblendet",
  "tags.emptyTitle": "Noch keine Tags",
  "tags.emptyBody":
    "Füge dauerhaften Kontext hinzu, etwa eine Veranstaltung, eine Beziehung oder eine Kohorte.",
  "tags.pickerLabel": "Tags suchen",
  "tags.alreadyAdded": "Bereits hinzugefügt",
  "tags.catalogTruncatedTitle": "Liste gekürzt",
  "tags.catalogTruncated":
    "Möglicherweise fehlt ein Tag. Suche nach dem Namen, bevor du ein neues anfragst.",
  "tags.noMatch":
    "Kein Tag mit diesem Namen. Admins und Operations können eines anlegen.",
  "tagResult.gone":
    "Dieses Tag existiert nicht mehr. Es wurde möglicherweise mit einem anderen zusammengeführt.",
  "tagResult.totalVisible": "Sichtbare Zuordnungen: {count}",
  "tagResult.contacts": "Kontakte",
  "tagResult.companies": "Unternehmen",
  "tagResult.deals": "Deals",
  "tagResult.viewAll": "Alle {count} {kind} anzeigen",
  "tagResult.resultsTitle": "Datensätze mit diesem Tag",
  "tagResult.nothingCarries":
    "Noch keine Datensätze mit diesem Tag. Vergib es bei einem Kontakt, einem Unternehmen oder einem Deal.",
  "tagResult.loadingRows": "{kind} werden geladen…",
  "tagResult.noneLeft": "Keine Datensätze mehr mit diesem Tag",
  "tagResult.unnamed": "Ohne Namen",
  "co.timeline.empty": "Noch keine Aktivitäten zu diesem Unternehmen erfasst.",
  "company.domains": "Domains",
  "company.factCategory.company": "Unternehmen",
  "company.factCategory.offering": "Angebot",
  "company.factCategory.market": "Markt",
  "company.factCategory.signal": "Signale",

  "lead.score": "Score",
  "lead.status": "Status",
  "lead.nextTask": "Nächster Schritt",
  "lead.openTaskCount": "Offene Aufgaben: {count}",
  "lead.noNextTask": "Kein nächster Schritt",
  "lead.scoreNoSignals": "Keine Signale",
  "lead.source": "Quelle",
  "lead.project": "Projekt",

  "lead.filterSource": "Quelle",
  "lead.filterSourceAll": "Alle Quellen",
  "lead.source.manual": "Manuell angelegt",
  "lead.source.inbound": "Eingehend",
  "lead.source.webform": "Webformular",
  "lead.source.referral": "Empfehlung",
  "lead.source.import": "Import",
  "lead.source.crawl": "Webrecherche",
  "lead.source.unknown": "Unbekannte Quelle",
  "lead.sourceFromConnector":
    "Von einem Connector geschrieben, der seine eigene Quelle behält.",
  "leadSources.title": "Lead-Quellen",
  "leadSources.sub":
    "Woher Leads kommen. Wird im Formular „Neuer Lead“, in Filtern und beim Scoring verwendet.",
  "leadSources.readOnly":
    "Nur Admins und Operations können diese Liste ändern.",
  "leadSources.readOnlyTitle":
    "Nur Admins und Operations können diese Liste ändern",
  "leadSources.notSaved": "Änderung nicht gespeichert",
  "leadSources.notAdded": "Quelle nicht hinzugefügt",
  "leadSources.labelFor": "Bezeichnung der Quelle {key}",
  "leadSources.intentFor": "Kaufinteresse von {label}",
  "leadSources.intent": "Kaufinteresse",
  "leadSources.intent.high": "Hohes Kaufinteresse",
  "leadSources.intent.neutral": "Neutral",
  "leadSources.intent.low": "Geringes Kaufinteresse",
  "leadSources.intentHint":
    "Hoch fügt dem Score Punkte hinzu, Gering zieht Punkte ab. Änderungen gelten ab der nächsten Neuberechnung jedes Leads.",
  "leadSources.leadCount": "Leads: {count}",
  "leadSources.builtIn": "Vorgegeben",
  "leadSources.builtInKept":
    "Vorgegebene Quellen lassen sich umbenennen oder deaktivieren, aber nicht entfernen.",
  "leadSources.inUse_one":
    "{count} Lead nutzt diese Quelle. Deaktiviere sie stattdessen.",
  "leadSources.inUse_other":
    "{count} Leads nutzen diese Quelle. Deaktiviere sie stattdessen.",
  "leadSources.deactivateInstead": "stattdessen deaktivieren",
  "leadSources.activeFor": "{label} ist aktiv",
  "leadSources.remove": "Entfernen",
  "leadSources.removeTitle": "Quelle entfernen?",
  "leadSources.removeBody":
    "„{label}“ wird von keinem Lead verwendet und wird aus der Liste entfernt.",
  "leadSources.newLabel": "Neue Quelle",
  "leadSources.labelField": "Bezeichnung",
  "leadSources.addOpen": "Neue Quelle",
  "leadSources.listLabel": "Quellen in der Liste",
  "leadSources.discovered": "Gefundene Werte",
  "leadSources.newPlaceholder": "Messe",
  "leadSources.add": "Quelle hinzufügen",
  "leadSources.discoveredSub":
    "Werte auf Leads aus Connectors und Importen, die noch nicht in der Liste stehen. Füge einen hinzu, um ihm Bezeichnung und Gewichtung zu geben.",
  "leadSources.adopt": "Zur Liste hinzufügen",
  "leadReasons.title": "Disqualifizierungsgründe",
  "leadReasons.sub":
    "Was Vertriebsmitarbeitende beim Disqualifizieren eines Leads wählen. Der Grund steht am Lead und lässt sich filtern.",
  "leadReasons.labelFor": "Bezeichnung des Grunds {label}",
  "leadReasons.leadCount": "Leads: {count}",
  "leadReasons.inUse_one":
    "{count} Lead hat diesen Grund. Deaktiviere ihn stattdessen.",
  "leadReasons.inUse_other":
    "{count} Leads haben diesen Grund. Deaktiviere ihn stattdessen.",
  "leadReasons.newLabel": "Neuer Grund",
  "leadReasons.listLabel": "Gründe in der Liste",
  "leadReasons.add": "Grund hinzufügen",
  "leadReasons.removeTitle": "Grund entfernen?",
  "leadReasons.removeBody":
    "„{label}“ wird von keinem Lead verwendet und wird aus der Liste entfernt.",
  "leadHandling.title": "Lead-Bearbeitung",
  "leadHandling.sub": "Wie neue Leads bearbeitet werden.",
  "leadHandling.firstResponse": "Zielzeit für die erste Antwort",
  "leadHandling.firstResponseHint":
    "Standardmäßig aus. Eingeschaltet erhält jeder offene Lead eine Antwortfrist, die Liste bekommt die Ansicht „Überfällig“, und überfällige Leads stehen oben.",
  "leadHandling.targetMinutes": "Zielzeit (Minuten)",
  "leadHandling.targetOutOfRange":
    "Gib eine ganze Zahl an Minuten von 15 bis 10.080 (7 Tage) ein.",
  "leadHandling.targetHint":
    "Längste Wartezeit auf eine erste Antwort nach Zuweisung oder Anlage, 15 Minuten bis 7 Tage.",
  "lead.boardCount": "Leads: {count}",
  "lead.duplicateFound":
    "Ein Lead mit dieser E-Mail oder diesem LinkedIn-Profil ist bereits vorhanden.",
  "lead.promote": "Qualifizieren",
  "lead.promoteIneligible":
    "Erfordert eine E-Mail-Adresse und einen offenen Status.",
  "lead.filterStatus": "Status",
  "lead.filterStatusAll": "Alle Status",
  "lead.filterScore": "Score",
  "lead.filterScoreAll": "Beliebiger Score",
  "lead.bulkSelected": "{count} ausgewählt",
  "lead.bulkOwner": "Neue Zuständigkeit",
  "lead.bulkOwnerPick": "Zuständigkeit wählen",
  "lead.bulkAssign": "Zuweisen",
  "lead.bulkDisqualify": "Disqualifizieren",
  "lead.bulkDisqualifyTitle_one": "Diesen Lead disqualifizieren?",
  "lead.bulkDisqualifyTitle_other": "{count} Leads disqualifizieren?",
  "lead.bulkDisqualifyBody":
    "Geschlossen mit dem Grund „{reason}“. Jeder Lead behält seinen Datensatz, und die Leads lassen sich nicht in einem Schritt wieder öffnen.",
  "lead.bulkFailed": "{count} nicht übernommen:",
  "lead.bulkFailedRow": "nicht gespeichert",
  "lead.bulkOutcomeConflict": "inzwischen von jemand anderem geändert",
  "lead.bulkOutcomeForbidden": "keine Berechtigung zum Neuzuweisen",
  "lead.bulkOutcomeNotFound": "nicht mehr in deiner Liste",
  "lead.bulkSelectRow": "{name} auswählen",
  "lead.unnamed": "Lead ohne Namen",
  "lead.timeline.empty": "Zu diesem Lead ist noch keine Aktivität erfasst.",
  "lead.sla.breached": "Überfällig",
  "lead.sla.atRisk": "Bald fällig",
  "lead.sla.withinTarget": "In der Zielzeit",
  "lead.sla.answeredAt": "{at}",
  "lead.sla.dueBy": "Fällig {at}",
  "lead.sla.overdueSince": "War fällig am {at}",
  "lead.filterSla": "Antwort",
  "lead.filterSlaAll": "Alle",
  "list.viewOverdue": "Überfällig",
  "lead.filterScoreHot": "Ab 80",
  "lead.filterScoreWarm": "Ab 60",
  "lead.filterScoreCool": "Ab 40",
  "lead.details": "Details",
  "lead.rail.deal.title": "Deal",
  "lead.rail.deal.empty":
    "Noch kein Deal. Beim Qualifizieren dieses Leads kann einer entstehen.",
  "lead.rail.project.title": "Projekt",
  "lead.rail.project.empty": "Noch kein Projekt.",
  "lead.rail.project.attach": "Projekt verknüpfen",
  "lead.rail.project.change": "Projekt ändern",
  "lead.terminalReadOnly":
    "Dieser Lead ist geschlossen und lässt sich nicht ändern.",
  "lead.notYoursToChange":
    "Du kannst diesen Lead nicht bearbeiten. Frage die zuständige Person, ob sie ihn mit dir teilt, oder einen Admin nach Bearbeitungsrechten.",
  "lead.boardCountsUnavailable":
    "Die Anzahlen für Qualifiziert und Disqualifiziert wurden nicht geladen.",
  "lead.boardTerminalRowsUnavailable":
    "Diese Leads wurden nicht geladen. Die Anzahl oben stimmt weiterhin.",
  "lead.boardTerminalOnly":
    "Hier sind keine offenen Leads. Die Leads zählen unter Qualifiziert und Disqualifiziert.",

  "lead.mergedTitle": "Mit einem anderen Lead zusammengeführt",
  "lead.mergedBody":
    "Dieser Lead ist ein Duplikat eines anderen Leads. Verlauf, Einwilligung und Score wurden dorthin übertragen. Dieser Datensatz bleibt als Verlauf erhalten.",
  "lead.promotedTitle": "Als Kontakt qualifiziert",
  "lead.promotedMerged":
    "Dieser Lead wurde mit einem bestehenden Kontakt zusammengeführt. Es wurde kein Duplikat angelegt.",
  "lead.promotedCreated": "Aus diesem Lead wurde ein neuer Kontakt.",
  "lead.promotedAt": "Qualifiziert",
  "lead.promotedTrigger": "Auslöser:",
  "lead.promotedEvidence": "Beleg:",
  "lead.previewPending": "Suche nach einem bestehenden Kontakt…",
  "lead.previewCreate": "Das Qualifizieren legt einen neuen Kontakt an.",
  "lead.previewMerge":
    "Das Qualifizieren führt den Lead mit dem bestehenden Kontakt zusammen",
  "lead.previewMergeWithheld":
    "Das Qualifizieren führt den Lead mit einem bestehenden Kontakt zusammen, den du nicht sehen kannst.",
  "lead.demote": "Qualifizierung zurücknehmen",
  "lead.demoteDialog": "Qualifizierung zurücknehmen?",
  "lead.demoteExplain":
    "Der Lead erhält wieder den Status „Im Gespräch“. Ein bei der Qualifizierung angelegter Kontakt wird archiviert, ein zusammengeführter Kontakt bleibt unverändert. Ist der Kontakt an einem laufenden Deal beteiligt, lässt sich die Qualifizierung nicht zurücknehmen.",
  "lead.demoteReason": "Grund (wird im Audit-Log erfasst)",
  "lead.demoteReasonRequired": "Gib zuerst einen Grund ein.",
  "lead.demoteConfirm": "Zurücknehmen",
  "lead.reopen": "Wieder öffnen",
  "lead.writeRefused": "Änderung nicht gespeichert",
  "lead.reopenDialog": "Lead wieder öffnen?",
  "lead.reopenExplain":
    "Der Lead erhält wieder seinen Status von vor der Disqualifizierung, und der Grund wird entfernt. Verlauf und Score bleiben erhalten.",
  "lead.reopenConfirm": "Lead wieder öffnen",
  "lead.promotedOutcomePending": "Ergebnis der Qualifizierung wird geladen…",
  "lead.promotedOutcomeUnavailable":
    "Das Ergebnis der Qualifizierung kann nicht angezeigt werden.",
  "lead.terminalPromoted": "Qualifiziert. Dieser Lead ist schreibgeschützt.",
  "lead.statusNew": "Neu",
  "lead.statusContacted": "Kontaktiert",
  "lead.statusEngaged": "Im Gespräch",
  "lead.statusPromoted": "Qualifiziert",
  "lead.statusDisqualified": "Disqualifiziert",
  "lead.disqualified": "Disqualifiziert",
  "lead.merged": "Zusammengeführt",
  "lead.status.new": "Neu",
  "lead.status.contacted": "Kontaktiert",
  "lead.status.engaged": "Im Gespräch",
  "lead.explainScore": "Score erklären",
  "lead.scoreOverridden": "Manuell überschrieben: {reason}",
  "lead.machineScore": "Modell-Score: {score}",
  "lead.overrideScore": "Score überschreiben",
  "lead.clearOverride": "Überschreibung aufheben",
  "lead.overrideReason": "Grund",
  "lead.shortfall.lead": "Eingaben für den Score:",
  "lead.shortfall.engagementMoves":
    "Eine Antwort oder ein Termin hebt den Score am stärksten.",
  "lead.shortfall.noSource": "Keine Quelle hinterlegt.",
  "lead.shortfall.sourcePenalised": "Die Quelle „{source}“ senkt den Score.",
  "lead.shortfall.noTitle": "Keine Position hinterlegt.",
  "lead.shortfall.titleNotSenior":
    "„{title}“ ist keine Führungsposition, die das Modell erkennt.",
  "lead.shortfall.sourceNoIntent":
    "Die Quelle „{source}“ zeigt für sich allein kein Kaufinteresse an.",
  "lead.scoreNotStoredYet":
    "Die Aufschlüsselung dieses Scores ist noch nicht gespeichert. Die nächste Aktualisierung zeigt sie.",
  "lead.scoreLoading": "Score-Faktoren werden geladen…",
  "lead.scoreNoFactors": "Bisher fließen keine Faktoren in diesen Score ein.",
  "lead.scoreFactorsFailed": "Die Score-Faktoren wurden nicht geladen.",
  "lead.scoreFactorsExplainMachine":
    "Du hast diesen Score manuell gesetzt. Die Faktoren unten erklären den Modell-Score: {score}.",
  "lead.scoreDecayed": "{base}, halbiert alle 14 Tage",
  "lead.scoreSources": "Aktivitäten: {count}",
  "lead.scoreReconciles": "Summe {raw}, gerundet auf {rounded}, Score {score}",
  "lead.factor.decision_maker_title": "Position mit Entscheidungsbefugnis",
  "lead.factor.high_intent_source": "Quelle mit hohem Kaufinteresse",
  "lead.factor.low_intent_source": "Quelle mit geringem Kaufinteresse",
  "lead.factor.reply": "Lead hat geantwortet",
  "lead.factor.meeting_held": "Termin stattgefunden",
  "lead.factor.meeting_booked": "Termin gebucht",
  "lead.signalsTitle": "Lead-Signale",
  "lead.signalUnset": "Nicht angegeben",
  "lead.signalClear": "Zurückziehen",
  "lead.signalBandPick": "Wert wählen",
  "lead.signalMore": "Mehr",
  "lead.signalProvenanceHint":
    "Bleibt das unverändert, wird die Antwort als Schätzung ohne Konfidenzangabe gespeichert.",
  "lead.signalEvidenceQuality": "Qualität des Belegs",
  "lead.signalConfidence": "Konfidenz",
  "lead.signalConfidenceUnstated": "Keine Angabe",
  "lead.signalConfidenceValue": "{value} % Konfidenz",
  "lead.signalRecordedAt": "Erfasst {at}",
  "lead.signalSuperseded": "Zuvor {value}, ersetzt durch {source}",
  "lead.signalAutomaticSource": "eine automatische Quelle",
  "lead.signalReason": "Beleg",
  "lead.signalReasonHint": "Optional. Wird mit dem Score gespeichert.",
  "lead.signalReasonUnstated": "Kein Beleg angegeben. Manuell erfasst.",
  "lead.signalSave": "Zum Score hinzufügen",
  "lead.signal.web_traffic": "Website-Traffic",
  "lead.signal.employees": "Mitarbeitende",
  "lead.signal.budget_hint": "Budget",
  "lead.signal.ask.web_traffic": "Website-Traffic?",
  "lead.signal.ask.employees": "Unternehmensgröße?",
  "lead.signal.ask.budget_hint": "Budget?",
  "lead.factorKind.fact": "Bestätigt",
  "lead.factorKind.assumption": "Geschätzt",
  "lead.factorKind.judgement": "Bewertung",
  "lead.signal.fact": "Bestätigt",
  "lead.signal.assumption": "Geschätzt",
  "lead.signal.judgement": "Bewertung",
  "lead.signal.web_traffic.low": "Niedrig",
  "lead.signal.web_traffic.medium": "Mittel",
  "lead.signal.web_traffic.high": "Hoch",
  "lead.signal.employees.1-10": "1 bis 10",
  "lead.signal.employees.11-50": "11 bis 50",
  "lead.signal.employees.51-200": "51 bis 200",
  "lead.signal.employees.201+": "201+",
  "lead.signal.budget_hint.none": "Kein Budget",
  "lead.signal.budget_hint.unknown": "Unbekannt",
  "lead.signal.budget_hint.some": "Etwas Budget",
  "lead.signal.budget_hint.confirmed": "Budget bestätigt",
  "lead.factor.manual:web_traffic": "Website-Traffic (manuell)",
  "lead.factor.manual:employees": "Mitarbeitende (manuell)",
  "lead.factor.manual:budget_hint": "Budget (manuell)",
  "lead.ownerLabel": "Zuständig",
  "lead.ownerYou": "Du",
  "lead.overriddenBadge": "überschrieben",
  "lead.unassigned": "Nicht zugewiesen",
  "lead.terminalDisqualified":
    "Disqualifiziert. Dieser Lead ist schreibgeschützt.",
  "lead.marker": "Lead",
  "lead.assign": "Zuweisen",
  "lead.assignToMe": "Mir zuweisen",
  "lead.assignTo": "Diesen Lead zuweisen an",
  "lead.assignChoose": "Teammitglied wählen",
  "lead.assignNobodyElse":
    "Kein weiteres Teammitglied, dem dieser Lead zugewiesen werden kann.",
  "lead.saveOverride": "Überschreibung speichern",
  "lead.overrideScoreValue": "Score",
  "lead.trigger.inboundReply": "Eingehende Antwort",
  "lead.trigger.meetingBooked": "Termin gebucht",
  "lead.trigger.meetingHeld": "Termin stattgefunden",
  "lead.trigger.humanQualify": "Manuell qualifiziert",
  "lead.evidenceNote": "Notiz zum Beleg (optional)",
  "lead.segregationTitle": "Leads bleiben von Kontakten getrennt",
  "lead.segregation":
    "Ein Lead wird erst zum Kontakt, wenn du ihn qualifizierst.",
  "lead.segregationDismiss": "Hinweis ausblenden",
  "list.emptyMine": "Keine {unit}, für die du zuständig bist.",
  "list.showAll": "Alle anzeigen",
  "lead.assignedAway":
    "{names} an {owner} zugewiesen. Nicht mehr unter „Meine“.",
  "lead.viewNew": "Neu",
  "lead.viewNewUnassigned": "Neu und nicht zugewiesen",
  "lead.viewUnassigned": "Nicht zugewiesen",
  "lead.viewNeedsFollowUp": "Follow-up nötig",
  "lead.viewEngaged": "Im Gespräch",
  "lead.ladder": "Lead-Status",
  "lead.ladder.new": "Neu: noch keine Kontaktaufnahme.",
  "lead.ladder.automatic":
    "{label} · automatisch aus erfasster Aktivität gesetzt",
  "lead.ladder.automaticWith": "{label} · automatisch gesetzt: {what} am {at}",
  "lead.ladder.byHand": "{label} · manuell gesetzt",
  "lead.ladder.theyReplied": "Antwort des Leads",
  "lead.ladder.meetingBooked": "Termin gebucht",
  "lead.ladder.meetingHeld": "Termin stattgefunden",
  "lead.ladder.qualified": "Qualifiziert: Dieser Lead ist jetzt ein Kontakt.",
  "lead.ladder.qualifiedOn":
    "Qualifiziert am {at}: Dieser Lead ist jetzt ein Kontakt.",
  "lead.ladder.disqualified": "Disqualifiziert.",
  "lead.ladder.disqualifiedWithReason": "Disqualifiziert: {reason}",
  "lead.qualify.title": "{name} qualifizieren",
  "lead.qualify.contact": "Kontakt",
  "lead.qualify.alsoDeal": "Auch einen Deal anlegen",
  "lead.qualify.pipeline": "Pipeline",
  "lead.qualify.stage": "Phase",
  "lead.qualify.dealName": "Deal-Name",
  "lead.qualify.amount": "Betrag ({currency})",
  "lead.qualify.amountHint":
    "Optional. Ganze Einheiten in der Basiswährung der Installation.",
  "lead.qualify.amountInvalid": "Gib eine Zahl ein oder lass das Feld leer.",
  "lead.qualify.amountNoCurrency":
    "Die Basiswährung ist noch nicht geladen. Versuche es gleich erneut oder lass den Betrag leer.",
  "lead.qualify.why": "Warum",
  "lead.qualify.reasonReplied": "Grund: Der Lead hat am {at} geantwortet.",
  "lead.qualify.reasonMeetingBooked":
    "Grund: Ein Termin wurde für {at} gebucht.",
  "lead.qualify.reasonMeetingHeld":
    "Grund: Am {at} hat ein Termin stattgefunden.",
  "lead.qualify.reasonHuman": "Grund: von dir qualifiziert.",
  "lead.qualify.confirm": "Qualifizieren",
  "lead.qualify.confirmWithDeal": "Qualifizieren und Deal anlegen",
  "lead.qualify.done": "{name} ist jetzt ein Kontakt:",
  "lead.disqualify.title": "{name} disqualifizieren?",
  "lead.disqualify.reason": "Grund",
  "lead.disqualify.pickReason": "Grund wählen",
  "lead.disqualify.reasonRequired": "Wähle zuerst einen Grund.",
  "lead.disqualify.note": "Notiz (optional)",
  "lead.disqualify.confirm": "Disqualifizieren",

  "deals.viewBoard": "Board",
  "deals.viewTable": "Tabelle",
  "deals.amount": "Wert",
  "deals.lastSignal": "Letztes Signal",
  "deals.lastSignalNone": "noch kein Signal",
  "deals.lastMailNone": "noch keine E-Mail",
  "deals.stage": "Phase",
  "deals.close": "Erwarteter Abschluss",
  "deals.confirmAdvance": "In die Phase {stage} verschieben?",
  "deals.confirmTerminal":
    "Damit wird der Deal als {status} abgeschlossen. Bis du bestätigst, ändert sich nichts.",
  "deals.lostReason": "Verlustgrund",
  "deals.winNoEvidence":
    "Es ist kein unterschriebener Vertrag angehängt. Erfasse, wie der Deal gewonnen wurde; die Antwort bleibt am Deal und wird in Berichten gezählt.",
  "deals.winReason": "Wie wurde er gewonnen?",
  "deals.winReasonPick": "Auswählen, wie er gewonnen wurde",
  "deals.winReasonImported": "Aus einem anderen System importiert",
  "deals.winReasonPurchaseOrder": "Per Bestellung",
  "deals.winReasonVerbal": "Mündlich, persönlich oder telefonisch",
  "deals.winReasonRenewalByEmail": "Per E-Mail verlängert",
  "deals.winReasonOther": "Sonstiges",
  "deals.winReasonDetail": "Details",
  "deals.confirm": "Bestätigen",
  "deals.cancel": "Abbrechen",
  "deals.advanced": "Nach {stage} verschoben",
  "deal.pendingApprovals": "Wartet auf Freigabe",

  "deal.ownerKeep": "Aktuelle Zuständigkeit behalten",
  "deal.ownerMe": "Mir zuweisen",
  "deal.ownerUnassign": "Zuweisung aufheben",
  "deal.partnerCompany": "Über Partner",
  "deal.companyWithheld": "Unternehmen ausgeblendet",
  "deal.partnerWithheld": "Partner ausgeblendet",
  "deal.forecastCategory": "Forecast-Kategorie",
  "deal.strip.title": "Deal-Status",
  "deal.seats.ours_one": "{count} Teammitglied an diesem Deal",
  "deal.seats.ours_other": "{count} Teammitglieder an diesem Deal",
  "deal.committee.title": "Buying Center",
  "deal.committee.legendEngaged": "Im Austausch",
  "deal.committee.legendQuiet": "Nicht im Austausch",
  "deal.committee.legendGap": "Abdeckungslücke",
  "deal.committee.threads":
    "{engaged} von {total} Mitgliedern des Buying Centers im Austausch.",
  "deal.committee.engagement": "Beteiligung",
  "deal.strip.close": "Abschlussdatum",
  "deal.strip.close.none": "Kein Datum",
  "deal.strip.close.inDays_one": "in {days} Tag",
  "deal.strip.close.inDays_other": "in {days} Tagen",
  "deal.strip.close.overdue_one": "{days} Tag überfällig",
  "deal.strip.close.overdue_other": "{days} Tage überfällig",
  "deal.strip.close.provisional": "vorläufig, von keinem Menschen bestätigt",
  "deal.strip.close.waiting": "die Käuferseite bat, bis {date} zu warten",
  "deal.forecast.commit": "Commit",
  "deal.forecast.bestCase": "Best Case",
  "deal.forecast.pipeline": "Pipeline",
  "deal.forecast.omitted": "nicht im Forecast",
  "deal.pulse.yourMove": "Deine Antwort ist fällig.",
  "deal.pulse.nothingFlagged": "Keine Antwort fällig.",
  "deal.pulse.nothingFlaggedWhy":
    "Keine eingehende Nachricht zu diesem Deal ist als zu beantworten markiert.",
  "deal.pulse.wroteOn":
    "Die Gegenseite hat zuletzt am {date} geschrieben. Tage seitdem: {days}.",
  "deal.pulse.wroteUnknown":
    "Die Gegenseite hat geschrieben, und niemand hat geantwortet.",
  "deal.timeline.empty": "Noch keine Aktivität zu diesem Deal.",
  "acqSources.title": "Akquisequellen",
  "acqSources.sub":
    "Geschäftskanäle, denen ein Deal zugeordnet werden kann. Lead-Quellen, die festhalten, wie ein Datensatz in Margince gelangt ist, sind davon getrennt.",
  "acqSources.listLabel": "Quellen",
  "acqSources.loading": "Quellen werden geladen…",
  "acqSources.addOpen": "Neue Quelle",
  "acqSources.addTitle": "Neue Akquisequelle",
  "acqSources.addLabel": "Bezeichnung",
  "acqSources.addHint":
    "Der Schlüssel wird aus der Bezeichnung abgeleitet und lässt sich später nicht ändern.",
  "acqSources.addConfirm": "Quelle hinzufügen",
  "acqSources.builtIn": "Vorgegeben",
  "acqSources.readOnly": "Für deine Rolle sind diese Quellen schreibgeschützt.",
  "acqSources.labelFor": "Bezeichnung für {key}",
  "acqSources.activeFor": "{label} ist an einem Deal wählbar",
  "settings.page.reviewtemplates.sub":
    "Fragen, die Vertriebsmitarbeitende zu einem gewonnenen oder verlorenen Deal beantworten.",
  "settings.tab.reviewtemplates": "Abschlussrückblicke",
  "reviewTemplates.editHint":
    "Änderungen gelten für künftige Rückblicke. Bestehende Rückblicke behalten ihre ursprünglichen Fragen und Antworten.",
  "reviewTemplates.question": "Frage",
  "reviewTemplates.answerType": "Antworttyp",
  "reviewTemplates.options": "Auswahlmöglichkeiten (eine pro Zeile)",
  "reviewTemplates.requiredChoice": "Antwort erforderlich",
  "reviewTemplates.removeQuestion": "Frage entfernen",
  "reviewTemplates.addQuestion": "Frage hinzufügen",
  "reviewTemplates.save": "Vorlage speichern",
  "reviewTemplates.edit": "Fragen bearbeiten",
  "reviewTemplates.title": "Fragen zum Abschlussrückblick",
  "reviewTemplates.empty": "Keine Fragen für den Rückblick eingerichtet",
  "reviewTemplates.retired": "Stillgelegt",
  "reviewTemplates.required": "(erforderlich)",
  "outcomeReview.title": "Abschlussrückblick",
  "outcomeReview.add": "Rückblick hinzufügen",
  "outcomeReview.save": "Rückblick speichern",
  "outcomeReview.empty": "Noch kein Rückblick geschrieben",
  "outcomeReview.emptyDetail":
    "Halte die Gründe für dieses Ergebnis fest, solange sie frisch sind.",
  "outcomeReview.earlier": "Aus einem früheren Abschluss",
  "outcomeReview.earlierMark": "früherer Abschluss",
  "outcomeReview.outcomeWon": "Gewonnen",
  "outcomeReview.outcomeLost": "Verloren",
  "outcomeReview.noAnswer": "Nicht beantwortet",
  "outcomeReview.notes": "Weitere Notizen",
  "deal.commercialContext": "Kaufmännischer Kontext",

  "deal.arrFromOffer":
    "Der erwartete ARR stammt aus dem angenommenen Angebot und wird hier nicht bearbeitet.",
  "deal.brief": "Deal-Bericht",
  "deal.briefHint": "Bedarf des Kunden, Umfang und angestrebtes Ergebnis.",
  "deal.briefMore": "Mehr lesen",
  "deal.briefLess": "Weniger anzeigen",
  "deal.briefEdit": "Bearbeiten",
  "deal.briefAdd": "Bericht schreiben",

  "deal.briefEmpty": "Noch kein Bericht geschrieben",
  "deal.briefEmptyDetail":
    "Halte den Bedarf des Kunden fest und wie ein Gewinn aussieht, damit das nächste zuständige Teammitglied den Kontext hat.",

  "deal.motion": "Geschäftsart",
  "deal.motionUnset": "Nicht festgelegt",
  "deal.motionNewBusiness": "Neugeschäft",
  "deal.motionRenewal": "Verlängerung",
  "deal.motionUpsell": "Upsell",
  "deal.motionCrossSell": "Cross-Selling",
  "deal.motionExpansion": "Ausbau",
  "deal.motionExistingBusiness": "Bestandsgeschäft, nicht näher bestimmt",
  "deal.priority": "Priorität",
  "deal.priorityUnset": "Nicht festgelegt",
  "deal.priorityHigh": "Hoch",
  "deal.priorityMedium": "Mittel",
  "deal.priorityLow": "Niedrig",
  "deal.acquisitionSource": "Akquisequelle",
  "deal.expectedArr": "Erwarteter ARR",

  "deal.monthlyApproximate": "etwa",
  "assignments.title": "Verantwortlich",
  "assignments.noAccessNote":
    "Hält fest, wer verantwortlich ist. Gewährt keinen Zugriff auf diesen Datensatz.",
  "assignments.empty": "Noch niemand zugewiesen",
  "assignments.emptyDetail":
    "Weise ein Teammitglied oder ein Team zu, um festzuhalten, wer für diese Arbeit verantwortlich ist.",
  "assignments.roleRetired": "(stillgelegte Rolle)",
  "assignments.teamSuffix": "(Team)",
  "assignments.subjectInactive": "(inaktiv)",
  "assignments.add": "Zuweisen",
  "assignments.change": "Ändern",
  "assignments.changeOne": "Zuweisung ändern: {who}",
  "assignments.removeOne": "Zuweisung entfernen: {who}",
  "assignments.addTitle": "Verantwortung zuweisen",
  "assignments.changeTitle": "Zuweisung ändern",
  "assignments.subjectKind": "Teammitglied oder Team",
  "assignments.kindUser": "Teammitglied",
  "assignments.kindTeam": "Team",
  "assignments.who": "Wer",
  "assignments.findColleague": "Teammitglied suchen",
  "assignments.findTeam": "Team suchen",
  "assignments.role": "Rolle",
  "assignments.rolePlaceholder": "Rolle wählen",
  "assignments.saveAdd": "Zuweisen",
  "assignments.saveChange": "Änderung speichern",
  "recordRoles.title": "Verantwortungsrollen",
  "recordRoles.sub":
    "Wofür ein Teammitglied oder ein Team bei einem Unternehmen, Deal oder Projekt verantwortlich sein kann. Eine Rolle gewährt keinen Zugriff auf den Datensatz.",
  "recordRoles.listLabel": "Rollen",
  "recordRoles.loading": "Rollen werden geladen…",
  "recordRoles.readOnly": "Nur Admins können diese Rollen ändern.",
  "recordRoles.builtIn": "Vorgegeben",
  "recordRoles.addOpen": "Rolle hinzuf\u00fcgen",
  "recordRoles.addTitle": "Verantwortungsrolle hinzuf\u00fcgen",
  "recordRoles.recordTypes": "Gilt für",
  "recordRoles.assigneeKinds": "Zuweisbar an",
  "recordRoles.addLabel": "Name",
  "recordRoles.addHint":
    "Wofür die zuständige Seite verantwortlich ist, in einfachen Worten.",
  "recordRoles.addConfirm": "Rolle hinzuf\u00fcgen",
  "recordRoles.labelFor": "Name f\u00fcr {key}",
  "recordRoles.activeFor": "{label} kann neu zugewiesen werden",
  "deal.acquisitionUnset": "Nicht festgelegt",
  "deal.acquisitionRetired": "(stillgelegt)",
  "deal.waitUntil": "Warten bis",
  "deal.fxBase": "Basis {value} · Kurs {rate} vom {date}",
  "deal.archive": "Deal archivieren",
  "deal.archiveConfirm":
    "Das Archivieren entfernt diesen Deal aus den offenen Deals. Das lässt sich hier nicht rückgängig machen.",
  "deal.archivedReadOnly":
    "Dieser Deal ist archiviert und lässt keine Änderungen zu.",
  "deal.notYoursToChange":
    "Du kannst diesen Deal nicht ändern. Frage die zuständige Person, ob sie ihn mit dir teilt, oder einen Admin nach Bearbeitungsrechten.",
  "deal.closedTakesNoStage":
    "Dieser Deal ist abgeschlossen. Öffne ihn wieder, um ihn in eine andere Phase zu verschieben.",
  "deal.reopen": "Wieder öffnen",
  "deal.reopenPick": "Diesen Deal in eine offene Phase zurücksetzen",
  "deal.reopenConfirm": "Wieder öffnen",
  "deal.fcCommit": "Commit",
  "deal.fcBestCase": "Best Case",
  "deal.fcPipeline": "Pipeline",
  "deal.fcOmitted": "Ausgeschlossen",
  "deal.fcSlipped": "Verschoben",
  "deal.fcUncategorised": "Ohne Kategorie",

  "deals.pipeline": "Pipeline",
  "deals.filterStalled": "Nur stockende",
  "deals.filterOwnerMe": "Meine Deals",
  "deals.totalsNeedOwnerFilter":
    "Nur geladene Deals. Für die Summe nach „Meine Deals“ filtern.",
  "deals.totalsNoTagFilter":
    "Nur geladene Deals. Keine Summe, solange ein Tag-Filter aktiv ist.",
  "deals.filterPartner": "Partner",
  "deals.filterPartnerAnyOne": "Beliebiger Partner",
  "deals.filterMotion": "Geschäftsart",
  "deals.filterMotionAll": "Alle Geschäftsarten",
  "deals.filterPriority": "Priorität",
  "deals.filterPriorityAll": "Alle Prioritäten",
  "deals.filterAcquisition": "Quelle",
  "deals.filterAcquisitionAll": "Alle Quellen",
  "deals.filterForecast": "Forecast",
  "deals.filterForecastAll": "Alle Forecast-Kategorien",
  "deals.filterPartnerSourced": "Von Partnern vermittelt",
  "deals.filterStageAll": "Alle Phasen",
  "deals.filterCompanyAll": "Alle Unternehmen",
  "deals.filterStalledAll": "Alle Deals",
  "deals.filterOwnerAll": "Alle Zuständigen",
  "deals.filterPartnerAll": "Alle Quellen",
  "deals.sortNewest": "Neueste",
  "deals.unit": "Deals",
  "deals.bulkSelected": "{count} ausgewählt",
  "deals.bulkSelectRow": "{name} auswählen",
  "deals.bulkOwner": "Neue Zuständigkeit",
  "deals.bulkOwnerPick": "Zuständigkeit wählen",
  "deals.bulkAssign": "Zuweisen",
  "deals.bulkStage": "In Phase verschieben",
  "deals.bulkStagePick": "Phase wählen",
  "deals.bulkMove": "Verschieben",
  "deals.bulkArchive": "Archivieren",
  "deals.bulkArchiveConfirmTitle_one": "Diesen Deal archivieren?",
  "deals.bulkArchiveConfirmTitle_other": "{count} Deals archivieren?",
  "deals.bulkArchiveConfirmBody":
    "Archivierte Deals verschwinden aus allen Listen und Berichten und lassen sich hier nicht wiederherstellen.",
  "deals.bulkFailed": "{count} nicht übernommen:",
  "deals.bulkFailedRow": "konnte nicht gespeichert werden",

  "deal.offers": "Angebote",
  "deal.newOffer": "Neues Angebot",
  "deal.offerNeedsCurrency":
    "Lege zuerst den Deal-Wert fest. Ein Angebot verwendet die Währung des Deals.",
  "deal.offerNumber": "Angebotsnummer",
  "deal.offerRevision": "Revision",
  "deal.offersEmpty": "Noch keine Angebote",

  "offer.backToDeal": "Zurück zum Deal",
  "offer.totals": "Summen",
  "offer.net": "Netto",
  "offer.tax": "Steuer",
  "offer.gross": "Brutto",
  "offer.arr": "Jährlich wiederkehrend",
  "offer.committedNet": "Verbindlicher Nettowert",
  "offer.edit": "Kopfdaten bearbeiten",
  "offer.currency": "Währung",
  "offer.buyerCompany": "Käuferunternehmen",
  "offer.buyerCompanyConfirm": "Käuferunternehmen: {name}",
  "offer.template": "Vorlage",
  "offer.validUntil": "Gültig bis",
  "offer.introText": "Einleitungstext",
  "offer.termsText": "Bedingungstext",
  "offer.lines": "Positionen",
  "offer.addLine": "Position hinzufügen",
  "offer.position": "Position",
  "offer.description": "Beschreibung",
  "offer.unit": "Einheit",
  "offer.quantity": "Menge",
  "offer.unitPrice": "Einzelpreis",
  "offer.discountPct": "Rabatt %",
  "offer.taxRate": "Steuer %",
  "offer.committedPeriods": "Perioden",
  "offer.lineTotal": "Positionssumme",
  "offer.unpriced": "ohne Preis, nicht in der Summe enthalten",
  "offer.removeLine": "Entfernen",
  "offer.pickProduct": "Produkt wählen",
  "offer.pickProductConfirm": "Produkt: {name}",
  "offer.send": "Senden",
  "offer.sendConfirm": "Dieses Angebot an die Käuferseite senden?",
  "offer.sendBody":
    "Das Angebot ist schreibgeschützt, bis die Käuferseite antwortet.",
  "offer.accept": "Annehmen",
  "offer.acceptConfirm": "Dieses Angebot als angenommen markieren?",
  "offer.acceptBody":
    "Wert und Währung des Deals werden an dieses Angebot angepasst.",
  "offer.reject": "Ablehnen",
  "offer.rejectConfirm": "Dieses Angebot als abgelehnt markieren?",
  "offer.rejectReason": "Grund (optional)",
  "offer.regenerate": "Revision neu erzeugen",
  "offer.aiDisclosureTitle": "KI-gestütztes Angebot",
  "offer.diffAdded_one": "{count} Position hinzugefügt",
  "offer.diffAdded_other": "{count} Positionen hinzugefügt",
  "offer.diffRemoved_one": "{count} Position entfernt",
  "offer.diffRemoved_other": "{count} Positionen entfernt",
  "offer.diffChanged_one": "{count} Position geändert",
  "offer.diffChanged_other": "{count} Positionen geändert",
  "offer.renderPdf": "PDF erzeugen",
  "offer.viewPdf": "PDF anzeigen",
  "offer.pdfUnavailable":
    "Die PDF-Erzeugung ist auf dieser Installation nicht verfügbar.",

  "decision.approveEdited": "Bearbeitet freigeben",
  "decision.reject": "Ablehnen",
  "decision.draftSubject": "Betreff",
  "decision.draftBody": "Nachricht",
  "decision.dismiss": "Ausblenden",
  "decision.versionSkew":
    "Der Datensatz wurde nach dem Vormerken geändert. Merke ihn erneut vor, bevor du entscheidest.",
  "decision.reRead": "Neu laden",
  "decision.alreadyDecided":
    "Bereits entschieden. Hier ist nichts mehr zu tun.",
  "decision.expired": "Abgelaufen",
  "decision.expiresIn": "läuft in {countdown} ab",
  "decision.detail": "Details zur Freigabe",
  "decision.detailLoading": "Freigabe wird geladen…",
  "decision.detailTechnical": "Technische Details",
  "decision.detailAsked": "Angefragt",
  "decision.detailDecided": "Entschieden",
  "decision.applied": "Übernommen",
  "decision.undoOnRecord": "Am Datensatz rückgängig machen",
  "decision.status.approved": "Freigegeben",
  "decision.status.rejected": "Abgelehnt",
  "decision.status.expired": "Abgelaufen",

  "brief.panel.weekly": "Wochenrückblick",
  "brief.weekly.learnings.title": "Zu prüfende Beobachtungen",
  "brief.weekly.learnings.worked": "Positives Ergebnis",
  "brief.weekly.learnings.didNotWork": "Erfolgloses Ergebnis",
  "brief.weekly.learnings.pattern": "Muster",
  "brief.weekly.learnings.experiment": "Experiment",
  "brief.weekly.learnings.notRun": "Diese Woche wurde noch nicht analysiert.",
  "brief.weekly.learnings.insufficient":
    "Nicht genug erfasste Belege für aussagekräftige Beobachtungen.",
  "brief.weekly.scorecard.title": "Wochenbilanz",
  "brief.weekly.scorecard.leadBlock": "Leads und Termine",
  "brief.weekly.scorecard.dealBlock": "Deals",
  "brief.weekly.scorecard.advanced": "Lead-Fortschritte",
  "brief.weekly.scorecard.advancedBasis": "Je Schritt · nicht je Lead",
  "brief.weekly.scorecard.answeredInTarget": "Zielzeit überschritten",
  "brief.weekly.scorecard.answeredDetail":
    "In der Zielzeit beantwortet: {count}",
  "brief.weekly.scorecard.allInTarget": "Alle in der Zielzeit beantwortet",
  "brief.weekly.scorecard.meetingsHeld": "Abgehaltene Termine",
  "brief.weekly.scorecard.meetingsBasis":
    "Gebucht: {booked} · Nicht erschienen: {noShow}",
  "brief.weekly.scorecard.partialHistory": "Termine ohne Verlauf",
  "brief.weekly.scorecard.partialHistoryBasis":
    "Vor Beginn des Verlaufs · Zahlen sind Mindestwerte",
  "brief.weekly.scorecard.advances": "Phasenfortschritte",
  "brief.weekly.scorecard.regressionsDetail": "Eine Phase zurück: {count}",
  "brief.weekly.scorecard.medianDaysInStage": "Tage in der Phase",
  "brief.weekly.scorecard.medianBasis":
    "Median · diese Woche verlassene Phasen",
  "brief.weekly.scorecard.withNextStep": "Nächster Schritt festgelegt",
  "brief.weekly.scorecard.ofOpen": "von {total} offenen Deals",
  "brief.weekly.scorecard.noOpen": "Keine offenen Deals",
  "brief.weekly.scorecard.multiThreaded": "Mehrere Kontakte",
  "brief.weekly.scorecard.multiThreadedBasis":
    "von {total} offenen Deals · letzte 30 Tage",
  "brief.weekly.scorecard.closeDateSound": "Festes Abschlussdatum",
  "brief.weekly.scorecard.forecastMoves": "Forecast-Hochstufungen",
  "brief.weekly.scorecard.forecastMovesBasis": "Herabgestuft: {down}",
  "brief.weekly.scorecard.unreconstructible": "Nicht rekonstruierte Deals",
  "brief.weekly.scorecard.unreconstructibleBasis":
    "Von einer Löschung betroffen · Zahlen sind Mindestwerte",
  // Die kommende Woche. Der eingefrorene Rückblick sagt, was war; dies ist der
  // einzige Teil dieser Seite, den noch jemand ändern kann.
  "plan.title": "Zusagen dieser Woche",
  // Die zwei Regler des Briefings: welches, und für wen.
  "brief.view.label": "Startseitenansicht",
  "brief.view.morning": "Morgen",
  "brief.view.weekly": "Woche",
  "brief.scope.label": "Wessen Morgenbericht",
  "brief.scope.mine": "Meine",
  "brief.scope.team": "Team",
  // Der Eröffnungssatz des Briefings, aus den Zeilen zusammengesetzt, die die
  // Seite zeigt — nie von einem Modell geschrieben.
  "brief.sentence.clear": "Keine unmittelbaren Prioritäten gefunden.",
  "brief.sentence.one": "Zuerst: {lead}",
  "brief.sentence.many": "Zuerst: {lead}. Danach {rest}.",
  "brief.sentence.rest": "{count} weitere",

  // Der Einstiegssatz des Wochen-Briefs, aus den eingefrorenen Zahlen gebaut.
  "brief.week.won_one": "Du hast {count} Deal gewonnen.",
  "brief.week.won_other": "Du hast {count} Deals gewonnen.",
  "brief.week.moved_one": "Du hast {count} Deal vorangebracht.",
  "brief.week.moved_other": "Du hast {count} Deals vorangebracht.",
  "brief.week.met_one": "Du hast {count} Termin abgehalten.",
  "brief.week.met_other": "Du hast {count} Termine abgehalten.",
  "brief.week.carryPromises_one": "{count} Zusage übertragen.",
  "brief.week.carryPromises_other": "{count} Zusagen übertragen.",
  "brief.week.carryTasks_one": "{count} Aufgabe übertragen.",
  "brief.week.carryTasks_other": "{count} Aufgaben übertragen.",
  "brief.week.andCarry": "{result} {carry}",
  "brief.week.quiet": "Keine erledigte Arbeit und keine Deal-Bewegung erfasst.",

  "brief.feed.title": "Fokus",
  "brief.feed.changedBadge_one": "1 geändert",
  "brief.feed.changedBadge_other": "{count} geändert",
  "brief.feed.loading": "Morgenbericht wird geladen…",
  "brief.feed.clear":
    "Keine unmittelbaren Prioritäten. Die vollständige Worklist ist verfügbar.",
  "teamweekly.title": "Teamwoche",
  "teamweekly.weekOf": "{team} · Woche vom {day}",
  "teamweekly.frozen": "Eingefroren",
  "teamweekly.loading": "Teamwoche wird geladen…",
  "teamweekly.empty": "Für diese Woche gibt es nichts anzuzeigen.",
  "teamweekly.forbidden":
    "Teamrückblicke erfordern Teamzugriff. Dein Zugriff umfasst nur deine eigenen Datensätze.",
  "teamweekly.noSnapshot":
    "Für dieses Team und diese Woche ist kein gespeicherter Rückblick verfügbar. Wähle eine andere Woche oder prüfe die aktuelle Arbeit des Teams.",
  "teamweekly.pickTeam": "Team auswählen",
  "teamweekly.chooseTeam": "Wähle ein Team, um seine Arbeit zu prüfen.",
  "teamweekly.repsUnread":
    "Teammitglieder ohne Snapshot: {count}. In diesen Zahlen gezählt: {counted}.",
  "teamweekly.ofTotal": "{part} von {whole}",
  "teamweekly.headline.plain":
    "Es wurden keine Termine oder fälligen Zusagen erfasst.",
  "teamweekly.card.firstResponse": "Beantwortete Leads",
  "teamweekly.card.firstResponseBasis": "Über der Zielzeit: {breached}",
  "teamweekly.card.noLeads": "Keine eingegangen",
  "teamweekly.card.meetings": "Termine mit Follow-up",
  "teamweekly.card.meetingsBasis": "Aufgabe vor Ende der Woche verknüpft",
  "teamweekly.card.noMeetings": "Keine erfasst",
  "teamweekly.card.commitments": "Eingehaltene Zusagen",
  "teamweekly.card.commitmentsBasis": "Im Plan fällig",
  "teamweekly.card.noCommitments": "Keine fällig",
  "teamweekly.card.won": "Gewonnen",
  "teamweekly.card.wonBasis": "Verloren: {lost} · Wert nicht verfügbar",
  "teamweekly.card.wonBasisValue": "{value} · Verloren: {lost}",
  "teamweekly.card.reps": "Gezählte Mitglieder",
  "teamweekly.card.repsBasis": "Ganze Woche erfasst",
  "teamweekly.movement.title": "Aktivität der Woche",
  "teamweekly.movement.won": "Gewonnen",
  "teamweekly.movement.lost": "Verloren",
  "teamweekly.movement.moved": "Vorangebracht",
  "teamweekly.movement.meetings": "Abgehaltene Termine",
  "teamweekly.movement.leads": "Verteilte Leads",
  "teamweekly.agenda.title": "Agenda für Montag",
  "teamweekly.agenda.empty":
    "Für dieses Team konnte keine Woche eines Mitglieds geladen werden, daher gibt es keine Agenda.",
  "teamweekly.agenda.summary_one":
    "{count} Punkt für Montag, beginnend mit {first}.",
  "teamweekly.agenda.summary_other":
    "{count} Punkte für Montag, beginnend mit {first}.",
  "teamweekly.agenda.copy": "Agenda kopieren",
  "teamweekly.agenda.copied": "Kopiert",
  "teamweekly.agenda.copyFailed":
    "Die Agenda wurde nicht kopiert. Markiere die Liste und kopiere sie von Hand.",
  "teamweekly.focus.help_requested": "Hilfe angefragt",
  "teamweekly.focus.leads_breached": "Zielzeiten für Antworten verfehlt",
  "teamweekly.focus.commitments_missed": "Verpasste Planzusagen",
  "teamweekly.focus.meetings_without_next_step": "Beleg für Follow-up fehlt",
  "teamweekly.focus.strong_week": "Starke Woche",
  "teamweekly.focus.quiet_week": "Keine Priorität erkannt",

  "plan.loading": "Plan wird geladen…",
  "plan.empty": "Noch nichts im Plan.",
  "plan.none": "Für diese Woche gibt es noch keinen Plan.",
  "plan.start": "Woche planen",
  "plan.readOnly":
    "Nur-Lese-Ansicht. Hier kannst du diese Woche nicht planen und keine Zusagen abschließen.",
  "plan.add": "Zusage hinzufügen",
  "plan.refusedTitle": "Einige Änderungen nicht gespeichert",
  "plan.saveRefused_one":
    "1 Zusage wurde nicht gespeichert und ist weiterhin angehakt. Versuche es erneut.",
  "plan.saveRefused_other":
    "{count} Zusagen wurden nicht gespeichert und sind weiterhin angehakt. Versuche es erneut.",
  "plan.save_one": "{count} Änderung speichern",
  "plan.save_other": "{count} Änderungen speichern",
  "plan.due": "fällig {day}",
  "plan.state.open": "Offen",
  "plan.state.done": "Erledigt",
  "plan.state.missed": "Verpasst",
  "plan.state.dropped": "Verworfen",
  "plan.help.label": "Was brauchst du von deiner Führungskraft?",
  "plan.contract.title": "Einschränkungen dieser Woche",
  "plan.contract.risks": "Risiken",
  "plan.contract.risksHint":
    "Was aus deiner Sicht schiefgehen könnte, in eigenen Worten",
  "plan.contract.capacityNote": "Verfügbare Kapazität",
  "plan.contract.capacityNoteHint":
    "Alles, was der Kalender nicht zeigt, zum Beispiel Urlaub, Reisen oder ein Launch",
  "plan.contract.unwritten": "Noch nicht geschrieben",
  "plan.contract.nothingToName": "Nichts zu nennen",
  "plan.contract.edit": "Bearbeiten",
  "plan.contract.save": "Speichern",
  "plan.contract.cancel": "Abbrechen",
  "plan.contract.crowded": "Woche bereits voll",
  "plan.contract.crowdedBody":
    "Bereits gebucht: {committed}. Geplante Zusagen: {commitments}. Nicht alles wird hineinpassen.",
  "plan.help.ask": "Um Hilfe bitten",
  "plan.help.edit": "Anfrage bearbeiten",
  "plan.help.send": "Senden",
  "plan.help.cancel": "Abbrechen",
  "plan.help.asked": "Anfrage: {text}",
  "plan.help.waiting": "Hilfe angefragt · Antwort ausstehend",
  "plan.new.label": "Zusage",
  "plan.new.due": "Fälligkeitsdatum",
  "plan.new.save": "Hinzufügen",
  "plan.new.cancel": "Abbrechen",

  "brief.weekly.outlook": "Forecast-Ausblick",
  "brief.weekly.outlook.week": "Diese Woche",
  "brief.weekly.outlook.month": "Dieser Monat",
  "brief.weekly.outlook.quarter": "Dieses Quartal",
  "brief.weekly.outlook.none":
    "Für diese Woche ist kein Forecast-Snapshot verfügbar.",
  "brief.weekly.outlook.won": "Gewonnener Wert",
  "brief.weekly.outlook.commit": "Verbleibender Commit",
  "brief.weekly.outlook.bestCase": "Best Case",
  "brief.weekly.outlook.bestCaseDetail": "Enthält Commit",
  "brief.weekly.outlook.weighted": "Gewichteter Deal-Wert",
  "brief.weekly.outlook.landing": "Voraussichtliches Ergebnis",
  "brief.weekly.outlook.measure.commit_evidence": "Aus Commit-Belegen",
  "brief.weekly.outlook.measure.weighted": "Aus gewichteten Deals",
  "brief.weekly.outlook.measure.manager_call":
    "Aus der Einschätzung der Führungskraft",
  "brief.weekly.bridge": "Veränderung der Woche",
  "brief.weekly.bridge.opening": "Montag",
  "brief.weekly.bridge.closing": "Freitag",
  "brief.weekly.bridge.noOpening":
    "Für diese Woche gibt es keinen Montags-Snapshot und daher keinen Ausgangswert.",
  "brief.weekly.bridge.reconcile":
    "Die Balken ergeben in Summe nicht den Endwert. Vergleiche stattdessen die beiden Summen.",
  "brief.weekly.bar.created": "Neu angelegt",
  "brief.weekly.bar.advanced": "Vorangekommen",
  "brief.weekly.bar.slipped": "Verschoben",
  "brief.weekly.bar.won": "Gewonnen",
  "brief.weekly.bar.lost": "Verloren",
  "brief.weekly.bar.other": "Wechselkurse und Definitionen",
  "brief.weekly.frozen": "Eingefroren",
  "brief.weekly.written": "erstellt {at}",
  "brief.weekly.pickWeek": "Andere Woche öffnen",
  "brief.weekly.none":
    "Noch kein Wochenrückblick. Der erste wird am Montag nach deiner ersten vollen Woche erstellt.",
  "brief.weekly.tasksDelivered": "Aufgaben erledigt",
  "brief.weekly.ofDue": "{done} von {due}",
  "brief.weekly.dealsWon": "Gewonnen",
  "brief.weekly.dealsLost": "Verloren",
  "brief.weekly.dealsMoved": "Bewegt",
  "brief.weekly.decided": "Entschiedene Vorschläge",
  "brief.weekly.acceptedRejected":
    "{accepted} angenommen · {rejected} abgelehnt",
  "brief.weekly.queueWorked": "Einträge im Morgenbericht",
  "brief.weekly.actedDismissed": "{acted} erledigt · {dismissed} ausgeblendet",
  "brief.weekly.sincePrior": "{delta} gegenüber der Woche vom {week}",
  "brief.weekly.wonVsPrior": "{value} · {delta} gegenüber der Woche vom {week}",
  "brief.weekly.leadsAnswered": "Beantwortete Leads",
  "brief.weekly.ofRouted": "{answered} von {routed}",
  "brief.weekly.planCommitmentsKept": "Eingehaltene Zusagen",
  "brief.weekly.meetingsHeld": "Termine mit Follow-up",
  "brief.weekly.ofMeetings": "{withStep} von {held}",
  "brief.weekly.carriedOver": "Übertragen",
  "brief.weekly.outcome.moved": "bewegt",
  "brief.weekly.outcome.won": "gewonnen",
  "brief.weekly.outcome.lost": "verloren",
  "brief.act": "Erledigt",
  "brief.dismiss": "Ausblenden",

  "brief.digestSynced": "Synchronisierungsdetails",
  "brief.digestContacts": "Angelegte Kontakte",
  "brief.digestCompanies": "Angelegte Unternehmen",
  "brief.digestDedupe": "Zu prüfende Duplikate",
  "brief.digestClassify":
    "Zusagen: {commitments} · Termine: {meetings} · Nachrichten ohne Vertriebsaktion: {noise}",
  "brief.digestProjects": "Projekte",
  "brief.digestPhaseChanges": "Phasenwechsel",
  "brief.digestNewCommitments": "Neue Zusagen",
  "brief.digestGoneQuiet": "Verstummt",
  "brief.digestPhaseChange": "{from} → {to}",
  "brief.digestCommitmentCount": "Neue offene Zusagen: {count}",
  "brief.digestQuietDays": "Tage ohne Aktivität: {days}",
  "brief.glance.morning": "Guten Morgen, {name}.",
  "brief.glance.morningAnon": "Guten Morgen.",
  "brief.glance.afternoon": "Guten Tag, {name}.",
  "brief.glance.afternoonAnon": "Guten Tag.",
  "brief.glance.evening": "Guten Abend, {name}.",
  "brief.glance.eveningAnon": "Guten Abend.",
  "brief.glance.night": "Noch bei der Arbeit, {name}.",
  "brief.glance.nightAnon": "Noch bei der Arbeit.",
  "brief.glance.introWeekly":
    "Prüfe die Ergebnisse und plane deine nächsten Schritte.",
  "brief.glance.intro": "Dein Tag im Überblick.",
  "brief.panel.decisions": "Freigaben",
  "brief.panel.overnight": "Über Nacht",
  "brief.rail.quietSchedule": "Nichts gebucht",
  "brief.panel.schedule": "Anstehende Termine",
  "brief.overnight.connectorsUnhealthy": "Connectors brauchen Aufmerksamkeit",
  "brief.overnight.fixConnector": "Connector reparieren",
  "brief.readings.label": "Zahlen von heute",
  "brief.readings.floorTip":
    "Eine Quelle hat ihr Limit erreicht, daher ist diese Zahl ein Mindestwert.",
  "brief.readings.urgent": "Dringend",
  "brief.readings.urgentBasis": "Kontakt wartet · Zusage fällig",
  "brief.readings.decisions": "Prüfungen",
  "brief.readings.decisionsBasis": "Vorschläge · Datensatzprüfungen",
  "brief.readings.decisionsBlocking_one": "1 blockiert Kundenarbeit",
  "brief.readings.decisionsBlocking_other": "{count} blockieren Kundenarbeit",
  "brief.snooze.done": "Zurückgestellt bis {at}",
  "brief.snooze.undo": "Rückgängig machen",
  "brief.readings.meetings": "Anstehende Termine",
  "brief.readings.meetingsBasis": "Heute noch im Kalender",
  "brief.readings.needsPrep_one": "1 braucht Vorbereitung",
  "brief.readings.needsPrep_other": "{count} brauchen Vorbereitung",
  "brief.readings.prepUnknown": "Vorbereitung nicht geprüft",
  "brief.readings.prepared": "Alle vorbereitet",
  "brief.readings.leads": "Akquise",
  "brief.readings.leadsBasis": "Zugewiesen · kein Erstkontakt",
  "brief.readings.leadsDue": "Nächste Fälligkeit {value}",
  "brief.rail": "Kontext",
  "brief.deck.later": "Später",
  "brief.deck.showMore": "Ganze Nachricht anzeigen",
  "brief.deck.showLess": "Weniger anzeigen",
  "brief.deck.view": "Ansicht",
  "brief.deck.rowDetail": "Vorschlag",
  "brief.deck.rowMore": "Weitere Optionen",
  "brief.deck.rest_one": "1 weitere Entscheidung in der Worklist",
  "brief.deck.rest_other": "{count} weitere Entscheidungen in der Worklist",
  "brief.deck.viewDeck": "Stapel",
  "brief.deck.viewList": "Liste",
  "brief.deck.keys":
    "Pfeiltasten merken eine Entscheidung vor: → annehmen · ← ablehnen · ↑ bearbeiten · ↓ später · U rückgängig · Enter sendet vorgemerkte Entscheidungen",
  "brief.deck.behind_one": "1 weitere dahinter",
  "brief.deck.behind_other": "{count} weitere dahinter",
  "brief.deck.staged_one": "1 Entscheidung vorgemerkt",
  "brief.deck.staged_other": "{count} Entscheidungen vorgemerkt",
  "brief.deck.commit": "Vorgemerkte Entscheidungen senden",
  "brief.deck.skipped_one": "1 übersprungen",
  "brief.deck.skipped_other": "{count} übersprungen",
  "brief.deck.edited_one": "1 in Bearbeitung",
  "brief.deck.edited_other": "{count} in Bearbeitung",
  "brief.deck.commitNothingToSend": "Übersprungene entfernen",
  "brief.deck.unstage": "Letzte rückgängig machen",
  "brief.deck.clearedTitle": "Stapel leer",
  "brief.deck.cleared_one": "1 Entscheidung gesendet",
  "brief.deck.cleared_other": "{count} Entscheidungen gesendet",
  "brief.deck.clearedTime": "um {at}",
  "brief.deck.empty": "Nichts wartet auf dich.",
  "brief.deck.bundleSummary": "1 Entscheidung · Einträge: {count}",
  "brief.deck.bundleMembers": "Einträge anzeigen ({count})",
  "brief.snooze": "Zurückstellen",

  "enrich.toInbox": "Worklist öffnen",

  "deepread.title": "Dieses Unternehmen recherchieren",
  "deepread.titleRead": "Website-Recherche",
  "deepread.sub":
    "Durchsucht die Website des Unternehmens nach Domain, Branche, Größe, Standorten und wahrscheinlichen Entscheidungstragenden und schlägt dann einen ersten Schritt vor. Die Befunde bleiben vorgemerkt, bis du sie annimmst.",
  "deepread.cta": "Unternehmensrecherche starten",
  "deepread.ctaAgain": "Website erneut lesen",
  "deepread.starting": "Wird gestartet…",
  "deepread.unavailable":
    "Die Website-Recherche ist auf diesem Server nicht eingerichtet.",
  "deepread.statusQueued": "Eingereiht",
  "deepread.statusDeferred": "Wartet auf KI-Kontingent",
  "deepread.statusRunning": "Website wird gelesen…",
  "deepread.statusDone": "Fertig",
  "deepread.statusPartial": "Vorzeitig beendet",
  "deepread.statusPageCapped": "Bis zum Seitenlimit gelesen",
  "deepread.statusByteCapped": "Bis zum Größenlimit gelesen",
  "deepread.statusTimeCapped": "Bis zum Zeitlimit gelesen",
  "deepread.statusFailed": "Fehlgeschlagen",
  "deepread.statusCancelled": "Abgebrochen",
  "deepread.resumesAt": "Wird am {when} automatisch fortgesetzt.",
  "deepread.pagesSoFar_one": "Bisher {count} Seite gelesen",
  "deepread.pagesSoFar_other": "Bisher {count} Seiten gelesen",
  "deepread.stoppedEarly": "Vorzeitig beendet: {reason}",
  "deepread.stage.crawling": "Website wird gelesen",
  "deepread.stage.extracting": "Fakten werden extrahiert",
  "deepread.step.done": "fertig",
  "deepread.step.running": "läuft",
  "deepread.step.queued": "wartet",
  "deepread.stopBudget": "Modellkontingent",
  "deepread.factCount_one": "{count} belegter Fakt vorgemerkt",
  "deepread.factCount_other": "{count} belegte Fakten vorgemerkt",
  "deepread.proposals_other": "{count} Vorschläge warten auf Prüfung",
  "deepread.proposals_one": "{count} Vorschlag wartet auf Prüfung",
  "deepread.kindHome": "Startseite",
  "deepread.kindImpressum": "Impressum",
  "deepread.kindAbout": "Über das Unternehmen",
  "deepread.kindTeam": "Team",
  "deepread.kindServices": "Leistungen",
  "deepread.kindProducts": "Produkte",
  "deepread.kindContact": "Kontakt",
  "deepread.kindOther": "Sonstiges",

  "transcriptread.title": "Lesen des Transkripts",
  "transcriptread.cta": "Transkript lesen",
  "transcriptread.starting": "Wird gestartet…",
  "transcriptread.unavailable":
    "Das Lesen von Transkripten ist auf diesem Server nicht eingerichtet.",
  "transcriptread.statusQueued": "Eingereiht",
  "transcriptread.statusRunning": "Wird gelesen…",
  "transcriptread.statusDone": "Abgeschlossen",
  "transcriptread.statusFailed": "Fehlgeschlagen",
  "transcriptread.lineCount_one": "{count} Zeile gelesen",
  "transcriptread.lineCount_other": "{count} Zeilen gelesen",
  "transcriptread.proposals_other":
    "{count} nächste Schritte warten auf Prüfung",
  "transcriptread.proposals_one": "{count} nächster Schritt wartet auf Prüfung",
  "transcriptread.staged_one": "{count} nächster Schritt vorgeschlagen",
  "transcriptread.staged_other": "{count} nächste Schritte vorgeschlagen",
  "transcriptread.decided_one": "{count} Vorschlag geprüft",
  "transcriptread.decided_other": "{count} Vorschläge geprüft",
  "transcriptread.decidedDetail": "{accepted} angenommen, {rejected} abgelehnt",
  "transcriptread.expired_one": "{count} ohne Prüfung abgelaufen",
  "transcriptread.expired_other": "{count} ohne Prüfung abgelaufen",
  "transcriptread.effectFailed_one":
    "{count} angenommener Vorschlag hat keine Aufgabe angelegt.",
  "transcriptread.effectFailed_other":
    "{count} angenommene Vorschläge haben ihre Aufgaben nicht angelegt.",
  "transcriptread.statusUnknown_one":
    "Status für {count} Vorschlag nicht verfügbar",
  "transcriptread.statusUnknown_other":
    "Status für {count} Vorschläge nicht verfügbar",
  "transcriptread.nothingStated":
    "Transkript vollständig gelesen. Keine nächsten Schritte gefunden.",
  "transcriptread.failedTitle": "Transkript konnte nicht gelesen werden",
  "transcriptread.failedFallback": "Es wurde nichts vorgemerkt.",
  "transcriptread.effectFailedTitle": "Angenommen, aber keine Aufgabe angelegt",

  "create.cancel": "Abbrechen",
  "create.save": "Anlegen",
  "create.saving": "Wird angelegt…",
  "create.contact": "Neuer Kontakt",
  "vcardImport.action": "vCards importieren",
  "vcardImport.title": "vCards importieren",
  "vcardImport.fileLabel": "vCard-Datei",
  "vcardImport.whichFile":
    "Eine .vcf-Datei, das Exportformat für Kontakte aus Telefonen und E-Mail-Programmen. Eine Karte stammt vom Kontakt selbst, daher brauchen importierte Karten keine Freigabe.",
  "vcardImport.choose": ".vcf-Datei auswählen",
  "vcardImport.working": "Karten werden gelesen…",
  "vcardImport.done": "Schließen",
  "vcardImport.noCards": "Die Datei enthält keine Karten.",
  "vcardImport.failed":
    "Die Karten wurden nicht importiert. Versuche es erneut.",
  "vcardImport.outcome.created": "Hinzugefügt",
  "vcardImport.outcome.updated": "Fehlende Felder ergänzt",
  "vcardImport.outcome.needsReview": "Mögliches Duplikat",
  "vcardImport.outcome.skipped": "Übersprungen",
  "create.quickCapture": "Schnellerfassung",
  "create.quickCaptureSaved": "{name} gespeichert",
  "create.company": "Neues Unternehmen",
  "create.lead": "Neuer Lead",
  "create.deal": "Neuer Deal",
  "create.fullName": "Vollständiger Name",
  "create.firstName": "Vorname",
  "create.lastName": "Nachname",
  "create.contactTitle": "Position",
  "create.email": "E-Mail",
  "create.phone": "Telefon",
  "create.linkedin": "LinkedIn",
  "create.linkedinUrl": "LinkedIn-URL",
  "create.displayName": "Unternehmensname",
  "create.legalName": "Rechtlicher Name",
  "create.industry": "Branche",
  "create.sizeBand": "Unternehmensgröße",
  "co.address.summary": "Adresse",

  "create.addressLine1": "Straße und Hausnummer",
  "create.addressLine2": "Adresszusatz",
  "create.city": "Stadt",
  "create.region": "Bundesland oder Region",
  "create.postalCode": "Postleitzahl",
  "create.country": "Ländercode (ISO 3166)",
  "create.companyName": "Unternehmen",
  "create.companyPicked":
    "Ordnet den Kontakt diesem bestehenden Unternehmen zu.",
  "create.companyNew":
    "Legt ein neues Unternehmen an, sofern keines aus der Liste gewählt wird.",
  "create.dealName": "Deal-Name",
  "create.amount": "Wert",
  "create.currency": "Währung",
  "create.stage": "Phase",
  "create.relatedCompany": "Unternehmen",
  "create.expectedClose": "Erwarteter Abschluss",

  "field.unset": "Nicht festgelegt",
  "field.addEmail": "E-Mail hinzufügen",
  "field.addPhone": "Telefon hinzufügen",
  "field.addDomain": "Domain hinzufügen",

  "field.addRegisterVat": "USt-ID hinzufügen",
  "field.addRegisteredAddress": "Registeranschrift hinzufügen",

  "field.addTitle": "Position hinzufügen",

  "field.domain": "Domain",

  "field.emailType": "Typ",
  "field.emailWork": "Geschäftlich",
  "field.emailPersonal": "Privat",
  "field.emailOther": "Sonstige",
  "field.phoneType": "Typ",
  "field.phoneWork": "Geschäftlich",
  "field.phoneMobile": "Mobil",
  "field.phoneHome": "Privat",
  "field.phoneOther": "Sonstige",
  "field.primary": "Primär",
  "field.removeRow": "Entfernen",
  "field.removeRowLabel": "Zeile {n} entfernen",
  "field.moveRowUp": "Zeile {n} nach oben verschieben",
  "field.moveRowDown": "Zeile {n} nach unten verschieben",
  "field.rowMoved": "An Position {n} verschoben",
  "field.yes": "Ja",
  "field.no": "Nein",

  "dedupe.viewExisting": "Vorhandenen Datensatz ansehen",

  "co.spine.earlierMore": "Weitere Threads davor",
  "co.spine.failed": "Der Thread wurde nicht geladen.",
  "co.spine.exchangeCount": "{count} Nachrichten",
  "co.spine.kind.email": "E-Mail",
  "co.spine.kind.call": "Anruf",
  "co.spine.kind.meeting": "Termin",
  "co.spine.kind.note": "Notiz",
  "co.spine.kind.message": "Nachricht",
  "co.spine.andOthers": "{names} und {count} weitere",
  "co.spine.said.to": "{what} an {who}",
  "co.spine.said.from": "{what} von {who}",
  "co.spine.said.with": "{what} mit {who}",
  "co.spine.today": "Heute",
  "co.spine.said.met": "{host} hat {who} getroffen",
  "co.spine.said.held": "Termin, geleitet von {host}",
  "co.spine.lastSpoke": "Letzter Kontakt",
  "co.spine.days_one": "{count} Tag",
  "co.spine.days_other": "{count} Tage",
  "co.spine.quietSince": "Kein Kontakt seit",
  "co.spine.neverReplied": "Nie geantwortet",
  "co.spine.singleThreaded": "Ein Kontakt, keine Antwort",
  "co.spine.overdue": "\u00dcberf\u00e4llig",
  "co.spine.expectedClose": "Erwarteter Abschluss",
  "co.360.subject": "{name} · 360",
  "co.360.subjectUnnamed": "Dieses Unternehmen · 360",
  "today.title": "Handlungsbedarf",
  "co.spine.earlier_other": "{count} frühere Threads",
  "co.spine.earlier_one": "{count} früherer Thread",
  "today.failed":
    "Dieser Abschnitt wurde nicht geladen. Der Rest der Seite ist nicht betroffen.",
  "today.quiet": "Gerade besteht kein Handlungsbedarf.",
  "task.untitled": "Aufgabe ohne Titel",
  "today.withheld":
    "Nicht enthalten: {sections}. Du hast keinen Zugriff darauf.",
  "today.source.moments": "Erkenntnisse von Margince",
  "today.source.nextSteps": "offene Aufgaben",
  "today.source.nextMeeting": "Kalender",
  "today.source.deals": "Deals",
  "today.meeting.prepare": "Termin vorbereiten",
  "today.source.contacts": "Kontakte",
  "today.source.standing": "Unternehmensstatus",
  "today.source.activities": "Aktivität",
  "today.silence.days": "Tage ohne Antwort: {count}",
  "today.draft.new": "Neue E-Mail",
  "today.draft.act": "Entwerfen",
  "today.moment.act.openTask": "Aufgabe öffnen",
  "today.moment.act.followUp": "Nachfassen",
  "today.moment.act.writeToThem": "E-Mail schreiben",
  "today.workQueue": "Worklist",

  "evidence.mark": "gelesen",
  "evidence.confirm": "Bestätigen",
  "evidence.correct": "Korrigieren",
  "evidence.save": "Speichern",
  "evidence.saving": "Wird gespeichert…",
  "evidence.cancel": "Abbrechen",
  "evidence.correctedValue": "Korrigierter Wert",
  "evidence.confirmedAt": "Von einer Person bestätigt am {when}",
  "evidence.humanSet": "Von einer Person festgelegt",
  "acctCoverage.open": "Abdeckung vergleichen",
  "acctCoverage.title": "Abdeckung des Unternehmens",
  "acctCoverage.contact": "Kontakt",
  "acctCoverage.findContact": "Kontakt suchen",
  "acctCoverage.untried": "Nicht versucht",
  "acctCoverage.noMatch": "Keine Treffer.",
  "acctCoverage.columnCap":
    "{cap} Teammitglieder angezeigt. Wähle eines ab, um ein weiteres hinzuzufügen.",
  "acctCoverage.partial":
    "Aus einem unvollständigen Lesevorgang erstellt. Eine leere Zelle kann also bedeuten, dass der Lesevorgang früh abbrach, nicht, dass es niemand versucht hat.",
  "acctCoverage.noneButPartial":
    "Keine Verbindungen zurückgegeben, aber der Lesevorgang war begrenzt. Das Unternehmen kann dennoch abgedeckt sein.",
  "acctCoverage.noneAtAll":
    "Noch kein Teammitglied hat Nachrichten mit diesem Unternehmen ausgetauscht.",
  "docs.title": "Dokumente",
  "docs.empty": "Noch keine Dokumente zu diesem Unternehmen.",
  "docs.noneInCategory": "Keine Dokumente in dieser Kategorie.",
  "docs.allOnAgreements":
    "Jedes Dokument hier ist einem Vertrag oben zugeordnet.",
  "docs.allSuperseded":
    "Nur ersetzte Dokumente sind übrig. Blende sie ein, um den Verlauf zu sehen.",
  "docs.superseded.show": "Ersetzte einblenden",
  "docs.superseded.hide": "Ersetzte ausblenden",
  "docs.superseded.hidden_one": "1 ersetztes Dokument ist ausgeblendet.",
  "docs.superseded.hidden_other":
    "{count} ersetzte Dokumente sind ausgeblendet.",
  "docs.superseded.shown_one": "1 ersetztes Dokument ist unten aufgeführt.",
  "docs.superseded.shown_other":
    "{count} ersetzte Dokumente sind unten aufgeführt.",
  "docs.reading.show": "Ausgelesene Felder einblenden",
  "docs.reading.hide": "Ausgelesene Felder ausblenden",

  // Ein Dokument hinzufügen. Die Frage „Wozu gehört es?" trägt die eigentliche
  // Entscheidung: nur ein Dokument an einem Deal kann für Deal-Felder gelesen
  // werden. Der Hinweis sagt das, statt es die Lesenden an einem Panel
  // herausfinden zu lassen, das nie erscheint.
  "docs.add.action": "Dokument hinzufügen",
  "docs.add.title": "Dokument hinzufügen",
  "docs.add.about": "Gehört zu",
  "docs.add.aboutHint":
    "Dokumente an einem Deal können für Deal-Felder ausgelesen werden, Dokumente am Unternehmen nicht.",
  "docs.add.thisCompany": "Dieses Unternehmen",
  "docs.add.aDeal": "Ein Deal",
  "docs.add.dealSearch": "Deals des Unternehmens durchsuchen",
  "docs.add.dealSearchReach":
    "Die Suche umfasst die {deals} neuesten Deals dieses Unternehmens und zeigt die ersten {matches} Treffer. Ältere Deals lassen sich hier nicht auswählen.",
  "docs.add.category": "Kategorie",
  "docs.add.name": "Titel",
  "docs.add.nameHint": "Optional. Standardmäßig der Dateiname.",
  "docs.add.file": "Datei",
  "docs.add.fileHint": "Bis zu {size}.",
  "docs.add.fileEmpty": "Datei hier ablegen oder zum Auswählen klicken",
  "docs.add.cancel": "Abbrechen",
  "docs.add.submit": "Hochladen",
  "docs.add.uploading": "Wird hochgeladen…",
  "docs.add.errNoFile": "Wähle eine Datei zum Hochladen.",
  "docs.add.errNoDeal": "Wähle den Deal, dem das Dokument zugeordnet wird.",
  "docs.add.errRefused":
    "Du hast keine Berechtigung, diesem Datensatz Dokumente hinzuzufügen.",
  "docs.add.errTooLarge":
    "Die Datei ist größer als {size}, die Grenze dieser Installation. Wähle eine kleinere Datei.",
  "docs.add.failedTitle": "Hochladen fehlgeschlagen",
  "docs.add.failed":
    "Es wurde nichts gespeichert. Versuche es erneut oder wähle eine andere Datei.",
  "docs.add.partialTitle": "Hochgeladen, aber nicht eingeordnet",
  "docs.add.partial":
    "Die Datei ist gespeichert und unten aufgeführt, aber Kategorie und Titel wurden nicht gespeichert, daher liegt sie unter „Sonstiges“.",

  // Das Panel für die abgelegte Dokumentenlesung (RD-AC-N-2/-3). Drei Zustände,
  // die auch in den Worten getrennt bleiben müssen: noch keine Antwort, gelesen
  // und keines der Felder genannt, gar nicht lesbar.
  "extraction.neverRead":
    "Diese Datei wurde noch nicht nach Deal-Feldern ausgewertet.",
  "extraction.readIt": "Diese Datei lesen",
  "extraction.readAgain": "Erneut lesen",
  "extraction.starting": "Wird gestartet…",
  "extraction.startFailed":
    "Die Datei wurde nicht zum Lesen übergeben. Es wurde nichts geändert.",
  "extraction.loading": "Lesestatus wird geprüft…",
  "extraction.reading": "Diese Datei wird gelesen…",
  "extraction.stalled":
    "Das Lesen dauert ungewöhnlich lange und ist möglicherweise stehen geblieben.",
  "extraction.failed": "Diese Datei konnte nicht gelesen werden.",
  "extraction.groundedNothing":
    "Die KI hat diese Datei gelesen und keines der Deal-Felder gefunden.",
  "extraction.heading_one":
    "Die KI hat diese Datei gelesen: {count} Feld mit Beleg, zur Prüfung vorgemerkt. Nimm es an, um es zu speichern.",
  "extraction.heading_other":
    "Die KI hat diese Datei gelesen: {count} Felder mit Beleg, zur Prüfung vorgemerkt. Nimm sie an, um sie zu speichern.",
  "extraction.accept_one": "{count} Feld annehmen",
  "extraction.accept_other": "{count} Felder annehmen",
  "extraction.dismiss": "Ausblenden",
  "extraction.dismissed":
    "Es wurde nichts gespeichert. Die Datei bleibt angehängt.",
  "extraction.acceptedLabel": "Angenommene Felder",
  "extraction.acceptedHeading_one":
    "{count} Feld in den Deal übernommen. Die Originalauszüge bleiben erhalten.",
  "extraction.acceptedHeading_other":
    "{count} Felder in den Deal übernommen. Die Originalauszüge bleiben erhalten.",
  "extraction.acceptFailed":
    "Die Felder wurden nicht gespeichert. Der Deal ist unverändert.",
  "extraction.edit": "Bearbeiten",
  "extraction.editValue": "{field} bearbeiten",
  "extraction.omitted.notStated": "ausgelassen (in dieser Datei nicht genannt)",
  "extraction.omitted.notConfident":
    "ausgelassen (genannt, aber nicht eindeutig genug zum Annehmen)",
  "extraction.field.name": "Deal-Name",
  "extraction.field.amount": "Betrag",
  "extraction.field.currency": "Währung",
  "extraction.field.closeDate": "Voraussichtliches Abschlussdatum",
  "docs.filterLabel": "Dokumentkategorie",
  "docs.category.all": "Alle",
  "docs.category.contract": "Vertrag",
  "docs.category.offer": "Angebot",
  "docs.category.legal": "Rechtliches",
  "docs.category.email": "E-Mail-Anhang",
  "docs.category.message": "Nachrichtenanhang",
  "docs.category.other": "Sonstiges",
  "files.title": "Dateien",
  "files.empty":
    "Noch keine Dateien an diesem Deal. Lade eine Datei hoch oder verknüpfe eine E-Mail mit Anhang.",
  "files.origin": "Anhang einer Nachricht von {who}, {when}",
  "files.originUnknown": "unbekanntem Absender",
  "files.uploaded": "Hochgeladen am {when}",
  "files.hiddenBadge": "Ausgeblendet",
  "files.rowActions": "Aktionen für {name}",
  "files.hide": "An diesem Deal ausblenden",
  "files.unhide": "Wieder an diesem Deal anzeigen",
  "files.delete": "Löschen",
  "files.hideTitle": "{name} bei diesem Deal ausblenden?",
  "files.hideBody":
    "Die Nachricht und ihr Anhang bleiben an der Aktivität und in der Dateibibliothek des Unternehmens. Nur dieser Deal führt sie nicht mehr auf.",
  "files.deleteTitle": "{name} löschen?",
  "files.deleteBody":
    "Die Datei wird aus diesem Deal entfernt und aus jedem Deal Room, der sie teilt.",
  "files.showHidden": "Ausgeblendete Dateien anzeigen",
  "files.hideHidden": "Ausgeblendete Dateien verbergen",
  "docs.state.draft": "Entwurf",
  "docs.state.current": "Aktuell",
  "docs.state.final": "Final",
  "docs.state.superseded": "Ersetzt",
  "log.title": "Aktivität erfassen",
  "log.addTask": "Aufgabe hinzufügen",
  "log.kind": "Art",
  "log.kindNote": "Notiz",
  "log.kindTask": "Aufgabe",
  "log.kindMeeting": "Termin",
  "log.kindCall": "Anruf",
  "log.transcriptLabel": "Transkript",
  "log.transcriptHint":
    "Aus deinem Videokonferenz-Tool einfügen, etwa Teams, Zoom oder Meet. Vorhandene Sprecherangaben bleiben erhalten.",
  "log.asTranscript": "Dieser Text ist ein Transkript",
  "log.transcriptUpload": "Oder Datei hochladen",
  "log.transcriptUploadRejected": "Nur eine .txt-Datei ist zulässig.",
  "log.transcriptUploadFailed":
    "Die Datei wurde nicht gelesen. Füge stattdessen den Text ein.",
  "log.attendee": "Teilnehmende",
  "log.subject": "Betreff",
  "log.body": "Details",
  "log.dueAt": "Fälligkeitsdatum",
  "log.date": "Datum",
  "log.assignee": "Zugewiesen an",
  "log.unassigned": "Nicht zugewiesen",
  "log.save": "Erfassen",
  "log.saving": "Wird erfasst…",

  "recordAccess.contact.title": "Wer diesen Kontakt sehen kann",
  "recordAccess.contact.privateToYou":
    "Privat für das zuständige Teammitglied. Sonst kann niemand im Unternehmen diesen Kontakt sehen, auch keine Teammitglieder und keine Admins.",
  "recordAccess.contact.shared":
    "Alle Nutzenden im Unternehmen können diesen Kontakt sehen.",
  "recordAccess.contact.privateTip":
    "Nur du kannst diesen Kontakt sehen. Teile ihn, damit alle Nutzenden ihn sehen.",
  "recordAccess.contact.share": "Mit allen Nutzenden teilen",
  "recordAccess.contact.published":
    "Dieser Kontakt ist jetzt für alle Nutzenden sichtbar.",
  "recordAccess.contact.makePrivate": "Privat machen",
  "recordAccess.contact.madePrivate":
    "Dieser Kontakt ist jetzt privat für das zuständige Teammitglied. Nutzende, mit denen er direkt geteilt wurde, behalten den Zugriff.",
  "recordAccess.company.title": "Wer dieses Unternehmen sehen kann",
  "recordAccess.company.privateToYou":
    "Privat für das zuständige Teammitglied. Sonst kann niemand im Unternehmen diesen Datensatz sehen, auch keine Teammitglieder und keine Admins.",
  "recordAccess.company.shared":
    "Alle Nutzenden im Unternehmen können diesen Datensatz sehen.",
  "recordAccess.company.privateTip":
    "Nur du kannst dieses Unternehmen sehen. Teile es, damit alle Nutzenden es sehen.",
  "recordAccess.company.share": "Mit allen Nutzenden teilen",
  "recordAccess.company.published":
    "Dieses Unternehmen ist jetzt für alle Nutzenden sichtbar.",
  "recordAccess.company.makePrivate": "Privat machen",
  "recordAccess.company.madePrivate":
    "Dieses Unternehmen ist jetzt privat für das zuständige Teammitglied. Deals, Kontakte und E-Mails, die diesem Unternehmen zugeordnet sind, behalten ihre eigene Sichtbarkeit.",
  "compose.reply": "Antworten",
  "compose.writeEmail": "E-Mail schreiben",
  "compose.relink": "Neu verknüpfen",
  "compose.previewEmail": "Vorschau",
  "compose.readEmail": "Ganze E-Mail lesen",
  "compose.replyIntent": "Zweck der Antwort",
  "compose.newIntent": "Zweck der E-Mail",
  "compose.draftReply": "Antwort mit KI entwerfen",
  "compose.newEmail": "Neue E-Mail",
  "compose.replyingTo": "Antwort auf „{subject}“ · {when}",
  "compose.followingUp": "Follow-up zu deiner E-Mail „{subject}“ · {when}",
  "compose.draftContextHint":
    "Beschreibe den Zweck. Margince nutzt den Kontext des Datensatzes.",
  "compose.draftWithAi": "Mit KI entwerfen",
  "compose.drafting": "Wird entworfen…",
  "compose.discardDraft": "Entwurf verwerfen",
  "compose.discardDraftHint":
    "Markiert diesen Entwurf für deine Voice DNA als Fehlgriff. Der erzeugte Text wird nie gespeichert.",
  "compose.saveDraft": "Als Entwurf speichern",
  "compose.savedDraftSaved": "Entwurf gespeichert",
  "compose.savedDraftDelete": "Löschen",
  "compose.savedDraftDeleted": "Gespeicherter Entwurf gelöscht",
  "compose.savedDraftRestored": "Gespeicherter Entwurf wiederhergestellt",
  "compose.savedDraftRemove": "Gespeicherten Entwurf löschen",
  "compose.savedDraftChangedTitle": "Entwurf in einem anderen Fenster geändert",
  "compose.savedDraftChangedBody":
    "Beim Speichern bleibt der Text auf dem Bildschirm erhalten. Lade stattdessen die gespeicherte Fassung, um mit ihr weiterzuarbeiten.",
  "compose.savedDraftGoneBody":
    "Er wurde dort gesendet oder gelöscht. Beim Speichern bleibt der Text auf dem Bildschirm als neuer Entwurf erhalten.",
  "compose.savedDraftFailed":
    "Der Entwurf wurde nicht gespeichert. Speichere ihn erneut oder schließe den Dialog noch einmal, um den Text zu verwerfen.",
  "compose.savedDraftLoad": "Gespeicherte Fassung laden",
  "compose.aiDisclosureTitle": "KI-gestützter Entwurf",
  "compose.aiDisclosureFallback":
    "Diesen Entwurf hat eine KI geschrieben. Prüfe und bearbeite ihn vor dem Senden.",
  "compose.voiceVersion": "Aus deinen Schreibproben erstellt · v{n}",
  "compose.voiceDegraded":
    "Dein Stilprofil konnte nicht geladen werden, daher nutzt dieser Entwurf deinen Schreibstil nicht. Entwirf ihn neu oder bearbeite ihn vor dem Senden.",
  "compose.voiceDegradedTitle":
    "Dieser Entwurf entspricht nicht deinem Schreibstil",
  "compose.provisional": "Vorläufiger Schreibstil",
  "compose.provisionalHint":
    "Deine Voice DNA wird noch aufgebaut und prägt diesen Entwurf genauso wie eine fertige.",
  "compose.to": "An",
  "compose.cc": "Cc",
  "compose.subject": "Betreff",
  "compose.noGroundableRecipient":
    "Noch keine Kontakte bei diesem Unternehmen. Schreibe die Nachricht selbst oder lege zuerst einen Kontakt an.",
  "compose.draftTo": "Entwurf an",
  "compose.draftToUnset": "Kontakt wählen",
  "compose.relatedTo": "Bezug",
  "compose.relatedToNone": "Gesamtes Unternehmen",
  "compose.project": "Projekt",
  "compose.projectNone": "Kein Projekt",
  "compose.scopedToCounted":
    "Bezogen auf {key} · {inScope} von {total} Aktivitäten",
  "compose.scopedTo": "Bezogen auf {key}",
  "compose.channelFiling":
    "Beim Senden wird sie mit dem beantworteten Thread unter {project} abgelegt.",
  "compose.basedOn": "Grundlage: {inputs}",
  "compose.whyThisDraft": "Warum dieser Entwurf?",
  "compose.body": "Nachrichtentext",
  "compose.bodyHint": "Klicke auf den Text, um ihn zu bearbeiten.",
  "compose.transport": "Senden über",
  "compose.transportEmail": "E-Mail",
  "compose.recipientHint": "Name oder Adresse",
  "compose.subjectHint": "Thema der E-Mail",
  "compose.bodyPlaceholder": "Nachrichtentext",
  "compose.bcc": "Bcc",
  "compose.bccHint":
    "Andere Empfangende sehen diese Adressen nicht und auch nicht, dass jemand in Kopie gesetzt wurde.",
  "compose.threadGone":
    "Dieser Thread kann nicht mehr beantwortet werden, daher wurde stattdessen eine neue E-Mail geöffnet. Prüfe die Empfangsadresse vor dem Senden.",
  "compose.colleagueMailbox_one":
    "Diese Nachricht wurde an das Postfach von {names} zugestellt. Deine Antwort wird aus deinem eigenen Postfach unter deinem Namen gesendet.",
  "compose.colleagueMailbox_other":
    "Diese Nachricht wurde an die Postfächer von {names} zugestellt. Deine Antwort wird aus deinem eigenen Postfach unter deinem Namen gesendet.",
  "compose.colleagueUnnamed": "ein Teammitglied",
  "compose.threadGoneTitle": "Thread kann nicht beantwortet werden",
  "compose.colleagueMailboxTitle": "An ein Teammitglied zugestellt",
  "compose.deadRecipientsTitle": "E-Mails an diese Adressen kommen nicht an",
  "compose.attach": "Anhängen",
  "compose.filesOnRecord": "An diesem Datensatz",
  "compose.filesLoading": "Dateien des Datensatzes werden geladen…",
  "compose.filesNone": "Noch keine Dateien an diesem Datensatz.",
  "compose.filesFull":
    "Eine Nachricht kann höchstens {most} Dateien enthalten. Sende den Rest in einer zweiten Nachricht.",
  "compose.fileRemove": "{filename} entfernen",
  "compose.fileUpload": "Datei hochladen",
  "compose.fileUploadHint":
    "Die Datei wird zuerst an diesem Datensatz gespeichert, damit der Verlauf jeden gesendeten Anhang behält.",
  "compose.fileUploadEmpty": "Datei hier ablegen oder auswählen",
  "compose.fileUploading": "Wird hochgeladen…",
  "compose.fileStoredUnnamed":
    "Die Datei ist am Datensatz gespeichert, konnte aber nicht erkannt werden. Hänge sie über die Liste oben an.",
  "compose.carriageTitle": "Senden über {channel} nicht möglich",
  "compose.carriageCarries_one":
    "{channel} unterstützt keine Dateien, daher kann diese Nachricht mit {count} Anhang dort nicht gesendet werden. Sende den Text über {channel} und die Datei auf anderem Weg.",
  "compose.carriageCarries_other":
    "{channel} unterstützt keine Dateien, daher kann diese Nachricht mit {count} Anhängen dort nicht gesendet werden. Sende den Text über {channel} und die Dateien auf anderem Weg.",
  "compose.carriageCount":
    "{channel} erlaubt höchstens {limit} Dateien pro Nachricht, diese hat {named}. Sende den Rest in einer zweiten Nachricht.",
  "compose.carriagePerFile":
    "{filename} überschreitet das Limit von {limit} pro Datei bei {channel}. Sende eine kleinere Version oder teile sie auf anderem Weg.",
  "compose.carriageAggregate":
    "Diese {count} Dateien umfassen zusammen {total}, und {channel} erlaubt höchstens {limit} pro Nachricht. Verteile sie auf mehrere Nachrichten.",
  "compose.carriageCaption":
    "{channel} sendet den Text einer Nachricht mit Dateien als Bildunterschrift mit höchstens {limit} Zeichen; diese hat {length}. Kürze ihn oder sende die Dateien separat.",
  "calendar.previousMonth": "Voriger Monat",
  "calendar.nextMonth": "Nächster Monat",
  "compose.schedulePick": "Datum und Uhrzeit wählen",
  "compose.scheduleDate": "Datum",
  "compose.scheduleTime": "Uhrzeit",
  "compose.scheduleGoesOut": "Versand: {when}",
  "compose.willGoOut": "Versand: {when}",
  "compose.scheduleAfternoon": "Morgen Nachmittag",
  "compose.rewrite": "Umschreiben",
  "compose.rewriteShorter": "Kürzer",
  "compose.rewriteShorterAsk":
    "Die Bedeutung beibehalten und weniger Wörter verwenden.",
  "compose.rewriteWarmer": "Wärmer",
  "compose.rewriteWarmerAsk": "Wärmer im Ton, ohne vertraulich zu werden.",
  "compose.rewriteFormal": "Förmlicher",
  "compose.rewriteFormalAsk": "Förmlicher im Ton.",
  "compose.rewriteDeadline": "Frist ergänzen",
  "compose.rewriteDeadlineAsk":
    "Um eine Antwort bis zu einem bestimmten Datum bitten.",
  "compose.sendOptions": "Weitere Versandoptionen",
  "compose.scheduleSend": "Senden planen",
  "compose.scheduleTomorrow": "Morgen früh",
  "compose.scheduleMonday": "Montagmorgen",
  "compose.scheduleNow": "Jetzt senden",
  "compose.why": "Kontaktgrund",
  "compose.whyHint":
    "Der Datensatz legt fest, was erlaubt ist; mit dem Grund lässt sich der Versand dagegen prüfen.",
  "compose.why.requestedFollowup": "Angefragtes Follow-up",
  "compose.why.activeDeal": "Laufender Deal",
  "compose.why.quote": "Angefragtes Angebot",
  "compose.why.service": "Support zu einem Kauf",
  "compose.why.invoice": "Rechnung oder Zahlung",
  "compose.why.contract": "Vertrag der Gegenseite",
  "compose.why.account": "Kundenbeziehung der Gegenseite",
  "compose.why.marketing": "Marketing",
  "sendPermission.refused": "Nachricht kann nicht gesendet werden",
  "sendPermission.sayWhy": "Kontaktgrund erfassen",
  "sendPermission.unproven":
    "Kein erfasster Kontaktgrund für diese Empfangsadresse",
  "sendPermission.unprovenHint":
    "Wenn es einen Grund gibt, zum Beispiel eine Anfrage, einen Termin oder eine Kundenbeziehung, erfasse ihn unter deinem Namen.",
  "sendPermission.unprovenRefuses":
    "Der Versand wird abgelehnt, bis ein Grund erfasst ist.",
  "sendPermission.ready": "Bereit zum Senden",
  "sendPermission.markName": "Kommunikationsstatus dieser Nachricht",
  "sendPermission.checking":
    "Wird geprüft, ob diese Nachricht gesendet werden kann…",
  "sendPermission.unanswered":
    "Die Versandprüfung wurde nicht abgeschlossen. Beim Senden wird sie erneut ausgeführt.",
  "sendPermission.reason.objected":
    "Die empfangende Person hat dem Marketing widersprochen. Das kann hier niemand aufheben, auch kein Admin.",
  "sendPermission.reason.withdrawn":
    "Die empfangende Person hat ihre Einwilligung widerrufen. Das kann hier niemand aufheben, auch kein Admin.",
  "sendPermission.reason.restricted":
    "Für die Daten der empfangenden Person gilt eine Einschränkung der Verarbeitung. Das kann hier niemand aufheben, auch kein Admin.",
  "sendPermission.reason.askedUsToStop":
    "Die empfangende Person hat darum gebeten, nicht kontaktiert zu werden. Das kann hier niemand aufheben, auch kein Admin.",
  "sendPermission.reason.bounced":
    "Diese Adresse nimmt keine E-Mails an. Korrigiere die Adresse; eine Ausnahme ist hier nicht möglich.",
  "sendPermission.reason.tooMany":
    "Die empfangende Person hat das Limit für Marketingnachrichten vorerst erreicht. Das hebt sich automatisch auf.",
  "sendPermission.reason.ambiguous":
    "Mehrere Datensätze teilen sich diese Adresse, daher lässt sich nicht feststellen, wer die Nachricht erhält. Führe die Datensätze zusammen, um das zu lösen.",
  "sendPermission.reason.unconfirmed":
    "Die empfangende Person hat nicht bestätigt, dass sie Nachrichten erhalten möchte. Nur die empfangende Person selbst kann das bestätigen.",
  "sendPermission.reason.other":
    "Diese Nachricht kann nicht gesendet werden, und niemand hier kann das übergehen.",
  "compose.derivedReply":
    "Das ist eine Antwort auf eine eigene Nachricht der Gegenseite, daher ist kein Grund nötig.",
  "compose.send": "Senden",
  "compose.sendConfirmTitle": "E-Mail senden",
  "compose.threadHeading": "Dieser Thread",
  "compose.continueHeading": "Thread fortsetzen?",
  "compose.threadLeave": "Neue E-Mail",
  "compose.messageCount_one": "{count} Nachricht",
  "compose.messageCount_other": "{count} Nachrichten",
  "compose.threadContinuing": "Letzter Austausch in diesem Thread",
  "compose.draftKept":
    "Deine Änderungen wurden behalten. Entwirf erneut, wenn du so weit bist.",
  "compose.threadFailed":
    "Die Nachricht konnte nicht geladen werden. Wähle im Thread „Erneut versuchen“.",
  "compose.anchorGone":
    "Auf diese Nachricht kann nicht mehr geantwortet werden. Schreibe stattdessen eine neue Nachricht.",
  "compose.threadPending": "Thread wird geladen…",
  "compose.sendBody":
    "Prüfe und bearbeite den Entwurf. Das Senden lässt sich nicht rückgängig machen.",
  "compose.schedule": "Senden planen",
  "compose.scheduleConfirmTitle": "E-Mail-Versand planen",
  // The composer computed that it had scheduled a send and said nothing —
  // it closed the way a SENT message closes it. The confirm dialog above
  // promises a place to move or withdraw the message from; these two are how a
  // rep gets there.
  "compose.scheduledQueued": "E-Mail geplant, aber noch nicht gesendet.",
  "compose.scheduledOpenQueue": "Geplante Nachrichten",
  "compose.scheduleBody":
    "Die E-Mail wird zum gewählten Zeitpunkt gesendet, und die Einwilligungs- und Postfachprüfungen laufen dann erneut. Bis dahin lässt sie sich unter „Geplante Nachrichten“ verschieben oder abbrechen.",
  "compose.sendMessageConfirmTitle": "Nachricht senden",
  "compose.sendMessageBody":
    "Die Nachricht wird sofort gesendet. Das lässt sich nicht rückgängig machen.",
  "compose.consentBlockedTitle": "Versand blockiert: keine Einwilligung",
  "compose.consentBlocked":
    "Jemand unter den Empfangenden hat für diesen Zweck nicht eingewilligt, daher wurde der Versand blockiert (standardmäßig abgelehnt).",
  "directSend.open": "Mit erfasster Ausnahme senden",
  "directSend.opening": "Wird geöffnet…",
  "directSend.alreadySettled":
    "Darüber wurde bereits entschieden. Öffne die Prüfung, um das Ergebnis zu sehen.",
  "directSend.couldNotOpen":
    "Die Prüfung konnte nicht geöffnet werden. Versuche es erneut.",
  "directSend.title": "Auf eigene Verantwortung senden",
  "directSend.confirm": "Ausnahme erfassen und senden",
  "directSend.failed":
    "Die Nachricht wurde nicht gesendet. Deine Entscheidung ist möglicherweise schon erfasst; öffne die Prüfung, bevor du erneut entscheidest.",
  "directSend.noWarningServed":
    "Diese Installation hat den Bestätigungstext nicht veröffentlicht, daher kann die Ausnahme hier nicht erfasst werden.",
  "directSend.needsAcknowledgement":
    "Hake die Bestätigung an, um fortzufahren.",
  "directSend.needsReason": "Gib einen Grund für den Versand ein.",
  "directSend.reasonCodeLabel": "Grundlage",
  "directSend.explanationLabel": "Erläuterung",
  "directSend.acknowledge":
    "Ich habe das Obenstehende gelesen und übernehme die Verantwortung für den Versand dieser Nachricht.",
  "directSend.reason.customer_requested_outside_crm":
    "Außerhalb des CRM angefragt",
  "directSend.reason.contractual_necessity": "Vertragliche Verpflichtung",
  "directSend.reason.legal_obligation": "Rechtliche Verpflichtung",
  "directSend.reason.other": "Sonstiges",
  "compose.reviewReference": "Prüfung",
  "compose.reviewRequest": "Prüfung anfordern",
  "compose.reviewRequesting": "Wird angefordert…",
  "compose.reviewRequested":
    "Prüfung angefordert. Jemand, der diese Nachricht senden darf, entscheidet; bis dahin wartet sie.",
  "compose.reviewRequestFailed":
    "Die Prüfungsanfrage ist fehlgeschlagen. Versuche es erneut oder öffne die Prüfung über die Ablehnung oben.",
  "compose.consentGoto": "Einwilligung prüfen",
  "compose.draftUnavailable":
    "KI-Entwürfe sind nicht verfügbar, weil kein Modell konfiguriert ist. Die E-Mail lässt sich weiterhin selbst schreiben.",
  "compose.draftUnsupportedHere":
    "KI-Entwürfe sind auf dieser Seite nicht verfügbar. Die E-Mail lässt sich weiterhin selbst schreiben.",
  "compose.sendUnavailable":
    "Senden ist nicht verfügbar, weil kein Versanddienst konfiguriert ist.",
  "compose.mailboxNotSendCapable":
    "Dein Postfach ist nur zum Erfassen verbunden, ohne Berechtigung zum Senden. Verbinde es neu und erlaube das Senden; ein Postfach, das vor Einführung des Versands verbunden wurde, lässt sich nicht nachträglich erweitern.",
  "compose.mailboxNotSendCapableGoto": "Postfach neu verbinden",
  "compose.sharedUnsubscribeToken":
    "Eine Nachricht mit Abmeldelink geht immer nur an eine Adresse, weil der Link der Einwilligungsnachweis genau dieser Adresse ist. Sende sie einzeln an jede Adresse, ohne Cc.",
  "compose.multiRecipientWarning":
    "Dieser Zweck fügt einen Abmeldelink hinzu, daher wird ein Versand an mehr als eine Adresse abgelehnt. Sende die Nachricht einzeln an jede Adresse, ohne Cc.",
  "compose.relinkTitle": "Diese Aktivität neu verknüpfen",
  "compose.relinkTarget":
    "Kontakte, Unternehmen, Deals, Leads oder Projekte suchen",
  "compose.relinkNoVersion":
    "Diese Aktivität wurde ohne Version geladen, daher kann die Neuverknüpfung nicht angewendet werden. Öffne sie erneut und versuche es erneut.",
  "compose.relinkReplace": "Bestehende Verknüpfung ersetzen",
  "compose.relinkReplaceHint":
    "Ersetzt die bestehende Verknüpfung desselben Typs, statt eine weitere hinzuzufügen.",
  "compose.relinkConfirm": "Neu verknüpfen",
  "compose.relinkThread": "Restlichen Thread verschieben",
  "compose.relinkThreadHint":
    "Jede Nachricht in diesem Thread, die du bearbeiten darfst, wird in einem Schritt verschoben.",
  "compose.emptyRecipients": "Füge mindestens eine Empfangsadresse hinzu.",
  "compose.missingSubject": "Gib einen Betreff ein.",
  "compose.missingBody": "Gib vor dem Senden eine Nachricht ein.",
  "compose.missingWhy": "Wähle den Kontaktgrund aus.",
  "compose.actionFailed": "Die Anfrage ist fehlgeschlagen. Versuche es erneut.",

  "tasks.complete": "Erledigt",
  "tasks.snooze": "1 Tag zurückstellen",
  "tasks.moveTo": "Verschieben auf",
  "tasks.detail": "Aufgabe",
  "tasks.source": "Ursprungstermin",
  "tasks.sourceEmail": "Ursprungs-E-Mail",
  "tasks.openSource": "Original öffnen",
  "tasks.detailLoading": "Aufgabe wird geladen…",
  "tasks.isDone": "Abgeschlossen",
  "tasks.logged": "Erfasst",

  "analytics.sub":
    "Nur offene Deals, in {currency} umgerechnet, ungewichtet und gewichtet",
  "analytics.currency": "Währung",
  "analytics.count": "Offene Deals",
  "analytics.unweighted": "Ungewichtet",
  "analytics.weighted": "Gewichtet",
  "analytics.priced": "{priced} von {total} bepreist",
  "analytics.planNote":
    "Der ausgeführte Plan und die Zeilen, mit denen diese Zahl abgeglichen wird",
  "analytics.reportDeals": "Offene Deals nach Phase",
  "analytics.sections": "Analytics-Bereiche",
  "analytics.sectionForecast": "Forecast",
  "analytics.sectionPipeline": "Deals",
  "analytics.sectionPerformance": "Leistung",
  "analytics.noClosedDeals": "Noch keine Deals abgeschlossen.",
  "analytics.sectionOutcomes": "Meine Ergebnisse",
  "analytics.sectionCoverage": "Datenabdeckung",
  "analytics.sectionDelivery": "Umsetzung",
  // The Questions section: a question composed from the seat's own analytics
  // schema. Field keys are the engine's wire names, rendered under the
  // `analytics.field.` stem; a backend gate holds them against the report catalog.
  "analytics.sectionQuestions": "Fragen",
  "analytics.reportDealsByStage": "Alle Deals nach Phase",
  "analytics.reportLeadsByStatus": "Leads nach Status",
  "analytics.reportActivitiesByKind": "Aktivitäten nach Art",
  "analytics.reportMeetingConversion": "Terminkonversion",
  "analytics.field.amount_base_minor": "Umgerechneter Betrag",
  "analytics.field.amount_minor": "Betrag",
  "analytics.field.became_opportunity": "Konvertiert",
  "analytics.field.company_id": "Unternehmen",
  "analytics.field.currency": "Währung",
  "analytics.field.days_in_stage": "Tage in Phase",
  "analytics.field.days_to_close": "Tage bis Abschluss",
  "analytics.field.direction": "Richtung",
  "analytics.field.forecast_category": "Forecast-Kategorie",
  "analytics.field.host_user_id": "Terminleitung",
  "analytics.field.key": "Projektschlüssel",
  "analytics.field.kind": "Aktivitätsart",
  "analytics.field.last_activity_at": "Letzte Aktivität",
  "analytics.field.lost_reason": "Verlustgrund",
  "analytics.field.meeting_status": "Terminstatus",
  "analytics.field.name": "Projektname",
  "analytics.field.open_commitments": "Offene Zusagen",
  "analytics.field.open_deal_value_minor": "Offener Deal-Wert",
  "analytics.field.overdue_commitments": "Überfällige Zusagen",
  "analytics.field.owner_id": "Zuständig",
  "analytics.field.partner_company_id": "Partnerunternehmen",
  "analytics.field.period_month": "Abschlussmonat",
  "analytics.field.period_quarter": "Abschlussquartal",
  "analytics.field.period_year": "Abschlussjahr",
  "analytics.field.phase": "Phase",
  "analytics.field.pipeline_id": "Pipeline",
  "analytics.field.project": "Projektkennung",
  "analytics.field.project_id": "Projekt",
  "analytics.field.quiet_since": "Ruhig seit",
  "analytics.field.size_band": "Unternehmensgröße",
  "analytics.field.source": "Quelle",
  "analytics.field.stage_id": "Phase",
  "analytics.field.status": "Status",
  "analytics.field.weighted_amount_minor": "Gewichteter Betrag",
  "analytics.field.weighted_base_minor": "Gewichteter umgerechneter Betrag",
  "analytics.field.win_probability": "Gewinnwahrscheinlichkeit",
  "analytics.field.won_deal_value_minor": "Gewonnener Deal-Wert",
  "analytics.fn.count": "Anzahl",
  "analytics.fn.count_distinct": "Verschiedene Werte",
  "analytics.fn.sum": "Summe",
  "analytics.fn.avg": "Durchschnitt",
  "analytics.fn.min": "Minimum",
  "analytics.fn.max": "Maximum",
  "analytics.fn.median": "Median",
  "analytics.fn.p75": "Oberes Quartil",
  "analytics.q.addFilter": "Filter hinzufügen",
  "analytics.q.addMeasure": "Kennzahl hinzufügen",
  "analytics.q.allRecords": "Alle Datensätze",
  "analytics.q.answerTitle": "Antwort",
  "analytics.q.ask": "Frage stellen",
  "analytics.q.asking": "Wird abgefragt…",
  "analytics.q.builderTitle": "Frage",
  "analytics.q.chooseField": "Feld wählen",
  "analytics.q.choosePopulation": "Bericht wählen",
  "analytics.q.copyLink": "Link kopieren",
  "analytics.q.linkCopied": "Link kopiert",
  "analytics.q.copyRemedy":
    "Kopiere die Adresse aus der Adressleiste des Browsers.",
  "analytics.q.edit": "Frage bearbeiten",
  "analytics.q.empty": "Keine Datensätze passen zu dieser Frage.",
  "analytics.q.explainEmpty": "Keine Datensätze in dieser Gruppe.",
  "analytics.q.explainLoading": "Datensätze werden geladen…",
  "analytics.q.explainTruncated_one":
    "Der erste Datensatz wird angezeigt. Die Gruppe enthält mehr.",
  "analytics.q.explainTruncated_other":
    "Die ersten {count} Datensätze werden angezeigt. Die Gruppe enthält mehr.",
  "analytics.q.explainWithheld":
    "Diese Gruppe umfasst zu wenige Datensätze, deshalb bleiben ihre Datensätze verborgen.",
  "analytics.q.filterN": "Filter {n}",
  "analytics.q.filters": "Filter",
  "analytics.q.groupBy": "Gruppieren nach",
  "analytics.q.limited_one":
    "Nur die erste Gruppe wird angezeigt. Füge einen Filter hinzu, um die Frage einzugrenzen.",
  "analytics.q.limited_other":
    "Nur die ersten {count} Gruppen werden angezeigt. Füge einen Filter hinzu, um die Frage einzugrenzen.",
  "analytics.q.loadingRun": "Gespeicherte Frage wird geladen…",
  "analytics.q.loadingSchema": "Fragen werden geladen…",
  "analytics.q.measureN": "Kennzahl {n}",
  "analytics.q.measureOf": "{fn} ({field})",
  "analytics.q.measuredOver": "Gemessen über",
  "analytics.q.measures": "Kennzahlen",
  "analytics.q.mixedBody":
    "Gruppiere nach Währung oder filtere auf eine Währung, um diese Beträge zu sehen.",
  "analytics.q.mixedCurrencies": "Gemischte Währungen",
  "analytics.q.mixedTitle":
    "Beträge in verschiedenen Währungen werden nicht addiert",
  "analytics.q.needEntity": "Wähle einen Bericht für die Frage.",
  "analytics.q.needField": "Wähle für jede Kennzahl ein Feld.",
  "analytics.q.needFilterField": "Wähle für jeden Filter ein Feld.",
  "analytics.q.needValue": "Gib jedem Filter einen Wert.",
  "analytics.q.newQuestion": "Neue Frage",
  "analytics.q.no": "Nein",
  "analytics.q.yes": "Ja",
  "analytics.q.noEntities":
    "Mit deiner Rolle sind keine Berichtsdaten lesbar. Admins können den Zugriff freigeben.",
  "analytics.q.noGrouping": "Keine Gruppierung",
  "analytics.q.none": "Keine",
  "analytics.q.notSet": "Nicht gesetzt",
  "analytics.q.population": "Bericht",
  "analytics.q.readerAccess":
    "Beantwortet mit deinem eigenen Zugriff. Die Zahlen können von dem abweichen, was beim Speichern zu sehen war.",
  "analytics.q.record": "Datensatz",
  "analytics.q.recordId": "ID",
  "analytics.q.refusal.invalid": "Diese Frage lässt sich so nicht beantworten.",
  "analytics.q.refusal.privacy":
    "Die Antwort würde zu wenige Datensätze beschreiben.",
  "analytics.q.refusal.unsupported":
    "Diese Frage lässt sich mit dem, was dir zur Verfügung steht, nicht beantworten.",
  "analytics.q.removeMeasure": "Kennzahl {n} entfernen",
  "analytics.q.save": "Frage speichern",
  "analytics.q.savedBy": "Gespeichert von",
  "analytics.q.savedTitle": "Gespeicherte Frage",
  "analytics.q.scopeUnknown": "Für dich nicht verfügbar",
  "analytics.q.withheldBody":
    "Eine Gruppe mit zu wenigen Datensätzen wird verborgen, zusammen mit genug vom Rest, dass sie sich nicht herausrechnen lässt.",
  "analytics.q.withheldTitle": "Einige Gruppen sind verborgen",
  "analytics.q.amountInvalid":
    "Gib jeden Betrag als Zahl mit nicht mehr Nachkommastellen ein, als seine Währung hat.",
  "analytics.q.needCurrencyFilter":
    "Füge einen Filter „Währung ist …“ hinzu, um Beträge in der Währung des jeweiligen Deals zu vergleichen.",
  "analytics.q.scopeNotAvailable":
    "Diese Frage wurde für einen Datensatzumfang gespeichert, den du nicht messen kannst. Die Frage wird für {scope} gestellt.",
  "analytics.q.staleTitle": "Antwort ist veraltet",
  "analytics.q.staleBody":
    "Diese Antwort gilt für die zuletzt gestellte Frage. Stelle die Frage erneut, um sie zu aktualisieren.",
  "analytics.q.calculationN": "Berechnung, Kennzahl {n}",
  "analytics.q.measureFieldN": "Feld, Kennzahl {n}",
  "analytics.q.filterFieldN": "Feld, Filter {n}",
  "analytics.q.filterOperatorN": "Operator, Filter {n}",
  "analytics.q.filterValueN": "Wert, Filter {n}",
  "analytics.q.withheldGroups_one": "1 Gruppe verborgen",
  "analytics.q.withheldGroups_other": "{count} Gruppen verborgen",
  "analytics.reportProjectsByPhase": "Projekte nach Phase",
  "analytics.reportProjectCommitments": "Projektzusagen",
  "analytics.reportProjectsGoneQuiet": "Verstummte Projekte",
  "analytics.projects": "Projekte",
  "analytics.project": "Projekt",
  "analytics.openDealValue": "Offener Deal-Wert ({currency})",
  "analytics.wonDealValue": "Gewonnener Deal-Wert ({currency})",
  "analytics.openCommitments": "Offen",
  "analytics.overdueCommitments": "Überfällig",
  "analytics.quietSince": "Ruhig seit",
  "analytics.nothingQuiet": "Kein Projekt in Umsetzung ist verstummt.",
  "analytics.noProjectsYet":
    "Noch keine Projekte. Ein gewonnener Deal legt eines an.",
  "analytics.coverageSub":
    "Quellen, die die nächtliche Prüfung lesen konnte, und wie weit. Eine gelesene Quelle ohne Aktivität gilt als geprüft; bei einer ungelesenen steht der Grund.",
  "analytics.covSource": "Quelle",
  "analytics.covState": "Status",
  "analytics.covThrough": "Geprüft bis",
  "analytics.covChecked": "Geprüft",
  "analytics.covStale": "Veraltet, zuletzt nichts gelesen",
  "analytics.covUnavailable":
    "Nicht verfügbar, die Prüfung konnte sie nicht lesen",
  "analytics.covPermissionLimited": "Zugriff muss erneuert werden",
  "analytics.covNotConnected": "Nicht verbunden",
  "analytics.coverageInputsElsewhere":
    "Eingabeprobleme auf Datensatzebene werden in der Prüfung der Forecast-Eingaben aufgelistet und gelöst.",
  "analytics.myPipeline": "Meine offenen Deals",
  "analytics.myMeetings": "Meine Termine",
  "analytics.meetingsAsTheyStand":
    "Termine, die du leitest, nach aktuellem Status. Ein stattgefundener Termin zählt nicht mehr als gebucht.",
  "analytics.meetingsBooked": "Gebucht",
  "analytics.meetingsHeld": "Stattgefunden",
  "analytics.meetingsNoShow": "Nicht erschienen",
  "analytics.meetingsCanceled": "Abgesagt",
  "analytics.outcomesOwnLensOnly":
    "Dieser Bereich misst die Datensätze eines einzelnen Nutzerkontos. Die breiteren Bereiche decken den Rest der Sicht ab.",
  "analytics.openOutcomeDeals": "Deals mit Status {outcome} öffnen",
  "analytics.reportWinLoss": "Gewonnen und verloren",
  "analytics.reportStageAge": "Verweildauer je Phase",
  "analytics.outcome": "Ergebnis",
  "analytics.won": "Gewonnen",
  "analytics.lost": "Verloren",
  "analytics.baseValue": "Deal-Wert · {currency}",
  "analytics.baseValueUnnamed": "Deal-Wert",
  "analytics.noBaseCurrency": "Kein Betrag",
  "analytics.noBaseCurrencyWhy": "Währung nicht festgelegt",
  "analytics.forecastNoFigure": "Keine Deals",
  "analytics.forecastDeals_one": "1 Deal",
  "analytics.forecastDeals_other": "{count} Deals",
  "analytics.forecastNoAmount": "Kein Betrag",
  "analytics.forecastWeighted": "{amount} gewichtet",
  "analytics.forecastPriced": "{priced} von {count} bepreist",
  "analytics.readingNone": "Keine",
  "analytics.readingLoading": "Wird geladen",
  "analytics.readingUnavailable": "Nicht verfügbar",
  "analytics.medianDaysToClose": "Tage bis Abschluss (Median)",
  "analytics.p75DaysToClose": "Tage bis Abschluss (P75)",
  "analytics.medianDaysInStage": "Tage in der Phase (Median)",
  "analytics.p75DaysInStage": "Tage in der Phase (P75)",
  "analytics.tooFewForMedian": "Zu wenige Deals",
  "analytics.days": "{days} T",
  "analytics.unknownStage": "Frühere Phase",
  "analytics.share.open": "Ansicht teilen",
  "analytics.share.title": "Diese Ansicht teilen",
  "analytics.share.kindLegend": "Was der Link zeigt",
  "analytics.share.liveLabel": "Live-Ansicht",
  "analytics.share.liveHelp":
    "Wird bei jedem Öffnen neu berechnet, begrenzt auf das, was die Lesenden sehen dürfen. Die Zahlen ändern sich mit den Deals.",
  "analytics.share.snapshotLabel": "Snapshot",
  "analytics.share.snapshotHelp":
    "Die Zahlen zum Zeitpunkt des Snapshots; sie ändern sich nicht, und der Link nennt den Zeitpunkt der Aufnahme.",
  "analytics.share.snapshotUnavailable":
    "Für diesen Zeitraum gibt es noch keinen Snapshot.",
  "analytics.share.expiryNote":
    "Der Link funktioniert nach 30 Tagen nicht mehr.",
  "analytics.share.create": "Link erstellen",
  "analytics.share.linkTitle": "Dein Link",
  "analytics.share.linkWarning":
    "Der Link wird nur einmal angezeigt. Kopiere ihn jetzt; später lässt er sich nicht mehr abrufen.",
  "analytics.share.leaveWarning":
    "Schließen ohne Kopieren verwirft den Link, und ein neuer muss erstellt werden.",
  "analytics.share.copy": "Link kopieren",
  "analytics.share.copied": "Kopiert",
  "analytics.share.copyFailed":
    "Markiere den Link oben und kopiere ihn von Hand.",
  "analytics.share.done": "Fertig",
  "analytics.share.revoke": "Link schließen",
  "analytics.share.closedTitle": "Link geschlossen",
  "analytics.share.closedBody":
    "Der Link öffnet nicht mehr. Wer ihm folgt, wird abgewiesen.",
  "analytics.share.listOpen": "Geteilte Links",
  "analytics.share.listTitle": "Deine geteilten Links",
  "analytics.share.listIntro":
    "Von dir erstellte Links, die noch funktionieren. Ein geschlossener Link funktioniert sofort nicht mehr.",
  "analytics.share.listEmpty":
    "Du hast keine offenen Links. Mit „Ansicht teilen“ erstellst du einen.",
  "analytics.share.listCreated": "Erstellt am {date}",
  "analytics.share.listExpires": "Läuft am {date} ab",
  "analytics.share.populationCompany": "Gesamtes Unternehmen",
  "analytics.share.populationTeam": "Team",
  "analytics.share.populationOwner": "Teammitglied",
  "analytics.share.closeTitle": "Diesen Link schließen?",
  "analytics.share.closeBody":
    "Der Link funktioniert ab sofort für niemanden mehr, der ihn hat. Ein geschlossener Link lässt sich nicht wieder öffnen.",
  "analytics.frame": "Stand {asOf} · {zone}",
  "review.title": "Prüfungen vor der Einschätzung",
  "review.ready": "Bereit",
  "review.readyWithExceptions": "Bereit, mit Hinweisen",
  "review.needsReview": "Prüfung nötig",
  "review.checksIncomplete": "Prüfungen unvollständig",
  "review.allSourcesRead": "Alle Quellen geprüft.",
  "review.source.mail": "Postfach",
  "review.source.calendar": "Kalender",
  "review.source.documents": "Dokumente",
  "review.source.contracts": "Verträge",
  "review.source.incumbent": "Altsystem",
  "analytics.coverageNeverRun":
    "Noch keine Prüfung gelaufen. Eine ungeprüfte Installation ist nicht dasselbe wie eine fehlerfreie.",
  "review.source.offers": "Angebote",
  "review.sourcesUnread":
    "Nicht geprüft: {sources}. Die Befunde unten decken nur ab, was geprüft wurde.",
  "review.notCheckedYet":
    "Noch nichts geprüft: Die erste Prüfung wurde noch nicht ausgeführt. Die Werte oben geben die aktuellen Datensätze wieder.",
  "review.firstCheck.title": "Eingabeprüfung starten",
  "review.firstCheck.body":
    "Die nächtliche Prüfung läuft hier erst, wenn du sie startest. Die erste Prüfung meldet alle Befunde auf einmal und öffnet eine Aufgabe je betroffenem Deal. Der Umfang ist deshalb vorab wissenswert.",
  "review.firstCheck.scope":
    "Zu prüfende Deals: {deals}. Zu meldende Befunde: {findings}.",
  "review.firstCheck.start": "Prüfung starten",
  "review.firstCheck.starting": "Prüfung wird gestartet…",
  "review.firstCheck.pending": "Ermittelt, was eine Prüfung finden würde…",
  "review.check.running":
    "Die Prüfung läuft. Die Befunde erscheinen hier, sobald sie fertig ist.",
  "review.recheck.label": "Erneut prüfen",
  "review.recheck.starting": "Prüfung wird gestartet…",
  "review.firstCheck.cannotLook":
    "Eine Prüfung könnte jetzt nicht alle nötigen Quellen lesen und kann deshalb nicht sagen, was sie finden würde. Starten ist trotzdem möglich; der Lauf meldet dieselbe Lücke.",
  "review.check.failed": "Die Prüfung konnte nicht gestartet werden",
  "review.nothingToCheck": "Nichts zu prüfen.",
  "review.answer": "Beantworten",
  "review.colSeverity": "Schweregrad",
  "review.colFinding": "Befund",
  "review.colDeal": "Deal",
  "review.colAtStake": "Gefährdeter Betrag",
  "review.colSeenSince": "Offen seit",
  "review.severityHigh": "Hoch",
  "review.severityMedium": "Mittel",
  "review.severityLow": "Niedrig",
  "review.closePast": "Abschlussdatum verstrichen",
  "review.closeUnconfirmed": "Abschlussdatum nicht bestätigt",
  "review.closePushed": "Abschlussdatum verschiebt sich laufend",
  "review.amountVsOffer": "Betrag weicht vom Angebot ab",
  "review.amountVsContract": "Betrag weicht vom Vertrag ab",
  "review.noNextStep": "Kein nächster Schritt",
  "review.noEconomicBuyer": "Keine Unterschriftsbefugnis benannt",
  "review.buyerSilent": "Käuferseite verstummt",
  "review.commitUnpriced": "Commit ohne Betrag",
  "review.unknownCheck": "Unbenannte Prüfung",
  "review.sheetTitle": "Prüfung beantworten",
  "review.outcomeLegend": "Antworttyp",
  "review.fixedRecord": "Datensatz korrigiert",
  "review.addedEvidence": "Beleg ergänzt",
  "review.valueCorrect": "Wert ist korrekt",
  "review.notRelevant": "Für diesen Deal nicht relevant",
  "review.remindLater": "Später erinnern",
  "review.reassign": "Von jemand anderem zu beantworten",
  "review.hidesUntilExpiry": "Blendet diese Prüfung bis zum Ablauf aus.",
  "review.reason": "Grund",
  "review.reasonHelp":
    "Wer diese Zahl als Nächstes sieht, braucht den Grund, warum sie nicht markiert ist.",
  "review.remindAt": "Erinnern am",
  "review.expiresAt": "Läuft ab am",
  "review.expiresHelp":
    "Höchstens 90 Tage: Ein Wert, der im Mai korrekt ist, beschreibt den Mai.",
  "review.cancel": "Abbrechen",
  "review.submit": "Antwort speichern",
  "forecast.question": "Forecast für den Zeitraum",
  "forecast.answerWithCall":
    "Die aktuelle Einschätzung liegt bei {call}. Die Belege stützen {evidence}.",
  "forecast.answerNoCall":
    "Für diesen Zeitraum ist keine Einschätzung erfasst. Die Belege stützen {evidence}.",
  "forecast.partialTitle": "Nicht jeder Deal ist bepreist",
  "forecast.partial":
    "{priced} von {eligible} Deals sind bepreist. Deals ohne Preis tragen nichts zu den Summen oben bei.",
  "forecast.currentCall": "Aktuelle Einschätzung",
  "forecast.currentCallNone": "Keine Einschätzung",
  "forecast.currentCallDetailOver":
    "Einschätzung vom {date} · {gap} über den Belegen",
  "forecast.currentCallDetailUnder":
    "Einschätzung vom {date} · {gap} unter den Belegen",
  "forecast.currentCallDetailEven":
    "Einschätzung vom {date} · entspricht den Belegen",
  "forecast.evidence": "Belege",
  "forecast.evidenceDetail": "Bestätigte Abschlussdaten",
  "forecast.alreadyWon": "Bereits gewonnen",
  "forecast.alreadyWonDetail": "In diesem Zeitraum abgeschlossen",
  "forecast.updateCall": "Einschätzung aktualisieren",
  "forecast.callExplains":
    "Eine Einschätzung ist der Betrag, den du abzuschließen erwartest. Festgehalten wird deine Zahl, kein Deal ändert sich.",
  "forecast.expectedTotal": "Erwartete Summe für diesen Zeitraum",
  "forecast.supportingNote": "Begründung",
  "forecast.cancel": "Abbrechen",
  "forecast.saveCall": "Einschätzung speichern",
  "analytics.scopeLabel": "Datensatzumfang",
  "analytics.scopeFixed": "Diese Zahlen umfassen {scope}.",
  "forecast.period": "Zeitraum",
  "forecast.period.quarter": "Quartal",
  "forecast.period.month": "Monat",
  "forecast.period.week": "Woche",
  "forecast.receipt": "Daten und Belege geprüft",
  "forecast.eligible": "Berücksichtigte Deals",
  "forecast.priced": "Bepreist",
  "forecast.confirmed": "Abschlussdatum bestätigt",
  "forecast.fxMissing": "Wechselkurs fehlt",
  "analytics.reportForecast": "Forecast-Kategorien",
  "analytics.reportOpenByCompany": "Offene Deals pro Unternehmen",
  "analytics.forecastBannerTitle": "So liest du diese Kacheln",
  "analytics.forecastBanner":
    "Jede Kachel zeigt die Rohsumme und darunter die nach Wahrscheinlichkeit gewichtete Summe. Gerundet wird pro Deal, daher stimmt sie immer mit „Diese Zahl erklären“ überein.",
  "analytics.company": "Unternehmen",
  "analytics.openStageDeals": "Deals in {stage} öffnen",
  "analytics.openCompanyDeals": "Deals dieses Unternehmens öffnen",
  "analytics.noCompany": "Kein Unternehmen",
  "analytics.openDeals": "Offene Deals",
  "explain.sources": "Quellzeilen",
  "explain.col.record": "Deal",
  "explain.col.stage": "Phase",
  "explain.col.owner": "Zuständig",
  "explain.col.pipeline": "Pipeline",

  "settings.accountCard": "Dein Nutzerkonto",
  "unsaved.title": "Ungespeicherte Änderungen verwerfen?",
  "unsaved.body":
    "Wenn du diese Seite verlässt, gehen deine Eingaben verloren. Gehe zurück, um sie zuerst zu speichern.",
  "unsaved.discard": "Änderungen verwerfen",
  "settings.addedItem": "„{name}“ hinzugefügt",
  "settings.removedItem": "„{name}“ entfernt",
  "settings.removed": "Entfernt",
  "settings.saved": "Gespeichert",
  "settings.signature": "E-Mail-Signatur",
  "settings.signatureSub":
    "Steht unter jeder Nachricht, die du sendest, oberhalb der Fußzeile zum Abbestellen.",
  "settings.signatureLabel": "Grußformel",
  "settings.signaturePlaceholder": "Marek Janetzke\nGradion · +49 40 123456",
  "settings.signatureHint":
    "Nur Text. Leer lassen, um ohne Signatur zu senden. KI-Entwürfe fügen nie eine Grußformel hinzu.",
  "settings.signatureSaving": "Wird gespeichert…",
  "settings.signatureEdit": "Signatur bearbeiten",
  "settings.signatureNone": "Keine Grußformel festgelegt",
  "settings.signatureCancel": "Abbrechen",
  "delivery.morningLabel": "Morgenbericht",
  "delivery.morningHelp":
    "Sendet den Morgenbericht des Tages zusätzlich per E-Mail. Auf der Startseite erscheint er immer.",
  "delivery.weeklyLabel": "Wochenrückblick",
  "delivery.weeklyHelp":
    "Sendet den Wochenrückblick vom Montag zusätzlich per E-Mail.",
  "delivery.byEmail": "Per E-Mail",
  "delivery.none": "Nicht per E-Mail",
  "settings.appearance": "Darstellung",
  "settings.appearanceHelp":
    "Hell, dunkel oder die Einstellung des Geräts. Auch das Kontomenü ändert sie.",
  "settings.displayName": "Anzeigename",
  "settings.displayNameHelp":
    "Wird im Team an Datensätzen, die du bearbeitest, in Auswahllisten und im Audit-Log angezeigt.",
  "settings.displayNameSave": "Speichern",
  "settings.languageHelp": "Gilt für diese Sitzung.",
  "role.admin": "Admin",
  "role.management": "Geschäftsleitung",
  "role.manager": "Teamleitung",
  "role.rep": "Mitglied",
  "role.readOnly": "Nur Lesezugriff",
  "role.ops": "Ops",
  "inlineChoice.change": "{field} ändern",
  "rbac.masked": "Verborgener Wert",
  "settings.passports": "Agenten-Passports",
  "settings.passportsSub":
    "Ein Agent handelt mit deinen Berechtigungen und nie darüber hinaus: Jede Anfrage prüft deine Berechtigungen erneut.",
  "passport.scope.read": "Datensätze lesen",
  "passport.scope.draft": "Nachrichten entwerfen",
  "passport.scope.write": "Datensätze ändern",
  "passport.scope.send": "Nachrichten senden",
  "passport.scope.enrich": "Kontaktdaten kaufen",
  "passport.select": "Passport",
  "passport.noneOption": "Kein Passport",
  "settings.passportsLendHint":
    "Passports, die du für Skripte und andere Werkzeuge erstellt hast. Eine MCP-Client-Verbindung nutzt sie nicht; sie ist unten aufgeführt.",
  "settings.passportLabel": "Agentenname",
  "settings.mint": "Passport ausstellen",
  "settings.minting": "Wird ausgestellt…",
  "settings.mintCancel": "Abbrechen",
  "settings.mintDone": "Fertig",
  "settings.mintOpen": "Neuer Passport",
  "settings.passportScopes": "Berechtigungen des Agenten",
  "settings.passportScopesHint":
    "Mindestens eine auswählen. Ein Agent kann nie mehr tun als du.",
  "settings.passportScopesRequired":
    "Wähle mindestens eine Berechtigung für diesen Agenten aus.",
  // Was der geplante Agent gerade für diese Leserin tut. "Morgenbriefing" ist
  // dasselbe Wort wie auf der Startseite; ein abgebrochener Lauf darf nie
  // klingen, als wäre er fertig.
  "agent.activity.weeklyReview.queued": "Wochenzusammenfassung eingereiht",
  "agent.activity.weeklyReview.running": "Woche wird zusammengefasst…",
  "agent.activity.weeklyReview.stalled":
    "Wochenzusammenfassung dauert ungewöhnlich lange",
  "agent.activity.weeklyReview.done": "Wochenzusammenfassung bereit",
  "agent.activity.weeklyReview.degraded":
    "Woche ausgewertet, aber keine Zusammenfassung geschrieben",
  "agent.activity.weeklyReview.failed": "Wochenzusammenfassung fehlgeschlagen",
  "agent.activity.weeklyLearnings.queued": "Wochenbeobachtungen eingereiht",
  "agent.activity.weeklyLearnings.running":
    "Woche wird auf Beobachtungen analysiert…",
  "agent.activity.weeklyLearnings.stalled":
    "Wochenbeobachtungen dauern ungewöhnlich lange",
  "agent.activity.weeklyLearnings.done": "Wochenbeobachtungen bereit",
  "agent.activity.weeklyLearnings.degraded":
    "Woche ausgewertet, aber keine Beobachtungen gefunden",
  "agent.activity.weeklyLearnings.failed": "Wochenbeobachtungen fehlgeschlagen",
  "agent.activity.morningBrief.queued": "Morgenbericht eingereiht",
  "agent.activity.morningBrief.running": "Morgenbericht wird vorbereitet…",
  "agent.activity.morningBrief.done": "Morgenbericht bereit",
  "agent.activity.morningBrief.degraded":
    "Morgenbericht auf halbem Weg abgebrochen",
  "agent.activity.morningBrief.failed": "Morgenbericht fehlgeschlagen",
  "agent.activity.morningBrief.stalled":
    "Morgenbericht dauert ungewöhnlich lange",
  "agent.activity.riskSweep.queued": "Deal-Risikoprüfung eingereiht",
  "agent.activity.riskSweep.running": "Deals werden auf Risiken geprüft…",
  "agent.activity.riskSweep.done": "Deal-Risikoprüfung abgeschlossen",
  "agent.activity.riskSweep.degraded":
    "Deal-Risikoprüfung auf halbem Weg abgebrochen",
  "agent.activity.riskSweep.failed": "Deal-Risikoprüfung fehlgeschlagen",
  "agent.activity.riskSweep.stalled":
    "Deal-Risikoprüfung dauert ungewöhnlich lange",
  "agent.activity.documentExtract.queued": "Dokumentauswertung eingereiht",
  "agent.activity.documentExtract.running": "Dokument wird ausgewertet…",
  "agent.activity.documentExtract.stalled":
    "Die Dokumentauswertung dauert ungewöhnlich lange. Öffne die Datei erneut, um sie neu zu starten.",
  "agent.activity.documentExtract.done": "Dokumentauswertung abgeschlossen",
  "agent.activity.documentExtract.degraded":
    "Dokumentauswertung auf halbem Weg abgebrochen",
  "agent.activity.documentExtract.failed": "Dokumentauswertung fehlgeschlagen",
  "agent.activity.documentExtractNamed.queued":
    "Auswertung von {name} eingereiht",
  "agent.activity.documentExtractNamed.running": "{name} wird ausgewertet…",
  "agent.activity.documentExtractNamed.stalled":
    "Die Auswertung von {name} dauert ungewöhnlich lange. Öffne die Datei erneut, um sie neu zu starten.",
  "agent.activity.documentExtractNamed.done":
    "Auswertung von {name} abgeschlossen",
  "agent.activity.documentExtractNamed.degraded":
    "Auswertung von {name} auf halbem Weg abgebrochen",
  "agent.activity.documentExtractNamed.failed":
    "Auswertung von {name} fehlgeschlagen",
  "agent.activity.accountScan.queued": "Unternehmensanalyse eingereiht",
  "agent.activity.accountScan.running": "Unternehmensverlauf wird analysiert…",
  "agent.activity.accountScan.stalled":
    "Die Unternehmensanalyse dauert ungewöhnlich lange. Öffne das Unternehmen erneut, um sie neu zu starten.",
  "agent.activity.accountScan.done": "Unternehmensanalyse abgeschlossen",
  "agent.activity.accountScan.degraded":
    "Unternehmensanalyse abgeschlossen, soweit die Datensätze reichten",
  "agent.activity.accountScan.failed": "Unternehmensanalyse fehlgeschlagen",
  "agent.activity.accountScanNamed.queued": "Analyse von {name} eingereiht",
  "agent.activity.accountScanNamed.running":
    "Verlauf von {name} wird analysiert…",
  "agent.activity.accountScanNamed.stalled":
    "Die Analyse von {name} dauert ungewöhnlich lange. Öffne das Unternehmen erneut, um sie neu zu starten.",
  "agent.activity.accountScanNamed.done": "Analyse von {name} abgeschlossen",
  "agent.activity.accountScanNamed.degraded":
    "Analyse von {name} abgeschlossen, soweit die Datensätze reichten",
  "agent.activity.accountScanNamed.failed": "Analyse von {name} fehlgeschlagen",
  "agent.activity.transcriptRead.queued": "Lesen des Transkripts eingereiht",
  "agent.activity.transcriptRead.running": "Transkript wird gelesen…",
  "agent.activity.transcriptRead.stalled":
    "Lesen des Transkripts dauert ungewöhnlich lange",
  "agent.activity.transcriptRead.done": "Transkript gelesen",
  "agent.activity.transcriptRead.degraded":
    "Lesen des Transkripts auf halbem Weg abgebrochen",
  "agent.activity.transcriptRead.failed":
    "Lesen des Transkripts fehlgeschlagen",
  "agent.activity.voiceBuild.queued": "Analyse des Schreibstils eingereiht",
  "agent.activity.voiceBuild.running": "Schreibstil wird gelernt…",
  "agent.activity.voiceBuild.stalled":
    "Analyse des Schreibstils dauert ungewöhnlich lange",
  "agent.activity.voiceBuild.done": "Schreibstil gelernt",
  "agent.activity.voiceBuild.degraded": "Schreibstil teilweise gelernt",
  "agent.activity.voiceBuild.failed": "Schreibstil konnte nicht gelernt werden",
  "agent.activity.siteRead.queued": "Lesen der Unternehmenswebsite eingereiht",
  "agent.activity.siteRead.running": "Unternehmenswebsite wird gelesen…",
  "agent.activity.siteRead.stalled":
    "Lesen der Unternehmenswebsite dauert ungewöhnlich lange",
  "agent.activity.siteRead.done": "Unternehmenswebsite gelesen",
  "agent.activity.siteRead.degraded":
    "Lesen der Unternehmenswebsite auf halbem Weg abgebrochen",
  "agent.activity.siteRead.failed":
    "Lesen der Unternehmenswebsite fehlgeschlagen",
  "agent.activity.siteReadNamed.queued":
    "Lesen der Website von {name} eingereiht",
  "agent.activity.siteReadNamed.running": "Website von {name} wird gelesen…",
  "agent.activity.siteReadNamed.stalled":
    "Lesen der Website von {name} dauert ungewöhnlich lange",
  "agent.activity.siteReadNamed.done": "Website von {name} gelesen",
  "agent.activity.siteReadNamed.degraded":
    "Lesen der Website von {name} auf halbem Weg abgebrochen",
  "agent.activity.siteReadNamed.failed":
    "Lesen der Website von {name} fehlgeschlagen",
  "agent.activity.summarize.queued": "Zusammenfassung eingereiht",
  "agent.activity.summarize.running": "Zusammenfassung wird geschrieben…",
  "agent.activity.summarize.done": "Zusammenfassung bereit",
  "agent.activity.summarize.degraded":
    "Zusammenfassung auf halbem Weg abgebrochen",
  "agent.activity.summarize.failed": "Zusammenfassung fehlgeschlagen",
  "agent.activity.summarize.stalled":
    "Zusammenfassung dauert ungewöhnlich lange",
  "agent.activity.summarizeNamed.queued":
    "Zusammenfassung von {name} eingereiht",
  "agent.activity.summarizeNamed.running": "{name} wird zusammengefasst…",
  "agent.activity.summarizeNamed.done": "Zusammenfassung von {name} bereit",
  "agent.activity.summarizeNamed.degraded":
    "Zusammenfassung von {name} auf halbem Weg abgebrochen",
  "agent.activity.summarizeNamed.failed":
    "Zusammenfassung von {name} fehlgeschlagen",
  "agent.activity.summarizeNamed.stalled":
    "Zusammenfassung von {name} dauert ungewöhnlich lange",
  "agent.activity.draftReply.queued": "Antwortentwurf eingereiht",
  "agent.activity.draftReply.running": "Antwort wird entworfen…",
  "agent.activity.draftReply.done": "Antwortentwurf bereit",
  "agent.activity.draftReply.degraded":
    "Antwortentwurf auf halbem Weg abgebrochen",
  "agent.activity.draftReply.failed": "Antwortentwurf fehlgeschlagen",
  "agent.activity.draftReply.stalled":
    "Antwortentwurf dauert ungewöhnlich lange",
  "agent.activity.offerDraft.queued": "Angebotsentwurf eingereiht",
  "agent.activity.offerDraft.running": "Angebot wird entworfen…",
  "agent.activity.offerDraft.done": "Angebotsentwurf bereit",
  "agent.activity.offerDraft.degraded":
    "Angebotsentwurf auf halbem Weg abgebrochen",
  "agent.activity.offerDraft.failed": "Angebotsentwurf fehlgeschlagen",
  "agent.activity.offerDraft.stalled":
    "Angebotsentwurf dauert ungewöhnlich lange",
  "agent.rail.region": "Margince-Agent",
  "agent.panel.label": "Agentenbereich",
  "agent.rail.open": "Agentenbereich öffnen",
  "agent.rail.close": "Agentenbereich schließen",
  "agent.state.idle": "Inaktiv",
  "agent.state.ingest": "Erfassung läuft",
  "agent.state.working": "Arbeitet",
  "agent.state.warning": "Warnung",
  "agent.state.error": "Fehler",
  "agent.thisMonth": "diesen Monat",
  "agent.rail.spend": "Kosten in diesem Monat",
  "agent.panel.runningNow": "Läuft gerade",
  "agent.panel.needsYou": "Erfordert Aufmerksamkeit",
  "agent.panel.recent": "Letzte Aktivität",
  "agent.panel.runtime": "Laufzeit",
  "agent.panel.fullLog": "Vollständiges Protokoll",
  "agent.panel.decisions": "Freigaben",
  "agent.panel.nothingWaiting": "Nichts offen",
  "agent.panel.nothingToday": "Heute nichts abgeschlossen",
  "agent.panel.unnamedLive": "Arbeitet im Hintergrund",
  "agent.fact.model": "Modell",
  "agent.fact.tools": "Tools",
  "agent.fact.sources": "Quellen",
  "agent.fact.offline": "offline",
  "agent.fact.noCalls": "Noch keine Modellaufrufe",
  "agent.fact.hidden": "Für deine Rolle ausgeblendet",
  "agent.fact.noModel": "Kein Modell konfiguriert",
  "agent.line.waiting_one": "{count} Freigabe offen",
  "agent.line.waiting_other": "{count} Freigaben offen",
  "agent.line.allClear": "Nichts erfordert Aufmerksamkeit",
  "agent.line.cannotReach": "{sources} nicht erreichbar",
  "agent.line.runFailed": "Ein Lauf ist fehlgeschlagen",
  "agent.line.runStopped": "Ein Lauf wurde vorzeitig beendet",
  "agent.line.justNow": "gerade eben",
  "agent.setting.edgeLight": "Leuchtender Bildschirmrand",
  "agent.tip.day": "Aufgaben, Freigaben und Duplikate stehen unter {name}.",
  "agent.tip.ask": "Frage den Agenten mit {chord}.",
  "agent.tip.recap": "Öffne den Agentenbereich für die heutige Aktivität.",
  "agent.tip.edge": "Der Bildschirmrand leuchtet, während der Agent arbeitet.",

  "agents.connected": "Verbundene Agenten",
  "agents.connectedSub":
    "MCP-Clients mit eigenen Zugangsdaten, beschränkt auf den Zugriff, den du freigegeben hast",
  "agents.noneConnected": "Noch keine Agenten verbunden.",
  "agents.connectedOn": "verbunden {date}",
  "agents.disconnect": "Trennen",
  "agents.disconnectOpen": "Trennen",
  "agents.disconnectNamed": "{client} trennen",
  "agents.disconnected": "Getrennt",
  "agents.lapsed": "Zugangsdaten abgelaufen",
  "agents.renewing": "Wird erneuert",
  "agents.renewsBy": "Zugangsdaten werden bis {date} erneuert",
  "agents.expiredOn": "Zugangsdaten abgelaufen am {date}",
  "agents.revokeGrantOpen": "Verbindung beenden",
  "agents.revokeGrantNamed": "Verbindung zu {client} beenden",
  "agents.disconnectConfirm":
    "Damit endet die gesamte Verbindung, nicht nur ein Satz Zugangsdaten. Der Agent verliert den Zugriff beim nächsten Aufruf und kann ihn nicht erneuern. Für eine neue Verbindung muss der Zugriff erneut freigegeben werden.",
  "agents.connectHow": "Agent verbinden",
  "agents.connectSteps":
    "Führe einen dieser Befehle aus. Der Client registriert sich selbst und kehrt hierher zurück, damit du seinen Zugriff wählen kannst.",
  "agents.connectAntigravityPath":
    "Antigravity hat keinen Befehl zum Hinzufügen. Füge den Block in ~/.gemini/config/mcp_config.json ein.",
  "agents.connectorOff":
    "Der MCP-Connector ist für diese Installation ausgeschaltet.",
  "agents.connectorOffDetail":
    "Kein Agent kann sich verbinden, bis Admins oder Operations ihn einschalten. Passports funktionieren weiterhin als REST-Zugangsdaten.",
  "settings.tokenOnce":
    "Jetzt kopieren. Diese Zugangsdaten werden nur einmal angezeigt.",
  "settings.token": "Zugangsdaten",
  "settings.autonomy": "Autonomiestufen",
  "settings.autonomySub": "Was sofort läuft und was auf Freigabe wartet",
  "settings.tierRead":
    "Lesen, zusammenfassen, entwerfen: läuft sofort und wird vollständig protokolliert.",
  "settings.tierSend":
    "E-Mails senden, Termine buchen, einen Kontakt oder Deal ändern: läuft sofort, wenn der Agent diese Berechtigung hat. Die Erteilung der Berechtigung ist die Freigabe.",
  "settings.tierWait":
    "Anreicherung, eigene Felder, Webhooks, Zusammenführen von Tags: warten in Freigaben.",
  "settings.tierAdvance":
    "Deal-Phase weiterschieben: wartet nur, wenn der Schritt den Deal als gewonnen oder verloren abschließt.",
  "settings.locked": "Gesperrt",
  "settings.purposes": "Einwilligungszwecke",
  "settings.purposesSub":
    "Wofür diese Installation Einwilligungen einholt und auf welcher Rechtsgrundlage jeder Zweck beruht.",
  "settings.created": "erstellt am {date}",
  "settings.expires": "läuft am {date} ab",
  "settings.revoked": "Widerrufen",
  "settings.revoke": "Widerrufen",
  "settings.revokeConfirm":
    "Die Zugangsdaten des Passports werden sofort ungültig. Der Agent verliert beim nächsten Aufruf den Zugriff.",
  "import.withheld": "Nur Admins und Operations können Dateien importieren.",
  "import.title": "Datei importieren",
  "import.sub":
    "Importiere eine CSV-Datei mit Leads, Kontakten oder Unternehmen. Es wird nichts geschrieben, bevor du geprüft hast, was der Import tun wird.",
  "import.startLabel": "CSV-Datei importieren",
  "import.start": "Import starten",
  "import.objectLabel": "Zeilentyp",
  "import.object.lead": "Interessenten",
  "import.object.company": "Unternehmen",
  "import.object.contact": "Kontakte",
  "import.objectHint.lead":
    "Importiert eine unbearbeitete Liste als Leads, die ein Teammitglied qualifiziert, bevor sie zu Kontakten werden.",
  "import.objectHint.company":
    "Abgleich über den zugeordneten Namen: Erneutes Hochladen aktualisiert, statt Duplikate anzulegen.",
  "import.objectHint.contact":
    "Für bestehende Kontakte. Abgleich über die E-Mail-Adresse: Erneutes Hochladen aktualisiert, statt Duplikate anzulegen, und bereits gespeicherte Adressen bleiben unverändert.",
  "import.fileLabel": "CSV-Datei",
  "import.choose": "Datei auswählen",
  "import.chooseAnother": "Andere Datei auswählen",
  "import.profiled": "Ausgewertete Zeilen ab Dateianfang: {rows}.",
  "import.mappingTable": "Spaltenzuordnung",
  "import.col.column": "Spalte",
  "import.col.filled": "Gefüllt",
  "import.col.samples": "Werte",
  "import.col.destination": "Feld",
  "import.dontImport": "Nicht importieren",
  "import.noSamples": "Leer",
  "import.destinationFor": "Feld für {column}",
  "import.identifiedBy":
    "Zeilen werden über {column} erkannt, daher aktualisiert ein erneuter Import dieser Datei die Zeilen, statt sie zu duplizieren.",
  "import.needsIdentifier":
    "Ordne {field} eine Spalte zu. Ohne sie lassen sich Zeilen bei einem späteren Hochladen nicht abgleichen oder rückgängig machen.",
  "import.validate": "Importvorschau anzeigen",
  "import.validating": "Wird geprüft…",
  "import.previewTitle": "Importvorschau",
  "import.outcomeTitle": "Importergebnis",
  "import.resumedRun":
    "Dieser Import lief am {when}. Alle Aktionen unten sind weiterhin verfügbar.",
  "import.count.created": "Anlegen",
  "import.count.updated": "Aktualisieren",
  "import.count.unchanged": "Unverändert",
  "import.count.skipped": "Übersprungen",
  "import.rowsRead": "Gelesene Zeilen: {rows}, erkannt über {column}.",
  "import.linksOffered":
    "Zeilen mit Arbeitgeber: {offered}. Zeilen mit einem Unternehmen, das noch nicht im CRM ist: {unresolved}.",
  "import.linksApplied":
    "{applied} von {offered} Arbeitgeber-Verknüpfungen geschrieben.",
  "import.issuesLead":
    "Einige Zeilen können nicht importiert werden. Jede ist mit ihrer Zeilennummer in der Datei aufgeführt.",
  "import.issueLine": "Zeile {line}:",
  "import.commit_one": "1 Zeile importieren",
  "import.commit_other": "{rows} Zeilen importieren",
  "import.importing": "Wird importiert…",
  "import.done": "Import abgeschlossen",
  "import.failed":
    "Der Import wurde angehalten. Verarbeitete Zeilen: {checkpoint}. Fortsetzen macht an dieser Stelle weiter.",
  "import.resume": "Import fortsetzen",
  "import.uploadFailed": "Datei konnte nicht gelesen werden",
  "import.resumedRunTitle": "Früherer Import",
  "import.failedTitle": "Import vorzeitig angehalten",
  "import.commitFailed": "Import nicht ausgeführt",
  "import.undoInterruptedTitle": "Import nur teilweise rückgängig gemacht",
  "import.undoFailed": "Import nicht rückgängig gemacht",
  "import.validateFailed": "Vorschau nicht erstellt",
  "import.another": "Weitere Datei importieren",
  "import.undo_one": "Import rückgängig machen (1 Zeile)",
  "import.undo_other": "Import rückgängig machen ({rows} Zeilen)",
  "import.undoing": "Wird rückgängig gemacht…",
  "import.undoInterrupted":
    "Das Rückgängigmachen wurde unterbrochen. Mit „Weiter rückgängig machen“ geht es dort weiter, wo es angehalten hat.",
  "import.continueUndo": "Weiter rückgängig machen",
  "import.undone": "Import rückgängig gemacht",
  "import.undoReversed_one": "1 Zeile rückgängig gemacht.",
  "import.undoReversed_other": "{rows} Zeilen rückgängig gemacht.",
  "import.undoKeptLead": "Beibehalten, weil nach dem Import bearbeitet:",
  "import.undoErroredLead":
    "Nicht rückgängig gemacht und unverändert belassen:",
  "settings.dangerZone": "Gefahrenbereich",
  "settings.dangerZoneSub":
    "Nur für nicht produktive Installationen. Kann nicht rückgängig gemacht werden.",
  "settings.resetDataDesc":
    "Setzt diese Installation auf den Zustand beim ersten Start zurück. Fach- und Konfigurationsdaten werden gelöscht; das Unternehmen und seine Nutzenden bleiben erhalten und angemeldet.",
  "settings.resetDataButton": "Daten zurücksetzen",
  "settings.resetDataLabel": "Alle Daten zurücksetzen",
  "settings.resetDataConfirmButton": "Alle Daten zurücksetzen",
  "settings.resetDataConfirmTitle": "Alle Daten zurücksetzen?",
  "settings.resetDataConfirmBody":
    "Gib zur Bestätigung den Namen des Unternehmens ein. Das kann nicht rückgängig gemacht werden.",
  "settings.resetDataConfirmName": "Gib diesen Unternehmensnamen ein:",
  "settings.resetDataConfirmLabel": "Unternehmensnamen bestätigen",
  "settings.resetDataResult":
    "Gelöscht: Tabellen {tables}, Job-Einträge {jobs}, Event-Streams {streams}, Cache-Schlüssel {keys}, gespeicherte Dateien {objects}.",
  "settings.resetDataDrainWarning":
    "Beim Start des Zurücksetzens lief ein Hintergrund-Job. Er schlägt an den gelöschten Daten fehl und protokolliert einen harmlosen Fehler.",

  "settings.jobs": "Hintergrund-Jobs",
  "settings.jobsSub":
    "Wartende und fehlgeschlagene Hintergrund-Jobs nach Zuständigkeit",
  "jobs.adminOnly":
    "Der Zustand der Hintergrund-Jobs umfasst die gesamte Installation und erfordert eine Berechtigung, die deine Rolle nicht hat.",
  "jobs.empty":
    "Die Hintergrundwarteschlange ist leer. Keine Jobs warten, laufen, werden wiederholt oder sind gestoppt.",
  "jobs.workspaceKinds": "Dieses Unternehmen",
  "jobs.workspaceEmpty": "Keine Hintergrund-Jobs für dieses Unternehmen.",
  "jobs.dispatcherKinds": "Flotten-Dispatcher",
  "jobs.dispatcherSub":
    "Einträge ohne Unternehmen: Ein Dispatcher verteilt Arbeit an jedes Unternehmen und führt selbst keine aus. Deren Zahlen gehören zur Installation.",
  "jobs.dispatcherEmpty":
    "Keine Dispatcher-Einträge. Periodische Ticks legen sie neu an, eine leere Liste heißt also, dass gerade keiner geplant ist.",
  "jobs.count.waiting": "Wartend: {count}",
  "jobs.count.running": "Laufend: {count}",
  "jobs.count.retrying": "In Wiederholung: {count}",
  "jobs.count.dead": "Gestoppt: {count}",
  "jobs.queue": "Warteschlange {queue}",
  "jobs.waitedSeconds_one": "der älteste wartet seit {count} Sekunde",
  "jobs.waitedSeconds_other": "der älteste wartet seit {count} Sekunden",
  "jobs.waitedMinutes_one": "der älteste wartet seit {count} Minute",
  "jobs.waitedMinutes_other": "der älteste wartet seit {count} Minuten",
  "jobs.waitedHours_one": "der älteste wartet seit {count} Stunde",
  "jobs.waitedHours_other": "der älteste wartet seit {count} Stunden",
  "jobs.waitedDays_one": "der älteste wartet seit {count} Tag",
  "jobs.waitedDays_other": "der älteste wartet seit {count} Tagen",
  "jobs.deadTitle_one": "{count} gestoppter Job in den letzten {hours} Stunden",
  "jobs.deadTitle_other":
    "{count} gestoppte Jobs in den letzten {hours} Stunden",
  "jobs.deadTotal":
    "In den letzten 7 Tagen verworfen oder abgebrochen: {count}. So lange werden diese Einträge aufbewahrt.",
  "jobs.deadBody_one":
    "{count} Job wurde verworfen oder abgebrochen und läuft ohne Eingriff nicht mehr. Verworfene Jobs haben alle Versuche verbraucht, abgebrochene wurden absichtlich gestoppt.",
  "jobs.deadBody_other":
    "{count} Jobs wurden verworfen oder abgebrochen und laufen ohne Eingriff nicht mehr. Verworfene Jobs haben alle Versuche verbraucht, abgebrochene wurden absichtlich gestoppt.",
  "jobs.failures": "Letzte Fehler",
  "jobs.failuresSub": "Neueste zuerst, höchstens 50.",
  "jobs.failuresEmpty": "Keine Fehler erfasst.",
  "jobs.state.retryable": "Wird wiederholt",
  "jobs.state.discarded": "Verworfen",
  "jobs.state.cancelled": "Abgebrochen",
  "jobs.attempt": "Versuch {attempt} von {max} · {when}",
  "jobs.remedy": "Zu tun: {remedy}",
  "jobs.jobId": "Job {id}",
  "jobs.failingSince": "fehlerhaft seit {when}",
  "jobs.reasonVetted":
    "Gründe, Klassen und Abhilfen stammen aus der Job-Schicht, nie aus der Rohursache des Workers. Einen Fehler, den sie nicht formulieren kann, zeigt sie mit einem festen Ersatztext und ohne Klasse.",
  "jobs.generatedAt": "Stand: {time}",

  "settings.extIngest": "Abgewiesene Connector-Datensätze",
  "settings.extIngestSub":
    "Datensätze eines installierten Connectors, die dieses CRM nicht speichern konnte.",
  "extIngest.adminOnly":
    "Der Zustand der Connector-Eingänge umfasst die gesamte Installation und erfordert eine Berechtigung, die deine Rolle nicht hat.",
  "extIngest.empty":
    "In den letzten {days} Tagen wurden keine Datensätze abgewiesen. Jeder Datensatz, den die installierten Connectors gesendet haben, wurde angenommen.",
  "extIngest.refusedTotal_one": "{count} Datensatz abgewiesen",
  "extIngest.refusedTotal_other": "{count} Datensätze abgewiesen",
  "extIngest.lastRefused": "zuletzt {when}",
  "extIngest.refusal.key": "Datensatzschlüssel",
  "extIngest.refusal.activity": "Aktivität selbst",
  "extIngest.refusal.addresses": "Adressen",
  "extIngest.refusal.counterparty": "Gegenseite",
  "extIngest.refusal.participants": "Teilnehmende",
  "extIngest.refusal.size": "Größenbeschränkungen",
  "extIngest.refusalCount": "{refusal}: {count}",
  "extIngest.noDetail":
    "Zeigt nur Anzahlen und die abweisende Prüfung, nie den Datensatz, weil das abgewiesene Feld Inhalte des Absenders zitieren kann. Das Connector-Protokoll enthält den vollständigen Grund für jeden Fall.",
  "extIngest.generatedAt": "Stand: {time}",

  "audit.you": "Du",
  "audit.system": "System",
  "audit.unknownBuyer": "Gast im Deal Room",
  "audit.unknownMember": "Unbekanntes Mitglied",
  "audit.viaAgent": "über einen Agenten",
  "audit.viaConnector": "über einen Connector",
  "audit.viaDealRoom": "im Deal Room",
  "audit.viaNamed": "über {client}",
  "audit.noHumanAuthority": "Keine menschliche Autorisierung erfasst",
  "settings.auditSub":
    "Jede Aktion, zugeordnet zu Nutzerkonto, Agent oder Connector",
  "settings.auditAdminOnly":
    "Deine Rolle darf das vollständige Audit-Log nicht lesen. Es verzeichnet alle Handelnden und jeden Datensatz, auf den sie zugegriffen haben.",
  "settings.auditFilters": "Filter",
  "settings.auditEntries": "Audit-Log",
  "settings.auditTrailLabel": "Aufgezeichnete Aktionen",
  "settings.auditActor": "Ausgeführt von",
  "settings.auditEntity": "Objekttyp",
  "settings.auditEntityId": "Objekt-ID",
  "settings.auditAction": "Aktion",
  "settings.auditFrom": "Von",
  "settings.auditTo": "Bis",
  "settings.auditExpand": "Änderungsdetails anzeigen",
  "settings.auditRule": "Berechtigungsregel",
  "settings.auditOnBehalf": "im Auftrag von",
  "settings.privacy": "Datenschutzanfragen",
  "settings.privacySub": "Betroffenenanfragen mit ihren gesetzlichen Fristen",
  "settings.due": "fällig am {date}",

  "privacy.purposesReadOnly":
    "Nur Lesezugriff. Zum Hinzufügen eines Zwecks fehlt deiner Rolle eine Berechtigung.",
  "privacy.addPurpose": "Zweck hinzufügen",
  "privacy.corrections": "Korrekturanfragen",
  "privacy.correctionsSub":
    "Kontakte haben diese über den Link eingegeben, den sie per E-Mail erhalten haben. Keine davon hat den Datensatz bisher geändert; eine Korrektur bleibt eine Anfrage, bis jemand sie annimmt.",
  "privacy.correctionsEmpty": "Nichts offen",
  "privacy.correctionsEmptySub":
    "Korrekturen, die ein Kontakt über den eigenen Link sendet, erscheinen hier, damit jemand sie beantwortet.",
  "privacy.correctionRemoval": "Hat um Entfernung gebeten",
  "privacy.correctionDecide": "Entscheiden",
  "privacy.correctionUnnamed": "Kontakt ohne Namen",
  "privacy.correctionAcknowledge": "Als gelesen markieren",
  "privacy.correctionNote": "Begründung (der Kontakt kann nachfragen)",
  "privacy.correctionAccept": "Annehmen und aktualisieren",
  "privacy.correctionReject": "Unverändert lassen",
  "privacy.purposesRegistry": "Registrierte Zwecke",
  "privacy.purposeKey": "Schlüssel",
  "privacy.purposeLabel": "Bezeichnung",
  "privacy.purposeDoi": "Double-Opt-in erforderlich",
  "privacy.purposeCreate": "Zweck anlegen",
  "privacy.purposeAppendOnly":
    "Ein Zweck lässt sich nach dem Anlegen weder umbenennen noch entfernen; der Katalog kann nur ergänzt werden. Wähle den Schlüssel mit Bedacht.",
  "notice.title": "Informationspflichten",
  "notice.sub":
    "Kontakte, deren Daten nicht bei ihnen selbst erhoben wurden, und ob sie schon informiert wurden.",
  "notice.facetLabel": "Pflichten anzeigen",
  "notice.facetOwed": "Noch offen",
  "notice.facetAll": "Alle",
  "notice.emptyOwed":
    "Nichts offen. Jeder erhaltene Kontakt wurde informiert, oder die Pflicht wurde ohne Versand beendet.",
  "notice.readOnlyForPrivacy":
    "Diese Pflichten nennen Kontakte und wie sie erhalten wurden. Deshalb sehen sie nur Nutzende mit Zugriff auf Datenschutzanfragen.",
  "notice.dueAt": "Fällig am {date}",
  "notice.overdue": "Überfällig",
  "privacynotice.title": "Was wir über Sie gespeichert haben",
  "privacynotice.intro":
    "Wir teilen Ihnen das mit, weil das Gesetz es verlangt. Sie müssen nicht antworten und nichts tun.",
  "privacynotice.source.title": "Woher Ihre Daten stammen",
  "privacynotice.source.when": "Erhalten am {date}",
  "privacynotice.source.subjectInitiated": "Sie haben uns zuerst geschrieben.",
  "privacynotice.source.customerContract":
    "Sie sind Ansprechperson in einer Geschäftsbeziehung mit uns.",
  "privacynotice.source.requested":
    "Sie haben uns um ein Angebot oder einen Termin gebeten.",
  "privacynotice.source.inPerson":
    "Jemand hat ein Gespräch mit Ihnen festgehalten.",
  "privacynotice.source.referral": "Jemand anderes hat uns Ihre Daten gegeben.",
  "privacynotice.source.eventOrForm":
    "Sie haben ein Formular ausgefüllt oder sich für etwas angemeldet.",
  "privacynotice.source.publicSource":
    "Wir haben Ihre Daten in einer öffentlichen oder geschäftlichen Quelle gefunden, etwa einem Verzeichnis oder einer Unternehmenswebsite.",
  "privacynotice.source.purchasedOrImported":
    "Ihre Daten stammen aus einer gekauften oder importierten Liste.",
  "privacynotice.source.unknown":
    "Wir können nicht sagen, wie Ihre Daten zu uns gelangt sind.",
  "privacynotice.purposes.title": "Wofür wir sie nutzen",
  "privacynotice.rights.title": "Ihre Rechte daran",
  "privacynotice.rights.how":
    "Um eines dieser Rechte zu nutzen, antworten Sie auf die Nachricht, die Sie hierher geführt hat, oder wenden Sie sich an die Adresse auf unserer Website.",
  "privacynotice.right.access":
    "Um eine Kopie dessen bitten, was wir über Sie gespeichert haben.",
  "privacynotice.right.rectification": "Uns bitten, Falsches zu korrigieren.",
  "privacynotice.right.erasure": "Uns bitten, die Daten zu löschen.",
  "privacynotice.right.restriction":
    "Uns bitten, die Nutzung auszusetzen, solange etwas strittig ist.",
  "privacynotice.right.objection": "Der Nutzung widersprechen.",
  "privacynotice.right.complain":
    "Sich bei Ihrer Datenschutzbehörde beschweren.",
  "notice.claimed": "Übernommen",
  "notice.unclaimed": "Nicht übernommen",
  "notice.excuse": "Ohne Versand beenden",
  "notice.excuseTitle": "Pflicht ohne Versand der Information beenden",
  "notice.excuseWhich": "Art des Grunds",
  "notice.excuseProvided": "Bereits anderweitig informiert",
  "notice.excuseExempt": "Pflicht gilt nicht",
  "notice.excuseGround": "Grund in eigenen Worten",
  "notice.excuseConfirm": "Grund erfassen",
  "privacy.caseNotHere":
    "Die Anfrage aus diesem Link ist noch nicht auf dieser Seite.",
  "privacy.caseNotHereBody":
    "Möglicherweise steht sie unter einem anderen Statusfilter oder weiter unten in der Liste, die jeweils 20 Einträge lädt. Wähle ihren Status, oder lade mehr.",
  "privacy.facetAll": "Alle",
  "privacy.inboxAdminOnly":
    "Zum Ansehen von Betroffenenanfragen fehlt deiner Rolle eine Berechtigung. Die Anfragen nennen die Personen, die angefragt haben, deshalb ist der Zugriff eingeschränkt.",
  "privacy.overdue": "Überfällig",
  "privacy.closed":
    "Abgeschlossen. Eine abgeschlossene Anfrage lässt sich nicht wieder öffnen; ein neues Anliegen braucht eine neue Anfrage.",
  "privacy.assignee": "Zuständig",
  "privacy.assigneeUnassignable":
    "Einmal gesetzt, lässt sich die Zuständigkeit hier nicht entfernen.",
  "privacy.resolution": "Ergebnis",
  "privacy.resolutionRequired":
    "Zum Abschließen einer Anfrage ist ihre Antwort nötig.",
  "privacy.movedOn":
    "Jemand anders hat zuerst über diese Anfrage entschieden. Prüfe unten den aktuellen Stand.",
  "privacy.inProgress": "In Bearbeitung",
  "privacy.fulfil": "Erfüllen",
  "privacy.reject": "Ablehnen",
  "privacy.newRequest": "Neue Anfrage",
  "privacy.queue": "Anfragen",
  "privacy.kind": "Art",
  "privacy.contact": "Kontakt",
  "privacy.subjectRef": "Betroffenenreferenz",
  "privacy.dueAt": "Fällig",
  "privacy.openRequest": "Anfrage anlegen",
  "privacy.erasureNeedsContact":
    "Eine Löschanfrage muss einen Kontakt in diesem Unternehmen nennen, weil ihre Erfüllung genau diesen Datensatz löscht. Eine Freitextangabe lässt sich nicht löschen.",
  "privacy.accessManual":
    "Eine Auskunftsanfrage wird manuell erfüllt: Halte im Ergebnis fest, was du gesendet hast. Dieses System stellt die Daten nicht für dich zusammen und exportiert sie nicht.",
  "privacy.fulfilErasureTitle": "Löschanfrage erfüllen",
  "privacy.erasureIrreversible":
    "Damit wird der Kontakt im gesamten System dauerhaft gelöscht: Datensatz, erfasste Aktivitäten und abgeleitete Werte. Das lässt sich nicht rückgängig machen. Die Löschung selbst wird im Audit-Log festgehalten.",
  "privacy.typeErase": "Zum Bestätigen ERASE eingeben",
  "privacy.erasureConfirm": "Löschen und sperren",
  "privacy.legalHoldTitle": "Durch Legal Hold blockiert",
  "privacy.legalHold":
    "Für diesen Kontakt läuft eine gesetzliche Aufbewahrungsfrist, daher hat die Löschung hier keinen Vorrang (Art. 17 Abs. 3 lit. b). Die Sperre gilt für jede Rolle, auch für Admins, ohne Möglichkeit zur Übersteuerung. Der Versuch wurde im Audit-Log festgehalten.",

  "restricted.title": "Eingeschränkte Datensätze",
  "restricted.sub":
    "Datensätze, die eine gesetzliche Aufbewahrungspflicht nach einer Löschung sperrt: welcher Datensatz, warum und bis wann. Die Korrespondenz selbst ist ausgeblendet, damit sie nicht gelesen wird.",
  "restricted.withheld":
    "Nur Admins und Operations sehen, welche Datensätze eine gesetzliche Pflicht sperrt. Dafür gilt dieselbe Berechtigung wie für Aufbewahrungsregeln.",
  "restricted.empty":
    "Keine Datensätze gesperrt. Jede bisherige Löschung wurde vollständig ausgeführt.",
  "restricted.heldLabel": "Derzeit gesperrte Datensätze",
  "restricted.kind": "Datensatz",
  "restricted.occurred": "Datum",
  "restricted.deals": "Deal oder Projekt",
  "restricted.noDeal": "Kein Deal hinterlegt",
  "restricted.reason": "Gesperrt wegen",
  "restricted.until": "Gesperrt bis",
  "restricted.redacted": "Geschwärzt",
  "restricted.nothingRedacted": "Nichts entfernt",
  "restricted.redactedCount_one": "{count} Feld entfernt",
  "restricted.redactedCount_other": "{count} Felder entfernt",
  "restricted.class.commercialCorrespondence": "Handelskorrespondenz",
  "restricted.kind.email": "E-Mail",
  "restricted.kind.call": "Anruf",
  "restricted.kind.meeting": "Termin",
  "restricted.kind.message": "Nachricht",
  "restricted.decide": "Entscheidung",
  "restricted.reasonLabel": "Begründung",
  "restricted.reasonHint":
    "Wird mit deinem Namen im Audit-Log festgehalten. Nenne, was du entschieden hast und auf welcher Grundlage.",
  "restricted.release.action": "Aufheben",
  "restricted.release.title": "Legal Hold für den Datensatz aufheben?",
  "restricted.release.body":
    "Das Aufheben LÖSCHT den Datensatz; es gibt ihn nicht zur Nutzung zurück. Die Löschanfrage, die durch diese Pflicht ausgesetzt wurde, ist noch offen, deshalb schließt das Aufheben sie ab. Das lässt sich nicht rückgängig machen.",
  "restricted.release.confirm": "Aufheben und löschen",
  "restricted.pin.action": "Datensatz festsetzen",
  "restricted.pin.submit": "Festsetzen",
  "restricted.pin.idHint":
    "Für Korrespondenz, die die automatische Regel nicht erkennen kann, zum Beispiel Lieferanten- und Einkaufspost nach § 257 HGB ohne Deal in diesem System. Die Datensatz-ID steht im Audit-Eintrag des Datensatzes.",
  "restricted.pin.idMalformed":
    "Keine gültige Datensatz-ID. Das Format ist 8-4-4-4-12 Hexadezimalzeichen; die vollständige ID steht im Audit-Eintrag des Datensatzes.",
  "restricted.pin.idPlaceholder": "Datensatz-ID",
  "restricted.pin.title": "Datensatz unter Legal Hold stellen?",
  "restricted.pin.body":
    "Der Datensatz wird für die gesetzliche Frist gesperrt: in keiner normalen Ansicht sichtbar, unveränderbar und nach Ablauf der Frist gelöscht. Seine Kennungen werden jetzt geschwärzt.",
  "restricted.pin.confirm": "Festsetzen und sperren",
  "retention.title": "Aufbewahrung",
  "retention.sub":
    "Wie lange jede Art von Datensatz aufbewahrt wird und was nach Ablauf der Frist geschieht",
  "retention.retainOnly": "Nur-Aufbewahren-Modus",
  "retention.retainOnlyHelp":
    "Solange er aktiv ist, vernichtet diese Installation nichts: kein Anonymisieren und kein Löschen, unabhängig davon, was eine Regel unten vorsieht. Archivieren läuft weiter; ein archivierter Datensatz bleibt erhalten und wird nicht vernichtet.",
  "retention.adminOnly":
    "Nur Admins und Operations können die Aufbewahrung ändern.",
  "retention.withheld":
    "Nur Admins und Operations sehen die Aufbewahrungsregeln. Die Regeln legen fest, was diese Installation für alle aufbewahrt.",
  "retention.addPolicy": "Regel hinzufügen",
  "retention.create": "Regel anlegen",
  "retention.scope": "Gilt für",
  "retention.window": "Frist in Tagen",
  "retention.windowDays": "{days} Tage",
  "retention.windowInvalid": "Gib eine ganze Zahl von Tagen ein, mindestens 1.",
  "retention.action": "Aktion",
  "retention.actionHint":
    "Archivieren behält den Datensatz. Anonymisieren und Löschen vernichten Daten und werden im Nur-Aufbewahren-Modus zurückgehalten.",
  "retention.lawfulBasis": "Rechtsgrundlage",
  "retention.lawfulBasisHint":
    "Optional. Die Rechtsgrundlage nach Art. 6, auf die sich diese Frist stützt, für Personen, die die Regel prüfen.",
  "retention.enabled": "Aktiv",
  "retention.edit": "Bearbeiten",
  "retention.save": "Regel speichern",
  "retention.delete": "Regel löschen",
  "retention.deleteTitle": "Aufbewahrungsregel löschen?",
  "retention.deleteBody":
    "Damit entfällt die Regel für {scope} vollständig, und in diesem Bereich verfällt nichts mehr. Um die Regel zu pausieren und ihre Frist zu behalten, schalte stattdessen „Aktiv“ aus.",
  "retention.duplicateScope":
    "Für diesen Bereich gibt es bereits eine Regel; jeder Bereich hat höchstens 1 Regel. Bearbeite stattdessen die vorhandene Regel.",
  "retention.empty":
    "Noch keine Aufbewahrungsregel. In dieser Installation verfällt nichts.",
  "retention.effectActing": "Läuft jede Nacht",
  "retention.effectSuppressed": "Durch den Nur-Aufbewahren-Modus pausiert",
  "retention.effectDisabled": "Deaktiviert",
  "retention.suppressedWhy":
    "Aktiv, aber der Nur-Aufbewahren-Modus hält sie zurück: Diese Regel vernichtet Daten und greift erst, wenn der Modus ausgeschaltet ist.",
  "retention.disabledWhy":
    "Ausgeschaltet und beibehalten. Die Frist bleibt erhalten, und in diesem Bereich verfällt nichts, solange die Regel aus ist.",
  "retention.actionArchive": "Archivieren",
  "retention.actionAnonymize": "Anonymisieren",
  "retention.actionErase": "Löschen",
  "retention.scopeLeadUnconverted": "Nie konvertierte Leads",
  "retention.scopeActivity": "Alle erfassten Aktivitäten",
  "retention.scopeActivityTranscript": "Gesprächstranskripte",
  "retention.scopeContactNoConsentNoDeal":
    "Kontakte ohne Einwilligung und ohne Deal",
  "retention.scopeDealLost": "Verlorene Deals",
  "retention.scopeDealWon": "Gewonnene Deals",
  "retention.scopeAiCallPayloadContent": "Nutzdaten von KI-Aufrufen",

  "retention.scopeRawCapture": "Gespeicherte Originalnachrichten",
  "settings.pipelines": "Pipelines",
  "settings.pipelinesReadOnly":
    "Nur Lesezugriff. Deine Rolle darf Pipelines und Phasen nicht ändern.",
  "settings.pipelinesSub":
    "Phasen, die ein Deal durchläuft, eine Abfolge pro Pipeline.",
  "pipeline.new": "Neue Pipeline",
  "pipeline.edit": "Pipeline bearbeiten",
  "pipeline.name": "Name",
  "pipeline.default": "Standard",
  "pipeline.notDefault": "Kein Standard",
  "pipeline.position": "Position",
  "pipeline.retired": "Stillgelegt",
  "pipeline.retire": "Stilllegen",
  "pipeline.retireConfirm":
    "{name} stilllegen? Die Pipeline verschwindet aus Auswahllisten und Formularen für neue Deals. Deals darauf behalten Phase, Verlauf und Beitrag zum Forecast, und du kannst die Pipeline jederzeit wiederherstellen.",
  "pipeline.retireBlocked":
    "Dies ist die Standard-Pipeline, und neue Deals brauchen eine. Mache zuerst eine andere Pipeline zum Standard und lege dann diese still.",
  "pipeline.retired.done": "{name} stillgelegt",
  "pipeline.restore": "Wiederherstellen",
  "pipeline.restored": "{name} wiederhergestellt",
  "stage.new": "Neue Phase",
  "stage.edit": "Phase bearbeiten",
  "stage.name": "Name",
  "stage.semantic": "Phasentyp",
  "stage.winProb": "Gewinnwahrscheinlichkeit",
  "stage.semOpen": "Offen",
  "stage.semWon": "Gewonnen",
  "stage.semLost": "Verloren",
  "stage.remove": "Entfernen",
  "stage.removeConfirm": "Phase entfernen",
  "stage.removeTitle": "Diese Phase entfernen?",
  "stage.removeBody":
    "„{name}“ wird aus der Pipeline entfernt, spätere Phasen rücken auf. Frühere Phasenwechsel bleiben lesbar. Verschiebe die Deals dieser Phase, bevor du sie entfernst.",
  "stage.criteria.title": "Austrittskriterien",
  "stage.criteria.sub":
    "Was zutreffen muss, bevor ein Deal diese Phase verlässt.",
  "stage.criteria.buyerCalloutTitle":
    "Belege müssen von der Käuferseite kommen",
  "stage.criteria.buyerCallout":
    "Eine Nachricht deines Teams erfüllt nie ein Kriterium zu einer Handlung der Käuferseite.",
  "stage.criteria.unreadableTitle": "Kriterien nicht geladen",
  "stage.criteria.unreadable":
    "Lade die Seite neu, bevor du sie änderst; die Liste ist möglicherweise unvollständig.",
  "stage.criteria.none": "Noch keine Austrittskriterien.",
  "stage.criteria.terminal":
    "Eine Phase vom Typ „Gewonnen“ oder „Verloren“ ist endgültig und hat daher keine Austrittskriterien.",
  "stage.criteria.add": "Kriterium hinzufügen",
  "stage.criteria.key": "Schlüssel",
  "stage.criteria.keyHint":
    "Der Name, den Belege zitieren. Kleinbuchstaben, Ziffern und Unterstriche. Lässt sich später nicht ändern.",
  "stage.criteria.label": "Bezeichnung",
  "stage.criteria.kind": "Art",
  "stage.criteria.required": "Erforderlich",
  "stage.criteria.optional": "Optional",
  "stage.criteria.hint": "Hinweis",
  "stage.criteria.edit": "Kriterium bearbeiten",
  "stage.criteria.remove": "Entfernen",
  "stage.criteria.removeTitle": "Dieses Kriterium entfernen?",
  "stage.criteria.removeBody":
    "„{name}“ wird nicht mehr abgefragt. Bereits dazu erfasste Belege bleiben lesbar.",
  "stage.criteria.kindBuyerConfirmed": "Von Käuferseite bestätigt",
  "stage.criteria.kindEventHeld": "Termin stattgefunden",
  "stage.criteria.kindDocumentSigned": "Dokument unterschrieben",
  "stage.criteria.kindRoleIdentified": "Rolle identifiziert",
  "stage.criteria.kindTermsAccepted": "Bedingungen akzeptiert",
  "stage.criteria.kindCustom": "Eigenes",

  "ob.url": "Website",
  "ob.urlScheme": "https://",
  "ob.back": "Zurück",
  "ob.restoring": "Einrichtung wird wiederhergestellt…",
  "ob.readManual": "Manuell eingeben",
  "ob.coreIntroTitle": "Zuerst der Rechtsträger",
  "ob.coreIntroBody":
    "Margince braucht den rechtlichen Namen, die Anschrift und die USt-ID oder Registernummer, danach, was das Unternehmen verkauft und an wen.",
  "ob.coreLegalKicker": "Zuerst die rechtliche Identität",
  "ob.corePathLabel": "Was Margince lernt",
  "ob.corePathLegal": "Rechtliche Identität",
  "ob.corePathOffer": "Angebot",
  "ob.corePathCustomer": "Kunden",
  "ob.coreReadingPage": "Wird gelesen",
  "ob.coreWebsiteTitle": "Welche Website soll Margince lesen?",
  "ob.coreWebsiteBody":
    "Zuerst wird das Impressum gelesen, dann Produkte, Kunden und Positionierung.",
  "ob.corePreparing": "Lesen von {host} wird vorbereitet",
  "ob.coreLegalReading": "Rechtliche Identität auf {host} wird gelesen",
  "ob.coreLegalReadingBody":
    "Gesucht werden Impressum, Anschrift und Registernummer oder USt-ID. Nicht genannte Angaben bleiben leer.",
  "ob.coreBusinessReading": "Geschäftsmodell wird erfasst",
  "ob.coreBusinessReadingBody":
    "Produkte, Kunden und Positionierung werden mit dem öffentlichen Text verknüpft, der sie belegt.",
  "ob.coreReady_one": "{count} belegte Unternehmensangabe gefunden",
  "ob.coreReady_other": "{count} belegte Unternehmensangaben gefunden",
  "ob.corePartial_one":
    "{count} verwertbare Angabe gefunden. Einige Lücken bleiben offen.",
  "ob.corePartial_other":
    "{count} verwertbare Angaben gefunden. Einige Lücken bleiben offen.",
  "ob.coreReadyBody":
    "Noch ist nichts gespeichert. Prüfe zuerst die rechtliche Identität, dann Angebot und Kunden.",
  "ob.coreDeferredBody": "Das Lesen wird automatisch fortgesetzt.",
  "ob.coreFailedBody":
    "Die Website war nicht gut genug lesbar. Statt zu raten, wurde das Lesen beendet. Gib die Angaben manuell ein.",
  "ob.coreFindingsTitle": "Belegte Befunde",
  "ob.coreFindingsBody":
    "Jeder Wert zeigt den öffentlichen Wortlaut, auf dem er beruht. Nicht überprüfbare Werte bleiben leer.",
  "ob.ai.identity": "Margince",
  "ob.ai.role": "KI für Unternehmensrecherche",
  "ob.ai.speaker": "M",
  "ob.ai.speakerName": "Margince",
  "ob.ai.ready": "Bereit für die Recherche",
  "ob.ai.configured": "Konfigurierte KI",
  "ob.ai.modelsUsed": "In dieser Aufgabe verwendete Modelle",
  "ob.ai.route": "Aufgabe · Modellstufe · Anbieter",
  "ob.ai.calls": "KI-Aufrufe",
  "ob.ai.tokens": "Tokens",
  "ob.ai.latency": "Modelllatenz",
  "ob.ai.estimatedCost": "Geschätzte Anbieterkosten",
  "ob.ai.partialEstimate": "Teilweise · Nutzung ohne Preisangabe vorhanden",
  "ob.ai.awaitingModel": "Sichtbar nach dem ersten Modellaufruf",
  "ob.ai.notAvailableYet": "Noch nicht verfügbar",
  "ob.ai.runtimeUnavailable": "Laufzeitdetails nicht verfügbar",
  // Die Laufzeit-Offenlegung ist ein Chip zum Öffnen, kein Dauerband: Kosten
  // stehen da, WÄHREND sie entstehen, aber wer entscheidet, ob eine
  // Rechtsform stimmt, soll dafür keine Abrechnungstabelle lesen müssen.
  "ob.ai.runtimeChip": "Aktives Modell und Kosten",
  "ob.ai.answeringNow": "Aktives Modell",
  "ob.ai.runScope":
    "Nur dieser Lauf. Das vollständige Protokoll steht in den Einstellungen unter KI.",
  "ob.ai.tier.localSmall": "lokal, schnell",
  "ob.ai.tier.cheapCloud": "Cloud, effizient",
  "ob.ai.tier.premium": "Premium-Reasoning",
  "ob.ai.tier.frontier": "Frontier-Reasoning",
  "ob.ai.tier.localLarge": "lokal, erweitert",
  // Die Klartext-Zeile im Rail-Footer: Die genauen IDs sind einen Klick
  // entfernt in der Zeile „Konfigurierte KI“ des Laufzeit-Chips — hier steht
  // nur, was auf den ersten Blick zählt: wie viele Modelle, und wo sie laufen.
  "ob.ai.summary.cloud_one": "1 Modell, läuft in der Cloud",
  "ob.ai.summary.cloud_other": "{count} Modelle, laufen in der Cloud",
  "ob.ai.summary.local_one": "1 Modell, läuft lokal",
  "ob.ai.summary.local_other": "{count} Modelle, laufen lokal",
  "ob.ai.summary.hybrid_one": "1 Modell, teils in der Cloud, teils lokal",
  "ob.ai.summary.hybrid_other":
    "{count} Modelle, teils in der Cloud, teils lokal",
  "ob.ai.summary.development_one": "1 Modell, Entwicklungsmodus",
  "ob.ai.summary.development_other": "{count} Modelle, Entwicklungsmodus",
  "ob.ai.summary.none": "Noch kein Modell konfiguriert",
  "ob.ai.summaryProviders_one": "1 Anbieter konfiguriert",
  "ob.ai.summaryProviders_other": "{count} Anbieter konfiguriert",
  "ob.ai.readFirst":
    "Starte die Unternehmenseinrichtung, bevor du Fragen dazu stellst.",
  "ob.ai.liveArtifact": "Live-Entwurf zur Prüfung",
  "ob.ai.companyKnowledge": "Unternehmenswissen",
  "ob.ai.companyKnowledgeBody":
    "Belege von der Website bleiben von diesem Chat getrennt. Du entscheidest, was Unternehmenskontext wird.",
  "ob.ai.companyKnowledgeManualBody":
    "Deine Antworten und die Vorschläge von Margince bleiben hier bearbeitbar. Du entscheidest, was Unternehmenskontext wird.",
  "ob.ai.askPlaceholder":
    "Frage nach einem Befund, korrigiere eine Angabe oder ergänze, was fehlt",
  "ob.ai.send": "An Margince senden",
  "ob.ai.reviewBoundary":
    "Margince kann hier Änderungen vorschlagen und übernimmt sie erst nach deiner Freigabe in den Entwurf.",
  "ob.ai.confirmBoundary":
    "Nichts wird Unternehmenskontext, bevor du diesen Entwurf bestätigst.",
  "ob.ai.confirmCompany": "Unternehmen bestätigen und speichern",
  "ob.ai.thinking": "Dossier wird geprüft…",
  "ob.ai.suggestedChanges": "Vorgeschlagene Änderungen am Entwurf",
  "ob.ai.applyChanges": "In Entwurf übernehmen",
  "ob.ai.applied": "In Entwurf übernommen",
  "ob.ai.finding_one": "belegter Befund",
  "ob.ai.finding_other": "belegte Befunde",
  "ob.continueManual": "Manuell eingeben",
  "ob.readStatus.queued": "Wird vorbereitet",
  "ob.readStatus.deferred": "Wartet auf KI-Kontingent",
  "ob.readStatus.reading": "Wird gelesen",
  "ob.readStatus.ready": "Lesen abgeschlossen",
  "ob.readStatus.partial": "Mit Lücken abgeschlossen",
  "ob.readStatus.failed": "Eingabe erforderlich",
  "ob.readStatus.confirmed": "Auswahl gespeichert",
  "ob.readStatus.abandoned": "Angehalten",
  "ob.pagesRead": "gelesene Seiten",
  "ob.legalEntitiesFound": "gefundene Rechtsträger",
  "ob.coverageDetails": "Abdeckung und ungelesene Seiten",
  "ob.legalFoundTitle": "Gefundene Rechtsträger",
  "ob.legalFoundBody":
    "Jeder Block zeigt eingetragenen Namen, Anschrift und Registernummer oder USt-ID. Wähle in der Prüfung den richtigen Rechtsträger aus.",
  "ob.legalEntity": "Rechtsträger",
  "ob.confirmWebsite_one":
    "Basiert auf {count} öffentlichen Seite. Du kannst jeden Wert bearbeiten; unveränderte Werte behalten ihre Belege.",
  "ob.confirmWebsite_other":
    "Basiert auf {count} öffentlichen Seiten. Du kannst jeden Wert bearbeiten; unveränderte Werte behalten ihre Belege.",
  "ob.confirmManual":
    "Diese Antworten stammen von dir und werden als menschliche Aussagen gespeichert.",
  "ob.legalTitle": "Rechtsträger auswählen",
  "ob.legalSub":
    "Das Impressum nennt mehrere Rechtsträger. Wähle deinen aus, um seine Angaben einzutragen.",
  "ob.factsTitle": "Weitere gefundene Fakten",
  "ob.factsSelected": "{selected} von {total} ausgewählt",
  "ob.factsSub":
    "Wähle alle Fakten ab, die nicht Unternehmenskontext werden sollen. Bis zu 100 Fakten sind auswählbar.",

  // Keine Schrittzahl: wie viele Stationen ein Leser bekommt, entscheidet die
  // Leiste, also gehört die Zählung zu ob.conv.scene.step, das sie von dort
  // liest. Eine hier hineingeschriebene Summe kann nur falsch werden.
  "ob.s1.kick": "Bestätigung",
  "ob.s1.title": "Unternehmensangaben prüfen",
  "ob.s1.sub":
    "Ausgefüllt sind nur Angaben, die die Website belegt. Korrigiere, was nicht stimmt.",
  "ob.s1.urlPlaceholder": "deinunternehmen.de",
  "ob.s1.identityLabel": "Rechtsträger",
  "ob.s1.offerLabel": "Produkte und Angebot",
  "ob.s1.customerLabel": "Kunden",
  "ob.s1.salesLabel": "Positionierung und Vertriebskontext",
  "ob.s1.fieldRequired": "Pflichtfeld",
  "ob.s1.requiredMissing": "Fülle diese Felder aus, um fortzufahren: {fields}",
  "ob.s1.saving": "Wird gespeichert…",
  "ob.s1.saveFailed": "Unternehmen nicht gespeichert",
  "ob.s1.savedNote":
    "Im Unternehmen gespeichert. Änderungen hier werden beim Fortfahren erneut gespeichert.",
  "ob.readGo": "Website lesen",
  "ob.urlWillRead": "Margince liest {host}",
  "ob.readFromSite": "von der Website gelesen",
  "ob.failTitle": "Von dieser Website ließ sich nicht genug lesen",

  "ob.manualChapterLegal": "Rechtsträger",
  "ob.manualChapterOffer": "Produkte und Angebot",
  "ob.manualChapterCustomer": "Idealkunde",
  "ob.manualChapterSales": "Vertriebsansatz",
  "ob.manualNext": "Nächste Frage",
  "ob.manualLater": "Später ergänzen",
  "ob.manualReview": "Antworten prüfen",
  "ob.manualRequired": "Erforderlich für ein nutzbares Unternehmensprofil",
  "ob.manualOptional": "Optional. Leer lassen, um es später zu ergänzen.",
  "ob.manual.display_name":
    "Unter welchem Namen kennen Kunden dein Unternehmen?",
  "ob.manual.display_nameHint":
    "Der geläufige Name oder Handelsname, der überall in Margince erscheint.",
  "ob.manual.legal_name":
    "Wie lautet der vollständige eingetragene Name des Unternehmens?",
  "ob.manual.legal_nameHint":
    "Mit Rechtsform, sofern zutreffend, etwa GmbH, Ltd, Inc. oder AG.",
  "ob.manual.registered_address": "Wie lautet die eingetragene Anschrift?",
  "ob.manual.registered_addressHint":
    "Nutze die offizielle Anschrift aus dem Handelsregister oder dem Impressum.",
  "ob.manual.register_vat": "Wie lauten Registernummer und USt-ID oder UID?",
  "ob.manual.register_vatHint":
    "Gib die Kennungen genau so ein, wie sie vergeben wurden. Leer lassen, wenn keine zutrifft.",
  "ob.manual.legal_form": "Welche Rechtsform hat das Unternehmen?",
  "ob.manual.legal_formHint":
    "Die Rechtsform, wie sie im Register steht, etwa GmbH, AG oder Ltd.",
  "ob.manual.register_court": "Welches Gericht führt den Registereintrag?",
  "ob.manual.register_courtHint":
    "Das im Impressum genannte Gericht, etwa Amtsgericht Charlottenburg.",
  "ob.manual.register_number": "Wie lautet die Handelsregisternummer?",
  "ob.manual.register_numberHint":
    "Nur der Registereintrag, etwa HRB 12345 B. Die USt-ID gehört in das Feld darüber.",
  "ob.manual.industry": "In welcher Branche ist das Unternehmen tätig?",
  "ob.manual.industryHint": "Die Beschreibung, die Kunden sofort erkennen.",
  "ob.manual.history":
    "Gibt es nützliche Angaben zur Unternehmensgeschichte, die Margince kennen sollte?",
  "ob.manual.historyHint":
    "Zum Beispiel Gründungsjahr, Ursprung oder eine wichtige Veränderung im Geschäft.",
  "ob.manual.offer_summary":
    "Welche Produkte oder Leistungen verkauft dein Unternehmen?",
  "ob.manual.offer_summaryHint":
    "1 oder 2 konkrete Sätze. Margince nutzt sie als geschäftliche Beschreibung.",
  "ob.manual.value_proposition": "Welches Ergebnis schafft das Angebot?",
  "ob.manual.value_propositionHint":
    "Der Nutzen, den Kunden erhalten, über die Produktfunktionen hinaus.",
  "ob.manual.usp": "Warum entscheiden sich Kunden für dein Unternehmen?",
  "ob.manual.uspHint": "Der stärkste Unterschied zu den Alternativen.",
  "ob.manual.icp": "Wer ist dein Idealkunde?",
  "ob.manual.icpHint":
    "Die Unternehmen oder Personen mit dem größten Nutzen: Größe, Branche, Situation oder Region.",
  "ob.manual.buying_center": "Wer prüft, kauft oder gibt den Kauf frei?",
  "ob.manual.buying_centerHint": "Typische Rollen und wer das letzte Wort hat.",
  "ob.manual.customer_pains":
    "Mit welchen Problemen kommen diese Kunden zu deinem Unternehmen?",
  "ob.manual.customer_painsHint": "Probleme so, wie Kunden sie beschreiben.",
  "ob.manual.desired_outcomes": "Was wollen sie erreichen?",
  "ob.manual.desired_outcomesHint":
    "Praktische oder geschäftliche Ergebnisse, die ihnen wichtig sind.",
  "ob.manual.buying_intents": "Was deutet meist auf Kaufinteresse hin?",
  "ob.manual.buying_intentsHint":
    "Zum Beispiel eine neue Initiative, ein Einstellungsmuster, eine Frist oder ein operatives Problem.",
  "ob.manual.common_objections": "Welche Einwände hörst du am häufigsten?",
  "ob.manual.common_objectionsHint":
    "Bedenken, die einen Kauf oft verzögern oder verhindern.",
  "ob.manual.sales_motion": "Wie läuft ein typischer Verkauf ab?",
  "ob.manual.sales_motionHint":
    "Der Weg vom Erstgespräch bis zur Entscheidung, sofern relevant mit Testphase oder Einkaufsprozess.",

  "ob.field.display_name": "Unternehmensname",
  "ob.field.offer_summary": "Produkte und Leistungen",
  "ob.field.icp": "Idealkunde",
  "ob.field.buying_center": "Buying Center",
  "ob.field.value_proposition": "Nutzenversprechen",
  "ob.field.usp": "Alleinstellungsmerkmal",
  "ob.field.customer_pains": "Kundenprobleme",
  "ob.field.desired_outcomes": "Gewünschte Ergebnisse",
  "ob.field.buying_intents": "Kaufsignale",
  "ob.field.common_objections": "Häufige Einwände",
  "ob.field.sales_motion": "Vertriebsprozess",
  "ob.field.legal_name": "Eingetragener Name",
  "ob.field.registered_address": "Eingetragene Anschrift",
  "ob.field.register_vat": "Registernummer und USt-ID",
  "ob.field.legal_form": "Rechtsform",
  "ob.field.register_court": "Registergericht",
  "ob.field.register_number": "Registernummer",
  "ob.field.industry": "Branche",
  "ob.field.history": "Unternehmensgeschichte",

  "ob.fieldHint.display_name":
    "Der Name, den Kunden verwenden, nicht der rechtliche Name. Erscheint überall in Margince.",
  "ob.fieldHint.offer_summary":
    "1 oder 2 klare Sätze dazu, was das Unternehmen verkauft.",
  "ob.fieldHint.icp":
    "Wer am meisten profitiert, nach Größe, Branche oder Situation.",
  "ob.fieldHint.buying_center": "Rollen, die prüfen oder freigeben.",
  "ob.fieldHint.value_proposition":
    "Das Ergebnis, das Kunden erhalten, keine Produktfunktion.",
  "ob.fieldHint.usp":
    "Der Unterschied, der über einen Kauf entscheidet, keine Stärke, die jeder Wettbewerber für sich beansprucht.",
  "ob.fieldHint.customer_pains":
    "Das Problem in den eigenen Worten der Kunden.",
  "ob.fieldHint.desired_outcomes":
    "Was Kunden erreichen wollen, in geschäftlichen Begriffen.",
  "ob.fieldHint.buying_intents":
    "Ein Signal, dass ein potenzieller Kunde kurz vor dem Kauf steht, etwa eine Neueinstellung oder eine Frist.",
  "ob.fieldHint.common_objections":
    "Der Einwand, der einen Deal am häufigsten verzögert oder stoppt.",
  "ob.fieldHint.sales_motion":
    "Der Weg vom Erstgespräch bis zum unterschriebenen Deal, einschließlich einer eventuellen Testphase oder eines Einkaufsschritts.",
  "ob.fieldHint.legal_name":
    "Der eingetragene Name inklusive Rechtsform. Wird auf Rechnungen verwendet.",
  "ob.fieldHint.registered_address":
    "Die Anschrift aus dem Impressum, keine Postanschrift oder Showroom-Adresse.",
  "ob.fieldHint.register_vat":
    "Beide Kennungen genau wie vergeben. Beide erscheinen auf Rechnungen und Verträgen.",
  "ob.fieldHint.legal_form": "Die Rechtsform, wie sie im Register steht.",
  "ob.fieldHint.register_court":
    "Das im Impressum genannte Gericht, das den Registereintrag führt.",
  "ob.fieldHint.register_number":
    "Nur der Registereintrag. Die USt-ID hat ein eigenes Feld darüber.",
  "ob.fieldHint.industry":
    "Die Beschreibung, die Kunden wiedererkennen, kein interner Klassifizierungscode.",
  "ob.fieldHint.history":
    "Nur, wenn es das Verständnis des Unternehmens verändert, etwa ein Gründungsjahr oder eine große Neuausrichtung.",

  "ob.fieldEg.display_name": "Northwind Robotics",
  "ob.fieldEg.offer_summary":
    "Cloud-Software für die Bestandsverwaltung mittelgroßer Händler.",
  "ob.fieldEg.icp": "Handelsketten mit 20 bis 200 Filialen.",
  "ob.fieldEg.buying_center":
    "Leitung Operations, Freigabe durch die Finanzabteilung.",
  "ob.fieldEg.value_proposition":
    "Halbiert Fehlbestände innerhalb eines Quartals.",
  "ob.fieldEg.usp": "Einziger Anbieter mit Vor-Ort-Support am selben Tag.",
  "ob.fieldEg.customer_pains": "„Der Bestand ist leer, bevor es jemand merkt.“",
  "ob.fieldEg.desired_outcomes": "Nie wieder eine Nachbestellfrist verpassen.",
  "ob.fieldEg.buying_intents":
    "Eröffnung eines neuen Lagers innerhalb von 90 Tagen.",
  "ob.fieldEg.common_objections": "Sorge vor dem Umstieg vom alten System.",
  "ob.fieldEg.sales_motion":
    "Demo, ein zweiwöchiger Pilot, dann ein Jahresvertrag.",
  "ob.fieldEg.legal_name": "Northwind Robotics GmbH",
  "ob.fieldEg.registered_address": "Musterstraße 12, 10115 Berlin",
  "ob.fieldEg.register_vat": "DE123456789",
  "ob.fieldEg.legal_form": "GmbH",
  "ob.fieldEg.register_court": "Amtsgericht Charlottenburg",
  "ob.fieldEg.register_number": "HRB 12345 B",
  "ob.fieldEg.industry": "E-Commerce-Logistik",
  "ob.fieldEg.history":
    "Gegründet 2015, ausgegliedert aus einem Logistik-Startup.",

  "ob.s4.provGoogle": "Google",
  "ob.s4.provMicrosoft": "Microsoft",
  "ob.s4.provImap": "Anderes Postfach (IMAP/SMTP)",
  "ob.s4.microsoftBtn": "Microsoft verbinden",
  "ob.s4.microsoftHint":
    "Liest deine E-Mails und kann aus dem Postfach senden. Beides erlaubst du auf der Seite von Microsoft und kannst die Verbindung jederzeit trennen.",
  "ob.s4.microsoftUnverified":
    "Möglicherweise erscheint ein Hinweis „Nicht verifizierte App“. Er bezieht sich auf diese selbst gehostete Installation, nicht auf einen Dritten.",
  "ob.s4.microsoftFailed": "Microsoft-Verbindung nicht abgeschlossen",
  "ob.s4.connectOkTitle": "Postfach verbunden",
  "ob.s4.connectOkBody":
    "Die Erfassung beginnt bei der nächsten Synchronisierung.",
  "ob.s4.connectVerifying": "Verbindung wird bestätigt…",
  "ob.s4.connectLive": "Aktiv, Erfassung läuft",
  "ob.s4.connectConfirmFailed": "Die Verbindung konnte nicht bestätigt werden.",
  "ob.s4.connectRetry":
    "Verbinde das Postfach erneut in den Einstellungen unter Verbindungen.",
  "ob.s4.connectDenied": "Zugriff abgelehnt. Es wurde nichts verbunden.",
  "ob.s4.googleBtn": "Gmail verbinden",
  "ob.s4.googleHint":
    "Liest deine E-Mails und kann aus dem Postfach senden. Beides erlaubst du auf der Seite von Google und kannst die Verbindung jederzeit trennen.",
  "ob.s4.googleUnverified":
    "Wenn Google vor einer „nicht verifizierten App“ warnt, wähle „Erweitert“ und dann „Weiter“. Die Seite von Google zeigt genau, was erlaubt wird.",
  "backfill.title": "Postfachverlauf importieren",
  "backfill.intro":
    "Wähle, wie weit zurück importiert wird. Umfang und geschätzte Kosten werden angezeigt, bevor etwas startet, und dieser Schritt lässt sich überspringen.",
  "backfill.windowLabel": "Importzeitraum",
  "backfill.window36m": "3 Jahre",
  "backfill.window84m": "7 Jahre",
  "backfill.window120m": "10 Jahre",
  "backfill.since": "Importiert E-Mails seit {date}.",
  "backfill.extendNote":
    "Der Verlauf lässt sich später erweitern. Bereits importierte E-Mails bleiben erhalten und werden nicht doppelt angelegt.",
  "backfill.costFloorNote":
    "Diese Schätzung umfasst nur die bisher gezählten Nachrichten. Der vollständige Import kann mehr Nachrichten enthalten und mehr kosten.",
  "backfill.window3m": "3 Monate",
  "backfill.window6m": "6 Monate",
  "backfill.window12m": "1 Jahr",
  "backfill.window24m": "2 Jahre",
  "backfill.window60m": "5 Jahre",
  "backfill.previewLoading": "Nachrichten werden gezählt…",
  "backfill.scopeIs": "Importiert {window} deines Postfachs.",
  "backfill.estimateMessagesExact_one": "{count} Nachricht in diesem Zeitraum.",
  "backfill.estimateMessagesExact_other":
    "{count} Nachrichten in diesem Zeitraum.",
  "backfill.estimateMessagesAtLeast_one":
    "Mindestens {count} Nachricht in diesem Zeitraum. Dort wurde das Zählen beendet, es können also mehr sein.",
  "backfill.estimateMessagesAtLeast_other":
    "Mindestens {count} Nachrichten in diesem Zeitraum. Dort wurde das Zählen beendet, es können also mehr sein.",
  "backfill.estimateCost": "Geschätzte KI-Kosten:",
  "backfill.estimateNote":
    "Eine Schätzung, keine Rechnung. Die tatsächliche Nutzung wird gemessen und laufend angezeigt.",
  "backfill.startCta": "Import starten",
  "backfill.starting": "Wird gestartet…",
  "backfill.skip": "Import des Postfachverlaufs überspringen",
  "backfill.skippedNote":
    "Kein Verlauf importiert. Neue E-Mails werden weiterhin erfasst, und ein Import lässt sich später in den Einstellungen starten.",
  "backfill.loading": "Importstatus wird geprüft…",
  "backfill.statusUnavailable":
    "Importstatus ist nicht verfügbar. Die Erfassung läuft weiter.",
  "backfill.queuedTitle": "Import eingereiht",
  "backfill.runningTitle": "Postfachverlauf wird importiert",
  "backfill.readingBadge": "Wird analysiert",
  "backfill.doneTitle": "Import des Postfachverlaufs abgeschlossen",
  "backfill.errorTitle": "Importfehler",
  "backfill.cancelledTitle": "Import abgebrochen",
  "backfill.progressLabel": "Importfortschritt",
  "backfill.countScanned": "Durchsuchte Nachrichten",
  "backfill.statEmails": "Erfasste E-Mails",
  "backfill.statContacts": "Kontakte",
  "backfill.statCompanies": "Zu prüfende Unternehmen",
  "backfill.errorNote":
    "Der Import wird automatisch wiederholt. Alles bisher Erfasste bleibt erhalten.",
  "backfill.cancel": "Import stoppen",
  "backfill.cancelledNote": "Gestoppt. Alles bisher Erfasste bleibt erhalten.",
  "backfill.restart": "Weiteren Import starten",
  "backfill.unsupportedNote":
    "Dieser Postfachtyp unterstützt keinen Import des Verlaufs. Nur neue E-Mails werden erfasst.",
  "backfill.narrowingNote":
    "Für dieses Postfach lief bereits ein größerer Zeitraum. Der Importzeitraum kann nur erweitert werden.",
  "backfill.staleUpdated":
    "Zuletzt aktualisiert vor {duration}. Kein aktueller Fortschritt.",

  // Connected inboxes (Einstellungen → Verbindungen).
  // Die Einheiten dieser Installation, auf der Einstellungsseite, die bereits
  // die Art von Zugangsdaten trägt, mit der die jeweilige Einheit konfiguriert
  // wird.
  "extUnits.open": "Öffnen",
  "extUnits.openNamed": "Seite {name} öffnen",
  "extUnits.user.title": "Deine weiteren Konten",
  "extUnits.user.sub":
    "Konten, die diese Installation in deinem Namen verbinden kann. Jedes Konto ist privat, und das Trennen betrifft nur dich.",
  "extUnits.workspace.title": "Erweiterungen der Installation",
  "extUnits.workspace.sub":
    "Erweiterungen, die diese Installation mit gemeinsamen Zugangsdaten betreibt. Einstellungen hier gelten für alle Nutzenden.",

  "connectors.title": "Verbundene Postfächer und Kalender",
  // Die dauerhafte Nacht-Vollmacht der Nutzerin — eine Frage, gestellt beim
  // Postfach-Verbinden im Onboarding und noch einmal in den Einstellungen.
  "overnightGrant.title": "Vorbereitung über Nacht",
  "overnightGrant.sub":
    "Margince arbeitet über Nacht deine Deals durch, und der Morgenbericht liegt bereit, wenn du anfängst. Margince handelt in deinem Namen, sieht nur, was du sehen kannst, und du kannst es jederzeit stoppen.",
  "overnightGrant.label":
    "Margince den Morgenbericht über Nacht vorbereiten lassen",
  "overnightGrant.help":
    "Margince liest deine Deals und E-Mails, um die Prioritäten des Tages zu ordnen, und schreibt Notizen zurück. Senden kann es nicht: Die hier erteilte Berechtigung umfasst nur Lesen und Schreiben, niemals Senden.",
  "overnightGrant.dangerTitle": "Vorbereitung über Nacht läuft nicht",
  "overnightGrant.danger":
    "Margince kann deinen Morgenbericht weder lesen noch mit Anmerkungen versehen. Deine Datensätze, die Worklist und der geplante Wochenrückblick bleiben verfügbar.",
  "overnightGrant.saveFailedTitle": "Antwort nicht gespeichert",
  "overnightGrant.saveFailed":
    "Alles andere ist verbunden. Lege das nach der Anmeldung in den Einstellungen unter Verbindungen fest.",
  "overnightGrant.renewTitle": "Vollmacht für die Nacht abgelaufen",
  "overnightGrant.renew":
    "Schalte die Option aus und wieder ein, um sie zu erneuern. Bis dahin wird dein Morgenbericht nicht vorbereitet.",
  "overnightGrant.renewScopeTitle": "Vollmacht deckt die Arbeit nicht mehr ab",
  "overnightGrant.writeFailedTitle": "Änderung nicht gespeichert",
  "overnightGrant.renewScope":
    "Margince kann inzwischen mehr als zum Zeitpunkt deines Einverständnisses. Schalte die Option aus und wieder ein, um sie zu erweitern. Bis dahin wird dein Morgenbericht nicht vorbereitet.",
  "aiHealth.title": "Modellstufen",
  "aiHealth.sub":
    "Ob jede Modellstufe antwortet. Eine ausgefallene und eine vorsichtige Modellstufe sehen an anderer Stelle gleich aus; erfasste E-Mails bleiben in beiden Fällen zurückgehalten.",
  "aiHealth.noCalls": "Keine Modellaufrufe in den letzten {hours} h.",
  "aiHealth.colTier": "Modellstufe",
  "aiHealth.colState": "Zustand",
  "aiHealth.colCalls": "Letzte {hours} h",
  "aiHealth.colLatency": "Median",
  "aiHealth.colLast": "Letzte Antwort",
  "aiHealth.answering": "Antwortet",
  "aiHealth.notAnswering": "Antwortet nicht",
  "aiHealth.callCounts_one": "Aufrufe: {count}, fehlgeschlagen: {failures}",
  "aiHealth.callCounts_other": "Aufrufe: {count}, fehlgeschlagen: {failures}",
  "aiHealth.ms": "{ms} ms",
  "heldThreads.title": "Zurückgehaltene Threads",
  "heldThreads.sub":
    "Threads, die dein Postfach zurückhält. Gibst du einen Thread frei, können alle Teammitglieder ihn lesen; nur du kannst deine freigeben.",
  "heldThreads.empty": "Dein Postfach hält keine Threads zurück.",
  "heldThreads.colThread": "Thread",
  "heldThreads.colWhy": "Grund",
  "heldThreads.colWhen": "Eingegangen",
  "heldThreads.colActions": "Aktionen",
  "heldThreads.release": "Mit dem Team teilen",
  "heldThreads.released": "Mit dem Team geteilt",
  "heldThreads.noSubject": "Erste Nachricht gelöscht",
  "heldThreads.nothingToShare":
    "Keine Nachricht mehr zum Teilen. Die erste Nachricht wurde gelöscht, und der Thread bleibt zurückgehalten, damit eine spätere Antwort nicht geteilt eintrifft.",
  "heldThreads.pending": "Wartet auf Einstufung",
  "heldThreads.attempts_one": "{count}-mal angefragt",
  "heldThreads.attempts_other": "{count}-mal angefragt",
  "heldThreads.backlogStalled_one":
    "Zu {count} Thread wurde wiederholt ohne Antwort angefragt. E-Mails bleiben zurückgehalten, bis der Klassifikator wieder antwortet; nichts geht verloren.",
  "heldThreads.backlogStalled_other":
    "Zu {count} Threads wurde wiederholt ohne Antwort angefragt. E-Mails bleiben zurückgehalten, bis der Klassifikator wieder antwortet; nichts geht verloren.",
  "heldThreads.heldByOthers_one":
    "{count} weiteres Postfach hat diese Nachricht importiert und nicht geteilt. Ein Thread wird erst geöffnet, wenn alle Empfangenden zustimmen.",
  "heldThreads.heldByOthers_other":
    "{count} weitere Postfächer haben diese Nachricht importiert und nicht geteilt. Ein Thread wird erst geöffnet, wenn alle Empfangenden zustimmen.",
  "heldThreads.releaseFailed": "Der Thread wurde nicht geteilt",
  "heldThreads.stillHeldTitle": "Wartet auf andere Postfächer",
  "heldThreads.backlogStalledTitle": "Klassifikator antwortet nicht",
  "heldThreads.kind.legal": "Rechtliches",
  "heldThreads.kind.financialCorporate": "Unternehmensfinanzen",
  "heldThreads.kind.personnel": "Personalangelegenheiten",
  "heldThreads.kind.personal": "Privat",
  "heldThreads.kind.securityIncident": "Sicherheitsvorfall",
  "heldThreads.kind.explicitlyConfidential": "Als vertraulich markiert",
  "senders.title": "Absender",
  "senders.sub":
    "Die Entscheidung zu jeder Adresse, die dein Postfach eingebracht hat, und deine eigene Antwort, sofern gegeben. Nur du siehst diese Liste.",
  "senders.emptyTitle": "Noch keine Absender",
  "senders.emptyBody":
    "Sobald dein Postfach E-Mails einbringt, erscheint hier jeder Absender mit seinem Ergebnis.",
  "senders.colSender": "Absender",
  "senders.colDecision": "Entscheidung",
  "senders.colRecord": "Kontakt",
  "senders.colActions": "Aktionen",
  "senders.recordYes": "Ja",
  "senders.recordNo": "Nein",
  "senders.byYou": "Deine Entscheidung",
  "senders.deletesOn": "Älteste Nachricht am {date} gelöscht",
  "senders.markBusiness": "Geschäftlich",
  "senders.keepOut": "Ausschließen",
  "senders.withdraw": "Rückgängig machen",
  "senders.keepOutTitle": "Diesen Absender dauerhaft ausschließen?",
  "senders.keepOutBody":
    "Es wird kein Kontakt angelegt, und E-Mails, die dieser Absender bereits in dein Postfach gebracht hat, werden vernichtet. Kopien, die ein Teammitglied importiert hat, bleiben bei diesem Teammitglied.",
  "senders.keepOutConfirm": "Ausschließen und vernichten",
  "senders.kind.contact": "Person",
  "senders.kind.roleMailbox": "Funktionspostfach",
  "senders.kind.companySender": "Unternehmen",
  "senders.kind.newsletter": "Newsletter",
  "senders.kind.transactional": "Automatisiertes Tool",
  "senders.kind.spam": "Spam",
  "senders.kind.personal": "Privat",
  "senders.kind.advisor": "Beratung",
  "senders.kind.business": "Geschäftlich",
  "senders.kind.keptOut": "Ausgeschlossen",
  "senders.kind.undecided": "Nicht entschieden",
  "mailSharing.title": "Teilen von E-Mails",
  "mailSharing.sub":
    "Erfasste E-Mails sind für alle Teammitglieder lesbar, die den Kontakt sehen können. Standardmäßig eingeschaltet, damit die Arbeit an Deals geteilt werden kann.",
  "mailSharing.label": "Erfasste E-Mails mit dem Team teilen",
  "mailSharing.help":
    "Einzelne Nachrichten lassen sich nachträglich einschränken, Adressen oder Domains vorab ausschließen.",
  "mailSharing.danger":
    "Ohne das Teilen von E-Mails ist das CRM schwer zu nutzen. Neue E-Mails sind dann nur für die Beteiligten der jeweiligen Nachricht sichtbar.",
  "mailSharing.posture.shared":
    "Neu erfasste E-Mails sind für Teammitglieder lesbar, die den Kontakt sehen können.",
  "mailSharing.posture.private":
    "Neu erfasste E-Mails bleiben auf die Beteiligten der Nachricht und das erfassende Postfach beschränkt.",
  "mailSharing.posture.where": "In den Erfassungsregeln ändern",
  "mailSharing.sharedPosture.label":
    "Teilen beim Eingang für Postfächer erlauben",
  "mailSharing.sharedPosture.help":
    "Erlaubt Teammitgliedern, ihr Postfach auf „geteilt“ zu stellen, sodass erfasste E-Mails beim Eingang für das Team lesbar sind, noch vor jeder Einstufung. Standardmäßig aus.",
  "mailSharing.sharedPosture.warning":
    "Das Postfach von Mitarbeitenden in ein gemeinsames CRM einzulesen, ist in Deutschland und Österreich Gegenstand einer Betriebsvereinbarung. Mit dem Einschalten wird erklärt, dass dein Unternehmen eine solche hat. Margince prüft das nicht.",
  "mailSharing.dangerTitle": "Teilen von E-Mails ist aus",
  "mailSharing.sharedPosture.warningTitle":
    "Damit wird eine Rechtsgrundlage erklärt",
  "mailSharing.saveFailed": "Einstellung nicht gespeichert",
  "mailSharing.save": "Speichern",
  "connectors.originLabel": "Adresse in versendeten Links",
  "connectors.originReachable": "Erreichbar",
  "connectors.originUnreachable": "Nicht erreichbar",
  "connectors.originUnchecked": "Nicht geprüft",
  "connectors.sub":
    "Postfächer und Kalender, aus denen das CRM Daten erfasst. Beim Trennen bleiben die bereits erfassten Datensätze erhalten.",
  "connectors.loading": "Connectors werden geladen…",
  "connectors.loadFailed":
    "Connectors konnten nicht geladen werden. Lade die Seite neu.",
  "connectors.empty": "Noch kein Postfach und kein Kalender verbunden.",
  "connectors.provGmail": "Gmail",
  "connectors.provGcal": "Google Kalender",
  "connectors.provGraph": "Outlook",
  "connectors.provGraphCal": "Outlook-Kalender",
  "connectors.provImap": "IMAP-Postfach",
  "connectors.provTestMailbox": "Testpostfach",
  "connectors.statusConnected": "Erfassung läuft",
  "connectors.statusPending": "Bestätigung ausstehend",
  "connectors.statusReauth": "Neuverbindung nötig",
  "connectors.statusError": "Synchronisierungsfehler",
  "connectors.statusDisconnected": "Getrennt",
  "connectors.cannotSend": "Nur Erfassung, kein Versand",
  "connectors.reconnectToSend":
    "Verbinde dieses Postfach neu, um daraus zu senden. Ein Postfach, das vor Einführung des Versands verbunden wurde, lässt sich nicht nachträglich erweitern; der Anbieter erteilt die Sendeberechtigung nur bei einer neuen Verbindung.",
  "connectors.lastSynced": "Zuletzt synchronisiert {at}",
  "connectors.neverSynced": "Wartet auf erste Synchronisierung",
  "connectors.nextCheck": "Nächste Prüfung gegen {at}",
  "connectors.polled": "Nach Zeitplan abgefragt (kein Push-Abonnement)",
  "connectors.pushRenewal": "Push-Erneuerung bis {at}",
  "connectors.notConfigured":
    "Die E-Mail-Erfassung ist in dieser Installation nicht konfiguriert.",
  "connectors.reconnect": "Neu verbinden",
  "connectors.disconnect": "Trennen",
  "connectors.signatureEnrich.label": "Kontaktdaten aus diesem Postfach lesen",
  "connectors.contextTag.label": "Tag für angelegte Kontakte",
  "connectors.contextTag.none": "Kein Tag",
  "connectors.contextTag.hint":
    "Jeder Kontakt, den dieser Connector ab jetzt anlegt, erhält dieses Tag. Vorhandene Kontakte behalten ihre Tags.",
  "connectors.contextTag.archived":
    "{name} ist archiviert, daher wird damit nichts getaggt. Wähle ein anderes Tag oder keines.",
  "connectors.signatureEnrich.followingDefault":
    "Folgt der Unternehmenseinstellung. Eine Änderung hier gibt diesem Postfach eine eigene Einstellung.",
  "connectors.signatureEnrich.ownAnswer":
    "Eigene Einstellung dieses Postfachs, unabhängig von der Unternehmenseinstellung.",
  "hold.sectionTitle": "Private Korrespondenz",
  "hold.notHeld":
    "E-Mails mit diesem Kontakt folgen der Einstellung deines Postfachs.",
  "hold.heldByAddress":
    "E-Mails mit dieser Adresse sind nur für die Beteiligten sichtbar.",
  "hold.heldByDomain":
    "E-Mails mit {domain} sind nur für die Beteiligten sichtbar.",
  "hold.holdAddress": "Privat halten",
  "hold.holdDomain": "Alles von {domain} privat halten",
  "hold.lift": "Aufheben",
  "hold.liftingWidensNothing":
    "Das Aufheben gilt für neue E-Mails. Bereits privat gehaltene E-Mails bleiben privat.",
  "hold.confirmVerb": "Privat halten",
  "hold.confirmTitle": "Diese Korrespondenz privat halten?",
  "hold.confirmAddressBody":
    "E-Mails mit {address} bleiben nur für die Beteiligten sichtbar. Die E-Mails werden weiterhin erfasst, und du kannst sie weiter lesen, andere Teammitglieder nicht.",
  "hold.confirmDomainBody":
    "E-Mails mit allen Adressen bei {domain}, einschließlich Subdomains, bleiben nur für die Beteiligten sichtbar. Die E-Mails werden weiterhin erfasst, und du kannst sie weiter lesen, andere Teammitglieder nicht.",
  "hold.confirmHistoryNote":
    "Das gilt für neue E-Mails. Bereits erfasste E-Mails behalten ihre bisherige Sichtbarkeit.",
  "captureNotice.title": "Was das Verbinden eines Postfachs bedeutet",
  "captureNotice.whatHappens":
    "Margince liest dieses Postfach und legt ab, was es findet: die Nachrichten, wer daran beteiligt war, und die Kontakte und Unternehmen hinter den Adressen. Anhänge werden mit ihrer Nachricht gespeichert.",
  "captureNotice.whoReads":
    "Ein neues Postfach wird standardmäßig zurückgehalten. Eine Nachricht bleibt bei den Beteiligten, bis ein Klassifikator den Thread als gewöhnliche geschäftliche Korrespondenz einstuft. Erst dann können andere Teammitglieder sie lesen. Du kannst das Postfach jederzeit so einstellen, dass stattdessen alles zurückgehalten wird.",
  "captureNotice.yourControl":
    "Du entscheidest pro Absender und pro Thread, in den Einstellungen unter Verbindungen: einen Absender ganz ausschließen, einen Thread mit dem Team teilen oder löschen, was ein Absender eingebracht hat. Hier wird nicht um dein Einverständnis gebeten. Der Text beschreibt, was passiert, damit du es vor dem Verbinden weißt.",
  "connectors.mailPosture.label": "Sichtbarkeit der E-Mails",
  "connectors.mailPosture.classified": "Bis zur Einstufung zurückgehalten",
  "connectors.mailPosture.held": "Immer zurückgehalten",
  "connectors.mailPosture.shared": "Mit dem Team geteilt",
  "connectors.mailPosture.sharedNeedsAdmin":
    "„Mit dem Team geteilt“ muss ein Admin für dieses Unternehmen erlauben.",
  "connectors.mailPosture.help.classified":
    "Neue Nachrichten bleiben auf die Beteiligten beschränkt, bis ein Klassifikator den Thread als gewöhnlich einstuft. Teammitglieder sehen vorher nichts.",
  "connectors.mailPosture.help.held":
    "Neue Nachrichten bleiben unabhängig von der Einstufung auf die Beteiligten beschränkt. Threads werden einzeln von Hand geteilt.",
  "connectors.mailPosture.help.shared":
    "Neue Nachrichten sind für Teammitglieder lesbar, sobald sie eingehen.",
  "connectors.mailPosture.historyTitle": "Auf erfasste E-Mails anwenden?",
  "connectors.mailPosture.historyBody":
    "Diese Einstellung gilt für E-Mails, die ab jetzt erfasst werden. E-Mails, die bereits im CRM sind, behalten ihre Sichtbarkeit, es sei denn, du schränkst sie entsprechend ein.",
  "connectors.mailPosture.historyConfirm": "E-Mail-Sichtbarkeit ändern",
  "connectors.mailPosture.historyApply": "Auch erfasste E-Mails einschränken",
  "connectors.disconnectTitle": "Dieses Postfach trennen?",
  "connectors.disconnectBody":
    "Dadurch werden die gespeicherten Zugangsdaten dieses Postfachs gelöscht. Die Erfassung stoppt sofort; erfasste Datensätze bleiben im CRM, und beim erneuten Verbinden wird die Berechtigung wieder angefragt.",
  "connectors.disconnectBodyGoogleNote":
    "Google führt Margince möglicherweise weiterhin unter den Drittanbieterzugriffen deines Kontos. Entferne es dort, um den Zugriff vollständig zu widerrufen.",
  "connectors.disconnectBodyMicrosoftNote":
    "Microsoft führt Margince möglicherweise weiterhin unter den verbundenen Apps deines Kontos. Entferne es dort, um den Zugriff vollständig zu widerrufen.",
  "connectors.errRateLimited":
    "Der Anbieter drosselt Anfragen. Die Erfassung ist langsamer als sonst; nichts geht verloren.",
  "connectors.errUnreachable":
    "Der Anbieter war nicht erreichbar. Es wird automatisch erneut versucht.",
  "connectors.errAuth":
    "Der Anbieter hat die gespeicherten Zugangsdaten abgelehnt. Verbinde neu, um fortzufahren.",
  "connectors.errHistoryGone":
    "Der Änderungsverlauf des Anbieters ist abgelaufen. Die nächste Synchronisierung beginnt an einem neuen Punkt.",
  "connectors.errInternal":
    "Ein interner Fehler ist aufgetreten. Die Erfassung wurde gestoppt, um unvollständige Daten zu vermeiden.",
  "connectors.errUnknown":
    "Die Erfassung ist aus einem nicht zugeordneten Grund fehlgeschlagen. Es wird automatisch erneut versucht.",

  // Das OAuth-Rückkehrergebnis (Task 2): der Callback landet auf
  // #/settings/connections/{outcome} — ein schließbarer Hinweis, gesteuert
  // von diesem Routensegment.
  "connectors.oauthOk": "Verbunden. Die Erfassung aus deinem Postfach läuft.",
  "connectors.oauthDenied":
    "Der Zugriff wurde abgelehnt, daher wurde nichts verbunden.",
  "connectors.oauthError":
    "Die Verbindung konnte nicht hergestellt werden. Versuche es erneut.",
  // Zwei Fälle, für die "erneut versuchen" falsch wäre: der Anbieter hat die
  // Freigabe abgelehnt, und die API des Anbieters ist für diese Installation
  // nicht aktiviert (das kann keine Nutzeraktion beheben).
  "connectors.oauthRejected":
    "Der Anbieter hat die Verbindung abgelehnt. Erteile alle angefragten Berechtigungen und verbinde dann erneut.",
  "connectors.oauthMisconfigured":
    "Diese Installation kann die Verbindung nicht abschließen, weil die API des Anbieters nicht aktiviert ist. Ein Admin muss sie aktivieren; das Server-Log nennt die API.",
  "connectors.oauthBadClient":
    "Der Anbieter hat die App-Zugangsdaten dieser Installation abgelehnt. Ein Admin muss Client-ID und Clientschlüssel in den Einstellungen unter Allgemein prüfen; erneutes Verbinden behebt das nicht.",
  "connectors.dismissOutcome": "Schließen",
  "connectors.oauthConnected": "Verbunden",
  "connectors.oauthNotConnected": "Nichts verbunden",
  "connectors.connectFailed": "Verbindung fehlgeschlagen",
  "connectors.imapConnectFailed": "Postfach nicht verbunden",

  // Das "Verbindung hinzufügen"-Element (Task 1): ein Button in der Kopfzeile
  // der Karte öffnet einen Dialog mit allen noch verfügbaren Anbietern, jeder
  // mit dem Satz, den er braucht.
  "connectors.addConnection": "Connector hinzufügen",
  "connectors.addOpen": "Connector hinzufügen",
  "connectors.connect": "Verbinden",
  "connectors.connectProvider": "{provider} verbinden",
  "connectors.rosterLabel": "Aktive Connectors",
  "connectors.addGmailBrings":
    "In Gmail gesendete und empfangene E-Mails. Margince kann auch darüber senden.",
  "connectors.addGcalBrings":
    "Dein Google Kalender, getrennt von Gmail verbunden.",
  "connectors.addGraphBrings":
    "Über ein Microsoft-Geschäftskonto gesendete und empfangene E-Mails. Margince kann auch darüber senden.",
  "connectors.addGraphCalBrings":
    "Dein Outlook-Kalender, getrennt von Outlook-E-Mail verbunden.",
  "connectors.addImapBrings":
    "Jeder andere Mailhost, mit einem App-Passwort. Nur Erfassung.",
  "connectors.addTestMailboxBrings":
    "Testpostfach für die Qualitätssicherung. Es werden keine echten E-Mails gesendet oder empfangen.",
  "connectors.providerNotConfigured":
    "{provider} ist in dieser Installation nicht konfiguriert.",

  // Das eingebettete IMAP-Verbindungsformular (Task 6).
  "connectors.imapModalTitle": "IMAP-Postfach verbinden",
  "connectors.imapHost": "IMAP-Server",
  "connectors.imapPort": "Port",
  "connectors.imapUsername": "E-Mail-Adresse",
  "connectors.imapSecret": "App-Passwort",
  "connectors.imapMailbox": "Postfach",
  "connectors.imapMaxMessages": "Nachrichten pro Synchronisierung",
  "connectors.imapSecretHint":
    "Verwende ein App-spezifisches Passwort. Es wird im Schlüsseltresor gespeichert und genutzt, um E-Mails nach Zeitplan zu lesen, bis du trennst; beim Trennen wird es gelöscht.",
  "connectors.imapSubmitCta": "Verbinden",
  "connectors.imapNeeded": "Pflichtfelder",
  "connectors.imapStillNeeded": "Erforderlich: {fields}",
  "connectors.imapLoginRejected":
    "Das Postfach hat diese Zugangsdaten abgelehnt. Prüfe Server, E-Mail-Adresse und App-Passwort.",
  "connectors.imapUnreachable":
    "Der Mailserver war nicht erreichbar. Prüfe Server und Port.",

  // Das Telegram-Connector-Panel (Task 17, Design §9.1-§9.2): ein Bot
  // verbindet sich für den gesamten Workspace — kein OAuth-Handshake,
  // sondern ein BotFather-Token im selben eingebetteten Formular wie beim
  // IMAP-Connector. Anders als bei den Mail-Anbietern bleibt die Verbindung
  // vor Ort bearbeitbar: das Ersetzen des Tokens läuft über PATCH, nie über
  // ein Trennen.
  "connectors.provTelegram": "Telegram",
  "connectors.telegramTitle": "Telegram-Bot",
  "connectors.telegramSub":
    "Ein Bot empfängt und sendet Nachrichten für das gesamte Unternehmen.",
  "connectors.telegramNotConfigured":
    "Nachrichtenkanäle sind in dieser Installation nicht konfiguriert.",
  "connectors.telegramConnectCta": "Telegram-Bot verbinden",
  "connectors.telegramRosterLabel": "Verbundener Bot",
  "connectors.telegramEmpty": "Kein Bot verbunden.",
  "connectors.telegramReadOnly":
    "Nur Admins und Operations können den Bot verbinden oder ändern.",
  "connectors.telegramEditToken": "Token ersetzen",
  "connectors.telegramDisconnectTitle": "Diesen Bot trennen?",
  "connectors.telegramDisconnectBody":
    "Dadurch wird das gespeicherte Token gelöscht und der Bot nicht mehr abgefragt. Erfassung und Versand stoppen sofort; erfasste Datensätze bleiben im CRM.",
  "connectors.telegramModalTitle": "Telegram-Bot verbinden",
  "connectors.telegramEditTitle": "Bot-Token ersetzen",
  "connectors.telegramBotToken": "Bot-Token",
  "connectors.telegramBotTokenHint":
    "Das Token, das BotFather für den Bot ausgestellt hat. Es wird im Schlüsseltresor gespeichert und nie wieder angezeigt.",
  "connectors.telegramSubmitCta": "Verbinden",
  "connectors.telegramReplaceCta": "Token ersetzen",
  "connectors.telegramConnectedAs": "Verbunden als @{username}.",
  "connectors.telegramConnectFailed": "Bot nicht verbunden",

  // Die Consumer-Mail-Liste des Workspace (CAP-PARAM-5).
  "consumerMail.title": "Freemail-Domains",
  "consumerMail.sub":
    "E-Mails aus einem Freemail-Postfach legen den Kontakt an, aber nie ein Unternehmen. Margince liefert eine Liste dieser Anbieter mit; ergänze fehlende Domains oder übersteuere einen falschen Eintrag.",
  "consumerMail.addedTitle": "Hier hinzugefügt",
  "consumerMail.addTitle": "Domain hinzufügen",
  "consumerMail.domainLabel": "Domain",
  "consumerMail.domainPlaceholder": "anbieter.example",
  "consumerMail.kindLabel": "Art der Domain",
  "consumerMail.kind.extra": "Freemail, nie ein Unternehmen",
  "consumerMail.kind.never":
    "Unternehmensdomain, übersteuert mitgelieferte Liste",
  "consumerMail.add": "Hinzufügen",
  "consumerMail.addOpen": "Domain hinzufügen",
  "consumerMail.remove": "Entfernen",
  "consumerMail.none":
    "Keine Domains hinzugefügt. Für jede Domain gilt die mitgelieferte Liste.",
  "consumerMail.adminOnly":
    "Du hast keine Berechtigung, diese Liste zu ändern.",
  "consumerMail.addOnly":
    "Du kannst Freemail-Domains hinzufügen. Die mitgelieferte Liste zu übersteuern oder Einträge zu entfernen, erfordert einen Admin.",
  "consumerMail.baselineTitle": "Mitgelieferte Liste",
  "consumerMail.baselineCount":
    "Margince liefert {total} bekannte Freemail-Domains mit.",
  "consumerMail.baselineSearchLabel": "Mitgelieferte Liste durchsuchen",
  "consumerMail.baselinePlaceholder": "gmail.com",
  "consumerMail.baselineNone": "Keine mitgelieferte Domain passt.",
  "consumerMail.removeFailed": "Domain nicht entfernt",
  "consumerMail.addFailed": "Domain nicht hinzugefügt",
  "consumerMail.baselineMore":
    "Die ersten {shown} von {matched} Treffern werden angezeigt.",

  "blockedDomains.title": "Abgelehnte Domains",
  "blockedDomains.sub":
    "Als Unternehmen abgelehnte Domains und was jeweils entschieden hat: ein Modellergebnis, eine Heuristik oder ein Mensch. Wird eine Domain zugelassen, ist die Frage wieder offen. Hier erscheinen nur offene Fragen ohne Zuständigkeit.",
  "blockedDomains.listTitle": "Erfasste Entscheidungen",
  "blockedDomains.record": "Entscheidung erfassen",
  "blockedDomains.recordOpen": "Entscheidung erfassen",
  "blockedDomains.domainLabel": "Domain",
  "blockedDomains.domainPlaceholder": "lieferant.example",
  "blockedDomains.admissionLabel": "Entscheidung",
  "blockedDomains.admission.suppressed": "Nie ein Unternehmen",
  "blockedDomains.admission.admitted": "Zugelassen",
  "blockedDomains.admission.undecided": "Nicht entschieden",
  "blockedDomains.reasonLabel": "Begründung",
  "blockedDomains.reasonHint":
    "Ein Satz, mit dem eine spätere Prüfung arbeiten kann.",
  "blockedDomains.reasonPlaceholder": "Lieferant, kein Kunde",
  "blockedDomains.save": "Entscheidung speichern",
  "blockedDomains.stored": "Gespeichert: {domain}, {admission}",
  "blockedDomains.saveFailed": "Entscheidung nicht gespeichert",
  "blockedDomains.adminOnly":
    "Nur Admins und Operations können Domain-Entscheidungen ändern. Für dich ist die Liste schreibgeschützt.",
  "blockedDomains.none":
    "Noch keine abgelehnten Domains. Ergebnisse zu Massenversand und manuelle Ablehnungen erscheinen hier.",
  "blockedDomains.unit": "Domain-Entscheidungen",
  "blockedDomains.openCompany": "Unternehmen öffnen",
  "blockedDomains.col.domain": "Domain",
  "blockedDomains.col.admission": "Entscheidung",
  "blockedDomains.col.source": "Entschieden von",
  "blockedDomains.col.reason": "Begründung",
  "blockedDomains.col.decided": "Zeitpunkt",
  "blockedDomains.col.revise": "Ändern",
  "blockedDomains.source.verdict": "Modellergebnis",
  "blockedDomains.source.heuristic": "Heuristik",
  "blockedDomains.source.human": "Mensch",
  "blockedDomains.source.unevidenced":
    "Kein Unternehmen auf der Website genannt",
  "blockedDomains.source.staleEvidence": "Belegende E-Mails zu alt",
  "blockedDomains.source.nearDuplicate": "Unternehmensname existiert bereits",
  "blockedDomains.rowAdmit": "Zulassen",
  "blockedDomains.rowRefuse": "Ablehnen",
  "blockedDomains.rowReopen": "Wieder öffnen",
  "blockedDomains.reopened": "{domain} wieder geöffnet",
  "blockedDomains.reopenFailed": "Domain nicht wieder geöffnet",

  "ob.s4.googleFailed": "Google-Verbindung nicht abgeschlossen",
  "ob.s4.imapHost": "IMAP-Host",
  "ob.s4.imapHostPlaceholder": "imap.gmail.com",
  "ob.s4.imapPort": "Port",
  "ob.s4.imapEmail": "E-Mail",
  "ob.s4.imapPassword": "App-Passwort", // NOSONAR: UI translation string, not a credential
  "ob.s4.imapMailbox": "Postfach",
  "ob.s4.imapMax": "Anzahl der neuesten E-Mails",
  "ob.s4.imapHint":
    "Nutze ein App-Passwort. Es wird verschlüsselt gespeichert und beim Trennen gelöscht.",
  "ob.s4.imapConnect": "Testen und verbinden",
  "ob.s4.connecting": "Wird verbunden…",
  "ob.s4.accessToggle": "Erteilter Zugriff",
  "ob.s4.scope1Lead": "Lesezugriff.",
  "ob.s4.scope1Rest":
    "E-Mails werden automatisch zu Kontakten, Unternehmen und Aktivitäten.",
  "ob.s4.scope2Lead": "Senden ist enthalten.",
  "ob.s4.scope2Rest":
    "Margince kann aus diesem Postfach senden, wenn du sendest und wenn du einem Agenten einen Passport gibst, der das Senden erlaubt. Die Vergabe dieses Passports ist die Freigabe. Du kannst ihn jederzeit entziehen.",
  "ob.s4.scope3Lead": "Die Daten bleiben in deinem Unternehmen.",
  "ob.s4.scope3Rest": "Alles jederzeit exportieren oder löschen.",
  "ob.s4.scope4Lead": "Trennen mit einem Klick.",
  "ob.s4.scope4Rest": "Das CRM funktioniert weiter, erfasst aber nichts mehr.",
  "ob.s4.capturedTitle": "Postfach verbunden",
  "ob.s4.capturedBody":
    "Neue E-Mails erscheinen hier, sobald der erste Durchlauf läuft, meist innerhalb weniger Minuten.",
  "ob.s4.connectFailed": "Postfach nicht verbunden",
  "ob.s4.notNow": "Nicht jetzt",

  "ob.conv.threadLabel": "Einrichtungsgespräch",
  "ob.conv.read.started": "Ich lese {host} und berichte, was ich finde.",
  "ob.conv.read.pages": "Bisher gelesene Seiten: {pages}.",
  "ob.conv.read.learnedField": "{field} gelernt: {value}",
  "ob.conv.read.extracting":
    "Durchsuchen abgeschlossen. Jetzt wird ausgewertet, was die Website über das Geschäft sagt.",
  "ob.conv.read.warning": "Hinweis: {warning}",
  "ob.conv.read.failed":
    "Ich konnte diese Website nicht lesen. Probiere eine andere URL oder gib die Angaben manuell ein.",
  "ob.conv.read.pollFailed":
    "Die Verbindung ist beim Lesen abgebrochen. Was ich gefunden habe, bleibt erhalten.",
  "ob.conv.read.deferred":
    "Das Lesen ist pausiert. Ich setze es automatisch fort.",
  "ob.conv.clarify.entity":
    "Die Website nennt mehr als einen Rechtsträger. Für welchen ist diese Installation gedacht?",
  "ob.conv.company.confirmed":
    "Unternehmensprofil bestätigt. Jeder gespeicherte Wert hält seine Quelle fest.",
  "ob.conv.manual.chosen": "Ich gebe es manuell ein.",
  "ob.conv.voice.skipped": "Schreibstil vorerst überspringen.",
  "ob.conv.voice.uploadAdded": "{name} hinzugefügt.",
  "ob.conv.voice.speakerQuestion":
    "Dieses Transkript hat mehrere Sprechende. Wer davon bist du? Nur deine eigenen Worte zählen.",
  "ob.conv.voice.speakerOptionDetail": "Wörter: {words} · Beiträge: {turns}",
  "ob.conv.voice.speakerFoot": "Deine Wahl gilt nur für diese Datei.",
  "ob.conv.voice.speakerContinue": "Diese Person verwenden",
  "ob.conv.voice.continueSkippedStatus":
    "Übersprungen. Später in den Einstellungen hinzufügen.",
  "ob.conv.voice.continueFailedStatus":
    "Dein Material bleibt erhalten. Versuche es jetzt erneut oder mache weiter und schließe das später ab.",
  "ob.conv.voice.continueDeferredStatus":
    "Nichts zu tun. Mache weiter; der Aufbau wird automatisch abgeschlossen.",
  "ob.conv.voice.composer": "Füge hier einen Text ein, den du geschrieben hast",
  "ob.conv.voice.dropHint":
    "Du kannst Dateien auch an jeder Stelle in diesem Gespräch ablegen: .txt, .md, .pdf, .docx oder ein Transkript als .vtt, .srt oder .json.",
  "ob.conv.voice.fileSkipped":
    "Ich kann {name} nicht lesen. Unterstützte Formate: .txt, .md, .pdf, .docx, .vtt, .srt, .json.",
  "ob.conv.voice.fileUnreadable":
    "Ich konnte {name} nicht öffnen. Ist die Datei passwortgeschützt oder beschädigt, füge stattdessen ihren Text ein.",
  "ob.conv.voice.fileEmpty":
    "{name} enthält keine Wörter, daher wurde nichts gezählt.",
  "ob.conv.voice.reactionTranscript":
    "Behaltene Wörter: {kept} von {total}. Nur deine Beiträge zählen; Gesprochenes trägt am meisten zum Schreibstil bei.",
  "ob.conv.voice.reactionDocument":
    "Gezählte Wörter: {words}. Alle Wörter in dieser Datei stammen von dir.",
  "ob.conv.voice.refusalUnattributed":
    "Das sieht nach einem Gespräch aus, aber ich kann nicht erkennen, welche Wörter von dir stammen. Deshalb wurde nichts gezählt.",
  "ob.conv.voice.refusalSpeaker":
    "Ich konnte diesen Namen im Transkript nicht finden. Es wurde nichts gezählt.",
  "ob.conv.voice.refusalUnsupported":
    "Ich konnte diese Datei weder als Text noch als Transkript lesen. Es wurde nichts gezählt.",
  "ob.conv.voice.ingestFailed":
    "Ich konnte diese Quelle nicht hinzufügen: {detail}",
  "ob.conv.voice.ingestUnexpected":
    "Ich konnte diese Quelle nicht hinzufügen. Versuche es gleich erneut.",
  "ob.conv.voice.pasteAdd": "Zu den Schreibproben hinzufügen",
  "ob.conv.voice.pasteDiscard": "Verwerfen",
  "ob.conv.voice.pasteSource": "Eingefügter Text",
  "ob.conv.voice.buildChip": "Mein Stilprofil aufbauen",
  "ob.conv.voice.retryBuild": "Aufbau erneut versuchen",
  "ob.conv.voice.buildPollFailed":
    "Die Verbindung ist während des Aufbaus abgebrochen. Deine Texte bleiben erhalten; versuche den Aufbau erneut.",
  "ob.conv.voice.statusBuilding": "Stilprofil wird aufgebaut…",
  "ob.conv.voice.resultTitle": "Das ist dein Stilprofil.",
  "ob.conv.voice.resultLoading": "Ergebnisse des Aufbaus werden geladen…",
  "ob.conv.voice.resultEmpty":
    "Der Aufbau ist abgeschlossen, hat aber noch nichts zu zeigen. Prüfe ihn in den Einstellungen.",
  "ob.conv.voice.candidateNote":
    "Diese Version muss von dir geprüft werden, bevor sie aktiv wird. Gib sie in den Einstellungen frei.",
  "ob.conv.voice.artifactTitle": "Stilkorpus",
  "ob.conv.voice.artifactBody":
    "Nur deine eigenen Wörter zählen. Alle Zahlen stammen vom Server, nachdem nach Sprechenden gefiltert wurde.",
  "ob.conv.voice.artifactEmpty":
    "Noch nichts gesammelt. Hänge ein Transkript oder einen selbst geschriebenen Text an.",
  "ob.conv.voice.meterWords": "Eigene Wörter: {words} von {target}",
  "ob.conv.voice.meterBand": "Qualität: {band}",
  "ob.conv.voice.manifestKept": "{kept} von {total} Wörtern behalten",
  "ob.conv.voice.manifestWords": "Wörter: {words}",
  "ob.conv.voice.registerMix": "Register: {mix}",
  "ob.conv.voice.stageTitle": "Fortschritt des Aufbaus",
  "ob.conv.corpus.words": "Eigene Wörter jetzt in deinem Korpus: {words}.",
  "ob.conv.corpus.band": "Korpusqualität jetzt: {band}.",
  "ob.conv.build.snapshot": "Dein Korpus wird fixiert.",
  "ob.conv.build.extract": "Deine Schreibmuster werden erfasst.",
  "ob.conv.build.evaluate":
    "Entwürfe werden mit zurückgehaltenen Proben getestet.",
  "ob.conv.build.activate": "Dein Stilprofil wird aktiviert.",
  "ob.conv.build.succeeded": "Dein Stilprofil ist fertig.",
  "ob.conv.build.deferred":
    "Der Aufbau ist eingereiht, bis KI-Kontingent verfügbar ist. Er startet automatisch.",
  "ob.conv.build.failed":
    "Der Aufbau wurde nicht abgeschlossen. Deine Texte bleiben erhalten; versuche es jederzeit erneut.",
  "ob.conv.done": "Einrichtung abgeschlossen. Dein CRM ist bereit.",
  "ob.conv.clarify.question": "{question}",
  "ob.conv.clarify.optionDetail": "{detail}",
  "ob.conv.clarify.dismiss": "Überspringen. Ich lege es selbst fest.",
  "ob.conv.clarify.keepMine": "Meinen Wert behalten",
  "ob.conv.review.skipped":
    "Du hast übersprungen: {fields}. Du kannst sie jederzeit bearbeiten.",
  "ob.conv.clarify.applyFailed":
    "Ich konnte diese Wahl nicht speichern: {detail} Wähle sie erneut aus.",
  "ob.conv.clarify.applyMissing":
    "Der Server hat diese Wahl nicht bestätigt. Wähle sie erneut aus.",
  "ob.conv.loadFailed":
    "Ich konnte deine Einrichtung nicht prüfen. Versuche es erneut.",
  "ob.conv.retry": "Erneut versuchen",
  "ob.conv.connect.persistFailed":
    "Ich konnte den Abschluss der Einrichtung nicht speichern. Versuche es erneut.",
  "ob.conv.review.title":
    "Das ist alles, was ich gefunden habe. Korrigiere, was nicht stimmt.",
  "ob.conv.review.showLess": "Weniger anzeigen",
  "ob.conv.review.continue": "Weiter",
  "ob.conv.review.progressLabel": "Ausgefüllte Pflichtfelder",
  "ob.conv.review.requiredRemaining_one":
    "Noch {count} Feld nötig, um fortzufahren",
  "ob.conv.review.requiredRemaining_other":
    "Noch {count} Felder nötig, um fortzufahren",
  "ob.conv.review.requiredDone": "Nichts weiter nötig. Du kannst fortfahren.",
  "ob.conv.review.confirmQuestionOpen":
    "Eine Entscheidung ist noch offen. Beantworte sie, um fortzufahren.",
  "ob.conv.triage.stateRequired": "erforderlich, noch leer",
  "ob.conv.triage.stateEmpty": "leer",
  "ob.conv.triage.stateTyped": "von dir eingetragen",
  "ob.conv.triage.stateStored": "aus deinem Profil",
  "ob.conv.triage.stateStoredBadge": "Aus deinem Profil",
  "ob.conv.triage.stateQuoted": "aus deinem Impressum gelesen",
  "ob.conv.triage.stateQuotedBadge": "Aus deinem Impressum gelesen",
  "ob.conv.triage.emptyHint": "Hier steht noch nichts. Trage es manuell ein.",
  "ob.conv.triage.legalNotPublished":
    "Nicht im Impressum deiner Website angegeben. Trage es manuell ein.",
  "ob.conv.triage.legalNotChecked":
    "Ich habe auf deiner Website kein Impressum gefunden. Trage es manuell ein.",
  "ob.conv.triage.legalUnpicked":
    "Dein Impressum nennt mehr als einen Rechtsträger. Wähle deinen aus, dann trage ich ihn ein.",
  "ob.conv.triage.omittedLabel": "Ausgelassen, nicht geraten",
  "ob.conv.triage.omittedField": "{field}: {reason}",
  "ob.conv.triage.mapLabel": "Zu einem Abschnitt springen",
  "ob.conv.triage.sectionBlocking": "Zum Fortfahren nötig: {count}",
  "ob.conv.triage.sectionAdvisory": "Zu prüfen: {count}",
  "ob.conv.triage.blockingHead": "Zum Fortfahren nötig",
  "ob.conv.triage.advisoryHead": "Zu prüfen",
  "ob.conv.triage.sectionSettled": "Hier ist nichts offen",
  "ob.conv.triage.sectionMore": "+{count} weitere",
  "ob.conv.triage.restTitle": "Hintergrundinformationen",
  "ob.conv.triage.looksSolid": "Sieht vollständig aus · {count}",
  "ob.conv.triage.companyWebsite": "Website",
  "ob.conv.triage.sourceCount": "Quellen: {count}",
  "ob.conv.triage.contactsLabel": "Kontakte",
  "ob.conv.triage.contactsCount": "Gefunden: {count}",
  "ob.conv.triage.contactsEmpty": "Keine Kontakte auf deiner Website gefunden.",
  "ob.conv.triage.factsLabel": "Fakten",
  "ob.conv.triage.factsCount": "Gefunden: {count}",
  "ob.rail.tokensUnit": "Tokens",
  "ob.conv.scene.step": "Schritt {n} von {m} · {label}",
  "ob.conv.scene.detour": "Entscheidung nötig",
  "ob.conv.scene.decisionSub":
    "Deine Website nennt mehrere Rechtsträger. Der ausgewählte erscheint auf jedem Angebot und jeder Rechnung.",
  "ob.conv.scene.continue": "Weiter",
  "ob.conv.connect.sceneTitle": "Deine Konten verbinden",
  "ob.conv.connect.sceneSub":
    "Ich baue deine Kontakte, Unternehmen und den Verlauf aus den E-Mails auf, die schon in deinem Postfach liegen.",
  "ob.conv.connect.mailboxTitle": "Dein Postfach",
  "ob.conv.connect.mailboxHint":
    "Wähle eins aus. Kontakte, Unternehmen und Verlauf stammen aus diesem Postfach.",
  "ob.conv.connect.networkTitle": "Dein Netzwerk",
  "ob.conv.connect.networkHint":
    "Speichere dein Profil, damit ein später importiertes Netzwerk dir zugeordnet wird. Der Import liegt in den Einstellungen.",
  "ob.conv.connect.recommended": "Empfohlen",
  "ob.conv.connect.gmailBrings": "E-Mails über Google gelesen und gesendet",
  "ob.conv.connect.microsoftBrings":
    "E-Mails über Microsoft gelesen und gesendet",
  "ob.conv.connect.imapBrings":
    "E-Mails von jedem Host, mit deiner E-Mail-Adresse und einem App-Passwort",
  "ob.conv.connect.linkedinAuth":
    "Profillink, in deinem Nutzerkonto gespeichert",
  "ob.conv.connect.saveCta": "Speichern",
  "ob.conv.connect.dialogDone": "Fertig",
  "ob.conv.connect.scopeGoogle": "OAuth, Lese- und Sendeberechtigungen",
  "ob.conv.connect.scopeMicrosoft": "OAuth, Graph API",
  "ob.conv.connect.scopeImap": "E-Mail-Adresse und Passwort",
  "ob.conv.connect.connectCta": "Verbinden",
  "ob.conv.connect.connectedCta": "Verbunden",
  "ob.conv.connect.savedCta": "Gespeichert",
  "ob.conv.connect.blockedCard":
    "Ein Postfach ist bereits ausgewählt. Trenne es in den Einstellungen, um zu wechseln.",
  "ob.conv.connect.guaranteesToggle": "Was das Verbinden bewirkt",
  "ob.conv.connect.dialogHeadlineAccess": "Zugriff auf {name} nötig",
  "ob.conv.connect.dialogHeadlineImap": "E-Mail-Host verbinden",
  "ob.conv.connect.appMissingCard":
    "Dein Unternehmen hat seine {name}-App noch nicht registriert.",
  "ob.conv.connect.appUnusableCard":
    "Die {name}-App deines Unternehmens lässt sich nicht öffnen. Ein Admin muss sie reparieren; eine neue App ist nicht nötig.",
  "ob.conv.connect.unsupportedCard":
    "Diese Installation bietet {name} nicht an.",
  "ob.conv.connect.appSetupLink": "In den Einstellungen einrichten",
  "ob.conv.connect.dialogIntro":
    "{brings}. Ich lese das Postfach einmal, um deine Kontakte und deinen Verlauf aufzubauen, und halte es danach synchron.",
  "ob.conv.connect.linkedinName": "LinkedIn",
  "ob.conv.connect.linkedinSaved": "Profil gespeichert",
  "ob.conv.connect.linkedinSkippedNote":
    "Übersprungen: später in den Einstellungen hinzufügen",
  "ob.conv.connect.rosterFailedTitle":
    "Postfächer konnten nicht geprüft werden",
  "ob.conv.connect.rosterFailedBody":
    "Der Verbindungsstatus wurde nicht geladen. Versuche es erneut, bevor du einen Anbieter auswählst.",
  "ob.conv.voice.sceneTitle": "Deinen Schreibstil trainieren",
  "ob.conv.voice.sceneSub":
    "Margince entwirft jede E-Mail in deinen eigenen Worten.",
  "ob.conv.voice.heroBody":
    "Margince lernt Ton, Rhythmus und Formulierungen nur aus deinen eigenen Texten.",
  "ob.conv.voice.whyToggle": "Warum Schreibproben hinzufügen",
  "ob.conv.voice.dropTitle": "Lege deine Texte hier ab",
  "ob.conv.voice.dropSub": "Gesendete E-Mails eignen sich am besten.",
  "ob.conv.voice.browse": "Dateien auswählen",
  "ob.conv.voice.pasteInstead": "Stattdessen Text einfügen",
  "ob.conv.voice.sourcesTitle": "Quellen",
  "ob.conv.voice.meterLabel": "Fortschritt zum Minimum von {min} Wörtern",
  "ob.conv.voice.meterProgress": "{words} von {min} Wörtern",
  "ob.conv.voice.meterReady":
    "{words} Wörter: genug für den Aufbau. Mehr Wörter verbessern ihn.",
  "ob.conv.voice.footReady":
    "Das Training dauert etwa eine Minute. Vor dem Speichern wird ein Beispiel angezeigt.",
  "ob.conv.voice.footFloor":
    "Mindestens {min} Wörter. Darunter kopiert das Modell Formulierungen.",
  "ob.conv.voice.buildingTitle": "Dein Schreibstil wird gelernt",
  "ob.conv.voice.buildingMeta_one": "{words} Wörter, {sources} Quelle",
  "ob.conv.voice.buildingMeta_other": "{words} Wörter, {sources} Quellen",
  "ob.conv.voice.resultSub":
    "Lies das Beispiel. Passt es, bestätige. Wenn nicht, füge weitere Quellen hinzu, dann baue ich neu auf.",
  "ob.conv.voice.resultSubNoSample":
    "Dieser Aufbau hat keinen Beispielentwurf geliefert. Das hat er gelernt; füge weitere Texte hinzu, dann baue ich neu auf.",
  "ob.conv.voice.resultContinue": "Das bin ich",
  "ob.conv.voice.revise": "Nicht ganz ich: weitere Texte hinzufügen",
  "ob.conv.voice.distilling": "Wird analysiert",
  "ob.conv.voice.hears": "hört",
  "ob.conv.voice.hearsWords":
    "{words} deiner eigenen Wörter, Quellen: {sources}",
  "ob.conv.voice.hearsBand": "bisher einen Korpus der Qualität {band}",
  "ob.conv.voice.hearsRegister": "{words} Wörter im Register {register}",
  "ob.conv.voice.sampleEyebrow": "Beispiel, nicht gesendet",
  "ob.conv.voice.sampleAnother": "Anderes Szenario",
  "ob.conv.voice.sampleSubjectLabel": "Betreff",
  "ob.conv.voice.sampleWhyTag": "Warum",
  "ob.conv.voice.dimensionsTitle": "Gemessene Dimensionen",
  "ob.conv.voice.dimensionsCount": "Gemessen: {count}",
  "ob.conv.voice.dimSentenceName": "Satzlänge",
  "ob.conv.voice.dimSentencePoleLow": "Knapp",
  "ob.conv.voice.dimSentencePoleHigh": "Ausführlich",
  "ob.conv.voice.dimSentenceMeasured": "Ausgewogen",
  "ob.conv.voice.dimSentenceEvidence": "Im Schnitt {count} Wörter pro Satz.",
  "ob.conv.scene.evidence": "Beleg",
  "ob.conv.scene.hideEvidence": "Beleg ausblenden",
  "ob.conv.scene.whyThis": "Was ich gelesen habe",
  "ob.conv.scene.foundOn": "Gefunden auf",
  "ob.conv.activity.steps_one": "{count} Schritt",
  "ob.conv.activity.steps_other": "{count} Schritte",
  "ob.conv.showField": "Anzeigen",
  "ob.conv.review.editDirectly": "Felder direkt bearbeiten",
  "ob.conv.review.backToDossier": "Zurück zum Dossier",
  "ob.conv.review.proposalFallback":
    "Ich konnte die vorbereitete Zuordnung nicht laden. Prüfe direkt, was ich gelesen habe; jedes Feld behält seine Quelle.",
  "ob.conv.review.confirmFailed":
    "Ich konnte das nicht speichern: {detail} Korrigiere es und übernimm erneut.",
  "ob.conv.review.confirmVersionSkew":
    "Deine Prüfung hat neuere Informationen erhalten. Prüfe sie und wähle dann erneut „Weiter“.",
  "ob.conv.review.confirmVersionSkewStuck":
    "Es hat sich noch nichts geändert, daher würde „Weiter“ erneut fehlschlagen. Prüfe erneut oder versuche es gleich erneut.",
  "ob.conv.review.refusalTitle": "„Weiter“ wurde nicht abgeschlossen",
  "ob.conv.review.confirmNotReady":
    "Dieser Lesevorgang hat noch keinen Entwurf zum Bestätigen. Prüfe erneut, wenn er abgeschlossen ist, oder starte einen neuen Lesevorgang.",
  "ob.conv.review.confirmCheckFailed":
    "Dieser Lesevorgang ist bestätigt, aber ich konnte das daraus erstellte Unternehmen nicht laden. Versuche es gleich erneut.",
  "ob.conv.artifact.empty":
    "Noch nichts gelesen. Gib eine Website ein, um diesen Bereich mit belegten Befunden zu füllen.",
  "ob.conv.results.continue": "Weiter",
  "ob.conv.recap.back": "Willkommen zurück. So steht die Einrichtung.",
  "ob.conv.recap.company": "Dein Unternehmensprofil für {name} ist bestätigt.",
  "ob.conv.recap.companyUnsaved":
    "Deine Unternehmensangaben sind noch nicht gespeichert. Vervollständige sie in den Einstellungen.",
  "ob.conv.recap.voiceBuilt":
    "Dein Stilprofil ist aufgebaut. Entwürfe verwenden deinen Schreibstil.",
  "ob.conv.recap.voiceSkipped":
    "Du hast das Stilprofil übersprungen. Entwürfe verwenden einen neutralen Ausgangsstil.",
  "ob.conv.recap.corpus":
    "Dein Korpus enthält bereits {words} deiner eigenen Wörter.",
  "ob.conv.recap.readTerminal":
    "Ich habe {host} fertig gelesen. Belegte Befunde: {count}, unten angezeigt.",
  "ob.conv.recap.readReading":
    "Ich lese {host} noch. Bisher gelesene Seiten: {pages}.",
  "ob.conv.recap.readFailed":
    "Mein früherer Lesevorgang von {host} wurde nicht abgeschlossen. Gib erneut eine Website ein oder trage die Angaben manuell ein.",
  "ob.conv.recap.readDeferred":
    "Mein Lesevorgang von {host} ist pausiert. Gib erneut eine Website ein oder trage die Angaben manuell ein.",
  "ob.conv.linkedin.cardBody":
    "Deine Profiladresse, damit eine importierte Verbindung „Anna kennt sie“ lautet, nicht „das Unternehmen kennt sie“.",
  "ob.conv.linkedin.dialogHeadline": "Dein LinkedIn-Profil",
  "ob.conv.linkedin.profileLabel": "URL deines LinkedIn-Profils",
  "ob.conv.linkedin.profilePlaceholder": "https://www.linkedin.com/in/…",
  "ob.conv.linkedin.profileWhy":
    "Ordnet das Netzwerk dir zu: „Anna kennt sie“, nicht „das Unternehmen kennt sie“.",
  "ob.conv.linkedin.save": "Profil speichern",
  "ob.conv.linkedin.skip": "LinkedIn vorerst \u00fcberspringen",
  "ob.conv.linkedin.importLater":
    "Verbindungen werden in den Einstellungen aus einem Connections.csv-Export importiert.",
  "ob.conv.linkedin.saved":
    "LinkedIn-Profil gespeichert. Dein importiertes Netzwerk wird dir zugeordnet.",
  "ob.conv.linkedin.skipped":
    "LinkedIn übersprungen. Füge dein Profil jederzeit in den Einstellungen hinzu.",
  "ob.conv.connect.skip": "Ohne Postfach fortfahren",
  "ob.conv.connect.continue": "Weiter",
  "ob.conv.connect.mailboxNeeded":
    "Ein Postfach fehlt noch, denn gelesen und entworfen werden E-Mails. Verbinde oben eins oder mache vorerst ohne weiter.",

  // Die Setup-Leiste: fünf Stationen, je ein Wort. Lang genug, den Schritt zu
  // benennen, kurz genug, dass fünf davon bei 10px in eine Spalte passen.
  "ob.rail.read": "Lesen",
  "ob.rail.confirm": "Bestätigen",
  "ob.rail.basis": "Basis",
  "ob.rail.voice": "Schreibstil",
  "ob.rail.connect": "Verbinden",

  "ob.conv.invite.title": "Wirst du selbst in Margince arbeiten?",
  "ob.conv.invite.body":
    "Das Unternehmen ist eingerichtet. Die nächsten 2 Schritte betreffen dich und gelten nur, wenn du Margince selbst nutzen wirst.",
  "ob.conv.invite.yes": "Ja, ich werde in Margince arbeiten",
  "ob.conv.invite.yesBody":
    "Trainiere deinen Schreibstil und verbinde Postfach und Kalender: 2 kurze Schritte.",
  "ob.conv.invite.no": "Nein, ich richte es nur ein",
  "ob.conv.invite.noBody":
    "Lade stattdessen das erste Mitglied ein, das hier arbeiten wird; dann ist die Einrichtung abgeschlossen.",
  "ob.conv.invite.foot":
    "Schreibstil und Konten lassen sich auch später in den Einstellungen einrichten.",
  "ob.conv.invite.continue": "Weiter",
  "ob.conv.invite.accepted": "Ja, ich werde selbst damit arbeiten.",
  "ob.conv.invite.declined": "Nein, ich richte es nur ein.",

  "ob.conv.team.title": "Das erste Mitglied einladen",
  "ob.conv.team.body":
    "Jemand muss die erste Person sein, die in Margince arbeitet. Füge sie jetzt hinzu oder später in den Einstellungen unter Personen.",
  "ob.conv.team.invitedLabel": "Bisher eingeladen",
  "ob.conv.team.invitedLine": "{name} ist eingeladen.",
  "ob.conv.team.skip": "Vorerst überspringen",
  "ob.conv.team.finish": "Einrichtung abschließen",
  "ob.conv.team.done":
    "Die Einrichtung ist abgeschlossen. Alle, die du hinzufügst, können in den Einstellungen ihren Schreibstil trainieren und ihre Konten verbinden.",
  "ob.conv.team.persistFailed":
    "Ich konnte nicht speichern, dass die Einrichtung abgeschlossen ist. Versuche es erneut oder schließe sie später in den Einstellungen ab.",
  "ob.conv.basis.title": "Berichtsbasis festlegen",
  "ob.conv.basis.body":
    "Basiswährung und Berichtszeitzone gelten für alle Deals, Berichte und Morgenberichte der Installation. Beide sind vorausgefüllt und lassen sich in den Einstellungen ändern, bis ein Deal die Währung festlegt.",
  "ob.conv.basis.reportingTitle": "Berichtsbasis",
  "ob.conv.basis.timezoneNeeded": "Eine Berichtszeitzone ist erforderlich.",
  "ob.conv.basis.continue": "Weiter",
  "ob.conv.basis.done": "Berichtsbasis festgelegt.",

  // --- das Tor: der erste Screen nach der Anmeldung ----------------------
  // Eine Frage und sonst nichts. Niemand soll das ganze Werkzeug auf dem
  // ersten Screen treffen, also nennt das Tor, was es tut, was es den Leser
  // kostet (zwei Minuten) und wer entscheidet (er selbst) — und fragt dann
  // einmal.
  "ob.gate.title": "Willkommen, {name}",
  "ob.gate.titleAnonymous": "Willkommen bei Margince",
  "ob.gate.sub":
    "Margince liest deine Website und entwirft das Unternehmensprofil. Gespeichert wird erst nach deiner Freigabe. Etwa 2 Minuten.",
  "ob.gate.trustToggle": "So funktioniert es",
  "ob.gate.trustBody":
    "Gelesen werden nur öffentliche Seiten. Gespeichert wird erst, wenn du bestätigst, und beim Lesen der Website wird an niemanden etwas gesendet.",
  "ob.gate.field": "Website-Adresse",
  "ob.gate.placeholder": "deinunternehmen.de",
  "ob.gate.submit": "Website lesen",
  "ob.gate.altPrompt": "Keine Website?",
  "ob.gate.altAction": "Angaben manuell eingeben",
  "ob.gate.invalidUrl":
    "Das ist keine gültige Webadresse. Gib sie im Format deinunternehmen.de ein.",
  // Ein String für zwei Fehler, die für den Leser gleich aussehen: die
  // Anfrage kam nie an, oder das Lesen begann und wurde nicht fertig.
  // {detail} ist die Erklärung des Servers und kann leer sein — der Satz muss
  // also auch ohne sie tragen.
  "ob.gate.startFailed":
    "Die Website konnte nicht gelesen werden. {detail} Versuche eine andere Adresse oder gib die Angaben manuell ein.",
  // Ein aufgeschobenes Lesen ist vertagt, nicht kaputt: der Server kommt darauf
  // zurück. Der Satz sagt also, was stimmt, und nennt beide Türen, ohne dass
  // der Leser irgendetwas reparieren soll.
  "ob.gate.readPaused":
    "Das Lesen ist pausiert. {detail} Es wird automatisch fortgesetzt. Du kannst auch eine andere Adresse oder die Angaben manuell eingeben.",

  // --- das Lese-Theater --------------------------------------------------
  // Sichtbar gemachtes Volumen. Die Schnittstelle liefert keinen Nenner für
  // die Seitenzahl, also ist jede Zahl hier ein offener Zähler — nie "14 von
  // 18", nie ein Balken mit bekanntem Ende, denn die Gesamtzahl zu erfinden
  // hieße, Daten zu erfinden.
  "ob.scan.title": "{host} wird gelesen",
  "ob.scan.sub":
    "Jeder Fakt behält die Seite, von der er stammt, damit sich jede Aussage prüfen lässt.",
  "ob.scan.doneTitle": "{host} gelesen",
  "ob.scan.doneSub":
    "Fakten: {facts}, Profilfelder: {fields}, jeweils mit Quellseite. Die Prüfung wird geöffnet.",
  "ob.scan.phaseCrawling": "Seiten werden abgerufen",
  "ob.scan.phaseExtracting": "Angebot des Unternehmens wird ermittelt",
  "ob.scan.phaseQueued": "Eingereiht",
  "ob.scan.phaseDeferred": "Pausiert",
  "ob.scan.pagesRead": "Gelesene Seiten: {pages}",
  "ob.scan.pagesSkipped": "Übersprungen: {count}",
  "ob.scan.stillReading": "wird noch gelesen",
  "ob.scan.pageStripLabel": "Bisher gelesene Seiten",
  "ob.scan.logLabel": "Gelesene Seiten, neueste zuerst",
  "ob.scan.pageFetched": "{url}: gelesen",
  "ob.scan.pageSkipped": "{url}: übersprungen, {reason}",
  "ob.scan.pageFailed": "{url}: nicht lesbar, {reason}",
  "ob.scan.pageNoReason": "kein Grund erfasst",
  "ob.scan.pageStatusFetched": "gelesen",
  "ob.scan.pageStatusSkipped": "übersprungen: {reason}",
  "ob.scan.pageStatusFailed": "nicht lesbar: {reason}",
  "ob.scan.skipReason.robots": "die Website erlaubt das Lesen nicht",
  "ob.scan.skipReason.offDomain": "sie liegt auf einer anderen Domain",
  "ob.scan.skipReason.pageCap":
    "das Seitenlimit für einen Lesevorgang wurde erreicht",
  "ob.scan.skipReason.byteCap":
    "das Textlimit für einen Lesevorgang wurde erreicht",
  "ob.scan.skipReason.unreadable": "die Seite konnte nicht gelesen werden",
  "ob.scan.transparency": "Transparenz",
  "ob.scan.costLine": "Aufrufe: {calls} · Tokens: {tokens} · {cost}",
  "ob.scan.costPending": "noch keine abgerechneten Modellaufrufe",
  "ob.scan.costUnpriced": " · Nutzung ohne Preisangabe vorhanden",

  // --- das Live-Panel: was der Lauf abgedeckt hat und was nicht ----------
  "ob.live.stateDone": "fertig",
  "ob.live.stateNow": "läuft",
  "ob.live.stateWaiting": "wartet",
  "ob.live.review": "Prüfen",
  "ob.live.hide": "Ausblenden",
  "ob.live.countPages": "{read} gelesen · {skipped} übersprungen",
  "ob.live.cardCoverage": "Gelesene und übersprungene Seiten",
  "ob.live.coverageWarning": "Warnung",
  "ob.live.coverageStopped": "Vorzeitig beendet",
  "ob.live.coverageCapped": "Limit erreicht",
  "ob.live.stoppedPageCap":
    "Das Seitenlimit für einen Lesevorgang wurde erreicht, daher wurden einige Seiten nicht geöffnet.",
  "ob.live.stoppedByteCap":
    "Das Größenlimit für einen Lesevorgang wurde erreicht, daher wurden einige Seiten nicht geöffnet.",
  "ob.live.stoppedBudget":
    "Das KI-Kontingent wurde erreicht, daher wurden einige Seiten nicht geöffnet.",
  "ob.live.stoppedDeadline":
    "Das Zeitlimit für einen Lesevorgang wurde erreicht, daher wurden einige Seiten nicht geöffnet.",
  "ob.live.coverageSkipped": "Übersprungen",
  "ob.live.coverageFailed": "Nicht lesbar",
  "ob.live.coverageClean":
    "Jede Seite hat geantwortet. Nichts wurde übersprungen, nichts ist fehlgeschlagen.",

  // --- Fakten: einen speichern, und die Obergrenze dafür ----------------
  "ob.facts.rowSave": "Fakt speichern: {fact}",
  "ob.facts.capReached":
    "Bis zu {max} Fakten können gespeichert werden. Wähle einen ab, um einen anderen hinzuzufügen.",

  // --- der Gegenwert: was zwei Minuten wirklich gebracht haben -----------
  // Zahlen, kein Applaus. Jede Zelle ist eine echte Zahl von der
  // Schnittstelle, und eine Zelle ohne Zahl sagt das, statt eine Null zu
  // zeigen, die wie ein Ergebnis aussieht.

  // --- die Übergabe in die App ------------------------------------------
  "ob.enter.assembling": "Unternehmensprofil wird zusammengestellt…",

  // --- das Zurücklesen des Postfachs -------------------------------------
  // Ein anderer Vorgang als das Verbinden, und der Text muss die beiden
  // getrennt halten: Verbinden erteilt Zugriff, das Zurücklesen verbraucht
  // Budget, um den Verlauf zu lesen. Es liest nur und schreibt nichts,
  // solange der Leser nicht zustimmt.
  "ob.backread.heading": "Importzeitraum",
  "ob.backread.estimating": "Nachrichten in diesem Zeitraum werden gezählt…",
  "ob.backread.estimate": "Etwa {messages} Nachrichten in diesem Zeitraum.",
  "ob.backread.estimateAtLeast":
    "Mindestens {messages} Nachrichten in diesem Zeitraum; die Zählung endete vorzeitig.",
  "ob.backread.estimateHeuristic": "Aus dem Postfach geschätzt, nicht gezählt.",
  "ob.backread.estimateCost": "Etwa {cost} für Modellaufrufe.",
  "ob.backread.estimateFailed":
    "Der Zeitraum konnte nicht geschätzt werden: {detail} Starte trotzdem oder wähle einen anderen Zeitraum.",
  "ob.backread.note":
    "Das Postfach wird nicht verändert. Importierte E-Mails und Kontakte erscheinen, während der Import läuft.",
  "ob.backread.start": "Verbinden und importieren",
  "ob.backread.startFailed":
    "Der Import des Postfachverlaufs wurde nicht gestartet: {detail} Versuche es erneut oder mache weiter und starte ihn später in den Einstellungen.",
  "ob.backread.running": "Postfachverlauf wird importiert",
  "ob.backread.runningNote":
    "Der Import läuft weiter, während du arbeitest, und setzt dort fort, wo er angehalten hat.",
  "ob.backread.queued": "Eingereiht. Startet in Kürze.",
  "ob.backread.progress": "{scanned} von etwa {total} Nachrichten",
  "ob.backread.progressNoTotal": "Nachrichten bisher: {scanned}",
  "ob.backread.tallyMessages": "gelesene Nachrichten",
  "ob.backread.tallyCaptured": "behalten",
  "ob.backread.tallySkipped": "ignoriert",
  "ob.backread.tallyContacts": "gefundene Kontakte",
  "ob.backread.tallyCompanies": "gefundene Unternehmen",
  "ob.backread.doneHeading": "Importergebnisse",
  "ob.backread.doneNote":
    "Noch ist nichts geschrieben. Alle Befunde warten in der Worklist auf Prüfung.",
  "ob.backread.failed":
    "Der Import des Postfachverlaufs wurde beendet: {detail} Die Verbindung besteht weiter; starte den Import in den Einstellungen neu.",
  "ob.backread.cancelled": "Import beendet. Es wurde nichts geschrieben.",
  "ob.backread.cancelledPartial":
    "Import beendet. Bereits erfasste Datensätze bleiben erhalten und warten in der Worklist auf Prüfung.",
  "ob.backread.cancelFailed":
    "Der Import konnte nicht beendet werden: {detail} Er läuft währenddessen weiter. Versuche es erneut.",
  "ob.backread.detailUnavailable": "Ein unerwarteter Fehler ist aufgetreten.",
  "ob.backread.cancel": "Import beenden",
  "ob.backread.explore": "Während des Imports weitermachen",
  "ob.backread.skip": "Import des Postfachverlaufs überspringen",

  "auth.title": "Margince",
  "auth.checking": "Sitzung wird geprüft…",
  "auth.pageTitle": "Anmelden · Margince",
  "auth.loginTitle": "Bei Margince anmelden",
  // "eine Admin-Contact", nicht "deine Administration": eine Administration ist
  // im Deutschen eine Stelle oder eine Tätigkeit, keine Contact — der Rest des
  // Katalogs sagt durchgehend "Admin-Contact". Und der zweite Satz nennt das Verb
  // statt des Nominalstils ("Eine Selbstregistrierung gibt es nicht").
  "auth.loginSub":
    "Nutzerkonten legt ein Admin an. Selbstregistrierung ist nicht möglich.",
  "auth.coreGreeting": "Das ist Margince.",
  "auth.corePurpose": "Es kümmert sich um die Arbeit rund um deine Arbeit.",
  // "auch ohne", nicht "weiterhin": "weiterhin" ist zeitlich und liest sich neben
  // dem Hinweis "KI nicht konfiguriert" wie "noch, aber nicht mehr lange".
  "auth.coreDevelopment": "Entwicklungs-KI",
  // Die Nachbarwerte sind alle Betriebsarten; "Modus" ist dafür das deutsche
  // Wort, "Pfad" die Übersetzung von "path".
  "auth.coreModeDevelopment": "Offline-Entwicklungsmodus",
  "auth.email": "E-Mail",
  // Der lokale Teil einer Adresse ist nie ein Pronomen — "du@" ist "you@"
  // Zeichen für Zeichen. "beispiel.de" ist im Deutschen, was "example.com" im
  // Englischen ist, und genau das pinnt die Login-Spec §7.2.
  "auth.emailPlaceholder": "name@example.com",
  "auth.password": "Passwort",
  "auth.passwordPlaceholder": "Passwort",
  "auth.passwordHint": "Mindestens 12 Zeichen",
  "auth.showPassword": "Passwort anzeigen",
  "auth.hidePassword": "Passwort ausblenden",
  "auth.capsLock": "Feststelltaste ist aktiviert",
  "auth.continueWith": "Weiter mit {brand}",
  "auth.orDivider": "oder",
  "auth.noMethodOffered":
    "Dieses Unternehmen meldet sich über einen Identitätsanbieter an, der nicht verfügbar ist. Wende dich an einen Admin, um die Einrichtung abzuschließen.",
  "auth.legalProtected": "Der Zugang zu diesem Unternehmen ist beschränkt.",
  "auth.legalTerms": "Nutzungsbedingungen",
  "auth.legalPrivacy": "Datenschutz",
  "auth.signingIn": "Wird angemeldet…",
  "auth.signIn": "Anmelden",
  "auth.failed": "Anmeldung fehlgeschlagen",
  "auth.errCredentials":
    "Anmeldung fehlgeschlagen. Prüfe E-Mail-Adresse und Passwort und versuche es erneut.",
  "auth.errRateLimited": "Zu viele Anmeldeversuche. Versuche es später erneut.",
  "auth.errUnreachable":
    "Margince war nicht erreichbar. Prüfe die Verbindung und versuche es erneut.",
  "auth.retry": "Erneut versuchen",
  "auth.noticeSignedOut": "Du wurdest abgemeldet.",
  "auth.noticeSessionExpired":
    "Die Sitzung ist abgelaufen. Melde dich erneut an, um fortzufahren.",
  "auth.noticeOidcFailed":
    "Die Anmeldung mit Google ist fehlgeschlagen. Wenn du eingeladen wurdest, öffne den Link in der Einladungs-E-Mail, um die Einrichtung deines Nutzerkontos abzuschließen.",
  "auth.connectionTitle": "Margince war nicht erreichbar",
  "auth.connectionBody":
    "Prüfe die Verbindung und versuche es erneut. Wenn das Problem bestehen bleibt, wird der Server möglicherweise gerade neu gestartet.",
  "auth.unavailableTitle": "Installation nicht bereit",
  // "Betreiber", nicht "Operator": ein Operator ist im Deutschen ein
  // mathematisches Zeichen oder eine Telefonvermittlung. Und eine Einrichtung
  // wird korrigiert, nicht repariert — repariert werden Geräte.
  "auth.unavailableBody":
    "Diese Margince-Installation ist nicht für die Anmeldung bereit. Ein Admin muss die Einrichtung abschließen oder reparieren.",
  "forcedPassword.pageTitle": "Passwort festlegen",
  "forcedPassword.title": "Eigenes Passwort festlegen",
  "forcedPassword.body":
    "Dieses Nutzerkonto nutzt noch das von einem Admin festgelegte Passwort. Lege ein Passwort fest, das nur du kennst, um fortzufahren.",
  "password.title": "Passwort",
  "password.body": "Ändere das Passwort für die Anmeldung.",
  "password.current": "Aktuelles Passwort",
  "password.next": "Neues Passwort",
  "password.confirm": "Neues Passwort bestätigen",
  "password.hint": "Mindestens 12 Zeichen",
  "password.tooShort":
    "Das Passwort ist zu kurz. Verwende mindestens 12 Zeichen.",
  "password.mismatch": "Die Passwörter stimmen nicht überein.",
  "password.changing": "Passwort wird geändert…",
  "password.open": "Passwort ändern",
  "password.cancel": "Abbrechen",
  "password.submit": "Neues Passwort speichern",
  "password.doneTitle": "Passwort geändert",
  "password.done": "Alle anderen Geräte wurden abgemeldet.",
  "password.changeFailedTitle": "Passwort nicht geändert",
  "password.errorGeneric":
    "Es wurde keine Ursache gemeldet. Versuche es erneut.",
  "setup.pageTitle": "Margince einrichten",
  "setup.title": "Diese Installation übernehmen",
  "setup.body":
    "Diese Installation hat noch kein Unternehmen. Der Admin hat ein einmaliges Einrichtungs-Token aus der Token-Datei, die der Server beim ersten Start geschrieben hat.",
  "setup.token": "Einrichtungs-Token",
  "setup.tokenHint":
    "Aus der Token-Datei, die beim ersten Start geschrieben wurde. Das Serverprotokoll nennt ihren Pfad und enthält das Token, falls die Datei nicht geschrieben werden konnte.",
  "setup.company": "Unternehmensname",
  "setup.baseCurrency": "Basiswährung",
  "setup.baseCurrencyHint":
    "Alle Beträge werden in diese Währung umgerechnet. Ändern lässt sie sich in den Einstellungen nur, bis der erste Betrag umgerechnet ist.",
  "setup.baseCurrencyMalformed":
    "Gib einen Währungscode aus 3 Buchstaben ein, etwa EUR, CHF oder USD.",
  "setup.baseLanguage": "Basissprache",
  "setup.baseLanguageHint":
    "Die Sprache, in der die KI schreibt, wenn das ganze Team den Text liest. Jede Person wählt ihre eigene Anzeigesprache, und Antworten an Kontakte folgen der Sprache des Threads.",
  "setup.timezone": "Berichtszeitzone",
  "setup.timezoneHint":
    "IANA-Zeitzonenname. Alle Berichtszeiträume werden darin berechnet. Aus diesem Browser ermittelt; ändere ihn, wenn das Team anderswo arbeitet.",
  "setup.adminName": "Dein Name",
  "setup.adminEmail": "Deine E-Mail-Adresse",
  "setup.adminPassword": "Passwort",
  "setup.passwordHint": "Mindestens 12 Zeichen",
  "setup.passwordShort":
    "Das Passwort ist zu kurz. Verwende mindestens 12 Zeichen.",
  "setup.rootWarning":
    "Damit wird das Adminkonto für die gesamte Installation angelegt, mit allen Berechtigungen, einschließlich der Verwaltung aller anderen Nutzerkonten.",
  "setup.claim": "Unternehmen anlegen",
  "setup.claiming": "Wird angelegt…",
  "setup.errorToken":
    "Dieses Einrichtungs-Token gilt nicht für diese Installation. Prüfe die Token-Datei, die das Serverprotokoll beim ersten Start nennt.",
  "setup.errorAlready":
    "Diese Installation hat bereits ein Unternehmen. Melde dich an oder lass es vom Admin zurücksetzen.",
  "setup.errorFields":
    "Einige Felder sind ungültig. Korrigiere sie und versuche es erneut.",
  "setup.errorServer":
    "Die Einrichtung wurde nicht abgeschlossen, es wurde nichts angelegt. Versuche es gleich erneut und prüfe das Serverprotokoll, falls es wieder fehlschlägt.",
  "setup.errorNetwork":
    "Margince war nicht erreichbar. Prüfe die Verbindung und versuche es erneut.",
  "auth.forgotLink": "Passwort vergessen?",
  "auth.forgotTitle": "Passwort zurücksetzen",
  // "gibt", nicht "existiert" (Amtsdeutsch), und das Feld will eine Adresse, keine
  // E-Mail — die E-Mail ist die Nachricht. Was unterwegs ist, ist ebenfalls die
  // Nachricht, nicht der Link. Die Existenz des Kontos bleibt offen.
  "auth.forgotSub":
    "Gib deine E-Mail-Adresse ein. Wenn es dazu ein Nutzerkonto gibt, wird ein Link zum Zurücksetzen an diese Adresse gesendet.",
  "auth.sendResetLink": "Link zum Zurücksetzen senden",
  "auth.forgotSentTitle": "Prüfe dein Postfach",
  "auth.forgotSentBody":
    "Wenn es zu dieser Adresse ein Nutzerkonto gibt, wurde ein Link zum Zurücksetzen gesendet. Er läuft nach 1 Stunde ab.",
  "auth.resetTitle": "Neues Passwort wählen",
  "auth.resetSub": "Der Link ist gültig. Gib ein neues Passwort ein.",
  "auth.newPassword": "Neues Passwort",
  "auth.setNewPassword": "Neues Passwort festlegen",
  // "bereits verwendet", nicht "verbraucht": ein Link wird verwendet, nicht
  // verbraucht wie Kraftstoff.
  "auth.resetFailed":
    "Dieser Link zum Zurücksetzen ist ungültig, wurde bereits verwendet oder ist abgelaufen.",
  // "nicht akzeptiert": abgelehnt werden Anträge und Angebote, nicht Passwörter.
  // Ein anderes zu wählen IST der neue Versuch, also entfällt der Nachsatz.
  "auth.resetRejectedPassword":
    "Das Passwort wurde abgelehnt. Wähle ein anderes Passwort.",
  // "speichern", nicht "setzen": gesetzt wird eine Variable, und "speichern" ist
  // genau das Verb, das auf dem Button darunter steht. Drei Sätze statt eines
  // Komma-Spleißes zwischen Aussage und Aufforderung.
  "auth.resetServerFailed":
    "Das Passwort wurde nicht festgelegt. Der Link ist weiterhin gültig; versuche es gleich erneut.",
  // Nicht "setze … erneut": das liest sich als "zurücksetzen", und dieser Schritt
  // liegt hinter dem Zurücksetzen.
  "auth.resetRateLimited":
    "Zu viele Versuche. Lege das Passwort später erneut fest.",
  "auth.requestNewLink": "Neuen Link anfordern",
  "auth.askAdminForNewLink":
    "Frage einen Admin nach einem neuen Link zum Festlegen des Passworts.",
  // "geändert", wie im Satz darunter: aktualisiert werden Daten, die veralten.
  "auth.resetDoneTitle": "Passwort aktualisiert",
  // "beendet", nicht "abgemeldet": abmelden tut sich eine Contact, eine Sitzung
  // wird beendet.
  "auth.resetDoneBody":
    "Das Passwort wurde geändert, und alle anderen Sitzungen wurden abgemeldet. Melde dich mit dem neuen Passwort an.",
  "auth.backToLogin": "Zurück zur Anmeldung",
  "auth.signOut": "Abmelden",

  "client.back": "Zurück zu Margince",
  "client.title": "Margince neben deinem Postfach",
  "client.sender": "Absender",
  "client.lookup": "Nachschlagen",
  "client.open360": "360-Ansicht öffnen",
  "client.unknown": "Noch nicht in deinem Unternehmen.",
  "client.unknownDetail":
    "Dieser Absender passt zu keinem Kontakt, den du sehen kannst. Von woanders wurde nichts geholt.",
  "client.createLead": "Als Lead erfassen",
  "client.isolation": "Nur mit deinem Unternehmen verbunden",
  "client.attribution": "Jede Erfassung ist zugeordnet und prüfbar.",

  "book.title": "Termin buchen",
  "book.min15": "15 min",
  "book.min30": "30 min",
  "book.min60": "60 min",
  "book.attendee": "E-Mail des Gasts",
  "book.welcomeBack": "Erkannt: {name}",
  "book.subject": "Termin über Margince",
  "book.confirmed": "Termin gebucht",
  "book.tellThemYourself":
    "Margince versendet keine Einladung. Teile der eingeladenen Person den Termin direkt mit.",
  "book.failed": "Buchung fehlgeschlagen. Es wurde nichts eingetragen.",
  "book.name": "Name",
  "book.email": "E-Mail-Adresse",
  "book.consentWording":
    "Ich bin einverstanden, dass mein Name und meine E-Mail-Adresse gespeichert werden, um diesen Termin zu vereinbaren und im Anschluss daran nachzufassen.",

  "prefs.title": "Wählen Sie, welche E-Mails Sie erhalten",
  "prefs.sub":
    "Jeder Zweck steht für sich. Transaktionale Nachrichten lassen sich hier nicht ausschalten, weil Sie sie benötigen; alle anderen Zwecke bestimmen Sie selbst.",
  "prefs.unsub.title": "Diese E-Mails nicht mehr erhalten?",
  "prefs.unsub.lead":
    "Ein Klick stoppt Nachrichten dieser Art an Ihre Adresse. Sonst ändert sich nichts.",
  "prefs.unsub.loading": "E-Mail-Einstellungen werden geöffnet…",
  "prefs.unsub.afterTitle": "Was als Nächstes passiert",
  "prefs.unsub.afterBody":
    "E-Mails dieser Art werden Ihnen nicht mehr gesendet. Sicherheits- und Servicenachrichten, die Sie für einen von Ihnen angeforderten Vorgang benötigen, sind davon nicht betroffen.",
  "prefs.unsub.confirm": "Diese E-Mails abbestellen",
  "prefs.unsub.busy": "Ihre Auswahl wird gespeichert…",
  "prefs.unsub.seeAll": "Alle Einstellungen ansehen",
  "prefs.unsub.privacy":
    "Keine Anmeldung nötig. Dieser persönliche Link steuert nur Ihre E-Mail-Einstellungen. Geben Sie ihn nicht weiter.",
  "prefs.unsub.doneTitle": "Abbestellt",
  "prefs.unsub.doneBody":
    "Sie erhalten {label} von diesem Absender nicht mehr. Die Änderung gilt sofort.",
  "prefs.unsub.manage": "Einstellungen verwalten",
  "prefs.unsub.alreadyOff":
    "Diese E-Mails waren bereits ausgeschaltet. Es hat sich nichts geändert.",
  "prefs.unsub.lockedTitle": "Diese Nachrichten lassen sich nicht ausschalten",
  "prefs.unsub.lockedBody":
    "Diese Nachrichten werden für einen Vorgang benötigt, den Sie angefordert haben, zum Beispiel das Zurücksetzen eines Passworts oder eine Bestätigung.",
  "prefs.unsub.retry": "Erneut versuchen",
  "prefs.unsub.unknownPurposeTitle": "Dieser Link passt zu keiner E-Mail-Art",
  "prefs.unsub.unknownPurpose":
    "Öffnen Sie Ihre Einstellungen, um alle Arten gesendeter E-Mails zu sehen.",
  "prefs.unsub.deadLinkTitle": "Dieser Link ist nicht mehr gültig",
  "prefs.unsub.deadLinkBody":
    "Einstellungslinks laufen ab und können widerrufen werden. Fordern Sie über eine aktuelle E-Mail einen neuen Link an.",
  "prefs.unsub.errorTitle": "Einstellungen konnten nicht geöffnet werden",
  "prefs.unsub.failedTitle": "E-Mails nicht abbestellt",
  "prefs.purpose.business_correspondence": "Direkte Korrespondenz",
  "prefs.purpose.marketing_email": "Produktneuigkeiten",
  "prefs.purpose.transactional": "Sicherheits- und Servicenachrichten",
  "prefs.sentVia": "Gesendet über Margince",
  "prefs.noObjection": "An: Sie haben dem nicht widersprochen",
  "prefs.optedOut": "Aus: Sie haben gebeten, diese nicht mehr zu erhalten",
  "prefs.invalidLink":
    "Dieser Link ist nicht mehr gültig. Einstellungslinks laufen ab und können widerrufen werden. Fordern Sie über eine aktuelle E-Mail einen neuen Link an.",
  "buyer.opening": "Ihr Deal Room wird geöffnet…",
  "buyer.deadTitle": "Dieser Link funktioniert nicht mehr",
  "buyer.deadAskContact":
    "Fragen Sie Ihre Ansprechperson nach einem neuen Link.",
  "buyer.linkDead":
    "Der Link wurde bereits verwendet, ist abgelaufen oder wurde durch einen neueren ersetzt. Fordern Sie unten einen neuen Link an.",
  "buyer.noLink":
    "Öffnen Sie diese Seite über den Link, den Sie erhalten haben. Falls Sie ihn nicht mehr haben, fordern Sie unten einen neuen an.",
  "buyer.emailLabel": "Ihre E-Mail-Adresse",
  "buyer.emailHint": "Die Adresse, an die die Einladung gesendet wurde.",
  "buyer.requestLink": "Neuen Link anfordern",
  "buyer.linkRequestedTitle": "Prüfen Sie Ihr Postfach",
  "buyer.linkRequested":
    "Falls diese Adresse eingeladen wurde, ist ein neuer Link unterwegs.",
  "buyer.pausedTitle": "Zugang pausiert",
  "buyer.pausedBody":
    "{steward} hat diesen Raum pausiert. Ihr Link bleibt gültig, und Sie können weitermachen, sobald der Zugang fortgesetzt wird.",
  "buyer.expiredTitle": "Zugang beendet",
  "buyer.expiredBody":
    "Der Zugang zu diesem Raum ist abgelaufen. Wenden Sie sich an {steward}, oder fordern Sie unten einen neuen Link an.",
  "buyer.eyebrow": "Deal Room",
  "buyer.contact": "Ihre Ansprechperson: {steward}.",
  "buyer.closed":
    "Dieser Raum ist geschlossen. Seine Inhalte bleiben als Aufzeichnung erhalten.",
  "buyer.previewBannerTitle": "Du siehst eine Vorschau dieses Raums",
  "buyer.previewBanner":
    "Das ist die Ansicht der Käuferseite. Du kannst alles lesen, aber nichts ändern.",
  "buyer.previewReadOnly":
    "Eine Vorschau ist schreibgeschützt. Schließen Sie diesen Tab, um zur Seite des Deal Rooms zurückzukehren.",
  "buyer.closedNote": "Dieser Raum ist jetzt schreibgeschützt.",
  "buyer.stewardUnknown": "Ihre Ansprechperson",
  "buyer.signOut": "Abmelden",
  "buyer.signedInAs": "Angemeldet als {name}.",
  "buyer.contactEyebrow": "Ihre Ansprechperson",
  "buyer.contactBody":
    "Stellen Sie Ihre Frage unter dem betreffenden Dokument; sie geht direkt an {steward}.",
  "buyer.closedOn": "Geschlossen am {date}",
  "room.docs.title": "Dokumente",
  "room.docs.empty": "Noch keine Dokumente im Raum.",
  "room.docs.fileLabel": "Datei aus diesem Deal",
  "room.docs.fileHint":
    "Jede Datei im Dateibereich des Deals kann hinzugefügt werden, auch hochgeladene Dateien und E-Mail-Anhänge.",
  "room.docs.pickFile": "Datei wählen",
  "room.docs.noFiles": "Keine Dateien an diesem Deal",
  "room.docs.groupLabel": "Gruppe",
  "room.docs.add": "Zum Raum hinzufügen",
  "room.docs.upload": "Datei hochladen",
  "room.docs.remove": "{title} aus dem Raum entfernen",
  "room.docs.group.commercial": "Kaufmännisches",
  "room.docs.group.legal": "Rechtliches",
  "room.docs.group.security_privacy": "Sicherheit und Datenschutz",
  "room.docs.group.delivery_operations": "Umsetzung und Betrieb",
  "buyer.docs.title": "Dokumente",
  "buyer.docs.empty": "Noch keine Dokumente.",
  "buyer.docs.download": "{title} herunterladen",
  "buyer.docs.downloadFailed":
    "Der Download wurde nicht gestartet. Versuchen Sie es erneut, oder fragen Sie Ihre Ansprechperson.",
  "buyer.docs.downloadShort": "Herunterladen",
  "buyer.poweredBy": "Betrieben mit",
  "buyer.poweredByMargince": "Betrieben mit Margince",
  "threads.roomTitle": "Threads im Raum",
  "threads.aboutThis_other": "{count} Threads zu diesem Dokument",
  "threads.aboutThis_one": "{count} Thread zu diesem Dokument",
  "threads.askAbout": "Frage zu diesem Dokument stellen",
  "threads.read": "Lesen",
  "threads.readTitle": "{title} lesen",
  "threads.unanswered_one": "{count} unbeantwortet",
  "threads.unanswered_other": "{count} unbeantwortet",
  "threads.cancel": "Abbrechen",
  "threads.empty": "Noch keine Nachrichten.",
  "threads.requiredChange": "Änderung erforderlich",
  "threads.resolved": "Erledigt",
  "threads.sideBuyer": "Käuferseite",
  "threads.sideSeller": "Anbieterseite",
  "threads.replyLabel": "Antwort",
  "threads.reply": "Antworten",
  "threads.resolve": "Als erledigt markieren",
  "threads.newLabel": "Neuer Thread",
  "threads.requireChangeLabel": "Dieses Dokument muss geändert werden",
  "threads.open": "Absenden",
  "threads.readOnly": "Nur Lesezugriff.",
  "deal360.blocker": "Blocker",
  "deal360.buyer": "Was die Käuferseite will",
  "deal360.verdict.live": "Aktiv",
  "deal360.verdict.drifting": "Stockt",
  "deal360.verdict.blocked": "Blockiert",
  "deal360.verdict.cold": "Kalt",
  "dealmail.title": "E-Mail",
  "dealmail.reply": "Antwort entwerfen",
  "dealmail.send": "E-Mail senden",
  "recordmail.title": "E-Mail",
  "recordmail.reply": "Antwort entwerfen",
  "recordmail.send": "E-Mail schreiben",
  "deal360.rewrite": "Neu erzeugen",
  "deal360.readFull": "Vollständigen Bericht lesen",
  "deal360.openTask": "Aufgabe öffnen",
  "deal360.createTask": "Aufgabe hinzufügen",
  "deal360.openBrief": "Terminbericht öffnen",
  "deal360.unreadable":
    "Dieser Bericht wurde nicht geladen. Lade die Seite neu oder erzeuge ihn neu.",
  "prefs.rateLimited":
    "Zu viele Versuche von diesem Gerät. Warten Sie eine Minute und laden Sie die Seite neu.",
  "prefs.subscribed": "An: Sie haben diese angefordert",
  "prefs.alwaysOn": "Immer an",
  "confirm.title": "Ihre Angaben",
  "confirm.intro":
    "Margince, ein KI-System, führt dieses CRM. Unten steht alles, was über Sie erfasst ist. Sie können jede Angabe korrigieren oder ihre Löschung beantragen.",
  "confirm.card.title": "Erfasste Angaben",
  "confirm.field.fullName": "Name",
  "confirm.field.title": "Position",
  "confirm.field.email": "E-Mail",
  "confirm.field.phone": "Telefon",
  "confirm.field.company": "Unternehmen",
  "confirm.field.none": "Nicht erfasst",
  "confirm.marketing.title": "Gelegentlich Neuigkeiten erhalten?",
  "confirm.marketing.ask":
    "Neuigkeiten von Zeit zu Zeit, etwa einmal im Monat. Ihre Entscheidung wird respektiert.",
  "confirm.marketing.yes": "Neuigkeiten abonnieren",
  "confirm.marketing.no": "Keine Neuigkeiten senden",
  "confirm.provenance.title": "Herkunft Ihrer Angaben",
  "confirm.provenance.empty": "Für diese Angaben ist keine Quelle erfasst.",
  "confirm.provenance.line": "{field}: aus {source}, erfasst am {date}",
  "confirm.erasure.ask": "Löschung beantragen",
  "confirm.erasure.staged":
    "Löschung angefordert. Bestätigen Sie unten, um die Anfrage zu senden.",
  "confirm.submit": "Angaben bestätigen",
  "confirm.subscription.title": "Bestätigen Sie Ihr Abonnement",
  "confirm.subscription.ask":
    "Bestätigen Sie, dass Sie {purpose} erhalten möchten.",
  "confirm.subscription.confirm": "Abonnement bestätigen",
  "confirm.subscription.alreadyTitle": "Abonnement aktiv",
  "confirm.subscription.alreadyBody":
    "Ihr Abonnement für {purpose} ist bestätigt. Sie können es jederzeit über jede an Sie gesendete E-Mail beenden.",
  "confirm.done.title": "Vielen Dank",
  "confirm.receipt.title": "So geht es weiter",
  "confirm.receipt.body":
    "Geben Sie diese Referenz bei jeder Frage zu Ihrer Anfrage an. Eine Antwort erfolgt innerhalb eines Monats.",
  "confirm.receipt.rectify": "Korrektur angefordert",
  "confirm.receipt.erasure": "Löschung angefordert",
  "confirm.done.body":
    "Ihre Antwort ist erfasst. Änderungen gehen zur Übernahme an eine Person aus dem Team, und dieser Link ist jetzt verbraucht.",
  "confirm.invalidLink":
    "Dieser Link ist nicht mehr gültig. Er wurde möglicherweise schon benutzt oder ist abgelaufen.",
  "prefs.lockedWhy":
    "Wird für einen von Ihnen angeforderten Vorgang benötigt und bleibt deshalb an.",
  "prefs.confirmationNeededWhy":
    "Um dies einzuschalten, nutzen Sie den Bestätigungslink in der E-Mail, die Ihnen gesendet wurde. Ausschalten können Sie es hier jederzeit.",
  "prefs.notSaved": "Noch nicht gespeichert.",
  "prefs.savePending": "Ausstehend: {changes}.",
  "prefs.saveProof":
    "Der genaue Wortlaut, den Sie gesehen haben, und ein Zeitstempel werden als Nachweis gespeichert. Die Auswahl gilt dann für jede künftige E-Mail.",
  "prefs.save": "Einstellungen speichern",
  "prefs.discard": "Verwerfen",
  "prefs.cannotGrant":
    "Für diesen Datensatz lässt sich Folgendes nicht starten: {purposes}. Wenn Sie das für falsch halten, antworten Sie auf eine beliebige E-Mail dieses Absenders, um es prüfen zu lassen.",
  "prefs.cannotGrantWhy":
    "Das lässt sich für diesen Datensatz nicht einschalten.",
  "prefs.choiceNotApplied":
    "Eine Ihrer Auswahlen wurde nicht wirksam. Ihre aktuellen Einstellungen werden oben angezeigt.",
  "prefs.confirmationSent":
    "Prüfen Sie Ihr Postfach und klicken Sie auf den Link, um {purposes} zu bestätigen. Das Abonnement beginnt erst, wenn Sie bestätigen.",
  "prefs.confirmationUnavailable":
    "Die Bestätigungs-E-Mail für {purposes} konnte nicht gesendet werden, daher hat das Abonnement nicht begonnen. Versuchen Sie es später erneut.",
  "prefs.partialSave":
    "Das Speichern wurde unterbrochen. Einige Ihrer Auswahlen wurden möglicherweise gespeichert; Ihre aktuellen Einstellungen wurden neu geladen, damit Sie genau sehen, was gilt.",
  "prefs.wording.business_correspondence":
    "„Senden Sie mir Antworten und direkte Nachrichten zu unseren Gesprächen.“",
  "prefs.wording.transactional":
    "„Senden Sie mir, was ich für einen von mir angeforderten Vorgang benötige.“",
  "prefs.wordingGeneric": "„Senden Sie mir {label}.“",
  "prefs.wording.marketing_email":
    "„Senden Sie mir Produkt-Updates und gelegentlich Marketing-E-Mails.“",
  "prefs.wording.events":
    "„Senden Sie mir Einladungen zu Events und Webinaren.“",
  "prefs.unsubscribeAll": "Alles Marketing abbestellen",
  "prefs.unsubscribeAllHint":
    "Schaltet alle Marketingzwecke oben aus. Antworten auf Ihre eigenen Anfragen und alles, was Sie angefordert haben, gehen weiter, weil niemand Sie dafür angemeldet hat.",
  "prefs.oneClickDone":
    "Sie haben die Marketing-E-Mails dieses Absenders abbestellt. Das gilt sofort für jede Kampagne.",
  "prefs.oneClickAlreadyOff":
    "Diese waren bereits ausgeschaltet. Es hat sich nichts geändert.",
  "prefs.undo": "Rückgängig machen und Marketing weiter erhalten",
  "prefs.undoExplicit":
    "Ein erneutes Abonnieren ist eine ausdrückliche Einwilligung (Opt-in) und wird nie automatisch wieder eingeschaltet. Speichern Sie unten, um Ihre Einwilligung festzuhalten, oder verwerfen Sie die Änderung.",

  "auto.tier.runs": "Läuft",
  "auto.tier.approval": "Freigabe",
  "auto.sub":
    "Eine Regel mit „Läuft“ handelt selbstständig. Eine Regel mit „Freigabe“ schickt ihre Aktionen an die Freigaben.",
  "auto.readOnly":
    "Nur Lesezugriff: Du hast keine Berechtigung, Automatisierungen zu ändern.",
  "auto.catalog": "Vorlagenbibliothek",
  "auto.catalogSub": "Verfügbare Automatisierungstypen",
  "auto.instances": "Eingerichtete Automatisierungen",
  "auto.use": "Vorlage verwenden",
  "auto.name": "Name",
  "auto.create": "Anlegen",
  "auto.createdPaused":
    "Pausiert angelegt. Nichts läuft, bis sie aktiviert ist.",
  "auto.delete": "Löschen",
  "auto.statusEnabled": "Aktiv",
  "auto.statusPaused": "Pausiert",
  "auto.dateField.placeholder": "Datumsfeld auswählen",
  "auto.dateField.needsObject":
    "Wähle zuerst ein Objekt, um seine Datumsfelder anzuzeigen.",
  "auto.dateField.empty": "Dieses Objekt hat noch keine aktiven Datumsfelder.",
  "auto.dateField.loadError":
    "Datumsfelder wurden nicht geladen. Versuche es erneut.",
  "auto.enabledFor": "{name} ist aktiv",
  "auto.rowActions": "Aktionen für {name}",
  "auto.withheld":
    "Eingerichtete Automatisierungen sind für deine Rolle ausgeblendet.",
  "auto.deleteTitle": "Diese Automatisierung löschen?",
  "auto.deleteBody":
    "„{name}“ und ihre Einstellungen werden endgültig gelöscht. Um die Automatisierung anzuhalten, ohne die Regel zu verlieren, schalte sie stattdessen aus.",

  "auto.runs.open": "Läufe",
  "auto.runs.title": "Bisherige Läufe",
  "auto.runs.filterAll": "Alle",
  "auto.runs.filterFired": "Ausgelöst",
  "auto.runs.filterFailed": "Fehlgeschlagen",
  "auto.runs.filterBlocked": "Blockiert",
  "auto.runs.filterSkipped": "Übersprungen",
  "auto.runs.filterQueued": "Zur Freigabe eingereiht",
  "auto.runs.empty": "Diese Automatisierung wurde noch nicht ausgelöst.",
  "auto.runs.emptyFiltered": "Keine Läufe mit diesem Ergebnis.",
  "auto.runs.needsApproval": "Freigabe erforderlich",
  "auto.runs.why": "Auslöser",
  "auto.runs.target": "Ziel",
  "auto.runs.result": "Ergebnis",
  "auto.runs.reason": "Grund",
  "auto.runs.outcomeFired": "Ausgelöst",
  "auto.runs.outcomeFailed": "Fehlgeschlagen",
  "auto.runs.outcomeBlocked": "Blockiert",
  "auto.runs.outcomeSkipped": "Übersprungen",
  "auto.runs.outcomeQueued": "Eingereiht",

  "auto.preview.open": "Vorschau",
  "auto.preview.title": "Auswirkung des Probelaufs",
  "auto.preview.window": "Zeitfenster",
  "auto.preview.window7": "7 Tage",
  "auto.preview.window30": "30 Tage",
  "auto.preview.window90": "90 Tage",
  "auto.preview.matchesNow": "Aktuelle Treffer: {n}",
  "auto.preview.wouldFire": "Würde auslösen: etwa {n} in {days} Tagen",
  "auto.preview.notComputable": "Rückblickende Schätzung nicht verfügbar",
  "auto.preview.hidden": "Ausgeblendet (kein Zugriff): {n}",
  "auto.preview.explainer":
    "Probelauf ohne Schreibzugriff. Es werden keine Datensätze geändert und nichts gesendet.",

  "strength.title": "Beziehungsstärke",
  "strength.score": "Score {score} von 100",
  "strength.bucket.none": "Ruhend",
  "strength.bucket.weak": "Schwach",
  "strength.bucket.moderate": "Warm",
  "strength.bucket.strong": "Stark",
  "strength.factor.recency": "Aktualität",
  "strength.factor.frequency": "Häufigkeit",
  "strength.factor.reciprocity": "Gegenseitigkeit",
  "strength.factor.direction": "Richtung",
  "strength.lastInteraction": "Letzte Interaktion: {when}",
  "strength.none": "Noch keine Interaktionen",
  "strength.inout": "{in} eingehend · {out} ausgehend (90 Tage)",
  "strength.computedFrom": "Berechnet aus Aktivitäten: {count}",

  // Die Abdeckungskarte des Beziehungsgraphen (ADR-0078).
  "coverage.engaged": "Im Austausch",
  "coverage.quiet": "Kein beidseitiger Kontakt",
  "coverage.seatWithheld": "Ein Kontakt, den du nicht lesen kannst",
  "coverage.daysSinceTouch": "{days} Tage",
  "coverage.risk.single_threaded_theirs": "Nur ein Kontakt",
  "coverage.risk.single_threaded_ours": "Von einer Person im Team betreut",
  "coverage.risk.coverage_gap": "Kein engagierter Champion",
  "coverage.risk.champion_left": "Champion ist ausgeschieden",
  "coverage.risk.stakeholder_left": "Stakeholder ist ausgeschieden",
  "coverage.risk.going_cold": "Kühlt ab",

  "cf.title": "Eigene Felder",
  "cf.formSection": "Eigene Felder",
  "cf.subtitle":
    "Füge einem vorhandenen Objekt zur Laufzeit ein typisiertes Feld hinzu, ohne Code und ohne neues Release. Neue Objekte und Beziehungen erfordern Codeänderungen.",
  "cf.object": "Objekt",
  "cf.obj.deal": "Deal",
  "cf.obj.company": "Unternehmen",
  "cf.obj.contact": "Kontakt",
  "cf.obj.lead": "Lead",
  "cf.obj.project": "Projekt",
  "cf.obj.contract": "Vertrag",
  "cf.listLabel": "Felder für {object}",
  "cf.col.field": "Feld",
  "cf.col.type": "Typ",
  "cf.col.addedBy": "Hinzugefügt von",
  "cf.addedByYou": "Du",
  "cf.addedByAdmin": "Admin",
  "cf.empty.deal":
    "Noch keine eigenen Felder für Deals. Füge eines hinzu, um Daten zu erfassen, die die Kernfelder nicht abdecken.",
  "cf.empty.company":
    "Noch keine eigenen Felder für Unternehmen. Füge eines hinzu, um Daten zu erfassen, die die Kernfelder nicht abdecken.",
  "cf.empty.contact":
    "Noch keine eigenen Felder für Kontakte. Die Kernfelder decken den Kontaktdatensatz ab. Füge eines hinzu, um mehr zu erfassen.",
  "cf.empty.lead":
    "Noch keine eigenen Felder für Leads. Ein hier hinzugefügtes Feld erscheint auch, nachdem ein Lead als Kontakt qualifiziert wurde.",
  "cf.empty.project":
    "Noch keine eigenen Felder für Projekte. Füge eines hinzu, um Daten zur Umsetzung zu erfassen, die die Kernfelder nicht abdecken.",
  "cf.empty.contract":
    "Noch keine eigenen Felder für Verträge. Füge eines für eine Einordnung hinzu, die die Standardkonditionen nicht abdecken.",
  "cf.type.text": "Text",
  "cf.type.number": "Zahl",
  "cf.type.date": "Datum",
  "cf.type.currency": "Währung",
  "cf.type.multiselect": "Mehrfachauswahl",
  "cf.type.picklist": "Auswahlliste",
  "cf.type.boolean": "Ja/Nein",
  "cf.builder.addTo": "Feld zu {object} hinzufügen",
  "cf.builder.open": "Neues Feld",
  "cf.builder.noCode": "Ohne Code",
  "cf.builder.intro":
    "Ein neues Feld ist eine echte Spalte in der bestehenden Tabelle. Es funktioniert in Filtern, Berichten, Exporten und der API wie jedes Kernfeld. Es ist kein neues Objekt.",
  "cf.label": "Bezeichnung",
  "cf.apiKey": "API-Name",
  "cf.apiKeyHint":
    "Wird automatisch abgeleitet und ist ab dem Livegang fest. Das Präfix cf_ verhindert Konflikte mit Kernfeldern.",
  "cf.typeLabel": "Typ",
  "cf.currencyCode": "Währungscode",
  "cf.currencyHint":
    "Dreistelliger ISO-4217-Code, zum Beispiel EUR oder USD. Beträge werden auf den Cent genau gespeichert.",
  "cf.options": "Optionen",
  "cf.addOption": "Option hinzufügen",
  "cf.removeOption": "Option entfernen",
  "cf.optionPlaceholder": "Bezeichnung der Option",
  "cf.lastOptionBlocked": "Eine Auswahlliste braucht mindestens eine Option",
  "cf.gate.title": "Dieses Feld hinzufügen?",
  "cf.gate.body":
    "Nach der Bestätigung wird es zu einer aktiven Spalte auf jedem {object}: auf der 360-Seite, in Suche und Filtern, Listen, Export und der API. Das Hinzufügen wird im Audit-Log erfasst.",
  "cf.refuse.title":
    "Das sieht nach einem neuen Objekt oder einer neuen Beziehung aus",
  "cf.refuse.body":
    "Dieser Builder fügt bestehenden Datensätzen nur einfache Felder hinzu. Ein neues Objekt, eine Verknüpfung zwischen Objekten oder ein berechneter Roll-up ist eine strukturelle Änderung. Das kommt als geprüfte Änderung in einer neuen Margince-Version, umgesetzt von Menschen und nicht vom Produkt, das seinen eigenen Code bearbeitet.",
  "cf.refuse.route":
    "Leite es über die Entwicklung: dein eigenes Entwicklungsteam, einen Implementierungspartner oder Margince Services.",
  "cf.confirm": "Feld hinzufügen",
  "cf.writing": "Wird gespeichert…",
  "cf.added":
    "Feld „{label}“ hinzugefügt. Es erscheint in Datensätzen, Filtern, Exporten und der API.",
  "cf.edit": "Bezeichnung bearbeiten",
  "cf.archive": "Feld archivieren",
  "cf.archived":
    "„{label}“ archiviert. Das Feld ist bei neuen Datensätzen ausgeblendet, bleibt in Audit-Log und Verlauf erhalten und lässt sich wiederherstellen.",
  "cf.renamePrompt": "Neue Bezeichnung",
  "cf.renamed": "Umbenannt in „{label}“",
  "cf.audit.title": "Letzte Feldänderungen",
  "cf.audit.empty": "Noch keine Änderungen an eigenen Feldern.",
  "cf.audit.footer":
    "Jedes Hinzufügen, Bearbeiten und Archivieren wird dauerhaft im Audit-Log erfasst.",
  "cf.noPermission": "Du hast nur Lesezugriff auf eigene Felder.",
  "cf.retired": "Stillgelegt",
  // "Allgemein" statt "Firma" für den ersten Eintrag: die Gruppen-
  // überschrift darüber sagt das Wort schon, und eine Zeile, die ihre eigene
  // Überschrift wiederholt, benennt nichts.
  "settings.home": "\u00dcbersicht",
  "settings.home.yours": "Deine Einstellungen",
  "settings.home.manage": "Bearbeitbare Einstellungen",
  "settings.home.lookUp": "Schreibgeschützte Einstellungen",
  "settings.home.rolesLabel": "Deine Rolle",
  "settings.home.seatLabel": "Dein Platz",
  "settings.home.seat.full": "Voller Platz: Änderungen erlaubt",
  "settings.home.seat.read": "Leseplatz: nur ansehen",
  "settings.home.reachLabel": "Sichtbare Datensätze",
  "settings.home.reach.own": "Deine eigenen Datensätze",
  "settings.home.reach.team": "Datensätze deines Teams",
  "settings.home.reach.all": "Alle Datensätze des Unternehmens",
  "settings.home.access": "Dein Zugriff",
  "settings.boundary.deniedTitle": "Kein Zugriff auf diese Einstellungsseite",
  "settings.boundary.deniedBody":
    "Diese Seite existiert, aber deine Rolle kann sie nicht öffnen. Kopiere die Adresse, um jemanden mit Zugriff zu fragen.",
  "settings.boundary.unknownTitle": "Einstellungsseite nicht gefunden",
  "settings.boundary.unknownBody":
    "Der Link ist möglicherweise veraltet oder falsch geschrieben. Die Einstellungsübersicht listet jede Seite auf, die deine Rolle öffnen kann.",
  "settings.page.account.sub": "Wie du angezeigt wirst und dich anmeldest.",
  "settings.page.voice.sub":
    "Die Formulierungen, die Entwürfe verwenden, wenn sie in deinem Namen schreiben.",
  "settings.page.agents.sub":
    "Was ein Agent unbeaufsichtigt tun darf und welche Clients deine Zugangsdaten halten.",
  "settings.page.connections.sub":
    "Postfächer und Adressen, die für diesen Platz gelesen werden.",
  "settings.page.capture-activity.sub":
    "Was die Erfassung mit deinen E-Mails gemacht hat, und warum.",
  "settings.page.company.sub":
    "Unternehmensname, Währung und Geschäftskontext für alle Datensätze.",
  "settings.page.authentication.sub":
    "Wie sich Personen an dieser Installation anmelden und welche Apps für diese Installation handeln dürfen.",
  "settings.page.members.sub":
    "Alle, die einen Platz haben, und worauf sie jeweils zugreifen können.",
  "settings.page.teams.sub":
    "Teammitgliedschaft, die den teambezogenen Zugriff auf Datensätze bestimmt.",
  "settings.page.seats.sub":
    "Belegte Plätze im Verhältnis zum Lizenzumfang dieser Installation.",
  "settings.page.stageautomation.sub":
    "Die bisherige Bilanz jedes Phasenübergangs, bevor er Deals automatisch verschiebt.",
  "settings.page.pipelines.sub":
    "Phasen, die ein Deal durchläuft, für das ganze Unternehmen.",
  "settings.page.acquisition.sub":
    "Geschäftskanäle, denen ein Deal zugeordnet werden kann.",
  "settings.page.recordroles.sub":
    "Wofür ein Teammitglied oder ein Team bei einem Datensatz zuständig sein kann. Gewährt keinen Zugriff.",
  "settings.page.leads.sub": "Begriffe für Lead-Quellen in diesem Unternehmen.",
  "settings.page.fields.sub":
    "Eigene Felder, die dieses Unternehmen zu den Standardfeldern hinzufügt.",
  "settings.page.tags.sub":
    "Gemeinsame Tags. Wird ein Tag hier umbenannt, ändert sich sein Name auf jedem Datensatz.",
  "settings.page.products.sub":
    "Produkte, die dieses Unternehmen verkauft, und Angebotsvorlagen.",
  "settings.page.capture.sub":
    "Welche E-Mails zu einem Datensatz werden und welche ignoriert werden.",
  "settings.page.integrations.sub":
    "Systeme, mit denen diese Installation Datensätze austauscht.",
  "settings.page.knowledge.sub":
    "Dokumente, die Entwürfe und Antworten verwenden dürfen.",
  "settings.page.import.sub":
    "Datensätze aus einer Datei importieren, ein Lauf nach dem anderen.",
  "settings.page.models.sub":
    "Welcher Anbieter welche Art von Arbeit übernimmt und ob er verfügbar ist.",
  "settings.page.automations.sub": "Regeln, die automatisch laufen.",
  "settings.page.usage.sub":
    "KI-Kosten dieses Monats im Verhältnis zum Kontingent.",
  "settings.page.model-calls.sub": "Jede Modellanfrage und ihre Antwort.",
  "settings.page.privacy.sub":
    "Betroffenenanfragen, Einwilligungszwecke und Aufbewahrung von Datensätzen.",
  "settings.page.audit.sub":
    "Alle Handelnden und jeder Datensatz, auf den sie zugegriffen haben.",
  "settings.page.system-health.sub":
    "Ob die Hintergrundverarbeitung Schritt hält.",
  "settings.page.extensions.sub":
    "Erweiterungen in diesem Build und welche Rollen darauf zugreifen können.",
  "settings.page.reset.sub":
    "Daten dieser Installation löschen. Kann nicht rückgängig gemacht werden.",
  "settings.scope.self": "Nur du",
  "settings.scope.mixed": "Gemischt",
  "settings.scope.workspace": "Unternehmen",
  "settings.scope.installation": "Installation",
  "settings.scopeAria": "Gilt für: {scope}",
  "settings.scopeAriaMixed":
    "Einstellungen auf dieser Seite betreffen unterschiedliche Personen; jede Einstellung nennt, wen.",
  "settings.readOnlyPageTitle": "Schreibgeschützte Einstellungen",
  "settings.readOnlyPage": "Deine Rolle darf diese Einstellungen nicht ändern.",
  "settings.saveFailed": "Änderung nicht gespeichert",
  "settings.mintFailed": "Passport nicht erstellt",
  "settings.search.label": "Einstellungen durchsuchen",
  "settings.search.placeholder": "Einstellungen durchsuchen",
  "settings.search.none": "Keine passende Einstellungsseite.",
  "settings.search.count": "Passende Einstellungsseiten: {n}",
  "settings.tab.company": "Unternehmensprofil",
  "settings.tab.authentication": "Anmeldung und Apps",
  "settings.tab.members": "Mitglieder",
  "settings.tab.teams": "Teams",
  "settings.tab.seats": "Plätze und Lizenz",
  "settings.tab.stageautomation": "Phasenautomatisierung",
  "settings.tab.pipelines": "Pipelines",
  "settings.tab.acquisition": "Akquisequellen",
  "settings.tab.recordroles": "Verantwortungsrollen",
  "settings.tab.leads": "Lead-Bearbeitung",
  "settings.tab.fields": "Felder",
  "settings.tab.tags": "Tags",
  "settings.tab.products": "Produkte und Angebote",
  "settings.tab.import": "Datenimport",
  "settings.tab.models": "Modelle und Routing",
  "settings.tab.automations": "Automatisierungen",
  "settings.tab.usage": "KI-Nutzung",
  "settings.tab.model-calls": "Modellaufrufe",
  "settings.tab.audit": "Audit-Log",
  "settings.tab.system-health": "Systemzustand",
  "settings.tab.reset": "Daten zurücksetzen",
  "settings.group.me": "Du",
  "settings.group.company": "Unternehmen",
  "settings.group.people": "Personen",
  "settings.group.sales": "Vertrieb",
  "settings.group.data": "Daten",
  "settings.group.ai": "KI",
  "settings.group.governance": "Governance",
  "settings.tab.account": "Nutzerkonto",
  "settings.tab.voice": "Schreibstil",
  "settings.tab.agents": "Agenten",
  "settings.tab.connections": "Verbindungen",
  "settings.tab.general": "Allgemein",
  "settings.tab.users": "Nutzende und Teams",
  "settings.tab.extensions": "Erweiterungen",
  "settings.tab.integrations": "Anbindungen",
  "settings.tab.capture": "Erfassungsregeln",
  "settings.tab.data-model": "Datenmodell",
  "settings.tab.ai": "KI",
  "settings.tab.knowledge": "Wissen",
  "corpusAsk.title": "Deine Dokumente befragen",
  "corpusAsk.sub":
    "Frage in eigenen Worten. Antworten stammen nur aus einer Dokumentensammlung, Fragen außerhalb der Sammlung werden abgelehnt, und jeder Satz zitiert seine Textstelle.",
  "corpusAsk.whichSet": "Dokumentensammlung",
  "corpusAsk.question": "Deine Frage",
  "corpusAsk.submit": "Fragen",
  "corpusAsk.byModel": "Von Margince aus deinen Dokumenten verfasst",
  "corpusAsk.byPassages":
    "Nur Textstellen aus den Quellen. Es wurde keine Antwort verfasst.",
  "corpusAsk.notReady":
    "Diese Sammlung wird noch gelesen: {embedded} von {total} Textstellen sind durchsuchbar. Versuche es gleich erneut. An der Frage liegt es nicht.",
  "corpusAsk.retrievalUnavailable":
    "Es wurde nichts durchsucht. In dieser Installation ist kein Suchindex eingerichtet.",
  "corpusAsk.unreviewed":
    "Diese Textstellen passen am besten zu deiner Frage. Ob sie die Frage beantworten, wurde nicht geprüft.",
  "corpusAsk.failed": "Frage nicht beantwortet",
  "corpusAsk.unreviewedTitle": "Textstellen nicht geprüft",
  "corpusAsk.notReadyTitle": "Diese Sammlung wird noch gelesen",
  "corpusAsk.retrievalUnavailableTitle": "Kein Suchindex eingerichtet",
  "corpusAsk.noGrantTitle":
    "Du kannst die Dokumente dieses Unternehmens nicht öffnen",
  "corpusAsk.noGrant":
    "Du hast keinen Zugriff auf diese Dokumentensammlung. Admins können dir Zugriff gewähren.",
  "corpusAsk.noSetsTitle": "Keine Dokumentensammlungen",
  "corpusAsk.noSets":
    "Dieses Unternehmen hat noch keine Dokumente, daher gibt es nichts zu durchsuchen.",
  "corpusAsk.citeAtLine": "{number}: {document}, Zeile {line}",
  "corpusAsk.citeInDocument": "{number}: in {document}",
  "corpusAsk.documentLoading": "Dokument wird geladen…",
  "corpusAsk.documentFailedTitle": "Dokument konnte nicht geöffnet werden",
  "corpusAsk.documentFailed":
    "{document} konnte nicht geladen werden. Die zitierte Textstelle steht unten.",
  "corpusAsk.openFile": "Datei öffnen",
  "corpusAsk.quoteNotPinpointed":
    "Dieses Zitat geht über einen Zeilenumbruch und ließ sich nicht genau markieren. Das Dokument ist an seiner Quelle geöffnet.",
  "corpusAsk.notCovered.title": "Nicht von dieser Sammlung abgedeckt",
  "corpusAsk.notCovered.body":
    "{name} wurde vollständig durchsucht und enthält nichts, das nah genug ist, um die Frage zu beantworten. Die Sammlung deckt ab:",
  "knowledge.title": "Dokumentensammlungen",
  "knowledge.sub":
    "Dokumentensammlungen, die dieses Unternehmen befragen kann. Antworten stützen sich nur auf abgelegte Dokumente, und Fragen, die diese nicht abdecken, werden abgelehnt.",
  "knowledge.withheld":
    "Du hast keinen Zugriff auf die Liste der Dokumentensammlungen.",
  "knowledge.coverage":
    "{documents} Dokumente · {embedded} von {total} Textstellen durchsuchbar",
  "knowledge.reindexingTitle": "Sammlung wird neu indexiert",
  "knowledge.reindexing":
    "Eine Änderung an der Textindexierung hat die Neuindexierung gestartet. Bis sie abgeschlossen ist, melden Fragen „nicht bereit“. Es sind keine Daten verloren gegangen.",
  "knowledge.showDocuments": "Dokumente anzeigen",
  "knowledge.hideDocuments": "Dokumente ausblenden",
  "knowledge.documents": "Dokumente",
  "knowledge.noDocuments": "Noch keine Dokumente.",
  "knowledge.archive": "Sammlung archivieren",
  "knowledge.archiveFailed": "Sammlung nicht archiviert",
  "knowledge.archiveConfirm.title": "Diese Dokumentensammlung archivieren?",
  "knowledge.archiveConfirm.body":
    "Die Sammlung und ihre Dokumente sind nicht mehr durchsuchbar. Es wird nichts gelöscht.",
  "knowledge.deleteDocument": "Löschen",
  "knowledge.deleteFailed":
    "Das Dokument wurde nicht gelöscht. Versuche es erneut.",
  "knowledge.deleteConfirm.title": "Dieses Dokument löschen?",
  "knowledge.deleteConfirm.body":
    "Die Datei, ihr extrahierter Text und ihr Suchindex werden endgültig gelöscht.",
  "knowledge.ingest.queued": "Eingereiht",
  "knowledge.ingest.running": "Wird indexiert…",
  "knowledge.ingest.done": "Durchsuchbar",
  "knowledge.ingest.failed": "Konnte nicht gelesen werden",
  "knowledge.ingestDetailTitle":
    "Warum diese Datei nicht gelesen werden konnte",
  "knowledge.upload.label": "Dokument hinzufügen",
  "knowledge.upload.hint":
    "Reiner Text, Markdown, CSV oder JSON. PDF- und Word-Dateien werden nicht unterstützt und abgelehnt.",
  "knowledge.upload.empty": "Textdatei hier ablegen oder auswählen",
  "knowledge.upload.submit_other": "{count} Dokumente hinzufügen",
  "knowledge.upload.refusedTitle_one": "1 Datei wurde nicht hinzugefügt",
  "knowledge.upload.refusedTitle_other":
    "{count} Dateien wurden nicht hinzugefügt",
  "knowledge.upload.refused": "{filename}: {message}",
  "knowledge.upload.submit_one": "Dokument hinzufügen",
  "knowledge.new.title": "Neue Dokumentensammlung",
  "knowledge.new.name": "Name",
  "knowledge.new.topic": "Was diese Sammlung abdeckt",
  "knowledge.new.topicHint":
    "Ein Satz. Er wird allen gezeigt, deren Frage diese Sammlung nicht abdeckt.",
  "knowledge.new.submit": "Sammlung anlegen",
  "knowledge.new.failed":
    "Die Sammlung wurde nicht angelegt. Versuche es erneut.",
  "settings.tab.privacy": "Datenschutz und Aufbewahrung",
  "settings.tab.capture-activity": "Erfassungsaktivität",
  "verdictPass.subject.senders": "Absender",
  "verdictPass.subject.threads": "Threads",
  "verdictPass.every_one": "{subject} werden jede Minute geprüft.",
  "verdictPass.every_other": "{subject} werden alle {minutes} Minuten geprüft.",
  "verdictPass.next": "Nächste Prüfung am {when}.",
  "verdictPass.queued": "Eine Prüfung ist fällig und wartet auf den Start.",
  "verdictPass.running": "Eine Prüfung läuft gerade.",
  "captureActivity.title": "Erfassungsaktivität",
  "captureActivity.sub":
    "Was deine E-Mails der letzten 24 Stunden ergeben haben. Ausgeschlossene Absender stehen oben.",
  "captureActivity.scope.label": "Wessen Aktivität",
  "captureActivity.outcomes": "Ergebnisse",
  "captureActivity.messages": "Nachrichten",
  "captureActivity.scope.mine": "Meine",
  "captureActivity.scope.workspace": "Geteilte Kanäle",
  "captureActivity.scopeNote":
    "Gezählt ab dem Zeitpunkt, an dem ein Connector eine Nachricht an dieses CRM übergibt. Was ein Connector auf seiner eigenen Seite gefiltert hat, etwa eine Chat-Reaktion oder eine E-Mail-Regel, ist nicht enthalten. Umfasst nur Nachrichten. Die Lead-Erfassung wird hier nicht angezeigt.",
  "captureActivity.filtered":
    "Angezeigt: {shown} von {total} mit dem Ergebnis „{outcome}“ in diesem Zeitraum.",
  "captureActivity.openTrace":
    "Alle Verarbeitungsschritte dieser Nachricht anzeigen",
  "captureActivity.emptyFiltered":
    "Keine geladene Zeile passt. Lade mehr, um den Rest des Zeitraums zu sehen.",
  "captureActivity.loadMore": "Mehr laden",
  "captureActivity.empty":
    "Keine Erfassungsaktivität in den letzten 24 Stunden.",
  "captureActivity.payloadsOff":
    "Diese Installation speichert weder Absender noch Betreff von Nachrichten. Die Zeilen unten zeigen daher nur das Ergebnis.",
  "captureActivity.contentNone": "Kein Absender erfasst",
  "captureActivity.outcome.captured": "Erfasst",
  "captureActivity.outcome.internal": "Als intern verworfen",
  "captureActivity.outcome.suppressed": "Kein Kontakt angelegt",
  "captureActivity.outcome.deferred": "Wartet auf Absenderprüfung",
  "captureActivity.outcome.fault": "Ableitung fehlgeschlagen",
  "captureActivity.funnel.captured": "Erfasst",
  "captureActivity.funnel.internal": "Intern",
  "captureActivity.funnel.suppressed": "Kein Kontakt",
  "captureActivity.funnel.deferred": "Wartet auf Prüfung",
  "captureActivity.funnel.fault": "Fehlgeschlagen",
  "captureActivity.reason.internal_only":
    "alle Beteiligten waren auf den Domains deines Unternehmens",
  "captureActivity.reason.deferral_capped":
    "das Limit für offene Fragen war erreicht, daher kommt kein Ergebnis",
  "captureActivity.reason.noise_prior":
    "eine frühere Prüfung hat diesen Absender als Rauschen eingestuft, daher wird die Nachricht archiviert",
  "captureActivity.reason.decided_prior":
    "über diesen Absender wurde bereits entschieden, daher wird kein Kontakt angelegt",
  "captureActivity.reason.no_granting_human":
    "die Verbindung nennt kein Nutzerkonto, in dessen Namen gehandelt werden kann",
  "captureActivity.reason.invisible_incumbent":
    "sie passte zu einem Datensatz, den du nicht sehen kannst",
  "captureActivity.reason.derivation_failed":
    "der Kontaktschritt ist fehlgeschlagen, die Nachricht selbst ist nicht betroffen",
  "captureActivity.reason.no_counterparty":
    "kein Absender, der erfasst werden kann",
  "captureActivity.reason.role_mailbox":
    "ein Sammelpostfach, keine Person: gespeichert, aber kein Kontakt angelegt",
  "captureActivity.reason.private_thread":
    "ein privater Thread: für dich gespeichert, aber kein Kontakt angelegt",
  "captureActivity.reason.transactional_infra":
    "der Absender ist E-Mail-Infrastruktur, kein Unternehmen, mit dem du arbeitest",
  "captureActivity.reason.transactional_prefix":
    "der Absender wirkt wie ein automatischer Versand, nicht wie eine Person",
  "captureActivity.outcome.deferred_capped": "Nicht eingereiht",
  "captureActivity.outcome.deferred_sent": "Zur Absenderprüfung gesendet",
  "captureActivity.resolution.pending": "wartet noch",
  "captureActivity.resolution.unsure": "zur Prüfung weitergeleitet",
  "captureActivity.resolution.real": "als echte Person eingestuft",
  "captureActivity.resolution.noise": "als Rauschen eingestuft",
  "captureActivity.resolution.rejected": "von einer Person abgelehnt",
  "captureActivity.resolution.suppressed": "unterdrückt",
  "pipeline.title": "Wie diese Nachricht verarbeitet wurde",
  "pipeline.sub":
    "Jeder Erfassungsschritt, in der Reihenfolge, in der diese Nachricht ihn durchlaufen hat.",
  "pipeline.payloadsOff":
    "Zu keinem Schritt sind Absender oder Betreff gespeichert: Die Inhaltserfassung ist auf dieser Installation ausgeschaltet.",
  "pipeline.transport": "Empfangen über",
  "pipeline.unavailable":
    "Die Erfassungsschritte für diese Nachricht wurden nicht geladen. Versuche es erneut.",
  "pipeline.status.done": "Erledigt",
  "pipeline.status.skipped": "Übersprungen",
  "pipeline.status.pending": "Wartet",
  "pipeline.status.failed": "Fehlgeschlagen",
  "pipeline.status.not_applicable": "Nicht zutreffend",
  "pipeline.status.unknown": "Unbekannt",
  "pipeline.reason.record_not_available":
    "der Datensatz dieses Schritts wird nicht mehr aufbewahrt oder ist für dich nicht sichtbar; nach dem Löschen eines Datensatzes lässt sich beides nicht unterscheiden",
  "pipeline.status.not_reported": "Hier nicht ausgewiesen",
  "pipeline.subject.message": "zu dieser Nachricht",
  "pipeline.subject.sender": "zum Absender, nicht nur zu dieser Nachricht",
  "pipeline.subject.domain": "zur Domain des Absenders",
  "pipeline.subject.thread": "zum gesamten Thread",
  "pipeline.stage.connector_filter": "Connector-Filter",
  "pipeline.stage.ingress_gate": "Eingangsprüfung",
  "pipeline.stage.erasure_check": "Löschprüfung",
  "pipeline.stage.internal_drop": "Prüfung auf rein interne Nachrichten",
  "pipeline.stage.activity_write": "Im Verlauf gespeichert",
  "pipeline.stage.tier_ladder": "Kontaktentscheidung",
  "pipeline.stage.contact_create": "Kontakt angelegt",
  "pipeline.stage.verdict": "Absenderprüfung",
  "pipeline.stage.company_triage": "Unternehmensprüfung",
  "pipeline.stage.attention_label": "Aufmerksamkeitslabel",
  "pipeline.stage.material_events": "Thread-Analyse",
  "pipeline.stage.claim_extraction": "Zusagen und offene Punkte",
  "pipeline.reason.internal_only":
    "alle Beteiligten waren auf den Domains deines Unternehmens",
  "pipeline.reason.invisible_incumbent":
    "sie passte zu einem Datensatz, den du nicht sehen kannst",
  "pipeline.reason.transactional_infra":
    "der Absender ist E-Mail-Infrastruktur, kein Unternehmen, mit dem du arbeitest",
  "pipeline.reason.transactional_prefix":
    "der Absender wirkt wie ein automatischer Versand, nicht wie eine Person",
  "pipeline.reason.deferral_capped":
    "das Limit für offene Fragen war erreicht, daher kommt kein Ergebnis",
  "pipeline.reason.noise_prior":
    "eine frühere Prüfung hat diesen Absender als Rauschen eingestuft, daher wird die Nachricht archiviert",
  "pipeline.reason.decided_prior":
    "über diesen Absender wurde bereits entschieden, daher wird kein Kontakt angelegt",
  "pipeline.reason.no_counterparty": "kein Absender, der erfasst werden kann",
  "pipeline.reason.role_mailbox":
    "ein Sammelpostfach, keine Person: gespeichert, aber kein Kontakt angelegt",
  "pipeline.reason.private_thread":
    "ein privater Thread: für dich gespeichert, aber kein Kontakt angelegt",
  "pipeline.reason.no_granting_human":
    "die Verbindung nennt kein Nutzerkonto, in dessen Namen gehandelt werden kann",
  "pipeline.reason.derivation_failed":
    "der Kontaktschritt ist fehlgeschlagen; die Nachricht selbst ist nicht betroffen",
  "pipeline.reason.not_linked_yet":
    "mit dieser Nachricht ist noch kein Kontakt verknüpft",
  "pipeline.reason.no_contact_intended":
    "die Kontaktentscheidung ergab, dass keiner anzulegen war",
  "pipeline.reason.awaiting_verdict": "die Absenderprüfung steht noch aus",
  "pipeline.reason.judged_real":
    "dieser Absender wurde als echte Person eingestuft",
  "pipeline.reason.judged_noise":
    "dieser Absender wurde als Rauschen eingestuft, daher wurde kein Datensatz angelegt",
  "pipeline.reason.judged_rejected":
    "dieser Absender wurde abgelehnt, daher wurde kein Datensatz angelegt",
  "pipeline.reason.judged_suppressed":
    "dieser Absender wurde unterdrückt, daher wurde kein Datensatz angelegt",
  "pipeline.reason.no_open_question":
    "zu diesem Absender gab es keine offene Frage",
  "pipeline.reason.thread_not_captured":
    "in diesem Thread ist keine erfasste Nachricht mehr vorhanden, es gab also nichts zu lesen",
  "pipeline.reason.events_raised":
    "dieser Thread wurde gelesen, und seine Ereignisse wurden beim Unternehmen abgelegt",
  "pipeline.reason.nothing_material":
    "dieser Thread wurde gelesen und enthielt nichts, was sich abzulegen lohnt",
  "pipeline.reason.thread_still_moving":
    "dieser Thread ist noch aktiv; er wird gelesen, sobald er eine Weile ruhig war",
  "pipeline.reason.awaiting_scan":
    "dieser Thread ist zum Lesen fällig und wurde noch nicht erreicht",
  "pipeline.reason.reading_parked":
    "das Lesen dieses Threads wurde mehrmals verweigert, daher ist es pausiert, bis sich der Thread ändert oder die Pause abläuft",
  "pipeline.reason.no_single_account":
    "dieser Thread lässt sich nicht genau einem Unternehmen zuordnen, daher haben seine Befunde kein sicheres Ziel",
  "pipeline.reason.two_bodies_of_work":
    "dieser Thread umfasst 2 Projekte, daher wären seine Befunde für eines davon falsch",
  "pipeline.reason.thread_not_all_open":
    "eine Nachricht in diesem Thread ist für einen Teil seiner Lesenden verborgen, daher würde eine Zusammenfassung des Ganzen eine Teilansicht als vollständig darstellen",
  "pipeline.reason.answered_on_the_company":
    "dieser Schritt betrifft die Domain des Absenders und wird daher einmal beim Unternehmen beantwortet, nicht bei jeder Nachricht",
  "pipeline.reason.company_warranted":
    "für diese Domain wurde ein Unternehmensdatensatz als gerechtfertigt eingestuft, und dies ist er",
  "pipeline.reason.no_site_identified":
    "nichts auf der Website dieser Domain wies auf ein Unternehmen hin, daher wurde stattdessen der Name des Absenders verwendet",
  "pipeline.reason.triage_queued": "diese Domain wartet auf Prüfung",
  "pipeline.reason.triage_unevidenced":
    "nichts, was bisher von dieser Domain vorliegt, deutet auf ein Unternehmen hin",
  "pipeline.reason.triage_stale_evidence":
    "was von dieser Domain vorliegt, ist zu alt für eine Entscheidung",
  "pipeline.reason.triage_near_duplicate":
    "diese Domain ähnelt einer bereits erfassten, daher wird sie zurückgehalten statt doppelt angelegt",
  "companyTriage.title": "Herkunft des Unternehmens",
  "companyTriage.sub":
    "Geprüfte E-Mail-Domains und das Ergebnis jeder Prüfung.",
  "companyTriage.empty":
    "Für dieses Unternehmen wurde keine E-Mail-Domain geprüft. Es wurde manuell angelegt oder importiert.",
  "companyTriage.checkedAt": "geprüft {when}",
  "pipeline.reason.no_named_reader":
    "dieser Thread hat keine Lesenden, an die sich ein Befund richten ließe",
  "pipeline.reason.transport_not_read":
    "dieser Schritt liest nur E-Mails, und die Nachricht kam über einen anderen Kanal",
  "pipeline.reason.sender_undecided":
    "die Absenderprüfung steht noch aus, daher wird die Nachricht zurückgehalten",
  "pipeline.reason.archived": "die Nachricht ist archiviert",
  "pipeline.reason.not_connector_captured":
    "die Nachricht wurde nicht von einem Connector erfasst",
  "pipeline.reason.awaiting_batch":
    "sie ist berechtigt und wartet auf den nächsten Durchlauf",
  "pipeline.reason.labelled": "die Nachricht wurde mit einem Label versehen",
  "pipeline.reason.not_comparable":
    "Filterung auf Connector-Seite wird hier nicht gezählt; die Zahlen bedeuten je Connector etwas anderes",
  "pipeline.reason.connector_side_defect":
    "Eingangsfehler sind ein Fehler der Verbindung, nicht einer einzelnen Nachricht",
  "pipeline.reason.would_restore_erased":
    "dies auszuweisen würde Daten wiederherstellen, die eine Löschung entfernt hat",
  "pipeline.reason.no_writer_yet": "diesen Schritt gibt es noch nicht",
  "settings.tab.maintenance": "Wartung",
  "settings.tab.license": "Lizenz",
  "license.card.title": "Lizenz und Plätze",
  "license.state.licensed": "Lizenziert",
  "license.state.uncapped": "Lizenziert, ohne Platzbegrenzung",
  "license.state.unlicensed": "Keine Lizenz konfiguriert",
  "license.state.refused": "Lizenz abgelehnt",
  "license.absent.title": "Diese Installation hat keine Lizenz",
  "license.absent.body":
    "Alles funktioniert weiter, und nichts ist begrenzt. Konfiguriere ein Lizenz-Token für diese Installation, damit Plätze gegen eine gewährte Anzahl gezählt werden.",
  "license.refused.title": "Die Lizenz dieser Installation wurde abgelehnt",
  "license.refused.body":
    "Das für diese Installation konfigurierte Lizenz-Token wurde vorgelegt und abgelehnt. Alles funktioniert weiter, ohne Begrenzung, bis es ersetzt wird. Prüfe das Token und die Uhr der Installation.",
  "license.seats.capacityOnly":
    "Volle Plätze, die diese Installation nutzt. Der Lizenzumfang ist für diese Rolle nicht sichtbar.",
  "license.seats.title": "Plätze",
  "license.seats.uncapped": "Keine Begrenzung",
  "license.seats.ofGranted": "{used} von {granted}",
  "license.seats.left": "{count} frei",
  "license.seats.over": "{count} über der gewährten Anzahl",
  "license.over.title": "Belegte Plätze überschreiten den Lizenzumfang",
  "license.over.body":
    "Belegte Plätze: {used}. Die Lizenz gewährt {granted}. Niemand verliert den Zugriff und kein Platz wird entfernt, aber neue Mitglieder können erst eingeladen werden, wenn die Anzahl innerhalb des Lizenzumfangs liegt. Deaktiviere ein Mitglied oder erhöhe den Lizenzumfang.",
  "license.holder.title": "Lizenziert für",
  "license.holder.company": "Unternehmen",
  "license.holder.contact": "Kontakt",
  "license.holder.installation": "Installation",
  "license.holder.validUntil": "Gültig bis",
  "license.holder.expiredOn": "Abgelaufen am",
  "license.holder.id": "Lizenz-ID",
  "license.grace.title": "Diese Lizenz ist abgelaufen",
  "license.grace.body":
    "Die Lizenz ist am {expiry} abgelaufen und funktioniert noch für einen begrenzten Zeitraum. Erneuere sie, damit die Installation in Betrieb bleibt.",
  "license.renewal.title": "Lizenz muss erneuert werden",
  "license.renewal.body":
    "Die Lizenz läuft am {expiry} ab. Vor diesem Datum ändert sich nichts.",
  "license.counting":
    "Volle Plätze, die weder deaktiviert noch gesperrt sind, Agenten eingeschlossen. Leseplätze sind unbegrenzt und werden nie gezählt. Neue Mitglieder werden anhand dieser Anzahl zugelassen.",
  "settings.rates.fxTitle": "Währungskurse",
  "settings.rates.fxIntro":
    "Wechselkurse, die Beträge in Fremdwährung in die Basiswährung umrechnen. Neue Kurse gelten ab heute oder später; frühere Kurse ändern sich nie.",
  "settings.rates.fxWithheld":
    "Nur Admins und Operations sehen Währungskurse. Jede Gesamtsumme in der Installation wird damit umgerechnet.",
  "settings.rates.modelWithheld":
    "Nur Admins und Operations sehen Modellpreise.",
  "settings.rates.readOnly":
    "Nur Lesezugriff. Deine Rolle darf keine Kurse ändern.",
  "settings.rates.fxTableLabel": "Geltende Kurse",
  "settings.rates.fxAdd": "Kurs festlegen",
  "settings.rates.fxEmpty": "Noch keine Währungskurse.",
  "settings.rates.fxModalTitle": "Währungskurs festlegen",
  "settings.rates.rateToBase": "Kurs (zur Basiswährung)",
  "settings.rates.modelTitle": "KI-Modellkosten",
  "settings.rates.modelIntro":
    "Preis pro Modell in USD pro Million Tokens, zur Schätzung der KI-Kosten. Preise beeinflussen das Modell-Routing nie.",
  "settings.rates.modelTableLabel": "Geltende Preise",
  "settings.rates.modelAdd": "Modellpreis hinzufügen",
  "settings.rates.modelEmpty": "Noch keine Modellpreise.",
  "settings.rates.modelModalTitle": "Modellpreis festlegen",
  "settings.rates.notSaved": "Wert nicht gespeichert",
  "settings.rates.setRate": "Speichern",
  "settings.rates.refresh": "Aus Quellen aktualisieren",
  "settings.rates.refreshEnqueued":
    "Aktualisierung angefordert. Vorgeschlagene Änderungen erscheinen in Freigaben.",
  "settings.rates.colFrom": "Von",
  "settings.rates.colRate": "Kurs (→{base})",
  "settings.rates.colEffective": "Gültig ab",
  "settings.rates.colProvider": "Anbieter",
  "settings.rates.colModel": "Modell",
  "settings.rates.colInput": "Eingabe $/M",
  "settings.rates.colOutput": "Ausgabe $/M",
  "settings.rates.colCacheRead": "Cache-Lesen $/M",
  "settings.rates.colCacheWrite": "Cache-Schreiben $/M",
  "settings.voice.title": "Voice DNA",
  "settings.voice.intro":
    "Dein persönlicher Schreibstil prägt Entwürfe, die für dich geschrieben werden. Nur du siehst ihn, und er lernt nur aus Schreibproben, die du hinzufügst.",
  "settings.voice.readOnly":
    "Nur Lesezugriff. Deine Rolle darf die Voice DNA nicht ändern.",
  "settings.voice.emptyBody":
    "Füge Schreibproben hinzu, um deine Voice DNA aufzubauen. Das dauert etwa eine Minute.",
  "settings.voice.status.collecting": "Wird gesammelt",
  "settings.voice.status.ready": "Bereit",
  "settings.voice.status.stale": "Neuaufbau nötig",
  "settings.voice.bandThin": "dünn",
  "settings.voice.bandGood": "gut",
  "settings.voice.bandRich": "reichhaltig",
  "settings.voice.bandSharp": "präzise",
  "settings.voice.version": "Version {n}",
  "settings.voice.derivedLabel": "Abgeleiteter Schreibstil",
  "settings.voice.derivedEmpty":
    "Noch nicht aufgebaut. Füge Proben hinzu und starte den Aufbau, um den abgeleiteten Schreibstil zu sehen.",
  "settings.voice.personalityLabel": "Vorgaben",
  "settings.voice.personalityPlaceholder":
    "Notizen dazu, wie du klingen möchtest. Der Text bleibt, wie du ihn schreibst; das Modell überschreibt ihn nie.",
  "settings.voice.savePreferences": "Vorgaben speichern",
  "settings.voice.corpusLabel": "Schreibproben",
  "settings.voice.corpusRowLabel": "Aktuelle Proben",
  "settings.voice.meter": "{count} von {target} W\u00f6rtern",
  "settings.voice.register.email": "E-Mail",
  "settings.voice.register.social": "Social Media",
  "settings.voice.register.long_form": "Langform",
  "settings.voice.register.spoken": "gesprochen",
  "settings.voice.register.general": "allgemein",
  "settings.voice.bandDrop":
    "Durch das Entfernen fällt der Schreibstil von {from} auf {to}. Wähle zum Bestätigen erneut „Entfernen“.",
  "voice.insights.avoidLabel": "Was dein Schreibstil vermeidet",
  "voice.insights.voiceScore": "Stilübereinstimmung {pct} %",
  "voice.insights.next.addTranscript":
    "Füge ein Gesprächs- oder Termintranskript hinzu; gesprochene Worte sind die stärkste Quelle.",
  "voice.insights.next.addEmail":
    "Füge gesendete E-Mails hinzu, die wichtigste Quelle dafür, wie du beruflich schreibst.",
  "voice.insights.next.addWords":
    "Füge etwa {count} weitere Wörter hinzu, um den Zielbereich zu erreichen.",
  "voice.insights.next.atTarget":
    "Deine Schreibproben haben den Zielumfang erreicht. Füge gelegentlich neue Texte hinzu, damit sie aktuell bleiben.",
  "voice.status.active": "Aktiv",
  "voice.status.candidate": "Wartet auf Pr\u00fcfung",
  "voice.status.superseded": "Abgel\u00f6st",
  "voice.status.rejected": "Abgelehnt",
  "voice.classification.routine": "routinem\u00e4\u00dfige \u00c4nderung",
  "voice.classification.material": "wesentliche \u00c4nderung",
  "voice.outcome.autoActivated": "automatisch aktiviert",
  "voice.outcome.reviewRequired": "Pr\u00fcfung erforderlich",
  "voice.outcome.manuallyActivated": "von dir aktiviert",
  "voice.outcome.rejected": "abgelehnt",
  "voice.outcome.rollback": "wiederhergestellt",
  "voice.history.versionRow": "v{n} \u00b7",
  "voice.history.loadMore": "\u00c4ltere Eintr\u00e4ge anzeigen",
  "voice.insights.provenance": "Aus deinen Schreibproben erstellt · v{n}",
  "voice.insights.statWords": "W\u00f6rter: {count}",
  "voice.insights.statSources": "Quellen: {count}",
  "voice.insights.statSentence": "Etwa {count} Wörter pro Satz",
  "voice.insights.thinkingLabel": "Wie du denkst",
  "voice.insights.movesLabel": "Deine typischen Formulierungen",
  "voice.insights.samplesLabel": "Beispielentwürfe in deinem Schreibstil",
  "voice.insights.draftOnly": "Nur Entwurf, wird nie gesendet",
  "voice.insights.disclosure":
    "KI-gestützte Entwürfe. Jeder Versand bleibt eine menschliche Entscheidung.",
  "voice.insights.nextBestLabel": "Zur Verbesserung:",
  "voice.candidate.title": "Schreibstil v{n} bereit zur Prüfung",
  "voice.candidate.whatItIs":
    "Das hat der Aufbau aus deinen Schreibproben gelernt. Für Entwürfe wird diese Version erst genutzt, wenn du sie auswählst.",
  "voice.candidate.reviewLabel":
    "Was diese Version über deinen Schreibstil sagt",
  "voice.candidate.concernsLabel": "Warum diese Version eine Prüfung braucht",
  "voice.candidate.applyHint":
    "Wenn er nach dir klingt, verwende ihn. Wenn nicht, behalte deinen aktuellen Schreibstil und füge weitere Schreibproben hinzu; der nächste Aufbau lernt daraus.",
  "voice.candidate.reason.malformed":
    "Die Bewertungsprüfung konnte einige Beispielentwürfe nicht lesen, daher beruht die Bewertung auf weniger Beispielen als üblich.",
  "voice.candidate.reason.lowScore":
    "Beispielentwürfe in diesem Schreibstil erreichten im Vergleich zu deinen Texten {score} und liegen damit unter dem Mindestwert von {floor}, den diese Installation für die automatische Aktivierung verlangt.",
  "voice.candidate.reason.hardFailures":
    "Formulierungen, die dieser Schreibstil vermeiden soll und die in den Beispielentwürfen vorkamen: {n}.",
  "voice.candidate.reason.rulesRemoved":
    "Seit deiner vorherigen Version weggefallene Vermeidungsregeln: {n}.",
  "voice.candidate.apply": "Diese Version verwenden",
  "voice.candidate.reject": "Aktuellen Schreibstil behalten",
  "voice.history.label": "Versionen und Lernen",
  "voice.history.empty":
    "Noch keine Versionen. Baue zuerst dein Stilprofil auf.",
  "voice.history.deltasLabel": "Was sich ge\u00e4ndert hat",
  "voice.history.deltasEmpty":
    "Noch nichts zu vergleichen. Änderungen erscheinen ab dem zweiten Aufbau.",
  "voice.history.deltaRow": "v{from} \u2192 v{to}",
  "voice.history.learning":
    "Lernt fortlaufend: erstellte Entwürfe {drafted}, vor dem Senden bearbeitet {edited}, abgelehnt {rejected}.",
  "voice.history.rollback": "Version {n} wiederherstellen",
  "settings.voice.corpusEmpty": "Noch keine Proben.",
  "settings.voice.excluded": "ausgeschlossen",
  "settings.voice.removeSource": "Probe entfernen",
  "settings.voice.addSource": "Schreibproben hinzufügen",
  "settings.voice.addFirstLabel": "Erste Schreibprobe",
  "settings.voice.dropHint":
    "Dateien ablegen oder auswählen (.txt, .md, .pdf, .docx, .vtt, .srt oder .json), auch mehrere auf einmal.",
  "settings.voice.dropEmpty": "Dateien hier ablegen oder zum Auswählen klicken",
  "settings.voice.whyToggle": "Warum Proben hinzufügen",
  "settings.voice.whyBody":
    "Margince entwirft E-Mails in deinen eigenen Worten. Ton, Rhythmus und Formulierungen lernt es nur aus deinen Texten. Proben sind nur für dich sichtbar.",
  "settings.voice.worksTitle": "Die besten Proben",
  "settings.voice.worksEmails":
    "Gesendete E-Mails, gespeichert als .txt, .md, .pdf oder .docx.",
  "settings.voice.worksDocs":
    "Angebote, Posts und andere Texte, die du selbst geschrieben hast.",
  "settings.voice.worksTranscripts":
    "Transkripte von Anrufen oder Terminen (.vtt, .srt, .json oder ein Textexport). Margince fragt, wer davon du bist, und behält nur deine Redebeiträge.",
  "settings.voice.worksNot":
    "Lass Texte anderer und KI-Entwürfe weg. Daraus entsteht ein anderer Schreibstil.",
  "settings.voice.floorNote":
    "Ein erster Aufbau braucht mindestens {min} Wörter. Darunter kopiert das Modell Formulierungen.",
  "settings.voice.floorLabel":
    "Fortschritt bis zum ersten Aufbau ({min} Wörter)",
  "settings.voice.floorProgress":
    "{words} von {min} Wörtern für einen ersten Aufbau",
  "settings.voice.speakerQuestion":
    "„{name}“ enthält mehrere Sprechende. Wer davon bist du?",
  "settings.voice.speakerWhy":
    "Nur deine Redebeiträge werden behalten. Die Worte anderer Sprechender werden verworfen.",
  "settings.voice.speakerDetail": "{words} Wörter, {turns} Redebeiträge",
  "settings.voice.speakerConfirm": "Diese Person verwenden",
  "settings.voice.speakerDismiss": "Datei überspringen",
  "settings.voice.noticeKept":
    "{name}: {kept} von {total} Wörtern behalten. Nur deine Redebeiträge zählen.",
  "settings.voice.noticeAdded": "{name}: {words} Wörter hinzugefügt.",
  "settings.voice.noticeSkippedType":
    "{name} wurde übersprungen. Unterstützte Formate: .txt, .md, .pdf, .docx, .vtt, .srt, .json.",
  "settings.voice.noticeSkippedUnreadable":
    "{name} konnte nicht geöffnet werden. Wenn die Datei passwortgeschützt oder beschädigt ist, füge stattdessen ihren Text ein.",
  "settings.voice.noticeSkippedEmpty":
    "{name} wurde übersprungen. Die Datei enthält keinen Text.",
  "settings.voice.noticeDismissed":
    "{name} wurde übersprungen. Nichts darin ließ sich dir zuordnen.",
  "settings.voice.noticeAskQueueFull":
    "{name} wurde nicht hinzugefügt. Beantworte die Fragen zu den Sprechenden oben und füge die Datei dann erneut hinzu.",
  "settings.voice.noticeFailed":
    "{name} konnte nicht hinzugefügt werden: {detail}",
  "settings.voice.noticeUnexpected": "{name} konnte nicht hinzugefügt werden.",
  "settings.voice.refusalUnattributed":
    "{name} enthält mehrere Sprechende, und kein Text ließ sich dir zuordnen, daher wurde nichts hinzugefügt.",
  "settings.voice.refusalSpeaker":
    "Diese Person kommt in {name} nicht vor, daher wurde nichts hinzugefügt.",
  "settings.voice.refusalUnsupported":
    "{name} liegt in einem nicht unterstützten Format vor.",
  "settings.voice.buildsTitle": "Aufbauvorgänge",
  "settings.voice.buildRowLabel": "Aus Proben aufbauen",
  "settings.voice.building": "Wird aufgebaut…",
  "settings.voice.buildRunning":
    "Das Stilprofil wird aufgebaut. Das dauert etwa eine Minute und läuft weiter, wenn du die Seite verlässt.",
  "settings.voice.rebuild": "Voice DNA neu aufbauen",
  "settings.voice.buildFirst": "Voice DNA aufbauen",
  "settings.voice.buildNeedsWords":
    "Ein erster Aufbau braucht noch etwa {n} Wörter. Darunter gibt es nicht genug Text, um daraus zu lernen.",
  "settings.voice.buildProvisional":
    "Genug für einen Aufbau. Etwa {n} weitere Wörter ergeben ein vollständigeres Bild deines Schreibstils.",
  "settings.voice.buildStatus.succeeded": "Voice DNA aktualisiert",
  "settings.voice.buildStatus.failed":
    "Aufbau nicht abgeschlossen. Versuche es erneut.",
  "settings.voice.buildStatus.deferred":
    "Eingereiht. Diese Seite aktualisiert sich, wenn der Aufbau abgeschlossen ist.",
  "settings.voice.buildStatus.pending":
    "Aufbau läuft noch. Diese Seite aktualisiert sich, wenn der Aufbau abgeschlossen ist.",
  "extAccess.title": "Erweiterungen und Zugriff",
  "extAccess.sub":
    "Was jede Erweiterungseinheit dieser Installation hinzufügt und welche Rollen sie nutzen dürfen. Nur für Admins.",
  "extAccess.adminOnly":
    "Diese Seite erfordert die Berechtigung, Erweiterungen und Rollen der Installation zu lesen. Deiner Rolle fehlt eine davon oder beide.",
  "extAccess.readOnly":
    "Deine Rolle kann diese Seite lesen. Um eine Berechtigung zu ändern, ist ein voller Platz nötig.",
  "extAccess.empty": "Es sind keine Erweiterungseinheiten installiert.",
  "extAccess.version": "Version {version}",
  "extAccess.openUnit": "Seite {name} öffnen",
  "extAccess.noPage":
    "{name} ist in der API installiert, aber diese App-Version hat keine Seite dafür. Die App ist vermutlich älter als der Server.",
  "extAccess.brings.heading": "Was diese Einheit hinzufügt",
  "extAccess.brings.objects": "Berechtigungsobjekte",
  "extAccess.brings.routes": "Routen",
  "extAccess.brings.jobs": "Hintergrund-Jobs",
  "extAccess.brings.none": "Keine",
  "extAccess.noObjects":
    "Diese Einheit registriert keine Berechtigungsobjekte, daher gibt es nichts zu vergeben.",
  "extAccess.roleColumn": "Rolle",
  "extAccess.action.read": "Lesen",
  "extAccess.action.create": "Anlegen",
  "extAccess.action.update": "Ändern",
  "extAccess.action.delete": "Löschen",
  "extAccess.matrixCaption": "Berechtigungen für {object}",
  "extAccess.cell": "{action} von {object} für {role} erlauben",
  "extAccess.versionSkew":
    "Jemand anderes hat diese Rolle geändert, während du sie angesehen hast, daher wurde deine Änderung nicht übernommen. Die Berechtigungen oben sind aktuell. Nimm die Änderung bei Bedarf erneut vor.",
  "extAccess.systemRole": "Vorgegebene Rolle",
  "extAccess.readOnlyTitle": "Nur Lesezugriff für deine Rolle",
  "extAccess.nobodyReadsTitle": "Keine Rolle kann diese Erweiterung lesen",
  "extAccess.grantFailed": "Berechtigung nicht geändert",
  "extAccess.nobodyReads":
    "Keine Rolle hat Lesezugriff auf {object}, daher sieht jedes Mitglied für diese Erweiterung eine leere Ansicht. Vergib unten mindestens einer Rolle das Leserecht.",
  "users.empty": "Noch keine Mitglieder.",
  "users.adminOnly": "Deine Rolle darf keine Mitglieder verwalten.",
  "users.inviteTitle": "Mitglied einladen",
  "users.teamsLabel": "Teams",
  "users.noTeamsYet": "Noch keine Teams.",
  "users.teamMembersLabel": "Teammitglieder",
  "users.teamMembersAdminOnly": "Deine Rolle darf keine Teammitglieder sehen.",
  "users.teamNobodyToAdd": "Keine Mitglieder zum Hinzufügen.",
  "users.teamsTitle": "Teams",
  "users.teamsSub":
    "Benannte Gruppen, mit denen Datensätze geteilt werden können. Für die meisten Rollen gewährt die Mitgliedschaft keinen Zugriff. Eine Teamleitung im Team kann dessen Datensätze auch ungeteilt lesen und bearbeiten.",
  "users.teamsAdminOnly": "Deine Rolle darf keine Teams verwalten.",
  "users.deactivated": "{name} deaktiviert",
  "users.reactivated": "{name} reaktiviert",
  "users.roleSaved": "Rolle für {name} geändert",
  "users.teamArchived": "Team „{name}“ archiviert",
  "users.teamRestored": "Team „{name}“ wiederhergestellt",
  "users.archiveTeam": "Team {name} archivieren",
  "users.newTeamLabel": "Neues Team",
  "users.newTeamOpen": "Neues Team",
  "users.teamNameLabel": "Teamname",
  "users.newTeamPlaceholder": "Zum Beispiel DACH Sales",
  "users.createTeam": "Team anlegen",
  "users.notArchived": "Team nicht archiviert",
  "users.notSaved": "Änderung nicht gespeichert",
  "users.notCreated": "Team nicht angelegt",
  "users.inviteFailed": "Einladung nicht gesendet",
  "users.access.title": "Zugriff des Mitglieds",
  "users.access.identity":
    "Liest alle Kontakte, Unternehmen, Leads und Deals dieses Unternehmens.",
  "users.access.writesAll": "Bearbeitet alle Datensätze.",
  "users.access.writesTeam":
    "Bearbeitet eigene Datensätze und die der Teams {teams}.",
  "users.access.writesTeamNone":
    "Bearbeitet nur eigene Datensätze. Noch keinem Team zugeordnet.",
  "users.access.writesOwn": "Bearbeitet nur eigene Datensätze.",
  "users.access.none": "kein Zugriff",
  "users.access.read": "lesen",
  "users.access.write": "schreiben",
  "users.access.delete": "löschen",
  "users.access.object.contact": "Kontakte",
  "users.access.object.company": "Unternehmen",
  "users.access.object.lead": "Leads",
  "users.access.object.deal": "Deals",
  "users.access.object.project": "Projekte",
  "users.access.mask": "{field} ist {when} ausgeblendet.",
  "users.access.maskAlways": "immer",
  "users.access.maskOutside": "bei Datensätzen ohne Bearbeitungsrecht",
  "users.inviteSub":
    "Füge dieser Installation ein Mitglied hinzu und wähle seine Startrolle.",
  "users.membersTitle": "Mitglieder",
  "users.membersSub":
    "Alle, die einen Platz haben, einschließlich deaktivierter Nutzerkonten.",
  "users.memberCount_one": "{count} Mitglied",
  "users.memberCount_other": "{count} Mitglieder",
  "users.teamMemberCount_one": "{count} Mitglied",
  "users.teamMemberCount_other": "{count} Mitglieder",
  "users.emailLabel": "E-Mail",
  "users.nameLabel": "Vollständiger Name",
  "users.emailPlaceholder": "name@company.com",
  "users.namePlaceholder": "Vollständiger Name",
  "users.deactivateConfirmTitle": "{name} deaktivieren?",
  "users.deactivateConfirmBody":
    "Die Person wird überall abgemeldet, und ihre Agenten-Passports werden sofort widerrufen. Eine spätere Reaktivierung ist möglich; danach muss sich die Person erneut anmelden.",
  "users.deactivateAgentConfirmBody":
    "Das ist die Agentenidentität des Unternehmens. Diese Identität meldet sich nirgends an, daher verliert niemand den Zugriff. Geplante Jobs von Erweiterungen laufen weiter; jeder Job handelt unter seiner eigenen Identität und erfasst unter der Berechtigung des Mitglieds, dessen Verbindung den Datensatz erzeugt hat.",
  "users.agentSeat": "Agent",
  "users.agentSeatRole": "Handelt über einen Passport, nicht über eine Rolle",
  "users.roleLabel": "Rolle",
  "users.inviteOpen": "Mitglied einladen",
  "users.invite": "Einladen",
  "users.setRole": "Rolle festlegen…",
  "users.setRoleFor": "Rolle für {name} festlegen",
  "users.rowActions": "Aktionen für {name}",
  "users.rolesHeld": "Hat {roles}. Eine Rollenauswahl ersetzt alle.",
  "users.deactivate": "Deaktivieren",
  "users.reactivate": "Reaktivieren",
  "users.status.active": "Aktiv",
  "users.status.invited": "Eingeladen",
  "users.status.deactivated": "Deaktiviert",
  "users.status.suspended": "Gesperrt",
  "users.link.action": "Passwort-Link erstellen",
  "users.link.title": "Passwort-Link für {name}",
  "users.link.pending": "Link wird erstellt…",
  "users.link.body":
    "Sende diesen Link über einen vertrauenswürdigen Kanal an das Mitglied. Er funktioniert einmal und wird nur jetzt angezeigt. Ein neuer Link kann in der Zeile des Mitglieds erstellt werden.",
  "users.link.urlLabel": "Passwort-Link",
  "users.link.copy": "Link kopieren",
  "users.link.copied": "Kopiert",
  "users.link.copyFailed":
    "Kopieren fehlgeschlagen. Markiere den Link im Feld und kopiere ihn von Hand.",
  "users.link.expires": "Läuft am {when} ab.",
  "users.link.failedTitle": "Link nicht erstellt",
  "users.link.failed":
    "Das Mitglied existiert, kann sich aber erst anmelden, wenn es einen Link erhält.",
  "users.link.offline":
    "Der Server war nicht erreichbar. Prüfe die Verbindung und versuche es erneut.",
  "users.link.retry": "Erneut versuchen",
  "users.link.done": "Fertig",
  "settings.companyReadOnly":
    "Nur Lesezugriff. Deine Rolle darf das Unternehmensprofil nicht ändern.",
  "settings.companyTitle": "Unternehmensprofil",
  "settings.companySub":
    "Gemeinsamer Geschäftskontext für Entwürfe, Angebote, Suche und Agenten. Zu jeder Aussage wird festgehalten, wer sie geliefert hat und woher sie stammt.",
  "settings.companyTrust":
    "Nur bestätigte Aussagen. Website-Text wird nie zu Anweisungen.",
  "settings.companyConfirmed_one": "{count} bestätigte Aussage",
  "settings.companyConfirmed_other": "{count} bestätigte Aussagen",
  "settings.companyMark": "Unternehmenslogo",
  "settings.companyMarkIntro":
    "Die Seitenleiste zeigt das Unternehmen in 2 Breiten. Die eingeklappte Seitenleiste verwendet das breite Logo, bis ein quadratisches Symbol hinzugefügt wird.",
  "settings.companyMarkWide": "Breites Logo",
  "settings.companyMarkWidePresent":
    "Wird hier und oben in der ausgeklappten Seitenleiste angezeigt.",
  "settings.companyMarkWideNone":
    "Noch kein Logo; es werden Initialen angezeigt. Füge hier eines hinzu oder lies die Website aus, um eines zu finden.",
  "settings.companyMarkIcon": "Quadratisches Symbol",
  "settings.companyMarkIconPresent":
    "Wird in der eingeklappten Seitenleiste angezeigt, wo das breite Logo zu klein zum Lesen ist.",
  "settings.companyMarkIconNone":
    "Noch kein Symbol. Die eingeklappte Seitenleiste verwendet das breite Logo.",
  "settings.companyMarkAdd": "Hinzufügen",
  "settings.companyMarkReplace": "Ersetzen",
  "settings.companyMarkRemove": "Entfernen",
  "settings.companyMarkUploadFailed": "Logo nicht hochgeladen",
  "settings.companyMarkRemoveFailed": "Logo nicht entfernt",
  "settings.companyMarkAddWide": "Breites Logo hinzufügen",
  "settings.companyMarkReplaceWide": "Breites Logo ersetzen",
  "settings.companyMarkRemoveWide": "Breites Logo entfernen",
  "settings.companyMarkAddIcon": "Quadratisches Symbol hinzufügen",
  "settings.companyMarkReplaceIcon": "Quadratisches Symbol ersetzen",
  "settings.companyMarkRemoveIcon": "Quadratisches Symbol entfernen",
  "settings.companyMarkWideHint":
    "SVG oder transparentes PNG, etwa 800 × 240 px (bis 4:1), unter 5 MB. JPEG, GIF, WebP und ICO funktionieren ebenfalls. Die Proportionen bleiben erhalten.",
  "settings.companyMarkIconHint":
    "SVG oder transparentes PNG, etwa 256 × 256 px, quadratisch, unter 5 MB. Eine breite Datei wird nicht beschnitten; sie wird klein im Quadrat angezeigt.",
  "settings.companyMarkEmpty": "Logo hier ablegen oder Datei auswählen",
  "settings.companyMarkIconEmpty": "Symbol hier ablegen oder Datei auswählen",
  "settings.companyWebsite": "Öffentliche Unternehmenswebsite",
  "settings.companyWebsiteHint":
    "Die öffentliche Website, bei der das Lesen der Website beginnt.",
  "settings.companySourceTitle": "Quelle",
  "settings.companyRefreshRow": "Website erneut lesen",
  "settings.companyRefreshHint":
    "Margince liest die öffentlichen Seiten und schlägt Änderungen vor. Nichts gelangt ins Profil, bevor du die Änderungen geprüft und übernommen hast.",
  "settings.companyEdit": "Bearbeiten",
  "settings.companyEditField": "{field} bearbeiten",
  "settings.companyWebsiteRequired":
    "Füge vor dem Aktualisieren eine Unternehmenswebsite hinzu.",
  "settings.companyRefresh": "Von der Website aktualisieren",
  "settings.companyEssentials": "Grundlagen",
  "settings.companyPositioning":
    "Positionierung, Käufergruppen und Vertriebsansatz",
  "settings.companyIdentity": "Identität und rechtliche Angaben",
  "settings.companySave": "Unternehmenskontext speichern",
  "settings.companySaved": "Gespeichert",
  "settings.companySaveFailed": "Unternehmenskontext nicht gespeichert",
  "settings.companyRefreshFailed": "Lesen der Website nicht abgeschlossen",
  "settings.companyApplyFailed": "Änderungen nicht übernommen",
  "settings.companyRefreshWarnings": "Warnungen aus diesem Lesevorgang",
  "settings.companyRefreshUnreadable":
    "Der Status dieses Website-Lesevorgangs ist verloren gegangen. Starte die Aktualisierung erneut.",
  "settings.companyRefreshStale":
    "Der Website-Vorschlag hat sich geändert. Prüfe den aktualisierten Vergleich, bevor du ihn übernimmst.",
  "settings.companyRefreshReview": "Website-Vergleich",
  "settings.companyRefreshReady": "Änderungen prüfen",
  "settings.companyRefreshReading": "Website wird gelesen…",
  "settings.companyCoverage": "Seitenabdeckung",
  "settings.companyResolveAll":
    "Wähle für jeden Konflikt mit einem manuell eingegebenen Wert eine Lösung.",
  "settings.companyApplyRefresh": "Ausgewählte Änderungen übernehmen",
  "settings.companySelectChange": "Änderung an {field} auswählen",
  "settings.companyClass.new": "Neu",
  "settings.companyClass.machine_change": "Auf der Website geändert",
  "settings.companyClass.human_conflict": "Deine Entscheidung nötig",
  "settings.companyClass.unchanged": "Unverändert",
  "settings.companyResolution.keep_current": "Aktuellen Wert behalten",
  "settings.companyResolution.accept_proposal": "Website-Wert übernehmen",
  "settings.companyResolution.useValueFor": "Wert, der für {field} bleibt",
  "settings.companyResolution.use_value": "Bearbeiteten Wert verwenden",
  "settings.companyManualKicker": "Manuelle Einrichtung",
  "settings.companyManualTitle": "Grundlagen des Unternehmens",
  "settings.companyManualSub":
    "Das Lesen von Websites ist für diese Installation nicht aktiviert. Diese 3 Antworten ergeben einen nutzbaren Unternehmenskontext, ohne Modellaufruf und ohne externe Anfrage.",
  "settings.companyCreateWorkspace": "Unternehmenskontext erstellen",
  "product.title": "Produkte",
  "product.readOnly":
    "Nur Lesezugriff. Deine Rolle darf keine Produkte ändern.",
  "product.settingsSub":
    "Preislisteneinträge, aus denen Angebotspositionen übernommen werden.",
  "product.new": "Neues Produkt",
  "product.edit": "Produkt bearbeiten",
  "product.archive": "Produkt archivieren",
  "product.archiveConfirm":
    "Dieses Produkt archivieren? Bestehende Angebotspositionen behalten ihren Snapshot.",
  "product.name": "Name",
  "product.sku": "SKU",
  "product.description": "Beschreibung",
  "product.unit": "Einheit",
  "product.unitPrice": "Einzelpreis",
  "product.currency": "Währung",
  "product.taxRate": "Standardsteuersatz %",
  "product.billingModel": "Abrechnung",
  "product.billingUnclassified": "Nicht angegeben",
  "product.billingOneTime": "Einmalig",
  "product.billingRecurring": "Wiederkehrend",
  "product.billingInterval": "Abrechnungszeitraum",
  "product.billingNoInterval": "Nicht angegeben",
  "product.billingMonthly": "Monatlich",
  "product.billingQuarterly": "Vierteljährlich",
  "product.billingHalfYearly": "Halbjährlich",
  "product.billingYearly": "Jährlich",
  "product.active": "Aktiv",
  "product.activeFilter": "Nur aktive",
  "product.activeFilterAll": "Alle",
  "product.inactive": "Inaktiv",
  "product.archived": "Archiviert",

  "template.title": "Angebotsvorlagen",
  "template.readOnly":
    "Nur Lesezugriff. Deine Rolle kann keine Angebotsvorlagen ändern.",
  "template.settingsSub":
    "PDF-Layouts im Unternehmensdesign für Angebote auf Deutsch und Englisch.",
  "template.new": "Neue Vorlage",
  "template.edit": "Vorlage bearbeiten",
  "template.archive": "Vorlage archivieren",
  "template.archiveConfirm":
    "Diese Vorlage archivieren? Angebote, die sie bereits verwenden, behalten ihr Layout. Neue Angebote können sie nicht mehr auswählen.",
  "template.name": "Name",
  "template.locale": "Sprache",
  "template.isDefault": "Standard für die Sprache",
  "template.header": "Kopfzeilentext",
  "template.footer": "Fußzeilentext",
  "template.localeFilter": "Sprache",
  "template.localeFilterAll": "Alle Sprachen",
  "template.localeDE": "Deutsch (DE)",
  "template.localeEN": "Englisch (US)",

  "tools.title": "Agenten-Werkzeuge",
  "tools.sub":
    "Werkzeuge, die ein Passport aufrufen kann; dieselbe Liste, die ein MCP-Client sieht.",
  "tools.egress": "Externer Zugriff",
  "tools.scopeAll": "Alle Passports",
  "tools.inventory": "Alle {count} Werkzeuge",
  "tools.scopeLabel": "Nach Passport filtern",
  "tools.scopedTo": "Erreichbar für {label}",
  "tools.unreachable": "Bereich nicht gewährt",

  "aiusage.title": "Geschätzte KI-Kosten und Nutzung",
  "aiusage.withheld":
    "Nur Admins und Operations sehen die KI-Kosten. Die Zahlen umfassen die gesamte Installation.",
  "aiusage.sub":
    "Bisherige Nutzung im gewählten Monat. Die Schätzungen sind unabhängig vom aktuellen Token-Kontingent und von der Rechnung deines Anbieters.",
  "aiusage.col.task": "Aufgabe",
  "aiusage.col.tier": "Modellstufe",
  "aiTier.decide": "Entscheidungsmodell",
  "aiusage.col.calls": "Aufrufe",
  "aiusage.col.cached": "Aus dem Cache",
  "aiusage.col.tokensIn": "Tokens (Eingabe)",
  "aiusage.col.tokensOut": "Tokens (Ausgabe)",
  "aiusage.col.cost": "Geschätzte Kosten",
  "aiusage.costNote":
    "Die Kosten sind Schätzungen auf Basis der konfigurierten Preise.",
  "aiusage.costPartial":
    "Diese Summe umfasst nur Aufrufe mit Preis: Für {calls} weitere war kein Preis konfiguriert, ihre Nutzung erscheint nur in den Token-Zahlen.",
  "aiusage.monthLabel": "Monat",
  "aiusage.spendLabel": "Kosten nach Aufgabe",
  "aiusage.days.show": "Tage anzeigen",
  "aiusage.empty": "Keine KI-Aufrufe in diesem Monat.",
  "aiusage.prevMonth": "Vorheriger Monat",
  "aiusage.nextMonth": "Nächster Monat",
  "aiusage.decisions.note":
    "Jede Zahl zählt eine Anfrage, gleich wie viele Modelle sie erreicht hat. Bestanden heißt, die Antwort des Entscheidungsmodells galt; bei einem Rückfall hat ein Sprachmodell übernommen.",
  "aiusage.decisions.empty":
    "Diesen Monat hat keine Aufgabe das Entscheidungsmodell gefragt.",
  "aiusage.decisions.col.asked": "Gefragt",
  "aiusage.decisions.col.passRate": "Bestanden",
  "aiusage.decisions.col.fallbackRate": "Rückfälle",
  "aiusage.decisions.col.reasons": "Rückfälle nach Grund",

  "aibanner.degraded":
    "80 % des KI-Kontingents erreicht. Prüfe die betroffenen Funktionen.",
  "aibanner.queued":
    "KI-Kontingent erreicht. Prüfe die zurückgestellte Arbeit.",
  "aibanner.unknown": "Status des KI-Kontingents nicht erkannt",
  "aibanner.link": "Kontingent verwalten",
  "aibanner.dismiss": "Ausblenden",

  "licensebanner.body":
    "Das Lizenz-Token dieser Installation wurde geprüft und abgelehnt.",
  "licensebanner.link": "{tab} öffnen",

  "aicalls.title": "KI-Aufrufprotokoll",
  "aicalls.withheld":
    "Nur Admins und Operations können das Aufrufprotokoll lesen. Es verzeichnet jeden Modellaufruf der Installation.",
  "aiHealth.withheld":
    "Nur Admins und Operations sehen, ob Modellstufen antworten. Das ist Infrastruktur der Installation, keine Daten über deine Arbeit.",
  "aicalls.sub":
    "Jeder Modellaufruf: Routing-Identität, Tokens, Wiederholungen, erfasste Nutzdaten.",
  "aicalls.col.detail": "Details",
  "aicalls.expandCall": "Versuche für {task} ({when}) anzeigen",
  "aicalls.col.when": "Zeitpunkt",
  "aicalls.col.task": "Aufgabe",
  "aicalls.col.model": "Modell",
  "aicalls.col.tokens": "Tokens",
  "aicalls.col.latency": "Latenz",
  "aicalls.ms": "{value} ms",
  "aicalls.badge.cacheHit": "Cache-Treffer",
  "aicalls.badge.degraded": "Herabgestuft",
  "aicalls.badge.retries": "Wiederholung ×{count}",
  "aicalls.badge.decision": "Entscheidungsmodell",
  "aicalls.reason.decision_below_floor":
    "Entscheidungsmodell unter seiner Konfidenzschwelle",
  "aicalls.reason.decision_error": "Entscheidungsmodell fehlgeschlagen",
  "aicalls.reason.decision_off_enum":
    "Entscheidungsmodell hat außerhalb der Optionen geantwortet",
  "aicalls.reason.decision_state_too_large":
    "Eingabe zu groß für das Entscheidungsmodell",
  "aicalls.reason.decision_uncertified":
    "Entscheidungsmodell für diese Aufgabe nicht zertifiziert",
  "aicalls.reason.decision_local_only":
    "Aufgabe nur lokal, Entscheidungsmodell nicht lokal",
  "aicalls.decisionAnswer": "antwortete {choice} mit {confidence}",
  "aicalls.callsLabel": "Letzte Aufrufe",
  "aicalls.filter.all": "Alle Aufgaben",
  "aicalls.loadMore": "Mehr laden",
  "aicalls.empty": "Noch keine KI-Aufrufe aufgezeichnet.",
  "aicalls.detail.identity":
    "{served} über {provider} ausgeliefert (konfiguriert: {configured})",
  "aicalls.detail.source": "Quelle der ausgelieferten Identität: {source}",
  "aicalls.detail.context": "Eingefügter Kontext: {scopes}",
  "aicalls.detail.contextNone": "Kein Unternehmenskontext eingefügt",
  "aicalls.detail.attempts": "Versuche",
  "aicalls.detail.request": "Anfrage-Nutzdaten",
  "aicalls.detail.response": "Antwort-Nutzdaten",
  "aicalls.payload.off":
    "Die Erfassung der Nutzdaten ist ausgeschaltet. Setze ai.capture_payloads: true in margince.yaml, um Inhalte von Anfrage und Antwort aufzuzeichnen.",
  "aicalls.payload.none": "Für diesen Aufruf wurden keine Nutzdaten erfasst.",

  "aiexport.button": "Als Zertifizierungsszenario exportieren",
  "aiexport.title": "Lauf als Zertifizierungsszenario exportieren",
  "aiexport.nameLabel": "Szenarioname",
  "aiexport.checklist":
    "Geheimnisse wurden bei der Erfassung entfernt, personenbezogene Daten nicht: Prüfe und entferne sie, und ersetze dann sanitized_by, bevor du diese Datei in den Korpus übernimmst.",
  "aiexport.copy": "YAML kopieren",
  "aiexport.copied": "Kopiert",
  "aiexport.download": ".yaml herunterladen",
  "aiexport.copyFailed":
    "Kopieren fehlgeschlagen. Verwende stattdessen die Vorschau oder lade die Datei herunter.",
  "aiexport.close": "Schließen",
  "aiexport.previewLabel": "Szenariovorschau",
  "aiexport.responseLabel": "Modellantwort",

  "countdown.daysHours": "{days} T {hours} h",
  "countdown.hoursMinutes": "{hours} h {minutes} min",
  "countdown.minutesSeconds": "{minutes} min {seconds} s",
  "countdown.expired": "Abgelaufen",

  "installationSettings.companyTitle": "Installation",
  "installationSettings.companySub":
    "Name der Installation und die Zeitzone, in der Berichtszeiträume berechnet werden.",
  "installationSettings.currencyTitle": "Währung",
  "installationSettings.dateFormat": "Datumsformat",
  "installationSettings.timeFormat": "Zeitformat",
  "installationSettings.formatsHint":
    "Gilt in der gesamten Oberfläche. Gespeicherte Datumsangaben und Zeitzonen bleiben unverändert.",
  "installationSettings.dateFormat.locale": "Sprache der Oberfläche verwenden",
  "installationSettings.dateFormat.dmy": "TT.MM.JJJJ · 23.09.2026",
  "installationSettings.dateFormat.mdy": "MM/TT/JJJJ · 09/23/2026",
  "installationSettings.dateFormat.ymd": "JJJJ-MM-TT · 2026-09-23",
  "installationSettings.timeFormat.locale": "Sprache der Oberfläche verwenden",
  "installationSettings.timeFormat.24h": "24-Stunden-Format · 17:30",
  "installationSettings.timeFormat.12h": "12-Stunden-Format · 05:30 pm",
  "installationSettings.name": "Unternehmensname",
  "installationSettings.nameHint":
    "Wird überall angezeigt, wo das Produkt das Unternehmen nennt.",
  "installationSettings.timezone": "Berichtszeitzone",
  "installationSettings.timezoneHint":
    "IANA-Zeitzonenname, zum Beispiel Europe/Berlin. Berichtszeiträume und Datumsangaben von Datensätzen werden darin für das ganze Team berechnet und angezeigt, unabhängig von der Anzeigezeitzone jedes Nutzerkontos.",
  "installationSettings.fiscalYearStart": "Beginn des Geschäftsjahres",
  "installationSettings.fiscalYearStartHint":
    "Monat, in dem das Geschäftsjahr beginnt. Berichte gruppieren nach diesem Jahr und Quartal; ein Jahr, das nicht im Januar beginnt, wird mit beiden Jahren benannt, etwa FY2026/27. Eine Änderung benennt jeden Bericht neu, und gespeicherte Ansichten mit Zeitraumfilter umfassen dann andere Monate.",
  "installationSettings.forwardMeasure": "Grundlage der Hochrechnung",
  "installationSettings.forwardMeasureHint":
    "Welcher verbleibende Deal-Wert in der Hochrechnung zum gewonnenen Umsatz addiert wird. Commit-Belege: Deals in Commit mit bestätigtem Abschlussdatum. Gewichtet: jeder offene Deal mit der Wahrscheinlichkeit seiner Phase. Einschätzung der Führungskraft: ersetzt die Hochrechnung; ein Zeitraum ohne Einschätzung fällt auf Commit-Belege zurück und weist darauf hin.",
  "installationSettings.forwardMeasure.commit_evidence":
    "Commit-Belege: nur bestätigte Abschlussdaten",
  "installationSettings.forwardMeasure.weighted":
    "Gewichtet: jeder offene Deal mit der Wahrscheinlichkeit seiner Phase",
  "installationSettings.forwardMeasure.manager_call":
    "Einschätzung der Führungskraft: die für den Zeitraum festgelegte Zahl",
  "forecast.landing": "Erwartetes Ergebnis",
  "forecast.landingFrom":
    "Hochgerechnet · {won} gewonnen + {remaining} erwartet",
  "forecast.landingFromCall": "Aus der Einschätzung · bisher {won} gewonnen",
  "forecast.landingCaveat": "Das erwartete Ergebnis hat einen Vorbehalt",
  "forecast.landing.caveat.call_absent":
    "Für diesen Zeitraum ist keine Einschätzung erfasst, daher werden die Commit-Belege gezeigt.",
  "forecast.landing.caveat.call_below_actual":
    "Die Einschätzung liegt unter dem bereits gewonnenen Betrag. Angezeigt wird sie wie erfasst, nicht korrigiert.",
  "forecast.pipelineNeeded": "Benötigter Deal-Wert",
  "forecast.pipelineNeededDetail": "{open} offen · um {landing} zu erreichen",
  "forecast.pipelineBasisWhy.manager_call":
    "Gemessen an der Einschätzung für diesen Zeitraum.",
  "forecast.pipelineBasisWhy.historical_median":
    "Gemessen am Median der letzten 4 vergleichbaren Zeiträume.",
  "forecast.pipelineAbsentTitle": "Keine Abdeckungszahl für diesen Zeitraum",
  "forecast.pipelineAbsent.insufficient_basis":
    "Für diesen Zeitraum ist keine Einschätzung erfasst, und weniger als 4 vergleichbare Zeiträume sind abgeschlossen. Gemessen an diesen offenen Deals selbst sähe die Abdeckung immer ausreichend aus.",
  "forecast.pipelineAbsent.insufficient_history":
    "Zu wenige abgeschlossene Deals für eine Abschlussquote. Eine Quote aus einer Handvoll Deals ist unzuverlässig.",
  "forecast.coverage": "{percent} % des benötigten Deal-Werts",
  "installationSettings.baseCurrency": "Basiswährung",
  "installationSettings.baseCurrencyHint":
    "ISO-4217-Code, in den alle Beträge für Gesamtsummen umgerechnet werden. Änderbar, bis der erste Betrag umgerechnet wurde.",
  "installationSettings.baseCurrencyLocked":
    "Gesperrt: Beträge wurden bereits in diese Währung umgerechnet, eine Änderung würde daher jede Gesamtsumme verändern.",
  "installationSettings.baseLanguage": "Basissprache",
  "installationSettings.baseLanguageHint":
    "Sprache, in der die KI schreibt, wenn das ganze Team den Text liest. Die Anzeigesprache ist davon getrennt, und Antworten nach außen folgen der Sprache des Threads.",
  "installationSettings.saveFailed": "Einstellungen nicht gespeichert",
  "installationSettings.readOnly":
    "Nur Admins und Operations können diese Einstellungen ändern.",
  "installationSettings.edit": "Bearbeiten",
  "installationSettings.editField": "{field} bearbeiten",
  "installationSettings.save": "Speichern",
  "signInMethods.title": "Anmeldemethoden",
  "signInMethods.saveFailed": "Änderung nicht gespeichert",
  "signInMethods.sub":
    "Wie sich Personen anmelden können. Die Liste zeigt die Anbieter, für die diese Installation Zugangsdaten hat: Ein Admin kann einen Anbieter ausschalten, aber keinen hinzufügen.",
  "signInMethods.password": "E-Mail und Passwort",
  "signInMethods.passwordAlways":
    "Immer verfügbar. Alle Nutzenden können sich so anmelden, daher lassen sich die anderen Methoden gefahrlos ausschalten.",
  "signInMethods.passwordReason":
    "Die Anmeldung mit Passwort lässt sich nicht ausschalten. Dadurch bleibt die Installation zugänglich.",
  "signInMethods.providerHint":
    "Zeigt diesen Anbieter auf der Anmeldeseite. Beim Ausschalten werden laufende Anmeldungen abgebrochen; bestehende Sitzungen bleiben bestehen.",
  "signInMethods.noneConfigured":
    "Für diese Installation ist kein externer Anbieter konfiguriert. Das Passwort ist die einzige Anmeldemethode.",
  "oauthApp.google.title": "Google-App",
  "oauthApp.google.sub":
    "Postfächer werden über eine Google-OAuth-App verbunden, die deinem Unternehmen gehört und dessen eigene Zugangsdaten nutzt. Auch die Anmeldung mit Google läuft darüber.",
  "oauthApp.google.absent":
    "Aus keiner Quelle ist eine App verfügbar. Gmail und Kalender lassen sich nicht verbinden, und die Anmeldung mit Google kann nicht angeboten werden.",
  "oauthApp.google.redirectSub":
    "Trage jede der folgenden URIs beim OAuth-Client in der Google Console ein. Fehlt eine URI, scheitert der Vorgang am Zustimmungsbildschirm mit redirect_uri_mismatch, ohne die URI zu nennen.",
  "oauthApp.google.clientIdPlaceholder":
    "000000000000-xxxx.apps.googleusercontent.com",
  "oauthApp.google.removeConfirmTitle": "Google-App entfernen?",
  "oauthApp.google.removeConfirmBody":
    "Der Clientschlüssel lässt sich nicht wieder auslesen. Um die App wiederherzustellen, müssen Client-ID und Clientschlüssel erneut aus der Google Console eingetragen werden. Gmail- und Kalender-Verbindungen nutzen diese App; Microsoft- und IMAP-Postfächer sind nicht betroffen. Die Ersteinrichtung fragt erneut nach einer App.",
  "oauthApp.microsoft.title": "Microsoft-App",
  "oauthApp.microsoft.sub":
    "Outlook-Postfächer und -Kalender werden über eine Entra-App-Registrierung verbunden, die deinem Unternehmen gehört und dessen eigene Zugangsdaten nutzt. Auch die Anmeldung mit Microsoft läuft darüber.",
  "oauthApp.microsoft.absent":
    "Aus keiner Quelle ist eine App verfügbar. Outlook-E-Mail und -Kalender lassen sich nicht verbinden, und die Anmeldung mit Microsoft kann nicht angeboten werden.",
  "oauthApp.microsoft.redirectSub":
    "Trage jede der folgenden URIs in der Entra-App-Registrierung unter „Authentifizierung“ als Web-Plattform ein. Fehlt eine URI, scheitert der Vorgang am Zustimmungsbildschirm mit AADSTS50011, ohne die URI zu nennen.",
  "oauthApp.microsoft.clientIdPlaceholder":
    "00000000-0000-0000-0000-000000000000",
  "oauthApp.microsoft.removeConfirmTitle": "Microsoft-App entfernen?",
  "oauthApp.microsoft.removeConfirmBody":
    "Der Clientschlüssel lässt sich nicht wieder auslesen. Um die App wiederherzustellen, müssen Client-ID und Clientschlüssel erneut aus dem Entra-Portal eingetragen werden. Outlook-E-Mail- und -Kalender-Verbindungen nutzen diese App; Google- und IMAP-Postfächer sind nicht betroffen. Die Ersteinrichtung fragt erneut nach einer App.",
  "oauthApp.configured": "In Verwendung: {clientId}",
  "oauthApp.fromEnvironment":
    "Aus der Konfiguration dieser Installation in Verwendung: {clientId}. Eine hier gespeicherte App hat Vorrang, bis sie entfernt wird.",
  "oauthApp.pinnedToDirectory": "An das Verzeichnis {tenant} gebunden.",
  "oauthApp.replaceHint":
    "Ein neues Paar ersetzt das gespeicherte. Bestehende Verbindungen funktionieren weiter, bis sie neu verbunden werden.",
  "oauthApp.store": "App speichern",
  "oauthApp.replace": "App ersetzen",
  "oauthApp.writeFailed": "Änderung nicht gespeichert",
  "oauthApp.remove": "App entfernen",
  "oauthApp.redirectCopied": "Kopiert",
  "oauthApp.redirectCopyFailed":
    "Markiere die Adresse und kopiere sie von Hand.",
  "oauthApp.redirectCopy": "URI für {purpose} kopieren",
  "oauthApp.redirect.mailbox_connect": "Postfach",
  "oauthApp.redirect.calendar_connect": "Kalender",
  "oauthApp.redirect.sign_in": "Anmeldung",
  "oauthApp.redirectTitle": "Autorisierte Weiterleitungs-URIs",
  "oauthApp.clientId": "Client-ID",
  "oauthApp.clientSecret": "Clientschlüssel",
  "oauthApp.tenant": "Verzeichnis-ID (Mandant)",
  "oauthApp.tenantHint":
    "Optional. Bindet die App an ein Entra-Verzeichnis: Nur dessen Mitglieder können ein Postfach verbinden, und die Microsoft-Anmeldung nutzt es. Leer lassen, damit jedes Unternehmen verbinden kann; die Anmeldung wartet dann, bis der Server deine Verzeichnisse benennt.",
  "oauthApp.tenantPlaceholder": "00000000-0000-0000-0000-000000000000",
  "oauthApp.saveFailed": "App nicht gespeichert",
  "firstRun.continue": "Weiter",
  "firstRun.ai.title": "Modellanbieter auswählen",
  "firstRun.ai.sub":
    "Margince nutzt deinen Zugang beim KI-Anbieter. All das lässt sich später in den Einstellungen unter KI ändern.",
  "firstRun.ai.provider": "Anbieter",
  "firstRun.ai.key": "API-Schlüssel",
  "firstRun.ai.keyHint":
    "Wird im Schlüsseltresor gespeichert und nie wieder angezeigt.",
  "firstRun.ai.chatModel": "Modell",
  "firstRun.ai.modelHint":
    "Ein Ausgangspunkt. Jedes vom Anbieter verfügbare Modell funktioniert.",
  "firstRun.ai.embedModel": "Embedding-Modell",
  "firstRun.ai.keyFailed": "Schlüssel nicht gespeichert",
  "firstRun.ai.bindFailed": "Anbieter nicht zugeordnet",
  "aiSettings.withheld": "Eingeschränkt",
  "aiSettings.unread": "Nicht verfügbar",
  "aiSettings.pending": "Wird geladen…",
  "aiSettings.spend.label": "Tokens · dieser Monat",
  "aiSettings.spend.value": "{spent} von {budget}",
  "aiSettings.spend.estimated": "≈ {amount} ausgegeben",
  "aiSettings.spend.notPriced": "Ohne Preis",
  "aiSettings.providers.label": "Anbieterschlüssel",
  "aiSettings.providers.value": "{count} von {total}",
  "aiSettings.providers.missing": "Zugeordnet ohne Schlüssel: {count}",
  "aiSettings.providers.lastCall": "letzter Aufruf {elapsed}",
  "aiSettings.providers.lastCallOnly": "Letzter Aufruf {elapsed}",
  "aiSettings.providers.neverCalled": "Noch nie aufgerufen",
  "aiSettings.providers.traceFailed": "Aufrufprotokoll nicht verfügbar",
  "elapsed.justNow": "gerade eben",
  "elapsed.minutes": "vor {minutes} min",
  "elapsed.hours": "vor {hours} h",
  "elapsed.days": "vor {days} T",
  "aiRouting.lane.local_small":
    "Niedrigste Modellstufe; die Zuordnung bestimmt den Verarbeitungsort",
  "aiRouting.lane.cheap_cloud":
    "Alltägliche Arbeit: Anreicherung, Zusammenfassungen, Triage",
  "aiRouting.lane.premium": "Text, den Kontakte lesen",
  "aiRouting.lane.frontier":
    "Modellstufe für komplexes Schlussfolgern; verfügbar heißt nicht, dass sie genutzt wird",
  "aiRouting.lane.local_large":
    "Höhere lokale Modellstufe; konfigurierten Endpunkt prüfen",
  "aiRouting.lane.embeddings": "Suche und Abruf über Datensätze hinweg",
  "aiRouting.lane.decisions":
    "Typisierte Fragen, vor den Modellstufen gestellt, wo zertifiziert",
  "aiRouting.decisions.add": "Entscheidungsmodell hinzufügen",
  "aiRouting.decisions.remove": "Entscheidungsmodell entfernen",
  "aiRouting.decisions.preset.openrouter": "OpenRouter verwenden",
  "aiRouting.decisions.preset.openrouterKey":
    "Trägt Endpunkt und Modell von OpenRouter ein. JEV_COMPATIBLE_API_KEY nimmt deinen OpenRouter-Schlüssel auf.",
  "aiRouting.decisions.absent":
    "Kein Entscheidungsmodell. Jede Aufgabe nutzt die Modellstufen.",
  "aiRouting.lanes.title": "Modellstufen",
  "aiRouting.priceSheet": "Preisliste",
  "aiRouting.provider.label": "Anbieter",
  "aiRouting.change": "Ändern",
  "aiRouting.done": "Fertig",
  "aiRouting.noKey": "Kein Schlüssel",
  "aiRouting.unpriced": "Ohne Preis",
  "aiRouting.effect":
    "Gespeicherte Zuordnungen erreichen jeden Prozess innerhalb einer Minute, ohne Neustart.",
  "aiProviderKeys.title": "Schlüssel der Modellanbieter",
  "aiProviderKeys.keyless": "Kein Schlüssel nötig",
  "aiProviderKeys.field": "API-Schlüssel",
  "aiProviderKeys.save": "Schlüssel speichern",
  "aiProviderKeys.adminOnly":
    "Nur Admins und Operations können einen Anbieterschlüssel ändern.",
  "aiProviderKeys.saveFailed": "Anbieter nicht aktualisiert",
  "aiProviderKeys.configured": "Konfiguriert",
  "aiProviderKeys.absent": "Nicht festgelegt",
  "aiProviderKeys.optional": "Optional",
  "aiProviderKeys.configuredHint":
    "Im Schlüsseltresor gespeichert und nicht auslesbar. Füge einen neuen Schlüssel ein, um ihn zu ersetzen. Er kann auch als {envVar} bereitgestellt werden.",
  "aiProviderKeys.absentHint":
    "Für diesen Anbieter gibt es keinen Schlüssel, daher können ihm zugeordnete Modelle nicht aufgerufen werden. Ein Schlüssel kann auch als {envVar} bereitgestellt werden.",
  "aiProviderKeys.addPlaceholder": "API-Schlüssel einfügen",
  "aiProviderKeys.replacePlaceholder": "Neuen Schlüssel einfügen",
  "aiProviderKeys.add": "Hinzufügen",
  "aiProviderKeys.replace": "Ersetzen",
  "aiProviderKeys.removeConfirmTitle": "{provider}-Schlüssel entfernen?",
  "aiProviderKeys.removeConfirmBody":
    "Der Schlüssel wird aus dem Schlüsseltresor gelöscht und kann nicht wiederhergestellt werden; es gibt keine lesbare Kopie. Jede diesem Anbieter zugeordnete Modellstufe steht still, bis ein neuer Schlüssel hinzugefügt wird.",
  "aiProviderKeys.withheld":
    "Nur Admins und Operations mit der Berechtigung zum Ändern von Modellzuordnungen sehen, welche Anbieter einen Schlüssel haben.",
  "aiProviderKeys.remove": "Entfernen",
  "aiRouting.withheld":
    "Nur Admins und Operations mit der Berechtigung zum Ändern von Modellzuordnungen sehen, welche Modelle diese Installation verwendet.",
  "aiRouting.title": "Modell-Routing",
  "aiRouting.sheetAsOf":
    "Die Modelllisten stammen aus der Preisliste mit Stand {date}. Jede neuere Modell-ID, die der Anbieter bereitstellt, funktioniert ebenfalls, wenn du sie direkt eingibst.",
  "aiRouting.sheetUnknown":
    "Die Modelllisten stammen aus der Preisliste, die deine Rolle nicht lesen kann. Jede Modell-ID, die der Anbieter bereitstellt, funktioniert, wenn du sie direkt eingibst.",
  "aiRouting.unboundTitle": "Keine Modelle zugeordnet",
  "aiRouting.unboundUnkeyed":
    "Es sind keine Modelle zugeordnet, daher sind die KI-Funktionen aus. Füge unten einen Anbieterschlüssel hinzu und ordne dann hier die Modellstufen zu. Eine Installation kann ihre erste Zuordnung auch unter seeds.ai_routing in margince.yaml festlegen; diese wird einmalig gelesen, wenn das Unternehmen angelegt wird.",
  "aiRouting.unboundKeyed":
    "Es sind keine Modelle zugeordnet, daher sind die KI-Funktionen aus. Beginne mit den Standardwerten eines Anbieters, passe sie bei Bedarf an und speichere.",
  "aiRouting.unboundStart": "Mit {provider} beginnen",
  "aiRouting.profile.card": "Installationsprofil",
  "aiRouting.profile.label": "Standort",
  "aiRouting.profile.help":
    "Wo die Inferenz läuft. Souverän bedeutet kein Datenabfluss: nur Modelle auf deinen eigenen Hosts; andere werden beim Speichern abgelehnt.",
  "aiRouting.profile.eu_hosted": "In der EU gehostet",
  "aiRouting.profile.sovereign": "Souverän (kein Datenabfluss)",
  "aiRouting.profile.cloud_frontier": "Cloud-Frontier",
  "aiRouting.dimensions.label": "Vektorbreite",
  "aiRouting.dimensions.help":
    "Leer lassen für den Standardwert des Anbieters. Werte außerhalb von 1 bis 2.000 werden abgelehnt.",
  "aiRouting.baseUrl.placeholder": "https://openrouter.ai/api",
  "aiRouting.baseUrl.label": "Host",
  "aiRouting.baseUrl.help":
    "Host-Wurzel des Anbieters ohne Versionssegment; /v1 wird angehängt. Erforderlich für openai_compatible, das keinen Standardwert hat.",
  "aiRouting.baseUrl.help.jev":
    "Vollständige Endpunkt-URL, unverändert verwendet. Leer lassen für die API von TypeSafe selbst, https://api.typesafe.ai/v1/systemone.",
  "aiRouting.baseUrl.placeholder.jev": "https://api.typesafe.ai/v1/systemone",
  "aiRouting.baseUrl.help.jevCompatible":
    "Vollständige Endpunkt-URL, unverändert verwendet. Erforderlich: OpenRouter ist https://openrouter.ai/api/alpha/decisions, ein selbst betriebener Server etwa http://127.0.0.1:8767/v1/systemone.",
  "aiRouting.baseUrl.placeholder.jevCompatible":
    "https://openrouter.ai/api/alpha/decisions",
  "aiRouting.models.noKey":
    "Nur Preisliste: Dieser Anbieter hat keinen Schlüssel, daher kann seine Modellliste nicht abgerufen werden. Jede Modell-ID, die er bereitstellt, funktioniert trotzdem, wenn du sie direkt eingibst.",
  "aiRouting.models.noEndpoint":
    "Nur Preisliste: Gib oben den Host ein, um die Modellliste dieses Anbieters abzurufen. Jede Modell-ID, die er bereitstellt, funktioniert trotzdem, wenn du sie direkt eingibst.",
  "aiRouting.models.profileForbids":
    "Nur Preisliste: Dieses Installationsprofil erlaubt keinen Zugriff auf diesen Anbieter.",
  "aiRouting.models.notPublished":
    "Nur Preisliste: Dieser Anbieter veröffentlicht keine Modellliste.",
  "aiRouting.models.unreachable":
    "Nur Preisliste: Dieser Anbieter hat nicht geantwortet. Jede Modell-ID, die er bereitstellt, funktioniert trotzdem, wenn du sie direkt eingibst.",
  "aiRouting.model.label": "Modell",
  "aiRouting.model.help":
    "Aufgeführt sind die Modelle, für die diese Installation Preise kennt, pro Million Tokens, Eingabe → Ausgabe. Jede andere Modell-ID, die der Anbieter bereitstellt, funktioniert ebenfalls, wenn du sie direkt eingibst.",
  "aiRouting.save": "Routing speichern",
  "aiRouting.saving": "Zuordnung wird gespeichert…",
  "aiRouting.savedTitle": "Routing gespeichert",
  "aiRouting.saved": "Alle Prozesse verwenden es jetzt.",
  "aiRouting.saveFailed": "Routing nicht gespeichert",
  "aiRouting.adminOnly":
    "Zum Ändern des Modell-Routings sind die Berechtigungen zum Aktualisieren des Routings und zum Lesen des Kontingents nötig.",
  "workingHours.title": "Buchbare Zeiten",
  "workingHours.sub":
    "Persönliche Einstellung. Nur du legst deine Zeiten fest.",
  "workingHours.unsetTitle": "Noch nicht festgelegt",
  "workingHours.unset":
    "Zur Buchung werden 09:00 bis 17:00 Uhr, Montag bis Freitag, in der Zeitzone der Installation angeboten.",
  "workingHours.start": "Tagesbeginn",
  "workingHours.end": "Tagesende",
  "workingHours.days": "Arbeitstage",
  "workingHours.timezone": "Zeitzone",
  "workingHours.timezoneHelp":
    "Zeitzone für Beginn und Ende. Aus diesem Browser vorausgefüllt.",
  "workingHours.narrowedTitle": "Weniger buchbare Zeiten",
  "workingHours.narrowed": "Für Buchungen stehen weniger Zeiten zur Auswahl.",
  "workingHours.saveFailed": "Arbeitszeiten nicht gespeichert",
  "workingHours.save": "Arbeitszeiten speichern",
  "workingHours.day.1": "Montag",
  "workingHours.day.2": "Dienstag",
  "workingHours.day.3": "Mittwoch",
  "workingHours.day.4": "Donnerstag",
  "workingHours.day.5": "Freitag",
  "workingHours.day.6": "Samstag",
  "workingHours.day.7": "Sonntag",
  "autonomy.title": "Automatische Änderungen",
  "autonomy.sub":
    "Automatische Änderungen sind anfangs eingeschaltet. Bestehende Einstellungen bleiben erhalten. Jeder Schalter gilt für deine Arbeit, nicht für das ganze Team.",
  "autonomy.noneDecidedYetTitle": "Noch keine Prüfungen",
  "autonomy.updateFailed": "Einstellung nicht gespeichert",
  "autonomy.noneDecidedYet":
    "Du hast noch keinen dieser Vorschläge geprüft. Automatische Änderungen können gemäß den Schaltern unten bereits laufen. Vorschläge hängen von deinen Datensätzen und der dir zugewiesenen Arbeit ab.",
  "autonomy.noRecord": "Noch keine Entscheidungen dieser Art.",
  "autonomy.record":
    "Bisher: {clean} wie vorgeschlagen freigegeben, {edited} nach Bearbeitung freigegeben, {rejected} abgelehnt.",
  "autonomy.kind.close_date_correction.label": "Abschlussdaten",
  "autonomy.kind.close_date_correction.help":
    "Pflegt Deals über Nacht: schätzt fehlende oder überfällige Abschlussdaten anhand des Deal-Tempos und prüft verstummte Deals. Ausschalten beendet diese Pflege.",
  "autonomy.kind.company_name_promotion.label": "Unternehmensnamen",
  "autonomy.kind.company_name_promotion.help":
    "Übernimmt vorgeschlagene Unternehmensnamen aus E-Mail-Signaturen für Unternehmen, die nach ihrer Domain benannt sind. Schalte es aus, um diese Vorschläge selbst zu prüfen. Von unabhängigen Quellen bestätigte Namen können weiterhin automatisch aktualisiert werden.",
  "autonomy.kind.lifecycle_change.label": "Lebenszyklusphasen",
  "autonomy.kind.lifecycle_change.help":
    "Verschiebt ein Unternehmen anhand der Aktivität in eine andere Lebenszyklusphase. Schalte es aus, um diese Vorschläge selbst zu prüfen. Phasenwechsel können beeinflussen, wer das Unternehmen sieht und welche Automatisierungen laufen.",
  "captureSettings.title": "Anreicherung",
  "captureSettings.sub":
    "Wie erfasste Unternehmen und Kontakte nach ihrer Erstellung angereichert werden.",
  "captureSettings.autoEnrich.label":
    "Erfasste Unternehmen automatisch anreichern",
  "captureSettings.autoEnrich.help":
    "Jedes neue Unternehmen aus erfassten E-Mails erhält automatisch ein Webprofil, das aus seiner Website erstellt wird. Läuft innerhalb eines Tageslimits.",
  "captureSettings.signatureEnrich.label": "Kontaktdaten aus E-Mails lesen",
  "captureSettings.signatureEnrich.help":
    "Wenn eingeschaltet, liest Margince, was eine Person in E-Mails an dich unter ihrem eigenen Namen angibt: eine Signatur oder eine angehängte Visitenkarte mit Position, Telefonnummer, Adresse oder Unternehmen. Das geschieht innerhalb von Minuten nach Eingang. Nichts wird erschlossen; eine Angabe, die die E-Mail nicht enthält, wird nicht geschrieben. Das ist die Standardeinstellung des Unternehmens; ein Postfach mit eigener Einstellung behält diese.",
  "captureSettings.removeFailed": "Ausschluss nicht entfernt",
  "captureSettings.addFailed": "Ausschluss nicht hinzugefügt",
  "captureSettings.updateFailed": "Einstellung nicht geändert",
  "captureSettings.adminOnly": "Nur Admins und Operations können das ändern.",

  "ownDomains.companyTitle": "Unternehmensdomains",
  "captureExclusions.title": "Erfassungsausschlüsse",
  "captureExclusions.sub":
    "Adressen und Domains, deren Nachrichten nie ins CRM gelangen. Deine Regeln gelten nur für Postfächer, die du verbunden hast; Unternehmensregeln gelten für alle.",
  "captureExclusions.notRetroactive":
    "Gilt ab der nächsten Nachricht. Bereits erfasste Nachrichten bleiben erhalten.",
  "captureExclusions.current": "Geltende Regeln",
  "captureExclusions.empty": "Keine Ausschlüsse.",
  "ownerIdentities.title": "Deine weiteren Adressen",
  "ownerIdentities.sub":
    "Adressen, die ebenfalls dir gehören: ein Alias zum Senden, eine private Domain, die du liest, eine Adresse, von der du weiterleitest. E-Mails zwischen diesen Adressen werden nicht erfasst und erzeugen nie einen Kontakt.",
  "ownerIdentities.add": "Adresse hinzufügen",
  "ownerIdentities.addLabel": "Eigene Adresse hinzufügen",
  "ownerIdentities.addDescription":
    "Nur für dich sichtbar. Teammitglieder sehen diese Liste nie.",
  "ownerIdentities.current": "Angegeben",
  "ownerIdentities.notRetroactive":
    "Gilt ab der nächsten Nachricht. Bereits erfasste E-Mails bleiben, und ein bereits aus einem Alias angelegter Kontakt bleibt, bis du ihn zusammenführst oder entfernst.",
  "ownerIdentities.empty": "Keine weiteren Adressen hinzugefügt.",
  "ownerIdentities.remove": "Adresse entfernen",
  "ownerIdentities.added": "Adresse hinzugefügt",
  "ownerIdentities.confirm": "Hinzufügen",
  "ownerIdentities.kindLabel": "Typ",
  "ownerIdentities.kind.address": "Eine Adresse",
  "ownerIdentities.learned.deliveredTo":
    "Automatisch gefunden: E-Mails an diese Adresse kommen in deinem verbundenen Postfach an. Entferne sie, wenn sie nicht dir gehört.",
  "ownerIdentities.learned.provider":
    "Dein E-Mail-Anbieter meldet diese Adresse als eine deiner Adressen. Entferne sie, wenn sie nicht dir gehört.",
  "ownerIdentities.kind.domain": "Eine ganze Domain",
  "ownerIdentities.valueLabel": "Adresse oder Domain",
  "ownerIdentities.addressPlaceholder": "du@beispiel.example",
  "ownerIdentities.removeFailed": "Adresse nicht entfernt",
  "ownerIdentities.addFailed": "Adresse nicht hinzugefügt",
  "ownerIdentities.domainPlaceholder": "beispiel.de",
  "captureExclusions.scope.user": "Deine Postfächer",
  "captureExclusions.scope.workspace": "Gesamtes Unternehmen",
  "captureExclusions.kind.address": "Adresse",
  "captureExclusions.kind.domain": "Domain",
  "captureExclusions.kind.container": "Label, Ordner oder Postfach",
  "captureExclusions.scopeLabel": "Gilt für",
  "captureExclusions.kindLabel": "Art",
  "captureExclusions.addLabel": "Adresse oder Domain ausschließen",
  "captureExclusions.placeholder.address": "name@beispiel.example",
  "captureExclusions.placeholder.domain": "beispiel.example",
  "captureExclusions.add": "Ausschließen",
  "captureExclusions.addOpen": "Neuer Ausschluss",
  "captureExclusions.remove": "Erfassung von {value} fortsetzen",
  "ownDomains.title": "Eigene E-Mail-Domains",
  "ownDomains.sub":
    "Domains, die zu diesem Unternehmen gehören. Nachrichten zwischen Teammitgliedern werden für niemanden gespeichert, auch nicht für dich.",
  "ownDomains.curatedTitle": "Hier verwaltet",
  "ownDomains.irreversible":
    "Eine hinzugefügte Domain gilt ab der nächsten Nachricht; wird sie entfernt, wird ab dann wieder erfasst. E-Mails, die während der Eintragung übersprungen wurden, bietet kein Postfach erneut an. Erfasste E-Mails bleiben erhalten.",
  "ownDomains.fromCompany": "Aus dem Unternehmensprofil. Dort ändern:",
  "ownDomains.openCompany": "Unternehmensprofil öffnen",
  "ownDomains.empty":
    "Keine weiteren Domains eingetragen. Füge eine hinzu, wenn dein Unternehmen auch von einer anderen Domain sendet.",
  "ownDomains.confirmed": "Bestätigt",
  "ownDomains.candidate":
    "In einem verbundenen Postfach gesehen, nicht bestätigt",
  "ownDomains.add": "Hinzufügen",
  "ownDomains.addOpen": "Domain hinzufügen",
  "ownDomains.addLabel": "Eigene Domain hinzufügen",
  "ownDomains.placeholder": "beispiel.example",
  "ownDomains.removeFailed": "Domain nicht entfernt",
  "ownDomains.addFailed": "Domain nicht hinzugefügt",
  "ownDomains.remove": "{domain} entfernen",

  "webhooks.title": "Webhooks",
  "webhooks.readOnly":
    "Nur Lesezugriff: Nur Admins und Operations können Abonnements ändern.",
  "webhooks.sub":
    "Ausgehende Abonnements, die für ausgewählte Ereignisse signierte HTTP-POSTs erhalten.",
  "webhooks.new": "Neues Abonnement",
  "webhooks.notConfigured":
    "Ausgehende Webhooks sind in dieser Installation nicht aktiviert. Konfiguriere zuerst einen Signaturschlüssel.",
  "webhooks.state.active": "Aktiv",
  "webhooks.state.paused": "Pausiert",
  "webhooks.updated": "Aktualisiert am {date}",
  "webhooks.field.targetUrl": "Ziel-URL",
  "webhooks.field.eventTypes": "Ereignistypen",
  "webhooks.field.state": "Status",
  "webhooks.edit": "Bearbeiten",
  "webhooks.saveDone": "Webhook gespeichert",
  "webhooks.archiveDone": "Webhook archiviert",
  "webhooks.archive": "Archivieren",
  "webhooks.archiveConfirm":
    "Das Archivieren stoppt jede Zustellung für dieses Abonnement. Das lässt sich nicht rückgängig machen.",
  "webhooks.rotate": "Signaturschlüssel rotieren",
  "webhooks.rotateConfirm.title": "Signaturschlüssel rotieren?",
  "webhooks.rotateConfirm.body":
    "Der aktuelle Signaturschlüssel wird sofort ungültig, und der neue wird einmal angezeigt. Kopiere ihn und aktualisiere sofort den empfangenden Dienst.",
  "webhooks.secret.title": "Signaturschlüssel",
  "webhooks.secret.warning":
    "Dieser Signaturschlüssel wird nur einmal angezeigt und lässt sich nicht erneut abrufen. Speichere ihn jetzt; Zustellungen werden damit signiert.",
  "webhooks.secret.copy": "Kopieren",
  "webhooks.secret.copied": "Kopiert",
  "webhooks.secret.copyFailed":
    "Markiere den Signaturschlüssel oben und kopiere ihn von Hand.",
  "webhooks.secret.done": "Fertig",
  "webhooks.secret.leaveWarning":
    "Beim Verlassen wird die einzige Kopie dieses Signaturschlüssels vernichtet. Kopiere ihn zuerst.",

  "webhooks.deliveries.show": "Zustellungen anzeigen",
  "webhooks.deliveries.hide": "Zustellungen ausblenden",
  "webhooks.deliveries.empty": "Noch keine Zustellversuche.",
  "webhooks.deliveries.title": "Zustellversuche",
  "webhooks.deliveries.deadLetterGroup": "Dead-Letter ({count})",
  "webhooks.deliveries.allGroup": "Weitere Versuche",
  "webhooks.deliveries.column.status": "Status",
  "webhooks.deliveries.column.event": "Ereignis",
  "webhooks.deliveries.column.attempts": "Versuche",
  "webhooks.deliveries.column.lastStatusCode": "Letzter Status",
  "webhooks.deliveries.column.lastError": "Letzter Fehler",
  "webhooks.deliveries.column.created": "Erstellt",
  "webhooks.deliveries.column.resolved": "Abgeschlossen oder nächster Versuch",
  "webhooks.deliveries.status.pending": "Ausstehend",
  "webhooks.deliveries.status.delivered": "Zugestellt",
  "webhooks.deliveries.status.retrying": "Wird wiederholt",
  "webhooks.deliveries.status.dead_lettered": "Dead-Letter",
  "webhooks.deliveries.status.visibility_revoked":
    "Gestoppt: nicht mehr sichtbar",
  "webhooks.deliveries.replay": "Erneut zustellen",
  "webhooks.deliveries.replayConfirm.title": "Zustellung wiederholen?",
  "webhooks.deliveries.replayConfirm.body":
    "Stellt sofort erneut zu, mit dem aktuellen Signaturschlüssel und einem neuen Zeitstempel, ohne auf den nächsten geplanten Versuch zu warten.",
  "reindexbanner.needed": "Neuindexierung erforderlich",
  "reindexbanner.link": "In den Einstellungen prüfen",

  "embedreindex.title": "Suchindex",
  "embedreindex.sub":
    "Status der Neuindexierung des Suchindex. Nur für Admins und Operations sichtbar.",
  "embedreindex.withheld":
    "Nur Admins und Operations können den Suchindex sehen. Ein Neuaufbau verbraucht Tokens für die gesamte Installation.",
  "embedreindex.unbound":
    "Es ist kein Embedding-Modell zugeordnet, daher gibt es keinen Suchindex zum Neuaufbauen.",
  "embedreindex.unboundLink": "KI-Einstellungen öffnen",
  "embedreindex.statusLabel": "Indexstatus",
  "embedreindex.reindexLabel": "Änderungen neu indexieren",
  "embedreindex.reindexHelp":
    "Bettet nur Datensätze neu ein, deren Text sich seit dem letzten Lauf geändert hat.",
  "embedreindex.rebuildLabel": "Gesamten Index neu aufbauen",
  "embedreindex.rebuildHelp":
    "Bettet jeden Datensatz neu ein. Nutze das, wenn ein Lauf feststeckt oder sich das Embedding-Modell geändert hat.",
  "embedreindex.statusIdle": "Aktuell",
  "embedreindex.statusNeeded": "Neuindexierung erforderlich",
  "embedreindex.statusReembedding": "Wird neu indexiert…",
  "embedreindex.lastProgress": "Letzter Fortschritt vor {duration}",
  "embedreindex.entitiesPending": "Ausstehende Datensätze: {count}",
  "embedreindex.workspacePending": "Ausstehend: {count}",
  "embedreindex.reviewCta": "Prüfen und neu indexieren",
  "embedreindex.rebuildCta": "Index neu aufbauen",
  "embedreindex.confirmTitle": "Neuindexierung starten?",
  "embedreindex.rebuildTitle": "Suchindex neu aufbauen?",
  "embedreindex.confirmCta": "Neuindexierung starten",
  "embedreindex.rebuildConfirmCta": "Jetzt neu aufbauen",
  "embedreindex.previewLoading": "Umfang wird geschätzt…",
  "embedreindex.previewFailed": "Schätzung nicht verfügbar",
  "embedreindex.estimateEntities": "Einzubettende Datensätze:",
  "embedreindex.estimateTokens": "Geschätzte KI-Tokens:",
  "embedreindex.estimateCost": "Geschätzte Kosten:",
  "embedreindex.estimateQualityHeuristic":
    "Heuristische Schätzung: ein Mindestwert, kein beobachteter Verbrauch.",
  "embedreindex.utilizationTitle": "Auswirkung auf das Kontingent",
  "embedreindex.impact.normal": "Normal",
  "embedreindex.impact.degraded": "Würde in den Sparmodus wechseln",
  "embedreindex.impact.queued": "Würde eingereiht werden",

  "consent.title": "Zugriff autorisieren",
  "consent.asks":
    "{client} kann dann in Margince als du handeln, mit dem unten ausgewählten Zugriff.",
  "consent.redirectsTo": "Margince sendet die Autorisierung zurück an {host}.",
  "consent.redirectsToLoopback":
    "Das ist eine Adresse auf diesem Computer, und diese Verbindung kann nicht belegen, welches Programm dort lauscht.",
  "consent.scopeNote.read": "sieht, was du sehen kannst",
  "consent.scopeNote.draft": "bereitet Nachrichten zu deiner Prüfung vor",
  "consent.scopeNote.write":
    "legt Datensätze in deinem Namen an, bearbeitet und archiviert sie",
  "consent.scopeNote.send":
    "sendet Nachrichten in deinem Namen, ohne vorher zu fragen",
  "consent.scopeNote.enrich":
    "verbraucht Anreicherungs-Credits; vor jedem Kauf wirst du weiterhin gefragt",
  "consent.ceiling":
    "Nie mehr als deine eigenen Berechtigungen. Du kannst die Verbindung jederzeit in den Einstellungen unter Agenten trennen.",
  "consent.pickOne":
    "Wähle mindestens eine Berechtigung aus oder verweigere den Zugriff.",
  "consent.offline":
    "Die App bleibt verbunden und erneuert den Zugriff ohne erneute Nachfrage, bis du ihn widerrufst.",
  "consent.approve": "Autorisieren",
  "consent.deny": "Zugriff verweigern",
  "consent.reentering": "Wird erneut verbunden…",
  "consent.backToApp": "Zurück zu Margince",
  "consent.staleTitle": "Diese Anfrage ist abgelaufen",
  "consent.staleBody":
    "Die Verbindungsanfrage ist nicht mehr gültig. Gehe zurück zur App, die du verbinden wolltest, und starte neu; ein Neuladen dieser Seite hilft nicht.",
  "consent.invalidTitle":
    "Diese Verbindungsanfrage konnte nicht abgeschlossen werden",
  "consent.invalidBody":
    "Diese Installation autorisiert die Anfrage in dieser Form nicht; die App ist hier möglicherweise nicht mehr registriert. Gehe zurück zur App, die du verbinden wolltest, und starte neu.",
  "contact.thin.title": "Bekannte Angaben",
  "contact.thin.known":
    "Zu {name} erfasst: {what}. Im Unternehmen hat noch niemand einen erfassten Austausch mit diesem Kontakt.",
  "contact.thin.remediation.capture":
    "Verbinde ein Postfach, das mit diesem Kontakt schreibt, um diese Seite zu füllen, mit der Quelle jedes Felds.",
  "contact.thin.remediation.employer":
    "Ergänze den Arbeitgeber, dann kann Margince auf der Website dieses Unternehmens die Rolle des Kontakts nachlesen.",
  "contact.thin.logFirst": "Erste Interaktion erfassen",
  "contact.enriched.title": "Angereicherte Angaben",
  "contact.confirm.title_one": "{count} Angabe zu bestätigen",
  "contact.confirm.title_other": "{count} Angaben zu bestätigen",
  "contact.confirm.body":
    "{fields} wurden am {when} aus den E-Mails des Kontakts gelesen.",
  "contact.confirm.review": "Prüfen",
  "contact.enriched.sub":
    "Jeder Wert mit dem Text, aus dem er gelesen wurde. Ein korrigierter Wert bleibt erhalten.",
  "contact.enriched.field.title": "Position",
  "contact.enriched.field.phone": "Telefon",
  "contact.enriched.field.role": "Rolle",
  "contact.enriched.field.linkedin": "LinkedIn",
  "contact.enriched.field.company_name": "Unternehmen",
  "contact.enriched.field.address": "Adresse",
  "contact.enriched.field.website": "Website",
  "contact.enriched.readFrom": "Am {when} aus {source} gelesen",
  "contact.enriched.undo": "Rückgängig machen",
  "contact.enriched.replaced": "Hat den älteren Wert „{was}“ ersetzt.",
  "contact.enriched.correctedByYou": "Von dir korrigiert",
  "contact.enriched.confirmed": "Best\u00e4tigt",
  "contact.enriched.confirm": "Wert bestätigen",
  "contact.enriched.save": "Korrektur speichern",
  "contact.enriched.cancel": "Abbrechen",
  "contact.graph.loading": "Kontaktnetzwerk wird geladen…",
  "contact.graph.routeDirect":
    "{name} steht bereits im Austausch mit diesem Kontakt.",
  "contact.graph.routeVia":
    "{name} steht im Austausch mit {through} im selben Unternehmen.",
  "contact.graph.routeDirectYou":
    "Du stehst bereits im Austausch mit diesem Kontakt.",
  "contact.graph.routeViaYou":
    "Du stehst im Austausch mit {through} im selben Unternehmen.",
  "contact.graph.noRoute":
    "Im Unternehmen steht noch niemand im Austausch mit diesem Kontakt oder dessen Unternehmen.",
  "contact.graph.noDirect":
    "Im Unternehmen hat noch niemand mit diesem Kontakt korrespondiert.",
  "contact.graph.sideColumn": "Vorstellungen und Veränderungen",
  "contact.graph.recordWorksWith": "Erfassen: arbeitet mit {name}",
  "contact.graph.noEdge": "Keine erfasste Korrespondenz mit {name}.",
  "contact.graph.withColleague": "mit {name}",
  "contact.graph.withContact": "mit diesem Kontakt",
  "contact.graph.counts":
    "{total} Interaktionen in 90 Tagen \u00b7 {inbound} eingehend, {outbound} ausgehend",
  "contact.graph.untitledMessage": "Ohne Titel",
  "contact.graph.countsOnly":
    "Nur Anzahlen. Die Nachrichten bleiben im Verlauf.",
  "contact.intro.routesTitle": "Wege",
  "contact.graph.droppedNote": "Weitere nicht angezeigt: {count}.",
  "contact.graph.withheldDirect":
    "Einige Teammitglieder werden nicht angezeigt.",
  "contact.graph.withheldAccount":
    "Einige Kontakte dieses Unternehmens werden nicht angezeigt.",
  "contact.intro.askFirstName": "{name} um eine Vorstellung bitten",
  "contact.intro.leadRouteBadge": "Starker Weg",
  "contact.intro.heroDirect": "kennt den Kontakt direkt",
  "contact.intro.heroIndirect": "erreicht den Kontakt über {through}",
  "contact.intro.heroYou": "Du",
  "contact.intro.heroDirectYou": "kennst den Kontakt direkt",
  "contact.intro.heroIndirectYou": "erreichst den Kontakt über {through}",
  "contact.intro.factReciprocal": "Wechselseitig",
  "contact.intro.factOneSided": "Einseitig",
  "contact.intro.factDirect": "Direkte Beziehung",
  "contact.intro.factIndirect": "Über ein Teammitglied",
  "contact.intro.factReceipts_one": "{count} sichtbarer Beleg",
  "contact.intro.factReceipts_other": "{count} sichtbare Belege",
  "contact.intro.verdictDirect":
    "Frage {name}. Das Teammitglied steht bereits im Austausch mit diesem Kontakt.",
  "contact.intro.verdictOneSided":
    "Frage {name}. Das Teammitglied hat diesem Kontakt geschrieben, noch ohne Antwort.",
  "contact.intro.verdictVia":
    "Frage {name}: Der Weg zu diesem Kontakt führt über {through}.",
  "contact.intro.verdictDirectYou":
    "Du stehst bereits im Austausch mit diesem Kontakt.",
  "contact.intro.verdictOneSidedYou":
    "Du hast diesem Kontakt geschrieben, noch ohne Antwort.",
  "contact.intro.verdictViaYou": "Du erreichst den Kontakt über {through}.",
  "contact.intro.ownRouteNoAsk":
    "Niemand zu fragen. Schreibe diesem Kontakt direkt.",
  "contact.intro.evidenceEyebrow": "Belege",
  "contact.intro.evidenceExchanges": "Austausch",
  "contact.intro.evidenceWindow": "in 90 Tagen",
  "contact.intro.evidenceFrom": "{count} von {name}",
  "contact.intro.evidenceFromYou": "{count} von dir",
  "contact.intro.evidenceLastContact": "Letzter Kontakt",
  "contact.intro.lastToday": "Heute",
  "contact.intro.lastYesterday": "Gestern",
  "contact.intro.lastDays": "Vor {days} Tagen",
  "contact.intro.lastNever": "Keiner in 90 Tagen",
  "contact.intro.stripWho": "Wege",
  "contact.intro.stripWhoMix":
    "{direct} direkt · {indirect} über einen Kontakt",
  "contact.intro.stripWhoOwn": "Inklusive deiner eigenen Beziehung",
  "contact.intro.otherRoutesTitle": "Weitere Wege",
  "contact.intro.otherRoutesSub":
    "Sortiert nach wechselseitiger Korrespondenz.",
  "contact.intro.relayDue": "fällig {date}",
  "contact.intro.stripDirect": "Direkte Beziehung",
  "contact.intro.stripNoPath": "Niemand steht im Austausch mit diesem Kontakt",
  "contact.intro.stripNoRoutes": "Keine",
  "contact.intro.stripWhyNow": "Letzte Veränderung",
  "contact.intro.stripNoMoment": "Nichts Neues",
  "contact.intro.change.replied": "Geantwortet",
  "contact.intro.change.repliedSub": "Nach {days} T Ruhe",
  "contact.intro.change.quiet": "Ruhig",
  "contact.intro.change.quietSub": "{days} T",
  "contact.intro.change.warmed": "Wärmer",
  "contact.intro.change.cooled": "Kühler",
  "contact.intro.change.buckets": "{from} → {to}",
  "contact.intro.stripHandoff": "Anfrage zur Vorstellung",
  "contact.intro.handoffNotStarted": "Nicht begonnen",
  "contact.intro.handoffOwner": "Als Nächstes: {name}",
  "contact.intro.ownerColleague": "ein Teammitglied",
  "contact.intro.ownerNobody": "niemand",
  "contact.intro.relayTitle": "Status der Vorstellung",
  "contact.intro.stepRoute": "Weg wählen",
  "contact.intro.stepRoutePick": "auswählen, wen du fragst",
  "contact.intro.stepRequest": "Anfrage",
  "contact.intro.stepNotSent": "nicht gesendet",
  "contact.intro.stepAwaitingAnswer": "wartet auf Teammitglied",
  "contact.intro.stepIntroduction": "Vorstellung",
  "contact.intro.stepNameDrop": "Name genannt",
  "contact.intro.stepWaiting": "wartet",
  "contact.intro.stepRecorded": "erfasst",
  "contact.intro.stepReply": "Antwort",
  "contact.intro.stepObserved": "aus Aktivitäten erkannt",
  "contact.intro.stepDone": "Erledigt",
  "contact.intro.stepCurrent": "Jetzt",
  "contact.intro.stepPending": "Später",
  "contact.intro.laneOurs": "Dein Team",
  "contact.intro.laneTheirs": "Unternehmen des Kontakts",
  "contact.intro.lanePeers": "Umfeld des Kontakts",
  "contact.intro.laneTarget": "Ziel",
  "contact.intro.useThisRoute": "Diesen Weg nutzen",
  "contact.intro.mapRegion": "Wege zu diesem Kontakt",
  "contact.intro.edgeDirect": "{name} korrespondiert direkt mit diesem Kontakt",
  "contact.intro.edgeAccount": "arbeitet mit {name}",
  "contact.intro.routesSub":
    "Bester Weg zuerst. Alternativen greifen, wenn der erste nicht verfügbar ist.",
  "contact.intro.best": "Bester Weg",
  "contact.intro.evidenceTwoWay_one":
    "{total} wechselseitiger Austausch in 90 Tagen · {when}",
  "contact.intro.evidenceTwoWay_other":
    "{total} wechselseitige Austausche in 90 Tagen · {when}",
  "contact.intro.evidenceOneSided_one":
    "{total} Interaktion in 90 Tagen, einseitig · {when}",
  "contact.intro.evidenceOneSided_other":
    "{total} Interaktionen in 90 Tagen, einseitig · {when}",
  "contact.intro.whenToday": "letzter Kontakt heute",
  "contact.intro.whenYesterday": "letzter Kontakt gestern",
  "contact.intro.whenDays": "letzter Kontakt vor {days} Tagen",
  "contact.intro.whenNever": "kein Kontakt in letzter Zeit",
  "contact.intro.askTitle": "Vorstellung bei {name} anfragen",
  "contact.intro.cancel": "Abbrechen",
  "contact.intro.askAction": "Vorstellung anfragen",
  "contact.intro.askFailed":
    "Die Anfrage wurde nicht gespeichert. Versuche es erneut.",
  "contact.intro.reasonLabel": "Grund der Anfrage",
  "contact.intro.reasonHint":
    "Das liest dein Teammitglied, nicht der Kontakt. Beschreibe, warum sich die Vorstellung lohnt.",
  "contact.intro.valueLabel": "Nutzen für den Kontakt",
  "contact.intro.valueHint":
    "Warum der Kontakt diese Vorstellung wollen würde.",
  "contact.intro.noteLabel": "Notiz zum Weiterleiten",
  "contact.intro.noteHint":
    "Der einzige Teil, den der Kontakt liest. Schreibe ihn so, dass er unverändert eingefügt werden kann.",
  "contact.intro.nameDropAsk": "Erlaubnis zur Namensnennung erfragen",
  "contact.intro.fallbackLegend": "Bei Ablehnung",
  "contact.intro.fallbackNone": "Nichts weiter",
  "contact.intro.fallbackNoneHelp":
    "Die Anfrage wird geschlossen, und du entscheidest über den nächsten Schritt.",
  "contact.intro.fallbackNameDrop": "Stattdessen Namensnennung erfragen",
  "contact.intro.fallbackNameDropHelp":
    "Du meldest dich selbst beim Kontakt und nennst dein Teammitglied.",
  "contact.intro.fallbackNextRoute": "Nächsten Weg versuchen",
  "contact.intro.fallbackNextRouteHelp":
    "Weiter zum nächsten Teammitglied auf der Liste.",
  "contact.intro.decideTitle": "Vorstellung bei {name}",
  "contact.intro.decideLegend": "Deine Antwort",
  "contact.intro.decideAction": "Antwort speichern",
  "contact.intro.decideFailed":
    "Die Antwort wurde nicht gespeichert. Versuche es erneut.",
  "contact.intro.decideReasonLabel": "Kommentar",
  "contact.intro.decideReasonHint":
    "Dein Teammitglied sieht das genau so, wie du es schreibst.",
  "contact.intro.noteByModel": "Von Margince entworfen",
  "contact.intro.nameDropRequested":
    "Außerdem wurde gefragt, ob dein Name genannt werden darf.",
  "contact.intro.answerAccept": "Vorstellung übernehmen",
  "contact.intro.answerAcceptHelp": "Du übernimmst die Vorstellung selbst.",
  "contact.intro.answerNameDrop": "Namensnennung erlauben",
  "contact.intro.answerNameDropHelp":
    "Dein Teammitglied meldet sich direkt bei diesem Kontakt und nennt dich. Das wird nicht als Vorstellung erfasst.",
  "contact.intro.answerSuggest": "Jemand anderen vorschlagen",
  "contact.intro.answerSuggestHelp":
    "Nenne die Person im Team, die besser helfen kann.",
  "contact.intro.answerDecline": "Ablehnen",
  "contact.intro.answerDeclineHelp":
    "Die Anfrage wird geschlossen. Ergänze bei Bedarf einen Grund.",
  "contact.intro.asksTitle": "Vorstellungen",
  "contact.intro.answerAction": "Antworten",
  "contact.intro.completeIntroducedAction": "Als vorgestellt markieren",
  "contact.intro.completeNameDroppedAction": "Namensnennung markieren",
  "contact.intro.completeFailed":
    "Das Ergebnis wurde nicht gespeichert. Versuche es erneut.",
  "contact.intro.withdrawAction": "Zurückziehen",
  "contact.intro.withdrawFailed":
    "Die Anfrage wurde nicht zurückgezogen. Versuche es erneut.",
  "contact.intro.stateRequested": "Wartet auf Antwort",
  "contact.intro.stateAccepted": "Vorstellung zugesagt",
  "contact.intro.stateNameDropApproved": "Namensnennung erlaubt",
  "contact.intro.stateSuggestOther": "Anderer Weg",
  "contact.intro.handoffOtherSub": "Jemand anderes wurde vorgeschlagen",
  "contact.intro.stateDeclined": "Abgelehnt",
  "contact.intro.stateIntroduced": "Vorgestellt",
  "contact.intro.stateNameDropped": "Name genannt",
  "contact.intro.stateReplied": "Geantwortet",
  "contact.intro.stateExpired": "Abgelaufen",
  "contact.intro.handoffExpiredSub": "Keine rechtzeitige Antwort",
  "contact.intro.stateCancelled": "Zurückgezogen",
  "contact.intro.alreadyRequested": "Bereits angefragt",
  "contact.intro.declined": "Früher abgelehnt",
  "contact.intro.unavailable": "Nicht verfügbar",
  "contact.network.momentsTitle": "Letzte Veränderungen",
  "contact.network.momentsSub":
    "Veränderungen in dieser Beziehung, aus den Nachrichten.",
  "contact.network.noMoments": "Keine neuen Veränderungen in dieser Beziehung.",
  "contact.change.repliedAfterGap": "Antwort nach {days} ruhigen Tagen.",
  "contact.change.wentQuiet": "Seit {days} Tagen keine Aktivität.",
  "contact.change.warmed":
    "Die Beziehung hat sich von {from} auf {to} verändert.",
  "contact.change.cooled":
    "Die Beziehung hat sich von {from} auf {to} abgekühlt.",
  "contact.band.none": "kein Kontakt",
  "contact.band.weak": "schwach",
  "contact.band.moderate": "mittel",
  "contact.band.strong": "stark",
  "contact.bandBadge.none": "Kein Kontakt",
  "contact.bandBadge.weak": "Schwach",
  "contact.bandBadge.moderate": "Mittel",
  "contact.bandBadge.strong": "Stark",
  "contact.identity.title": "Identität",
  "contact.identity.emailDead":
    "Unzustellbar. E-Mails an diese Adresse werden nicht zugestellt.",
  "contact.identity.email": "E-Mail",
  "contact.identity.phone": "Telefon",
  "contact.identity.currentRole": "Aktuelle Rolle",
  "contact.identity.buyingRole": "Rolle im Kaufprozess",
  "contact.career.title": "Frühere Rollen",
  "contact.consent.title": "Einwilligung für ausgehende Nachrichten",
  "contact.consent.allowed": "Erlaubt: {purposes}",
  "contact.consent.noneGranted":
    "Für keinen Zweck ist eine Einwilligung erteilt, daher sind ausgehende Nachrichten blockiert.",
  "contact.consent.blocked": "Blockiert: {purposes}",
  "contact.network.title": "Teammitglieder, die diesen Kontakt kennen",

  "contact.page.loading": "Wird geladen…",
  "contact.page.notOpened": "Dieser Kontakt wurde nicht geöffnet.",
  "contact.page.buyingRole": "Rolle im Kaufprozess",
  "contact.page.owner": "Zuständig",
  "contact.page.ownerUnassigned": "Nicht zugewiesen",
  "contact.page.linkedin": "LinkedIn",

  "contact.rail.detailsTitle": "Details",
  "contact.rail.archivedReadOnly":
    "Dieser Kontakt ist archiviert und lässt keine Änderungen zu.",
  "contact.notYoursToChange":
    "Du kannst diesen Kontakt nicht bearbeiten. Frage die zuständige Person, ob sie ihn mit dir teilt, oder einen Admin nach Bearbeitungsrechten.",
  "contact.rail.employmentVersionUnresolved":
    "Die aktuelle Version dieser Zeile wurde nicht geladen. Lade die Seite neu und versuche es erneut.",
  "contact.rail.employmentTitle": "Unternehmen",
  "contact.rail.noEmployment": "Keine Anstellung erfasst.",
  "contact.rail.noPrimaryEmployer": "Kein Hauptarbeitgeber. Wähle einen aus.",
  "contact.rail.addEmployment": "Unternehmen hinzufügen",
  "contact.employer.contacts_one": "{count} Kontakt",
  "contact.employer.contacts_other": "{count} Kontakte",
  "contact.employer.openDeals_one": "{count} offener Deal",
  "contact.employer.openDeals_other": "{count} offene Deals",
  "contact.employer.noOpenDeals": "keine offenen Deals",
  "contact.rail.employer": "Arbeitgeber",
  "contact.rail.allCompaniesConnected":
    "Alle Treffer sind bereits mit diesem Kontakt verknüpft.",
  "contact.rail.isCurrentEmployer": "Das ist der aktuelle Arbeitgeber",
  "contact.rail.markEnded": "Als beendet markieren",
  "contact.rail.removeEmploymentTitle":
    "Verknüpfung zum Unternehmen entfernen?",
  "contact.rail.removeEmploymentBody":
    "Die Verknüpfung zu {company} und ihr Verlauf werden dauerhaft gelöscht. {company} selbst bleibt erhalten. Wenn der Kontakt das Unternehmen verlassen hat, markiere die Rolle stattdessen als beendet.",
  "contact.timeline.empty":
    "Noch keine Aktivitäten mit diesem Kontakt erfasst.",
  "contact.deals.empty": "Dieser Kontakt ist an keinem Deal beteiligt.",
  "contact.deals.untitled": "Deal ohne Titel",
  "contact.deals.noStage": "Noch keine Phase",
  "contact.meetings.upcoming": "Anstehend",
  "contact.meetings.past": "Vergangene Termine",
  "contact.meetings.noneBooked": "Keine anstehenden Termine.",
  "contact.meetings.noneLogged": "Keine Termine erfasst.",
  "contact.meetings.untitled": "Termin ohne Titel",
  "contact.documents.empty": "Keine Dateien zu diesem Kontakt.",
  "contact.research.empty": "Noch keine Recherche zu diesem Kontakt.",
  "contact.research.fields": "Belege der Anreicherung",
  "contact.research.fieldsEmpty":
    "Noch hat kein angereichertes Feld einen Beleg.",
  "contact.action.email": "E-Mail",
  "contact.action.write": "Schreiben",
  "contact.action.messageOn": "Nachricht über {transport}",
  "contact.action.noTransport":
    "Keine Adresse und kein Thread, auf den geantwortet werden kann.",
  "contact.action.call": "Anrufen",
  "contact.action.meetings": "Termine",
  "contact.action.addTask": "Aufgabe hinzufügen",
  "contact.action.research": "Recherche",

  "contact.strip.never": "Nie",
  "contact.strip.today": "Heute",
  "contact.strip.yesterday": "Gestern",
  "contact.strip.days": "{count} Tage",
  "contact.consent.allowedWord": "Erlaubt",
  "contact.consent.blockedWord": "Blockiert",
  "contact.consent.unknownWord": "Unbekannt",

  "contact.moment.rule.meeting_prep": "Termin steht bevor",
  "contact.moment.rule.re_engaged": "Wieder aktiv",
  "contact.moment.rule.job_change": "Stelle gewechselt",
  "contact.moment.rule.overdue_promise": "Zusage überfällig",
  "contact.moment.rule.gone_quiet": "Verstummt",
  "contact.moment.rule.open_promise": "Zusage fällig",
  "contact.moment.rule.public_signal": "In den Nachrichten",
  "contact.moment.rule.missing_next_step": "Kein nächster Schritt",
  "contact.moment.rule.thin_relationship": "Keine Interaktionen erfasst",
  "contact.moment.rule.nothing_needed": "Nichts zu tun",

  "contact.overview.detailsPermissions": "Details und Berechtigungen",
  "contact.overview.detailsShow": "Details und Berechtigungen einblenden",
  "contact.overview.detailsHide": "Details und Berechtigungen ausblenden",
  "contact.overview.partial":
    "Einige Bereiche sind für deine Rolle nicht verfügbar. Diese Zusammenfassung umfasst die Datensätze, die du sehen kannst.",
  "contact.overview.coverage":
    "Beruht auf den Datensätzen, die du sehen kannst.",
  "contact.overview.about": "Über diesen Kontakt",
  "contact.overview.profileOnly":
    "Beruht auf den erfassten Kontaktdaten. Ergänze Kontext, sobald du mehr erfährst.",
  "contact.overview.briefFailed": "Der Beziehungsbericht wurde nicht geladen.",

  "contact.brief.title": "Beziehungsbericht",
  "contact.brief.sources": "Quellen",
  "contact.brief.updatedAt": "Aktualisiert am {when}",
  "contact.brief.reading": "Beziehungsbericht wird geladen…",
  "contact.brief.sourceActivity": "Aktivität",
  "contact.brief.sourceDeal": "Deal-Notizen",

  "contact.matters.title": "Was {name} wichtig ist",
  "contact.matters.priorities": "Prioritäten",
  "contact.matters.objections": "Einwände",
  "contact.matters.successCriteria": "Erfolgskriterien",
  "contact.matters.absent": "Noch nichts erfasst",

  "contact.commercial.title": "Offener Deal und Rolle im Kaufprozess",
  "contact.commercial.withheld":
    "Du hast keinen Zugriff auf die Deals dieses Kontakts.",
  "contact.commercial.noDeal": "Kein offener Deal.",
  "contact.commercial.closes": "Abschluss {date}",
  "contact.commercial.committee": "Buying Center",
  "contact.commercial.openDeal": "Deal öffnen",

  "contact.loops.title": "Zusagen und offene Fragen",
  "contact.loops.empty":
    "Keine Zusagen oder Fragen in den erfassten Nachrichten.",
  "contact.loops.ours": "Du",
  "contact.loops.question": "Offene Frage",
  "contact.loops.overdue_one": "{count} Tag überfällig",
  "contact.loops.overdue_other": "{count} Tage überfällig",
  "contact.loops.overdueUnderDay": "seit weniger als einem Tag überfällig",
  "contact.loops.due": "fällig {when}",
  "contact.loops.dueToday": "heute",
  "contact.loops.dueTomorrow": "morgen",
  "contact.loops.dueInDays": "in {count} Tagen",
  "contact.loops.waiting": "Wartet",
  "contact.loops.openBadge": "Offen",

  "contact.memory.title": "Aktivitäten",
  "contact.memory.showAll": "Alle Aktivitäten anzeigen",
  "contact.memory.empty": "Auf diesem Kanal noch keine Aktivitäten erfasst.",
  "contact.memory.all": "Alle",
  "contact.memory.email": "E-Mail",
  "contact.memory.meetings": "Termine",
  "contact.memory.calls": "Anrufe",
  "contact.memory.notes": "Notizen",
  "contact.memory.channelEmail": "E-Mail",
  "contact.memory.channelMeeting": "Termin",
  "contact.memory.channelCall": "Anruf",
  "contact.memory.channelNote": "Notiz",
  "contact.memory.channelMessage": "Nachricht",
  "contact.memory.channelTask": "Aufgabe",
  "contact.memory.replied": "Beantwortet",
  "contact.memory.unanswered": "Unbeantwortet",

  "contact.rail.blocked": "Blockiert",
  "contact.rail.direction": "Richtung",
  "contact.rail.lastReply": "Letzte Antwort",
  "contact.rail.trend": "Trend",
  "contact.rail.twoWay": "Wechselseitig",
  "contact.rail.noDirection": "Keine Richtung erfasst",
  "contact.overview.unavailable": "Nicht angezeigt: {sections}.",
  "contact.rail.inboundOnly": "Nur eingehend",
  "contact.rail.outboundOnly": "Nur ausgehend",
  "contact.rail.coverage": "Abdeckung",
  "contact.rail.exchanges": "Austausche: {count}",
  "contact.rail.colleagues_one": "{count} Teammitglied",
  "contact.rail.colleagues_other": "{count} Teammitglieder",
  "contact.rail.noInbound": "Nichts eingehend",
  "contact.rail.cooling": "Kühlt ab",
  "contact.rail.warming": "Wird wärmer",
  "contact.rail.overall": "Gesamt",
  "contact.rail.thin": "Dünn",
  "contact.rail.atRisk": "Gefährdet",
  "contact.rail.strong": "Stark",
  "contact.standing.why.strong":
    "Der Kontakt hat in den letzten {days} Tagen geschrieben, daher gilt die Beziehung als stark.",
  "contact.standing.why.atRisk":
    "Seit über {days} Tagen keine Nachricht vom Kontakt, daher gilt die Beziehung als gefährdet.",
  "contact.standing.why.thin":
    "Der Kontakt hat noch nie geschrieben, daher gibt es noch keine Bewertung.",
  "contact.standing.trend.warming":
    "Wird wärmer: Die letzte Nachricht des Kontakts ist neuer als die deines Teams.",
  "contact.standing.trend.cooling":
    "Kühlt ab: Dein Team hat zuletzt geschrieben und wartet auf den Kontakt.",
  "contact.standing.direction.twoWay": "Beide Seiten haben geschrieben.",
  "contact.standing.direction.inboundOnly":
    "Bisher hat nur der Kontakt geschrieben. Dein Team hat noch nichts gesendet.",
  "contact.standing.direction.outboundOnly":
    "Bisher hat nur dein Team geschrieben. Noch keine Antwort.",
  "contact.standing.direction.none":
    "Noch keine Nachrichten, in keiner Richtung.",
  "contact.rail.consentTitle": "Kommunikationsberechtigungen",
  "contact.rail.email": "E-Mail",
  "contact.rail.phone": "Telefon",
  "contact.rail.noEmailAddress": "Keine Adresse erfasst",
  "contact.rail.noPhoneNumber": "Keine Nummer erfasst",
  "contact.rail.channelNotDeliverable": "Nicht zustellbar",
  "contact.drawer.close": "Schließen",
  "richtext.bold": "Fett",
  "richtext.italic": "Kursiv",
  "richtext.bulletList": "Aufzählung",
  "richtext.numberList": "Nummerierte Liste",
  "richtext.link": "Link",
  "richtext.linkPrompt": "Link-URL (leer lassen, um den Link zu entfernen)",
  "contact.composer.intentAgenda":
    "eine Agenda für den anstehenden Termin vorschlagen",
  "contact.composer.intentReply":
    "auf die letzte Nachricht des Kontakts antworten",
  "contact.composer.intentCommitment": "eine Zusage einlösen",
  "contact.composer.intentFollowUp": "nach einer ruhigen Phase nachfassen",
  "contact.research.title": "Tiefenrecherche · {name}",
  "contact.research.publicOnly": "Nur öffentliche Quellen",
  "contact.research.running": "Öffentliche Quellen werden gelesen…",
  // "Recherche-Anbieter", nicht "Datenanbieter": Letzteres ist das Wort für die
  // zugekauften Kontaktdaten (provider.profile.*), die direkt darüber stehen.
  "contact.research.notConnected":
    "Es ist kein Recherche-Anbieter verbunden, daher wurde für diesen Kontakt keine öffentliche Quelle gelesen. Das ist unabhängig von oben angezeigten, zugekauften Kontaktdaten. Margince recherchiert einen Kontakt nie aus eigener Befugnis, und eine Tiefenrecherche erfordert einen lizenzierten Anbieter, der die Rechtsgrundlage dafür hat.",
  "contact.research.staged":
    "Die Recherche ist vorgemerkt. Der Datensatz von {name} ändert sich erst, wenn du prüfst und speicherst.",
  "contact.research.stats":
    "{sources} Quellen gelesen · {claims} belegte Aussagen",
  "contact.research.dismiss": "Schließen",
  "contact.research.discard": "Verwerfen",
  "contact.research.save_one": "{count} Aussage prüfen und speichern",
  "contact.research.save_other": "{count} Aussagen prüfen und speichern",
  "contact.research.mapField": "Profilfeld",
  "contact.research.mapFieldPlaceholder": "Feld auswählen",
  "contact.research.mapValue": "Wert",
  "contact.research.mapQuote": "Quellenzitat",
  "contact.research.mapUrl": "Quellenlink",
  "contact.research.mapUrlInvalid": "Gib einen http- oder https-Link ein.",
  "contact.research.mapIncomplete":
    "Ergänze Wert, Zitat und Link, um diese Aussage zu speichern.",
  "contact.research.saved_one": "{count} Aussage zum Datensatz hinzugefügt",
  "contact.research.saved_other": "{count} Aussagen zum Datensatz hinzugefügt",
  "contact.research.field.title": "Position",
  "contact.research.field.role": "Rolle",
  "contact.research.field.company_name": "Unternehmen",
  "contact.research.field.phone": "Telefon",
  "contact.research.field.linkedin": "LinkedIn",
  "contact.research.field.address": "Adresse",
  "contact.research.field.website": "Website",
  "contact.research.evidenceOrOmit":
    "KI-gestützt · Aussagen ohne Beleg ausgelassen · nur öffentliche Informationen",
  "contact.meeting.title": "Terminbericht",
  "contact.meeting.brief": "Bericht erstellen",
  "contact.meeting.empty": "Zu diesem Termin ist noch nichts erfasst.",
  "contact.meeting.loading": "Bericht wird erstellt…",
  "contact.meeting.assembledNow": "Aus den neuesten Daten erstellt",
  "contact.meeting.header": "Auf einen Blick",
  "contact.meeting.what_changed": "Seit dem letzten Kontakt",
  "contact.meeting.goal": "Ziel dieses Termins",
  "contact.meeting.attendees": "Teilnehmende",
  "contact.meeting.commitments": "Offene Zusagen",
  "contact.meeting.deal_state": "Stand des Deals",
  "contact.meeting.risks": "Risiken und Warnsignale",
  "contact.meeting.talking_points": "Vorgeschlagene Gesprächspunkte",
  "contact.meeting.company_context": "Beim letzten Treffen",
  "contact.meeting.objective": "Angestrebtes Ergebnis",
  "contact.meeting.openWith": "Einstieg",
  "contact.meeting.arc": "Verlauf der Beziehung",
  "contact.meeting.arcSub":
    "Nur die Ereignisse, die für diesen Termin relevant sind.",
  "contact.meeting.close": "Termin abschließen",
  "contact.meeting.advance.minimum": "Mindestziel",
  "contact.meeting.advance.best": "Bestes Ziel",
  "contact.meeting.advance.fallback": "Rückfalloption",
  "contact.meeting.unknowns": "Lücken im Datensatz",
  "contact.meeting.likelyAsks": "Wahrscheinliche Fragen",
  "contact.meeting.beReady": "Vorbereitet sein auf",
  "contact.meeting.say": "Sagen",
  "contact.meeting.show": "Zeigen",
  "contact.meeting.avoid": "Vermeiden",
  "contact.meeting.scenarios": "Alternative Szenarien",
  "contact.meeting.relevance.high": "Wahrscheinlich",
  "contact.meeting.relevance.medium": "Möglich",
  "contact.meeting.relevance.low": "Weniger wahrscheinlich",
  "contact.meeting.coach.title": "Coaching-Schwerpunkt",
  "contact.meeting.coach.eyebrow": "Ansicht für Führungskräfte",
  "contact.meeting.coach.listenFor": "Achte auf",
  "contact.meeting.coach.watchFor": "Warnsignale",
  "contact.meeting.coach.interveneIf": "Nur eingreifen, wenn",
  "contact.meeting.coach.paths": "Mögliche Ergebnisse",
  "contact.meeting.background": "Hintergrund und Quellen",
  "contact.meeting.omittedSource": "Nicht in diesem Bericht",
  "contact.meeting.preparedFor": "Erstellt für {name}",
  "contact.meeting.preparedForAt": "Erstellt für {name} · {company}",

  "today.source.suggestions": "Vorschläge",

  // Der Datenanbieter (ADR-0101). Zwei Oberflächen teilen sich diese
  // Begriffe — die Einstellungskarte und die Personenseite —, damit ein
  // Zustand überall gleich heißt.
  "today.scan.queued": "Margince analysiert dieses Unternehmen in Kürze.",
  "today.scan.reading":
    "Margince analysiert die Kommunikation und die Deals dieses Unternehmens.",
  "today.scan.read": "Analysiert: {exchanges} und {deals}",
  "today.scan.readExchanges_one": "{count} Nachricht",
  "today.scan.readExchanges_other": "{count} Nachrichten",
  "today.scan.readDeals_one": "{count} Deal",
  "today.scan.readDeals_other": "{count} Deals",
  "today.scan.stale":
    "Das Unternehmen hat sich seitdem geändert. Es wird innerhalb einer Stunde erneut analysiert.",
  "today.scan.resumes":
    "Die Analyse wird am {when} fortgesetzt. Wegen des KI-Kontingents pausiert.",
  "provider.readOnly":
    "Nur Lesezugriff: Einen Anbieter zu verbinden kostet Geld, daher können das nur Admins und Operations.",
  "provider.title": "Kontaktdaten",
  "provider.sub":
    "Kaufe verifizierte Kontaktdaten für deine Kontakte. Der Anbieter berechnet Credits; die Ausgaben stehen unten.",
  "provider.notConfigured":
    "In dieser Installation ist kein Datenanbieter verfügbar, daher kann nichts gekauft werden.",
  "provider.status.connected": "Verbunden",
  "provider.status.disconnected": "Nicht verbunden",
  "provider.status.validating": "API-Schlüssel wird geprüft…",
  "provider.status.invalidCredentials": "API-Schlüssel abgelehnt",
  "provider.status.insufficientCredits": "Keine Credits mehr",
  "provider.status.rateLimited": "Anfragen gedrosselt",
  "provider.status.providerError": "Fehler beim Anbieter",
  "provider.connect": "Verbinden",
  "provider.reconnect": "API-Schlüssel ersetzen",
  "provider.apiKey": "API-Schlüssel",
  "provider.apiKeyStored": "API-Schlüssel ersetzen",
  "provider.apiKeyReplaceHint":
    "Ein Schlüssel ist gespeichert und in Verwendung. Er kann nicht erneut angezeigt werden; füge nur dann einen neuen ein, wenn du ihn ersetzen willst.",
  "provider.apiKeyReplacePlaceholder":
    "Neuen Schlüssel einfügen, um den gespeicherten zu ersetzen",
  "provider.apiKeyHint":
    "Wird nach der Prüfung im Schlüsseltresor gespeichert. Er wird nie wieder angezeigt und verlässt diese Installation nur in Richtung Anbieter.",
  "provider.connectConfirm.title": "Diesen Datenanbieter verbinden?",
  "provider.connectConfirm.body":
    "Der Schlüssel wird beim Anbieter geprüft, bevor er gespeichert wird. Nach dem Verbinden kostet jede Anreicherung eines Kontakts Credits.",
  "provider.disconnect": "Trennen",
  "provider.disconnectConfirm.title": "Diesen Anbieter trennen?",
  "provider.disconnectConfirm.body":
    "Neue Abfragen stoppen sofort, und der Schlüssel wird vernichtet. Gekaufte Daten bleiben an deinen Datensätzen; das Trennen löscht sie nicht.",
  "provider.deleteData": "Gekaufte Daten löschen",
  "provider.deleteDataConfirm.title": "Alle Daten dieses Anbieters löschen?",
  "provider.deleteDataConfirm.body":
    "Jeder Wert, den dieser Anbieter geliefert hat, wird von jedem Kontakt entfernt. Die Ausgabenaufzeichnungen bleiben, die Daten nicht. Das lässt sich nicht rückgängig machen.",
  "provider.deleteDataConfirm.typed":
    "Zum Bestätigen den Namen des Anbieters eingeben",
  "provider.automaticLookup": "Kontakte automatisch nachschlagen",
  "provider.automaticLookupHint":
    "Jeder Kontakt wird einmal nach den Angaben nachgeschlagen, die diese Verbindung auswählt und für die der Anbieter nichts berechnet, in der Regel Link zum beruflichen Profil, aktuelle Rolle und Arbeitgeber sowie Werdegang. E-Mail-Adressen und Mobilnummern werden so nie gekauft; sie kosten Credits und bleiben eine Entscheidung pro Kontakt.",
  "provider.automaticLookupJurisdiction":
    "Schalte das aus, wenn die Kontakte in deinem CRM einem Recht unterliegen, das den Handel mit personenbezogenen Daten verbietet, etwa dem vietnamesischen. Die Schaltfläche an jedem Kontakt funktioniert weiterhin, damit die Entscheidung bei denen bleibt, die sie treffen.",
  "provider.buyable": "Kaufen erlauben: {category}",
  "provider.buyableWriteFailed": "Kategorie nicht geändert",
  "provider.postureReadFailed": "Aktuelle Einstellung unbekannt",
  "provider.postureWriteFailed": "Automatisches Nachschlagen nicht geändert",
  "provider.buyableHint_one":
    "Das Einschalten kauft nichts. Es fügt an jedem Kontakt eine Schaltfläche hinzu, mit der sich diese Angabe für {credits} Credit einzeln pro Kontakt kaufen lässt.",
  "provider.buyableHint_other":
    "Das Einschalten kauft nichts. Es fügt an jedem Kontakt eine Schaltfläche hinzu, mit der sich diese Angabe für {credits} Credits einzeln pro Kontakt kaufen lässt.",
  "provider.buyableNeeds":
    "Der Anbieter sucht danach nur zusammen mit der Angabe „{prerequisite}“, daher lässt sie sich nicht einzeln kaufen. Erlaube diese Angabe zuerst.",
  "provider.backlog": "Noch nachzuschlagen",
  "provider.backlogRemaining_one": "{count} Kontakt",
  "provider.backlogRemaining_other": "{count} Kontakte",
  "provider.backlogWorking":
    "Kontakte, die beim Verbinden des Anbieters schon existierten, werden nach und nach nachgeschlagen.",
  "provider.backlogPaused":
    "Es laufen keine Abfragen: Automatische Abfragen sind aus, das Tageslimit ist erreicht, oder der Anbieter ist nicht verfügbar.",
  "provider.credits": "Credit-Guthaben beim Anbieter",
  "provider.credits.none": "Der Anbieter hat noch keinen Stand gemeldet.",
  "provider.credits.notConnected":
    "Verbinde einen API-Schlüssel, um das Credit-Guthaben beim Anbieter zu sehen.",
  "provider.constraints": "Geltende Limits",
  "provider.spend": "Verbrauchte Credits",
  "provider.spend.hint":
    "Eigene Aufzeichnung von Margince zu den Anreicherungskosten, nicht die Rechnung des Anbieters. Credits lassen sich auch in der App des Anbieters ausgeben, daher können die Zahlen abweichen.",
  "provider.spend.thisMonth": "Diesen Monat",
  "provider.spend.month": "Monat",
  "provider.spend.pool": "Pool",
  "provider.spend.chargedHead": "Credits",
  "provider.spend.heldHead": "Reserviert",
  "provider.spend.runsHead": "Abfragen",
  "provider.spend.none": "Noch nichts gekauft.",

  // Der Abschnitt auf der Personenseite. Die drei „nichts da"-Zustände sind
  // mit Absicht drei verschiedene Sätze: nur bei einem davon kann der Leser
  // etwas tun.
  "provider.profile.title": "Gekaufte Kontaktdaten",
  "provider.profile.notConnected":
    "Kein Datenanbieter verbunden, daher wurde nichts gekauft.",
  "provider.profile.notEligible":
    "Dieser Kontakt ist nicht zulässig: Der Kontakt hat widersprochen, oder der Datensatz ist archiviert.",
  "provider.profile.nothingToLookUp":
    "Keine Kennung, um diesen Kontakt nachzuschlagen. Füge die LinkedIn-URL oder den Arbeitgeber hinzu, um die Abfrage auszuführen.",
  "provider.profile.neverRun":
    "Dieser Kontakt wurde noch nicht nachgeschlagen.",
  "provider.profile.queued": "Eingereiht",
  "provider.profile.inProgress": "Wird nachgeschlagen…",
  "provider.profile.workingTitle": "{provider} wird abgefragt",
  "provider.profile.working": "Das dauert bis zu einer Minute.",
  "provider.profile.landingTitle": "Antwort erhalten",
  "provider.profile.landing": "Wird im Datensatz gespeichert…",
  "provider.profile.lookupRefused": "Abfrage nicht ausgeführt",
  "provider.profile.completed": "Gefunden",
  "provider.profile.noMatch": "Der Anbieter hat keine Daten zu diesem Kontakt.",
  "provider.profile.stale":
    "Früher gekauft. Der Anbieter ist nicht mehr verbunden, daher lässt sich das nicht aktualisieren.",
  "provider.profile.invalidCredentials":
    "Der Anbieter hat den API-Schlüssel abgelehnt, daher wurde die Abfrage nicht ausgeführt.",
  "provider.profile.insufficientCredits":
    "Nicht gekauft: Das Credit-Limit für diesen Monat ist ausgeschöpft.",
  "provider.profile.rateLimited":
    "Nicht gekauft: Der Anbieter drosselt Anfragen.",
  "provider.profile.providerError":
    "Die letzte Abfrage ist fehlgeschlagen. Versuche es erneut oder prüfe die Karte des Anbieters in den Einstellungen, falls der Fehler bleibt.",
  "provider.profile.submissionUnknown":
    "Das Ergebnis dieser Abfrage ist unbekannt. Möglicherweise wurden Credits verbraucht.",
  "provider.profile.claimsUnwritten":
    "Bezahlt, aber die Angaben sind nicht in diesem Datensatz angekommen.",
  "provider.profile.enrichNow": "Kontakt nachschlagen · kostenlos",
  "provider.profile.recheck": "Erneut prüfen · kostenlos",
  "provider.profile.lookingUp": "Anbieter wird abgefragt…",
  "provider.profile.emptyTitle": "Keine Daten für diesen Kontakt gekauft",
  "provider.profile.emptyBody":
    "Eine Abfrage holt bei {provider} die Angaben, die diese Verbindung kauft, und verbraucht {provider}-Credits. Ergebnisse erscheinen neben dem Datensatz und überschreiben nie Einträge aus dem Team.",
  "provider.profile.emails": "E-Mail-Adressen",
  "provider.profile.emailType.provider": "{type}, laut Anbieter",
  "provider.profile.emailType.requested": "{type}, wie angefragt",
  "provider.profile.mobiles": "Mobilnummern",
  "provider.profile.confidence": "{percent} % Sicherheit",
  "provider.profile.linkedin": "LinkedIn",
  "provider.profile.employment": "Aktuelle Rolle",
  "provider.profile.jobHistory": "Frühere Rollen",
  "provider.profile.location": "Standort",
  "provider.profile.departments": "Abteilungen",
  "provider.profile.seniorities": "Hierarchieebene",
  "provider.profile.notRequested": "Nicht angefragt: {categories}.",
  "provider.profile.buy_one": "{category} kaufen · {credits} Credit",
  "provider.profile.buy_other": "{category} kaufen · {credits} Credits",
  "provider.profile.buyRebuys":
    "Der Preis enthält erneut: {categories}. Der Anbieter verlangt das für diese Suche und berechnet alles, was er zurückliefert.",
  "provider.freeTier.hint":
    "LinkedIn-Profil, aktuelle Rolle und Werdegang kosten keine Credits. Lass das eingeschaltet, damit jeder neue Kontakt sie automatisch erhält.",
  "provider.pricedTier.hint":
    "Wird nie automatisch gekauft, sondern einzeln pro Kontakt, mit dem Preis auf der Schaltfläche.",
  "provider.profile.receiptAt": "Abgefragt am {at}.",
  "provider.profile.receipt":
    "Abgefragt am {at} · {answered} von {asked} Angaben geliefert.",
  "provider.profile.noAnswer": "Angefragt, nicht gefunden: {categories}.",
  "provider.category.professionalEmail": "geschäftliche E-Mail",
  "provider.category.personalEmail": "private E-Mail",
  "provider.category.mobile": "Mobilnummer",
  "provider.category.linkedin": "LinkedIn-Profil",
  "provider.category.currentEmployment": "aktuelle Rolle",
  "provider.category.jobHistory": "frühere Rollen",

  // Der Filter-Baukasten (AC-filters-and-views-3/4).
  "filters.joinAll": "Alle (UND)",
  "filters.joinAny": "Mindestens eine (ODER)",
  "filters.joinLabel": "Verknüpfungsmodus",
  "filters.removeGroup": "Gruppe entfernen",
  "filters.addGroup": "Gruppe hinzuf\u00fcgen",
  "filters.addClause": "Bedingung hinzuf\u00fcgen",
  "filters.emptyGroup":
    "Noch keine Bedingungen. Eine leere Gruppe trifft auf nichts zu; füge eine Bedingung hinzu.",
  "filters.field": "Feld",
  "filters.field.classification": "Klassifizierung",
  "filters.field.company_industry": "Branche des Unternehmens",
  "filters.field.company_lifecycle": "Lebenszyklus des Unternehmens",
  "filters.field.company_size_band": "Größe des Unternehmens",
  "filters.field.hosting_provider": "Hosting",
  "filters.field.mail_provider": "E-Mail-System",
  "filters.field.operated_service": "Betriebener Dienst",
  "filters.field.owner_team_id": "Zuständiges Team",
  "filters.field.phase": "Projektphase",
  "filters.field.pipeline_id": "Pipeline",
  "filters.field.relationship_type": "Beziehungstyp",
  "filters.field.stage_id": "Phase",
  "filters.field.tag": "Tag",
  "filters.field.technology": "Technologie",
  "filters.choosePlaceholder": "Feld ausw\u00e4hlen",
  "filters.customBadge": "Eigenes Feld",
  "filters.operator": "Operator",
  "filters.value": "Wert",
  "filters.values": "Werte",
  "filters.addValue": "Wert hinzuf\u00fcgen",
  "filters.removeClause": "Bedingung {field} entfernen",
  "filters.existsLabel": "Feld hat einen Wert",
  "filters.hasValue": "hat einen Wert",
  "filters.isEmpty": "ist leer",
  "filters.yes": "ja",
  "filters.no": "nein",
  "filters.op.eq": "ist",
  "filters.op.neq": "ist nicht",
  "filters.op.in": "ist eines von",
  "filters.op.contains": "enth\u00e4lt",
  "filters.op.exists": "hat einen Wert",
  "filters.op.afterDate": "ist nach",
  "filters.op.onOrAfterDate": "ist am oder nach",
  "filters.op.beforeDate": "ist vor",
  "filters.op.onOrBeforeDate": "ist am oder vor",
  "filters.op.moreThan": "ist gr\u00f6\u00dfer als",
  "filters.op.atLeast": "ist mindestens",
  "filters.op.lessThan": "ist kleiner als",
  "filters.op.atMost": "ist h\u00f6chstens",

  // Die Oberfl\u00e4che \u201eFilter & Ansichten\u201c.
  "filters.title": "Filter und Ansichten",
  "filters.subtitle":
    "Filter erstellen, Treffer in der Vorschau prüfen und als Ansicht speichern.",
  "filters.objectLabel": "Datensatztyp",
  "filters.tab.contacts": "Kontakte",
  "filters.tab.companies": "Unternehmen",
  "filters.tab.deals": "Deals",
  "filters.builderTitle": "Filter",
  "filters.dynamic": "Dynamisch: wird bei jedem Ereignis aktualisiert",
  "filters.matchContacts": "Passende Kontakte: {count}",
  "filters.matchCompanies": "Passende Unternehmen: {count}",
  "filters.matchDeals": "Passende Deals: {count}",
  "filters.noFilterYet": "Bedingung hinzufügen, um Treffer anzuzeigen",
  "filters.countUnavailable": "Anzahl nicht verf\u00fcgbar",
  "filters.loadingVocabulary": "Felder werden geladen…",
  "filters.noFields": "Keine filterbaren Felder f\u00fcr diesen Datensatztyp.",
  "filters.resultsTitle": "Passende Datens\u00e4tze",
  "filters.resultsCaption":
    "Erste Seite der Treffer, zum Prüfen des Filters. Nicht die vollständige Auswahl.",
  "filters.noMatches": "Keine Datensätze passen zu diesem Filter.",
  "filters.loadView": "Gespeicherten Filter laden",
  "filters.pickRecord": "Datensatz auswählen",
  "filters.searchRecords": "Unternehmen durchsuchen",
  "filters.typeToSearch": "Zum Suchen tippen",
  "filters.searching": "Wird gesucht…",
  "filters.searchFailed": "Suche fehlgeschlagen",
  "filters.noRecordMatches": "Keine passenden Unternehmen",
  "filters.changeRecord": "Ändern",
  "filters.removeRecord": "{record} entfernen",
  "filters.loadingRecords": "Auswahl wird geladen…",
  "filters.pickValue": "Wert auswählen",
  "filters.exportCsv": "CSV exportieren",
  "filters.exportJson": "JSON exportieren",

  // Siehe en.ts: zwei Leser gleichzeitig — wer hinein will, und wer es
  // reparieren muss.
  "release.skewTitle": "App- und Serverversion unterscheiden sich",
  "release.skewBody":
    "Die App im Browser und der Server laufen mit unterschiedlichen Releases, daher ist diese Seite nicht verlässlich. Lade die Seite neu, um die aktuelle Version zu erhalten. Bleibt die Meldung, bringe alle Komponenten auf dasselbe Release.",
  "release.skewVersions": "App {app} · Server {server}",
  "release.skewReload": "Neu laden",

  // Die Warteschlange hinter „später senden“. „Zurückziehen“ statt „löschen“
  // oder „abbrechen“: es wurde nichts übertragen und nichts erscheint in der
  // Chronik, also gibt es keinen Versand abzubrechen und keinen Datensatz zu
  // löschen — die Nachricht wird zurückgenommen, bevor sie geht.
  "nav.scheduled": "Geplante Nachrichten",
  "sched.sub":
    "Deine geplanten Nachrichten, die noch nicht gesendet wurden. Nur du kannst sie sehen.",
  "sched.empty": "Noch keine geplanten Nachrichten.",
  "sched.group.held": "Zurückgehalten, wartet auf dich",
  "sched.group.heldEmpty": "Keine zurückgehaltenen Nachrichten.",
  "sched.group.waiting": "Geplant",
  "sched.group.waitingEmpty": "Keine Nachrichten geplant.",
  "sched.group.closed": "Gesendet oder zurückgezogen",
  "sched.group.closedEmpty": "Noch nichts gesendet oder zurückgezogen.",
  "sched.status.scheduled": "Geplant",
  "sched.status.released": "Wird gesendet",
  "sched.status.sent": "Gesendet",
  "sched.status.cancelled": "Zurückgezogen",
  "sched.status.held": "Zurückgehalten",
  "sched.held.consentWithdrawn":
    "Jemand unter den Empfangenden hat die Einwilligung widerrufen, nachdem du diese Nachricht geplant hast. Die Nachricht wird erst gesendet, wenn du an diese Adresse unter einem Zweck schreibst, für den ihre Einwilligung vorliegt.",
  "sched.held.senderInactive":
    "Dein Platz oder dein Postfach hat sich nach der Planung geändert, deshalb kann die Nachricht nicht in deinem Namen gesendet werden.",
  "sched.held.missedWindow":
    "Der Sendezeitpunkt ist verstrichen, während das System nicht lief, und für den Versand ist es jetzt zu spät. Plane die Nachricht neu oder ziehe sie zurück.",
  "sched.held.timerExhausted":
    "Der Sendeauftrag hat keine Versuche mehr. Plane die Nachricht neu, um es erneut zu versuchen.",
  "sched.held.sendRefused":
    "Eine Prüfung hat die Nachricht bei Fälligkeit blockiert. Es wurde nichts gesendet.",
  "sched.inZone": "in {zone}",
  "sched.recipientsUnknown": "Keine Empfangsadresse in dieser Nachricht",
  "sched.recipientsMore": "{first} und {count} weitere",
  "sched.move": "Neu planen",
  "sched.moveTo": "Neuer Zeitpunkt für „{subject}“",
  "sched.moveSave": "Neu planen",
  "sched.moveCancel": "Abbrechen",
  "sched.withdraw": "Zurückziehen",
  "sched.withdrawTitle": "Nachricht zurückziehen?",
  "sched.withdrawBody":
    "„{subject}“ wird nicht gesendet, und im Verlauf erscheint nichts. Für einen späteren Versand musst du sie neu schreiben.",
  "sched.withdrawConfirm": "Nachricht zurückziehen",
  "sched.skewTitle": "Liste nicht mehr aktuell",
  "sched.skew":
    "Die Nachricht wurde bereits gesendet, zurückgezogen oder verschoben. Lade die Liste neu.",
  "sched.writeFailed": "Änderung nicht gespeichert",
  "sched.reload": "Neu laden",
  "nav.projects": "Projekte",
  "unit.projects": "Projekte",
  "companyProjects.title": "Projekte",
  "companyProjects.empty":
    "Noch keine Projekte. Ein Unternehmen erscheint hier, sobald es als Auftraggeber, Partner oder Subunternehmen an einem Projekt beteiligt ist.",
  "projectCompanies.title": "Unternehmen",
  "projectCompanies.empty":
    "Noch keine Unternehmen. Ein Projekt kann den Kunden und alle Partner oder Subunternehmen umfassen, die es umsetzen.",
  "projectCompanies.attach": "Unternehmen verknüpfen",
  "projectCompanies.detachTitle": "Unternehmen aus dem Projekt entfernen?",
  "projectCompanies.detachConfirm": "Unternehmen entfernen",
  "projectCompanies.searchLabel": "Unternehmen nach Name suchen",
  "contactProjects.title": "Projekte",
  "contactProjects.empty":
    "Noch keine Projekte. Dieser Kontakt erscheint hier, sobald er einem Projekt in einer beliebigen Rolle hinzugefügt wurde.",
  "projectRole.customer": "Kunde",
  "projectRole.partner": "Partner",
  "projectRole.subcontractor": "Subunternehmen",
  "contactRole.sponsor": "Sponsor",
  "contactRole.projectLead": "Projektleitung",
  "contactRole.deliveryLead": "Umsetzungsleitung",
  "contactRole.expert": "Fachexpertise",
  "contactRole.user": "Nutzende",
  "projectLinks.attach": "Projekt verknüpfen",
  "projectLinks.move": "Zu anderem Projekt verschieben",
  "projectLinks.detach": "Verknüpfung lösen",
  "projectLinks.detachConfirm": "Projektverknüpfung lösen",
  "projectLinks.detachNamed": "Verknüpfung zu {name} lösen",
  "projectLinks.roleLabel": "Als",
  "projectLinks.detachTitle": "Verknüpfung zu diesem Projekt lösen?",
  "projectLinks.detachBody":
    "{name} bleibt unverändert. Nur die Verknüpfung mit diesem Datensatz endet; gelöscht wird nichts.",
  "projectLinks.emptyTitle": "Noch keine Projekte",
  "projectLinks.searchLabel": "Projekte nach Name oder Kürzel suchen",
  "project.name": "Projektname",
  "projectHealth.title": "Zustand der Umsetzung",
  "projectHealth.empty": "Noch keine Bewertung",
  "projectHealth.emptyDetail":
    "Halte fest, wie die Umsetzung läuft, damit die nächsten Lesenden den Status kennen.",
  "projectHealth.assessedOn": "Stand {date}",
  "projectHealth.corrected": "(korrigiert)",
  "projectHealth.state.onTrack": "Im Plan",
  "projectHealth.state.atRisk": "Gef\u00e4hrdet",
  "projectHealth.state.offTrack": "Außer Plan",
  "projectHealth.record": "Bewertung erfassen",
  "projectHealth.correct": "Bewertung korrigieren",
  "projectHealth.correctOne": "Bewertung vom {date} korrigieren",
  "projectHealth.recordTitle": "Zustand der Umsetzung erfassen",
  "projectHealth.correctTitle": "Diese Bewertung korrigieren",
  "projectHealth.stateLabel": "Zustand",
  "projectHealth.note": "Notiz",
  "projectHealth.noteOptionalHint":
    "Optional, solange das Projekt im Plan ist.",
  "projectHealth.noteRequiredHint":
    "Beschreibe das Problem, damit die nächsten Lesenden den Kontext haben.",
  "projectHealth.correctionNote":
    "Eine Korrektur ändert, was erfasst wurde, nie wann. Die Bewertung behält ihr ursprüngliches Datum.",
  "projectHealth.saveReading": "Bewertung erfassen",
  "projectHealth.saveCorrection": "Korrektur speichern",
  "project.keyMinted":
    "Jedes Projekt erhält ein kurzes Kürzel. Setze [{key}] in den Betreff einer E-Mail, um sie unter diesem Projekt abzulegen.",
  "project.company": "Unternehmen",
  "project.owner": "Zuständig",
  "project.ownerKeep": "Aktuelle Zuständigkeit behalten",
  "project.ownerMe": "Mir zuweisen",
  "project.ownerUnassign": "Zuweisung aufheben",
  "project.assignOwner": "Einem Teammitglied zuweisen",
  "project.assignOwnerTitle": "Einem Teammitglied zuweisen",
  "project.assignOwnerSearch": "Teammitglieder suchen",
  "project.assignOwnerNoneSelected": "Wähle zuerst ein Teammitglied",
  "project.assignOwnerConfirm": "Zuweisen",
  "project.assignOwnerDone": "{name} zugewiesen",
  "project.description": "Beschreibung",
  "project.targetEnd": "Geplantes Enddatum",
  "project.targetEnd.inDays_one": "in {days} Tag",
  "project.targetEnd.inDays_other": "in {days} Tagen",
  "project.targetEnd.overdue_one": "{days} Tag überfällig",
  "project.targetEnd.overdue_other": "{days} Tage überfällig",
  "project.new": "Neues Projekt",
  "project.edit": "Projekt bearbeiten",
  "project.archive": "Projekt archivieren",
  "project.archiveConfirm":
    "Das Archivieren entfernt dieses Projekt aus der aktiven Liste und gibt sein Kürzel frei. Das lässt sich hier nicht rückgängig machen.",
  "project.archivedReadOnly":
    "Dieses Projekt ist archiviert und lässt keine Änderungen zu.",
  "project.notYoursToChange":
    "Du kannst dieses Projekt nicht bearbeiten. Frage die zuständige Person, ob sie es mit dir teilt, oder einen Admin nach Bearbeitungsrechten.",
  "project.phaseLabel": "Phase",
  "project.filterPhaseAll": "Alle Phasen",
  "project.viewDelivering": "In Umsetzung",
  "project.phase.initiative": "Initiative",
  "project.phase.pursuing": "Im Vertrieb",
  "project.phase.delivering": "In Umsetzung",
  "project.phase.closed": "Abgeschlossen",
  "project.emptyTitle": "Noch keine Projekte",
  "project.emptyBody":
    "Ein Projekt ist die Arbeit, um die es in einem Deal geht. Es beginnt während des Deals in der Phase Initiative und läuft weiter, wenn der Deal gewonnen ist; dann wird hier die Umsetzung verfolgt.",
  "project.emptyKey":
    "Jedes Projekt erhält ein kurzes Kürzel. E-Mails mit dem Kürzel in eckigen Klammern im Betreff werden automatisch unter dem Projekt abgelegt.",
  "project.rollups.empty": "Noch keine Kennzahlen zu diesem Projekt.",
  "project.rollups.openValue": "Offene Deals",
  "project.rollups.wonValue": "Gewonnene Deals",
  "project.rollups.noneOpen": "Keine",
  "project.rollups.noneWon": "Noch keine",
  "project.rollups.openCommitments": "Offene Zusagen",
  "project.rollups.never": "Keine",
  "project.rollups.activityCount": "Aktivität",
  "project.rollups.activityFiled": "{count} abgelegt",
  "project.rollups.activityLast": "Zuletzt · {date}",
  "project.history.title": "Phasenverlauf",
  "project.history.empty": "Noch kein Phasenwechsel erfasst.",
  "project.history.current": "aktuell",
  "project.history.moved": "{from} → {to}",
  "project.history.born": "Gestartet in der Phase „{phase}“",
  "project.history.bySystem": "System",
  "project.deals.title": "Deals",
  "project.deals.empty":
    "Noch kein Deal ist mit diesem Projekt verknüpft. Ein Deal wählt sein Projekt im eigenen Formular.",
  "project.deals.more":
    "Es gibt mehr Deals als angezeigt. Öffne Deals, um alle zu sehen.",
  "project.stakeholders.title": "Stakeholder",
  "project.stakeholders.empty":
    "Noch keine Stakeholder. Ein Stakeholder ist ein Kontakt mit einer Rolle in diesem Projekt, zum Beispiel Sponsor, Projektleitung oder Champion.",
  "project.stakeholders.add": "Stakeholder hinzufügen",
  "project.stakeholders.addConfirm": "Hinzufügen",
  "project.stakeholders.addHint":
    "Eine Rolle pro Kontakt. Wird ein Kontakt hinzugefügt, der schon am Projekt beteiligt ist, wechselt seine Rolle auf die hier gewählte.",
  "project.stakeholders.searchLabel": "Kontakte nach Name suchen",
  "project.stakeholders.removeTitle": "Stakeholder entfernen?",
  "project.stakeholders.removeConfirm":
    "{name} ist nicht mehr Stakeholder dieses Projekts. Die Aktivitäten bleiben, wo sie sind.",
  "project.stakeholders.removeOne": "{name} aus dem Projekt entfernen",
  "project.role.sponsor": "Sponsor",
  "project.role.project_lead": "Projektleitung",
  "project.role.delivery_lead": "Umsetzungsleitung",
  "project.role.subject_matter_expert": "Fachexpertise",
  "project.contracts.title": "Verträge",
  "project.contracts.empty":
    "Unter diesem Projekt ist kein Vertrag abgelegt. Ein Vertrag nennt sein Projekt, wenn er erfasst wird.",
  "project.documents.title": "Dokumente",
  "project.documents.empty":
    "An diesem Projekt hängt keine Datei. Dateien an seinen Deals bleiben bei den Deals.",
  "project.commitments.title": "Offene Zusagen",
  "project.commitments.empty":
    "Unter diesem Projekt ist keine offene Aufgabe abgelegt. Verknüpfte Aufgaben erscheinen hier, die am frühesten fällige zuerst.",
  "project.commitments.overdue": "Überfällig",
  "project.timeline.empty":
    "Unter diesem Projekt ist noch nichts abgelegt. E-Mails mit dem Kürzel im Betreff und verknüpfte Aktivitäten erscheinen hier.",
  "project.advance.title": "Wechsel zu {phase}",
  "project.advance.confirm": "Wechseln",
  "project.advance.close": "Projekt abschließen",
  "project.advance.body":
    "Der Wechsel wird mit deiner Begründung im Phasenverlauf erfasst.",
  "project.advance.closeBody":
    "Der Abschluss beendet die Umsetzung des Projekts. Es kann später wieder geöffnet werden, und die Begründung bleibt erfasst.",
  "project.advance.reason": "Begründung",
  "project.advance.reasonRequired":
    "Ein abgeschlossenes Projekt braucht eine Begründung.",
  "deal.project": "Projekt",
  "deal.projectNew": "Neues Projekt…",
  "deal.projectWithheld": "Projekt ausgeblendet",
  "deal.projectNeedsCompany":
    "Wähle zuerst das Unternehmen des Deals. Ein Projekt gehört zu einem Unternehmen.",
  "deal.projectUnnamed": "Projekt",
  "deal.startDeliveryTitle": "Umsetzung starten",
  "deal.startDelivery": "Umsetzung starten",
  "deal.startDeliveryFailed": "Umsetzung nicht gestartet",
  "deal.startDeliveryAttached":
    "Dieser Deal ist mit {project} verknüpft, aber das Projekt ist noch nicht in Umsetzung. Jetzt verschieben?",
  "deal.startDeliveryBody":
    "Dieser Deal ist gewonnen und nennt kein Projekt. Mit {project} verknüpfen und das Projekt in die Umsetzung verschieben?",

  // The Worklist's own words: the ranked queue, its dials, and the phrase
  // for every fact the server sends as a closed vocabulary.
  "worklist.loading": "Worklist wird geladen…",
  "worklist.queue": "Heute",
  "worklist.review": "Zu prüfen",
  "worklist.more": "Mehr laden",
  "worklist.more.where":
    "Mehr vom Tag: Neue Einträge können unter „{today}“ oder „{review}“ erscheinen.",
  "worklist.more.failed":
    "Weitere Einträge wurden nicht geladen. Versuche es erneut.",
  "worklist.summary":
    "{urgent} dringend · {due} fällig · {inPlay} in Arbeit · {lower} Routine · {total} gesamt",
  "worklist.summary.noMiddle":
    "{urgent} dringend · {due} fällig · {lower} Routine · {total} gesamt",
  "worklist.summary.split": "{today} heute · {review} zu prüfen",
  "worklist.completeness": "{shown} von {considered} angezeigt",
  "worklist.review.partial":
    "{loaded} von {total} angezeigt. Lade unten mehr, um den Rest zu sehen.",
  "worklist.review.partialDone": "{loaded} von {total} angezeigt.",
  "worklist.completeness.bounded_one":
    "{shown} angezeigt · {sources} Quelle hat mehr",
  "worklist.completeness.bounded_other":
    "{shown} angezeigt · {sources} Quellen haben mehr",
  "worklist.clear": "Nichts wartet auf dich.",
  "worklist.clearFor": "Nichts wartet auf {name}.",
  "worklist.clearOfTasksToday":
    "Keine Aufgaben heute fällig oder überfällig. Spätere Aufgaben stehen im Tab „Aufgaben“ des jeweiligen Datensatzes.",
  "worklist.clearOfWhatWasRead": "Keine Einträge in den geladenen Quellen.",
  "worklist.partialTitle": "Worklist unvollständig",
  "worklist.partial": "{sources}.",
  "worklist.overdue": "Überfällig",
  "worklist.pair.ask": "Welcher Datensatz soll bleiben?",
  "worklist.pair.keep": "{name} behalten",
  "worklist.pair.notDuplicate": "Keine Duplikate",
  "worklist.pair.mergeBlocked":
    "Diese Datensätze lassen sich nicht zusammenführen: Beide haben laufende Projekte, und nichts zeigt, welche Arbeit wohin gehört. Du kannst trotzdem festhalten, dass es keine Duplikate sind.",
  "worklist.pair.related": "{count} verknüpft",
  "worklist.pair.failed": "Das Paar wurde nicht geklärt. Versuche es erneut.",
  "worklist.pair.refused":
    "Du kannst dieses Paar nicht klären. Dafür ist Bearbeitungszugriff auf beide Datensätze nötig, den Admins und Vertriebsleitungen haben.",
  "worklist.pair.alreadySettled":
    "Das Paar wurde nicht geklärt: Möglicherweise hat jemand zuerst entschieden, oder die Datensätze haben sich geändert. Lade die Liste neu.",
  "worklist.pair.stewardOnly":
    "Nur wer beide Datensätze bearbeiten kann, kann das klären: Admins oder Vertriebsleitungen.",
  "worklist.needsPrep": "Braucht Vorbereitung",
  "worklist.pane.title": "Datensatzdetails",
  "worklist.pane.openRow": "Details für {position}, {title} anzeigen",
  "worklist.pane.loading": "Datensatz wird geladen…",
  "worklist.pane.nothing": "Noch nichts erfasst.",
  "worklist.pane.lastInbound": "Zuletzt eingehend",
  "worklist.pane.lastOutbound": "Zuletzt ausgehend",
  "worklist.pane.never": "Nie",
  "worklist.pane.company": "Unternehmen",
  "worklist.pane.role": "Rolle",
  "worklist.band.now": "Jetzt",
  "worklist.dueGroup.tomorrow": "Morgen fällig",
  "worklist.dueGroup.this_week": "Diese Woche fällig",
  "worklist.dueGroup.later": "Später",
  "worklist.band.build_pipeline": "Akquise",
  "worklist.band.keep_momentum": "In Bewegung halten",
  "worklist.band.review": "Prüfen",
  "worklist.bandClear.now":
    "Keine dringenden Einträge. Weitere Arbeit steht unten.",
  "worklist.bandClear.build_pipeline": "Keine Akquise-Arbeit offen.",
  "worklist.bandClear.keep_momentum":
    "Keine vereinbarte Arbeit gerät ins Stocken.",
  "worklist.bandClear.review": "Nichts zu prüfen.",
  "worklist.disposition.verb.snooze": "Zurückstellen",
  "worklist.disposition.snoozeForDays_one": "{value} Tag zurückstellen",
  "worklist.disposition.snoozeForDays_other": "{value} Tage zurückstellen",
  "worklist.disposition.snoozeFor": "Dauer der Zurückstellung",
  "worklist.disposition.snoozeUntil.reply": "Bis zur Antwort",
  "worklist.disposition.verb.not_mine": "Nicht zuständig",
  "worklist.disposition.verb.not_sales": "Kein Kunde",
  "worklist.disposition.done.snooze": "Morgen wieder auf deiner Liste.",
  "worklist.disposition.doneSnooze_one": "Morgen wieder auf deiner Liste.",
  "worklist.disposition.doneSnooze_other":
    "In {value} Tagen wieder auf deiner Liste.",
  "worklist.disposition.doneSnoozeUntil.reply":
    "Wieder auf deiner Liste, sobald eine Antwort kommt.",
  "worklist.disposition.done.not_mine":
    "Von deiner Liste entfernt. Wer zuständig ist, sieht den Eintrag weiterhin.",
  "worklist.disposition.done.not_sales": "Von allen Listen entfernt.",
  "worklist.disposition.swipeCancel": "Behalten",
  "worklist.disposition.menu": "Aus der Liste entfernen",
  "worklist.disposition.undo": "Rückgängig machen",
  "worklist.disposition.undoFailed":
    "Rückgängig machen fehlgeschlagen. Die Nachricht ist weiterhin nicht auf deiner Liste.",
  "worklist.disposition.failed":
    "Der Eintrag wurde nicht entfernt. Versuche es erneut.",
  "worklist.scope.label": "Wessen Arbeit",
  "worklist.scope.mine": "Meine",
  "worklist.scope.unassigned": "Nicht zugewiesen",
  "worklist.scope.team": "Team",
  "worklist.scope.all": "Alle",
  "worklist.owner.visibleLabel": "Ansicht",
  "worklist.manager.cancel": "Abbrechen",
  "worklist.owner.mine": "Meine Worklist",
  "worklist.owner.backToMine": "Zurück zu deiner Worklist",
  "worklist.manager.reassign": "Neu zuweisen",
  "worklist.manager.reassignTo": "Neu zuweisen an",
  "worklist.manager.reassignConfirm": "Neu zuweisen",
  "worklist.manager.takeOwnership": "Übernehmen",
  "worklist.manager.takeOwnershipAsk":
    "Damit wird der Datensatz aus der bisherigen Worklist in deine verschoben.",
  "worklist.manager.takeOwnershipConfirm": "Übernehmen",
  "worklist.manager.tookOwnership": "Übernommen",
  "worklist.manager.takeOwnershipFailed":
    "Übernahme fehlgeschlagen. Die Zuständigkeit für den Datensatz bleibt unverändert.",
  "worklist.manager.reassigned": "Neu zugewiesen",
  "worklist.manager.reassignFailed":
    "Der Eintrag wurde nicht neu zugewiesen. Versuche es erneut.",
  "worklist.manager.coach": "Notiz hinzufügen",
  "worklist.manager.coachTitle": "Notiz für {name}",
  "worklist.manager.coachTitleUnnamed": "Notiz",
  "worklist.manager.coachIntro":
    "Eine kurze Notiz, die in der Worklist des Teammitglieds erscheint.",
  "worklist.manager.coachAbout": "Thema",
  "worklist.manager.coachConfirm": "Notiz hinzufügen",
  "worklist.manager.coached": "Notiz zur Worklist hinzugefügt",
  "worklist.manager.coachFailed":
    "Die Notiz wurde nicht hinzugefügt. Versuche es erneut.",
  "worklist.manager.coachRefused":
    "Du kannst für {name} keine Notiz hinzufügen.",
  "worklist.manager.note": "Notiz (optional)",
  "worklist.manager.kind.reply_aging": "Lange offene Antwort",
  "worklist.manager.kind.next_step": "Nächster Schritt im Deal",
  "worklist.manager.kind.review_backlog": "Prüfrückstand",
  "worklist.manager.kind.general": "Sonstiges",
  "worklist.board.title": "Teamarbeit mit Handlungsbedarf",
  "worklist.exceptions.title": "Team-Ausnahmen",
  "worklist.handled.title": "Für dich erledigt",
  "worklist.walk.arrived":
    "Neu seit dem Öffnen der Liste: {arrived}. Aktualisiere, um sie hinzuzufügen.",
  "worklist.walk.gone": "Davon seit dem Öffnen der Liste erledigt: {gone}.",
  "worklist.walk.both":
    "Seit dem Öffnen der Liste: {arrived} neu, {gone} bereits erledigt.",
  "worklist.walk.title": "Liste geändert",
  "worklist.walk.refresh": "Aktualisieren",
  "worklist.handled.empty":
    "Heute wurden keine Aktionen in deinem Auftrag ausgeführt.",
  "worklist.handled.loading": "Aktionen werden geladen…",
  "worklist.handled.count": "{count} erledigt",
  "worklist.handled.what": "Aktion",
  "worklist.handled.about": "Datensatz",
  "worklist.handled.when": "Zeitpunkt",
  "worklist.handled.noRecord": "Kein Datensatz",
  "worklist.handled.wayBack": "Rückgängig machen",
  "worklist.handled.putBackDone": "Bereits rückgängig gemacht",
  "worklist.handled.truncated": "Liste gekürzt. Es gibt weitere Einträge.",
  "worklist.exceptions.empty": "Keine Team-Ausnahmen.",
  "worklist.exceptions.loading": "Team wird geladen…",
  "worklist.exceptions.count": "{count} mit Handlungsbedarf",
  "worklist.exceptions.condition": "Bedingung",
  "worklist.exceptions.subject": "Datensatz",
  "worklist.exceptions.owner": "Zuständig",
  "worklist.exceptions.basis": "Grundlage",
  "worklist.exceptions.intervene": "Aktion",
  "worklist.exceptions.nobody": "Niemand zuständig",
  "worklist.exceptions.ownerWithheld": "Ausgeblendet",
  "worklist.exceptions.truncated": "Liste gekürzt. Es gibt weitere Einträge.",
  "worklist.exceptions.kind.response_breached": "Erste Antwort überfällig",
  "worklist.exceptions.kind.revenue_at_risk": "Gefährdeter Umsatz",
  "worklist.exceptions.kind.unassigned": "Nicht zugewiesen",
  "worklist.exceptions.kind.repeated_failure": "Wiederholter Fehler",
  "worklist.board.loading": "Teamarbeit wird geladen…",
  "worklist.board.empty": "Noch keine Teammitglieder zugewiesen.",
  "worklist.board.member": "Mitglied",
  "worklist.board.waiting": "Wartet auf Antwort",
  "worklist.board.atRisk": "Gefährdete Deals",
  "worklist.board.overdue": "Überfällig",
  "worklist.board.nobody": "Nicht zugewiesene Arbeit",
  "worklist.coaching.title": "Coaching-Vorschläge",
  "worklist.coaching.promises_one": "{name} hat {count} fällige Kundenzusage",
  "worklist.coaching.promises_other":
    "{name} hat {count} fällige Kundenzusagen",
  "worklist.coaching.waiting_one": "{count} Kontakt wartet auf {name}",
  "worklist.coaching.waiting_other": "{count} Kontakte warten auf {name}",
  "worklist.coaching.overdue_one": "{name} hat {count} überfällige Aufgabe",
  "worklist.coaching.overdue_other": "{name} hat {count} überfällige Aufgaben",
  "worklist.board.promises": "Fällige Zusagen",
  "worklist.board.truncated":
    "Die Zählung ist unvollständig. Jede Zahl ist ein Mindestwert.",
  "worklist.readings.label": "Zahlen von heute",
  "worklist.readings.revenue": "Gefährdeter Umsatz",
  "worklist.readings.revenue.detail": "Heute stockende Deals",
  "worklist.readings.revenue.unpriced": "Stockende Deals · keiner mit Preis",
  "worklist.readings.revenue.noFigure": "Ohne Preis",
  "worklist.readings.replies": "Antworten der Käuferseite",
  "worklist.readings.replies.detail": "Wartende Kontakte",
  "worklist.readings.prospecting": "Akquise",
  "worklist.readings.prospecting.detail": "Wartet auf erste Antwort",
  "worklist.readings.review": "Prüfungen",
  "worklist.readings.review.detail": "Vorschläge · Datensatzprüfungen",
  "worklist.readings.truncated":
    "Die Zählung ist unvollständig. Jede Zahl ist ein Mindestwert.",
  "worklist.hidden.title": "Aus der Worklist ausgeblendet",
  "worklist.hidden.loading": "Ausgeblendete Einträge werden geprüft…",
  "worklist.hidden.clear":
    "Nichts ist ausgeblendet. Jeder wartende Kontakt erreicht eine Worklist.",
  "worklist.hidden.truncated":
    "Die Zählung ist unvollständig. Jede Zahl ist ein Mindestwert.",
  "worklist.hidden.count": "{count} wartend",
  "worklist.hidden.pastHorizon": "Zu alt für die Worklist",
  "worklist.hidden.pastHorizon.detail":
    "Das hat niemand entschieden. Die Nachricht kam vor Monaten und blieb unbeantwortet.",
  "worklist.hidden.unlinked": "Mit keinem Datensatz verknüpft",
  "worklist.hidden.unlinked.detail":
    "Meist kein Vertrieb. Manchmal ein nicht zugeordneter Kontakt.",
  "worklist.hidden.colleagues": "Von den Domains deines Unternehmens",
  "worklist.hidden.colleagues.detail":
    "Wird als intern behandelt. Eine falsch eingetragene Unternehmensdomain kann einen echten Kontakt verbergen.",
  "worklist.hidden.notSales": "Als nicht vertriebsrelevant markiert",
  "worklist.hidden.notSales.detail":
    "Für das gesamte Unternehmen ausgeblendet. Das läuft nicht ab.",
  "worklist.hidden.setAside": "Von dir zurückgestellt",
  "worklist.hidden.setAside.detail":
    "Zurückgestellt oder als nicht zuständig markiert. Zurückgestellte Einträge kommen automatisch zurück.",
  "worklist.hidden.shown": "Angezeigt in der Worklist: {count}.",
  "worklist.filter.label": "Art der Arbeit",
  "worklist.filter.all": "Alle",
  "worklist.filter.customer_waiting": "Kontakt wartet",
  "worklist.filter.leads": "Leads",
  "worklist.filter.deals_at_risk": "Gefährdete Deals",
  "worklist.filter.meetings": "Termine",
  "worklist.filter.tasks": "Aufgaben",
  "worklist.filter.decisions": "Freigaben",
  "worklist.filter.system": "System",
  "worklist.filter.linked.urgent":
    "Nur dringende Einträge: Jemand wartet, oder eine Zusage droht gebrochen zu werden.",
  "worklist.filter.linked.except_decisions":
    "Alles außer Freigaben, die bereits in deinem Morgenbericht stehen.",
  "worklist.filter.linked.changed_since_brief":
    "Nur Änderungen seit dem Morgenbericht der letzten Nacht.",
  "worklist.filter.linked.clear": "Vollständige Worklist anzeigen",
  "worklist.category.customer_waiting": "Kontakt wartet",
  "worklist.category.leads": "Lead",
  "worklist.signal.closing_soon": "Abschluss bald",
  "worklist.signal.stalled": "Deal gefährdet",
  "worklist.signal.opportunity": "Chance",
  "worklist.signal.moved": "Kürzlich bewegt",
  "worklist.category.deals_at_risk": "Deal gefährdet",
  "worklist.category.meetings": "Termin",
  "worklist.category.tasks": "Aufgabe",
  "worklist.category.decisions": "Freigabe",
  "worklist.category.system": "System",
  "worklist.because.pinned": "von dir angeheftet",
  "worklist.because.buyer_wrote_last": "Käuferseite hat zuletzt geschrieben",
  "worklist.because.waiting_days": "wartet",
  "worklist.because.more_one": "+{count} weitere",
  "worklist.because.more_other": "+{count} weitere",
  "worklist.because.waiting_days.value_one": "wartet seit {value} Tag",
  "worklist.because.waiting_days.value_other": "wartet seit {value} Tagen",
  "worklist.because.overdue": "überfällig",
  "worklist.because.due_today": "heute fällig",
  "worklist.because.closing_soon": "Abschluss innerhalb von 2 Wochen erwartet",
  "worklist.because.expected_revenue": "ein offener Deal hängt davon ab",
  "worklist.because.expected_revenue.value": "Wert {value}",
  "worklist.because.material": "über dem typischen Wert offener Deals",
  "worklist.because.material.value":
    "Wert {value}, über dem typischen Wert offener Deals",
  "worklist.because.below_material":
    "nicht über dem typischen Wert gefährdeter Deals",
  "worklist.because.below_material.value":
    "{value}, nicht über dem typischen Wert gefährdeter Deals",
  "worklist.because.quiet_days": "verstummt",
  "worklist.because.quiet_days.value_one": "seit {value} Tag still",
  "worklist.because.quiet_days.value_other": "seit {value} Tagen still",
  "worklist.because.no_champion": "kein Champion",
  "worklist.because.promised": "von dir zugesagt",
  "worklist.because.approved_and_failed": "freigegeben, aber nicht ausgeführt",
  "worklist.because.blocks_customer_work": "Kontakt wartet darauf",
  "worklist.because.routine": "Routinebereinigung",
  "worklist.because.repeated_failure": "wiederholter Fehler",
  "worklist.because.legal_deadline": "gesetzliche Frist läuft",
  "worklist.because.meeting_soon": "beginnt bald",
  "worklist.because.meeting_unprepared": "nichts vorbereitet",
  "worklist.because.outcome_unrecorded": "kein Ergebnis erfasst",
  "worklist.because.response_overdue": "Antwort überfällig",
  "worklist.because.response_due_soon": "Antwort bald fällig",
  "worklist.because.response_due_soon.value": "Antwort fällig bis {value}",
  "worklist.because.unassigned": "niemand zuständig",
  "worklist.because.stale": "wartet schon lange",
  "worklist.because.no_reply_history": "keine bisherigen Antworten",
  "worklist.because.asks_nothing": "fordert nichts an",
  "worklist.because.addressed_elsewhere": "an jemand anderen im Team gerichtet",
  "worklist.above.pin":
    "Steht über dem nächsten Eintrag, weil du ihn angeheftet hast.",
  "worklist.above.level":
    "Steht über dem nächsten Eintrag, weil er dringender ist.",
  "worklist.above.deadline":
    "Steht wegen des Datums über dem nächsten Eintrag.",
  "worklist.above.deadline.pair":
    "Steht über dem nächsten Eintrag: {mine} gegenüber {theirs}.",
  "worklist.above.expected_revenue":
    "Steht wegen des erwarteten Umsatzes über dem nächsten Eintrag.",
  "worklist.above.expected_revenue.pair":
    "Steht über dem nächsten Eintrag: {mine} gegenüber {theirs}.",
  "worklist.above.waiting_days":
    "Steht wegen der Wartezeit über dem nächsten Eintrag.",
  "worklist.above.waiting_days.pair":
    "Steht über dem nächsten Eintrag: {mine} gegenüber {theirs}.",
  "worklist.above.relationship":
    "Steht wegen der Beziehungsstärke über dem nächsten Eintrag.",
  "worklist.above.crowded":
    "Steht über dem nächsten Eintrag, der einer von vielen seiner Art ist.",
  "worklist.verdict.live": "Aktiv",
  "worklist.verdict.drifting": "Stockt",
  "worklist.verdict.blocked": "Blockiert",
  "worklist.verdict.cold": "Kalt",
  "worklist.verdict.believes": "Bewertung",
  "worklist.verdict.rule": "Warum das hier steht",
  "worklist.verdict.asOf": "Stand {when}",
  "worklist.consequence.buyer_waits":
    "Ohne Reaktion wartet die Käuferseite weiter.",
  "worklist.consequence.promise_breaks":
    "Ohne Reaktion wird eine Zusage gebrochen.",
  "worklist.consequence.deal_drifts": "Ohne Reaktion stockt der Deal weiter.",
  "worklist.consequence.deal_slips_past_close":
    "Ohne Reaktion verstreicht das vereinbarte Abschlussdatum.",
  "worklist.consequence.meeting_unprepared":
    "Ohne Reaktion gehst du unvorbereitet in den Termin.",
  "worklist.consequence.task_slips":
    "Ohne Reaktion verzögert sich die Aufgabe.",
  "worklist.consequence.work_blocked":
    "Ohne Reaktion bleibt die Arbeit blockiert.",
  "worklist.consequence.customer_never_received":
    "Ohne Reaktion erhält der Kontakt es nie.",
  "worklist.consequence.you_believe_it_happened":
    "Ohne Reaktion wirkt es weiter erledigt, obwohl es nie ausgeführt wurde.",
  "worklist.consequence.legal_deadline_missed":
    "Ohne Reaktion verstreicht eine gesetzliche Frist.",
  "worklist.consequence.mailbox_blind":
    "Ohne Reaktion fehlen auf dieser Seite weiterhin E-Mails, die nicht ankommen.",
  "worklist.consequence.data_drifts": "Ohne Reaktion veralten die Datensätze.",
  "worklist.untitled.approval": "Freigabe ausstehend",
  "worklist.untitled.dedupe_candidate": "Mögliche doppelte Datensätze",
  "worklist.untitled.task": "Aufgabe ohne Titel",
  "worklist.untitled.brief_item": "Zu prüfender Deal",
  "worklist.untitled.conversation_claim": "Deine Zusage",
  "worklist.untitled.customer_waiting": "Kontakt wartet auf Antwort",
  "worklist.untitled.lead_response": "Lead ohne Titel",
  "worklist.untitled.deal_at_risk": "Deal stockt",
  "worklist.untitled.meeting": "Termin ohne Titel",
  "worklist.untitled.meeting_outcome": "Terminergebnis",
  "worklist.untitled.relationship_decay": "Beziehung wird still",
  "worklist.untitled.failed_approval": "Freigegebene Aktion nicht ausgeführt",
  "worklist.untitled.dsr": "Offene Datenschutzanfrage",
  "worklist.untitled.notice_case": "Dem Kontakt geschuldete Information",
  "worklist.untitled.capture_health":
    "Postfachverbindung braucht Aufmerksamkeit",
  "worklist.untitled.ai_work_health": "KI-Arbeit muss geprüft werden",
  "worklist.untitled.bounce": "E-Mail unzustellbar",
  "worklist.untitled.undelivered": "E-Mail nicht gesendet",
  "worklist.untitled.automation_run": "Automatisierungsregel fehlgeschlagen",
  "worklist.untitled.notice": "Hinweis",
  // Eine Domain-Frage trägt immer die Domain als Titel, daher sollte dieser
  // Ersatztext nie erscheinen. Er existiert, weil die Zuordnung alle bekannten
  // Quellen abdecken muss.
  "worklist.untitled.domain_question": "Ungeprüfte Domain",
  "worklist.untitled.introduction_request":
    "Vorstellung aus dem Team angefragt",
  "worklist.verb.decide": "Entscheiden",
  // Die Schublade, in der entschieden wird.
  "worklist.decision.title": "Deine Entscheidung",
  "worklist.decision.loading": "Vorschlag wird geladen…",
  "worklist.decision.unavailable":
    "Der Vorschlag wurde nicht geladen. Beantworte ihn unter Freigaben.",
  "worklist.verb.merge": "Zusammenführen",
  "worklist.verb.open": "Öffnen",
  "worklist.verb.complete": "Öffnen",
  "worklist.verb.snooze": "Öffnen",
  "worklist.verb.acknowledge": "Zur Kenntnis nehmen",
  "worklist.verb.promiseKept": "Erledigt",
  "worklist.verb.promiseSettled": "Als eingehalten markiert",
  "worklist.verb.promiseSettleFailed":
    "Die Zusage wurde nicht als eingehalten markiert. Versuche es erneut.",
  "worklist.verb.meetingUpdate": "Aktualisieren",
  "worklist.verb.meetingUpdateTitle": "Termin",
  "worklist.verb.meetingReading": "Termin wird geladen…",
  "worklist.verb.meetingWhatHappened": "Terminnotizen",
  "worklist.verb.meetingBodyHint":
    "Besprochenes und nächste Schritte. Kalendernotizen lassen sich hier bearbeiten.",
  "worklist.verb.meetingHeld": "Stattgefunden",
  "worklist.verb.meetingNoShow": "Nicht erschienen",
  "worklist.verb.meetingCanceled": "Abgesagt",
  "worklist.verb.meetingOutcomeRecorded": "Terminergebnis erfasst",
  "worklist.verb.meetingOutcomeFailed":
    "Das Ergebnis wurde nicht erfasst. Versuche es erneut.",
  "worklist.verb.retry": "Erneut ausf\u00fchren",
  "worklist.verb.retryStarted": "Regel wird erneut ausgeführt…",
  "worklist.verb.retryFailed":
    "Die Regel wurde nicht erneut ausgeführt. Versuche es erneut.",
  "worklist.verb.keep": "Unternehmen anlegen",
  "worklist.verb.discard": "Domain ausschließen",
  "worklist.verb.domainKept": "Unternehmen aus Domain angelegt",
  "worklist.verb.domainKeepFailed":
    "Das Unternehmen wurde nicht angelegt. Versuche es erneut.",
  "worklist.verb.domainDiscarded":
    "Deine E-Mails von dieser Domain werden nicht mehr erfasst.",
  "worklist.verb.domainDiscardFailed":
    "Die Domain wurde nicht ausgeschlossen. Versuche es erneut.",
  "worklist.verb.retryRefusedNotFailed":
    "Nichts erneut auszuführen. Dieser Lauf wurde absichtlich gestoppt, nicht durch einen Fehler.",
  "worklist.verb.retryRefusedRepeats":
    "Diese Regel darf nicht zweimal laufen, daher könnte ein erneuter Lauf ihre Aktionen wiederholen.",
  "worklist.verb.retryRefusedEventGone":
    "Das auslösende Ereignis existiert nicht mehr, daher lässt sich das nicht erneut ausführen. Zeitgesteuerte Regeln prüfen automatisch erneut.",
  "worklist.verb.acknowledgeFailed":
    "Der Eintrag wurde nicht als gesehen markiert. Versuche es erneut.",
  "worklist.verb.completeFailed":
    "Die Aufgabe wurde nicht abgeschlossen. Versuche es erneut.",
  "worklist.verb.pin": "Anheften",
  "worklist.verb.pinHint":
    "Setzt den Eintrag an den Anfang deiner Worklist, solange er zu deinen letzten Anheftungen gehört. Die Dringlichkeit bleibt unverändert.",
  "worklist.verb.unpinHint":
    "Setzt den Eintrag zurück an seinen Rangplatz in deiner Worklist.",
  "worklist.verb.unpin": "Nicht mehr anheften",
  "worklist.verb.pinFailed":
    "Der Eintrag wurde nicht angeheftet. Versuche es erneut.",
  "worklist.verb.unpinFailed":
    "Der Eintrag ist weiterhin angeheftet. Versuche es erneut.",
  "worklist.verb.completed": "Aufgabe erledigt",
  "worklist.verb.dismiss": "Nicht jetzt",
  "worklist.verb.dismissed": "Für einen Monat zurückgestellt",
  "worklist.verb.dismissUndo": "Rückgängig machen",
  "worklist.verb.dismissFailed":
    "Der Kontakt wurde nicht zurückgestellt. Versuche es erneut.",
  "worklist.verb.dismissUndoFailed":
    "Der Kontakt wurde nicht wiederhergestellt. Versuche es erneut.",
  "worklist.verb.completeUndo": "Rückgängig machen",
  "worklist.verb.completeUndoFailed":
    "Die Aufgabe wurde nicht wieder geöffnet. Versuche es erneut.",
  "worklist.source.failed": "Quelle nicht geladen: {source}",
  "worklist.source.withheld": "Quelle für dich nicht verfügbar: {source}",
  "worklist.untitled.generic": "Eintrag braucht Aufmerksamkeit",
  "worklist.batch.likely_automated_one":
    "{count} vermutlich automatischer Absender",
  "worklist.batch.likely_automated_other":
    "{count} vermutlich automatische Absender",
  "worklist.batch.company_match_one":
    "{count} Adresse bei einem bekannten Unternehmen",
  "worklist.batch.company_match_other":
    "{count} Adressen bei bekannten Unternehmen",
  "worklist.batch.uncertain_contact_one": "{count} zu prüfende Adresse",
  "worklist.batch.uncertain_contact_other": "{count} zu prüfende Adressen",
  "worklist.batch.duplicates_one": "{count} mögliches Duplikat",
  "worklist.batch.duplicates_other": "{count} mögliche Duplikate",
  "worklist.batch.held_draft_one": "{count} Entwurf wartet auf den Versand",
  "worklist.batch.held_draft_other": "{count} Entwürfe warten auf den Versand",
  "worklist.untitled.batch": "Zu prüfende Routineeinträge",
  "worklist.verb.review_batch": "Prüfen",
  "worklist.verb.draft_reply": "Lesen und antworten",
  // Wo der Editor wirklich aufgeht, ist das Verb die HANDLUNG.
  "worklist.verb.draft_reply_now": "Antwort entwerfen",
  // Eine ERSTE Nachricht, keine Antwort auf eine bestehende.
  "worklist.verb.draft_email": "E-Mail schreiben",
  "worklist.verb.draft_email_now": "E-Mail entwerfen",
  // Ein Briefing, keine Nachricht — ein einziger Schlüssel, weil die
  // Steuerung nie den Editor öffnet und daher die "jetzt"-Aufteilung oben
  // nicht braucht.
  "worklist.verb.open_meeting_brief": "Termin vorbereiten",
  "worklist.deal.closes": "Abschluss {date}",
  "worklist.when.starts": "Beginn: {when}",
  "worklist.when.due": "Fällig: {when}",
  "worklist.batch.system_incident_one":
    "{cause} ist {count}-mal fehlgeschlagen",
  "worklist.batch.system_incident_other":
    "{cause} ist {count}-mal fehlgeschlagen",
  "worklist.batch.unnamedCause": "Ein Prozess",

  "ob.conv.scene.settleEyebrow": "Deine Entscheidung ist gefragt",
  "ob.conv.review.boardSub":
    "Jede Zeile nennt ihre Herkunft. Geschrieben wird erst, wenn du bestätigst.",
  "ob.conv.manual.boardTitle": "Angaben manuell eingeben",
  "ob.conv.scene.writes": "Schreibt",
  "ob.core.idle": "Core · inaktiv",
  "ob.core.ingest": "Core · liest Eingaben",
  "ob.core.working": "Core · arbeitet",
  "ob.core.warning": "Core · braucht Aufmerksamkeit",
  "ob.core.error": "Core · angehalten",
  "ob.scan.tallyPages": "gelesene Seiten",
  "ob.scan.tallyFacts": "gefundene Fakten",
  "ob.scan.tallyUncertain": "für dich offen gelassen",
  "ob.scan.tickerFact": "{field}: {value}",
  "ob.digest.where": "Was Margince über das Unternehmen weiß",
  "ob.digest.written": "{n} von {m} Zeilen geschrieben",
  "ob.digest.companyLine":
    "Unternehmensprofil von {host}, gelesene Seiten: {n}",
  "ob.digest.citedCaption": "mit Quellseite",
  "ob.digest.openCaption": "noch offen",
  "ob.digest.section.identity": "Identität",
  "ob.digest.section.offer": "Was das Unternehmen verkauft",
  "ob.digest.section.customer": "An wen es verkauft",
  "ob.digest.section.sales": "Wie es verkauft",
  "ob.digest.facts": "Belege",
  "ob.digest.contacts": "Kontakte",
  "ob.digest.sources": "Quellen",
  "ob.digest.blank": "noch nicht geschrieben",
  "ob.digest.notWritten": "nicht erfasst",
  "ob.digest.settle": "Entscheiden",
  "ob.digest.deciding": "Entscheidung läuft",
  "ob.digest.yours": "von dir eingetragen",
  "ob.digest.editLine": "{label} bearbeiten",
  "ob.digest.saveChanges": "Änderungen speichern",
  "ob.digest.changed_one": "{count} ungespeicherte Zeilenänderung",
  "ob.digest.changed_other": "{count} ungespeicherte Zeilenänderungen",
  "ob.digest.pickFacts": "Zu behaltende Fakten auswählen",
  "ob.digest.referenceNote":
    "Ein späteres erneutes Lesen kann Änderungen an diesem Datensatz vorschlagen. Eine Zeile, die jemand bearbeitet hat, wird dabei nie überschrieben.",
  "ob.digest.sidebarLabel": "Fakten zum Unternehmen",
  "ob.digest.sidebar.legalName": "Rechtlicher Name",
  "ob.digest.sidebar.founded": "Gegründet",
  "ob.digest.sidebar.headquarters": "Hauptsitz",
  "ob.digest.sidebar.offices": "Standorte",
  "ob.digest.sidebar.employees": "Mitarbeitende",
  "ob.digest.sidebar.certifications": "Zertifizierungen",
  "ob.digest.pageKind.home": "Startseite",
  "ob.digest.pageKind.impressum": "Impressum",
  "ob.digest.pageKind.about": "Seite zum Unternehmen",
  "ob.digest.pageKind.team": "Team-Seite",
  "ob.digest.pageKind.services": "Leistungsseite",
  "ob.digest.pageKind.products": "Produktseite",
  "ob.digest.pageKind.contact": "Kontaktseite",
  "ob.digest.pageKind.other": "Seite",
  "ob.deck.counter": "{n} von {m}",
  "ob.deck.left": "Noch {n} von {m}",
  "ob.deck.settled_one": "{count} Fakt aus Belegen hinzugefügt",
  "ob.deck.settled_other": "{count} Fakten aus Belegen hinzugefügt",
  "ob.deck.needed": "Zum Fortfahren nötig",
  "ob.deck.optional": "Optional",
  "ob.deck.next": "Weiter",
  "ob.deck.leaveOut": "Weglassen",
  "ob.deck.readWhole": "Ganzes Profil lesen",
  "ob.deck.backToOpen": "Zurück zu den offenen Fragen",
  "ob.deck.backToRecord": "Zurück zum Datensatz",
  "ob.deck.confirm": "Profil bestätigen",
  "ob.deck.stillNeeded": "Noch erforderlich: {fields}",
  "ob.deck.openLeft_one":
    "{count} unbeantwortete Frage. Der Datensatz wird ohne sie gespeichert.",
  "ob.deck.openLeft_other":
    "{count} unbeantwortete Fragen. Der Datensatz wird ohne sie gespeichert.",
  "ob.conv.invite.pickOne":
    "Wähle eine der beiden Optionen aus, um fortzufahren.",
  "ob.conv.voice.speakerPick": "Wähle aus, wer spricht, um fortzufahren.",
  "ob.deck.clear_one": "Nichts mehr zu entscheiden. {count} Fakt erfasst.",
  "ob.deck.clear_other": "Nichts mehr zu entscheiden. {count} Fakten erfasst.",
  "ob.deck.eyebrow": "Weitere belegte Fakten",
  "ob.deck.title": "Fragen an dich",
  "ob.stage.flow": "Einrichtung",
  "ob.stop.read": "Website lesen",
  "firstRun.ai.rankedHint":
    "Listet außerdem die 10 am besten bewerteten Modelle, die OpenRouter aktuell anbietet, sortiert nach {rankedBy}, mit Anbieterpreisen.",
  "firstRun.ai.rankedUnavailable":
    "Die aktuelle Modellliste von OpenRouter konnte nicht gelesen werden, daher werden die Modelle aus der Preisliste angezeigt.",
  "firstRun.ignite.title": "Modell verbunden",
  "firstRun.ignite.sub":
    "Der Schlüssel ist gespeichert und das Modell hat geantwortet. Das ändert sich dadurch.",
  "firstRun.ignite.sealed": "Im Tresor gespeichert · {vendor}",
  "firstRun.ignite.reaching": "Modell wird kontaktiert…",
  "firstRun.ignite.canNow": "Kann jetzt",
  "firstRun.ignite.cannot": "Kann nicht",
  "firstRun.ignite.read":
    "die Website des Unternehmens lesen und über die Befunde berichten",
  "firstRun.ignite.draft":
    "Entwürfe in dem Schreibstil schreiben, den du trainiert hast",
  "firstRun.ignite.act":
    "ohne deine Freigabe etwas senden oder einen Datensatz ändern",
  "firstRun.ignite.carryOn": "Weiter",
  "firstRun.step.model": "Modell",
  "firstRun.step.platform": "Plattform",
  "firstRun.google.eyebrow": "Modell verbunden · E-Mail nicht verbunden",
  "firstRun.platform.title": "Mit welcher Plattform arbeitet dein Unternehmen?",
  "firstRun.platform.sub":
    "Diese Antwort entscheidet, wie E-Mails zu Margince gelangen und wie sich Nutzende anmelden. Ändern lässt sie sich später in den Einstellungen.",
  "firstRun.platform.legend": "Plattform, mit der dieses Unternehmen arbeitet",
  "firstRun.platform.google": "Google Workspace",
  "firstRun.platform.googleWhat":
    "E-Mail, Kalender und Anmeldung über eine eigene Google-App.",
  "firstRun.platform.microsoft": "Microsoft 365",
  "firstRun.platform.microsoftWhat":
    "E-Mail, Kalender und Anmeldung über eine eigene Entra-App.",
  "firstRun.platform.imap": "IMAP",
  "firstRun.platform.imapWhat":
    "Jedes Postfach verbindet sich mit einem eigenen IMAP-App-Passwort. Die Anmeldung erfolgt mit E-Mail-Adresse und Passwort.",
  "firstRun.platform.redirectTitle":
    "Diese Weiterleitungs-URIs in der App registrieren",
  "firstRun.platform.redirectHint":
    "Füge jede URI der App hinzu, bevor du hier speicherst. „Anmeldung“ bringt die Anmeldeschaltfläche auf die Anmeldeseite; mit „Postfach“ und „Kalender“ können Nutzende ihre eigenen verbinden. Fehlt eine URI, scheitert es am Zustimmungsbildschirm des Anbieters.",
  "firstRun.google.helpToggle": "Wo du diese Angaben findest",
  "firstRun.google.helpStep1":
    "Öffne in der Google Cloud Console ein Projekt und gehe zu „APIs und Dienste“, dann „Anmeldedaten“. Wähle „Anmeldedaten erstellen“, dann „OAuth-Client-ID“, und wähle „Webanwendung“.",
  "firstRun.google.helpStep2":
    "Aktiviere die Gmail API und füge dem Zustimmungsbildschirm die Bereiche gmail.readonly und gmail.send hinzu. Beide werden gemeinsam angefragt, weil Google einem bereits ausgestellten Refresh-Token keinen Bereich hinzufügt; das Senden später anzufordern heißt, das Postfach erneut zu verbinden.",
  "firstRun.google.helpStep3":
    "Füge unter „Autorisierte Weiterleitungs-URIs“ die oben aufgeführten URIs hinzu. „Postfach“ ist für E-Mails erforderlich; „Kalender“ und „Anmeldung“ aktivieren diese Funktionen.",
  "firstRun.google.helpStep4":
    "Kopiere Client-ID und Clientschlüssel in die 2 Felder unten. Der Clientschlüssel wird einmal gesendet, im Schlüsseltresor gespeichert und ist danach nie wieder lesbar.",
  "firstRun.google.helpConsole": "Google Cloud Console für Anmeldedaten",
  "firstRun.google.helpDocs":
    "Alle Voraussetzungen, inklusive Microsoft und IMAP: docs/how-to/connect-a-mailbox.md",
  "firstRun.platform.imapNote":
    "Für die ganze Installation wird nichts konfiguriert. Verbinde dein eigenes Postfach jetzt oder später; weitere Postfächer werden in den Einstellungen unter Verbindungen verbunden, jedes mit eigenem App-Passwort.",
  "firstRun.platform.skip": "Nicht jetzt",
  "firstRun.needed": "Zum Fortfahren nötig",
  "firstRun.stillNeeded": "Noch erforderlich: {fields}",
  "firstRun.platform.foot":
    "Alle Antworten hier lassen sich später in den Einstellungen ändern.",
  "firstRun.microsoft.note":
    "Registriere in Microsoft Entra eine App mit den Weiterleitungs-URIs oben und füge hier ihre Client-ID und ihren Clientschlüssel ein. Binde sie an dein Verzeichnis: Die Postfächer dieses Verzeichnisses verbinden sich darüber, und seine Mitglieder melden sich damit an.",
  "firstRun.microsoft.helpSignIn":
    "Das Verzeichnis bringt Microsoft auf die Anmeldeseite, daher ist es hier erforderlich. Um eine App ohne Verzeichnis zu registrieren (jedes Unternehmen kann ein Postfach verbinden, niemand meldet sich mit Microsoft an), nutze stattdessen die Einstellungen.",
  "firstRun.microsoft.tenantHint":
    "Das Entra-Verzeichnis, zu dem deine Mitarbeitenden gehören. Postfächer verbinden sich darüber, und die Microsoft-Anmeldung läuft darauf.",
  "firstRun.ai.eyebrow": "Kein Modell verbunden",
  "aiRates.chatLane": "Chat-Modell",
  "aiRates.embedLane": "Embedding-Modell",
  "aiRates.perMTokInOut": "pro Million Tokens, Eingabe → Ausgabe",
  "aiRates.perMTok": "pro Million Tokens",
  "aiRates.unpriced": "Ohne Preis",
  "aiRates.unpricedDetail": "Aufrufe laufen weiter",
  "aiRates.unpricedConsequence": "Fehlt in Nutzung und Kosten",
  "aiRates.unpricedBasis":
    "Um die Kosten dieser Aufrufe auszuweisen, hinterlege einen Preis in den Einstellungen unter KI.",
  "aiRates.priced": "Ab {date}",
  "aiRates.proposed": "Preis von OpenRouter",
  "aiRates.proposedDetail": "Anbieterpreis · noch nicht freigegeben",
  "aiRates.proposedBasis":
    "Beim Zuordnen geht der Preis in Freigaben. Nutzung und Kosten berücksichtigen ihn nach der Bestätigung.",
  "firstRun.ai.foot":
    "Bis du „Weiter“ wählst, wird nichts an den Anbieter gesendet.",
  "contact.readings.title": "Kontaktstatus",
  "deal360.brief": "Deal-Zusammenfassung",
  "lead.brief.title": "Lead-Bericht",
  "lead.standing.qualified": "Qualifiziert",
  "lead.standing.qualifiedOn":
    "Qualifiziert am {at}. Dieser Lead ist jetzt ein Kontakt.",
  "lead.standing.qualifiedUndated": "Dieser Lead ist jetzt ein Kontakt.",
  "lead.standing.merged": "Zusammengeführt",
  "lead.standing.mergedBecause":
    "Duplikat eines anderen Leads. Sieh dir den anderen Lead an. Dieser Datensatz bleibt als Verlauf erhalten.",
  "lead.standing.closed": "Geschlossen",
  "lead.standing.closedFor":
    "Geschlossen: {reason}. Der Datensatz bleibt als Verlauf erhalten.",
  "lead.standing.closedUnreasoned":
    "Geschlossen. Der Datensatz bleibt als Verlauf erhalten.",
  "lead.standing.yourMove": "Wartet auf deine Antwort",
  "lead.standing.noResponse": "Noch keine Antwort an diesen Lead.",
  "lead.standing.theirMove": "Wartet auf den Lead",
  "lead.standing.answeredOn": "Am {at} beantwortet. Noch keine Rückmeldung.",
  "lead.standing.inMotion": "In Bewegung",
  "lead.standing.engagedBecause":
    "Der Lead hat geantwortet, oder ein Termin ist geplant.",
  "lead.standing.rests.promoted": "Als Kontakt qualifiziert.",
  "lead.standing.rests.merged": "Mit einem anderen Lead zusammengeführt.",
  "lead.standing.rests.closed": "Disqualifiziert, kein Grund erfasst.",
  "lead.standing.rests.ladder": "Lead-Status",
  "lead.standing.rests.record": "Lead-Datensatz",
  "lead.standing.rests.captured": "Erfasst am {at}.",
  "lead.standing.rests.noResponse": "Keine erste Antwort erfasst.",
  "lead.standing.rests.engaged": "Interaktion erfasst {at}.",
  "lead.readings.title": "Lead-Übersicht",
  "lead.readings.firstResponse": "Erste Antwort",
  "lead.readings.noClock": "Keine Zielzeit gesetzt",
  "lead.readings.archived": "Archiviert",
  "lead.readings.merged": "Zusammengeführt",
  "lead.readings.mergedInto": "In einen anderen Lead",
  "lead.readings.company": "Unternehmen",
  "lead.readings.noCompany": "Keins",
  "lead.readings.scoreManual": "Manuell gesetzt",
  "lead.readings.owed": "Ausstehend",
  "lead.today.answer": "{name} antworten",
  "lead.today.answerMeta": "Erste Antwort fällig",
  "lead.today.nextTask": "Nächste Aufgabe",
  "lead.today.reply": "Antworten",
  "lead.today.openTasks": "Aufgaben öffnen",
  "lead.readings.answered": "Beantwortet",
  "lead.standing.dueBy": "Noch keine Antwort. Erste Antwort fällig bis {at}.",
  "lead.standing.overdueSince":
    "Noch keine Antwort. Erste Antwort war fällig am {at}.",
  "stageAutomation.title": "Phasenautomatisierung",
  "stageAutomation.intro":
    "Ergebnisse der Phasenwechsel, die Margince vorgeschlagen hat. Dieser Bericht ändert nichts; er ist der Beleg dafür, einen Übergang Deals automatisch verschieben zu lassen.",
  "stageAutomation.pipeline": "Pipeline",
  "stageAutomation.transition": "\u00dcbergang",
  "stageAutomation.reviewed": "Geprüft",
  "stageAutomation.reviewedHint":
    "Vorschläge, über die jemand entschieden hat. Jede Quote ist ein Anteil dieser Zahl.",
  "stageAutomation.open": "Noch offen",
  "stageAutomation.expired": "Abgelaufen",
  "stageAutomation.expiredHint":
    "Niemand hat geantwortet, bevor das Zeitfenster endete. Das ist keine Ablehnung.",
  "stageAutomation.cleanAcceptance": "Wie vorgeschlagen angenommen",
  "stageAutomation.edits": "Nach Änderungen angenommen",
  "stageAutomation.rejections": "Abgelehnt",
  "stageAutomation.unsafe": "Rückgängig gemacht oder korrigiert",
  "stageAutomation.unsafeHint":
    "Wechsel, die jemand rückgängig gemacht hat oder deren Beleg als falsch markiert wurde. Ein Wechsel zählt einmal, auch wenn beides geschah.",
  "stageAutomation.observationDays": "Beobachtete Tage",
  "stageAutomation.observationHint":
    "Vom ersten bis zum letzten geprüften Vorschlag. Eine gute Quote an einem Nachmittag ist keine belastbare Bilanz.",
  "stageAutomation.evidenceKinds": "Nach Beleg",
  "stageAutomation.empty":
    "Margince hat auf dieser Pipeline noch keinen Phasenwechsel vorgeschlagen.",
  "stageAutomation.noPipelines": "Noch keine Pipelines für einen Bericht.",
  "stageAutomation.unreadable": "Dieser Bericht wurde nicht geladen",
  "stageAutomation.readOnly":
    "Du siehst die Bilanz jedes Übergangs. Um einen Übergang zu ändern, brauchst du die Berechtigung, Pipelines zu bearbeiten.",
  "stageAutomation.rules": "Regeln für Übergänge",
  "stageAutomation.rulesIntro":
    "Das Einschalten eines Übergangs verschiebt noch keine Deals. Margince fragt weiter nach, bis die Bilanz oben die Schwelle erreicht, und verschiebt dann automatisch.",
  "stageAutomation.modeHint":
    "Wenn eingeschaltet und die Bilanz ausreicht, verschiebt Margince den Deal und benachrichtigt dich danach.",
  "stageAutomation.notEarnedYet": "Noch nicht qualifiziert: {why}",
  "stageAutomation.suspended": "Von Margince ausgesetzt",
  "stageAutomation.suspendedSince": "Ausgesetzt am {date}",
  "stageAutomation.resume": "Fortsetzen",
  "stageAutomation.resumeTitle": "Diesen Übergang fortsetzen?",
  "stageAutomation.resumeBody":
    "Margince hat ihn ausgesetzt (Grund: {reason}). Das Fortsetzen überspringt die Schwelle nicht: Der Übergang muss die Bilanz oben erreichen, bevor er Deals automatisch verschiebt.",
  "stageAutomation.noRules":
    "Noch kein Übergang dieser Pipeline hat eine Regel, daher wird jeder Wechsel einer Person zur Prüfung vorgeschlagen.",
  "stageAutomation.undoWindow": "Rückgängig für {hours} h",
  "stageAutomation.rulesLoading": "Regeln für Übergänge werden geladen…",
  "stageAutomation.saveFailed": "Änderung nicht gespeichert",
  "stageAutomation.nothingReviewed": "Vorgeschlagen, aber noch keiner geprüft.",
  "employment.importLoading": "Gekaufter Berufsverlauf wird geladen…",
  "employment.apply": "Importierte Unternehmen verknüpfen",
  "employment.status.current": "Aktuell",
  "employment.status.former": "Ehemalig",
  "employment.status.unknown": "Status unbekannt",
  "employment.statusLabel": "Anstellungsstatus",
  "employment.review": "Prüfe diese Rolle, bevor du sie verknüpfst.",
  "employment.matchNeeded": "Unternehmenszuordnung erforderlich.",
  "employment.resolve": "Unternehmen zuordnen",
  "employment.dismiss": "Beleg verwerfen",
  "employment.research": "Recherche",
  "employment.researchDeferred":
    "Wartet auf Recherche-Einstellungen und KI-Kontingent",
  "employment.website": "Bestätigte Unternehmenswebsite",
  "employment.websiteHint":
    "Wähle oben ein vorhandenes Unternehmen aus oder bestätige seine Website, um es anzulegen.",
  "employment.saveMatch": "Unternehmensverknüpfung speichern",
  "employment.researchNeedsWebsite": "Website erforderlich",
  "employment.research.queued": "Eingereiht",
  "employment.research.running": "Wird recherchiert",
  "employment.research.done": "Abgeschlossen",
  "employment.research.partial": "Teilweise abgeschlossen",
  "employment.research.failed":
    "Fehlgeschlagen. Öffne das Unternehmen, um es erneut zu versuchen.",
  "employment.research.cancelled": "Abgebrochen",
  "employment.researchQueued":
    "Der Recherche-Status wird auf der Unternehmensseite angezeigt.",
  "employment.edit": "Anstellung bearbeiten",
  "employment.start": "Beginn",
  "employment.end": "Ende",
  "employment.dateHint":
    "JJJJ-MM oder JJJJ-MM-TT. Leer lassen, wenn unbekannt.",
  "employment.more": "Weitere Anstellungen anzeigen",

  "magic.title": "Was Margince erledigt hat",
  "magic.since": "Seit {when}",
  "magic.loading": "Wird gelesen, was die Maschinerie getan hat",
  "magic.lane.done": "Für dich erledigt",
  "magic.lane.needsYou": "Wartet auf dich",
  "magic.lane.couldNotComplete": "Konnte nicht abgeschlossen werden",
  "magic.lane.watching": "Muss wiederhergestellt werden",
  "magic.empty.done": "In diesem Zeitraum wurde nichts für dich erledigt.",
  "magic.empty.needsYou": "Keine Entscheidung wartet auf dich.",
  "magic.empty.couldNotComplete": "Alles Begonnene ist angekommen.",
  "magic.empty.watching": "Alle Quellen und Regeln sind in Ordnung.",
  "magic.laneCount_one": "{count} Zeile",
  "magic.laneCount_other": "{count} Zeilen",
  "magic.col.what": "Was passiert ist",
  "magic.col.about": "Betrifft",
  "magic.col.when": "Wann",
  "magic.failingSince": "Fehlerhaft seit {when}",
  "magic.col.wayBack": "Weg zurück",
  "magic.noRecord": "Kein Datensatz genannt",
  "magic.undo.fromHistory":
    "Lässt sich über den Verlauf des Datensatzes zurücknehmen",
  "magic.undoReason.noCompletedChange":
    "Es hat sich nichts geändert, also gibt es nichts zurückzunehmen.",
  "magic.undoReason.notEvaluated":
    "Diese Installation hat nicht geprüft, ob sich das zurücknehmen lässt.",
  "magic.notShown_one": "{count} Änderung wird nicht gezeigt: {reason}",
  "magic.notShown_other": "{count} Änderungen werden nicht gezeigt: {reason}",
  "magic.notShown.unadmittedAction": "Routinearbeit ohne Aussage für dich",
  "magic.notShown.unknownEntityType":
    "ein Datensatztyp, den diese Seite nicht einordnen kann",
  "magic.notShown.outOfScope": "außerhalb deiner eigenen Datensätze",
  "magic.action.advance_stage": "Ein Deal ist eine Phase weitergerückt",
  "magic.action.promote": "Aus einem Lead wurde ein Deal",
  "magic.action.update": "Ein Datensatz wurde aktualisiert",
  "magic.action.assign": "Arbeit wurde neu zugewiesen",
  "magic.action.activity_relink":
    "Ein Austausch wurde dem richtigen Datensatz zugeordnet",
  "magic.action.send_email": "Eine Nachricht wurde gesendet",
  "magic.action.schedule": "Ein Termin wurde gebucht",
  "magic.action.disqualify": "Ein Lead wurde disqualifiziert",
  "magic.action.automation_troubled": "{name} steckt fest: {outcome}",
  "magic.action.approval_coldstart": "Eine Erstansprache wartet auf dein Wort",
  "magic.action.approval_send_email": "Eine Nachricht wartet auf dein Wort",
  "magic.action.approval_advance_deal":
    "Ein Phasenwechsel wartet auf dein Wort",
  "magic.action.approval_promote_lead":
    "Die Umwandlung eines Leads wartet auf dein Wort",
  "magic.action.approval_overnight":
    "Ein Vorschlag aus der Nacht wartet auf dein Wort",
  "magic.action.approval_transcript_proposal":
    "Ein Vorschlag aus einer Aufzeichnung wartet auf dein Wort",
  "magic.action.approval_pending":
    "Ein Vorschlag vom Typ {kind} wartet auf dein Wort",
  "magic.action.capture_reauth_required":
    "{provider} muss neu verbunden werden",
  "magic.action.capture_connection_error": "{provider} war nicht erreichbar",
  "magic.action.capture_sync_failing": "{provider} kommt nicht hinterher",
  "magic.action.capture_backfill_failed":
    "{provider} konnte die eigene Historie nicht zu Ende lesen",
  "magic.consequence.stage_moved":
    "Der Deal steht jetzt in einer späteren Phase.",
  "magic.consequence.lead_promoted": "In der Pipeline steht ein neuer Deal.",
  "magic.consequence.owner_changed": "Jetzt ist jemand anders zuständig.",
  "magic.consequence.record_relinked":
    "Der Austausch steht jetzt beim richtigen Datensatz.",
  "magic.consequence.message_sent":
    "Empfangende haben sie, zurückholen geht nicht.",
  "magic.consequence.meeting_booked": "Der Termin steht im Kalender.",
  "magic.consequence.lead_disqualified":
    "Der Lead ist aus der Pipeline heraus.",
  "magic.consequence.automation_did_nothing":
    "Nichts von dem, was die Regel zugesagt hat, ist passiert.",
  "magic.consequence.awaits_your_decision":
    "Es passiert nichts, bis du entscheidest.",
  "magic.consequence.capture_not_collecting":
    "Aus dieser Quelle wird nichts mehr erfasst.",
  "magic.consequence.capture_may_be_incomplete":
    "Was du aus dieser Quelle siehst, kann unvollständig sein.",
  "magic.consequence.capture_history_incomplete":
    "Älterer Austausch aus dieser Quelle fehlt.",
} as const satisfies Record<MessageKey, string>;
