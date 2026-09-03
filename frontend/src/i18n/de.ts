import type { MessageKey } from "./en";

// German catalog — the A24 default locale. `satisfies` forces exact key
// parity with en at compile time; i18n.test.ts re-checks it at runtime so a
// build without typechecking still fails loudly. Register: informal "du",
// natural spoken-adjacent German, no translationese, no corporate filler.
export const de = {
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

  "trust.accept": "Übernehmen",
  "trust.edit": "Bearbeiten",
  "trust.dismiss": "Verwerfen",
  "trust.save": "Speichern",
  "trust.typedByYou": "von dir eingetragen",
  "trust.typedByHuman": "von einer Person eingetragen",
  "trust.typedByBuyer": "von einem Käufer eingetragen",
  "trust.typedByPrefix": "eingetragen von",
  "trust.sourceUnknown": "Herkunft nicht erfasst",
  "trust.agentTag": "Automatisiert durch {agent}",
  "trust.agentUnnamed": "Automatisiert durch einen Agenten",
  "trust.systemTag": "Systemvorgang {job}",
  "trust.systemUnnamed": "Systemvorgang",
  "trust.connectorTag": "über {connector}",
  "trust.dismissed": "Vorschlag verworfen.",
  "trust.stagedProposal": "vorgemerkter Vorschlag",
  "trust.resolvedValue": "übernommener Wert",
  "trust.editValue": "{description} bearbeiten",
  "trust.evidenceFrom": "Beleg von {source}",
  "trust.evidenceLine_one": "Zeile {lines}",
  "trust.evidenceLine_other": "Zeilen {lines}",

  "history.created": "— angelegt —",
  "history.oldValue": "Vorheriger Wert",
  "history.newValue": "Neuer Wert",
  "history.cleared": "— geleert —",
  "history.passport": "Agent-Passport",
  "history.empty": "Keine Änderungen erfasst",
  "history.fieldEmpty":
    "Beim Anlegen gesetzt und nie geändert — der Audit-Log verzeichnet keine Bearbeitungen. Ein leerer Verlauf ist ehrlich, keine Lücke.",
  "history.filterEmpty": "Keine Änderungen entsprechen diesem Filter.",
  "history.clearFilter": "Filter zurücksetzen",
  "history.allFields": "Alle Felder",
  "history.actorAll": "Alle",
  "history.actorHuman": "Mensch",
  "history.actorAgent": "Agent",
  "history.tabChanges": "Nach Änderung",
  "history.tabFields": "Nach Feld",
  "history.undo.action": "Zurücksetzen",
  "history.undo.redo": "Erneut anwenden",
  "history.undo.busy": "Änderung wird zurückgesetzt…",
  "history.undo.confirmTitle": "Diese Änderung zurücksetzen?",
  "history.undo.confirmEdgeBody":
    "Dies ändert die Verknüpfung mit {other}. Die Datensätze bleiben; nur die Verbindung zwischen ihnen ändert sich.",
  "history.undo.confirmBody":
    "{count} Felder kehren auf den Stand vor dieser Änderung zurück:",
  "history.undo.versionSkew":
    "Der Datensatz hat sich beim Lesen verändert. Der Verlauf wurde neu geladen — prüfen Sie die Änderung erneut, bevor Sie sie zurücksetzen.",
  "history.undo.noBeforeImage":
    "Diese Änderung hat nicht festgehalten, was der Datensatz vorher enthielt — es gibt nichts zurückzusetzen.",
  "history.undo.notReplayable":
    "Diese Art von Änderung wird nicht rückwärts abgespielt.",
  "history.undo.unsupportedRecordType":
    "Änderungen an dieser Art von Datensatz lassen sich nicht zurücksetzen.",
  "history.undo.superseded":
    "Jemand hat diese Felder seitdem geändert. Ein Zurücksetzen würde auch diese Entscheidung aufheben.",
  "history.undo.behindErasureBoundary":
    "Diese Änderung liegt hinter einer Löschung; ihr Inhalt wurde endgültig entfernt.",
  "history.undo.alreadyUndone": "Diese Änderung wurde bereits zurückgesetzt.",
  "history.undo.notRestorableByThisPath":
    "Diese Felder werden nicht über den Weg geschrieben, den ein Zurücksetzen nimmt.",
  "history.undo.recordArchived":
    "Der Datensatz ist archiviert. Holen Sie ihn zuerst zurück, bevor Sie eine Änderung zurücksetzen.",
  "history.undo.nullUnwritable":
    "Ein Zurücksetzen müsste ein Feld leeren, das dieser Datensatz nicht leeren kann, und ist daher nicht möglich.",
  "history.undo.notWritableByCaller":
    "Sie haben keine Berechtigung, diese Felder zu schreiben.",
  "history.undo.edgeRelinkUnsupported":
    "Eine entfernte Verknüpfung wiederherzustellen ist noch nicht möglich — legen Sie sie auf diesem Datensatz erneut an.",
  "history.reversal.collapsed":
    "Änderung von {actor}, zurückgesetzt von {undoer}",
  "history.reversal.collapsedSelf":
    "{actor} hat die eigene Änderung zurückgesetzt",
  "history.reversal.partly":
    "Änderung von {actor}, teilweise zurückgesetzt von {undoer}",
  "history.reversal.partlySelf":
    "{actor} hat die eigene Änderung teilweise zurückgesetzt",
  "history.reversal.net": "Ergebnis: unverändert",
  "history.reversal.stillChanged": "weiterhin geändert",
  "history.reversal.expand": "Beide Änderungen anzeigen",
  "history.reversal.collapse": "Ausblenden",
  "history.reversal.undoneBy": "zurückgesetzt von {undoer}",
  "history.reversal.unpaired": "setzt eine frühere Änderung zurück",
  "history.edge.marker": "Verknüpfung",
  "history.field.address": "Adresse",
  "history.field.amount_minor": "Wert",
  "history.field.assignee_id": "Zuständig",
  "history.field.body": "Notizen",
  "history.field.candidate_org_key": "Zugeordnetes Unternehmen",
  "history.field.company_name": "Firmenname",
  "history.field.currency": "Währung",
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
  "history.field.linkedin_url": "LinkedIn-URL",
  "history.field.lost_reason": "Verlustgrund",
  "history.field.name": "Name",
  "history.field.occurred_at": "Zeitpunkt",
  "history.field.organization_id": "Unternehmen",
  "history.field.owner_id": "Verantwortlich",
  "history.field.parent_org_id": "Muttergesellschaft",
  "history.field.partner_attribution": "Partnerzuordnung",
  "history.field.partner_org_id": "Partner",
  "history.field.project_id": "Projekt",
  "history.field.relationship_types": "Beziehungsarten",
  "history.field.remind_at": "Erinnerung",
  "history.field.score": "Score",
  "history.field.score_override_reason": "Grund für die Score-Anpassung",
  "history.field.size_band": "Größe",
  "history.field.social": "Social-Profile",
  "history.field.source": "Quelle",
  "history.field.started_at": "Beginn",
  "history.field.status": "Status",
  "history.field.subject": "Betreff",
  "history.field.target_end_date": "Geplantes Ende",
  "history.field.title": "Position",
  "history.field.wait_until": "Wartet bis",
  "history.emptyList": "nichts gesetzt",

  "confidence.high": "hoch",
  "confidence.med": "mittel",
  "confidence.low": "niedrig",

  "autonomy.auto": "automatisch",
  "autonomy.confirm": "erst bestätigen",

  "nav.home": "Briefing",
  "nav.contacts": "Kontakte",
  "nav.companies": "Firmen",
  "nav.leads": "Leads",
  "nav.deals": "Pipeline",
  "nav.today": "Arbeitsliste",
  "nav.analytics": "Analytics",
  "nav.ai": "Margince fragen",
  "nav.settings": "Einstellungen",
  "nav.automations": "Automatisierungen",
  "nav.group.records": "Datensätze",
  "nav.group.work": "Arbeit",
  "nav.group.intelligence": "Auswertung",
  "nav.offers": "Angebot",
  "nav.share": "Freigabe",
  "nav.search": "Suchergebnisse",
  "nav.tags": "Tag",

  "shell.railAria": "Hauptnavigation",
  "shell.aside.hide": "Ausblenden",
  "shell.aside.show": "Kontextspalte einblenden",
  "shell.skipToContent": "Zum Inhalt springen",
  "shell.logoAria": "Margince",
  "shell.companyLogoAria": "{company} Startseite, betrieben mit Margince",
  "shell.poweredBy": "Betrieben mit Margince",
  "shell.alpha": "Alpha",
  "shell.searchEverything": "Alles durchsuchen…",
  "shell.breadcrumbAria": "Navigationspfad",
  "shell.license.none": "Keine Lizenz",
  "shell.license.refused": "Lizenz abgelehnt",
  "shell.signOutAria": "Abmelden",
  "shell.collapse": "Seitenleiste einklappen",
  "shell.expand": "Seitenleiste ausklappen",
  "shell.accountAria": "Konto",
  "shell.theme": "Design",
  "shell.more": "Mehr",
  "shell.unknownPage": "Nicht gefunden",
  "shell.closeMenu": "Schließen",
  "shell.navBack": "Zurück",
  "shell.navBackTo": "Zurück zu {name}",
  "shell.navTop": "Bereiche",
  "shell.sectionSwitch": "{name} — Bereich wechseln",
  "attention.selected": "{n} ausgewählt",
  "locale.name.en": "English",
  "locale.name.de": "Deutsch",
  "locale.name.vi": "Tiếng Việt",
  "locale.switchLabel": "Sprache",

  "screen.pending":
    "Noch nicht gebaut — diese Oberfläche kommt mit ihrem Build-Ticket.",

  "ext.notFound":
    "Auf dieser Installation ist keine Erweiterung namens „{name}“ aktiviert.",
  "ext.operations": "Veröffentlichte Operationen",

  "search.placeholder":
    "Personen, Firmen, Deals, Aktivitäten, Leads durchsuchen…",
  "search.prompt": "Gib ein, wonach du suchst.",
  "search.empty": "Keine Treffer für „{q}“.",
  "search.group.person": "Personen",
  "search.group.organization": "Organisationen",
  "search.group.deal": "Deals",
  "search.group.activity": "Aktivitäten",
  "search.group.lead": "Leads",
  "search.group.tag": "Tags",
  "search.tag.carriedBy": "Auf {count} Datensätzen",
  "search.tier.mirrored": "aus einem verbundenen System",
  "search.tier.unverified": "nicht verifiziert",

  "context.title": "Verwandte Belege",
  "context.empty": "Noch nichts Verwandtes.",

  "palette.aria": "Befehlspalette",
  "palette.placeholder": "Wohin? Oder frag einfach etwas…",
  "palette.empty": "Keine Treffer.",
  "palette.askAi": "KI fragen: „{query}“",
  "palette.typeScreen": "Ansicht",
  "palette.typeAction": "Aktion",
  "palette.typeRecord": "Datensatz",
  "palette.seeAll": "Alle Ergebnisse für „{query}“ anzeigen",
  "action.newDeal": "Neuer Deal",
  "action.readCompany": "Firma einlesen",
  "action.booking": "Buchungsseite",

  "common.undo": "Rückgängig",
  "common.close": "Schließen",

  "explain.open": "Diese Zahl erklären",
  "explain.title": "So setzt sich die Zahl zusammen",
  "explain.rate": "Kurs {rate} am {date}",

  "board.count": "{count} Deals",
  "board.weighted": "gewichtet {value}",
  "board.mixedCurrencies": "mehrere Währungen — keine Gesamtsumme",
  "dealfiles.hidden": "Von diesem Deal ausgeblendet",
  "dealfiles.unhidden": "Wieder an diesem Deal sichtbar",
  "deal.stalled": "stockt",
  "deal.archived": "archiviert",
  "deal.singleThreaded": "nur ein Kontakt",
  "deal.staged": "vorgemerkt",
  "record.notShown": "Nicht angezeigt",
  "record.timelineLoading": "Verlauf dieses Datensatzes wird geladen…",
  "record.timeline": "Verlauf",
  "record.edit": "Bearbeiten",
  "record.save": "Speichern",
  "record.saveDone": "„{name}“ gespeichert",
  "record.archiveDone": "„{name}“ archiviert",
  "record.archive": "Archivieren",
  "record.disqualify": "Disqualifizieren",
  "record.archiveConfirm":
    "Bist du sicher? Das archiviert den Datensatz — es gibt keine Rückgängig-Funktion.",
  "record.archived": "Archiviert",
  "record.archivedReadOnly":
    "Diese Firma ist archiviert. Stellen Sie sie wieder her, um etwas zu ändern.",
  "record.notYoursToChange":
    "Diese Firma gehört jemand anderem. Bitten Sie den Inhaber um Freigabe, wenn Sie etwas ändern möchten.",
  "record.logActivityRefused":
    "Sie haben keine Berechtigung, Aktivitäten zu diesem Datensatz zu erfassen.",
  "record.share": "Teilen",
  "record.moreActions": "Weitere Aktionen",
  "record.fullHistory": "Vollständiger Verlauf",

  "share.title": "Diesen Datensatz teilen",
  "share.ceiling.pre": "Eine Freigabe ändert, wer ",
  "share.ceiling.recordEmphasis": "genau diesen einen Datensatz",
  "share.ceiling.mid":
    " sehen kann — sonst nichts am Zugriffsbereich einer Person. Eine Freigabe ist auf deinen eigenen Zugriff begrenzt, ",
  "share.ceiling.noWider": "nicht weiter",
  "share.ceiling.post": ".",
  "share.unknownRecord": "Dieser Datensatztyp kann nicht geteilt werden.",
  "share.grantAccess": "Zugriff gewähren",
  "share.subject": "Person oder Team",
  "share.holdsRead": "Hat Lesezugriff",
  "share.holdsWrite": "Hat Schreibzugriff",
  "share.kindPerson": "Person",
  "share.kindTeam": "Team",
  "share.access": "Zugriffsstufe",
  "share.access.read": "Lesen",
  "share.access.write": "Schreiben",
  "share.access.readNote":
    "Kann diesen Datensatz öffnen und lesen — nicht bearbeiten oder senden.",
  "share.access.writeNote":
    "Kann diesen Datensatz öffnen, bearbeiten und ergänzen — nicht Eigentümerschaft oder Freigaben ändern.",
  "share.expiry": "Ablauf",
  "share.expiry.none": "Kein Ablauf (bis zum Widerruf)",
  "share.expiry.day": "Läuft in 24 Stunden ab",
  "share.expiry.week": "Läuft in 7 Tagen ab",
  "share.expiry.month": "Läuft in 30 Tagen ab",
  "share.expiryConsequence_one":
    "Der Zugriff endet automatisch in {days} Tag. Sie können ihn jederzeit früher widerrufen.",
  "share.expiryConsequence_other":
    "Der Zugriff endet automatisch in {days} Tagen. Sie können ihn jederzeit früher widerrufen.",
  "share.expiryConsequenceNone":
    "Der Zugriff bleibt bestehen, bis Sie ihn widerrufen — er endet nicht von selbst.",
  "share.reason": "Grund",
  "share.grant": "Zugriff gewähren",
  "share.update": "Zugriff ändern",
  "share.unchanged":
    "Nichts geändert. {name} hatte bereits {access}-Zugriff auf diesen Datensatz.",
  "share.downgradeTitle": "Zugriff reduzieren?",
  "share.downgradeBody":
    "{name} hat {from}-Zugriff auf diesen Datensatz. Wenn Sie fortfahren, bleibt nur noch {to}-Zugriff. Beide Richtungen werden im Audit-Protokoll festgehalten.",
  "share.downgradeConfirm": "Auf {to} reduzieren",
  "share.seatCeiling":
    "Dieser Sitzplatz ist nur lesend und kann daher keinen Schreibzugriff auf einen Datensatz erhalten. Erhöhen Sie zuerst die Sitzplatzstufe, oder gewähren Sie Lesezugriff.",
  "share.whoHasAccess": "Wer hat Zugriff",
  "share.grantedBy": "gewährt von",
  "share.revoke": "Widerrufen",
  "share.revokeConfirm":
    "Diese Freigabe widerrufen? Der Zugriff auf diesen Datensatz entfällt beim nächsten Request — es gibt keine Rückgängig-Funktion.",
  "share.approvalRequired":
    "Diese Freigabe braucht erst eine Genehmigung — sie wartet auf eine Entscheidung und ist noch nicht angewendet.",
  "share.teamMembers_one": "Team · {count} Mitglied",
  "share.teamMembers_other": "Team · {count} Mitglieder",
  "share.rosterLoading": "Personen und Teams werden geladen…",
  "share.rosterErrorUsers":
    "Personenliste konnte nicht geladen werden — Teams werden unten angezeigt.",
  "share.rosterErrorTeams":
    "Teamliste konnte nicht geladen werden — Personen werden unten angezeigt.",
  "share.rosterErrorBoth": "Personen und Teams konnten nicht geladen werden.",
  "share.rosterEmpty": "Keine freigebbaren Personen oder Teams gefunden.",

  "edit.versionSkew":
    "Dieser Datensatz hat sich geändert, seit du ihn geöffnet hast — neu laden und erneut versuchen.",

  "merge.person": "Kontakt zusammenführen",
  "merge.org": "Firma zusammenführen",
  "merge.searchPlaceholder": "Suchen…",
  "merge.pickTarget": "Überlebenden Datensatz auswählen",
  "merge.confirm":
    "{source} in {target} zusammenführen? {source} wird archiviert.",
  "merge.submit": "Zusammenführen",

  "tab.overview": "Übersicht",
  "tab.relationships": "Personen & Firmen",
  "tab.partner": "Partner",
  "tab.rollup": "Roll-up",
  "tab.history": "Verlauf",

  "rollup.weightedPipeline": "Gewichtete Pipeline",
  "rollup.closedWon": "Abgeschlossen (aktuelles Quartal)",
  "rollup.activity30d": "Aktivität (30 Tage)",
  "rollup.accounts": "Zusammengefasste Accounts",
  "rollup.excluded":
    "{count} für dich nicht sichtbare Account(s) wurden ausgeschlossen",
  "rollup.fxUnavailable":
    "Ein Wechselkurs fehlt — das Roll-up kann nicht berechnet werden.",
  "rollup.computedAt": "Berechnet am {when}",

  "nav.partners": "Partner",
  "deal.partnerSourced": "über",
  "deal.partnerInfluenced": "unterstützt von",
  "deal.partnerAttribution": "Was der Partner getan hat",
  "deal.attributionUnset": "Nicht angegeben — gilt als gebracht",
  "deal.attributionSourced": "Hat diesen Deal gebracht (mit Provision)",
  "deal.attributionInfluenced":
    "Hat bei einem bestehenden Deal geholfen (ohne Provision)",
  "partnerDeals.panelTitle": "Gebrachte Deals",
  "partnerDeals.panelSub":
    "Deals bei anderen Firmen, die über diesen Partner zustande kamen",
  "partnerDeals.none": "Noch keine Deals gebracht",
  "partnerDeals.column.deal": "Deal",
  "partnerDeals.column.customer": "Kunde",
  "partnerDeals.column.attribution": "Sein Anteil",
  "partnerDeals.column.amount": "Deal-Wert",
  "partnerDeals.column.status": "Status",
  "commission.panelTitle": "Provision",
  "commission.panelSub":
    "Was dieser Partner an selbst gebrachten Deals verdient hat",
  "commission.none": "Noch nichts verdient",
  "commission.column.deal": "Deal",
  "commission.column.amount": "Verdient",
  "commission.column.rate": "Satz",
  "commission.column.basis": "Deal-Wert",
  "commission.column.status": "Status",
  "commission.status.accrued": "Aufgelaufen",
  "commission.status.approved": "Freigegeben",
  "commission.status.paid": "Ausgezahlt",
  "commission.status.void": "Storniert",
  "commission.outstanding": "Noch offen",
  "commission.column.actions": "Entscheidung",
  "commission.decide.withheld": "Nicht Ihre Entscheidung",
  "commission.decide.approve": "Freigeben",
  "commission.decide.pay": "Als ausgezahlt markieren",
  "commission.decide.void": "Stornieren",
  "commission.decide.approveConfirm":
    "Mit der Freigabe halten Sie fest, dass diese Provision vereinbart ist. Ausgezahlt wird dadurch nichts — zahlen Sie in Ihrem Finanzsystem und markieren Sie es danach hier.",
  "commission.decide.payConfirm":
    "Markieren Sie erst als ausgezahlt, wenn Ihr Finanzsystem tatsächlich gezahlt hat. Margince hält die Tatsache fest und bewegt kein Geld.",
  "commission.decide.voidConfirm":
    "Eine Stornierung schreibt eine Gegenbuchung daneben. Nichts wird gelöscht, der ursprüngliche Eintrag bleibt lesbar.",
  "commission.decide.reasonLabel": "Warum wird storniert?",
  "commission.decide.reasonRequired":
    "Eine Stornierung braucht einen Grund — damit lässt sie sich dem Partner später erklären.",
  "commission.decide.approved": "Provision freigegeben",
  "commission.decide.paid": "Provision als ausgezahlt markiert",
  "commission.decide.voided": "Provision storniert",
  "commission.decide.settledElsewhere":
    "Ausgezahlt wird im Finanzsystem. Hier wird festgehalten, was dort passiert ist.",
  "partner.setup": "Zum Partner machen",
  "partner.edit": "Partner bearbeiten",
  "partner.none": "Noch kein Partner",
  "partner.organization": "Organisation",
  "partner.role": "Partnerrolle",
  "partner.roleAll": "Alle Rollen",
  "partner.certStatus": "Zertifizierungsstatus",
  "partner.certStatusAll": "Alle Status",
  "partner.marginTier": "Margen-Stufe",
  "partner.stage": "Beziehungsphase",
  "partner.nextStep": "Nächster Schritt",
  "partner.nextStepDue": "Nächster Schritt fällig",
  "partner.servedSegments": "Betreute Segmente",
  "partner.servedSegmentsHint": "kommagetrennt",
  "partner.role.hosting": "Hosting",
  "partner.role.consulting": "Beratung",
  "partner.role.strategic": "Strategisch",
  "partner.cert.applied": "Beantragt",
  "partner.cert.certified": "Zertifiziert",
  "partner.cert.suspended": "Ausgesetzt",
  "partner.marginTier.tier1": "Intro (15 %)",
  "partner.marginTier.tier2": "Aktive Zusammenarbeit (20 %)",
  "partner.marginTier.tier3": "Partner hat abgeschlossen (25 %)",
  "partner.stage.research": "Recherche",
  "partner.stage.identified": "Identifiziert",
  "partner.stage.contacted": "Kontaktiert",
  "partner.stage.inConversation": "Im Gespräch",
  "partner.stage.fitConfirmed": "Passung bestätigt",
  "partner.stage.agreementPending": "Vereinbarung ausstehend",
  "partner.stage.active": "Aktiv",
  "partner.stage.activeReferring": "Aktiv — empfiehlt",
  "partner.stage.dormant": "Ruhend",
  "partner.stage.noFit": "Keine Passung",

  "rel.add": "Beziehung hinzufügen",
  "rel.addStakeholder": "Beteiligten hinzufügen",
  "rel.dealStakeholders": "Beteiligte",
  "rel.dealStakeholdersEmpty": "Für diesen Deal ist niemand erfasst",
  "rel.kind": "Art",
  "rel.saveDone": "Beziehung gespeichert",
  "rel.role": "Rolle",
  "rel.startedAt": "Beginn",
  "rel.endedAt": "Ende",
  "rel.current": "aktuell",
  "rel.endedOn": "bis {when}",
  "rel.remove": "Entfernen",
  "rel.removeConfirm":
    "Bist du sicher? Das entfernt die Beziehung — es gibt keine Rückgängig-Funktion.",
  "rel.empty": "Noch keine Beziehungen",
  "rel.counterparty": "Verknüpft mit",
  "rel.dates": "Zeitraum",
  "rel.pickCounterparty": "Die andere Seite auswählen",
  "rel.addConfirm": "{kind}-Verknüpfung zu {target} hinzufügen.",
  "rel.kind.employment": "Anstellung",
  "rel.kind.dealStakeholder": "Deal-Beteiligter",
  "rel.kind.projectStakeholder": "Projekt-Beteiligter",
  "rel.kind.projectCompany": "Unternehmen im Projekt",
  "rel.kind.partnerOf": "Partner von",
  "rel.kind.referredBy": "Empfohlen von",
  "rel.kind.coSellWith": "Co-Sell mit",
  "rel.kind.worksWith": "Arbeitet zusammen mit",

  "common.error": "Konnten diese Ansicht nicht laden.",
  "common.errorNoCause":
    "Die Anfrage ist fehlgeschlagen. Keine Ursache gemeldet.",
  "common.assistantUnavailable":
    "Der Assistent hat nicht geantwortet und kann das hier nicht entwerfen. Eine Administratorin oder ein Administrator kann die Modellbindung unter Einstellungen → KI prüfen. Nötig ist er nicht — die Angaben lassen sich von Hand eintragen.",
  "common.gatewayUnavailable":
    "Der Server hat diese Anfrage nicht rechtzeitig abgeschlossen. Sie läuft möglicherweise noch — warte einen Moment, bevor du es erneut versuchst, sonst läuft dieselbe Arbeit zweimal.",
  "common.permissionDenied":
    "Du hast keine Berechtigung für diese Aktion. Bitte einen Admin oder die Person, die diesen Datensatz mit dir geteilt hat, deinen Zugriff zu erweitern.",
  "common.seatReadOnly":
    "Dieser Sitzplatz ist nur lesend, daher wurde die Anfrage abgelehnt. Bitte einen Betreiber, den Sitzplatz höherzustufen.",
  "common.retry": "Erneut versuchen",
  "common.empty": "Hier ist noch nichts.",
  "common.saving": "Wird gespeichert…",
  "common.loading": "Wird geladen…",
  "ref.nameLoadFailed": "Name konnte nicht geladen werden",
  "ref.notInRoster": "Aktuell zugeordnet (nicht mehr in der Nutzerliste)",

  // "Funktioniert nicht mehr", nicht "Fehler aufgetreten": die Ansicht ist
  // stehengeblieben, und das ist die Beobachtung, die der Lesende selbst
  // machen kann. Kein Wort über den Fehler.
  "app.errorTitle": "Diese Ansicht funktioniert nicht mehr.",
  "app.errorBody":
    "Versuch es noch einmal. Wenn es weiter fehlschlägt, lade die Seite neu.",
  "app.errorRetry": "Erneut versuchen",

  // Die Grenze um EINE Karte: sagt weniger als die App-Grenze, weil sie
  // weniger genommen hat — Seite und Navigation stehen noch.
  "card.errorTitle": "Diese Karte funktioniert nicht mehr.",
  "card.errorRetry": "Erneut versuchen",

  // Das neunteilige Zustandsvokabular (design-system/surfacestate.tsx):
  // gehört dem ZUSTAND, nicht einer einzelnen Fläche.
  "state.withheld": "Ausgeblendet — deine Rolle darf das nicht lesen",
  "state.unavailable":
    "Konnte nicht geladen werden — das ist möglicherweise nicht das ganze Bild",
  "state.unsupported":
    "In diesem Modus nicht verfügbar — das angebundene System führt es nicht",
  "state.failed": "Dieser Abschnitt wurde nicht geladen.",
  "state.loading": "Dieser Abschnitt wird geladen…",
  "state.retry": "Erneut versuchen",
  "state.stale": "Zuletzt bekannte Werte — seitdem nicht aktualisiert",
  "state.staleAsOf": "Zuletzt bekannte Werte, Stand {when}",
  "state.partial": "Nur ein Teil der Liste",
  "state.partialCount": "{count} weitere nicht angezeigt",

  "list.search": "Suchen",
  "list.showArchived": "Archivierte anzeigen",
  "list.loadMore": "Mehr laden",
  "list.viewAll": "Alle",
  "list.viewAZ": "A–Z",
  "list.viewHot": "Heiß",
  "list.overlayReadOnly":
    "Sortierung und Filter laufen über HubSpot — dort öffnen",

  "table.range": "{first}–{last} von {count} {unit}",
  "table.pagination": "Seiten",
  "table.page": "Seite {number}",
  "table.prev": "‹ Zurück",
  "table.next": "Weiter ›",
  "table.rowsPerPage": "Zeilen pro Seite",
  "table.perPage": "{count} pro Seite",
  "table.sortedBy": "sortiert nach {column}",
  "table.columns": "Spalten",
  "table.shownColumns": "Sichtbare Spalten",
  "table.compact": "Kompakt",
  "table.sort": "Sortieren",
  "table.sortMenu": "Sortieren nach",
  "table.sortDefault": "Standardreihenfolge",
  "table.sortAscending": "aufsteigend",
  "table.sortDescending": "absteigend",
  "table.sortBy": "Nach {column} sortieren",
  "table.noMatches": "Keine {unit} passen zu diesen Filtern.",
  "table.clearFilters": "Filter zurücksetzen",
  "table.none": "Noch keine {unit}.",
  "table.actions": "Aktionen",
  "table.rangeLoaded": "{first}–{last} von bisher {count} geladenen {unit}",
  "unit.contacts": "Kontakte",
  "unit.companies": "Firmen",
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
  "table.filterValueSearch": "{filter}-Werte durchsuchen",
  "table.filterTypeToSearch": "Zum Suchen tippen",
  "table.filterSearching": "Suche läuft…",
  "table.filterSearchFailed":
    "Die Suche ist fehlgeschlagen. Bitte erneut versuchen.",
  "table.filterNoMatches": "Keine Treffer.",
  "overlay.unavailable":
    "In der HubSpot-Ansicht nicht verfügbar — in HubSpot öffnen",
  "overlay.chipLabel": "Liest aus HubSpot",
  "overlay.chipAria":
    "Diese Installation liest Datensätze aus einem HubSpot-Spiegel statt aus nativen Tabellen. Öffne Einstellungen → Integrationen, um die Verbindung zu verwalten.",
  "overlay.refused":
    "Beim Lesen aus HubSpot nicht verfügbar — der Spiegel kann diesen Schreibvorgang nicht ausführen.",
  "overlay.filterUnsupported":
    "Dieser Filter oder diese Sortierung ist beim Lesen aus HubSpot nicht verfügbar — bitte entfernen und erneut versuchen.",
  "overlay.emptyOwnerHint":
    "Eine leere Liste bedeutet hier meist, dass die HubSpot-E-Mail des Owners keinem Benutzer dieser Organisation entspricht — nicht, dass das HubSpot-Portal leer ist.",
  "overlay.partialWriteBack":
    "Nur die Felder, die HubSpot akzeptiert, werden zurückgeschrieben — alles andere hier, einschließlich Custom Fields und Owner, wird überhaupt nicht angewendet; der aktuelle Wert in HubSpot bleibt bestehen.",

  "overlay.title": "HubSpot-Spiegel",
  "overlay.sub":
    "Verbindet das führende CRM der Organisation, damit Datensätze aus dessen Spiegel statt aus nativen Tabellen gelesen werden.",
  "overlay.loading": "Lade die Anbieter-Verbindung…",
  "overlay.notConfigured":
    "Overlay-Modus ist in diesem Deployment nicht konfiguriert.",
  "overlay.loadFailed": "Die Anbieter-Verbindung konnte nicht geladen werden.",
  "overlay.empty":
    "Kein führendes System verbunden. Verbinde HubSpot, um Datensätze aus dessen Spiegel zu lesen.",
  "overlay.adminOnly":
    "Du hast keine Berechtigung, die HubSpot-Verbindung zu ändern.",
  "overlay.region": "Region",
  "overlay.regionEu1": "EU",
  "overlay.connectionLabel": "Verbindung",
  "overlay.notConnectedYet": "Nicht verbunden",
  "overlay.regionUs": "USA",
  "overlay.token": "Private-App-Token",
  "overlay.tokenHint": "Wird im Vault versiegelt; wird nie wieder angezeigt.",
  "overlay.connect": "HubSpot verbinden",
  "overlay.reconnect": "Erneut verbinden",
  "overlay.connectConfirmTitle":
    "HubSpot für die ganze Organisation verbinden?",
  "overlay.reconnectConfirmTitle":
    "HubSpot für die ganze Organisation erneut verbinden?",
  "overlay.connectConfirmBody":
    "Dies schaltet die Lesezugriffe aller Sitze sofort auf den HubSpot-Spiegel um, und Datensätze werden schreibgeschützt, wo immer der Spiegel kein Schreiben unterstützt. Dies betrifft die gesamte Installation, nicht nur die eigene Sitzung.",
  "overlay.statusActive": "Verbunden",
  "overlay.statusRevoked": "Widerrufen",
  "overlay.statusError": "Sync-Fehler",
  "overlay.connectedAt": "Verbunden {at}",
  "overlay.syncTitle": "Spiegel-Synchronisierung",
  "overlay.syncLoadFailed": "Sync-Status konnte nicht geladen werden.",
  "overlay.syncEmpty": "Noch nichts synchronisiert.",
  "overlay.syncStateFresh": "Aktuell",
  "overlay.syncStatePending": "Sync ausstehend",
  "overlay.syncStateStale": "Veraltet",
  "overlay.backfillDone": "Backfill abgeschlossen",
  "overlay.backfillPending": "Backfill läuft",
  "overlay.lastSynced": "Zuletzt synchronisiert {at}",
  "overlay.neverSynced": "Noch nie synchronisiert",
  "overlay.budgetTitle": "API-Budget",
  "overlay.budgetLoadFailed": "Das Budget-Fenster konnte nicht geladen werden.",
  "overlay.budgetHeadroom": "Spielraum: {headroom}",
  "overlay.budgetUnmeasured":
    "Das Aufrufbudget kann gerade nicht gemessen werden, Live-Aufrufe pausieren vorsorglich. Das ist kein HubSpot-Quotendruck — der Zähler selbst liefert keine Werte.",
  "overlay.budgetEmpty":
    "Das Altsystem hat für diesen Zeitraum kein Budgetfenster gemeldet.",
  "overlay.budgetSources":
    "Force-Fresh {forceFresh} · Poller {poller} · Capture {capture}",
  "overlay.budgetSearch": "Such-API: {consumed} / {limit} pro Sekunde",
  "overlay.bandOk": "Gesund",
  "overlay.bandWarn": "Nähert sich dem Limit",
  "overlay.bandShed": "Drosselt Last",
  "overlay.reconcile": "Jetzt synchronisieren",
  "overlay.reconcileQueued":
    "Abgleich eingereiht — der Worker holt ihn beim nächsten Poll ab (etwa alle 2 Minuten).",
  "overlay.disconnect": "Trennen",
  "overlay.disconnectTitle": "HubSpot trennen?",
  "overlay.disconnectBody":
    "Dies löscht die gespiegelten Daten und schaltet die Organisation zurück auf native Datensätze. Das Audit-Protokoll bleibt erhalten.",

  "overlay.userMap.title": "Nutzerzuordnung des Spiegels",
  "overlay.userMap.sub":
    "Wer jede Person in dieser Organisation als {principal}-Nutzer ist. Diese Zuordnung entscheidet allein darüber, was sie im Spiegel sieht.",
  "overlay.userMap.cost":
    "Wer nicht zugeordnet ist, sieht überhaupt keine gespiegelten Datensätze — alle Listen bleiben leer.",
  "overlay.userMap.loading": "Lade die Nutzerzuordnung…",
  "overlay.userMap.loadFailed":
    "Die Nutzerzuordnung konnte nicht geladen werden.",
  "overlay.userMap.adminOnly":
    "Du hast keine Berechtigung, die Zuordnung zu prüfen.",
  "overlay.userMap.notOverlay":
    "Diese Organisation liest aus nativen Tabellen, es gibt also nichts zuzuordnen.",
  "overlay.userMap.notConfigured":
    "Overlay-Modus ist in diesem Deployment nicht konfiguriert.",
  "overlay.userMap.empty": "Diese Organisation hat keine Nutzer zum Zuordnen.",
  "overlay.userMap.view": "Gruppierung",
  "overlay.userMap.viewByUser": "Nach Nutzer",
  "overlay.userMap.viewByOwner": "Nach {principal}-Nutzer",
  "overlay.userMap.principal.hubspot": "HubSpot",
  "overlay.userMap.principal.generic": "verbundenes CRM",
  "overlay.userMap.you": "Du",
  "overlay.userMap.matchEmail": "Über E-Mail zugeordnet",
  "overlay.userMap.matchManual": "Manuell gesetzt",
  "overlay.userMap.map": "Zuordnen",
  "overlay.userMap.change": "Ändern",
  "overlay.userMap.unmap": "Zuordnung aufheben",
  "overlay.userMap.cancel": "Abbrechen",
  "overlay.userMap.pickerLabel": "{principal}-Nutzer suchen",
  "overlay.userMap.pickTitle": "Einem {principal}-Nutzer zuordnen",
  "overlay.userMap.truncated":
    "Das {principal}-Verzeichnis ist länger als diese Liste — wen du hier nicht findest, liegt vielleicht hinter der Grenze.",
  "overlay.userMap.directoryFailed":
    "Das {principal}-Verzeichnis konnte nicht gelesen werden, deshalb lässt sich gerade niemand auswählen.",
  "overlay.userMap.notMapped": "Nicht zugeordnet",
  "overlay.userMap.chip.noEmailMatch": "Keine E-Mail-Übereinstimmung",
  "overlay.userMap.chip.ambiguousEmail": "Mehrdeutige E-Mail",
  "overlay.userMap.chip.blockedByAdmin": "Von Admin aufgehoben",
  "overlay.userMap.chip.notYetSynced": "Noch nicht synchronisiert",
  "overlay.userMap.chip.directoryUnavailable": "Grund unbekannt",
  "overlay.userMap.reason.noEmailMatch":
    "Kein {principal}-Nutzer hat diese E-Mail-Adresse.",
  "overlay.userMap.reason.ambiguousEmail":
    "Zwei oder mehr {principal}-Nutzer teilen sich diese E-Mail-Adresse, eine automatische Zuordnung wäre also nicht sicher.",
  "overlay.userMap.reason.blockedByAdmin":
    "Eine Admin-Person hat die Zuordnung aufgehoben; die automatische Zuordnung setzt sie nicht erneut.",
  "overlay.userMap.reason.notYetSynced":
    "Das {principal}-Verzeichnis führt diese Person noch nicht.",
  "overlay.userMap.reason.directoryUnavailable":
    "Das {principal}-Verzeichnis konnte nicht vollständig gelesen werden, deshalb lässt sich kein Grund ableiten.",
  "overlay.userMap.staleChip": "Nicht mehr im {principal}-Verzeichnis",
  "overlay.userMap.staleNote":
    "Diese manuelle Zuordnung gewährt keine Sichtbarkeit. Sie wird gemeldet, aber nie automatisch zurückgenommen — die Entscheidung bleibt bei dir.",
  "overlay.userMap.unmapTitle": "Zuordnung dieser Person aufheben?",
  "overlay.userMap.unmapSelfTitle": "Deine eigene Zuordnung aufheben?",
  "overlay.userMap.unmapBody":
    "{user} sieht dann keine gespiegelten Datensätze mehr, bis die Zuordnung wieder gesetzt ist.",
  "overlay.userMap.unmapSelfBody":
    "Du siehst dann keine gespiegelten Datensätze mehr, bis du wieder zugeordnet bist. Dieser Tab bleibt erreichbar, du kannst es hier rückgängig machen.",
  "overlay.userMap.sharedSeat": "Geteilter Sitz — {count} Nutzer",
  "overlay.userMap.ownerEmpty":
    "Bisher ist niemand einem {principal}-Nutzer zugeordnet.",
  "overlay.userMap.unmappedCount_one":
    "1 Person ist nicht zugeordnet und fehlt hier — wechsle zu Nach Nutzer, um das zu beheben.",
  "overlay.userMap.unmappedCount_other":
    "{count} Personen sind nicht zugeordnet und fehlen hier — wechsle zu Nach Nutzer, um das zu beheben.",
  "overlay.userMap.partialView":
    "Diese Gruppierung und die Zählung umfassen nur die bisher geladenen Nutzer. Lade mehr, um den Rest zu sehen.",

  "people.name": "Name",
  "people.email": "E-Mail",
  "list.owner": "Zuständig",
  "list.unowned": "Nicht zugewiesen",
  "list.created": "Erstellt",
  "list.lastActivity": "Letzte Aktivität",
  "list.filterOwnerMe": "Meine Datensätze",
  "list.filterOwnerAll": "Alle Zuständigen",
  "list.filterOwnerUnassigned": "Nicht zugewiesen",
  "views.save": "Ansicht speichern",
  "views.saveConfirm": "Speichern",
  "views.saveTitle": "Diese Ansicht speichern",
  "views.name": "Name",
  "views.rail": "Gespeicherte Ansichten",
  "list.viewMine": "Meine",
  "list.viewCustomers": "Kunden",
  "list.viewProspects": "Interessenten",
  "org.filterLifecycleAll": "Alle Phasen",
  "org.filterRelTypeAll": "Alle Typen",
  "org.filterSizeBandAll": "Alle Größen",
  "person.consent": "Einwilligung",
  "consent.grant": "Erteilen",
  "consent.withdraw": "Widerrufen",
  "consent.doiBySubject":
    "Diesen Zweck best\u00e4tigt der Kontakt selbst \u2013 \u00fcber einen Link an seine eigene Adresse. Nutzen Sie unten \u201eUm Best\u00e4tigung der Daten bitten\u201c.",
  "consent.askToConfirm": "Um Bestätigung der Daten bitten",
  "consent.askToConfirmWhat":
    "Schickt diesem Kontakt einen persönlichen Link: Er sieht, was ihr über ihn gespeichert habt, kann es korrigieren und sagen, ob er von euch hören möchte. Der Link geht an seine hinterlegte Adresse — woandershin könnt ihr ihn nicht schicken.",
  "consent.askSent": "An {address} geschickt.",
  "consent.askNotDelivered":
    "Der Link wurde für {address} erstellt, aber diese Installation verschickt keine Mails — es hat ihn also niemand bekommen.",
  "consent.askSendFailed":
    "Der Link wurde für {address} erstellt, aber die Mail ist nicht rausgegangen. Versuch es nochmal — ein neuer Link ersetzt diesen.",
  "consent.askExpires": "Der Link gilt bis",
  "consent.noRecord": "kein Eintrag",
  "consent.noPurposes":
    "Diese Organisation erfasst noch keine Einwilligungszwecke.",
  "consent.defaultDeny":
    "Ausgehende Kommunikation ist pro Zweck standardmäßig gesperrt: ein Versand wird blockiert, sofern keine aktive, nachgewiesene Einwilligung für diesen Zweck vorliegt. Eine Einwilligung für einen Zweck berechtigt niemals einen anderen.",
  "consent.proofLog": "Nachweisprotokoll",
  "consent.proofEmpty":
    "Für diesen Zweck ist keine Einwilligungsentscheidung erfasst. Ein leeres Protokoll ist ehrlich, keine Lücke.",
  "consent.sourceUnknown": "Quelle nicht erfasst",
  "consent.actorHuman": "Mensch",
  "consent.actorAgent": "Agent",
  "consent.actorSystem": "System",
  "consent.actorConnector": "Connector",
  "consent.actorUnknown": "Akteur nicht erfasst",
  "consent.purposesUnavailable":
    "Der Einwilligungszweck-Katalog konnte nicht geladen werden — welche Zwecke ein Double-Opt-in brauchen, lässt sich gerade nicht anzeigen.",

  "org.name": "Firma",
  "org.description": "Was sie tun",
  "org.website": "Website",
  "org.contactCount": "Kontakte",
  "org.openDealCount": "Offene Deals",
  // Nur dort angeboten, wo es noch kein Partnerprogramm gibt: der Tab mit dem
  // Formular erscheint erst, wenn eines besteht — so entsteht das erste.
  // Wo der Account bei uns steht, und was er für uns ist — die zwei Fragen,
  // die die abgelöste Einstufung mit einem Wert beantworten wollte.
  "org.lifecycle": "Account-Status",
  "org.relationshipTypes": "Beziehung zu uns",
  "org.sizeBand": "Unternehmensgröße",
  "org.lifecycle.unknown": "Nicht eingeschätzt",
  "org.lifecycle.target": "Zielkunde",
  "org.lifecycle.prospect": "Interessent",
  "org.lifecycle.opportunity": "Chance",
  "org.lifecycle.customer": "Kunde",
  "org.lifecycle.former_customer": "Ehemaliger Kunde",
  "org.lifecycle.disqualified": "Disqualifiziert",
  "org.relType.customer": "Kunde",
  "org.relType.partner": "Partner",
  "org.relType.supplier": "Lieferant",
  "org.relType.investor": "Investor",
  "org.relType.portfolio_company": "Portfoliounternehmen",
  "org.relType.competitor": "Wettbewerber",
  "org.relType.other": "Sonstige",
  // Warum ein Fakt seinem eigenen Feld widerspricht. Der Fakt bleibt mit
  // seinem Beleg sichtbar — ein Mensch erkennt es, Ausblenden wäre schlechter.
  "co.factSuspect.phoneShapedLocation": "Sieht nach einer Telefonnummer aus",
  "co.factSuspect.notAPhone": "Sieht nicht nach einer Telefonnummer aus",
  "co.factSuspect.notAYear": "Sieht nicht nach einer Jahreszahl aus",
  "co.factSuspect.notAnEmail": "Sieht nicht nach einer E-Mail-Adresse aus",
  "co.factSuspect.notASize": "Sieht nicht nach einer Mitarbeiterzahl aus",
  // Die drei Aussagen, mit denen die Übersicht beginnt, und was das
  // Ausführen eines Vorschlags bedeutet.
  "co.strip.title": "Wo dieser Account steht",
  "co.strip.convertedAsOf": "{count} umgerechnet, Kurse vom {date}",
  "co.strip.noOpenDeals": "Keine offenen Deals",
  "co.strip.pipeline": "Offene Pipeline",
  "co.description.label": "Beschreibung",
  "co.description.placeholder": "Beschreibung hinzufügen",
  "co.strip.netInvoiced": "Netto fakturiert · 12 Monate",
  "co.strip.notAssessed": "Nicht bewertet",
  "co.strip.lifetimeOf": "{amount} gesamt",
  "co.strip.finance": "Finanzen",
  "co.strip.financeUnknown": "—",
  "co.strip.basis.health": "Woraus sich dieser Wert ergibt",
  "co.strip.open.deals": "Deals öffnen",
  "co.strip.open.finance": "Finanzen öffnen",
  "co.strip.open.people": "Personen öffnen",
  "co.strip.basis.reading": "Wie es steht",
  "co.strip.fin.notACustomer": "Noch kein Kunde",
  "co.strip.fin.noConnection": "Buchhaltung verbinden",
  "co.strip.fin.unmapped": "Noch keinem Kunden zugeordnet",
  "co.strip.fin.syncing": "Wird synchronisiert…",
  "co.strip.fin.withheld":
    "Du darfst die Finanzdaten dieses Accounts nicht sehen",
  "co.strip.fin.staleFigure":
    "Zuletzt vor längerer Zeit synchronisiert — Datum prüfen",
  "co.strip.fin.errorFigure":
    "Letzte Synchronisierung fehlgeschlagen — evtl. nicht aktuell",
  "co.strip.fin.nothingBilled": "Noch nichts in Rechnung gestellt",
  "co.strip.fin.error": "Konnte nicht gelesen werden",
  "co.strip.fin.loading": "Wird geladen…",
  "co.strip.unpriced": "Keine umrechenbaren Beträge hinterlegt",
  "co.strip.pricedPartly": "{priced} von {total} Deals mit Betrag",
  "co.strip.health": "Austausch",
  "co.strip.healthOneSided": "Einseitig",
  "co.strip.healthBalanced": "Ausgeglichener Austausch",
  "co.strip.replyShare": "{percent}% des Austauschs kommt von ihnen",
  "co.strip.healthActive": "Im Gespräch",
  "co.strip.healthQuiet": "Still geworden",
  "co.strip.noInboundEver": "Sie haben nie geschrieben",
  "co.strip.engagement.never_contacted": "Nie kontaktiert",
  "co.strip.engagement.active": "Im Gespräch",
  "co.strip.engagement.waiting_on_them": "Warten auf sie",
  "co.strip.engagement.waiting_on_us": "Warten auf uns",
  "co.strip.engagement.dormant": "Still geworden",
  "co.strip.openDeals": "{count} offen",
  "co.strip.stalled": "{count} ins Stocken geraten",
  "co.suggest.act.draftReply": "Entwurf erstellen",
  "co.suggest.act.openDeal": "Deal öffnen",
  "co.suggest.act.addTask": "Nächsten Schritt anlegen",
  // Ein Verlauf als ein Ereignis sagt zuerst, WAS er ist.
  "timeline.group.thread_other": "{count} Nachrichten",
  "timeline.group.thread_one": "{count} Nachricht",
  "timeline.group.bulk_other": "an {count} Personen gesendet",
  "timeline.group.bulk_one": "an {count} Person gesendet",
  "timeline.group.expand": "Öffnen",
  "timeline.group.collapse": "Schließen",
  "timeline.group.openThread": "Ganzen Verlauf ansehen",
  "timeline.group.mayContinue": "kann früher weitergehen",
  "timeline.filters.kind": "Aktivitätsart",
  "timeline.filters.kind.all": "Alle Arten",
  "timeline.filters.kind.email": "E-Mail",
  "timeline.filters.kind.message": "Nachrichten",
  "timeline.filters.kind.call": "Anrufe",
  "timeline.filters.kind.meeting": "Termine",
  "timeline.filters.kind.note": "Notizen",
  "timeline.filters.kind.task": "Aufgaben",
  "timeline.filters.search": "In dieser Chronik suchen",
  "timeline.filters.from": "Von",
  "timeline.filters.to": "Bis",
  "timeline.filters.searchOmitsLimited":
    "Unterhaltungen, deren Inhalt Sie nicht öffnen dürfen, bleiben bei einer Suche außen vor.",
  "tab.people": "Personen",
  "tab.deals": "Deals",
  "tab.tasks": "Aufgaben",
  "tab.timeline": "Verlauf",
  "tab.finance": "Finanzen",
  "tab.network": "Netzwerk",
  "tab.documents": "Dokumente",
  "tab.profile": "Profil",
  "tab.meetings": "Termine",
  "tab.research": "Daten & Tools",
  // Das Briefing nach den Fragen, die es beantwortet, und die Art jeder
  // Aussage — eine Einschätzung darf nicht wie ein Fakt wirken.
  "co.brief.nature.fact": "Fakt",
  "co.brief.nature.assessment": "Unsere Einschätzung",
  "co.brief.nature.recommendation": "Vorschlag",
  "co.details.title": "Details",
  "co.health.dim.relationship": "Beziehung",
  "co.health.dim.commercial": "Geschäftlich",
  "co.health.dim.payment": "Zahlung",
  "co.health.rating.atRisk": "Gefährdet",
  "co.health.rating.good": "Gut",
  "co.health.rating.strong": "Stark",
  "co.health.payment.overdue": "Aktuell ist Geld überfällig.",
  "co.health.payment.late": "Zahlt typisch {days} Tage nach Fälligkeit.",
  "co.health.payment.onTime": "Zahlt pünktlich.",
  "co.health.sinceInbound": "Sie schrieben zuletzt vor {days} Tagen",
  "org.partnerSetUp": "Partnerprogramm einrichten",
  "signal.kind.stalled_deal": "Deal steht",
  "signal.kind.champion_left": "Champion ist weg",
  "signal.kind.reengagement": "Wieder ansprechen",
  "signal.kind.buying_intent": "Kaufinteresse",
  "signal.kind.risk": "Risiko",
  "signal.kind.other": "Sonstiges",
  "signal.kind.contract_ended": "Vertrag endet",
  "signal.kind.new_opportunity": "Neue Chance",
  "signal.kind.commitment_made": "Etwas wurde zugesagt",
  "signal.kind.ghosted_thread": "Keine Antwort",
  "signal.kind.project_gone_quiet": "Projekt ist still geworden",
  "signal.kind.funding": "Finanzierung",
  "signal.kind.leadership_change": "Führungswechsel",
  "signal.kind.expansion": "Expansion",
  "signal.kind.product_launch": "Neues Produkt",
  "co.routeIn.band.strong": "regelmäßig in Kontakt",
  "co.routeIn.band.some": "etwas Kontakt",
  "co.routeIn.band.faint": "kaum in Kontakt",
  "co.routeIn.band.unknown": "Kontakt vorhanden, noch kein Muster",
  "record.profile": "Profil",
  "record.context": "Kontext",
  "record.restsOn": "Worauf das beruht",
  "record.restsOn.source_one": "Quelle",
  "record.restsOn.source_other": "Quellen",
  "record.tabs": "Bereiche dieses Datensatzes",
  "record.panel.show": "Panel zeigen",
  "record.panel.hide": "Panel ausblenden",
  "room.editorial":
    "Ein Dokument, das Sie hinzufügen, ist sofort geteilt; Kommentare ebenso.",
  "room.readOnly": "Sie können diesen Raum lesen, aber nicht ändern.",
  "room.finished":
    "Dieser Raum ist abgeschlossen, das Geteilte ist jetzt ein Protokoll.",
  "room.card.title": "Deal Room",
  "room.card.people": "{invited} eingeladen · {active} angemeldet",
  "room.card.lastSeen": "Zuletzt von einem Käufer gesehen: {when}",
  "room.card.open": "Deal Room öffnen",
  "room.create.sub":
    "Ein Raum, den der Käufer per Link betritt, um zu lesen, was Sie teilen, und darüber zu sprechen.",
  "room.create.open": "Deal Room eröffnen",
  "room.create.confirm": "Eröffnen",
  "room.create.titleLabel": "Titel des Raums",
  "room.create.titleHint":
    "Was der Käufer als Überschrift sieht. Später änderbar.",
  "room.create.defaultTitle": "{deal}",
  "roompage.none":
    "Dieser Deal hat noch keinen Deal Room. Eröffnen Sie einen auf der Deal-Seite.",
  "roompage.backToDeal": "← Zurück zum Deal",
  "roompage.accessMenu": "Zugang zum Raum",
  "roompage.pause": "Pausieren",
  "roompage.pauseHint":
    "Käufer behalten ihre Links, sehen aber eine Pausenseite, bis Sie fortsetzen.",
  "roompage.resume": "Fortsetzen",
  "roompage.close": "Raum schließen",
  "roompage.closeHint":
    "Käufer lesen weiter; nichts kann mehr hinzugefügt oder gesagt werden.",
  "roompage.setExpiry": "Enddatum setzen",
  "roompage.setExpiryHint": "Der Zugang endet an diesem Tag.",
  "roompage.closeTitle": "Diesen Deal Room schließen?",
  "roompage.closeBody":
    "Käufer lesen den Raum weiter. Danach wird kein Dokument, Kommentar oder Beschluss mehr angenommen. Sie können weiterhin Zugänge entziehen und Links ausstellen.",
  "roompage.expiryLabel": "Zugang endet am",
  "roompage.expiryHint": "Leer lassen für kein Enddatum.",
  "roompage.banner.paused":
    "Pausiert. Käufer sehen eine Pausenseite, bis Sie fortsetzen.",
  "roompage.banner.closed":
    "Geschlossen. Käufer können den Raum weiter lesen; mehr wird nicht angenommen.",
  "roompage.banner.expired":
    "Abgelaufen. Käufer-Links funktionieren nicht mehr.",
  "roompage.banner.archived": "Archiviert. Niemand kann diesen Raum betreten.",
  "roompage.banner.liveUntil": "Live. Der Zugang endet am {when}.",
  "roompage.text.title": "Titel und Begrüßung",
  "roompage.text.sub": "Was der Käufer zuerst liest. Er sieht es sofort.",
  "roompage.text.titleLabel": "Titel des Raums",
  "roompage.text.welcomeLabel": "Begrüßungstext",
  "roompage.viewAsBuyer": "Als Käufer ansehen",
  "roompage.previewArchived": "Ein archivierter Raum hat keine Vorschau.",
  "access.title": "Zugang",
  "access.sub": "Wer eintreten darf und was jede Person tun kann.",
  "access.invite": "Einladen",
  "access.empty": "Noch niemand eingeladen.",
  "access.cap.view": "Nur lesen",
  "access.cap.viewHint": "Kann Dokumente und Unterhaltung lesen.",
  "access.cap.comment": "Lesen und kommentieren",
  "access.cap.commentHint": "Kann außerdem Fragen stellen und antworten.",
  "access.state.invited": "eingeladen",
  "access.state.active": "angemeldet",
  "access.state.revoked": "entzogen",
  "access.lastSeen": "zuletzt gesehen {when}",
  "access.downloads": "{count} Dokument(e) heruntergeladen",
  "access.linkRequested":
    "Hat {when} um einen neuen Link gebeten. Stellen Sie einen aus und senden Sie ihn selbst.",
  "access.rowActions": "Aktionen für {name}",
  "access.issueLink": "Neuen Link ausstellen",
  "access.changeCapability": "Rechte ändern",
  "access.revoke": "Zugang entziehen",
  "access.inviteTitle": "Jemanden in den Deal Room einladen",
  "access.inviteConfirm": "Einladen",
  "access.done": "Fertig",
  "access.save": "Speichern",
  "access.nameLabel": "Name",
  "access.emailLabel": "E-Mail",
  "access.capabilityLegend": "Was darf die Person tun?",
  "access.inviteNote":
    "Sie erhalten den Link zum Kopieren. Ist ein Mail-Relay konfiguriert, wird er zusätzlich versandt.",
  "access.issued.title": "Link für {name}",
  "access.issued.mailed":
    "An {email} gesendet. Sie können ihn unten auch kopieren.",
  "access.issued.notMailed":
    "Es wurde keine Mail gesendet. Kopieren Sie den Link und senden Sie ihn selbst.",
  "access.issued.linkLabel": "Der Link",
  "access.issued.copy": "Link kopieren",
  "access.issued.copied": "Kopiert",
  "access.issued.copyFailed":
    "Kopieren fehlgeschlagen; markieren und kopieren Sie den Link.",
  "access.issued.oneTime":
    "Persönlicher Einmal-Link. Er funktioniert einmal, auf einem Gerät. Jede Person braucht ihre eigene Einladung.",
  "access.issueLinkTitle": "Neuen Link für {name} ausstellen",
  "access.issueLinkBody":
    "Der bisherige Link funktioniert dann nicht mehr. Sie erhalten den neuen zum Kopieren.",
  "access.revokeTitle": "Zugang für {name} entziehen?",
  "access.neverSignedIn": "nie angemeldet",
  "access.revokeBody":
    "Die Sitzung endet sofort und der Link funktioniert nicht mehr. Kommentare bleiben sichtbar und zugeordnet. Der Zugang lässt sich nicht durch eine Link-Anfrage wiederherstellen.",
  "access.changeCapabilityTitle": "Was darf {name} tun?",
  "persondealrooms.title": "Deal Rooms",
  "persondealrooms.sub": "Räume, die dieser Kontakt noch betreten kann.",
  "persondealrooms.open": "Öffnen",
  "persondealrooms.seatGone":
    "Diese Adresse hat in dem Raum keinen Platz mehr.",
  "persondealrooms.cut":
    "Nur die ersten Räume werden gezeigt; dieser Kontakt sitzt in weiteren.",
  "persondealrooms.revokeTitle": "Zugang zu {room} entziehen?",
  "room.state.draft": "Entwurf",
  "room.state.building": "Wird erstellt",
  "room.state.ready": "Bereit",
  "room.state.publishing": "Wird veröffentlicht",
  "room.state.live": "Live",
  "room.state.paused": "Pausiert",
  "room.state.closed": "Abgeschlossen",
  "room.state.expired": "Abgelaufen",
  "room.state.archived": "Archiviert",
  "co.pulse.created": "Erstellt {when}",
  "co.pulse.lastExchange": "Letzter Kontakt {when}, in beide Richtungen",
  "co.pulse.neverTouched": "Noch nie kontaktiert",
  "co.pulse.owner": "Betreut von",
  "co.pulse.strongestLead": "Zugang \u00fcber",
  "co.pulse.strengthTail_one": "\u2014 der einzige Kontakt hier",
  "co.pulse.strengthTail_other": "\u2014 von {count} Kontakten hier",
  "co.pulse.unowned": "Nicht zugewiesen",
  "co.since.first": "Du öffnest diesen Account zum ersten Mal.",
  "co.partial":
    "Teile dieser Seite konnten nicht geladen werden; sie zeigt möglicherweise nicht alles zu diesem Account.",
  "evidence.explain": 'Herkunft von "{value}"',
  "evidence.fullHistory": "Vollständiger Verlauf",
  "co.section.unavailable":
    "Konnte nicht geladen werden — das ist möglicherweise nicht das ganze Bild",
  "finance.title": "Finanzen",
  "finance.titleHistorical": "Finanzen · historisch",
  "finance.none": "Nichts erfasst.",
  "finance.syncing":
    "Abgleich mit der Buchhaltungsquelle läuft. Zahlen erscheinen nach dem ersten Durchlauf.",
  "finance.noConnection":
    "Keine Finanzquelle verbunden — verbinde eine, um zu sehen, was diesem Kunden berechnet wurde und ob er pünktlich zahlt",
  "finance.unmapped":
    "Verbunden, aber dieses Unternehmen ist noch keinem Kunden im Buchhaltungssystem zugeordnet",
  "finance.netInvoiced": "Netto fakturiert · 12 Monate",
  "finance.overdue": "Überfällig",
  "finance.behaviour": "Zahlungsverhalten",
  "finance.behaviourShape":
    "Verzugstage je beglichener Rechnung, älteste zuerst",
  "finance.shareOfOpen": "{percent} % des gesamten Offenen",
  "finance.overdueShareLabel": "Überfälliger Anteil am offenen Saldo",
  "finance.legendOverdue": "Überfällig {amount}",
  "finance.legendOpen": "Offen {amount}",
  "finance.medianAfterDue": "Typisch {days} Tage nach Fälligkeit",
  "finance.medianEarly": "Typisch {days} Tage früher",
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
  "finance.connect": "Finanzquelle verbinden",
  "finance.syncedFrom": "Aus {provider} · Stand {when}",
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
  "contracts.empty": "Keine Vertr\u00e4ge hinterlegt",
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
  "contracts.endedPendingStatus":
    "Laufzeit beendet \u2014 Status\u00e4nderung offen",
  "contracts.form.title": "Vertrag erfassen",
  "contracts.form.name": "Titel",
  "contracts.form.number": "Vertragsnummer",
  "contracts.form.value": "Wert",
  "contracts.form.basis": "Dieser Wert ist",
  "contracts.basis.total": "der Gesamtwert der Laufzeit",
  "contracts.basis.annual": "zw\u00f6lf Monate eines unbefristeten Vertrags",
  "contracts.form.startsOn": "Beginnt",
  "contracts.form.endsOn": "Endet",
  "contracts.form.endsOnHint":
    "Leer lassen f\u00fcr einen unbefristeten Vertrag.",
  "contracts.form.renewalOn": "Verl\u00e4ngert sich",
  "contracts.form.noticeDays": "K\u00fcndigungsfrist (Tage)",
  "contracts.form.noticeDaysHint":
    "Wie viel Vorlauf eine K\u00fcndigung braucht. Die Verl\u00e4ngerungswarnung kommt vor dieser Frist, nicht erst vor dem Verl\u00e4ngerungsdatum.",
  "contracts.form.signedOn": "Unterschrieben",
  "contracts.form.signedOnHint":
    "Nur wenn bekannt ist, dass unterschrieben wurde \u2014 nie aus dem Abschlussdatum eines Deals \u00fcbernommen.",
  "contracts.form.save": "Vertrag erfassen",
  "contracts.form.errNoName": "Ein Vertrag braucht einen Titel.",
  "contracts.form.errTermOrder":
    "Eine Laufzeit kann nicht vor ihrem Beginn enden.",
  "contracts.add": "Vertrag hinzuf\u00fcgen",
  "contracts.rowMenu": "Vertragsaktionen",
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
    "\u201e{title}\u201c verschwindet aus den Listen und den Summen des Kontos. Der Datensatz und seine Historie bleiben erhalten \u2014 gel\u00f6scht wird nichts.",
  "contracts.archive.confirm": "Archivieren",
  "contracts.form.editTitle": "Vertrag bearbeiten",
  "contracts.form.saveEdit": "\u00c4nderungen speichern",
  "contracts.form.file": "Unterschriebenes Dokument",
  "contracts.form.fileHint":
    "Das unterschriebene PDF hier ablegen oder klicken, um eines auszuw\u00e4hlen. Es wird diesem Vertrag zugeordnet und erscheint bei den Dokumenten des Kontos.",
  "contracts.form.fileEmpty": "Datei hier ablegen oder klicken",
  "contracts.form.fileAdd": "Weitere Datei hier ablegen oder klicken",
  "contracts.perYear": "{amount} / Jahr",
  "contracts.state.title": "Unter Vertrag · {count} aktiv",
  "contracts.state.none": "Kein Vertrag hinterlegt",
  "contracts.state.renewsOn": "Verlängert sich am {when}",
  "contracts.state.endsOn": "Gekündigt — endet am {when}",
  "contracts.state.partial": "{priced} von {total} mit Wert",
  "commercial.lastOffer": "Letztes Angebot · {deal}",
  "commercial.offerUnnumbered": "Angebot",
  "commercial.validUntil": "gültig bis {when}",
  "commercial.offer.draft": "Entwurf",
  "commercial.offer.sent": "Versendet",
  "commercial.offer.accepted": "Angenommen",
  "commercial.offer.rejected": "Abgelehnt",
  "commercial.offer.expired": "Abgelaufen",
  "commercial.offer.superseded": "Ersetzt",
  "co.section.restricted":
    "Ausgeblendet \u2014 deine Rolle darf das nicht lesen",
  "co.next.title": "Aufgaben",
  "co.next.empty": "Keine offene Aufgabe zu diesem Account.",
  "co.next.overdue": "\u00dcberfällig",
  "co.next.due": "Fällig {when}",
  "co.next.undated": "Ohne Datum",
  "co.facts.pipeline": "Offene Pipeline",
  "co.facts.inFlight": "Laufend",
  "co.facts.reading": "Wird gelesen\u2026",
  "co.facts.noDeals": "Keine offenen Deals",
  "co.facts.unpriced": "Noch nicht beziffert",
  "co.facts.nothing": "Nichts",
  "co.facts.deals_one": "1 Deal",
  "co.facts.deals_other": "{count} Deals",
  "co.facts.projects_one": "1 Projekt",
  "co.facts.projects_other": "{count} Projekte",
  "co.facts.atLeast": "oder mehr",
  "co.work.title": "Was läuft, und warum",
  "co.work.count": "{count} laufend",
  "co.work.countAtLeast": "{count}+ laufend",
  "co.work.deals": "Deals",
  "co.work.noDealsDetail":
    "Im Deal stehen Betrag und Abschlussdatum. Leg einen an, sobald es etwas zu gewinnen gibt.",
  "co.work.noDeals": "Keine offenen Deals.",
  "co.work.closes": "Abschluss {date}",
  "co.work.stalled": "Zu diesem Deal wurde seit 60 Tagen nichts erfasst.",
  "co.work.overdueTask":
    "{who} sollte \u201a{title}\u2018 bis {date} erledigen und hat es nicht getan.",
  "co.work.overdueTaskUnnamed":
    "\u201a{title}\u2018 war am {date} f\u00e4llig und ist offen.",
  "co.work.owesUs": "{who} sagte: \u201a{body}\u2018",
  "co.work.owesUsUnnamed": "Sie sagten: \u201a{body}\u2018",
  "co.work.wasDue": "\u2014 bis {date}.",
  "co.work.statusesWithheld":
    "Du darfst die Konversationen dieses Accounts nicht lesen, deshalb tragen die Zeilen oben keine Begr\u00fcndungen.",
  "co.brief.by.model": "Von Margince geschrieben",
  "co.brief.by.deterministic": "Aus deinen Daten zusammengestellt",
  "co.brief.generatedAt": "Stand {when}",
  "co.growthFit.title": "Was sie dir wert sind",
  "co.growthFit.unavailable":
    "Diese Einschätzung ließ sich nicht lesen. An der Firma hat sich nichts geändert.",
  "co.growthFit.assembling":
    "Der Wert dieses Accounts wird ermittelt — die erste Einschätzung liest den Datensatz und dauert einen Moment.",
  "co.growthFit.reassess": "Neu einschätzen",
  "co.growthFit.reassessing": "Wird eingeschätzt…",
  "co.growthFit.band.strong": "Passt gut",
  "co.growthFit.dim.industryFit": "Branchenpassung",
  "co.growthFit.dim.companySize": "Unternehmensgröße",
  "co.growthFit.dim.transformationNeed": "Veränderungsbedarf",
  "co.growthFit.dim.access": "Zugang",
  "co.growthFit.band.moderate": "Passt teilweise",
  "co.growthFit.band.weak": "Passt kaum",
  "co.growthFit.band.unknown": "Zu wenig für ein Urteil",
  "co.growthFit.completeness": "{present} von {expected} Angaben erfasst",
  "co.growthFit.missing": "Es fehlt noch",
  "co.growthFit.capped": "Zurückgehalten: {reason}.",
  "co.growthFit.nextStep": "Als Nächstes: {step}.",
  "co.growthFit.positive": "Dafür",
  "co.growthFit.negative": "Dagegen",
  "co.growthFit.whitespace": "Noch zu verkaufen",
  "co.growthFit.objections": "Voraussichtliche Einwände",
  "co.growthFit.angle": "Vorgeschlagener Ansatz",
  "co.dossier.title": "Was diese Firma ist",
  "co.dossier.unavailable":
    "Diese Beschreibung ließ sich nicht lesen. An der Firma hat sich nichts geändert.",
  "co.dossier.empty":
    "Zu dieser Firma ist noch nichts erfasst. Lies ihre Website, oder füll das Profil unten aus.",
  "co.dossier.stale": "Vor über einem Monat gelesen",
  "co.dossier.rewrite": "Neu schreiben",
  "co.dossier.rewriting": "Wird geschrieben…",
  "co.dossier.section.summary": "Kurz gesagt",
  "co.dossier.section.products_services": "Was sie verkaufen",
  "co.dossier.section.markets": "Wo und an wen",
  "co.dossier.section.buying_center": "Wer entscheidet",
  "co.dossier.section.differentiation": "Was sie auszeichnet",
  "co.dossier.section.firmographics": "Größe, Alter und Registrierung",
  "co.evidence.unavailable":
    "Dieser Nachweis ließ sich nicht lesen. Am Datensatz selbst hat sich nichts geändert.",
  "co.evidence.producedBy": "erfasst von {who}",
  "co.evidence.retrievedAt": "Gelesen {when}",
  "co.evidence.verifiedAt": "Von einer Person bestätigt {when}",
  "co.evidence.confidence": "Das Modell war zu {percent}% sicher",
  "co.evidence.gaps": "Nicht erfasst: {fields}.",
  "co.evidence.kind.site_read": "Von ihrer Website gelesen",
  "co.evidence.kind.connector": "Aus einem angebundenen System",
  "co.evidence.kind.human": "Von einer Person eingetragen",
  "co.evidence.kind.migration": "Importiert",
  "co.evidence.kind.rule": "Abgeleitet",
  "co.brief.cite.deal": "Deal",
  "co.brief.cite.activity": "Aktivität",
  "co.brief.cite.person": "Kontakt",
  "co.brief.cite.organization": "Account",
  "co.brief.cite.fact": "Fakt",
  "co.brief.cite.profile_field": "Profilfeld",
  "co.brief.cite.deal.many": "{count} Deals",
  "co.brief.cite.activity.many": "{count} Aktivitäten",
  "co.brief.cite.person.many": "{count} Kontakte",
  "co.brief.cite.organization.many": "{count} Accounts",
  "co.brief.cite.fact.many": "{count} Fakten",
  "co.brief.cite.profile_field.many": "{count} Profilfelder",
  "approval.kind.advance_deal": "Deal weiterbringen",
  "approval.kind.promote_lead": "Lead überführen",
  "approval.kind.close_date_correction": "Abschlussdatum korrigieren",
  "approval.kind.deal_follow_up": "Wiedervorlage zum Deal anlegen",
  "approval.kind.archive_record": "Datensatz archivieren",
  "approval.kind.merge_records": "Datensätze zusammenführen",
  "approval.kind.merge_tags": "Ein Schlagwort in ein anderes überführen",
  "approval.kind.update_record": "Datensatz ändern",
  "approval.kind.create_record": "Datensatz anlegen",
  "approval.kind.send_email": "E-Mail senden",
  "approval.kind.held_draft": "Entworfene E-Mail prüfen",
  "approval.kind.book_meeting": "Termin buchen",
  "approval.kind.volume_release": "Einen Agenten weiterarbeiten lassen",
  "approval.kind.coldstart": "Neuen Account befüllen",
  "approval.kind.enrich": "Aus dem Web anreichern",
  "approval.kind.deepread": "Unternehmensseite lesen",
  "approval.kind.linkedin_match": "LinkedIn-Zuordnung",
  "approval.kind.site_lead": "Person von der Website aufnehmen",
  "approval.kind.capture_counterparty": "Person aus deiner Mail aufnehmen",
  "approval.kind.org_name_promotion": "Account umbenennen",
  "approval.kind.vcard_create": "Kontakt aus einer Visitenkarte anlegen",
  "approval.kind.lifecycle_change": "Account-Phase",
  "approval.kind.transcript_proposal":
    "Nächsten Schritt aus einem Transkript übernehmen",
  "approval.kind.fx_rate_proposal": "Wechselkurse aktualisieren",
  "approval.kind.ai_model_rate_proposal": "Modellpreise aktualisieren",
  "approval.kind.disqualify_lead": "Lead disqualifizieren",
  "approval.kind.advance_project_phase": "Projekt in die nächste Phase bringen",
  "approval.kind.assign_owner": "Datensatz übergeben",
  "approval.kind.commit_import": "Import übernehmen",
  "approval.kind.emit_flow_event": "Automatisierungsschritt festhalten",
  "approval.kind.relink_activity": "Aktivität neu zuordnen",
  "approval.kind.relink_thread": "Konversation neu zuordnen",
  "approval.kind.relink_activities": "Mehrere Aktivitäten neu zuordnen",
  "approval.kind.scheduled_send_held": "Gestoppte Nachricht freigeben",
  "approval.kind.send_account_email": "E-Mail an ein Unternehmen senden",
  "approval.kind.send_message": "Nachricht senden",
  "approval.field.basis": "Warum",
  "approval.field.because": "Warum",
  "approval.field.step": "Der Schritt",
  "approval.field.intent": "Warum das entworfen wurde",
  "approval.field.evidence_snippet": "Was auf der Seite stand",
  "approval.field.previous_close_date": "Bisheriges Datum",
  "approval.field.expected_close_date": "Vorgeschlagenes Datum",
  "approval.field.due_date": "Fällig",
  "approval.field.scheduled_at": "Sollte rausgehen",
  "approval.field.flags": "Was daran nicht stimmt",
  "approval.field.closeDateFlag.overdue": "das Datum ist vorbei",
  "approval.field.closeDateFlag.missing": "gar kein Datum",
  "approval.field.closeDateFlag.unrealistic_soon":
    "zu früh, um realistisch zu sein",
  "approval.field.closeDateFlag.unrealistic_stale":
    "es bewegt sich nichts mehr",
  "approval.field.name": "Name",
  "approval.field.role": "Rolle",
  "approval.field.email": "E-Mail",
  "approval.field.domain": "Domain",
  "approval.field.company": "Firma",
  "approval.field.title": "Position",
  "approval.field.phone": "Telefon",
  "approval.field.url": "Website",
  "approval.field.address": "Adresse",
  "approval.field.published_email": "E-Mail auf der Seite",
  "approval.field.connection_name": "Auf LinkedIn",
  "approval.field.connection_company": "Arbeitet bei",
  "approval.field.person_name": "Kontakt hier",
  "approval.field.owner": "Zuständig",
  "approval.field.to": "An",
  "approval.field.currency": "Währung",
  "approval.field.rate": "Neuer Kurs",
  "approval.field.prior_rate": "Bisheriger Kurs",
  "approval.field.provider": "Anbieter",
  "approval.field.model": "Modell",
  "approval.field.input_per_mtok": "Eingabe, pro Million Tokens",
  "approval.field.output_per_mtok": "Ausgabe, pro Million Tokens",
  "approval.field.tool": "Was es gemacht hat",
  "approval.field.observed": "Verbraucht",
  "approval.field.limit": "Grenze",
  "approval.field.allowance": "Angefragt",
  "co.assistant.title": "Diesen Account befragen",
  "co.assistant.aiTag": "KI-gestützt",
  "co.decisions.open": "{count} offene prüfen",
  "co.decisions.title": "Offene Entscheidungen",
  "co.decisions.group": "{count} × {kind}",
  "co.decisions.empty": "Hier wartet nichts auf eine Entscheidung.",
  "co.ask.title": "Margince fragen",
  "co.ask.q.whats_open": "Was ist hier offen?",
  "co.ask.q.meeting_prep": "Auf ein Gespräch vorbereiten",
  "co.ask.q.whats_changed": "Was hat sich zuletzt geändert?",
  "co.ask.nothing": "Dazu ist nichts sichtbar, was das beantworten würde.",
  "co.ask.failed":
    "Die Frage konnte nicht beantwortet werden — bitte erneut versuchen.",
  "co.suggest.title": "Margince hat das gefunden",
  "co.suggest.kind.no_reply": "Keine Antwort",
  "co.suggest.kind.stalled_deal": "Deal steht",
  "co.suggest.kind.no_next_step": "Nichts geplant",
  "co.suggest.kind.lifecycle_conflict": "Widerspruch im Datensatz",
  "co.suggest.more": "{count} weitere hier nicht gezeigt.",
  "co.suggest.basedOn": "Worauf das beruht",
  "co.suggest.dismiss": "Nicht jetzt",
  "co.suggest.found": "Margince hat das gefunden",
  "co.suggest.dismissFailed":
    "Konnte nicht ausgeblendet werden — es wird weiter angezeigt",
  "co.suggest.viewTasks": "Aufgaben ansehen",
  "co.suggest.commitment.overdueCount": "{count} überfällig",
  "co.suggest.commitment.openCount": "{count} offen",
  "co.suggest.commitment.overdueAtLeast": "{count}+ überfällig",
  "co.suggest.commitment.openAtLeast": "{count}+ offen",
  "co.deals.title": "Deals",
  "co.deals.empty": "Kein offener Deal zu diesem Account.",
  "co.deals.wonLifetime": "Bisher gewonnen",
  "co.deals.lostCount": "{count} verloren",
  "co.deals.noStage": "Keine Phase",
  "co.rail.all": "Alle {count}",
  "co.rail.add": "Hinzufügen",
  "co.rail.deals.title": "Aktive Deals",
  "co.rail.deals.empty": "Für diesen Account gibt es noch keine Deals.",
  "co.rail.deals.emptyClosedOnly":
    "Nichts offen — nur abgeschlossene Historie.",
  "co.rail.deals.noCloseDate": "kein Abschlussdatum",
  "co.rail.deals.attentionOverdue": "Überfällig",
  "co.rail.deals.attentionCommitment": "Sie schulden uns",
  "co.rail.people.title": "Ihre Ansprechpartner",
  "co.rail.people.empty":
    "Noch keine Kontakte. Niemand, dem man schreiben kann.",
  "co.rail.people.add": "Kontakt hinzufügen",
  "co.rail.people.inTouch": "Bereits mit ihnen in Kontakt",
  "co.rail.details.all": "Alle Felder",
  "co.commercial.title": "Kommerziell",
  "co.commercial.lostFigure": "Verlorene Deals",
  "co.commercial.allDeals": "Alle Deals",
  "co.commercial.truncated":
    "Dieser Account hat mehr offene Deals, als hier Platz haben. Öffnen Sie Alle Deals, um den Rest zu sehen.",
  "linkedinImport.title": "LinkedIn-Kontakte",
  "linkedinImport.sub":
    "Eigenen Export importieren und sehen, wen das Team bereits kennt",
  "linkedinImport.profileLabel": "Deine LinkedIn-Profil-URL",
  "linkedinImport.profilePlaceholder": "https://www.linkedin.com/in/…",
  "linkedinImport.saveProfile": "Profil speichern",
  "linkedinImport.editProfile": "\u00c4ndern",
  "linkedinImport.editProfileTitle": "Ihr LinkedIn-Profil",
  "linkedinImport.profileNotSet": "Noch nicht erfasst",
  "linkedinImport.connectedNote":
    "Verbunden. Importierte Kontakte werden diesem Profil zugeordnet, damit das CRM sagen kann, welche Kollegin jemanden kennt \u2014 und nicht nur, dass \u201edas Unternehmen\u201c ihn kennt.",
  "linkedinImport.notConnectedNote":
    "Mit deiner hinterlegten Profil-URL werden importierte Kontakte dir namentlich zugeordnet.",
  "linkedinImport.whichFile":
    "LinkedIn stellt dir Connections.csv unter Einstellungen \u2192 Datenschutz \u2192 Kopie deiner Daten bereit; das Archiv enth\u00e4lt ein Dutzend weitere, gesucht ist diese eine. Was du hochl\u00e4dst, wird niemals zu Kontakten: die Verbindungen erscheinen weder in der Suche noch in Listen oder auf Kontaktseiten, und niemand kann ihnen schreiben oder mailen.",
  "linkedinImport.choose": "Connections.csv ausw\u00e4hlen",
  "linkedinImport.importLabel": "Kontakt-Export",
  "linkedinImport.noMatchesYet":
    "Noch keine Treffer, und das ist in einer neuen Organisation normal: Deine Kontakte werden mit den Personen abgeglichen, die das CRM kennt, und die entstehen erst beim Lesen deiner E-Mails. Der Abgleich l\u00e4uft st\u00fcndlich erneut, Treffer erscheinen also nach und nach.",
  "linkedinImport.working": "Export wird gelesen…",
  "linkedinImport.imported": "Kontakte importiert",
  "linkedinImport.confirmed": "Einer Person zugeordnet",
  "linkedinImport.suggested": "Wartet auf deine Bestätigung",

  // Die Prüfliste und die Reichweiten-Tabelle (ADR-0078 §2.1b).
  "linkedinReach.title": "Wohin dein Netzwerk reicht",
  "linkedinReach.sub":
    "Firmen im CRM, bei denen du bereits jemanden kennst — die meisten Verbindungen zuerst.",
  "linkedinReach.empty":
    "Noch keine deiner Verbindungen arbeitet bei einer erfassten Firma.",
  "linkedinReach.allUnresolved":
    "Alle {unresolved} deiner Verbindungen arbeiten bei Firmen, die noch nicht erfasst sind.",
  "linkedinReach.accountsLabel": "Firmen, die du erreichst",
  "linkedinReach.account": "Firma",
  "linkedinReach.connections": "Du kennst",
  "linkedinReach.onFile": "Bereits Personen",
  "linkedinReach.onFileOf": "{onFile} von {total}",
  "linkedinReach.footnote":
    "{shown} von {total} Firmen angezeigt. {unresolved} Verbindungen arbeiten bei einer Firma, die noch nicht erfasst ist.",
  "linkedinImport.skipped": "Übersprungen (kein verwertbarer Name)",
  "co.signals.title": "Margince ist außerdem aufgefallen",
  "co.signals.emptyDetail":
    "Margince liest Meetings, Mails und Rechnungen auf Zusagen, Blocker und Risiken. Dafür braucht es zuerst mindestens eines davon.",
  "co.signals.empty": "Kein offenes Signal zu diesem Account.",
  "co.signals.openProject": "Projekt öffnen",
  "co.signals.openSource": "Meldung lesen",
  "chronology.label": "Was im Verlauf angezeigt wird",
  "chronology.activities": "Aktivitäten",
  "chronology.changes": "Änderungen",
  "filter.label": "Liste eingrenzen",
  "chronology.all": "Alles",
  "chronology.conversations": "Gespräche",
  "chronology.conversationsEmpty": "Noch keine Gespräche mit ihnen.",
  "convo.yourMove": "Du bist dran",
  "convo.waitingOnThem": "Wartet auf Antwort",
  "chronology.changesEmpty":
    "Seit dem Anlegen wurde kein Feld dieses Datensatzes geändert.",
  "chronology.allEmpty": "Zu diesem Datensatz ist noch nichts passiert.",
  "chronology.truncated":
    "Ältere Einträge fehlen hier — es gibt mehr von beidem, als diese Ansicht in eine Reihenfolge bringen kann. Wähle Aktivitäten oder Änderungen, um weiter zurückzulesen.",
  "chronology.truncatedActivities":
    "Es gibt hier mehr Aktivitäten, als hineinpassen. Gezeigt werden nur die neuesten.",
  "timeline.sentTo": "An {who}",
  "timeline.receivedFrom": "Von {who}",
  "timeline.withWhom": "Mit {who}",
  "timeline.fieldUpdated": "Feld geändert",
  "timeline.sent": "Gesendet",
  "timeline.received": "Erhalten",
  "timeline.kind.email": "E-Mail",
  "timeline.kind.meeting": "Termin",
  "timeline.kind.note": "Notiz",
  "timeline.kind.call": "Anruf",
  "timeline.kind.task": "Aufgabe",
  "timeline.kind.message": "Nachricht",
  "timeline.kind.change": "Datensatz",
  "timeline.withheld": "Inhalt nur f\u00fcr Beteiligte",
  "compose.deadRecipients":
    "E-Mails an {addresses} kommen nicht an. Die letzte Zustellung dorthin wurde abgelehnt, und seitdem ist keine Zustellung mehr durchgekommen. Trotzdem senden oder eine andere Adresse verwenden.",
  "compose.threadShare": "Verlauf teilen",
  "compose.threadKeepPrivate": "Privat halten",
  "compose.threadStillHeld":
    "Weiterhin zurückgehalten: {count} weitere Person(en) in diesem Verlauf haben ihn nicht freigegeben.",
  "compose.reason.posture": "Durch Ihre Einstellung zurückgehalten",
  "compose.reason.workspaceFloor": "Durch die Organisation zurückgehalten",
  "compose.reason.noRecord": "Zurückgehalten, kein Datensatz",
  "compose.reason.pendingVerdict": "Bis zur Einstufung zurückgehalten",
  "compose.reason.manual": "Privat gehalten",
  "compose.audience": "Sichtbarkeit",
  "compose.audienceTitle": "Wer darf diese Nachricht lesen?",
  "compose.audienceLegend": "Sichtbarkeit dieser einen Nachricht",
  "email.aMessage": "Eine Nachricht",
  "email.noSubject": "Kein Betreff",
  "email.withheldSubject": "Nicht für Sie freigegeben",
  "email.receivedFrom": "Erhalten von",
  "email.sentTo": "Gesendet an",
  "email.access.team": "Team",
  "email.access.participants": "Beteiligte",
  "email.access.selected": "Ausgewählte",
  "email.access.withheld": "Zurückgehalten",
  "email.move.needsReply": "Antwort offen",
  "email.move.waitingForThem": "Warten auf Antwort",
  "email.detail.loading": "Nachricht wird geöffnet",
  "email.detail.none": "Diese Nachricht",
  "email.detail.attachments_one": "{count} Anhang",
  "email.detail.attachments_other": "{count} Anhänge",
  "email.detail.showQuoted": "Zitierten Verlauf anzeigen",
  "email.detail.close": "Schließen",
  "email.detail.withheldReason":
    "Diese Nachricht ist nicht für Sie freigegeben",
  "email.detail.from": "Von",
  "email.detail.to": "An",
  "email.detail.cc": "Cc",
  "email.detail.bccWithheld":
    "Einige Empfänger stehen im Blindkopie-Feld und werden Ihnen nicht angezeigt",
  "compose.audienceWorkspace": "Alle in der Organisation",
  "compose.audienceWorkspaceHint":
    "Jeder, der den Kontakt sehen darf, liest auch diese Nachricht.",
  "compose.audienceParticipants": "Nur Beteiligte",
  "compose.audienceParticipantsHint":
    "Nur die Personen auf dieser Nachricht lesen Betreff und Inhalt. Andere sehen nur, dass an diesem Tag eine Nachricht gewechselt wurde.",
  "compose.audienceSelected": "Benannte Personen",
  "compose.audienceSelectedHint":
    "nur die Personen und Teams, die Sie benennen, sowie alle, die bereits auf der Nachricht stehen.",
  "compose.audienceMembersLegend": "Wer sie lesen darf",
  "compose.audienceMembersLoading": "Personenliste wird gelesen…",
  "compose.audienceConfirm": "Sichtbarkeit speichern",
  "compose.audienceNote":
    "Gilt nur f\u00fcr diese Nachricht \u2014 nicht f\u00fcr den Thread und nicht f\u00fcr den Kontakt.",
  "timeline.textMore": "Lesen",
  "timeline.textLess": "Weniger",
  "timeline.tailMore": "Signatur und Zitat anzeigen",
  "timeline.tailLess": "Signatur und Zitat ausblenden",
  "co.profileField.display_name": "Firmenname",
  "co.profileField.offer_summary": "Was sie verkaufen",
  "co.profileField.icp": "An wen sie verkaufen",
  "co.profileField.buying_center": "Wer dort entscheidet",
  "co.profileField.value_proposition": "Was sie versprechen",
  "co.profileField.usp": "Wodurch sie sich abheben",
  "co.profileField.customer_pains": "Welches Problem sie lösen",
  "co.profileField.desired_outcomes": "Welches Ergebnis sie versprechen",
  "co.profileField.buying_intents": "Was einen Kauf auslöst",
  "co.profileField.common_objections": "Einwände, die sie hören",
  "co.profileField.sales_motion": "Wie sie verkaufen",
  "co.profileField.legal_name": "Eingetragener Name",
  "co.profileField.registered_address": "Eingetragene Anschrift",
  "co.profileField.register_vat": "Register / USt-IdNr.",
  "co.profileField.legal_form": "Rechtsform",
  "co.profileField.register_court": "Registergericht",
  "co.profileField.register_number": "Registernummer",
  "co.profileField.industry": "Branche",
  "co.profileField.history": "Historie",
  "co.narrative.title": "Was sie tun",
  "co.narrative.sub":
    "Die Geschichte des Kontos, wie sie die Website erzählt. Korrigieren Sie, was falsch ist - eine Korrektur bleibt, der nächste Lesevorgang überschreibt sie nicht.",
  "co.narrative.add": "Hinzufügen",
  "co.people.engagement": "Kontaktstand",
  "co.people.lastInteraction": "Letzter Austausch",
  "co.people.strength": "Beziehung",
  "co.people.neverInTouch": "Noch kein Austausch",
  "co.people.theyWrote": "Sie schrieben",
  "co.people.weWrote": "Wir schrieben",
  "co.people.filter.status": "Kontaktstand",
  "co.people.filter.statusAll": "Beliebiger Kontaktstand",
  "co.people.band.wayIn": "Bester Zugang",
  "co.people.band.noWayIn": "Niemand hat geantwortet",
  "co.people.band.noWayInWhy":
    "Alle wurden angeschrieben, niemand hat geantwortet",
  "co.people.band.showAnswered": "Wer geantwortet hat",
  "co.people.band.committee": "Buying-Team",
  "co.people.band.missing": "Kein {role}",
  "co.people.band.committeeComplete": "Champion und Economic Buyer benannt",
  "co.people.band.committeeUnread": "Für dich nicht sichtbar",
  "co.people.band.committeeUnreadWhy":
    "Deine Rolle darf die Deals dieses Kontos nicht lesen",
  "co.people.band.seatsHeld": "{count} im Team",
  "co.people.band.someHidden": "{count} weitere für dich nicht sichtbar",
  "co.people.band.coverage": "Abdeckung",
  "co.people.band.reachable": "{count} antworten",
  "co.people.band.untried": "{count} nie kontaktiert",
  "co.people.band.showUntried": "Wer nie kontaktiert wurde",
  "co.people.board.nobodyHolds": "Diese Rolle ist unbesetzt",
  "co.people.band.noOpenDeal": "Kein offener Deal",
  "co.people.band.noOpenDealWhy": "Buying-Rollen werden auf einem Deal erfasst",
  "co.people.band.committeePartial": "Nicht beurteilbar",
  "co.people.band.showAll": "Alle anzeigen",
  "co.people.board.otherRoles": "Weitere Rollen",
  "co.people.band.unavailable": "Konnte nicht geladen werden",
  "co.people.band.unavailableWhy":
    "Die Abdeckung konnte nicht geladen werden; die Liste unten ist davon unberührt",
  "co.people.view": "Team-Ansicht",
  "co.people.view.board": "Board",
  "co.people.view.map": "Karte",
  "co.people.map.region": "Wer wen in diesem Konto erreicht",
  "co.people.map.bestRoute": "Bester Weg",
  "co.people.map.alternatives": "Alternativen",
  "co.people.map.noRoute": "Kein Weg erfasst",
  "co.people.map.more": "{count} weitere zeigen",
  "co.people.map.clear": "Auswahl aufheben",
  "co.people.map.emptyTitle": "Noch kein Weg erfasst",
  "co.people.map.emptyBody":
    "Vergib die Buying-Rollen oder importiere die vorhandenen Interaktionen.",
  "co.people.map.nothingSelected":
    "Wähle eine Person, um den besten Weg zu ihr zu sehen.",
  "co.people.map.ourSide": "Unsere Seite",
  "co.people.map.account": "Konto",
  "co.people.map.missing": "{role} fehlt",
  "co.people.map.awaiting": "wartet auf Antwort",
  "co.people.map.replied": "sie haben geantwortet",
  "co.people.map.never": "nie angeschrieben",
  "co.people.map.onDeal": "auf dem Deal",
  "co.people.map.routesWithheld":
    "Wer sie erreicht, ist für dich nicht sichtbar",
  "co.people.map.assignHint": "Niemand trägt diesen Deal",
  "co.people.map.scope": "{count} im Buying-Team · nur der gewählte Deal.",
  "co.people.map.scopePartial":
    "{count} im Buying-Team · {hidden} weitere für dich nicht sichtbar.",
  "co.people.board.readFromMessages": "Aus ihren Nachrichten gelesen",
  "co.intro.title": "Um eine Vorstellung bitten",
  "co.intro.who": "{colleague} wird gebeten, Sie {contact} vorzustellen.",
  "co.intro.write": "Nachricht schreiben",
  "co.intro.writing": "Wird geschrieben",
  "co.intro.fromTemplate":
    "Aus einer Vorlage geschrieben — in dieser Installation ist kein Modell eingerichtet.",
  "co.intro.subject": "Betreff",
  "co.intro.body": "Nachricht",
  "co.intro.basedOn": "Grundlage",
  "co.intro.copy": "Kopieren",
  "co.intro.copyFailed":
    "Der Browser hat das Kopieren nicht zugelassen. Markieren Sie die Nachricht und kopieren Sie sie selbst.",
  "co.intro.copied": "Kopiert",
  "co.intro.openMail": "Im E-Mail-Programm öffnen",
  "co.map.askIntro": "Um Vorstellung bitten",
  "co.people.board.suggest": "Rollen vorschlagen",
  "co.people.board.suggesting": "Nachrichten werden gelesen",
  "co.people.board.suggestNoDeal":
    "Rollen gehören zu einem Deal, und dieses Unternehmen hat keinen offenen.",
  "co.people.board.suggestWrote":
    "{count} aus ihren eigenen Worten eingetragen.",
  "co.people.board.suggestUnavailable":
    "Für das Lesen von Rollen wird ein Modell benötigt; in dieser Installation ist keines eingerichtet.",
  "co.people.board.suggestNothing":
    "In ihren Nachrichten steht nicht, wer entscheidet.",
  "co.people.board.suggestRefused":
    "Nichts war eindeutig genug. {count} Lesung(en) wurden wegen schwacher Belege verworfen.",
  "co.people.board.confirm": "Bestätigen",
  "co.people.board.confirming": "Wird bestätigt",
  "co.people.board.change": "Rolle ändern",
  "co.reach.answered": "Antwortet",
  "co.reach.silent": "Keine Antwort",
  "co.reach.untried": "Nie angesprochen",
  "co.role.champion": "Champion",
  "co.role.economic_buyer": "wirtschaftlicher Entscheider",
  "co.role.blocker": "Bremser",
  "co.role.influencer": "Einflussnehmer",
  "co.role.user": "Anwender",
  "co.evidence.extractedUnconfirmed": "KI-extrahiert · noch nicht bestätigt",
  "co.evidence.previous": "Vorherige Aussage",
  "co.evidence.next": "Nächste Aussage",
  "co.evidence.title": "Woher das stammt",
  "co.relationships.title": "Verknüpfte Personen und Firmen",
  "co.tools.title": "Daten & Werkzeuge",
  "co.prep.withheld":
    "Teile dieses Accounts sind für dich nicht sichtbar. Diese Einschätzung ist deshalb unvollständig.",
  "co.read.newActivity_one": "Ein neuer Vorgang seit deinem letzten Besuch.",
  "co.read.newActivity_other":
    "{count} neue Vorgänge seit deinem letzten Besuch.",
  "co.factField.founded_year": "Gegründet",
  "co.factField.employee_range": "Mitarbeitende",
  "co.factField.phone": "Telefon",
  "co.factField.contact_email": "Kontakt-E-Mail",
  "co.factField.location": "Standort",
  "co.factField.service": "Leistung",
  "co.factField.product": "Produkt",
  "co.factField.capability": "Fähigkeit",
  "co.factField.served_industry": "Bedient",
  "co.factField.company_size": "Größe",
  "co.factField.geography": "Region",
  "co.factField.language": "Sprache",
  "co.factField.certification": "Zertifizierung",
  "co.factField.partner": "Partner",
  "co.factField.named_customer": "Kunde",
  "co.factField.technology": "Technologie",
  "co.factField.mail_provider": "Mailsystem",
  "co.factField.email_security": "Mail-Authentifizierung",
  "co.factField.hosting_provider": "Hosting",
  "co.factField.operated_service": "Dienst",
  "co.vat.markVerdict": "USt-IdNr.: {verdict}",
  "co.vat.markUnchecked": "USt-IdNr.: noch nicht beim Register abgefragt",
  "co.vat.markUnreadable":
    "USt-IdNr.: Die Prüfung konnte gerade nicht geladen werden — zum Wiederholen drücken",
  "co.vat.numberMoved":
    "Die Nummer auf diesem Datensatz hat sich seit dieser Abfrage geändert. Fragen Sie erneut ab, um die neue zu prüfen.",
  "co.vat.verdict": "Antwort des Registers",
  "co.vat.number": "Abgefragte Nummer",
  "co.vat.registeredName": "Eingetragen auf",
  "co.vat.registeredAddress": "Eingetragene Anschrift",
  "co.vat.checkedAt": "Abgefragt am",
  "co.vat.receipt": "Abfrage-Nummer",
  "co.vat.status.valid": "Gültig",
  "co.vat.status.invalid": "Nicht gültig",
  "co.vat.noReceipt":
    "Keine vergeben. Das Register vergibt eine Abfrage-Nummer nur für eine Abfrage unter eurer eigenen USt-IdNr. — trag sie in den Einstellungen ein, dann trägt die nächste Abfrage einen Nachweis, den ein Finanzamt akzeptiert.",
  "co.vat.never":
    "Die USt-IdNr. dieser Firma wurde noch nicht abgefragt. Das passiert von selbst, sobald die Nummer aus dem Impressum gelesen wird — oder Sie fragen jetzt beim Register nach.",
  "co.vat.askNow": "Beim Register abfragen",
  "co.vat.askAgain": "Erneut abfragen",
  "co.vat.askingBusy": "Register wird gefragt",
  "co.vat.asking":
    "Das Register wird gefragt — die Antwort erscheint hier, sobald sie vorliegt.",
  "co.tech.title": "Technik",
  "co.tech.sub":
    "Was diese Firma öffentlich betreibt — gelesen aus ihren DNS-Einträgen, ihren Zertifikaten und ihrer eigenen Startseite.",
  "co.tech.mail": "Mail",
  "co.tech.web": "Website-Technik",
  "co.tech.services": "Dienste",
  "co.tech.hosting": "Hosting",
  "co.tech.empty":
    "Noch nichts Technisches gelesen. Das füllt sich von selbst, sobald die Website der Firma gelesen wird, und frischt sich selbst auf.",
  "co.tech.laneFailed":
    "{lane} hat nicht geantwortet — was von dort zuletzt kam, bleibt unverändert.",
  "co.tech.laneRefused": "Die Website möchte nicht gelesen werden.",
  "co.tech.lane.dns": "DNS",
  "co.tech.lane.certlog": "Zertifikate",
  "co.tech.lane.homepage": "Startseite",
  "signal.kind.technical_change": "Technik geändert",
  "co.factField.quantified_outcome": "Ergebnis",
  "co.facts.title": "Fakten über diese Firma",
  "co.facts.empty":
    "Noch nichts erfasst. Lesen Sie die Website, oder tragen Sie ein, was Sie bereits wissen.",
  "co.facts.add": "Fakt hinzufügen",
  "co.facts.addField": "Art des Fakts",
  "co.facts.addValue": "Was er besagt",
  "co.facts.addSave": "Fakt speichern",
  "co.facts.addCancel": "Abbrechen",
  "co.facts.addIncomplete":
    "Wählen Sie die Art des Fakts und tragen Sie ein, was er besagt.",
  "co.facts.remove": "{value} entfernen",
  "co.facts.removeTitle": "Diesen Fakt entfernen?",
  "co.facts.removeConfirm": "Entfernen",
  "co.facts.removeAsk":
    "{field} ist als \u201e{value}\u201c erfasst. Ihn zu entfernen bedeutet: Das ist kein Fakt über die Firma. Ein späteres Lesen der Website kann ihn erneut erfassen.",
  "co.facts.showAll": "Alle {count} anzeigen",
  "co.facts.showLess": "Weniger anzeigen",
  "co.project.new": "Neues Projekt",
  "co.deal.new": "Neuer Deal",
  "co.recent.title": "Was zuletzt passiert ist",
  "co.recent.emptyDetail":
    "Sobald Sie eine E-Mail senden, einen Anruf festhalten oder sich treffen, steht der Austausch hier, mit dem, was jede Seite getan hat.",
  "co.recent.empty": "Noch nichts mit ihnen erfasst.",
  "co.recent.viewHistory": "Verlauf ansehen",
  "co.recent.kind.email": "E-Mail",
  "co.recent.kind.call": "Anruf",
  "co.recent.kind.meeting": "Termin",
  "co.recent.kind.note": "Notiz",
  "co.recent.kind.task": "Aufgabe",
  "co.recent.kind.message": "Nachricht",
  "co.recent.dir.theyWrote": "sie schrieben",
  "co.recent.dir.weSent": "wir schrieben",
  "co.recent.dir.theyCalled": "sie riefen an",
  "co.recent.dir.weCalled": "wir riefen an",
  "co.recent.dir.both": "beide Seiten",
  "co.recent.minutes": "{count} Min.",
  "co.recent.re": "zu einem Deal",
  "co.recent.reNamed": "zu {name}",
  "tagAdmin.title": "Tags",
  "tagAdmin.sub":
    "Die Wörter, unter denen diese Organisation Datensätze ablegt. Anwenden darf jeder; anlegen, umbenennen und stilllegen nur Admin- und Ops-Plätze.",
  "tagAdmin.listLabel": "Vokabular",
  "tagAdmin.empty": "Noch keine Tags. Legen Sie das erste Wort an.",
  "import.contextTag": "Diesen Stapel unter einem Tag ablegen",
  "import.contextTagChosen":
    "Neu angelegte Datensätze werden unter {name} abgelegt.",
  "import.contextTagChosenUnnamed":
    "Neu angelegte Datensätze werden unter dem für diesen Lauf gewählten Tag abgelegt.",
  "import.contextTagHint":
    "Wird auf neu angelegte Datensätze angewendet, damit der Stapel auffindbar bleibt. Aktualisierte Datensätze behalten ihre Tags.",
  "import.contextTagNone": "Kein Tag",
  "tagAdmin.add": "Tag anlegen",
  "tagAdmin.addTitle": "Tag anlegen",
  "tagAdmin.editTitle": "Tag bearbeiten",
  "tagAdmin.nameLabel": "Name",
  "tagAdmin.colorLabel": "Farbe",
  "tagAdmin.colorNone": "Keine Farbe",
  "tagAdmin.create": "Anlegen",
  "tagAdmin.save": "Speichern",
  "tagAdmin.edit": "Bearbeiten",
  "tagAdmin.merge": "Zusammenführen",
  "tagAdmin.archive": "Stilllegen",
  "tagAdmin.restore": "Wiederherstellen",
  "tagAdmin.usage": "{count} Datensätze",
  "tagAdmin.usagePending": "Wird gezählt…",
  "tagAdmin.nearMatch":
    "Ähnlich einem vorhandenen Wort: {names}. Verwenden Sie dieses, sofern nicht wirklich etwas anderes gemeint ist.",
  "tagAdmin.mergeTitle": "{name} in ein anderes Tag zusammenführen",
  "tagAdmin.mergeIntoLabel": "Dieses Tag behalten",
  "tagAdmin.mergeIntoNone": "Tag wählen",
  "tagAdmin.mergeConfirm": "Zusammenführen",
  "tagAdmin.mergeWarning":
    "Nicht widerrufbar. Datensätze mit {name} tragen danach das andere Tag, und der Name wird wieder freigegeben.",
  "tagAdmin.mergedTitle": "Zusammengeführt",
  "tagAdmin.mergedBody":
    "{moved} Datensätze übernommen. {collapsed} trugen bereits beide; das doppelte Tag wurde entfernt.",
  "tagAdmin.countUsage": "Datensätze zählen",
  "tagAdmin.noVersion":
    "Dieses Tag wurde ohne Version gelesen und kann nicht gespeichert werden. Seite neu laden und erneut versuchen.",
  "tagAdmin.withheld":
    "Sie haben keinen Zugriff auf das Tag-Vokabular dieser Organisation.",
  "tagAdmin.truncated":
    "Diese Liste ist gekürzt. Wörter jenseits der Grenze erscheinen hier nicht und lassen sich nicht bearbeiten.",
  "tagAdmin.usageFailed": "Zählung nicht verfügbar",
  "tagAdmin.done": "Fertig",
  "tags.archived": "archiviert",
  "tags.columnHeader": "Tags",
  "tags.filterAll": "Beliebiger Tag",
  "tags.moreUncounted": "weitere",
  "tags.moreUncountedTip": "Darunter {names}. Alle im Datensatz.",
  "tags.columnHeaderPartial": "Tags (Teilliste)",
  "tags.loading": "Tags werden geladen…",
  "tags.panelTitle": "Tags",
  "tags.panelSub": "Tag öffnen oder über das Menü diese Zuordnung verwalten",
  "tags.add": "Tag hinzufügen",
  "tags.more": "+{count} weitere",
  "tags.showLess": "Weniger anzeigen",
  "tags.options": "Optionen für {name}",
  "tags.addedBy": "Hinzugefügt von {who} · {when}",
  "tags.addedOn": "Hinzugefügt {when}",
  "tags.visibleWorkspaceWide":
    "Tag-Namen sind in der gesamten Organisation sichtbar.",
  "tags.removeFromRecord": "Von diesem Datensatz entfernen",
  "tags.withheld": "Verborgen — Ihre Rolle kann das Tag-Vokabular nicht lesen",
  "tags.emptyTitle": "Noch keine Tags",
  "tags.emptyBody":
    "Fügen Sie dauerhaften Kontext hinzu, etwa eine Veranstaltung, eine Beziehung oder eine Kohorte.",
  "tags.pickerLabel": "Tag suchen",
  "tags.alreadyAdded": "Bereits hinzugefügt",
  "tags.catalogTruncated":
    "Diese Liste ist gekürzt, ein Wort kann also fehlen. Suchen Sie danach, bevor Sie ein neues anfragen.",
  "tags.noMatch":
    "Kein Tag mit diesem Namen. Ein Admin- oder Ops-Platz kann eines zum Vokabular hinzufügen.",
  "tagResult.gone":
    "Dieses Tag existiert nicht mehr. Es wurde möglicherweise zusammengeführt.",
  "tagResult.totalVisible": "{count} sichtbare Zuordnungen",
  "tagResult.people": "Personen",
  "tagResult.companies": "Unternehmen",
  "tagResult.deals": "Deals",
  "tagResult.viewAll": "Alle {count} {kind} anzeigen",
  "tagResult.resultsTitle": "Datensätze mit diesem Tag",
  "tagResult.nothingCarries":
    "Noch kein Datensatz trägt dieses Tag. Vergeben Sie es auf einem Kontakt, einem Unternehmen oder einem Deal.",
  "tagResult.loadingRows": "{kind} werden geladen…",
  "tagResult.noneLeft": "Trägt niemand mehr",
  "tagResult.unnamed": "Ohne Namen",
  "co.timeline.empty": "Zu diesem Account ist noch nichts erfasst.",
  "co.overlayFallback":
    "Dieser Account wird aus dem verbundenen führenden System bedient; die Firmenansicht wird hier nicht zusammengestellt. \u00d6ffne ihn dort für das vollständige Bild.",
  "org.domains": "Domains",
  "org.factCategory.company": "Unternehmen",
  "org.factCategory.offering": "Angebot",
  "org.factCategory.market": "Markt",
  "org.factCategory.signal": "Signale",

  "lead.score": "Score",
  "lead.status": "Status",
  "lead.nextTask": "Nächster Schritt",
  "lead.openTaskCount": "{count} offene Aufgaben",
  "lead.noNextTask": "Kein nächster Schritt",
  "lead.scoreNoSignals": "Keine qualifizierenden Signale",
  "lead.source": "Quelle",
  "lead.project": "Projekt",
  "lead.openLinkedIn": "LinkedIn-Profil öffnen",
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
    "Von einer Integration geschrieben — behält ihre eigene Quelle.",
  "leadSources.title": "Lead-Quellen",
  "leadSources.sub":
    "Woher Leads kommen. Wird im Formular „Neuer Lead“, als Filter und vom Score verwendet.",
  "leadSources.readOnly": "Nur ein Admin- oder Ops-Sitz ändert diese Liste.",
  "leadSources.labelFor": "Bezeichnung der Quelle {key}",
  "leadSources.intentFor": "Gewichtung von {label}",
  "leadSources.intent": "Gewichtung",
  "leadSources.intent.high": "Hohes Interesse",
  "leadSources.intent.neutral": "Neutral",
  "leadSources.intent.low": "Geringes Interesse",
  "leadSources.intentHint":
    "Hoch gibt Punkte auf den Score, Gering zieht ab; eine Änderung greift bei der nächsten Neuberechnung jedes Leads.",
  "leadSources.leadCount": "{count} Leads",
  "leadSources.builtIn": "vorgegeben",
  "leadSources.builtInKept":
    "Vorgegebene Quellen lassen sich umbenennen und abschalten, nicht entfernen.",
  "leadSources.inUse":
    "{count} Leads nutzen diese Quelle — stattdessen abschalten.",
  "leadSources.deactivateInstead": "stattdessen abschalten",
  "leadSources.activeFor": "{label} ist aktiv",
  "leadSources.remove": "Entfernen",
  "leadSources.removeTitle": "Diese Quelle entfernen?",
  "leadSources.removeBody":
    "„{label}“ wird von keinem Lead genutzt und verschwindet aus der Liste.",
  "leadSources.newLabel": "Neue Quelle",
  "leadSources.labelField": "Bezeichnung",
  "leadSources.addOpen": "Neue Quelle",
  "leadSources.listLabel": "Quellen in der Liste",
  "leadSources.discovered": "Entdeckte Werte",
  "leadSources.newPlaceholder": "Messe",
  "leadSources.add": "Quelle hinzufügen",
  "leadSources.discoveredSub":
    "Aus Integrationen und Importen — Werte, die auf Leads stehen, aber noch nicht in der Liste sind. Hinzufügen gibt ihnen Bezeichnung und Gewichtung.",
  "leadSources.adopt": "In die Liste aufnehmen",
  "leadReasons.title": "Disqualifizierungsgründe",
  "leadReasons.sub":
    "Was beim Verwerfen eines Leads gewählt wird. Der Grund steht am Lead und ist filterbar.",
  "leadReasons.labelFor": "Bezeichnung des Grundes {label}",
  "leadReasons.leadCount": "{count} Leads",
  "leadReasons.inUse":
    "{count} Leads tragen diesen Grund — stattdessen abschalten.",
  "leadReasons.newLabel": "Neuer Grund",
  "leadReasons.listLabel": "Gründe in der Liste",
  "leadReasons.add": "Grund hinzufügen",
  "leadReasons.removeTitle": "Diesen Grund entfernen?",
  "leadReasons.removeBody":
    "„{label}“ wird von keinem Lead genutzt und verschwindet aus der Liste.",
  "leadHandling.title": "Lead-Bearbeitung",
  "leadHandling.sub": "Wie diese Installation mit einem neuen Lead umgeht.",
  "leadHandling.firstResponse": "Ziel für die erste Antwort",
  "leadHandling.firstResponseHint":
    "Standardmäßig aus. An: jeder offene Lead trägt eine Antwortfrist, die Liste bekommt die Ansicht „Überfällig“, und überfällige Leads stehen zuerst.",
  "leadHandling.targetMinutes": "Ziel (Minuten)",
  "leadHandling.targetOutOfRange":
    "Eine ganze Minutenzahl zwischen 15 und 10080 (7 Tage) eingeben.",
  "leadHandling.targetHint":
    "Wie lange ein Lead nach Zuweisung (oder Anlage) auf die erste Antwort warten darf. 15 Minuten bis 7 Tage.",
  "lead.boardCount": "{count} Leads",
  "lead.duplicateFound":
    "Ein Lead mit dieser E-Mail oder diesem LinkedIn-Profil existiert bereits.",
  "lead.promote": "Qualifizieren",
  "lead.promoteIneligible":
    "braucht eine E-Mail-Adresse und einen offenen Status",
  "lead.filterStatus": "Status",
  "lead.filterStatusAll": "Alle Status",
  "lead.filterScore": "Score",
  "lead.filterScoreAll": "Beliebiger Score",
  "lead.bulkSelected": "{count} ausgewählt",
  "lead.bulkOwner": "Neuer Verantwortlicher",
  "lead.bulkOwnerPick": "Verantwortlichen wählen",
  "lead.bulkAssign": "Zuweisen",
  "lead.bulkDisqualify": "Disqualifizieren",
  "lead.bulkDisqualifyTitle_one": "Diesen Lead disqualifizieren?",
  "lead.bulkDisqualifyTitle_other": "{count} Leads disqualifizieren?",
  "lead.bulkDisqualifyBody":
    "Geschlossen mit dem Grund \u201e{reason}\u201c. Jeder Lead beh\u00e4lt seinen eigenen Datensatz, und es gibt keinen einzelnen Schritt zur\u00fcck.",
  "lead.bulkFailed": "{count} nicht übernommen –",
  "lead.bulkFailedRow": "konnte nicht gespeichert werden",
  "lead.bulkSelectRow": "{name} auswählen",
  "lead.unnamed": "Lead ohne Namen",
  "lead.sla.breached": "Überfällig",
  "lead.sla.atRisk": "Bald fällig",
  "lead.sla.withinTarget": "Im Rahmen",
  "lead.sla.answeredAt": "Erste Antwort am {at}",
  "lead.sla.dueBy": "Erste Antwort fällig bis {at}",
  "lead.sla.overdueSince": "Erste Antwort war fällig am {at}",
  "lead.filterSla": "Antwort",
  "lead.filterSlaAll": "Alle",
  "list.viewOverdue": "Überfällig",
  "lead.filterScoreHot": "Ab 80",
  "lead.filterScoreWarm": "Ab 60",
  "lead.filterScoreCool": "Ab 40",
  "lead.details": "Details",
  "lead.ladder.title": "Wo dieser Lead steht",
  "lead.railTitle": "Verantwortlich",
  "lead.detailsUnset": "Nicht gesetzt",
  "lead.terminalReadOnly":
    "Dieser Lead ist abgeschlossen und nimmt keine Änderungen an.",
  "lead.boardTerminalOnly":
    "Das Board zeigt nur offene Leads. Diese Leads sind übernommen oder disqualifiziert.",
  "person.fromLead": "Aus Lead übernommen",
  "lead.promotedTitle": "Als Kontakt übernommen",
  "lead.promotedMerged":
    "Dieser Lead wurde mit einem bereits bekannten Kontakt zusammengeführt — es entstand kein Duplikat.",
  "lead.promotedCreated": "Aus diesem Lead wurde ein neuer Kontakt.",
  "lead.promotedAt": "Übernommen am",
  "lead.promotedTrigger": "Auslöser:",
  "lead.promotedEvidence": "Beleg:",
  "lead.previewPending": "Prüfe, ob wir diese Person schon kennen …",
  "lead.previewCreate": "Die Übernahme legt einen neuen Kontakt an.",
  "lead.previewMerge":
    "Die Übernahme führt mit dem bestehenden Kontakt zusammen:",
  "lead.previewMergeWithheld":
    "Die Übernahme führt mit einem bestehenden Kontakt zusammen, den Sie nicht sehen können.",
  "lead.demote": "Übernahme rückgängig machen",
  "lead.demoteDialog": "Übernahme rückgängig machen?",
  "lead.demoteExplain":
    "Der Lead kehrt als „In Bearbeitung“ in die Liste zurück. Ein Kontakt, den die Übernahme angelegt hat, wird archiviert; ein Kontakt, mit dem zusammengeführt wurde, bleibt unverändert. Bei einem Kontakt an einem laufenden Deal ist die Rücknahme nicht möglich.",
  "lead.demoteReason": "Grund (wird im Protokoll festgehalten)",
  "lead.demoteReasonRequired": "Bitte zuerst begr\u00fcnden.",
  "lead.demoteConfirm": "Rückgängig machen",
  "lead.promotedOutcomePending":
    "Wird gelesen, was diese Übernahme bewirkt hat …",
  "lead.promotedOutcomeUnavailable":
    "Wir können nicht anzeigen, ob dabei zusammengeführt oder neu angelegt wurde.",
  "lead.terminalPromoted":
    "Übernommen — dieser Lead ist jetzt schreibgeschützt.",
  "lead.statusNew": "Neu",
  "lead.statusContacted": "Kontaktiert",
  "lead.statusEngaged": "Im Gespräch",
  "lead.statusPromoted": "Qualifiziert",
  "lead.statusDisqualified": "Disqualifiziert",
  "lead.disqualified": "Disqualifiziert",
  "lead.status.new": "Neu",
  "lead.status.contacted": "Kontaktiert",
  "lead.status.engaged": "Im Gespräch",
  "lead.explainScore": "Score erklären",
  "lead.scoreOverridden": "Manuell überschrieben: {reason}",
  "lead.machineScore": "Maschinen-Score war {score}",
  "lead.overrideScore": "Score überschreiben",
  "lead.clearOverride": "Überschreibung aufheben",
  "lead.overrideReason": "Begründung",
  "lead.shortfall.lead": "Womit dieser Score arbeitet:",
  "lead.shortfall.engagementMoves":
    "Am stärksten bewegen ihn eine Antwort oder ein Termin.",
  "lead.shortfall.noSource":
    "Keine Quelle hinterlegt — es ist nicht festgehalten, woher dieser Lead kam.",
  "lead.shortfall.sourcePenalised":
    "Kam über „{source}“ herein, was den Score mindert.",
  "lead.shortfall.noTitle": "Keine Position hinterlegt.",
  "lead.shortfall.titleNotSenior":
    "„{title}“ gehört nicht zu den Positionen, auf die das Modell achtet.",
  "lead.shortfall.sourceNoIntent":
    "Kam über „{source}“ herein — daraus allein spricht noch kein Kaufinteresse.",
  "lead.scoreNotStoredYet":
    "Die Aufschlüsselung zu diesem Score ist noch nicht gespeichert — die nächste Aktualisierung zeigt sie.",
  "lead.scoreLoading": "Begründung wird geladen…",
  "lead.scoreNoFactors": "Bisher zahlt nichts auf diesen Score ein.",
  "lead.scoreFactorsFailed":
    "Was auf diesen Score einzahlt, konnte nicht gelesen werden.",
  "lead.scoreFactorsExplainMachine":
    "Sie haben diesen Score selbst gesetzt. Die Faktoren unten erklären den Wert des Modells: {score}.",
  "lead.scoreDecayed": "{base}, halbiert sich alle 14 Tage",
  "lead.scoreSources": "{count} Aktivitäten",
  "lead.scoreReconciles": "{raw} in Summe, gerundet {rounded}, Score {score}",
  "lead.factor.decision_maker_title": "Position mit Entscheidungsbefugnis",
  "lead.factor.high_intent_source": "Aus einer Quelle mit hohem Interesse",
  "lead.factor.low_intent_source": "Aus einer Quelle mit geringem Interesse",
  "lead.factor.reply": "Hat geantwortet",
  "lead.factor.meeting_held": "Termin stattgefunden",
  "lead.factor.meeting_booked": "Termin vereinbart",
  "lead.signalsTitle": "Was Sie über diesen Lead wissen",
  "lead.signalUnset": "Nicht erfasst",
  "lead.signalClear": "Zurückziehen",
  "lead.signalBandPick": "Wert wählen",
  "lead.signalMore": "Mehr",
  "lead.signalProvenanceHint":
    "Unverändert wird eine Antwort als Schätzung ohne Konfidenzangabe festgehalten.",
  "lead.signalEvidenceQuality": "Wie verlässlich ist das?",
  "lead.signalConfidence": "Konfidenz",
  "lead.signalConfidenceUnstated": "Keine Angabe",
  "lead.signalConfidenceValue": "{value} % Konfidenz",
  "lead.signalRecordedAt": "Erfasst am {at}",
  "lead.signalSuperseded": "Zuvor {value}; ersetzt durch {source}",
  "lead.signalAutomaticSource": "eine automatische Quelle",
  "lead.signalReason": "Woher wissen Sie das?",
  "lead.signalReasonHint":
    "Optional. Was Sie hier schreiben, wird mit dem Score festgehalten.",
  "lead.signalReasonUnstated": "Keine Quelle angegeben. Manuell erfasst.",
  "lead.signalSave": "In den Score aufnehmen",
  "lead.signal.web_traffic": "Web-Traffic",
  "lead.signal.employees": "Mitarbeiter",
  "lead.signal.budget_hint": "Budget",
  "lead.signal.ask.web_traffic": "Website-Traffic?",
  "lead.signal.ask.employees": "Unternehmensgröße?",
  "lead.signal.ask.budget_hint": "Budget?",
  "lead.signal.fact": "Bestätigt",
  "lead.signal.assumption": "Geschätzt",
  "lead.signal.judgement": "Meine Einschätzung",
  "lead.signal.web_traffic.low": "Niedrig",
  "lead.signal.web_traffic.medium": "Mittel",
  "lead.signal.web_traffic.high": "Hoch",
  "lead.signal.employees.1-10": "1–10",
  "lead.signal.employees.11-50": "11–50",
  "lead.signal.employees.51-200": "51–200",
  "lead.signal.employees.201+": "201+",
  "lead.signal.budget_hint.none": "Kein Budget",
  "lead.signal.budget_hint.unknown": "Unbekannt",
  "lead.signal.budget_hint.some": "Etwas Budget",
  "lead.signal.budget_hint.confirmed": "Budget bestätigt",
  "lead.factor.manual:web_traffic": "Web-Traffic (Ihre Angabe)",
  "lead.factor.manual:employees": "Mitarbeiter (Ihre Angabe)",
  "lead.factor.manual:budget_hint": "Budget (Ihre Angabe)",
  "lead.ownerLabel": "Verantwortlich",
  "lead.ownerYou": "Sie",
  "lead.overriddenBadge": "überschrieben",
  "lead.unassigned": "Nicht zugewiesen",
  "lead.terminalDisqualified":
    "Disqualifiziert — dieser Lead ist jetzt schreibgeschützt.",
  "lead.marker": "Lead",
  "lead.assign": "Zuweisen",
  "lead.assignToMe": "Mir zuweisen",
  "lead.assignTo": "Diesen Lead zuweisen an",
  "lead.assignChoose": "Kollegin oder Kollegen wählen",
  "lead.assignNobodyElse":
    "Es gibt niemanden sonst, dem dieser Lead zugewiesen werden kann.",
  "lead.saveOverride": "Überschreibung speichern",
  "lead.overrideScoreValue": "Score",
  "lead.trigger.inboundReply": "Eingehende Antwort",
  "lead.trigger.meetingBooked": "Termin gebucht",
  "lead.trigger.meetingHeld": "Termin stattgefunden",
  "lead.trigger.humanQualify": "Manuell qualifiziert",
  "lead.evidenceNote": "Beleg-Notiz (optional)",
  "lead.segregation":
    "Leads bleiben von Kontakten getrennt. Ein Lead wird erst zum Kontakt, wenn du ihn qualifizierst.",
  "lead.segregationDismiss": "Verstanden",
  "list.emptyMine": "Dir sind keine {unit} zugewiesen.",
  "list.showAll": "Alle anzeigen",
  "lead.assignedAway":
    "{names} an {owner} zugewiesen — nicht mehr unter „Meine“.",
  "lead.viewNew": "Neu",
  "lead.viewNeedsFollowUp": "Nachfassen",
  "lead.viewEngaged": "Im Gespräch",
  "lead.ladder": "Lead-Status",
  "lead.ladder.new": "Neu — noch niemand hat Kontakt aufgenommen.",
  "lead.ladder.overlay":
    "Der Spiegel ändert den Lead-Status nicht; im Quellsystem ändern.",
  "lead.ladder.automatic":
    "{label} · automatisch aus erfasster Aktivität gesetzt",
  "lead.ladder.automaticWith": "{label} · automatisch gesetzt — {what} am {at}",
  "lead.ladder.byHand": "{label} · von Hand gesetzt",
  "lead.ladder.theyReplied": "Antwort erhalten",
  "lead.ladder.meetingBooked": "Termin gebucht",
  "lead.ladder.meetingHeld": "Termin stattgefunden",
  "lead.ladder.qualified": "Qualifiziert — dieser Lead ist jetzt ein Kontakt.",
  "lead.ladder.qualifiedOn":
    "Qualifiziert am {at} — dieser Lead ist jetzt ein Kontakt.",
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
  "lead.qualify.amountInvalid": "Eine Zahl eingeben oder leer lassen.",
  "lead.qualify.amountNoCurrency":
    "Die Basiswährung der Installation ist noch nicht geladen — gleich noch einmal versuchen oder den Betrag leer lassen.",
  "lead.qualify.why": "Warum",
  "lead.qualify.reasonReplied": "Grund: Antwort am {at}.",
  "lead.qualify.reasonMeetingBooked": "Grund: Termin gebucht für {at}.",
  "lead.qualify.reasonMeetingHeld": "Grund: Termin am {at} stattgefunden.",
  "lead.qualify.reasonHuman": "Grund: von dir qualifiziert.",
  "lead.qualify.confirm": "Qualifizieren",
  "lead.qualify.confirmWithDeal": "Qualifizieren und Deal anlegen",
  "lead.qualify.done": "{name} ist jetzt ein Kontakt:",
  "lead.disqualify.title": "{name} disqualifizieren",
  "lead.disqualify.reason": "Grund",
  "lead.disqualify.pickReason": "Grund wählen",
  "lead.disqualify.reasonRequired": "Zuerst einen Grund wählen.",
  "lead.disqualify.note": "Notiz (optional)",
  "lead.disqualify.confirm": "Disqualifizieren",

  "deals.viewBoard": "Board",
  "deals.viewTable": "Tabelle",
  "deals.amount": "Wert",
  "deals.lastSignal": "Letztes Signal",
  "deals.lastSignalNone": "noch kein Signal",
  "deals.stage": "Phase",
  "deals.close": "Erwarteter Abschluss",
  "deals.confirmAdvance": "Nach {stage} verschieben?",
  "deals.confirmTerminal":
    "Damit wird der Deal als {status} geschlossen. Erst bestätigen — bis dahin passiert nichts.",
  "deals.lostReason": "Verlustgrund",
  "deals.winNoEvidence":
    "Für diesen Deal ist kein unterschriebener Vertrag hinterlegt. Bitte geben Sie an, wie er gewonnen wurde. Die Angabe bleibt am Deal und wird in Berichten gezählt.",
  "deals.winReason": "Wie wurde er gewonnen?",
  "deals.winReasonPick": "Bitte auswählen",
  "deals.winReasonImported": "Aus einem anderen System importiert",
  "deals.winReasonPurchaseOrder": "Per Bestellung",
  "deals.winReasonVerbal": "Mündlich, persönlich oder telefonisch",
  "deals.winReasonRenewalByEmail": "Per E-Mail verlängert",
  "deals.winReasonOther": "Etwas anderes",
  "deals.winReasonDetail": "Was war es?",
  "deals.confirm": "Bestätigen",
  "deals.cancel": "Abbrechen",
  "deals.advanced": "Nach {stage} verschoben",
  "deal.pendingApprovals": "Wartet auf deine Bestätigung",
  "deal.edit": "Deal bearbeiten",
  "deal.ownerKeep": "Aktuellen Inhaber behalten",
  "deal.ownerMe": "Mir zuweisen",
  "deal.ownerUnassign": "Zuweisung aufheben",
  "deal.partnerOrg": "über Partner",
  "deal.companyWithheld": "Firma nicht sichtbar",
  "deal.partnerWithheld": "Partner nicht sichtbar",
  "deal.forecastCategory": "Forecast-Kategorie",
  "deal.strip.title": "Wie es um den Deal steht",
  "deal.seats.title": "Wer an diesem Deal beteiligt ist",
  "deal.seats.empty": "Für diesen Deal ist niemand erfasst",
  "deal.seats.ours": "{count} von uns tragen ihn",
  "deal.committee.title": "Das Buying Center",
  "deal.committee.empty": "Für diesen Deal ist niemand hinterlegt",
  "deal.committee.engaged": "Im Austausch",
  "deal.committee.quiet": "Keine Antwort",
  "deal.committee.unnamedSeat": "Beteiligte Person, für Sie nicht sichtbar",
  "deal.committee.legendEngaged": "Im Austausch mit uns",
  "deal.committee.legendQuiet": "Am Deal beteiligt, aber still",
  "deal.committee.legendGap": "Fehlende Abdeckung",
  "deal.committee.threads":
    "{engaged} von {total} Beteiligten sprechen mit uns.",
  "deal.strip.money": "Das Geld",
  "deal.strip.money.offer": "Angebot {number} · {status}",
  "deal.strip.money.noOffer": "Noch kein Angebot geschrieben",
  "deal.strip.close": "Der Abschluss",
  "deal.strip.close.none": "Kein Datum",
  "deal.strip.close.noneDetail":
    "Niemand hat gesagt, wann das abgeschlossen wird",
  "deal.strip.close.inDays": "in {days} Tagen",
  "deal.strip.close.overdue": "{days} Tage über dem Datum",
  "deal.strip.close.provisional": "vorläufig, von niemandem bestätigt",
  "deal.strip.close.waiting": "wir sollen bis {date} warten",
  "deal.strip.people": "Die Menschen",
  "deal.strip.people.count": "{engaged} von {total} im Austausch",
  "deal.strip.people.champion": "ein Fürsprecher ist benannt",
  "deal.strip.people.noChampion": "kein Fürsprecher benannt",
  "deal.strip.people.none": "Niemand",
  "deal.strip.people.noneDetail": "Für diesen Deal ist niemand erfasst",
  "deal.strip.momentum": "Die Bewegung",
  "deal.strip.momentum.detail": "seit dem letzten Kontakt",
  "deal.strip.withheld": "Verborgen",
  "deal.strip.withheldDetail":
    "Sie dürfen nicht sehen, wer an diesem Deal beteiligt ist",
  "deal.forecast.commit": "zugesagt",
  "deal.forecast.bestCase": "bester Fall",
  "deal.forecast.pipeline": "Pipeline",
  "deal.forecast.omitted": "nicht in der Prognose",
  "deal.pulse.yourMove": "Sie sind am Zug.",
  "deal.pulse.theirMove": "Die andere Seite ist am Zug.",
  "deal.pulse.theirMoveWhy": "Hier wartet niemand auf eine Antwort.",
  "deal.pulse.wroteOn": "Zuletzt geschrieben am {date} — vor {days} Tagen.",
  "deal.pulse.wroteUnknown":
    "Sie haben geschrieben und niemand hat geantwortet.",
  "deal.waitUntil": "Warten bis",
  "deal.fxBase": "Basis {value} · Kurs {rate} vom {date}",
  "deal.archive": "Deal archivieren",
  "deal.archiveConfirm":
    "Durch das Archivieren wird dieser Deal aus der aktiven Pipeline entfernt. Dies kann in der Oberfläche nicht rückgängig gemacht werden.",
  "deal.archivedReadOnly":
    "Dieser Deal ist archiviert und nimmt keine Änderungen an.",
  "deal.reopen": "Wieder öffnen",
  "deal.reopenPick": "Diesen Deal in eine offene Phase zurücksetzen",
  "deal.reopenConfirm": "Wieder öffnen",
  "deal.fcCommit": "Commit",
  "deal.fcBestCase": "Best Case",
  "deal.fcPipeline": "Pipeline",
  "deal.fcOmitted": "Ausgeschlossen",
  "deal.fcSlipped": "Verschoben",
  "deal.fcUncategorised": "Noch keine Kategorie",

  "deals.pipeline": "Pipeline",
  "deals.filterStalled": "Nur ins Stocken geraten",
  "deals.filterOwnerMe": "Meine Deals",
  "deals.filterPartner": "Partner",
  "deals.filterPartnerAnyOne": "Alle Partner",
  "deals.filterPartnerSourced": "Über Partner",
  "deals.filterStageAll": "Alle Phasen",
  "deals.filterOrgAll": "Alle Firmen",
  "deals.filterStalledAll": "Alle Deals",
  "deals.filterOwnerAll": "Alle Inhaber",
  "deals.filterPartnerAll": "Alle Quellen",
  "deals.sortNewest": "Neueste",
  "deals.unit": "Deals",
  "deals.bulkSelected": "{count} ausgewählt",
  "deals.bulkSelectRow": "{name} auswählen",
  "deals.bulkOwner": "Neuer Verantwortlicher",
  "deals.bulkOwnerPick": "Verantwortlichen wählen",
  "deals.bulkAssign": "Zuweisen",
  "deals.bulkStage": "In Phase verschieben",
  "deals.bulkStagePick": "Phase wählen",
  "deals.bulkMove": "Verschieben",
  "deals.bulkArchive": "Archivieren",
  "deals.bulkArchiveConfirmTitle_one": "Diesen Deal archivieren?",
  "deals.bulkArchiveConfirmTitle_other": "{count} Deals archivieren?",
  "deals.bulkArchiveConfirmBody":
    "Sie verschwinden aus allen Listen und Auswertungen, und zurückholen lässt sich hier noch keiner.",
  "deals.bulkFailed": "{count} nicht übernommen –",
  "deals.bulkFailedRow": "konnte nicht gespeichert werden",

  "deal.offers": "Angebote",
  "deal.newOffer": "Neues Angebot",
  "deal.offerNeedsCurrency":
    "Bepreisen Sie zuerst diesen Deal — ein Angebot wird in der Währung des Deals erstellt.",
  "deal.offerNumber": "Angebots-Nr.",
  "deal.offerRevision": "Rev.",
  "deal.offersEmpty": "Noch keine Angebote",

  "offer.revision": "Revision {revision}",
  "offer.backToDeal": "Zurück zum Deal",
  "offer.totals": "Summen",
  "offer.net": "Netto",
  "offer.tax": "Steuer",
  "offer.gross": "Brutto",
  "offer.edit": "Kopfdaten bearbeiten",
  "offer.currency": "Währung",
  "offer.buyerOrg": "Käufer-Organisation",
  "offer.buyerOrgConfirm": "Käufer-Organisation: {name}",
  "offer.template": "Vorlage",
  "offer.validUntil": "Gültig bis",
  "offer.introText": "Einleitungstext",
  "offer.termsText": "Bedingungen",
  "offer.lines": "Positionen",
  "offer.addLine": "Position hinzufügen",
  "offer.position": "Pos.",
  "offer.description": "Beschreibung",
  "offer.unit": "Einheit",
  "offer.quantity": "Menge",
  "offer.unitPrice": "Einzelpreis",
  "offer.discountPct": "Rabatt %",
  "offer.taxRate": "Steuer %",
  "offer.lineTotal": "Positionssumme",
  "offer.unpriced": "kein Preis — von der Summe ausgeschlossen",
  "offer.removeLine": "Entfernen",
  "offer.pickProduct": "Produkt wählen",
  "offer.pickProductConfirm": "Produkt: {name}",
  "offer.send": "Senden",
  "offer.sendConfirm": "Dieses Angebot an den Käufer senden?",
  "offer.sendBody":
    "Das Angebot wird schreibgeschützt, bis der Käufer antwortet.",
  "offer.accept": "Annehmen",
  "offer.acceptConfirm": "Dieses Angebot als angenommen markieren?",
  "offer.acceptBody":
    "Betrag und Währung des Deals werden an dieses Angebot angeglichen.",
  "offer.reject": "Ablehnen",
  "offer.rejectConfirm": "Dieses Angebot als abgelehnt markieren?",
  "offer.rejectReason": "Grund (optional)",
  "offer.regenerate": "Revision neu generieren",
  "offer.aiDisclosureTitle": "KI-gestützter Hinweis",
  "offer.diffAdded": "{count} Position(en) hinzugefügt",
  "offer.diffRemoved": "{count} Position(en) entfernt",
  "offer.diffChanged": "{count} Position(en) geändert",
  "offer.renderPdf": "PDF erzeugen",
  "offer.viewPdf": "PDF ansehen",
  "offer.pdfUnavailable":
    "PDF-Erzeugung auf diesem Deployment nicht verfügbar.",

  "decision.viaTool": "über {verb}",
  "decision.approveEdited": "Bearbeitet übernehmen",
  "decision.reject": "Ablehnen",
  "decision.draftSubject": "Betreff",
  "decision.draftBody": "Nachricht",
  "decision.dismiss": "Schließen",
  "decision.versionSkew":
    "Dieser Datensatz hat sich seit dem Vormerken geändert — bitte neu vormerken.",
  "decision.reRead": "Neu einlesen",
  "decision.alreadyDecided":
    "Bereits entschieden — hier gibt es nichts mehr zu tun.",
  "decision.expired": "Abgelaufen",
  "decision.expiresIn": "läuft ab in {countdown}",
  "decision.detail": "Freigabe-Detail",
  "decision.detailTechnical": "Technische Details",
  "decision.detailAsked": "Gefragt am",
  "decision.detailDecided": "Entschieden am",
  "decision.applied": "Erledigt.",
  "decision.undoOnRecord": "Am Datensatz rückgängig machen",
  "decision.status.approved": "Genehmigt",
  "decision.status.rejected": "Abgelehnt",
  "decision.status.expired": "Abgelaufen",

  "home.pipelineWeighted": "{amount} gewichtet",
  "home.pipelineCount_one": "{count} offener Deal",
  "home.pipelineCount_other": "{count} offene Deals",
  "home.pipelinePartial":
    "{count} Deals fehlen in diesen Zahlen – Ihre Berechtigung deckt sie nicht ab.",
  "home.pipelineUnavailable": "Diese Zahl konnte nicht geladen werden.",
  "home.asOf": "Stand {at}",
  "home.generating": "Stelle zusammen…",
  "home.generate": "Briefing jetzt holen",
  "home.noneBody":
    "Dein Morgenbriefing sortiert die Deals, die deine erste Stunde verdienen — Gewinnchance, Umsatz, Timing, Momentum und Nähe, jeder Faktor mit Beleg. Es entsteht über Nacht und liegt morgen früh bereit, sobald offene Deals da sind.",
  "home.honestShort":
    "Nur {count} Deals haben die Schwelle geschafft — die Liste wird nie aufgefüllt.",
  "home.overflow":
    "{shown} von {count} qualifizierten Deals — ehrlich kurz, oben die besten.",
  "home.narrativeNoPass":
    "Heute keine Nacht-Zusammenfassung — Margince hat für diese Lage keinen Durchgang gemacht. Die Reihenfolge unten ist trotzdem die von heute.",
  "home.panel.weekly": "Letzte Woche",
  "home.weekly.weekOf": "Woche ab {day}",
  // Die kommende Woche. Der eingefrorene Rückblick sagt, was war; dies ist der
  // einzige Teil dieser Seite, den noch jemand ändern kann.
  "plan.title": "Nächste Woche planen",
  // Der Kopf der sortierten Liste, auf der Seite, die zuerst geöffnet wird —
  // dieselben Zeilen wie in der Arbeitsliste, in der Reihenfolge des Servers.
  "brief.donext.title": "Als Nächstes",
  // Der Eröffnungssatz des Briefings, aus den Zeilen zusammengesetzt, die die
  // Seite zeigt — nie von einem Modell geschrieben.
  "brief.eyebrow": "Dein Morgen",
  "brief.eyebrow.weekly": "Deine Woche",
  "brief.eyebrow.asOf": "{scope} · Stand {at}",
  // Die zwei Regler des Briefings: welches, und für wen.
  "brief.view.label": "Welches Briefing",
  "brief.view.morning": "Morgen",
  "brief.view.weekly": "Woche",
  "brief.scope.label": "Wessen Briefing",
  "brief.scope.mine": "Meins",
  "brief.scope.team": "Team",
  "brief.sentence.clear": "Heute Morgen wartet nichts auf dich.",
  "brief.sentence.one": "Zuerst: {lead}",
  "brief.sentence.oneWithCost": "Zuerst: {lead} — {consequence}",
  "brief.sentence.many": "Zuerst: {lead} Danach {rest} weitere.",
  "brief.sentence.manyWithCost":
    "Zuerst: {lead} — {consequence} Danach {rest} weitere.",

  // Der Einstiegssatz des Wochen-Briefs, aus den eingefrorenen Zahlen gebaut.
  "brief.week.won": "Sie haben {count} Abschlüsse gemacht.",
  "brief.week.moved": "Sie haben {count} Deals vorangebracht.",
  "brief.week.met": "Sie hatten {count} Termine.",
  "brief.week.carryPromises": "{count} Zusagen sind offen geblieben.",
  "brief.week.carryTasks": "{count} Aufgaben sind offen geblieben.",
  "brief.week.andCarry": "{result} {carry}",
  "brief.week.quiet":
    "Eine ruhige Woche — nichts abgeschlossen, nichts bewegt.",

  "brief.donext.sub": "Eine Reihenfolge, aus deiner Arbeitsliste.",
  "brief.donext.loading": "Was auf dich wartet, wird gelesen",
  "brief.donext.clear": "Gerade wartet nichts auf dich.",
  "brief.donext.rest": "{count} weitere in der Arbeitsliste",

  // Die Woche eines Teams, eingefroren beim Abschluss. Zwei Wochen sind
  // vergleichbar, weil keine sich unter dem Vergleich bewegt.
  "teamweekly.title": "Die Woche des Teams",
  "teamweekly.weekOf": "{team} · Woche ab {day}",
  "teamweekly.frozen": "Eingefroren",
  "teamweekly.loading": "Teamwoche wird gelesen",
  "teamweekly.empty": "Für diese Woche gibt es nichts zu zeigen.",
  "teamweekly.forbidden":
    "Die Woche eines Teams ist eine Teamfrage, und dein Zugriff reicht nur bis zu deinen eigenen Datensätzen.",
  "teamweekly.noSnapshot":
    "Für dieses Team wurde noch keine Woche abgeschlossen. Die erste Momentaufnahme entsteht am Montag nach der ersten vollen Woche.",
  "teamweekly.pickTeam": "Team auswählen",
  "teamweekly.repsUnread":
    "{count} Mitglied(er) konnten nicht gelesen werden. Alle Zahlen hier decken {counted} ab.",
  "teamweekly.ofTotal": "{part} von {whole}",
  "teamweekly.headline.plain":
    "Die Woche lief ohne einen Wert, der in eine Richtung heraussticht.",
  "teamweekly.headline.healthy":
    "{reading} ist gesund bei {pct}%, gemessen an einer Schwelle von {bar}%.",
  "teamweekly.headline.weak":
    "{reading} ist es nicht, bei {pct}% gegen eine Schwelle von {bar}%.",
  "teamweekly.reading.firstResponse": "Erstreaktion",
  "teamweekly.reading.nextStep": "Termine mit nächstem Schritt",
  "teamweekly.reading.commitments": "Planzusagen eingehalten",
  "teamweekly.card.firstResponse": "Rechtzeitig beantwortet",
  "teamweekly.card.firstResponseBasis": "{breached} überschritten",
  "teamweekly.card.meetings": "Termine mit nächstem Schritt",
  "teamweekly.card.meetingsBasis": "der gehaltenen Termine",
  "teamweekly.card.commitments": "Planzusagen eingehalten",
  "teamweekly.card.commitmentsBasis": "des Zugesagten",
  "teamweekly.card.won": "Gewonnen",
  "teamweekly.card.wonBasis": "{lost} verloren",
  "teamweekly.card.wonBasisValue": "{value} gewonnen · {lost} verloren",
  "teamweekly.card.reps": "Gezählte Mitglieder",
  "teamweekly.card.repsBasis": "deren Woche vollständig gelesen wurde",
  "teamweekly.movement.title": "Was die Woche bewegt hat",
  "teamweekly.movement.won": "Gewonnen",
  "teamweekly.movement.lost": "Verloren",
  "teamweekly.movement.meetings": "Gehaltene Termine",
  "teamweekly.movement.leads": "Zugewiesene Leads",
  "teamweekly.coach.title": "Diese Woche begleiten",
  "teamweekly.coach.sub":
    "Ein Schwerpunkt pro Mitglied — auch für das Mitglied, dessen Woche gut lief.",
  "teamweekly.coach.empty": "In dieser Woche war niemand in diesem Team.",
  "teamweekly.focus.help_requested": "Hat um Hilfe gebeten",
  "teamweekly.focus.leads_breached": "Leads blieben unbeantwortet",
  "teamweekly.focus.commitments_missed": "Planzusagen verpasst",
  "teamweekly.focus.meetings_without_next_step":
    "Termine ohne nächsten Schritt",
  "teamweekly.focus.strong_week": "Zum Nachmachen",
  "teamweekly.focus.quiet_week": "Eine ruhige Woche",

  "plan.sub": "Was du dir vorgenommen hast — und was du dafür brauchst.",
  "plan.loading": "Plan wird gelesen",
  "plan.empty": "Noch nichts auf dem Plan.",
  "plan.none": "Du hast diese Woche noch nicht geplant.",
  "plan.start": "Meine Woche planen",
  "plan.add": "Vorhaben hinzufügen",
  "plan.saveRefused_one":
    "Eine Zusage konnte nicht gespeichert werden. Sie ist weiterhin angehakt — bitte erneut versuchen.",
  "plan.saveRefused_other":
    "{count} Zusagen konnten nicht gespeichert werden. Sie sind weiterhin angehakt — bitte erneut versuchen.",
  "plan.save_one": "{count} Änderung speichern",
  "plan.save_other": "{count} Änderungen speichern",
  "plan.due": "bis {day}",
  "plan.state.open": "Offen",
  "plan.state.done": "Erledigt",
  "plan.state.missed": "Verpasst",
  "plan.state.dropped": "Verworfen",
  "plan.help.label": "Was brauchst du von deiner Führungskraft?",
  "plan.help.ask": "Um Hilfe bitten",
  "plan.help.edit": "Anfrage bearbeiten",
  "plan.help.send": "Senden",
  "plan.help.cancel": "Abbrechen",
  "plan.help.asked": "Du hast gefragt: {text}",
  "plan.help.waiting": "Wartet auf deine Führungskraft.",
  "plan.new.label": "Was wirst du tun?",
  "plan.new.due": "Bis wann",
  "plan.new.save": "Hinzufügen",
  "plan.new.cancel": "Abbrechen",

  "home.weekly.frozen": "Eingefroren",
  "home.weekly.written": "geschrieben {at}",
  "home.weekly.pickWeek": "Andere Woche öffnen",
  "home.weekly.none":
    "Noch kein Wochenrückblick — der erste wird am Montag nach deiner ersten vollen Woche geschrieben.",
  "home.weekly.tasksDelivered": "Aufgaben erledigt",
  "home.weekly.ofDue": "{done} von {due}",
  "home.weekly.dealsWon": "Gewonnen",
  "home.weekly.dealsLost": "Verloren",
  "home.weekly.dealsMoved": "Bewegt",
  "home.weekly.decided": "Von dir entschieden",
  "home.weekly.acceptedRejected": "{accepted} ja · {rejected} nein",
  "home.weekly.noNarrative":
    "Keine Zusammenfassung dieser Woche — Margince hat keinen Durchgang gemacht. Die Zahlen unten sind trotzdem die der Woche.",
  "home.weekly.queueWorked": "Morgen-Liste",
  "home.weekly.actedDismissed": "{acted} bearbeitet · {dismissed} weggeklickt",
  "home.weekly.sincePrior": "{delta} ggü. Vorwoche",
  "home.weekly.leadsAnswered": "Leads rechtzeitig beantwortet",
  "home.weekly.ofRouted": "{answered} von {routed}",
  "home.weekly.planCommitmentsKept": "Planzusagen eingehalten",
  "home.weekly.meetingsHeld": "Meetings mit nächstem Schritt",
  "home.weekly.ofMeetings": "{withStep} von {held}",
  "home.weekly.carriedOver": "Übernommen",
  "home.weekly.outcome.moved": "bewegt",
  "home.weekly.outcome.won": "gewonnen",
  "home.weekly.outcome.lost": "verloren",
  "home.focus.allAbove":
    "Alles aus der Nacht steht schon oben, bei dem, was auf dich wartet.",
  "home.quietRun":
    "Heute Morgen hat nichts die Schwelle geschafft. Keine erfundene Dringlichkeit — genieß die Ruhe.",
  "home.act": "Erledigt",
  "home.dismiss": "Ausblenden",
  "home.actedState": "erledigt",
  "home.dismissedState": "ausgeblendet",
  "home.evidence_other": "{count} Belege",
  "home.evidence_one": "{count} Beleg",
  "home.openDeal": "Deal öffnen",
  "home.factorWinnability": "Gewinnchance",
  "home.factorRevenue": "Umsatz",
  "home.factorTiming": "Timing",
  "home.factorMomentum": "Momentum",
  "home.factorWarmth": "Nähe",

  "home.digestFor": "Digest vom {date}",
  "home.digestSynced": "E-Mails synchronisiert",
  "home.digestPeople": "Personen angelegt",
  "home.digestOrgs": "Firmen angelegt",
  "home.digestDedupe": "Dubletten zu prüfen",
  "home.digestClassify":
    "Über Nacht einsortiert: {commitments} Zusagen · {meetings} Termine · {noise} Rauschen",
  "home.digestProjects": "Projekte",
  "home.digestPhaseChanges": "Phasenwechsel",
  "home.digestNewCommitments": "Neue Zusagen",
  "home.digestGoneQuiet": "Still geworden",
  "home.digestPhaseChange": "{from} → {to}",
  "home.digestCommitmentCount": "{count} neue offene Zusagen",
  "home.digestQuietDays": "seit {days} Tagen still",
  "home.glance.morning": "Guten Morgen, {name}.",
  "home.glance.morningAnon": "Guten Morgen.",
  "home.glance.afternoon": "Guten Tag, {name}.",
  "home.glance.afternoonAnon": "Guten Tag.",
  "home.glance.evening": "Guten Abend, {name}.",
  "home.glance.eveningAnon": "Guten Abend.",
  "home.glance.night": "Noch im Einsatz, {name}.",
  "home.glance.nightAnon": "Noch im Einsatz.",
  "home.glance.introWeekly": "Das ist deine abgeschlossene Woche.",
  "home.glance.intro": "Das ist dein Tag.",
  "home.panel.decisions": "Wartet auf dich",
  "home.panel.focus": "Wenn Zeit bleibt",
  "home.panel.overnight": "Über Nacht",
  "home.panel.position": "Bestand",
  "home.panel.schedule": "Heutiger Kalender",
  "home.schedule.clear": "Heute steht nichts an.",
  "home.panel.promises": "Zusagen & Aufgaben",
  "home.promises.clear": "Nichts ist offen.",
  "home.promises.untracked":
    "Zusagen aus Gesprächen werden noch nicht erfasst — hier stehen nur Aufgaben.",
  "home.panel.watch": "Still geworden",
  "home.overnight.fixConnector": "Verbindung prüfen",
  "home.watch.clear": "Nichts ist still geworden.",
  "home.readings.label": "Dein Morgen in fünf Kennzahlen",
  "home.readings.truncated":
    "Eine Quelle wurde bis zur Grenze gelesen, jede Zahl oben ist also ein Mindestwert.",
  "home.readings.waiting": "Kunden warten",
  "home.readings.waitingBasis": "warten auf eine Antwort",
  "home.readings.meetings": "Termine heute",
  "home.readings.meetingsBasis": "im heutigen Kalender",
  "home.readings.needsPrep_one": "1 unvorbereitet",
  "home.readings.needsPrep_other": "{count} unvorbereitet",
  "home.readings.prepUnknown": "nicht alle prüfbar",
  "home.readings.prepared": "alle vorbereitet",
  "home.readings.promises": "Zusagen fällig",
  "home.readings.promisesBasis": "Zusagen werden noch nicht erfasst",
  "home.readings.leads": "Erstkontakt",
  "home.readings.leadsBasis": "warten auf die erste Antwort",
  "home.readings.leadsDue": "nächste fällig {value}",
  "home.readings.quota": "Ziel-Tempo",
  "home.readings.quotaBasis": "kein Ziel hinterlegt",
  "home.rail": "Kontext",
  "home.pct": "{pct} %",
  "home.deck.later": "Später",
  "home.deck.showMore": "Ganze Nachricht anzeigen",
  "home.deck.showLess": "Weniger anzeigen",
  "home.deck.view": "Wie die Warteschlange gezeigt wird",
  "home.deck.viewDeck": "Stapel",
  "home.deck.viewList": "Liste",
  "home.deck.keys":
    "→ annehmen · ← ablehnen · ↑ bearbeiten · ↓ später · U zurücknehmen · Enter senden",
  "home.deck.behind_one": "1 weitere Karte dahinter",
  "home.deck.behind_other": "{count} weitere Karten dahinter",
  "home.deck.staged_one": "1 Entscheidung vorbereitet",
  "home.deck.staged_other": "{count} Entscheidungen vorbereitet",
  "home.deck.commit": "Vorbereitete Entscheidungen senden",
  "home.deck.unstage": "Letzte zurücknehmen",
  "home.deck.clearedTitle": "Stapel leer",
  "home.deck.cleared_one": "1 Entscheidung gesendet",
  "home.deck.cleared_other": "{count} Entscheidungen gesendet",
  "home.deck.clearedTime": "um {at}",
  "home.deck.empty": "Es wartet nichts auf dich.",
  "home.deck.bundleSummary": "Eine Entscheidung · {count} Vorgänge",
  "home.deck.bundleMembers": "Die {count} Vorgänge anzeigen",
  "home.brief.rank": "Rang",
  "home.brief.composite": "Bewertung",
  "home.brief.previouslyDismissed":
    "Am {day} markiert — du hast es weggeklickt.",
  "home.brief.returnedWith": "Zurück durch Aktivität am",
  "home.brief.revenueBasis": "Umsatz gemessen an {amount}",
  "home.brief.resurfaces": "Zurück",
  "home.evidenceNone": "keine Belege erfasst",
  "home.snooze": "Zurückstellen",
  "home.snoozedState": "zurückgestellt",

  "enrich.toInbox": "Arbeitsliste öffnen",

  "deepread.title": "Margince kann das ausfüllen",
  "deepread.sub":
    "Margince liest die Website des Unternehmens nach Domain, Branche, Größe, Standorten und wahrscheinlichen Entscheidern und schlägt dann einen ersten Schritt vor. Die Funde werden zur Prüfung vorgemerkt — nichts wird geschrieben, bevor Sie zustimmen.",
  "deepread.cta": "Unternehmensrecherche starten",
  "deepread.starting": "Startet…",
  "deepread.unavailable":
    "Website-Lesen ist auf diesem Server nicht eingerichtet.",
  "deepread.statusQueued": "In Warteschlange",
  "deepread.statusDeferred": "Wartet auf KI-Budget",
  "deepread.statusRunning": "Liest…",
  "deepread.statusDone": "Fertig",
  "deepread.statusPartial": "Früh beendet",
  "deepread.statusFailed": "Fehlgeschlagen",
  "deepread.statusCancelled": "Abgebrochen",
  "deepread.resumesAt": "Wird automatisch am {when} fortgesetzt.",
  "deepread.pagesSoFar_one": "{count} Seite gelesen",
  "deepread.pagesSoFar_other": "{count} Seiten gelesen",
  "deepread.stoppedEarly": "Früh beendet: {reason}",
  "deepread.stage.crawling": "Website wird gelesen",
  "deepread.stage.extracting": "Inhalte werden ausgewertet",
  "deepread.step.done": "fertig",
  "deepread.step.running": "läuft",
  "deepread.step.queued": "wartet",
  "deepread.stopBudget": "Modellbudget",
  "deepread.stopPageCap": "Seitenlimit",
  "deepread.stopByteCap": "Größenlimit",
  "deepread.stopDeadline": "Zeitlimit",
  "deepread.factCount_one": "{count} belegter Fakt vorgemerkt",
  "deepread.factCount_other": "{count} belegte Fakten vorgemerkt",
  "deepread.proposals_other": "{count} Vorschläge warten auf deine Prüfung",
  "deepread.proposals_one": "{count} Vorschlag wartet auf deine Prüfung",
  "deepread.kindHome": "Startseite",
  "deepread.kindImpressum": "Impressum",
  "deepread.kindAbout": "Über uns",
  "deepread.kindTeam": "Team",
  "deepread.kindServices": "Leistungen",
  "deepread.kindProducts": "Produkte",
  "deepread.kindContact": "Kontakt",
  "deepread.kindOther": "Sonstiges",

  "transcriptread.title": "Dieses Transkript lesen",
  "transcriptread.sub":
    "Findet die nächsten Schritte und Zusagen, die dieses Gespräch nennt. Nichts wird geschrieben, bis du bestätigst.",
  "transcriptread.cta": "Transkript lesen",
  "transcriptread.starting": "Wird gestartet…",
  "transcriptread.unavailable":
    "Transkript-Lesen ist auf diesem Server nicht eingerichtet.",
  "transcriptread.statusQueued": "In Warteschlange",
  "transcriptread.statusRunning": "Wird gelesen…",
  "transcriptread.statusDone": "Fertig",
  "transcriptread.statusFailed": "Fehlgeschlagen",
  "transcriptread.lineCount_one": "{count} Zeile gelesen",
  "transcriptread.lineCount_other": "{count} Zeilen gelesen",
  "transcriptread.proposals_other":
    "{count} nächste Schritte warten auf deine Prüfung",
  "transcriptread.proposals_one":
    "{count} nächster Schritt wartet auf deine Prüfung",
  "transcriptread.nothingStated":
    "Vollständig gelesen. Dieses Gespräch nennt keine nächsten Schritte.",
  "transcriptread.failedFallback":
    "Dieses Transkript konnte nicht gelesen werden. Es wurde nichts vorgemerkt.",

  "create.cancel": "Abbrechen",
  "create.multiselect.required":
    "Erforderlich – mindestens eine Option wählen.",
  "create.save": "Anlegen",
  "create.saving": "Wird angelegt…",
  "create.contact": "Neuer Kontakt",
  "vcardImport.action": "Karten importieren",
  "vcardImport.title": "Visitenkarten importieren",
  "vcardImport.fileLabel": "Visitenkarten-Datei",
  "vcardImport.whichFile":
    "Eine .vcf-Datei — das Format, in dem jedes Telefon und jedes Mailprogramm Kontakte exportiert. Eine Karte, die dir jemand gegeben hat, gibt er dir bewusst, deshalb wird sie direkt geschrieben und nicht zur Freigabe gestellt.",
  "vcardImport.choose": ".vcf-Datei auswählen",
  "vcardImport.working": "Karten werden gelesen…",
  "vcardImport.done": "Schließen",
  "vcardImport.noCards": "In der Datei war keine Karte.",
  "vcardImport.outcome.created": "Angelegt",
  "vcardImport.outcome.updated": "Lücken gefüllt",
  "vcardImport.outcome.needsReview": "Sieht aus wie jemand, den ihr schon habt",
  "vcardImport.outcome.skipped": "Übersprungen",
  "create.quickCapture": "Schnellerfassung",
  "create.quickCaptureSaved": "{name} gespeichert",
  "create.company": "Neue Firma",
  "create.lead": "Neuer Lead",
  "create.deal": "Neuer Deal",
  "create.fullName": "Vollständiger Name",
  "create.firstName": "Vorname",
  "create.lastName": "Nachname",
  "create.personTitle": "Titel",
  "create.email": "E-Mail",
  "create.phone": "Telefon",
  "create.linkedin": "LinkedIn",
  "create.linkedinUrl": "LinkedIn-URL",
  "create.displayName": "Firmenname",
  "create.legalName": "Rechtlicher Name",
  "create.industry": "Branche",
  "create.sizeBand": "Unternehmensgröße",
  "co.address.summary": "Adresse",
  "co.address.add": "Adresse hinzufügen",
  "create.addressLine1": "Straße und Hausnummer",
  "create.addressLine2": "Adresszusatz",
  "create.city": "Stadt",
  "create.region": "Bundesland / Region",
  "create.postalCode": "PLZ",
  "create.country": "Land (ISO-3166, z. B. DE)",
  "create.companyName": "Firma",
  "create.dealName": "Deal-Name",
  "create.amount": "Wert",
  "create.currency": "Währung",
  "create.stage": "Phase",
  "create.organization": "Firma",
  "create.expectedClose": "Erwarteter Abschluss",

  "field.unset": "Nicht gesetzt",
  "field.addEmail": "E-Mail hinzufügen",
  "field.addPhone": "Telefon hinzufügen",
  "field.addDomain": "Domain hinzufügen",
  "field.addLegalName": "Rechtlichen Namen hinzufügen",
  "field.addIndustry": "Branche hinzufügen",
  "field.addLinkedinUrl": "LinkedIn-URL hinzufügen",
  "field.addRegisterVat": "USt-IdNr. hinzufügen",
  "field.addRegisteredAddress": "Registeranschrift hinzufügen",
  "field.addFullName": "Namen hinzufügen",
  "field.addTitle": "Titel hinzufügen",
  "field.addAddressLine1": "Straße und Hausnummer hinzufügen",
  "field.addAddressLine2": "Adresszusatz hinzufügen",
  "field.addPostalCode": "Postleitzahl hinzufügen",
  "field.addCity": "Stadt hinzufügen",
  "field.addRegion": "Bundesland / Region hinzufügen",
  "field.addCountry": "Ländercode hinzufügen, z. B. DE",
  "field.country": "Land",
  "field.domain": "Domain",
  "field.domainRequired":
    "Eine Domain kann hier nicht gelöscht werden — dafür den vollständigen Editor verwenden.",
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
  "field.yes": "Ja",
  "field.no": "Nein",

  "dedupe.viewExisting": "Vorhandenen Eintrag anzeigen",

  "co.spine.earlierMore": "Weitere Gespräche davor",
  "co.spine.failed": "Der Verlauf konnte nicht gelesen werden.",
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
  "co.spine.said.met": "{host} traf {who}",
  "co.spine.said.held": "Termin von {host}",
  "co.spine.lastSpoke": "Zuletzt gesprochen",
  "co.spine.days_one": "{count} Tag",
  "co.spine.days_other": "{count} Tage",
  "co.spine.quietSince": "Seitdem Funkstille",
  "co.spine.neverReplied": "Sie haben nie geantwortet",
  "co.spine.singleThreaded": "Ein Kontakt, und keine Antwort von ihm",
  "co.spine.overdue": "\u00dcberf\u00e4llig",
  "co.spine.expectedClose": "Erwarteter Abschluss",
  "co.360.title": "Margince hat diesen Datensatz gelesen",
  "co.360.subject": "{name} · 360",
  "co.360.subjectUnnamed": "Dieser Account · 360",
  "today.title": "Was heute eine Person braucht",
  "co.spine.earlier_other": "{count} frühere Gespräche",
  "co.spine.earlier_one": "{count} früheres Gespräch",
  "today.failed":
    "Das ließ sich nicht zusammenstellen. Der Rest der Seite zeigt weiterhin, was gelesen werden konnte.",
  "today.quiet": "Hier braucht Sie heute nichts.",
  "task.untitled": "Aufgabe ohne Titel",
  "today.withheld":
    "Für Sie ausgeblendet: {sections}. Diese Liste ist ohne sie zusammengestellt.",
  "today.source.moments": "was Margince gefunden hat",
  "today.source.nextSteps": "offene Aufgaben",
  "today.source.nextMeeting": "der Kalender",
  "today.source.deals": "Deals",
  "today.meeting.prepare": "Meeting vorbereiten",
  "today.source.people": "die Kontakte",
  "today.source.standing": "wer am Zug ist und die Signale",
  "today.source.activities": "was gesprochen wurde",
  "today.silence.days": "seit {count} Tagen keine Antwort",
  "today.draft.to": "Follow-up an {name} entwerfen",
  "today.draft.act": "Entwerfen",

  "evidence.mark": "gelesen",
  "evidence.confirm": "Bestätigen",
  "evidence.correct": "Korrigieren",
  "evidence.save": "Speichern",
  "evidence.saving": "Wird gespeichert…",
  "evidence.cancel": "Abbrechen",
  "evidence.correctedValue": "Korrigierter Wert",
  "evidence.confirmedAt": "Von einer Person bestätigt {when}",
  "evidence.humanSet": "Von einer Person gesetzt",
  "acctCoverage.open": "Abdeckung vergleichen",
  "acctCoverage.title": "Wer diesen Account abdeckt",
  "acctCoverage.contact": "Kontakt",
  "acctCoverage.findContact": "Kontakt suchen",
  "acctCoverage.untried": "Nicht versucht",
  "acctCoverage.noMatch": "Kein Kontakt passt dazu.",
  "acctCoverage.columnCap":
    "{cap} Kolleginnen und Kollegen angezeigt — einen abwählen, um einen weiteren hinzuzufügen.",
  "acctCoverage.partial":
    "Dieses Raster stammt aus einem unvollständigen Lesevorgang. Eine leere Zelle kann also bedeuten, dass der Lesevorgang abgebrochen wurde — nicht, dass niemand es versucht hat.",
  "acctCoverage.noneButPartial":
    "Es wurde niemand mit Kontakt zurückgegeben, der Lesevorgang war aber begrenzt — das ist keine Aussage darüber, dass niemand diesen Account abdeckt.",
  "acctCoverage.noneAtAll":
    "Bisher hat hier niemand Nachrichten mit jemandem in dieser Firma ausgetauscht.",
  "docs.title": "Dokumente",
  "docs.empty": "Noch keine Dokumente zu diesem Account.",
  "docs.noneInCategory": "Keine Dokumente dieser Art zu diesem Account.",
  "docs.allOnAgreements":
    "Alle Dokumente hier sind einem Vertrag oben zugeordnet.",
  "docs.allSuperseded":
    "Hier stehen nur noch ersetzte Dokumente. Einblenden, um die Historie zu lesen.",
  "docs.superseded.show": "Ersetzte einblenden",
  "docs.superseded.hide": "Ersetzte ausblenden",
  "docs.superseded.hidden_one": "1 ersetztes Dokument ist ausgeblendet.",
  "docs.superseded.hidden_other":
    "{count} ersetzte Dokumente sind ausgeblendet.",
  "docs.superseded.shown_one": "1 ersetztes Dokument steht unten in der Liste.",
  "docs.superseded.shown_other":
    "{count} ersetzte Dokumente stehen unten in der Liste.",
  "docs.reading.show": "Dokument auslesen",
  "docs.reading.hide": "Auslesung ausblenden",

  // Ein Dokument hinzufügen. Die Frage „Wozu gehört es?" trägt die eigentliche
  // Entscheidung: nur ein Dokument an einem Deal kann für Deal-Felder gelesen
  // werden. Der Hinweis sagt das, statt es die Lesenden an einem Panel
  // herausfinden zu lassen, das nie erscheint.
  "docs.add.action": "Dokument hinzufügen",
  "docs.add.title": "Dokument hinzufügen",
  "docs.add.about": "Gehört zu",
  "docs.add.aboutHint":
    "Ein Dokument an einem Deal kann für Deal-Felder gelesen werden, eines an der Firma nicht.",
  "docs.add.thisCompany": "Diese Firma",
  "docs.add.aDeal": "Ein Deal",
  "docs.add.dealSearch": "Deals dieses Accounts durchsuchen",
  "docs.add.dealSearchReach":
    "Die Suche erfasst die {deals} neuesten Deals dieses Accounts und zeigt die ersten {matches} Treffer. Ein älterer Deal lässt sich hier nicht auswählen.",
  "docs.add.category": "Kategorie",
  "docs.add.name": "Titel",
  "docs.add.nameHint":
    "Optional. Leer gelassen, zeigt die Zeile den Dateinamen.",
  "docs.add.file": "Datei",
  "docs.add.fileHint": "Bis zu {size}.",
  "docs.add.fileEmpty": "Datei hierher ziehen oder zum Auswählen klicken",
  "docs.add.cancel": "Abbrechen",
  "docs.add.submit": "Hochladen",
  "docs.add.uploading": "Wird hochgeladen…",
  "docs.add.errNoFile": "Wählen Sie eine Datei zum Hochladen.",
  "docs.add.errNoDeal":
    "Wählen Sie den Deal, dem das Dokument zugeordnet werden soll.",
  "docs.add.errRefused":
    "Sie dürfen zu diesem Datensatz keine Dokumente hinzufügen.",
  "docs.add.errTooLarge":
    "Diese Datei ist größer als {size} — mehr nimmt diese Installation nicht an. Bitte eine kleinere wählen.",
  "docs.add.failedTitle": "Der Upload ist fehlgeschlagen",
  "docs.add.failed":
    "Es wurde nichts gespeichert. Versuchen Sie es erneut oder wählen Sie eine andere Datei.",
  "docs.add.partialTitle": "Hochgeladen, aber nicht eingeordnet",
  "docs.add.partial":
    "Die Datei liegt am Datensatz und steht unten in der Liste. Nur Kategorie und Titel wurden nicht gespeichert, sie ist daher unter Sonstiges abgelegt.",

  // Das Panel für die abgelegte Dokumentenlesung (RD-AC-N-2/-3). Drei Zustände,
  // die auch in den Worten getrennt bleiben müssen: noch keine Antwort, gelesen
  // und keines der Felder genannt, gar nicht lesbar.
  "extraction.neverRead":
    "Diese Datei wurde noch nicht auf Deal-Felder gelesen.",
  "extraction.readIt": "Diese Datei lesen",
  "extraction.readAgain": "Erneut versuchen zu lesen",
  "extraction.starting": "Wird gestartet…",
  "extraction.startFailed":
    "Diese Datei konnte nicht zum Lesen übergeben werden. Es wurde nichts geändert.",
  "extraction.loading": "Prüfe, ob diese Datei bereits gelesen wurde…",
  "extraction.reading": "Diese Datei wird gelesen…",
  "extraction.failed": "Diese Datei konnte nicht gelesen werden.",
  "extraction.groundedNothing":
    "Die KI hat diese Datei gelesen — sie nennt keines der Deal-Felder.",
  "extraction.heading_one":
    "Die KI hat diese Datei gelesen — {count} belegbares Feld, für Ihren Datensatz vorbereitet (zum Übernehmen bestätigen)",
  "extraction.heading_other":
    "Die KI hat diese Datei gelesen — {count} belegbare Felder, für Ihren Datensatz vorbereitet (zum Übernehmen bestätigen)",
  "extraction.accept_one": "{count} Feld übernehmen",
  "extraction.accept_other": "{count} Felder übernehmen",
  "extraction.dismiss": "Verwerfen",
  "extraction.dismissed":
    "Es wurde nichts geschrieben. Die Datei bleibt angehängt.",
  "extraction.acceptedLabel": "Übernommene Felder",
  "extraction.acceptedHeading_one":
    "{count} Feld in den Deal übernommen — die Originalauszüge bleiben erhalten",
  "extraction.acceptedHeading_other":
    "{count} Felder in den Deal übernommen — die Originalauszüge bleiben erhalten",
  "extraction.acceptFailed":
    "Diese Felder wurden nicht geschrieben. Am Deal hat sich nichts geändert.",
  "extraction.edit": "Bearbeiten",
  "extraction.editValue": "{field} bearbeiten",
  "extraction.omitted.notStated": "ausgelassen (in dieser Datei nicht genannt)",
  "extraction.omitted.notConfident":
    "ausgelassen (die Datei sagt etwas dazu, aber nicht klar genug zum Übernehmen)",
  "extraction.field.name": "Deal-Name",
  "extraction.field.amount": "Betrag",
  "extraction.field.currency": "Währung",
  "extraction.field.closeDate": "Erwartetes Abschlussdatum",
  "docs.filterLabel": "Dokumente nach Art",
  "docs.category.all": "Alle",
  "docs.category.contract": "Vertrag",
  "docs.category.offer": "Angebot",
  "docs.category.legal": "Recht",
  "docs.category.email": "E-Mail-Anhang",
  "docs.category.message": "Nachrichtenanhang",
  "docs.category.other": "Sonstiges",
  "files.title": "Dateien",
  "files.sub":
    "Was Sie an diesem Deal hochgeladen haben und was mit seinen E-Mails und Nachrichten eingegangen ist.",
  "files.empty":
    "Noch keine Dateien an diesem Deal. Laden Sie eine hoch oder verknüpfen Sie eine E-Mail mit Anhang.",
  "files.origin": "Anhang einer Nachricht von {who}, {when}",
  "files.originUnknown": "unbekanntem Absender",
  "files.uploaded": "Hochgeladen {when}",
  "files.hiddenBadge": "Ausgeblendet",
  "files.rowActions": "Aktionen für {name}",
  "files.hide": "Aus diesem Deal ausblenden",
  "files.unhide": "Wieder an diesem Deal zeigen",
  "files.delete": "Löschen",
  "files.hideTitle": "{name} aus diesem Deal ausblenden?",
  "files.hideBody":
    "Die Nachricht und ihr Anhang bleiben an der Aktivität und in der Bibliothek des Unternehmens. Nur dieser Deal führt sie nicht mehr auf.",
  "files.deleteTitle": "{name} löschen?",
  "files.deleteBody":
    "Die Datei wird aus diesem Deal entfernt – und aus jedem Deal Room, der sie teilt.",
  "files.showHidden": "Ausgeblendete Dateien zeigen",
  "files.hideHidden": "Ausgeblendete verbergen",
  "docs.state.draft": "Entwurf",
  "docs.state.current": "Aktuell",
  "docs.state.final": "Final",
  "docs.state.superseded": "Ersetzt",
  "log.title": "Aktivität erfassen",
  "log.addTask": "Aufgabe anlegen",
  "log.sub": "eine Notiz oder Aufgabe, direkt auf diese Timeline",
  "log.kind": "Art",
  "log.kindNote": "Notiz",
  "log.kindTask": "Aufgabe",
  "log.kindMeeting": "Meeting",
  "log.transcriptLabel": "Transkript",
  "log.transcriptHint":
    "Aus Ihrem Meeting-Tool einfügen (Teams, Zoom, Meet …) — Sprecherkennzeichnungen bleiben, sofern vorhanden, erhalten.",
  "log.asTranscript": "Dieser Text ist ein Transkript",
  "log.transcriptUpload": "Oder eine Datei hochladen",
  "log.transcriptUploadRejected": "Nur eine .txt-Datei wird akzeptiert.",
  "log.transcriptUploadFailed":
    "Die Datei konnte nicht gelesen werden — versuchen Sie stattdessen, den Text einzufügen.",
  "log.subject": "Betreff",
  "log.body": "Details",
  "log.dueAt": "Fällig am",
  "log.date": "Datum",
  "log.save": "Erfassen",
  "log.saving": "Wird erfasst…",

  "compose.reply": "Antworten",
  "compose.relink": "Neu verknüpfen",
  "compose.draftWithAi": "Mit KI entwerfen",
  "compose.drafting": "Wird entworfen…",
  "compose.discardDraft": "Entwurf verwerfen",
  "compose.discardDraftHint":
    "Meldet Ihrer Voice DNA, dass dieser Entwurf danebenlag. Der erzeugte Text wird nie gespeichert.",
  "compose.aiDisclosureTitle": "KI-gestützter Entwurf",
  "compose.aiDisclosureFallback":
    "Dieser Entwurf stammt von einer KI. Lesen und überarbeiten Sie ihn, bevor Sie senden.",
  "compose.voiceVersion": "Aus Ihrem Korpus gebaut · v{n}",
  "compose.provisional": "Vorläufige Stimme",
  "compose.provisionalHint":
    "Ihre Voice DNA wird noch aufgebaut. Sie prägt diesen Entwurf schon genauso wie eine fertige — es wird nichts zurückgehalten.",
  "compose.intent": 'Entwurf steuern (optional), z. B. "höfliche Nachfrage"',
  "compose.to": "An",
  "compose.answering": "Antwort auf „{subject}“ · {when}",
  "compose.answeringTo": "Antwort an {who} · „{subject}“ · {when}",
  "compose.answeringNoSubject": "Antwort auf die Nachricht vom {when}",
  "compose.answeringNothing":
    "Hier gibt es keine frühere Nachricht — das beginnt einen neuen Verlauf.",
  "compose.cc": "Cc",
  "compose.subject": "Betreff",
  "compose.noGroundableRecipient":
    "Noch kein Kontakt zu diesem Account — schreibe die Nachricht selbst oder lege zuerst einen Kontakt an",
  "compose.draftTo": "Entwurf an",
  "compose.draftToUnset": "Kontakt wählen",
  "compose.relatedTo": "Bezug",
  "compose.relatedToNone": "Der Account allgemein",
  "compose.project": "Projekt",
  "compose.projectNone": "Kein Projekt",
  "compose.scopedToCounted":
    "Bezogen auf {key} · {inScope} von {total} Aktivitäten",
  "compose.scopedTo": "Bezogen auf {key}",
  "compose.channelFiling":
    "Wird zusammen mit der beantworteten Konversation unter {project} abgelegt.",
  "compose.basedOn": "Grundlage: {inputs}",
  "compose.whyThisDraft": "Warum dieser Entwurf?",
  "compose.body": "Nachricht",
  "compose.bodyHint": "In den Text klicken, um ihn zu bearbeiten.",
  "calendar.previousMonth": "Voriger Monat",
  "calendar.nextMonth": "Nächster Monat",
  "compose.schedulePick": "Datum und Uhrzeit wählen",
  "compose.scheduleDate": "Datum",
  "compose.scheduleTime": "Uhrzeit",
  "compose.scheduleGoesOut": "Geht raus {when}",
  "compose.willGoOut": "Geht raus {when}",
  "compose.scheduleAfternoon": "Morgen Nachmittag",
  "compose.rewrite": "Umschreiben",
  "compose.rewriteShorter": "Kürzer",
  "compose.rewriteShorterAsk": "Dasselbe mit weniger Worten sagen.",
  "compose.rewriteWarmer": "Wärmer",
  "compose.rewriteWarmerAsk": "Wärmer im Ton, ohne vertraulich zu werden.",
  "compose.rewriteFormal": "Förmlicher",
  "compose.rewriteFormalAsk": "Förmlicher im Ton.",
  "compose.rewriteDeadline": "Frist ergänzen",
  "compose.rewriteDeadlineAsk":
    "Um eine Antwort bis zu einem genannten Datum bitten.",
  "compose.sendOptions": "Andere Sendewege",
  "compose.scheduleSend": "Später senden",
  "compose.scheduleTomorrow": "Morgen früh",
  "compose.scheduleMonday": "Montagmorgen",
  "compose.scheduleNow": "Doch sofort senden",
  "compose.purpose": "Einwilligungszweck",
  "compose.purposeHint":
    "Der Versand ist nur erlaubt, wenn jeder Empfänger für diesen Zweck eingewilligt hat.",
  "compose.sendLaterLabel": "Später senden (optional)",
  "compose.send": "Senden",
  "compose.sendConfirmTitle": "Diese E-Mail senden?",
  "compose.threadHeading": "Dieser Verlauf",
  "compose.continueHeading": "Einen Verlauf fortsetzen?",
  "compose.threadLeave": "Anderen wählen",
  "compose.messageCount_one": "{count} Nachricht",
  "compose.messageCount_other": "{count} Nachrichten",
  "compose.threadContinuing": "Der letzte Austausch, den dies fortsetzt",
  "compose.threadPending": "Verlauf wird geladen\u2026",
  "compose.sendBody":
    "Sie senden diese E-Mail jetzt. Dies ist eine ausgehende, unwiderrufliche Aktion.",
  "compose.schedule": "Einplanen",
  "compose.scheduleConfirmTitle": "Diese E-Mail einplanen?",
  // The composer computed that it had scheduled a send and said nothing —
  // it closed the way a SENT message closes it. The confirm dialog above
  // promises a place to move or withdraw the message from; these two are how a
  // rep gets there.
  "compose.scheduledQueued": "Geplant. Sie ist noch nicht rausgegangen.",
  "compose.scheduledOpenQueue": "Geplante Nachrichten",
  "compose.scheduleBody":
    "Sie geht nicht jetzt hinaus. Sie wartet auf den gewählten Zeitpunkt, und die Einwilligungs- und Postfachprüfungen laufen dann erneut. Bis sie hinausgeht, können Sie sie unter „Geplante Nachrichten“ verschieben oder zurückziehen.",
  "compose.sendMessageConfirmTitle": "Diese Nachricht senden?",
  "compose.sendMessageBody":
    "Sie senden diese Nachricht jetzt. Dies ist eine ausgehende, unwiderrufliche Aktion.",
  "compose.consentBlockedTitle": "Versand blockiert — keine Einwilligung",
  "compose.consentBlocked":
    "Ein Empfänger hat für diesen Zweck nicht eingewilligt, daher wurde der Versand unterdrückt (Standard-Ablehnung).",
  "compose.consentGoto": "Einwilligung prüfen",
  "compose.draftUnavailable":
    "KI-Entwurf ist nicht verfügbar (das Modell ist nicht konfiguriert). Sie können die E-Mail weiterhin selbst schreiben.",
  "compose.sendUnavailable":
    "Versand ist nicht verfügbar (kein Mailer konfiguriert).",
  "compose.mailboxNotSendCapable":
    "Dein Postfach ist zum Erfassen verbunden, hat aber nie die Erlaubnis zum Senden erhalten. Verbinde es neu und stimme dem Versand zu — ein Postfach, das vor der Versandfunktion verbunden wurde, lässt sich nicht nachträglich erweitern.",
  "compose.mailboxNotSendCapableGoto": "Postfach neu verbinden",
  "compose.sharedUnsubscribeToken":
    "Eine Nachricht mit Abmeldelink erreicht immer nur eine Adresse, denn dieser Link ist der Einwilligungsnachweis genau dieses Empfängers. Senden Sie sie einzeln, ohne Cc.",
  "compose.multiRecipientWarning":
    "Dieser Zweck führt einen Abmeldelink mit sich; ein Versand an mehr als eine Adresse wird deshalb abgelehnt. Senden Sie einzeln, ohne Cc.",
  "compose.relinkTitle": "Diese Aktivität neu verknüpfen",
  "compose.relinkTarget":
    "Person, Organisation, Deal, Lead oder Projekt suchen",
  "compose.relinkNoVersion":
    "Diese Aktivität wurde ohne Version gelesen, daher kann eine Neuverknüpfung nicht sagen, was sie ändert. Öffne sie erneut und versuche es noch einmal.",
  "compose.relinkReplace": "Verschieben statt zusätzlich verknüpfen",
  "compose.relinkReplaceHint":
    "Ersetzt die bestehende Verknüpfung desselben Typs, statt eine weitere hinzuzufügen.",
  "compose.relinkConfirm": "Neu verknüpfen",
  "compose.relinkThread": "Auch den Rest dieser Konversation verschieben",
  "compose.relinkThreadHint":
    "Jede Nachricht dieses Threads, die Sie bearbeiten dürfen, wird in einem Schritt mit verschoben.",
  "compose.emptyRecipients": "Fügen Sie mindestens einen Empfänger hinzu.",
  "compose.missingSubject": "Gib dieser E-Mail einen Betreff.",
  "compose.missingBody": "Schreibe die Nachricht, bevor du sie sendest.",
  "compose.missingPurpose": "Wähle, wozu diese Nachricht dient.",
  "compose.removeRecipient": "{recipient} entfernen",
  "compose.actionFailed":
    "Die Anfrage ist fehlgeschlagen. Bitte erneut versuchen.",

  "tasks.complete": "Erledigt",
  "tasks.snooze": "1 Tag später",
  "tasks.detail": "Aufgabe",
  "tasks.isDone": "Abgeschlossen",
  "tasks.logged": "Erfasst",

  "analytics.sub": "Deals je Phase — ungewichtet neben gewichtet",
  "analytics.currency": "Währung",
  "analytics.count": "Deals",
  "analytics.unweighted": "Ungewichtet",
  "analytics.weighted": "Gewichtet",
  "analytics.planNote":
    "der ausgeführte Plan und die Zeilen, auf die sich die Zahl zurückrechnet",
  "analytics.reportDeals": "Deals nach Phase",
  "analytics.sections": "Analytics-Bereiche",
  "analytics.sectionForecast": "Forecast",
  "analytics.sectionPipeline": "Pipeline",
  "analytics.share.open": "Ansicht teilen",
  "analytics.share.title": "Diese Ansicht teilen",
  "analytics.share.kindLegend": "Was der Link zeigt",
  "analytics.share.liveLabel": "Live-Ansicht",
  "analytics.share.liveHelp":
    "Wird bei jedem Öffnen neu berechnet, begrenzt auf das, was die lesende Person sehen darf. Die Zahlen bewegen sich mit der Pipeline.",
  "analytics.share.snapshotLabel": "Eingefrorener Stand",
  "analytics.share.snapshotHelp":
    "Die Zahlen, wie sie beim Einfrieren standen. Sie ändern sich nicht, deshalb nennt der Link den Zeitpunkt.",
  "analytics.share.snapshotUnavailable":
    "Für diesen Zeitraum wurde noch kein Stand eingefroren.",
  "analytics.share.expiryNote":
    "Der Link läuft nach 30 Tagen ab. Sie können ihn früher schließen.",
  "analytics.share.create": "Link erstellen",
  "analytics.share.linkTitle": "Ihr Link",
  "analytics.share.linkWarning":
    "Der Link wird nur dieses eine Mal angezeigt. Kopieren Sie ihn jetzt — er lässt sich nicht erneut auslesen.",
  "analytics.share.leaveWarning":
    "Wenn Sie ohne Kopieren schließen, ist der Link verloren. Sie müssten einen neuen erstellen.",
  "analytics.share.copy": "Link kopieren",
  "analytics.share.copied": "Kopiert",
  "analytics.share.copyFailed":
    "Der Link konnte nicht kopiert werden. Markieren Sie ihn oben und kopieren Sie ihn von Hand.",
  "analytics.share.done": "Fertig",
  "analytics.frame": "Stand {asOf} · {zone} · {currency}",
  "review.title": "Was sollte vor dem Call geprüft werden?",
  "review.ready": "Bereit",
  "review.readyWithExceptions": "Bereit, mit Anmerkungen",
  "review.needsReview": "Prüfung nötig",
  "review.checksIncomplete": "Prüfung unvollständig",
  "review.allSourcesRead": "Alle Quellen wurden gelesen.",
  "review.sourcesUnread":
    "Nicht gelesen: {sources}. Die Befunde unten decken nur ab, was geprüft werden konnte.",
  "review.nothingToCheck": "Nichts zu prüfen.",
  "review.answer": "Prüfen",
  "review.closePast": "Abschlussdatum ist verstrichen",
  "review.closeUnconfirmed": "Abschlussdatum nicht bestätigt",
  "review.closePushed": "Abschlussdatum verschiebt sich laufend",
  "review.amountVsOffer": "Betrag weicht vom Angebot ab",
  "review.amountVsContract": "Betrag weicht vom Vertrag ab",
  "review.noNextStep": "Kein nächster Schritt",
  "review.noEconomicBuyer": "Niemand mit Unterschriftsbefugnis benannt",
  "review.buyerSilent": "Käufer ist verstummt",
  "review.commitUnpriced": "Commit ohne Betrag",
  "review.unknownCheck": "Etwas zu prüfen",
  "review.sheetTitle": "Diese Prüfung beantworten",
  "review.outcomeLegend": "Was für eine Antwort ist das?",
  "review.fixedRecord": "Ich habe den Datensatz korrigiert",
  "review.addedEvidence": "Ich habe den Beleg ergänzt",
  "review.valueCorrect": "Aktueller Wert ist korrekt",
  "review.notRelevant": "Für diesen Deal nicht relevant",
  "review.remindLater": "Jetzt nicht",
  "review.reassign": "Jemand anderes beantwortet das",
  "review.hidesUntilExpiry": "Blendet diese Prüfung bis zum Ablauf aus.",
  "review.reason": "Begründung",
  "review.reasonHelp":
    "Wer diese Zahl als Nächstes sieht, hat ein Recht auf den Grund, warum sie nicht markiert ist.",
  "review.remindAt": "Wieder vorlegen am",
  "review.expiresAt": "Gilt nicht mehr ab",
  "review.expiresHelp":
    "Höchstens 90 Tage: Ein Wert, der im Mai korrekt war, ist eine Aussage über den Mai.",
  "review.cancel": "Abbrechen",
  "review.submit": "Antwort speichern",
  "forecast.question": "Wo landen wir dieses Quartal?",
  "forecast.answerWithCall":
    "Der aktuelle Call liegt bei {call}. Durch Belege gestützt sind {evidence}.",
  "forecast.answerNoCall":
    "Für diesen Zeitraum gibt es noch keinen Call. Durch Belege gestützt sind {evidence}.",
  "forecast.partialTitle": "Nicht jeder Deal hat einen Betrag",
  "forecast.partial":
    "{priced} von {eligible} Deals führen einen Betrag. Die übrigen sind echte Pipeline und tragen nichts zu den Summen oben bei.",
  "forecast.currentCall": "Aktueller Call",
  "forecast.evidence": "Durch Belege gestützt",
  "forecast.alreadyWon": "Bereits gewonnen",
  "forecast.updateCall": "Aktuellen Call aktualisieren",
  "forecast.callExplains":
    "Ein Call ist das, was Sie erwarten. Er hält Ihre Zahl fest und ändert keinen Deal.",
  "forecast.expectedTotal": "Erwartete Summe für diesen Zeitraum",
  "forecast.supportingNote": "Begründung",
  "forecast.cancel": "Abbrechen",
  "forecast.saveCall": "Call speichern",
  "forecast.receipt": "Daten und Belege geprüft",
  "forecast.eligible": "Berücksichtigte Deals",
  "forecast.priced": "Mit Betrag",
  "forecast.confirmed": "Abschlussdatum bestätigt",
  "forecast.fxMissing": "Wechselkurs fehlt",
  "analytics.reportForecast": "Forecast-Kategorien",
  "analytics.reportOpenByCompany": "Offene Deals pro Firma",
  "analytics.forecastBanner":
    "Jede Kachel zeigt die Rohsumme und darunter die gewichtete Summe — pro Deal gerundet, sodass sie immer mit „Diese Zahl erklären“ übereinstimmt.",
  "analytics.company": "Firma",
  "analytics.openDeals": "Offene Deals",
  "explain.sources": "Quellzeilen",

  "ai.sub": "bring deinen eigenen Agenten mit — geregelt über die zwei Stufen",
  "ai.tiers": "Was ein Agent darf",
  "ai.tierAutoExecute": "Lesen & Entwerfen läuft sofort.",
  "ai.tierAutoExecuteDetail":
    "Nachschlagen, Zusammenfassen, Entwürfe — sichtbar, umkehrbar, protokolliert.",
  "ai.tierConfirmationRequired": "Sensible Änderungen warten auf dich.",
  "ai.tierConfirmationRequiredDetail":
    "Neue benutzerdefinierte Felder, Webhook-Abonnements und kostenpflichtige Anreicherung landen zuerst im Eingang. Die meisten Datensatzänderungen und Sendungen laufen sofort, innerhalb der von dir erteilten Berechtigungen.",
  "ai.connect": "Agent verbinden",
  "ai.connectDetail":
    "Verbinde einen MCP-fähigen Agenten mit deiner Organisation und bestätige den Zugriff, um den er bittet. Es gibt nichts vorab einzurichten.",
  "ai.paletteHint": "Frag von überall mit",

  "settings.accountCard": "Ihr Konto",
  "unsaved.title": "Du hast ungespeicherte Änderungen",
  "unsaved.body":
    "Wenn du die Seite jetzt verlässt, gehen deine Eingaben verloren. Geh zurück, um sie zuerst zu speichern.",
  "unsaved.discard": "Änderungen verwerfen",
  "settings.addedItem": "„{name}“ hinzugefügt",
  "settings.removedItem": "„{name}“ entfernt",
  "settings.removed": "Entfernt.",
  "settings.saved": "Gespeichert.",
  "settings.signature": "E-Mail-Signatur",
  "settings.signatureSub":
    "Steht unter jeder Nachricht, die Sie senden — über dem Abmelde-Hinweis.",
  "settings.signatureLabel": "Ihre Grußformel",
  "settings.signaturePlaceholder": "Marek Janetzke\nGradion · +49 40 123456",
  "settings.signatureHint":
    "Nur Text. Leer lassen heißt: ohne Signatur senden. Die KI schreibt nie eine Grußformel — diese hier geht raus.",
  "settings.signatureSaving": "Wird gespeichert…",
  "settings.signatureEdit": "Signatur bearbeiten",
  "settings.signatureNone": "Keine Grußformel gesetzt",
  "settings.signatureCancel": "Abbrechen",
  "brief.coverage.summary": "Einige Quellen haben mehr, als diese Seite zeigt",
  "brief.coverage.bounded":
    "{shown} von mindestens {considered} gelesenen angezeigt",
  "delivery.morningLabel": "Ihr Tagesbriefing",
  "delivery.morningHelp":
    "Ob das Briefing des Tages zusätzlich per E-Mail kommt. Auf Ihrer Briefing-Seite steht es ohnehin.",
  "delivery.weeklyLabel": "Ihr Wochenrückblick",
  "delivery.weeklyHelp":
    "Ob der Rückblick am Montag zusätzlich per E-Mail kommt.",
  "delivery.byEmail": "Per E-Mail",
  "delivery.none": "Nicht per E-Mail",
  "settings.languageHelp": "Gilt für diese Sitzung.",
  "role.admin": "Admin",
  "role.management": "Geschäftsleitung",
  "role.manager": "Teamleitung",
  "role.rep": "Benutzer",
  "role.readOnly": "Nur Lesen",
  "role.ops": "Ops",
  "inlineChoice.change": "{field} ändern",
  "rbac.masked": "Verborgener Wert",
  "settings.passports": "Agenten-Passports",
  "settings.passportsSub":
    "Ein Agent handelt als du, nie über dir: jeder Aufruf prüft deine Rechte neu.",
  "passport.scope.read": "Datensätze lesen",
  "passport.scope.draft": "Nachrichten entwerfen",
  "passport.scope.write": "Datensätze ändern",
  "passport.scope.send": "Nachrichten senden",
  "passport.scope.enrich": "Kontaktdaten kaufen",
  "passport.select": "Passport",
  "passport.noneOption": "Kein Passport",
  "settings.passportsLendHint":
    "Zugangsdaten, die du für Skripte und Integrationen ausgestellt hast. Das Verbinden eines MCP-Clients nutzt diese nicht — er erstellt eine eigene Verbindung, unten aufgeführt.",
  "settings.passportLabel": "Agentenname",
  "settings.mint": "Passport ausstellen",
  "settings.minting": "Wird ausgestellt…",
  "settings.mintCancel": "Abbrechen",
  "settings.mintDone": "Fertig",
  "settings.mintOpen": "Neuer Passport",
  "settings.passportScopes": "Was dieser Agent darf",
  "settings.passportScopesHint":
    "Mindestens eins auswählen. Ein Agent darf nie mehr als Sie selbst.",
  "settings.passportScopesRequired":
    "Wählen Sie mindestens eine Sache aus, die dieser Agent tun darf.",
  // Was der geplante Agent gerade für diese Leserin tut. "Morgenbriefing" ist
  // dasselbe Wort wie auf der Startseite; ein abgebrochener Lauf darf nie
  // klingen, als wäre er fertig.
  "agent.activity.weeklyReview.queued":
    "Deine Woche wartet auf eine Zusammenfassung.",
  "agent.activity.weeklyReview.running": "Deine Woche wird zusammengefasst…",
  "agent.activity.weeklyReview.stalled":
    "Die Zusammenfassung deiner Woche dauert länger als erwartet.",
  "agent.activity.weeklyReview.done": "Deine Woche hat eine Zusammenfassung.",
  "agent.activity.weeklyReview.degraded":
    "Deine Woche ist ausgewertet, ohne Zusammenfassung — die Zahlen sind vollständig.",
  "agent.activity.weeklyReview.failed":
    "Diesmal keine Zusammenfassung deiner Woche. Die Zahlen sind trotzdem die der Woche.",
  "agent.activity.morningBrief.queued": "Dein Morgenbriefing ist eingereiht.",
  "agent.activity.morningBrief.running":
    "Ich stelle dein Morgenbriefing zusammen.",
  "agent.activity.morningBrief.done": "Dein Morgenbriefing ist fertig.",
  "agent.activity.morningBrief.degraded":
    "Ich bin bei deinem Morgenbriefing nur zum Teil gekommen und habe gestoppt.",
  "agent.activity.morningBrief.failed":
    "Ich konnte dein Morgenbriefing nicht abschließen.",
  "agent.activity.morningBrief.stalled":
    "Dein Morgenbriefing läuft ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.activity.riskSweep.queued":
    "Die nächtliche Risikoprüfung ist eingereiht.",
  "agent.activity.riskSweep.running": "Ich prüfe deine Deals auf Risiken.",
  "agent.activity.riskSweep.done":
    "Fertig. Ich habe deine Deals nachts auf Risiken geprüft.",
  "agent.activity.riskSweep.degraded":
    "Ich bin bei der Risikoprüfung nur zum Teil gekommen und habe gestoppt.",
  "agent.activity.riskSweep.failed":
    "Ich konnte die nächtliche Risikoprüfung nicht abschließen.",
  "agent.activity.riskSweep.stalled":
    "Die Risikoprüfung läuft ungewöhnlich lange. Möglicherweise wurde sie abgebrochen.",
  "agent.activity.documentExtract.queued":
    "Dein Dokument steht zum Lesen in der Warteschlange.",
  "agent.activity.documentExtract.running": "Ich lese dein Dokument.",
  "agent.activity.documentExtract.stalled":
    "Das Lesen deines Dokuments dauert ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.activity.documentExtract.done": "Ich habe dein Dokument gelesen.",
  "agent.activity.documentExtract.degraded":
    "Ich bin bei deinem Dokument nur teilweise durchgekommen und habe gestoppt.",
  "agent.activity.documentExtract.failed":
    "Ich konnte dein Dokument nicht lesen.",
  "agent.activity.documentExtractNamed.queued":
    "{name} steht zum Lesen in der Warteschlange.",
  "agent.activity.documentExtractNamed.running": "Ich lese {name}.",
  "agent.activity.documentExtractNamed.stalled":
    "Das Lesen von {name} dauert ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.activity.documentExtractNamed.done": "Ich habe {name} gelesen.",
  "agent.activity.documentExtractNamed.degraded":
    "Ich bin bei {name} nur teilweise durchgekommen und habe gestoppt.",
  "agent.activity.documentExtractNamed.failed":
    "Ich konnte {name} nicht lesen.",
  "agent.activity.summarize.queued":
    "Das Zusammentragen zu diesem Unternehmen steht in der Warteschlange.",
  "agent.activity.summarize.running":
    "Ich trage zusammen, was ich über dieses Unternehmen weiß.",
  "agent.activity.summarize.done":
    "Was ich über dieses Unternehmen weiß, ist fertig.",
  "agent.activity.summarize.degraded":
    "Ich habe über dieses Unternehmen nur teilweise Informationen zusammengetragen und dann aufgehört.",
  "agent.activity.summarize.failed":
    "Ich konnte das Zusammentragen zu diesem Unternehmen nicht abschließen.",
  "agent.activity.summarize.stalled":
    "Das Zusammentragen zu diesem Unternehmen dauert ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.activity.draftReply.queued":
    "Deine Antwort steht zum Entwerfen in der Warteschlange.",
  "agent.activity.draftReply.running": "Ich entwerfe deine Antwort.",
  "agent.activity.draftReply.done": "Dein Antwortentwurf ist fertig.",
  "agent.activity.draftReply.degraded":
    "Ich bin bei deiner Antwort nur teilweise durchgekommen und habe gestoppt.",
  "agent.activity.draftReply.failed":
    "Ich konnte deine Antwort nicht entwerfen.",
  "agent.activity.draftReply.stalled":
    "Das Entwerfen deiner Antwort dauert ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.activity.offerDraft.queued":
    "Dein Angebot steht zum Entwerfen in der Warteschlange.",
  "agent.activity.offerDraft.running": "Ich entwerfe dein Angebot.",
  "agent.activity.offerDraft.done": "Dein Angebotsentwurf ist fertig.",
  "agent.activity.offerDraft.degraded":
    "Ich bin bei deinem Angebot nur teilweise durchgekommen und habe gestoppt.",
  "agent.activity.offerDraft.failed":
    "Ich konnte dein Angebot nicht entwerfen.",
  "agent.activity.offerDraft.stalled":
    "Das Entwerfen deines Angebots dauert ungewöhnlich lange. Möglicherweise wurde es abgebrochen.",
  "agent.panel.runningNow": "Läuft jetzt",

  "agents.connected": "Verbundene Agenten",
  "agents.connectedSub":
    "MCP-Clients mit eigenem Credential, mit dem Zugriff, den du bei der Autorisierung angekreuzt hast",
  "agents.noneConnected": "Noch ist kein Agent verbunden.",
  "agents.connectedOn": "verbunden {date}",
  "agents.disconnect": "Trennen",
  "agents.disconnectOpen": "Trennen",
  "agents.disconnectNamed": "{client} trennen",
  "agents.disconnected": "getrennt",
  "agents.lapsed": "Credential abgelaufen",
  "agents.renewing": "wird erneuert",
  "agents.renewsBy": "Credential erneuert bis {date}",
  "agents.expiredOn": "Credential abgelaufen {date}",
  "agents.revokeGrantOpen": "Verbindung beenden",
  "agents.revokeGrantNamed": "Verbindung zu {client} beenden",
  "agents.disconnectConfirm":
    "Das beendet die ganze Verbindung, nicht nur ein Credential: der Agent verliert den Zugriff beim nächsten Aufruf und kann nicht erneuern. Für eine neue Verbindung musst du den Zugriff erneut genehmigen.",
  "agents.connectHow": "Agent verbinden",
  "agents.connectSteps":
    "Führe einen dieser Befehle aus. Der Client registriert sich selbst und bringt dich hierher zurück, um den Zugriff zu wählen, den er erhalten darf.",
  "agents.connectAntigravityPath":
    "Antigravity hat keinen Add-Befehl — trage den Block in ~/.gemini/config/mcp_config.json ein.",
  "agents.connectorOff": "Der MCP-Connector ist für diese Installation aus.",
  "agents.connectorOffDetail":
    "Bis ein Betreiber ihn einschaltet, kann sich kein Agent verbinden. Deine Passports funktionieren weiterhin als REST-Credentials.",
  "settings.tokenOnce": "Jetzt kopieren — dieses Token siehst du nur einmal.",
  "settings.token": "Token",
  "settings.autonomy": "Autonomie-Stufen",
  "settings.autonomySub": "was sofort läuft und was im Eingang wartet",
  "settings.tierRead":
    "Lesen, Zusammenfassen, Entwerfen — läuft sofort, voll protokolliert.",
  "settings.tierSend":
    "E-Mail senden, Termine buchen, Daten ändern — wartet auf deine Freigabe.",
  "settings.tierAdvance": "Deal-Phase weiterschieben — immer erst bestätigen.",
  "settings.locked": "gesperrt",
  "settings.purposes": "Einwilligungszwecke",
  "settings.purposesSub":
    "Wofür diese Installation Einwilligung einholt und auf welcher Rechtsgrundlage jeder Zweck steht.",
  "settings.created": "erstellt {date}",
  "settings.expires": "läuft ab {date}",
  "settings.revoked": "widerrufen",
  "settings.revoke": "Widerrufen",
  "settings.revokeConfirm":
    "Das Credential dieses Passports wird sofort ungültig — der Agent verliert beim nächsten Aufruf den Zugriff.",
  "import.withheld":
    "Eine Datei zu importieren ist eine Admin- oder Ops-Aktion — es gibt sie in dieser Installation, du darfst sie nur nicht ausführen.",
  "import.title": "Datei importieren",
  "import.sub":
    "Eine CSV mit Interessenten oder Firmen einlesen. Es wird nichts geschrieben, bevor Sie gelesen haben, was passieren wird.",
  "import.startLabel": "CSV-Datei importieren",
  "import.start": "Import starten",
  "import.objectLabel": "Was die Zeilen sind",
  "import.object.lead": "Interessenten",
  "import.object.organization": "Firmen",
  "import.object.person": "Kontakte",
  "import.objectHint.lead":
    "Eine unbearbeitete Liste landet als Leads zur Qualifizierung, bevor sie jemand als Kontakte behandelt.",
  "import.objectHint.organization":
    "Firmen werden über den zugeordneten Namen erkannt, ein erneuter Upload korrigiert also statt zu duplizieren.",
  "import.objectHint.person":
    "Für Personen, mit denen Sie bereits zu tun haben. Erkennung über die E-Mail-Adresse: Ein erneuter Upload korrigiert statt zu duplizieren, und eine bereits vergebene Adresse bleibt unangetastet.",
  "import.fileLabel": "Die zu importierende CSV",
  "import.choose": "Datei wählen",
  "import.chooseAnother": "Andere Datei wählen",
  "import.profiled": "Aus den ersten {rows} Zeilen der Datei gelesen.",
  "import.mappingTable": "Spaltenzuordnung",
  "import.col.column": "Spalte",
  "import.col.filled": "Gefüllt",
  "import.col.samples": "Werte",
  "import.col.destination": "Geht nach",
  "import.dontImport": "Nicht importieren",
  "import.noSamples": "leer",
  "import.destinationFor": "Wohin {column} geht",
  "import.identifiedBy":
    "Zeilen werden über {column} erkannt — ein erneuter Import dieser Datei aktualisiert statt zu duplizieren.",
  "import.needsIdentifier":
    "Ordnen Sie eine Spalte {field} zu. Ohne sie ist keine Zeile beim zweiten Upload wiedererkennbar oder rückgängig zu machen.",
  "import.validate": "Prüfen, was passieren würde",
  "import.validating": "Wird geprüft…",
  "import.previewTitle": "Was dieser Import tun wird",
  "import.outcomeTitle": "Was dieser Import getan hat",
  "import.resumedRun":
    "Von vorhin übernommen: Dieser Import lief am {when}. Alles darunter steht Ihnen weiterhin offen.",
  "import.count.created": "Anlegen",
  "import.count.updated": "Aktualisieren",
  "import.count.unchanged": "Unverändert",
  "import.count.skipped": "Übersprungen",
  "import.rowsRead": "{rows} Zeilen gelesen, erkannt über {column}.",
  "import.linksOffered":
    "{offered} Zeilen nennen einen Arbeitgeber; bei {unresolved} ist die Firma noch nicht im CRM.",
  "import.linksApplied":
    "{applied} von {offered} Arbeitgeber-Verknüpfungen geschrieben.",
  "import.issuesLead":
    "Einige Zeilen können nicht importiert werden. Sie sind mit der Zeilennummer in Ihrer Datei aufgeführt.",
  "import.issueLine": "Zeile {line}:",
  "import.commit_one": "1 Zeile importieren",
  "import.commit_other": "{rows} Zeilen importieren",
  "import.importing": "Wird importiert…",
  "import.done": "Der Import ist abgeschlossen.",
  "import.failed":
    "Der Import hat nach {checkpoint} Zeilen gestoppt. Fortsetzen macht dort weiter, statt neu zu beginnen.",
  "import.resume": "Import fortsetzen",
  "import.another": "Weitere Datei importieren",
  "import.undo_one": "Diesen Import rückgängig machen (1 Zeile)",
  "import.undo_other": "Diesen Import rückgängig machen ({rows} Zeilen)",
  "import.undoing": "Wird rückgängig gemacht…",
  "import.undoInterrupted":
    "Das Rückgängigmachen wurde unterbrochen. Fortsetzen macht dort weiter, wo es aufgehört hat, nicht von vorn.",
  "import.continueUndo": "Rückgängigmachen fortsetzen",
  "import.undone": "Der Import wurde rückgängig gemacht.",
  "import.undoReversed_one": "1 Zeile rückgängig gemacht.",
  "import.undoReversed_other": "{rows} Zeilen rückgängig gemacht.",
  "import.undoKeptLead":
    "Beibehalten — diese wurden seit dem Import bearbeitet:",
  "import.undoErroredLead":
    "Konnte nicht rückgängig gemacht werden — unverändert belassen:",
  "settings.dangerZone": "Gefahrenzone",
  "settings.dangerZoneSub":
    "Nur nicht-produktiv — auf dieser Installation nicht rückgängig zu machen.",
  "settings.resetDataDesc":
    "Setzt diese Installation auf den Zustand nach der Ersteinrichtung zurück. Fach- und Konfigurationsdaten werden gelöscht; die Organisation und ihre Nutzer bleiben erhalten und angemeldet.",
  "settings.resetDataButton": "Daten zurücksetzen",
  "settings.resetDataLabel": "Alle Daten zurücksetzen",
  "settings.resetDataConfirmButton": "Alles zurücksetzen",
  "settings.resetDataConfirmTitle": "Alle Daten zurücksetzen?",
  "settings.resetDataConfirmBody":
    "Gib zur Bestätigung den Namen deiner Organisation ein. Dies kann nicht rückgängig gemacht werden.",
  "settings.resetDataConfirmName": "Gib diesen Organisationsnamen ein:",
  "settings.resetDataConfirmLabel": "Organisationsname bestätigen",
  "settings.resetDataResult":
    "{tables} Tabellen, {jobs} Job-Einträge, {streams} Event-Streams, {keys} Cache-Schlüssel und {objects} gespeicherte Dateien gelöscht.",
  "settings.resetDataDrainWarning":
    "Beim Start des Zurücksetzens lief noch ein Hintergrund-Job. Er schlägt gegen die gelöschten Daten fehl — unkritisch, aber es erscheint ein Fehler im Log.",

  "settings.jobs": "Hintergrund-Jobs",
  "settings.jobsSub":
    "Was in der Warteschlange hängt und wessen Arbeit gescheitert ist.",
  "jobs.adminOnly":
    "Nur ein Admin sieht den Zustand der Hintergrund-Jobs. Der Bericht umfasst die Arbeit der ganzen Installation und wird deshalb nicht breiter gezeigt.",
  "jobs.empty":
    "Nichts in der Hintergrund-Warteschlange — nichts wartet, läuft, wiederholt sich oder ist tot.",
  "jobs.workspaceKinds": "Diese Organisation",
  "jobs.workspaceEmpty":
    "Keine Hintergrundarbeit irgendeiner Art in dieser Organisation.",
  "jobs.dispatcherKinds": "Flotten-Dispatcher",
  "jobs.dispatcherSub":
    "Einträge ohne Organisation: ein Dispatcher verteilt Arbeit an jede Organisation und erledigt selbst keine. Diese Zahlen gehören der Installation, nicht dir.",
  "jobs.dispatcherEmpty":
    "Keine Dispatcher-Einträge. Die periodischen Ticks legen sie neu an — eine leere Liste heißt also, dass gerade keiner geplant ist.",
  "jobs.count.waiting": "{count} warten",
  "jobs.count.running": "{count} laufen",
  "jobs.count.retrying": "{count} wiederholen",
  "jobs.count.dead": "{count} tot",
  "jobs.queue": "Queue {queue}",
  "jobs.waitedSeconds_one": "ältester wartet seit {count} Sekunde",
  "jobs.waitedSeconds_other": "ältester wartet seit {count} Sekunden",
  "jobs.waitedMinutes_one": "ältester wartet seit {count} Minute",
  "jobs.waitedMinutes_other": "ältester wartet seit {count} Minuten",
  "jobs.waitedHours_one": "ältester wartet seit {count} Stunde",
  "jobs.waitedHours_other": "ältester wartet seit {count} Stunden",
  "jobs.waitedDays_one": "ältester wartet seit {count} Tag",
  "jobs.waitedDays_other": "ältester wartet seit {count} Tagen",
  "jobs.deadTitle": "Tote Arbeit braucht deine Hand",
  "jobs.deadBody":
    "{count} Jobs sind verworfen oder abgebrochen: diese Arbeit passiert ohne Eingriff nicht mehr. Ein verworfener Job hat alle Versuche verbraucht, ein abgebrochener wurde absichtlich gestoppt. Lies die Fehler unten, bevor du etwas neu einreihst.",
  "jobs.failures": "Letzte Fehler",
  "jobs.failuresSub":
    "Neueste zuerst, maximal 50. Eine begrenzte Liste, kein Log.",
  "jobs.failuresEmpty": "Keine Fehler erfasst.",
  "jobs.state.retryable": "wiederholt",
  "jobs.state.discarded": "verworfen",
  "jobs.state.cancelled": "abgebrochen",
  "jobs.attempt": "Versuch {attempt} von {max} · {when}",
  "jobs.remedy": "Zu tun: {remedy}",
  "jobs.jobId": "Job {id}",
  "jobs.failingSince": "fehlerhaft seit {when}",
  "jobs.reasonVetted":
    "Grund, Klasse und Abhilfe werden jeweils von der Job-Schicht selbst formuliert, nie aus der Rohursache des Workers. Kann sie einen Fehler nicht formulieren, meldet sie einen festen Ersatztext und gar keine Klasse. Eine für ungeprüften Text erfundene Klasse würde deine Alarme auf eine Vermutung stützen.",
  "jobs.generatedAt": "Gelesen um {time}",

  "audit.you": "Du",
  "audit.system": "System",
  "audit.unknownBuyer": "Teilnehmer im Deal Room",
  "audit.unknownMember": "Unbekanntes Mitglied",
  "audit.viaAgent": "über einen Agenten",
  "audit.viaConnector": "über einen Connector",
  "audit.viaDealRoom": "im Deal Room",
  "audit.viaNamed": "über {client}",
  "audit.noHumanAuthority": "Keine menschliche Autorisierung erfasst",
  "settings.auditSub": "jede Aktion, zugeordnet — Mensch, Agent oder Connector",
  "settings.auditAdminOnly":
    "Nur Admins lesen den vollständigen Verlauf. Er hält jede handelnde Person und jeden berührten Datensatz fest — deshalb ist er nicht weiter zugänglich.",
  "settings.auditFilters": "Filter",
  "settings.auditEntries": "Audit-Log",
  "settings.auditTrailLabel": "Aufgezeichnete Aktionen",
  "settings.auditActor": "Akteur",
  "settings.auditEntity": "Entitätstyp",
  "settings.auditEntityId": "Entitäts-ID",
  "settings.auditAction": "Aktion",
  "settings.auditFrom": "Von",
  "settings.auditTo": "Bis",
  "settings.auditExpand": "Änderungsdetail anzeigen",
  "settings.auditRule": "Berechtigungsregel",
  "settings.auditOnBehalf": "im Auftrag von",
  "settings.privacy": "Datenschutz-Eingang",
  "settings.privacySub": "Betroffenenanfragen mit ihren gesetzlichen Fristen",
  "settings.due": "fällig {date}",

  "privacy.purposesReadOnly":
    "Nur-Lese-Ansicht — nur ein Admin oder Ops kann einen Zweck anlegen.",
  "privacy.addPurpose": "Zweck hinzufügen",
  "privacy.purposesRegistry": "Erfasste Zwecke",
  "privacy.purposeKey": "Schlüssel",
  "privacy.purposeLabel": "Bezeichnung",
  "privacy.purposeDoi": "Erfordert Double-Opt-in",
  "privacy.purposeCreate": "Zweck anlegen",
  "privacy.purposeAppendOnly":
    "Ein Zweck kann nach dem Anlegen nicht umbenannt oder entfernt werden — der Katalog ist append-only. Wähle den Schlüssel sorgfältig.",
  "privacy.facetAll": "Alle",
  "privacy.inboxAdminOnly":
    "Nur Admins sehen Betroffenenanfragen. Sie nennen die Personen, die angefragt haben — deshalb ist die Liste nicht weiter zugänglich.",
  "privacy.overdue": "Überfällig",
  "privacy.closed":
    "Abgeschlossen — eine abgeschlossene Anfrage wird nie wieder geöffnet. Ein neues Anliegen ist eine neue Anfrage.",
  "privacy.assignee": "Zuständig",
  "privacy.assigneeUnassignable":
    "Einmal gesetzt, kann die Zuständigkeit hier nicht entfernt werden.",
  "privacy.resolution": "Ergebnis",
  "privacy.resolutionRequired":
    "Zum Abschließen einer Anfrage braucht es ihre Antwort.",
  "privacy.movedOn":
    "Diese Anfrage ist weitergezogen — jemand anders hat zuerst entschieden. Bitte unten neu lesen.",
  "privacy.inProgress": "In Bearbeitung",
  "privacy.fulfil": "Erfüllen",
  "privacy.reject": "Ablehnen",
  "privacy.newRequest": "Neuer Antrag",
  "privacy.queue": "Anträge",
  "privacy.kind": "Art",
  "privacy.person": "Person",
  "privacy.subjectRef": "Betroffenen-Referenz",
  "privacy.dueAt": "Frist",
  "privacy.openRequest": "Antrag anlegen",
  "privacy.erasureNeedsPerson":
    "Ein Löschantrag muss eine Person in dieser Organisation benennen — bei Erfüllung wird genau dieser Datensatz gelöscht. Eine Freitext-Referenz kann nicht gelöscht werden.",
  "privacy.accessManual":
    "Ein Auskunftsantrag wird von Hand erfüllt: Halte im Ergebnis fest, was du versendet hast. Dieses System stellt die Daten nicht automatisch zusammen und exportiert sie nicht für dich.",
  "privacy.fulfilErasureTitle": "Löschantrag erfüllen",
  "privacy.erasureIrreversible":
    "Dies löscht die Person dauerhaft im gesamten System — Datensatz, erfasste Aktivität und abgeleitete Werte. Das kann nicht rückgängig gemacht werden. Die Löschung selbst wird protokolliert.",
  "privacy.typeErase": "Zum Bestätigen ERASE eingeben",
  "privacy.erasureConfirm": "Löschen + sperren",
  "privacy.legalHold":
    "Blockiert — gesetzliche Aufbewahrungspflicht. Diese Person befindet sich innerhalb einer gesetzlichen Aufbewahrungsfrist, daher setzt sich die Löschung hier nicht durch (Art. 17 Abs. 3 lit. b). Die Sperre gilt für jede Rolle, einschließlich Admin — es gibt kein Umgehen davon. Der Versuch wurde protokolliert.",

  "restricted.title": "Zurückgehaltene Datensätze",
  "restricted.sub":
    "was eine gesetzliche Aufbewahrungspflicht nach einer Löschung zurückhält — welcher Datensatz, warum und bis wann. Die Korrespondenz selbst wird nicht gezeigt: Sie ist genau deshalb eingeschränkt, damit sie nicht gelesen wird.",
  "restricted.withheld":
    "Nur ein Admin oder Ops sieht, welche Datensätze eine gesetzliche Pflicht zurückhält. Es gilt dieselbe Berechtigung wie für die Aufbewahrungsregeln.",
  "restricted.empty":
    "Kein Datensatz wird zurückgehalten — jede bisherige Löschung konnte vollständig ausgeführt werden.",
  "restricted.heldLabel": "Aktuell zurückgehaltene Datensätze",
  "restricted.kind": "Datensatz",
  "restricted.occurred": "Datiert",
  "restricted.deals": "Geschäft",
  "restricted.noDeal": "Kein Deal hinterlegt",
  "restricted.reason": "Zurückgehalten wegen",
  "restricted.until": "Zurückgehalten bis",
  "restricted.redacted": "Geschwärzt",
  "restricted.nothingRedacted": "Nichts entfernt",
  "restricted.redactedCount": "{count} Felder entfernt",
  "restricted.class.commercialCorrespondence": "Handelsbrief",
  "restricted.kind.email": "E-Mail",
  "restricted.kind.call": "Anruf",
  "restricted.kind.meeting": "Meeting",
  "restricted.kind.message": "Nachricht",
  "restricted.decide": "Entscheidung",
  "restricted.reasonLabel": "Begründung",
  "restricted.reasonHint":
    "Wird mit Ihrem Namen im Audit-Protokoll festgehalten. Das macht die Entscheidung nachvollziehbar — schreiben Sie, was Sie entschieden haben und auf welcher Grundlage.",
  "restricted.release.action": "Freigeben",
  "restricted.release.title":
    "Diesen Datensatz aus der Aufbewahrungspflicht entlassen?",
  "restricted.release.body":
    "Die Freigabe LÖSCHT den Datensatz. Er kehrt nicht in den Betrieb zurück: Das Löschersuchen, das diese Pflicht ausgesetzt hat, ist weiterhin offen — die Freigabe führt es aus. Das lässt sich nicht rückgängig machen.",
  "restricted.release.confirm": "Freigeben und löschen",
  "restricted.pin.action": "Datensatz festsetzen",
  "restricted.pin.submit": "Anheften",
  "restricted.pin.idHint":
    "Für Korrespondenz, die die automatische Regel nicht erkennt — Lieferanten- und Einkaufspost ist nach §257 HGB aufbewahrungspflichtig und hat in diesem Produkt kein Geschäft, an dem sie hängt. Die Datensatz-ID steht im Audit-Eintrag.",
  "restricted.pin.idMalformed":
    "Das ist keine Datensatz-ID. Sie besteht aus 8-4-4-4-12 Hexadezimalzeichen und steht vollständig im Audit-Eintrag des Datensatzes.",
  "restricted.pin.idPlaceholder": "Datensatz-ID",
  "restricted.pin.title":
    "Diesen Datensatz der Aufbewahrungspflicht unterstellen?",
  "restricted.pin.body":
    "Der Datensatz wird für die gesetzliche Frist zurückgehalten: in keiner normalen Ansicht sichtbar, unveränderbar, und nach Ablauf gelöscht. Seine Identifikatoren werden sofort geschwärzt.",
  "restricted.pin.confirm": "Festsetzen und zurückhalten",
  "retention.title": "Aufbewahrung",
  "retention.sub":
    "wie lange jede Art von Datensatz aufbewahrt wird und was nach Ablauf der Frist geschieht",
  "retention.retainOnly": "Nur-Aufbewahren-Modus",
  "retention.retainOnlyHelp":
    "Solange dies aktiv ist, vernichtet diese Installation nichts: kein Anonymisieren und kein Löschen, unabhängig davon, was eine Richtlinie unten vorsieht. Archivieren läuft weiter — ein archivierter Datensatz bleibt erhalten.",
  "retention.adminOnly": "Nur ein Admin oder Ops kann die Aufbewahrung ändern.",
  "retention.withheld":
    "Nur ein Admin oder Ops sieht die Aufbewahrungsregeln. Diese Regeln legen fest, was diese Installation für alle behält, und werden deshalb nicht breiter gezeigt.",
  "retention.addPolicy": "Richtlinie hinzufügen",
  "retention.create": "Richtlinie erstellen",
  "retention.scope": "Gilt für",
  "retention.window": "Frist in Tagen",
  "retention.windowDays": "{days} Tage",
  "retention.windowInvalid":
    "Eine Frist ist eine ganze Zahl von Tagen, mindestens 1.",
  "retention.action": "Aktion",
  "retention.actionHint":
    "Archivieren bewahrt den Datensatz; Anonymisieren und Löschen vernichten Daten und sind die beiden, die der Nur-Aufbewahren-Modus zurückhält.",
  "retention.lawfulBasis": "Rechtsgrundlage",
  "retention.lawfulBasisHint":
    "Optional. Die Grundlage nach Art. 6, auf die sich diese Frist stützt — für die prüfende Person, die die Zeile liest.",
  "retention.enabled": "Aktiv",
  "retention.edit": "Bearbeiten",
  "retention.save": "Richtlinie speichern",
  "retention.delete": "Richtlinie löschen",
  "retention.deleteTitle": "Aufbewahrungsrichtlinie löschen?",
  "retention.deleteBody":
    "Damit entfällt die Regel für {scope} vollständig, und in diesem Bereich verfällt nichts mehr. Um die Regel zu pausieren und ihre Frist zu behalten, deaktiviere sie stattdessen.",
  "retention.duplicateScope":
    "Für diesen Bereich existiert bereits eine Richtlinie — jeder Bereich trägt höchstens eine Regel. Bearbeite stattdessen die vorhandene Zeile.",
  "retention.empty":
    "Noch keine Aufbewahrungsrichtlinie — in dieser Installation verfällt nichts.",
  "retention.effectActing": "Läuft nächtlich",
  "retention.effectSuppressed": "Durch Nur-Aufbewahren zurückgehalten",
  "retention.effectDisabled": "Deaktiviert",
  "retention.suppressedWhy":
    "Aktiv, aber der Nur-Aufbewahren-Modus hält sie zurück: diese Regel vernichtet Daten und greift erst, wenn der Modus abgeschaltet wird.",
  "retention.disabledWhy":
    "Abgeschaltet und behalten — die Frist bleibt gespeichert, und in diesem Bereich verfällt währenddessen nichts.",
  "retention.actionArchive": "Archivieren",
  "retention.actionAnonymize": "Anonymisieren",
  "retention.actionErase": "Löschen",
  "retention.scopeLeadUnconverted": "Nicht konvertierte Leads",
  "retention.scopeActivity": "Alle erfassten Aktivitäten",
  "retention.scopeActivityTranscript": "Gesprächstranskripte",
  "retention.scopePersonNoConsentNoDeal":
    "Personen ohne Einwilligung und ohne Deal",
  "retention.scopeDealLost": "Verlorene Deals",
  "retention.scopeDealWon": "Gewonnene Deals",
  "retention.scopeAiCallPayloadContent": "KI-Aufruf-Nutzdaten",

  "settings.pipelines": "Pipelines",
  "settings.pipelinesReadOnly":
    "Nur-Lese-Ansicht — du darfst Pipelines und ihre Phasen nicht ändern.",
  "settings.pipelinesSub":
    "Die Phasen, die ein Deal durchläuft — eine Leiter je Pipeline.",
  "pipeline.new": "Neue Pipeline",
  "pipeline.edit": "Pipeline bearbeiten",
  "pipeline.name": "Name",
  "pipeline.default": "Standard",
  "pipeline.notDefault": "Kein Standard",
  "pipeline.position": "Position",
  "stage.new": "Neue Phase",
  "stage.edit": "Phase bearbeiten",
  "stage.name": "Name",
  "stage.semantic": "Semantik",
  "stage.winProb": "Gewinnwahrscheinlichkeit",
  "stage.semOpen": "Offen",
  "stage.semWon": "Gewonnen",
  "stage.semLost": "Verloren",
  "stage.remove": "Entfernen",
  "stage.removeConfirm": "Phase entfernen",
  "stage.removeTitle": "Diese Phase entfernen?",
  "stage.removeBody":
    "„{name}“ verlässt die Pipeline, die nachfolgenden Phasen rücken auf. Frühere Phasenwechsel bleiben lesbar. Deals, die noch darauf stehen, müssen zuerst umziehen.",

  "ob.url": "Website",
  "ob.urlScheme": "https://",
  "ob.back": "Zurück",
  "ob.restoring": "Deine Einrichtung wird wiederhergestellt…",
  "ob.readManual": "Erzähl es mir selbst",
  "ob.coreIntroTitle": "Zuerst muss ich dein rechtliches Unternehmen kennen.",
  "ob.coreIntroBody":
    "Ich brauche Firmenname, Anschrift und USt- oder Registernummer. Dann lerne ich, was ihr verkauft und an wen.",
  "ob.coreLegalKicker": "Ich beginne mit der rechtlichen Identität",
  "ob.corePathLabel": "Was ich lerne",
  "ob.corePathLegal": "Rechtliche Identität",
  "ob.corePathOffer": "Angebot",
  "ob.corePathCustomer": "Kunden",
  "ob.coreReadingPage": "Ich lese gerade",
  "ob.coreWebsiteTitle": "Welche Website soll ich lesen?",
  "ob.coreWebsiteBody":
    "Ich lese zuerst das Impressum, dann Produkte, Kunden und Positionierung.",
  "ob.corePreparing": "Ich bereite das Einlesen von {host} vor",
  "ob.coreLegalReading": "Ich lese die rechtliche Identität auf {host}",
  "ob.coreLegalReadingBody":
    "Ich suche Impressum, Anschrift und Register- oder USt-Nummer. Ungenanntes bleibt leer.",
  "ob.coreBusinessReading": "Ich lerne, wie das Geschäft funktioniert",
  "ob.coreBusinessReadingBody":
    "Ich verbinde Produkte, Kunden und Positionierung mit dem genauen öffentlichen Text, der sie belegt.",
  "ob.coreReady": "Ich habe {count} belegte Firmendaten gefunden",
  "ob.corePartial": "Ich habe {count} nützliche Angaben gefunden — mit Lücken",
  "ob.coreReadyBody":
    "Noch nichts gespeichert. Prüf zuerst die rechtliche Identität, dann das Angebot.",
  "ob.coreDeferredBody": "Ich setze das Einlesen automatisch fort.",
  "ob.coreFailedBody":
    "Ich konnte diese Website nicht sicher lesen und habe gestoppt statt zu raten. Sag es mir selbst.",
  "ob.coreFindingsTitle": "Was ich gefunden habe und belegen kann",
  "ob.coreFindingsBody":
    "Zu jedem Wert gehört der öffentliche Wortlaut. Unbelegtes lasse ich leer.",
  "ob.ai.identity": "Hallo, ich bin Margince",
  "ob.ai.role": "Deine KI für Firmenrecherche",
  "ob.ai.speaker": "M",
  "ob.ai.speakerName": "Margince",
  "ob.ai.ready": "Ich bin bereit für die Recherche",
  "ob.ai.configured": "Konfigurierte KI",
  "ob.ai.modelsUsed": "In dieser Aufgabe verwendete Modelle",
  "ob.ai.route": "Aufgabe · Stufe · Provider",
  "ob.ai.calls": "KI-Aufrufe",
  "ob.ai.tokens": "Tokens",
  "ob.ai.latency": "Modell-Latenz",
  "ob.ai.estimatedCost": "Geschätzte Provider-Kosten",
  "ob.ai.partialEstimate": "Teilbetrag · nicht bepreiste Nutzung vorhanden",
  "ob.ai.awaitingModel": "Nach meinem ersten Modellaufruf sichtbar",
  "ob.ai.notAvailableYet": "Noch nicht verfügbar",
  "ob.ai.runtimeUnavailable": "Laufzeitdetails nicht verfügbar",
  // Die Laufzeit-Offenlegung ist ein Chip zum Öffnen, kein Dauerband: Kosten
  // stehen da, WÄHREND sie entstehen, aber wer entscheidet, ob eine
  // Rechtsform stimmt, soll dafür keine Abrechnungstabelle lesen müssen.
  "ob.ai.runtimeChip": "Was antwortet, und was es kostet",
  "ob.ai.answeringNow": "Was gerade antwortet",
  "ob.ai.runScope": "Nur dieser Lauf. Das ganze Protokoll: Einstellungen → KI.",
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
  "ob.ai.summary.hybrid_one": "1 Modell, teils Cloud, teils lokal",
  "ob.ai.summary.hybrid_other": "{count} Modelle, teils Cloud, teils lokal",
  "ob.ai.summary.development_one": "1 Modell, Entwicklungsmodus",
  "ob.ai.summary.development_other": "{count} Modelle, Entwicklungsmodus",
  "ob.ai.summary.none": "Noch kein Modell konfiguriert",
  "ob.ai.summaryProviders_one": "1 Provider konfiguriert",
  "ob.ai.summaryProviders_other": "{count} Provider konfiguriert",
  "ob.ai.readFirst": "Starte zuerst die Firmeneinrichtung.",
  "ob.ai.liveArtifact": "Lebendes, prüfbares Ergebnis",
  "ob.ai.companyKnowledge": "Was ich über dein Unternehmen verstehe",
  "ob.ai.companyKnowledgeBody":
    "Website-Belege bleiben von unserem Gespräch getrennt. Du entscheidest, was Firmenkontext wird.",
  "ob.ai.companyKnowledgeManualBody":
    "Deine Antworten und meine Vorschläge bleiben hier bearbeitbar. Du entscheidest, was Firmenkontext wird.",
  "ob.ai.askPlaceholder":
    "Frag mich zu einem Fund, korrigiere ein Detail oder sag mir, was fehlt…",
  "ob.ai.send": "An Margince senden",
  "ob.ai.reviewBoundary":
    "Ich kann hier Änderungen vorschlagen. Ich übernehme sie erst nach deiner Freigabe in den Entwurf.",
  "ob.ai.confirmBoundary":
    "Nichts wird Firmenkontext, bevor du diesen Entwurf bestätigst.",
  "ob.ai.confirmCompany": "Firma bestätigen und speichern",
  "ob.ai.thinking": "Ich prüfe das Dossier und bereite eine Antwort vor…",
  "ob.ai.suggestedChanges": "Vorgeschlagene Änderungen am Entwurf",
  "ob.ai.applyChanges": "In meinen Entwurf übernehmen",
  "ob.ai.applied": "In Entwurf übernommen",
  "ob.ai.finding_one": "belegter Fund",
  "ob.ai.finding_other": "belegte Funde",
  "ob.continueManual": "Erzähl es mir stattdessen",
  "ob.readStatus.queued": "Ich bereite mich vor",
  "ob.readStatus.deferred": "Ich warte auf KI-Budget",
  "ob.readStatus.reading": "Ich lese gerade",
  "ob.readStatus.ready": "Ich bin mit dem Lesen fertig",
  "ob.readStatus.partial": "Ich bin fertig — mit einigen Lücken",
  "ob.readStatus.failed": "Ich brauche deine Hilfe",
  "ob.readStatus.confirmed": "Ich habe deine Auswahl gespeichert",
  "ob.readStatus.abandoned": "Ich habe aufgehört",
  "ob.pagesRead": "Seiten, die ich gelesen habe",
  "ob.legalEntitiesFound": "rechtliche Einheiten, die ich gefunden habe",
  "ob.coverageDetails": "Was ich abgedeckt und nicht lesen konnte",
  "ob.legalFoundTitle": "Rechtliche Einheiten, die ich gefunden habe",
  "ob.legalFoundBody":
    "Jeder Block behält Name, Anschrift und Register- oder USt-Nummer. Deine wählst du in der Prüfung.",
  "ob.legalEntity": "Rechtliche Einheit",
  "ob.confirmWebsite":
    "Ich habe diese Angaben mit {count} öffentlichen Seiten belegt. Änderungen werden deine Aussage; unveränderte Werte behalten ihre Belege.",
  "ob.confirmManual":
    "Du hast mir diese Angaben direkt gegeben, deshalb speichere ich sie als menschliche Aussagen.",
  "ob.legalTitle": "Welche rechtliche Einheit soll ich verwenden?",
  "ob.legalSub":
    "Ich habe mehrere Einheiten im Impressum gefunden. Wähle deine und ich trage ihre Daten ein.",
  "ob.factsTitle": "Weitere Fakten, die ich gefunden habe",
  "ob.factsSelected": "{selected} von {total} ausgewählt",
  "ob.factsSub":
    "Wähle ab, was nicht Teil des Firmenkontexts werden soll — bis zu 100 Angaben können ausgewählt sein.",
  "ob.nowUnderstands": "Ich verstehe jetzt",
  "ob.contextReady":
    "Ich nutze diesen Kontext für Entwürfe, Suche, Agenten und Voice DNA — mit Herkunft.",

  // Keine Schrittzahl: wie viele Stationen ein Leser bekommt, entscheidet die
  // Leiste, also gehört die Zählung zu ob.conv.scene.step, das sie von dort
  // liest. Eine hier hineingeschriebene Summe kann nur falsch werden.
  "ob.s1.kick": "Bestätigen",
  "ob.s1.title": "Prüfe, was ich über dein Unternehmen gelernt habe",
  "ob.s1.sub":
    "Ich habe nur ausgefüllt, was ich auf deiner Website belegen konnte. Bitte korrigiere, was nicht stimmt.",
  "ob.s1.urlPlaceholder": "deinefirma.de",
  "ob.s1.identityLabel": "Rechtliche Organisation",
  "ob.s1.offerLabel": "Produkte und Angebot",
  "ob.s1.customerLabel": "Kunde",
  "ob.s1.salesLabel": "Positionierung und Vertriebskontext",
  "ob.s1.fieldRequired": "Pflichtfeld.",
  "ob.s1.requiredMissing": "Diese Felder fehlen noch: {fields}",
  "ob.s1.saving": "Wird gespeichert…",
  "ob.s1.saveFailed": "Deine Firma konnte nicht gespeichert werden",
  "ob.s1.savedNote":
    "In deiner Organisation gespeichert. Ändere hier etwas und geh weiter — dann wird erneut gespeichert.",
  "ob.readGo": "Meine Website einlesen",
  "ob.urlWillRead": "Ich lese {host}",
  "ob.readFromSite": "von der Website gelesen",
  "ob.failTitle": "Ich konnte von dieser Website nicht genug lesen",

  "ob.manualChapterLegal": "Deine rechtliche Organisation",
  "ob.manualChapterOffer": "Produkte und Angebot",
  "ob.manualChapterCustomer": "Idealkunde",
  "ob.manualChapterSales": "Wie du verkaufst",
  "ob.manualNext": "Nächste Frage",
  "ob.manualLater": "Später ergänzen",
  "ob.manualReview": "Antworten prüfen",
  "ob.manualRequired": "Erforderlich für ein nutzbares Firmenprofil",
  "ob.manualOptional": "Optional — leer lassen und später ergänzen",
  "ob.manual.display_name":
    "Unter welchem Namen kennen Kunden dein Unternehmen?",
  "ob.manual.display_nameHint":
    "Nutze den vertrauten Firmen- oder Markennamen, der in Margince erscheinen soll.",
  "ob.manual.legal_name":
    "Wie lautet der vollständige eingetragene Firmenname?",
  "ob.manual.legal_nameHint":
    "Inklusive Rechtsform wie GmbH, AG, Ltd oder Inc. Ergänze ihn später, wenn das nicht zutrifft.",
  "ob.manual.registered_address": "Wie lautet die eingetragene Anschrift?",
  "ob.manual.registered_addressHint":
    "Nutze die offizielle Anschrift aus Handelsregister oder Impressum.",
  "ob.manual.register_vat": "Wie lauten Register- und USt-IdNr./UID?",
  "ob.manual.register_vatHint":
    "Trage die Kennungen exakt wie ausgegeben ein. Leer lassen, wenn keine zutrifft.",
  "ob.manual.legal_form": "Welche Rechtsform hat das Unternehmen?",
  "ob.manual.legal_formHint":
    "Die Form, wie sie im Register steht, etwa GmbH, AG oder Ltd.",
  "ob.manual.register_court": "Welches Gericht führt den Registereintrag?",
  "ob.manual.register_courtHint":
    "Das im Impressum genannte Gericht, etwa Amtsgericht Charlottenburg.",
  "ob.manual.register_number": "Wie lautet die Handelsregisternummer?",
  "ob.manual.register_numberHint":
    "Nur der Registereintrag, etwa HRB 12345 B. Die USt-IdNr. gehört in das Feld darüber.",
  "ob.manual.industry": "In welcher Branche ist das Unternehmen tätig?",
  "ob.manual.industryHint":
    "Wähle die Beschreibung, die deine Kunden sofort verstehen würden.",
  "ob.manual.history": "Welche Firmengeschichte sollte Margince kennen?",
  "ob.manual.historyHint":
    "Zum Beispiel Gründungsjahr, Ursprung oder eine wichtige Veränderung des Geschäfts.",
  "ob.manual.offer_summary": "Welche Produkte oder Leistungen verkauft ihr?",
  "ob.manual.offer_summaryHint":
    "Ein oder zwei konkrete Sätze genügen. Diese Erklärung nutzt Margince für euer Geschäft.",
  "ob.manual.value_proposition": "Welches Ergebnis schafft das Angebot?",
  "ob.manual.value_propositionHint":
    "Erkläre den Kundennutzen, nicht nur die Produktfunktionen.",
  "ob.manual.usp": "Warum entscheiden sich Kunden für euch?",
  "ob.manual.uspHint":
    "Nenne den wichtigsten echten Unterschied zu den Alternativen.",
  "ob.manual.icp": "Wer ist euer Idealkunde?",
  "ob.manual.icpHint":
    "Beschreibe Unternehmen oder Personen mit dem größten Nutzen — Größe, Branche, Situation oder Region.",
  "ob.manual.buying_center": "Wer prüft, kauft oder genehmigt den Kauf?",
  "ob.manual.buying_centerHint":
    "Nenne die typischen Rollen und wer am Ende entscheidet.",
  "ob.manual.customer_pains":
    "Mit welchen Problemen kommen diese Kunden zu euch?",
  "ob.manual.customer_painsHint":
    "Nutze die Probleme, wie Kunden sie selbst beschreiben würden.",
  "ob.manual.desired_outcomes": "Was möchten sie erreichen?",
  "ob.manual.desired_outcomesHint":
    "Beschreibe die praktischen oder geschäftlichen Ergebnisse, die ihnen wichtig sind.",
  "ob.manual.buying_intents": "Was signalisiert üblicherweise Kaufinteresse?",
  "ob.manual.buying_intentsHint":
    "Zum Beispiel eine neue Initiative, Einstellungen, eine Frist oder ein operatives Problem.",
  "ob.manual.common_objections": "Welche Einwände hört ihr am häufigsten?",
  "ob.manual.common_objectionsHint":
    "Nenne Bedenken, die einen Kauf regelmäßig verzögern oder verhindern.",
  "ob.manual.sales_motion": "Wie läuft ein typischer Verkauf ab?",
  "ob.manual.sales_motionHint":
    "Beschreibe den Weg vom ersten Gespräch zur Entscheidung, einschließlich Test oder Einkauf, wenn relevant.",

  "ob.field.display_name": "Firmenname",
  "ob.field.offer_summary": "Was verkaufst du?",
  "ob.field.icp": "Idealkunde",
  "ob.field.buying_center": "Wer kauft",
  "ob.field.value_proposition": "Nutzenversprechen",
  "ob.field.usp": "Was dich unterscheidet",
  "ob.field.customer_pains": "Kundenprobleme",
  "ob.field.desired_outcomes": "Gewünschte Ergebnisse",
  "ob.field.buying_intents": "Kaufanlässe",
  "ob.field.common_objections": "Häufige Einwände",
  "ob.field.sales_motion": "Vertriebsmodell",
  "ob.field.legal_name": "Eingetragener Firmenname",
  "ob.field.registered_address": "Anschrift",
  "ob.field.register_vat": "Register / USt-ID",
  "ob.field.legal_form": "Rechtsform",
  "ob.field.register_court": "Registergericht",
  "ob.field.register_number": "Registernummer",
  "ob.field.industry": "Branche",
  "ob.field.history": "Firmengeschichte",

  "ob.s3.title": "Sieh, was du gebaut hast —",
  "ob.s3.titleEm": "ganz ohne Anbindung.",
  "ob.s3.sub":
    "Deine Organisation kennt dein Geschäft und deine Stimme. Verbinde dein Postfach, es füllt sich.",
  "ob.s3.subNoVoice":
    "Deine Organisation kennt dein Geschäft. Verbinde dein Postfach, es füllt sich.",
  "ob.s3.cardProfile": "Geschäftsprofil",
  "ob.s3.cardProfileBody":
    "Bestätigt und auf deiner Firmenseite gespeichert. Gelesene Felder behalten ihre Quelle.",
  "ob.s3.cardProfileSkippedBody":
    "Gelesen, aber nicht gespeichert: du hast Bestätigen übersprungen. Geh zurück und bestätige.",
  "ob.s3.cardVoice": "Deine Schreibstimme",
  "ob.s3.cardVoiceBody":
    "Gebaut aus dem Korpus, den du uns gerade gegeben hast. Entwürfe klingen ab Tag eins nach dir.",
  "ob.s3.cardVoiceSkippedBody":
    "Übersprungen — Entwürfe nutzen eine neutrale Stimme. Deine baust du in den Einstellungen.",
  "ob.s3.cardPipeline": "Vertriebs-Pipeline",
  "ob.s3.cardPipelineBody":
    "Die Standard-B2B-Vorlage mit 7 Stufen, auf deine Branche gestimmt. Leer, bis du verbindest.",
  "ob.s3.cardDraft": "Ein Beispiel-Entwurf, in deiner Stimme",
  "ob.s3.cardDraftExample": "Ein Beispiel-Entwurf",
  "ob.s3.cardDraftBody": "Sieh ihn unten.",
  "ob.s3.originLabel": "Woher diese Pipeline kommt",
  "ob.s3.originBody":
    "Die Standard-B2B-Vorlage, aus dem Read auf deine Branche gestimmt. Leer, bis du verbindest. Du gibst frei, was ein Deal wird.",
  "ob.s3.stillNothing":
    "Noch immer nichts verbunden. Du bestimmst, wann sich das ändert.",

  "ob.s4.provGoogle": "Google",
  "ob.s4.provMicrosoft": "Microsoft",
  "ob.s4.provImap": "Beliebiges Postfach (IMAP)",
  "ob.s4.microsoftBtn": "Zugriff auf mein Microsoft erlauben",
  "ob.s4.microsoftHint":
    "Nur-Lese-Zugriff auf E-Mails. Du kannst die Verbindung jederzeit in den Einstellungen trennen.",
  "ob.s4.microsoftUnverified":
    "Eventuell erscheint ein Hinweis „nicht verifizierte App“ — das ist diese selbstgehostete Installation, kein Dritter.",
  "ob.s4.microsoftFailed":
    "Die Microsoft-Verbindung wurde nicht abgeschlossen.",
  "ob.s4.connectOkTitle": "Verbunden",
  "ob.s4.connectOkBody":
    "Dein Postfach ist verknüpft. Die Erfassung startet beim nächsten Sync.",
  "ob.s4.connectVerifying": "Verbindung wird bestätigt…",
  "ob.s4.connectLive": "Aktiv und erfassend",
  "ob.s4.connectConfirmFailed": "Die Verbindung konnte nicht bestätigt werden.",
  "ob.s4.connectRetry":
    "Öffne Einstellungen → Verbindungen, um es erneut zu versuchen.",
  "ob.s4.connectDenied":
    "Du hast den Zugriff abgelehnt — es wurde nichts verbunden.",
  "ob.s4.googleBtn": "Zugriff auf mein Gmail erlauben",
  "ob.s4.googleHint":
    "Liest deine Mails und sendet nur, was du freigibst. Du bestätigst bei Google und kannst jederzeit trennen.",
  "ob.s4.googleUnverified":
    "Zeigt Google „nicht verifizierte App“, wähl Erweitert → Fortfahren. Ohne deine Freigabe geht nichts raus.",
  "backfill.title": "E-Mail-Verlauf importieren",
  "backfill.intro":
    "Wähle, wie weit zurück importiert wird. Umfang und geschätzte Kosten siehst du vor dem Start — du kannst diesen Schritt auch überspringen.",
  "backfill.windowLabel": "Import-Zeitraum",
  "backfill.window3m": "3 Monate",
  "backfill.window6m": "6 Monate",
  "backfill.window12m": "12 Monate",
  "backfill.window24m": "2 Jahre",
  "backfill.window60m": "5 Jahre",
  "backfill.previewLoading": "Postfach wird gezählt…",
  "backfill.estimateMessages": "Nachrichten in diesem Zeitraum:",
  "backfill.estimateCost": "Geschätzte KI-Kosten:",
  "backfill.estimateNote":
    "Eine Schätzung, keine Rechnung — der tatsächliche Verbrauch wird laufend gemessen und angezeigt.",
  "backfill.startCta": "Import starten",
  "backfill.starting": "Wird gestartet…",
  "backfill.skip": "Verlaufs-Import überspringen",
  "backfill.skippedNote":
    "Kein Verlauf importiert. Neue Mails werden ab jetzt trotzdem erfasst — der Import lässt sich später in den Einstellungen starten.",
  "backfill.loading": "Import-Status wird geprüft…",
  "backfill.statusUnavailable":
    "Der Import-Status ist gerade nicht lesbar — die Erfassung selbst läuft weiter.",
  "backfill.queuedTitle": "Import eingereiht",
  "backfill.runningTitle": "E-Mail-Verlauf wird importiert",
  "backfill.doneTitle": "Verlaufs-Import abgeschlossen",
  "backfill.errorTitle": "Der Import hat ein Problem",
  "backfill.cancelledTitle": "Import abgebrochen",
  "backfill.progressLabel": "Import-Fortschritt",
  "backfill.countScanned": "Nachrichten durchsucht",
  "backfill.statEmails": "E-Mails erfasst",
  "backfill.statPeople": "Personen",
  "backfill.statCompanies": "Firmen zu prüfen",
  "backfill.errorNote":
    "Er versucht es selbstständig erneut; alles bisher Erfasste bleibt erhalten.",
  "backfill.cancel": "Import stoppen",
  "backfill.cancelledNote": "Gestoppt. Alles bisher Erfasste bleibt erhalten.",
  "backfill.unsupportedNote":
    "Dieser Postfachtyp kann nicht rückwirkend importiert werden — ab jetzt werden nur neue E-Mails erfasst.",
  "backfill.narrowingNote":
    "Für dieses Postfach lief bereits ein größerer Zeitraum; der Import-Zeitraum kann nur erweitert, nicht verkleinert werden.",
  "backfill.staleUpdated":
    "Zuletzt aktualisiert vor {duration} — kein aktueller Fortschritt.",

  // Connected inboxes (Einstellungen → Verbindungen).
  // Die Einheiten dieser Installation, auf der Einstellungsseite, die bereits
  // die Art von Zugangsdaten trägt, mit der die jeweilige Einheit konfiguriert
  // wird.
  "extUnits.open": "Öffnen",
  "extUnits.openNamed": "Seite {name} öffnen",
  "extUnits.user.title": "Deine weiteren Konten",
  "extUnits.user.sub":
    "Konten, die diese Installation in deinem Namen verbinden kann. Jedes gehört dir allein — niemand sonst sieht es, und ein Trennen betrifft nur dich.",
  "extUnits.workspace.title": "Erweiterungen der Installation",
  "extUnits.workspace.sub":
    "Erweiterungen, die diese Installation mit einem gemeinsamen Zugang betreibt. Was du hier einstellst, gilt für alle.",

  "connectors.title": "Verbundene Postfächer",
  // Die dauerhafte Nacht-Vollmacht der Nutzerin — eine Frage, gestellt beim
  // Postfach-Verbinden im Onboarding und noch einmal in den Einstellungen.
  "overnightGrant.title": "Vorbereitung über Nacht",
  "overnightGrant.sub":
    "Margince arbeitet nachts deine Deals durch und hat deinen Morgen fertig, wenn du kommst. Es handelt als du, sieht nur was du sehen darfst, und du kannst es jederzeit stoppen.",
  "overnightGrant.label":
    "Margince darf meinen Morgen-Überblick über Nacht vorbereiten",
  "overnightGrant.help":
    "Es liest deine Deals und E-Mails, um zu ordnen, was heute wichtig ist. Es verschickt nie etwas von allein — alles, was nach außen geht, wartet auf deine Freigabe.",
  "overnightGrant.danger":
    "ACHTUNG: Ohne diese Freigabe bleiben dein Morgen-Überblick, deine Arbeitsliste und dein Wochenrückblick leer. Das sind die Bildschirme, mit denen Margince startet — der größte Teil des Produkts wirkt dann, als würde er nicht funktionieren.",
  "overnightGrant.saveFailed":
    "Deine Antwort auf die Nacht-Frage konnte nicht gespeichert werden. Alles andere ist verbunden — stelle es unter Einstellungen → Verbindungen ein, sobald du drin bist.",
  "overnightGrant.renew":
    "Du hast zugestimmt, aber die Vollmacht, unter der Margince gearbeitet hat, ist abgelaufen. Schalte die Option aus und wieder ein, um sie zu erneuern — bis dahin wird dein Überblick nicht vorbereitet.",
  "overnightGrant.renewScope":
    "Du hast zugestimmt, aber Margince kann inzwischen mehr, und die erteilte Vollmacht deckt die neue Arbeit nicht ab. Schalte die Option aus und wieder ein, um sie zu erweitern — bis dahin wird dein Überblick nicht vorbereitet.",
  "aiHealth.title": "Modell-Lanes",
  "aiHealth.sub":
    "Ob jede Modellstufe antwortet. Eine ausgefallene Lane und eine, die nur vorsichtig ist, sehen überall sonst gleich aus — erfasste Post bleibt in beiden Fällen zurückgehalten.",
  "aiHealth.noCalls":
    "in den letzten {hours} Stunde(n) wurde kein Modell aufgerufen",
  "aiHealth.colTier": "Stufe",
  "aiHealth.colState": "Zustand",
  "aiHealth.colCalls": "Letzte {hours} Std.",
  "aiHealth.colLatency": "Median",
  "aiHealth.colLast": "Letzte Antwort",
  "aiHealth.answering": "Antwortet",
  "aiHealth.notAnswering": "Antwortet nicht",
  "aiHealth.callCounts": "{calls} Aufrufe, {failures} fehlgeschlagen",
  "aiHealth.ms": "{ms} ms",
  "heldThreads.title": "Vom Team zurückgehalten",
  "heldThreads.sub":
    "Konversationen, die Ihr Postfach zurückhält. Wenn Sie eine freigeben, können alle Kolleginnen und Kollegen sie lesen; niemand sonst kann Ihre freigeben.",
  "heldThreads.empty": "Ihr Postfach hält derzeit nichts zurück",
  "heldThreads.colThread": "Konversation",
  "heldThreads.colWhy": "Warum zurückgehalten",
  "heldThreads.colWhen": "Eingegangen",
  "heldThreads.colActions": "Aktionen",
  "heldThreads.release": "Mit dem Team teilen",
  "heldThreads.released": "Mit dem Team geteilt",
  "heldThreads.noSubject": "die ursprüngliche Nachricht ist gelöscht",
  "heldThreads.blankSubject": "kein Betreff",
  "heldThreads.nothingToShare":
    "Es ist keine Nachricht mehr da, die geteilt werden könnte — die erste Nachricht dieser Konversation wurde gelöscht. Die Zurückhaltung bleibt, damit eine spätere Antwort nicht offen eintrifft.",
  "heldThreads.pending": "Wartet auf Beurteilung",
  "heldThreads.attempts": "{count}-mal angefragt",
  "heldThreads.backlogStalled":
    "Zu {count} Threads wurde mehrfach angefragt, ohne Antwort. Solange das anhält, bleibt Post zurückgehalten — nichts geht verloren, und es löst sich von selbst, sobald die Klassifikation wieder antwortet.",
  "heldThreads.heldByOthers":
    "Weiterhin zurückgehalten: {count} weiteres Postfach hat diese Nachricht ebenfalls importiert und nicht freigegeben. Eine Konversation wird erst geöffnet, wenn alle Empfänger zustimmen.",
  "heldThreads.kind.legal": "Rechtliches",
  "heldThreads.kind.financialCorporate": "Unternehmensfinanzen",
  "heldThreads.kind.personnel": "Personal",
  "heldThreads.kind.personal": "Privat",
  "heldThreads.kind.securityIncident": "Sicherheitsvorfall",
  "heldThreads.kind.explicitlyConfidential": "Als vertraulich markiert",
  "senders.title": "Absender",
  "senders.sub":
    "Was über jede Adresse entschieden wurde, die Ihr Postfach eingebracht hat — und Ihre eigene Antwort, wo Sie eine gegeben haben. Nur Ihre Absender; Kolleginnen und Kollegen sehen diese Liste nie.",
  "senders.emptyTitle": "Noch nichts entschieden",
  "senders.emptyBody":
    "Sobald Ihr Postfach E-Mails eingebracht hat, steht hier jeder Absender mit dem, was aus ihm wurde.",
  "senders.colSender": "Absender",
  "senders.colDecision": "Entschieden",
  "senders.colRecord": "Kontakt",
  "senders.colActions": "Aktionen",
  "senders.recordYes": "Ja",
  "senders.recordNo": "Nein",
  "senders.byYou": "— von Ihnen entschieden",
  "senders.deletesOn": "Älteste Nachricht wird am {date} gelöscht",
  "senders.markBusiness": "Geschäftlich",
  "senders.keepOut": "Aussperren",
  "senders.withdraw": "Zurücknehmen",
  "senders.keepOutTitle": "Diesen Absender dauerhaft aussperren?",
  "senders.keepOutBody":
    "Es wird kein Kontakt angelegt, und die E-Mails, die dieser Absender bereits in Ihr Postfach eingebracht hat, werden vernichtet. E-Mails, die auch eine Kollegin importiert hat, bleiben ihr erhalten.",
  "senders.keepOutConfirm": "Aussperren und vernichten",
  "senders.kind.person": "Eine Person",
  "senders.kind.roleMailbox": "Ein Funktionspostfach",
  "senders.kind.organizationSender": "Eine Organisation",
  "senders.kind.newsletter": "Ein Newsletter",
  "senders.kind.transactional": "Ein automatisiertes Werkzeug",
  "senders.kind.spam": "Spam",
  "senders.kind.personal": "Privat",
  "senders.kind.advisor": "Beratung",
  "senders.kind.business": "Geschäftlich",
  "senders.kind.keptOut": "Ausgesperrt",
  "senders.kind.undecided": "Noch nicht entschieden",
  "mailSharing.title": "E-Mail-Freigabe",
  "mailSharing.sub":
    "Erfasste E-Mails sind für alle Kolleginnen und Kollegen lesbar, die den Kontakt sehen können. Standardmäßig eingeschaltet — das macht die Pipeline gemeinsam.",
  "mailSharing.label": "Erfasste E-Mails im Team teilen",
  "mailSharing.help":
    "Einzelne Nachrichten lassen sich nachträglich einschränken, Adressen und Domains vorab ausschließen.",
  "mailSharing.danger":
    "ACHTUNG: E-Mail-Freigabe ausschalten macht die Nutzung des CRM schwierig. Neue E-Mails sind dann nur noch für die Beteiligten der jeweiligen Nachricht sichtbar.",
  "mailSharing.sharedPosture.label": "Postfächern erlauben, sofort zu teilen",
  "mailSharing.sharedPosture.help":
    "Erlaubt Kolleginnen und Kollegen, das eigene Postfach auf „geteilt“ zu stellen — eine erfasste Nachricht ist dann für das Team lesbar, sobald sie ankommt, bevor sie eingestuft wurde. Standardmäßig aus.",
  "mailSharing.sharedPosture.warning":
    "Das Postfach von Beschäftigten in ein gemeinsames CRM einzulesen, ist in Deutschland und Österreich Gegenstand einer Betriebsvereinbarung. Wer dies einschaltet, erklärt, dass Ihre Organisation eine solche hat. Margince prüft das nicht.",
  "mailSharing.save": "Speichern",
  "connectors.originLabel": "Adresse in versendeten Links",
  "connectors.originReachable": "Antwortet",
  "connectors.originUnreachable": "Antwortet nicht",
  "connectors.originUnchecked": "Noch nicht gepr\u00fcft",
  "connectors.sub":
    "Postfächer, die dein CRM automatisch füllen. Trenne eines bei Bedarf — bereits erfasste Datensätze bleiben.",
  "connectors.loading": "Verbindungen werden geladen…",
  "connectors.loadFailed": "Verbindungen konnten nicht geladen werden.",
  "connectors.empty": "Noch kein Postfach verbunden.",
  "connectors.provGmail": "Gmail",
  "connectors.provGcal": "Google Kalender",
  "connectors.provGraph": "Outlook",
  "connectors.provGraphCal": "Outlook-Kalender",
  "connectors.provImap": "IMAP-Postfach",
  "connectors.statusConnected": "Aktiv",
  "connectors.statusPending": "Ausstehend — noch nicht bestätigt aktiv",
  "connectors.statusReauth": "Neu verbinden nötig",
  "connectors.statusError": "Sync-Fehler",
  "connectors.statusDisconnected": "Getrennt",
  "connectors.cannotSend": "Nur Erfassung — kein Versand",
  "connectors.reconnectToSend":
    "Verbinde dieses Postfach neu, um daraus zu senden. Ein Postfach, das vor der Versandfunktion verbunden wurde, lässt sich nicht nachträglich erweitern — der Anbieter erteilt die Sendeerlaubnis nur bei einer neuen Verbindung.",
  "connectors.lastSynced": "Zuletzt synchronisiert {at}",
  "connectors.neverSynced": "Wartet auf die erste Synchronisierung",
  "connectors.nextCheck": "Nächste Prüfung ~{at}",
  "connectors.polled": "Wird gepollt (kein Push-Abo)",
  "connectors.pushRenewal": "Push-Erneuerung bis {at}",
  "connectors.notConfigured":
    "Die Mail-Erfassung ist in dieser Installation nicht konfiguriert.",
  "connectors.reconnect": "Neu verbinden",
  "connectors.disconnect": "Trennen",
  "connectors.signatureEnrich.label": "Kontaktdaten aus diesem Postfach lesen",
  "connectors.signatureEnrich.followingDefault":
    "Folgt der Einstellung Ihrer Organisation. Wird sie hier geändert, behält dieses Postfach seine eigene Antwort.",
  "connectors.signatureEnrich.ownAnswer":
    "Eigene Antwort dieses Postfachs — bleibt bestehen, was auch immer Ihre Organisation einstellt.",
  "hold.sectionTitle": "Private Korrespondenz",
  "hold.notHeld":
    "E-Mails mit diesem Kontakt folgen der Einstellung Ihres Postfachs.",
  "hold.heldByAddress":
    "Sie behalten E-Mails mit dieser Adresse bei den Beteiligten.",
  "hold.heldByDomain": "Sie behalten E-Mails mit {domain} bei den Beteiligten.",
  "hold.holdAddress": "Privat halten",
  "hold.holdDomain": "Ganz {domain} privat halten",
  "hold.lift": "Aufheben",
  "hold.liftingWidensNothing":
    "Das Aufheben gilt für neue E-Mails. Bereits Zurückgehaltenes bleibt zurückgehalten.",
  "hold.confirmVerb": "Privat halten",
  "hold.confirmTitle": "Diese Korrespondenz privat halten?",
  "hold.confirmAddressBody":
    "E-Mails mit {address} bleiben bei den Beteiligten. Sie werden weiterhin erfasst und sind für Sie lesbar — Kolleginnen und Kollegen sehen sie nicht.",
  "hold.confirmDomainBody":
    "E-Mails mit allen bei {domain}, einschließlich Subdomains, bleiben bei den Beteiligten. Sie werden weiterhin erfasst und sind für Sie lesbar — Kolleginnen und Kollegen sehen sie nicht.",
  "hold.confirmHistoryNote":
    "Das gilt ab jetzt. Bereits erfasste E-Mails behalten ihre bisherige Sichtbarkeit.",
  "captureNotice.whatHappens":
    "Margince liest dieses Postfach und legt ab, was es findet: die Nachrichten, wer daran beteiligt war, sowie die Kontakte und Firmen hinter den Adressen. Anhänge werden mit ihrer Nachricht gespeichert.",
  "captureNotice.whoReads":
    "Ein neues Postfach ist standardmäßig zurückgehalten. Eine Nachricht bleibt bei den Beteiligten, bis eine Einstufung den Verlauf als gewöhnliche geschäftliche Korrespondenz beurteilt — erst dann können Kolleginnen und Kollegen sie lesen. Sie können das Postfach jederzeit so einstellen, dass alles zurückgehalten bleibt.",
  "captureNotice.yourControl":
    "Sie entscheiden pro Absender und pro Verlauf unter Einstellungen → Verbindungen: eine Korrespondenz ganz heraushalten, einen Verlauf mit dem Team teilen oder löschen, was ein Absender eingebracht hat. Hier wird um nichts gebeten — so läuft es ab, damit Sie es vor dem Verbinden wissen.",
  "connectors.mailPosture.label": "Wer E-Mails aus diesem Postfach lesen darf",
  "connectors.mailPosture.classified": "Zurückgehalten bis eingestuft",
  "connectors.mailPosture.held": "Immer zurückgehalten",
  "connectors.mailPosture.shared": "Mit dem Team geteilt",
  "connectors.mailPosture.sharedNeedsAdmin":
    "„Mit dem Team geteilt“ muss eine Administratorin für diese Organisation erlauben.",
  "connectors.mailPosture.help.classified":
    "Eine neue Nachricht bleibt auf die Beteiligten beschränkt, bis eine Einstufung den Verlauf als gewöhnlich beurteilt. Kolleginnen und Kollegen sehen vorher nichts.",
  "connectors.mailPosture.help.held":
    "Eine neue Nachricht bleibt auf die Beteiligten beschränkt, unabhängig von jeder Einstufung. Sie geben einen Verlauf selbst frei, einzeln.",
  "connectors.mailPosture.help.shared":
    "Eine neue Nachricht ist für Kolleginnen und Kollegen lesbar, sobald sie ankommt.",
  "connectors.mailPosture.historyTitle": "Und die bereits erfassten E-Mails?",
  "connectors.mailPosture.historyBody":
    "Diese Antwort gilt für E-Mails, die ab jetzt erfasst werden. Bereits erfasste E-Mails behalten ihre Sichtbarkeit, sofern Sie sie nicht entsprechend einschränken.",
  "connectors.mailPosture.historyConfirm": "Sichtbarkeit ändern",
  "connectors.mailPosture.historyApply":
    "Auch bereits erfasste E-Mails einschränken",
  "connectors.disconnectTitle": "Dieses Postfach trennen?",
  "connectors.disconnectBody":
    "Dies löscht die für dieses Postfach gespeicherte Zugangsdaten. Die Erfassung stoppt sofort; alles bereits Erfasste bleibt in deinem CRM, und beim erneuten Verbinden wird wieder um Erlaubnis gebeten.",
  "connectors.disconnectBodyGoogleNote":
    "Google listet Margince unter Umständen weiterhin unter den Drittanbieter-Zugriffen deines Kontos — entferne es dort, wenn du den Zugriff vollständig widerrufen möchtest.",
  "connectors.disconnectBodyMicrosoftNote":
    "Microsoft listet Margince unter Umständen weiterhin unter den verbundenen Apps deines Kontos — entferne es dort, wenn du den Zugriff vollständig widerrufen möchtest.",
  "connectors.errRateLimited":
    "Der Anbieter drosselt uns. Die Erfassung läuft langsamer als sonst; es geht nichts verloren.",
  "connectors.errUnreachable":
    "Wir konnten den Anbieter nicht erreichen. Wir versuchen es weiter.",
  "connectors.errAuth":
    "Der Anbieter hat unsere Zugangsdaten abgelehnt. Neu verbinden, um fortzufahren.",
  "connectors.errHistoryGone":
    "Der Änderungsverlauf des Anbieters ist abgelaufen. Die nächste Synchronisierung setzt neu an.",
  "connectors.errInternal":
    "Bei uns ist etwas schiefgelaufen. Wir haben gestoppt, statt unvollständige Daten zu erfassen.",
  "connectors.errUnknown":
    "Bei der Erfassung ist ein Problem aufgetreten, das wir noch nicht einordnen können. Wir versuchen es weiter.",

  // Das OAuth-Rückkehrergebnis (Task 2): der Callback landet auf
  // #/settings/connections/{outcome} — ein schließbarer Hinweis, gesteuert
  // von diesem Routensegment.
  "connectors.oauthOk": "Verbunden. Dein Postfach erfasst jetzt.",
  "connectors.oauthDenied":
    "Du hast den Zugriff abgelehnt — es wurde nichts verbunden.",
  "connectors.oauthError":
    "Die Verbindung konnte nicht hergestellt werden — bitte versuch es erneut.",
  // Zwei Fälle, für die "erneut versuchen" falsch wäre: der Anbieter hat die
  // Freigabe abgelehnt, und die API des Anbieters ist für diese Installation
  // nicht aktiviert (das kann keine Nutzeraktion beheben).
  "connectors.oauthRejected":
    "Der Anbieter hat die Verbindung abgelehnt. Bestätige alle angefragten Berechtigungen und versuche es dann erneut.",
  "connectors.oauthMisconfigured":
    "Diese Installation kann die Verbindung noch nicht abschließen — die API des Anbieters ist dafür nicht aktiviert. Ein Administrator muss sie aktivieren; das Server-Log nennt die betroffene API.",
  "connectors.oauthBadClient":
    "Der Anbieter hat die App-Zugangsdaten dieser Installation abgelehnt. Ein Administrator sollte Client-ID und Secret unter Einstellungen → Allgemein prüfen; ein erneuter Verbindungsversuch behebt es nicht von selbst.",
  "connectors.dismissOutcome": "Schließen",

  // Das "Verbindung hinzufügen"-Element (Task 1): ein Button in der Kopfzeile
  // der Karte öffnet einen Dialog mit allen noch verfügbaren Anbietern, jeder
  // mit dem Satz, den er braucht.
  "connectors.addConnection": "Verbindung hinzufügen",
  "connectors.addOpen": "Konto verbinden",
  "connectors.connect": "Verbinden",
  "connectors.connectProvider": "{provider} verbinden",
  "connectors.rosterLabel": "Erfassende Postfächer",
  "connectors.addGmailBrings":
    "Die Mails, die du sendest und empfängst, von Google. Margince kann auch darüber senden.",
  "connectors.addGcalBrings":
    "Dein Google Kalender. Er wird separat von Gmail verbunden.",
  "connectors.addGraphBrings":
    "Die Mails, die du über ein Microsoft-Geschäftskonto sendest und empfängst. Margince kann auch darüber senden.",
  "connectors.addGraphCalBrings":
    "Dein Outlook-Kalender. Er wird getrennt von deiner Outlook-Mail verbunden.",
  "connectors.addImapBrings":
    "Jeder andere Mail-Host, mit einem App-Passwort. Nur Erfassung.",
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
    "Verwende ein App-Passwort. Wir versiegeln es im Credential-Vault und lesen deine Mails nach Zeitplan, bis du trennst — beim Trennen wird es gelöscht.",
  "connectors.imapSubmitCta": "Verbinden",
  "connectors.imapLoginRejected":
    "Das Postfach hat diese Zugangsdaten abgelehnt. Prüfe Server, E-Mail und App-Passwort.",
  "connectors.imapUnreachable": "Der Mailserver konnte nicht erreicht werden.",

  // Das Telegram-Connector-Panel (Task 17, Design §9.1-§9.2): ein Bot
  // verbindet sich für den gesamten Workspace — kein OAuth-Handshake,
  // sondern ein BotFather-Token im selben eingebetteten Formular wie beim
  // IMAP-Connector. Anders als bei den Mail-Anbietern bleibt die Verbindung
  // vor Ort bearbeitbar: das Ersetzen des Tokens läuft über PATCH, nie über
  // ein Trennen.
  "connectors.provTelegram": "Telegram",
  "connectors.telegramTitle": "Telegram-Bot",
  "connectors.telegramSub":
    "Ein Bot empfängt und sendet Nachrichten für die gesamte Organisation.",
  "connectors.telegramNotConfigured":
    "Messaging-Kanäle sind in dieser Installation nicht konfiguriert.",
  "connectors.telegramConnectCta": "Telegram-Bot verbinden",
  "connectors.telegramRosterLabel": "Bot, der Nachrichten überträgt",
  "connectors.telegramEmpty": "Noch kein Bot verbunden.",
  "connectors.telegramEditToken": "Token ersetzen",
  "connectors.telegramDisconnectTitle": "Diesen Bot trennen?",
  "connectors.telegramDisconnectBody":
    "Dies löscht das gespeicherte Token und beendet das Abrufen neuer Nachrichten. Erfassung und Versand stoppen sofort; alles bereits Erfasste bleibt in deinem CRM.",
  "connectors.telegramModalTitle": "Telegram-Bot verbinden",
  "connectors.telegramEditTitle": "Bot-Token ersetzen",
  "connectors.telegramBotToken": "Bot-Token",
  "connectors.telegramBotTokenHint":
    "Füge das Token ein, das BotFather beim Anlegen des Bots ausgegeben hat. Wir versiegeln es im Credential-Vault und zeigen es nie wieder an.",
  "connectors.telegramSubmitCta": "Verbinden",
  "connectors.telegramReplaceCta": "Token ersetzen",
  "connectors.telegramConnectedAs": "Verbunden als @{username}.",

  // Die Consumer-Mail-Liste des Workspace (CAP-PARAM-5).
  "consumerMail.title": "Consumer-Mail-Domains",
  "consumerMail.sub":
    "Mail von einem privaten Postfach legt weiterhin die Person an — nur eben keine Firma. Margince liefert eine Liste dieser Anbieter mit; ergänze, was fehlt, oder nimm eine Domain wieder heraus, die zu Unrecht darauf steht.",
  "consumerMail.addedTitle": "Hier hinzugefügt",
  "consumerMail.addTitle": "Domain hinzufügen",
  "consumerMail.domainLabel": "Domain",
  "consumerMail.domainPlaceholder": "anbieter.example",
  "consumerMail.kindLabel": "Was diese Domain ist",
  "consumerMail.kind.extra": "Consumer-Mail — niemals eine Firma",
  "consumerMail.kind.never":
    "Eine echte Firma — mitgelieferte Liste ignorieren",
  "consumerMail.add": "Hinzufügen",
  "consumerMail.addOpen": "Domain hinzufügen",
  "consumerMail.remove": "Entfernen",
  "consumerMail.none":
    "Nichts ergänzt. Die mitgelieferte Liste entscheidet über jede Domain.",
  "consumerMail.adminOnly":
    "Du hast keine Berechtigung, diese Liste zu ändern.",
  "consumerMail.addOnly":
    "Du kannst Consumer-Mail-Domains ergänzen. Die mitgelieferte Liste übersteuern und Einträge entfernen kann nur ein Admin.",
  "consumerMail.baselineTitle": "Mitgelieferte Liste",
  "consumerMail.baselineCount":
    "Margince liefert {total} bekannte Consumer-Mail-Domains mit.",
  "consumerMail.baselineSearchLabel": "Mitgelieferte Liste durchsuchen",
  "consumerMail.baselinePlaceholder": "gmail.com",
  "consumerMail.baselineNone": "Keine mitgelieferte Domain passt.",
  "consumerMail.baselineMore": "Die ersten {shown} von {matched} Treffern.",

  "blockedDomains.title": "Abgelehnte Domains",
  "blockedDomains.sub":
    "Welchen Domains diese Installation die Firma verweigert und was das jeweils entschieden hat — ein Modellurteil, eine Heuristik oder ein Mensch. Eine Domain wieder zuzulassen stellt die Firmenfrage neu, statt nur eine Markierung zu entfernen.",
  "blockedDomains.listTitle": "Erfasste Entscheidungen",
  "blockedDomains.record": "Entscheidung erfassen",
  "blockedDomains.recordOpen": "Entscheidung erfassen",
  "blockedDomains.domainLabel": "Domain",
  "blockedDomains.domainPlaceholder": "anbieter.example",
  "blockedDomains.admissionLabel": "Entscheidung",
  "blockedDomains.admission.suppressed": "Nie eine Firma",
  "blockedDomains.admission.admitted": "Zugelassen, und zwar dauerhaft",
  "blockedDomains.reasonLabel": "Begründung",
  "blockedDomains.reasonHint":
    "Ein Satz, mit dem jemand später etwas anfangen kann.",
  "blockedDomains.reasonPlaceholder":
    "ein Werkzeug, das wir nutzen — kein Kunde",
  "blockedDomains.save": "Entscheidung speichern",
  "blockedDomains.stored": "Gespeichert: {domain} — {admission}.",
  "blockedDomains.adminOnly":
    "Nur Admin- oder Ops-Plätze dürfen eine Domain-Entscheidung ändern. Die Liste selbst darfst du lesen.",
  "blockedDomains.none":
    "Bisher wurde keiner Domain die Firma verweigert. Urteile über Massenversender landen von selbst hier, ebenso alles, was du selbst ablehnst.",
  "blockedDomains.unit": "Domain-Entscheidungen",
  "blockedDomains.openCompany": "die Firma",
  "blockedDomains.col.domain": "Domain",
  "blockedDomains.col.admission": "Entscheidung",
  "blockedDomains.col.source": "Entschieden von",
  "blockedDomains.col.reason": "Begründung",
  "blockedDomains.col.decided": "Wann",
  "blockedDomains.col.revise": "Ändern",
  "blockedDomains.source.verdict": "Ein Modellurteil",
  "blockedDomains.source.heuristic": "Eine Heuristik",
  "blockedDomains.source.human": "Ein Mensch",
  "blockedDomains.rowAdmit": "Diese zulassen",
  "blockedDomains.rowRefuse": "Diese ablehnen",

  "ob.s4.googleFailed": "Die Google-Verbindung wurde nicht abgeschlossen",
  "ob.s4.imapHost": "IMAP-Host",
  "ob.s4.imapHostPlaceholder": "imap.gmail.com",
  "ob.s4.imapPort": "Port",
  "ob.s4.imapEmail": "E-Mail",
  "ob.s4.imapPassword": "App-Passwort", // NOSONAR: UI translation string, not a credential
  "ob.s4.imapMailbox": "Postfach",
  "ob.s4.imapMax": "Wie viele aktuelle E-Mails",
  "ob.s4.imapHint":
    "Nutz ein App-Passwort. Wir speichern es verschlüsselt, beim Trennen wird es gelöscht.",
  "ob.s4.imapConnect": "Testen und verbinden",
  "ob.s4.connecting": "Sichere Verbindung…",
  "ob.s4.accessToggle": "Welchen Zugriff das gibt",
  "ob.s4.scope1Lead": "Wir lesen — wir müllen nichts voll.",
  "ob.s4.scope1Rest":
    "Deine Post wird automatisch zu Kontakten, Firmen und Aktivitäten.",
  "ob.s4.scope2Lead": "Wir senden nie etwas ohne deine Freigabe.",
  "ob.s4.scope2Rest": "Entwürfe warten auf deine Entscheidung.",
  "ob.s4.scope3Lead": "Deine Daten bleiben in deiner Organisation.",
  "ob.s4.scope3Rest":
    "Own-your-data — jederzeit alles exportieren oder löschen.",
  "ob.s4.scope4Lead": "Trennung mit einem Klick.",
  "ob.s4.scope4Rest": "Das CRM läuft weiter; es hört nur auf zu erfassen.",
  "ob.s4.capturedTitle": "Postfach verbunden",
  "ob.s4.capturedBody":
    "Dein CRM baut sich selbst. Neue Post landet hier, sobald der erste Durchlauf läuft, meist in Minuten.",
  "ob.s4.enterCrm": "Ins CRM",
  "ob.s4.connectFailed": "Dieses Postfach ließ sich nicht verbinden",
  "ob.s4.notNow": "Nicht jetzt",

  "ob.conv.threadLabel": "Einrichtungsgespräch",
  "ob.conv.welcome":
    "Hallo, ich bin Margince. Ich baue dein CRM aus dem, was schon da ist, und zeige jede Quelle.",
  "ob.conv.welcomeMember":
    "Hallo, ich bin Margince. Dein Team ist schon eingerichtet. Zwei kurze Schritte, dann bist du drin.",
  "ob.conv.read.started": "Ich lese jetzt {host}. Ich sage dir, was ich finde.",
  "ob.conv.read.pages": "Bisher gelesene Seiten: {pages}.",
  "ob.conv.read.learnedField": "{field} gelernt: {value}",
  "ob.conv.read.extracting":
    "Das Durchsuchen ist fertig. Jetzt werte ich aus, was die Website über dein Geschäft sagt.",
  "ob.conv.read.warning": "Hinweis: {warning}",
  "ob.conv.read.failed":
    "Ich konnte diese Website nicht lesen. Probiere eine andere URL oder sag es mir direkt.",
  "ob.conv.read.pollFailed":
    "Ich habe die Verbindung beim Lesen verloren. Was ich schon gefunden habe, bleibt erhalten.",
  "ob.conv.read.deferred":
    "Das Einlesen pausiert gerade. Ich setze es automatisch fort.",
  "ob.conv.clarify.entity":
    "Die Website nennt mehr als eine juristische Person. Für welche ist diese Installation?",
  "ob.conv.company.confirmed":
    "Firmenprofil bestätigt. Alles Gespeicherte trägt seine Quelle.",
  "ob.conv.manual.chosen": "Ich tippe es selbst ein.",
  "ob.conv.voice.skipped": "Stimme erstmal überspringen.",
  "ob.conv.voice.uploadAdded": "{name} hinzugefügt.",
  "ob.conv.voice.speakerQuestion":
    "Dieses Transkript hat mehrere Sprecher. Wer davon bist du? Nur deine eigenen Worte zählen.",
  "ob.conv.voice.speakerOptionDetail": "Wörter: {words} · Beiträge: {turns}",
  "ob.conv.voice.guideSpeaker":
    "Rechts wartet eine Sprecherwahl — wähle, welche Person du bist.",
  "ob.conv.voice.speakerFoot": "Deine Wahl gilt nur für diese Datei.",
  "ob.conv.voice.speakerContinue": "Diese Person bin ich",
  "ob.conv.voice.continueSkippedStatus":
    "Erstmal übersprungen — später in den Einstellungen nachholbar.",
  "ob.conv.voice.continueFailedStatus":
    "Deine Materialien sind sicher — versuch es erneut oder mach weiter und komm später zurück.",
  "ob.conv.voice.continueDeferredStatus":
    "Hier ist nichts zu tun — mach weiter, der Rest läuft von selbst.",
  "ob.conv.voice.collectAsk":
    "Schick mir eigene Texte. Gesprächs-Transkripte sind am besten, Dokumente gehen auch.",
  "ob.conv.voice.composer": "Füge hier deinen Text ein",
  "ob.conv.voice.dropHint":
    "Du kannst Dateien auch überall in dieses Gespräch ziehen.",
  "ob.conv.voice.fileSkipped":
    "Ich kann {name} nicht lesen. Ich nehme .txt, .md, .vtt, .srt oder .json.",
  "ob.conv.voice.fileEmpty":
    "In {name} stehen keine Wörter, also wurde nichts gezählt.",
  "ob.conv.voice.reactionTranscript":
    "Behalten: {kept} von {total}. Nur deine Beiträge zählen. Gesprochenes schärft am meisten.",
  "ob.conv.voice.reactionDocument":
    "Gezählte Wörter: {words}. Jedes Wort hier ist deins, also zählen alle.",
  "ob.conv.voice.refusalUnattributed":
    "Das sieht nach einem Gespräch aus, aber ich erkenne deine Wörter nicht. Ich habe nichts gezählt.",
  "ob.conv.voice.refusalSpeaker":
    "Ich konnte diesen Sprecher im Transkript nicht finden. Nichts wurde gezählt.",
  "ob.conv.voice.refusalUnsupported":
    "Ich konnte diese Datei weder als Text noch als Transkript lesen. Nichts wurde gezählt.",
  "ob.conv.voice.ingestFailed":
    "Ich konnte diese Quelle nicht hinzufügen: {detail}",
  "ob.conv.voice.ingestUnexpected":
    "Ich konnte diese Quelle nicht hinzufügen. Versuch es gleich noch einmal.",
  "ob.conv.voice.pasteAdd": "Ja, in meinen Korpus.",
  "ob.conv.voice.pasteDiscard": "Nein, verwerfen.",
  "ob.conv.voice.pasteSource": "Eingefügter Text",
  "ob.conv.voice.buildFloor":
    "Eigene Wörter bisher: {words}. Ich brauche mindestens {min}, bevor ich bauen kann.",
  "ob.conv.voice.buildNudge":
    "Ich habe genug zum Bauen. Ab 4.000 Wörtern wird deine Stimme deutlich schärfer.",
  "ob.conv.voice.buildChip": "Mein Stimmprofil bauen",
  "ob.conv.voice.retryBuild": "Aufbau erneut versuchen",
  "ob.conv.voice.buildPollFailed":
    "Ich habe die Verbindung während des Aufbaus verloren. Deine Texte bleiben erhalten. Versuche den Aufbau erneut.",
  "ob.conv.voice.statusBuilding": "Dein Stimmprofil entsteht",
  "ob.conv.voice.resultTitle":
    "Das ist deine Stimme, in deinen eigenen Worten.",
  "ob.conv.voice.resultLoading": "Ich lade, was der Aufbau gelernt hat.",
  "ob.conv.voice.resultEmpty":
    "Der Aufbau ist fertig, aber es gibt noch nichts zu zeigen. Du kannst ihn in den Einstellungen prüfen.",
  "ob.conv.voice.candidateNote":
    "Diese Version braucht deine Prüfung, bevor sie aktiv wird. Freigeben kannst du sie in den Einstellungen.",
  "ob.conv.voice.artifactTitle": "Stimm-Korpus",
  "ob.conv.voice.artifactBody":
    "Hier zählen nur deine eigenen Wörter. Jede Zahl kommt vom Server, nach dem Sprecher-Filter.",
  "ob.conv.voice.artifactEmpty":
    "Noch nichts gesammelt. Hänge ein Transkript oder einen eigenen Text an.",
  "ob.conv.voice.meterWords": "Eigene Wörter: {words} von {target}",
  "ob.conv.voice.meterBand": "Qualität: {band}",
  "ob.conv.voice.manifestKept": "{kept} von {total} Wörtern behalten",
  "ob.conv.voice.manifestWords": "{words} Wörter",
  "ob.conv.voice.registerMix": "Register: {mix}",
  "ob.conv.voice.stageTitle": "Aufbau-Fortschritt",
  "ob.conv.corpus.words": "Eigene Wörter jetzt im Korpus: {words}.",
  "ob.conv.corpus.band": "Korpusqualität ist jetzt {band}.",
  "ob.conv.build.snapshot": "Ich friere deinen Korpus ein.",
  "ob.conv.build.extract": "Ich suche deine typischen Formulierungen.",
  "ob.conv.build.evaluate": "Ich teste Entwürfe gegen zurückgehaltene Proben.",
  "ob.conv.build.activate": "Ich aktiviere dein Stimmprofil.",
  "ob.conv.build.succeeded": "Dein Stimmprofil ist fertig.",
  "ob.conv.build.deferred":
    "Der Aufbau wartet auf Budget. Er läuft automatisch an.",
  "ob.conv.build.failed":
    "Der Aufbau wurde nicht fertig. Deine Texte bleiben erhalten, du kannst es jederzeit erneut versuchen.",
  "ob.conv.recap":
    "Das weiß dein CRM jetzt, mit einer Quelle zu jedem Eintrag.",
  "ob.conv.consent":
    "Letzter Schritt: Was darf ich erfassen, und zu welchem Zweck? Nichts ist standardmäßig aktiv.",
  "ob.conv.done": "Einrichtung abgeschlossen. Dein CRM ist bereit.",
  "ob.conv.clarify.question": "{question}",
  "ob.conv.clarify.optionDetail": "{detail}",
  "ob.conv.clarify.dismiss": "Überspringen - ich trage es selbst ein",
  "ob.conv.clarify.keepMine": "Meinen Wert behalten",
  "ob.conv.review.skipped":
    "Du hast übersprungen: {fields}. Du kannst sie jederzeit bearbeiten.",
  "ob.conv.clarify.applyFailed":
    "Ich konnte diese Wahl nicht übernehmen: {detail} Wähle bitte erneut.",
  "ob.conv.clarify.applyMissing":
    "Der Server hat diese Wahl nicht bestätigt. Wähle bitte erneut.",
  "ob.conv.loadFailed":
    "Ich konnte deine Einrichtung nicht prüfen. Bitte versuche es erneut.",
  "ob.conv.retry": "Erneut versuchen",
  "ob.conv.connect.persistFailed":
    "Ich konnte den Abschluss nicht speichern. Versuche es erneut.",
  "ob.conv.review.title":
    "Hier ist alles, was ich gefunden habe. Korrigiere mich.",
  "ob.conv.review.showLess": "Weniger zeigen",
  "ob.conv.review.continue": "Weiter",
  "ob.conv.review.progressLabel": "Ausgefüllte Pflichtfelder",
  "ob.conv.review.requiredRemaining_one":
    "{count} Feld nötig, bevor du fortfahren kannst",
  "ob.conv.review.requiredRemaining_other":
    "{count} Felder nötig, bevor du fortfahren kannst",
  "ob.conv.review.requiredDone": "Nichts weiter nötig — du kannst fortfahren.",
  "ob.conv.review.confirmQuestionOpen":
    "Eine Entscheidung ist noch offen. Beantworte sie, um fortzufahren.",
  "ob.conv.triage.stateRequired": "erforderlich, noch leer",
  "ob.conv.triage.stateEmpty": "leer",
  "ob.conv.triage.stateTyped": "von dir eingetragen",
  "ob.conv.triage.stateStored": "aus deinem Profil",
  "ob.conv.triage.stateQuoted": "aus deinem Impressum gelesen",
  "ob.conv.triage.emptyHint":
    "Hier steht noch nichts. Trag es ein, wenn es zählt.",
  "ob.conv.triage.legalNotPublished":
    "Nicht auf deinem Impressum angegeben. Trag es selbst ein.",
  "ob.conv.triage.legalNotChecked":
    "Ich habe kein Impressum auf deiner Website gefunden, das ich prüfen könnte. Trag es selbst ein.",
  "ob.conv.triage.legalUnpicked":
    "Dein Impressum nennt mehr als ein Unternehmen. Wähl aus, welches deins ist, dann trage ich es ein.",
  "ob.conv.triage.omittedLabel": "Ausgelassen, nicht geraten",
  "ob.conv.triage.omittedField": "{field}: {reason}",
  "ob.conv.triage.mapLabel": "Zu einem Abschnitt springen",
  "ob.conv.triage.sectionBlocking": "{count} nötig, um fortzufahren",
  "ob.conv.triage.sectionAdvisory": "{count} prüfenswert",
  "ob.conv.triage.blockingHead": "Nötig, um fortzufahren",
  "ob.conv.triage.advisoryHead": "Prüfenswert",
  "ob.conv.triage.sectionSettled": "Hier ist nichts offen",
  "ob.conv.triage.sectionMore": "+{count} weitere",
  "ob.conv.triage.restTitle": "Hintergrund, keine Aufgabe",
  "ob.conv.triage.looksSolid": "Sieht belegt aus · {count}",
  "ob.conv.triage.companyWebsite": "Website",
  "ob.conv.triage.sourceCount": "{count} Quelle",
  "ob.conv.triage.peopleLabel": "Personen",
  "ob.conv.triage.peopleCount": "{count} gefunden",
  "ob.conv.triage.peopleEmpty": "Keine Personen auf deiner Website gefunden.",
  "ob.conv.triage.factsLabel": "Fakten",
  "ob.conv.triage.factsCount": "{count} gefunden",
  "ob.rail.spend": "Tokens für dieses Setup",
  "ob.rail.tokensUnit": "Tok.",
  "ob.conv.scene.step": "Schritt {n} von {m} · {label}",
  "ob.conv.scene.detour": "Ein kurzer Umweg",
  "ob.conv.scene.decisionSub":
    "Deine Website nennt mehrere Gesellschaften. Die gewählte steht auf jeder Rechnung.",
  "ob.conv.scene.continue": "Weiter",
  "ob.conv.scene.candidates": "{count} Kandidaten",
  "ob.conv.connect.sceneTitle": "Verbinde deine Konten.",
  "ob.conv.connect.sceneSub":
    "Ich baue Kontakte, Firmen und Historie aus dem, was schon im Postfach liegt.",
  "ob.conv.connect.mailboxTitle": "Dein Postfach",
  "ob.conv.connect.mailboxHint":
    "Wähle eins. Von hier kommen deine Kontakte, Firmen und Historie.",
  "ob.conv.connect.networkTitle": "Dein Netzwerk",
  "ob.conv.connect.networkHint":
    "Optional, aber lohnend. Macht aus Bekannten Accounts und beobachtet sie auf Trigger.",
  "ob.conv.connect.required": "erforderlich",
  "ob.conv.connect.recommended": "empfohlen",
  "ob.conv.connect.gmailBrings": "Mail, Kontakte und Kalender von Google",
  "ob.conv.connect.microsoftBrings":
    "Mail, Kontakte und Kalender über die Graph-API",
  "ob.conv.connect.imapBrings": "Jeder andere Mail-Host, mit App-Passwort",
  "ob.conv.connect.linkedinAuth": "Profillink, nur lesend",
  "ob.conv.connect.scopeGoogle": "OAuth, Lese- und Sendeberechtigung",
  "ob.conv.connect.scopeMicrosoft": "OAuth, Graph API",
  "ob.conv.connect.scopeImap": "Jeder andere Anbieter, App-Passwort",
  "ob.conv.connect.connectCta": "verbinden →",
  "ob.conv.connect.connectedCta": "verbunden",
  "ob.conv.connect.blockedCard":
    "Du hast schon ein Postfach gewählt. Trenne es in den Einstellungen, um zu wechseln.",
  "ob.conv.connect.guaranteesToggle": "Was Verbinden tatsächlich bedeutet",
  "ob.conv.connect.railPromise":
    "Wir lesen nur, und nichts wird ohne deine Freigabe gesendet.",
  "ob.conv.connect.dialogHeadlineAccess": "Zugriff auf {name} nötig",
  "ob.conv.connect.dialogHeadlineImap": "Verbinde deinen Mail-Host",
  "ob.conv.connect.dialogIntro":
    "{brings}. Ich lese es einmal, um deine Kontakte und Historie aufzubauen, und halte es danach synchron.",
  "ob.conv.connect.dialogClose": "Schließen",
  "ob.conv.connect.linkedinName": "LinkedIn",
  "ob.conv.connect.linkedinConnected": "Verbunden",
  "ob.conv.connect.linkedinSkippedNote":
    "Übersprungen: später in den Einstellungen nachholbar",
  "ob.conv.connect.rosterFailedTitle":
    "Postfächer konnten nicht geprüft werden",
  "ob.conv.connect.rosterFailedBody":
    "Beim Laden deines Verbindungsstatus ist etwas schiefgelaufen. Versuche es erneut, bevor du einen Anbieter auswählst.",
  "ob.conv.voice.sceneTitle": "Zeig mir, wie du schreibst.",
  "ob.conv.voice.sceneSub":
    "Dieses CRM entwirft jede Mail in deinen Worten, und ohne deine Freigabe geht nichts raus.",
  "ob.conv.voice.heroBody":
    "Es lernt Ton, Rhythmus und Formulierung aus deinen Texten, aus keinen anderen.",
  "ob.conv.voice.whyToggle": "Warum das zählt",
  "ob.conv.voice.dropTitle": "Leg deine Texte hier ab",
  "ob.conv.voice.dropSub":
    "Gesendete Mails eignen sich am besten, weil sie zeigen, wie du schreibst, wenn du etwas willst.",
  "ob.conv.voice.browse": "Dateien wählen",
  "ob.conv.voice.pasteInstead": "Stattdessen Text einfügen",
  "ob.conv.voice.sourcesTitle": "Quellen",
  "ob.conv.voice.meterLabel": "Fortschritt zum Minimum von {min} Wörtern",
  "ob.conv.voice.meterProgress": "{words} von {min} Wörtern",
  "ob.conv.voice.meterReady":
    "{words} Wörter — genug für den Aufbau. Mehr schärft es weiter.",
  "ob.conv.voice.footReady":
    "Das Training dauert etwa eine Minute. Du siehst ein Beispiel, bevor etwas gespeichert wird.",
  "ob.conv.voice.footFloor":
    "Mindestens {min} Wörter. Darunter kopiert das Modell nur Formulierungen.",
  "ob.conv.voice.buildingTitle": "Ich lerne deine Stimme",
  "ob.conv.voice.buildingMeta": "{words} Wörter, {sources} Quellen",
  "ob.conv.voice.resultSub":
    "Lies zuerst das Beispiel. Passt es, bestätige. Passt es nicht, gib mir mehr Quellen und ich baue neu.",
  "ob.conv.voice.resultSubNoSample":
    "Dein Korpus reicht noch nicht für ein Beispiel. Das hat der Aufbau gelernt. Füg Quellen hinzu.",
  "ob.conv.voice.resultContinue": "Das bin ich",
  "ob.conv.voice.sampleEyebrow": "Beispiel, nicht gesendet",
  "ob.conv.voice.sampleAnother": "Anderes Szenario",
  "ob.conv.voice.sampleSubjectLabel": "Betreff",
  "ob.conv.voice.sampleWhyTag": "Warum",
  "ob.conv.voice.dimensionsTitle": "Gemessene Dimensionen",
  "ob.conv.voice.dimensionsCount": "Gemessen: {count}",
  "ob.conv.voice.dimSentenceName": "Satzlänge",
  "ob.conv.voice.dimSentencePoleLow": "Knapp",
  "ob.conv.voice.dimSentencePoleHigh": "Ausführlich",
  "ob.conv.voice.dimSentenceMeasured": "Mittel",
  "ob.conv.voice.dimSentenceEvidence": "Im Schnitt {count} Wörter pro Satz.",
  "ob.conv.scene.evidence": "Beleg",
  "ob.conv.scene.hideEvidence": "Beleg ausblenden",
  "ob.conv.scene.whyThis": "Was ich gelesen habe",
  "ob.conv.scene.foundOn": "Gefunden auf",
  "ob.conv.guide.decision":
    "Ich brauche eine Entscheidung von dir: {question} Sie steht rechts, mit den Belegen zu jeder Option.",
  "ob.conv.guide.reviewBlocked_one":
    "Deine Prüfung ist rechts bereit. {count} Feld blockiert die Übernahme.",
  "ob.conv.guide.reviewBlocked_other":
    "Deine Prüfung ist rechts bereit. {count} Felder blockieren die Übernahme.",
  "ob.conv.guide.reviewAdvisory_one":
    "Deine Prüfung ist rechts bereit. Nichts blockiert dich; {count} Punkt ist einen Blick wert.",
  "ob.conv.guide.reviewAdvisory_other":
    "Deine Prüfung ist rechts bereit. Nichts blockiert dich; {count} Punkte sind einen Blick wert.",
  "ob.conv.guide.reviewClean":
    "Deine Prüfung ist rechts bereit. Sie sieht sauber aus, prüfe was du willst und übernimm, wenn du bereit bist.",
  "ob.conv.guide.attentionHeading": "Diese brauchen deine Eingabe",
  "ob.conv.guide.attentionGroup.blocking": "Nötig, um fortzufahren",
  "ob.conv.guide.attentionGroup.decisions": "Braucht eine Entscheidung",
  "ob.conv.guide.attentionGroup.advisory": "Einen Blick wert",
  "ob.conv.guide.attentionStatus.blocks": "nötig zum Fortfahren",
  "ob.conv.guide.attentionStatus.empty": "noch leer",
  "ob.conv.guide.attentionStatus.decision": "braucht eine Entscheidung",
  "ob.conv.guide.attentionStatus.check": "einen Blick wert",
  "ob.conv.activity.steps": "{count} Schritte",
  "ob.conv.showField": "Zeig mir",
  "ob.conv.review.editDirectly": "Felder direkt bearbeiten",
  "ob.conv.review.backToDossier": "Zurück zum Dossier",
  "ob.conv.review.proposalFallback":
    "Ich konnte die vorbereitete Zuordnung nicht laden. Prüfe direkt, was ich gelesen habe. Jedes Feld behält seine Quelle.",
  "ob.conv.review.confirmFailed":
    "Ich konnte noch nicht speichern: {detail} Korrigiere das und übernimm erneut.",
  "ob.conv.review.confirmVersionSkew":
    "Deine Prüfung hat gerade neuere Daten bekommen. Schau nach und drück erneut Weiter.",
  "ob.conv.review.confirmVersionSkewStuck":
    "Es hat sich noch nichts geändert, Weiter würde erneut scheitern. Schau nach oder prüf gleich.",
  "ob.conv.review.confirmNotReady":
    "Dieser Read hat noch keinen Entwurf. Prüf erneut, wenn er fertig ist, oder starte neu.",
  "ob.conv.review.confirmCheckFailed":
    "Der Read ist bestätigt, aber die Firma lud nicht. Prüf es gleich noch einmal.",
  "ob.conv.artifact.empty":
    "Noch nichts gelesen. Nenn mir eine Website und dieses Panel füllt sich mit belegten Funden.",
  "ob.conv.results.continue": "Weiter",
  "ob.conv.results.artifactTitle": "Einrichtung im Überblick",
  "ob.conv.results.artifactBody":
    "Womit dein CRM startet. Hier steht nichts, das nicht wirklich passiert ist.",
  "ob.conv.results.company":
    "Firmenprofil für {name} bestätigt. Alles Gespeicherte trägt seine Quelle.",
  "ob.conv.results.companyUnsaved":
    "Deine Firmendaten sind noch nicht gespeichert. Du kannst sie später in den Einstellungen vervollständigen.",
  "ob.conv.results.voiceBuilt":
    "Dein Stimmprofil ist gebaut. Entwürfe klingen nach dir.",
  "ob.conv.results.voiceSkipped":
    "Noch kein Stimmprofil. Entwürfe nutzen eine neutrale Startstimme; deins kannst du später in den Einstellungen bauen.",
  "ob.conv.recap.back": "Willkommen zurück. Hier stehen wir.",
  "ob.conv.recap.company": "Dein Firmenprofil für {name} ist bestätigt.",
  "ob.conv.recap.companyUnsaved":
    "Deine Firmendaten sind noch nicht gespeichert. Du kannst sie in den Einstellungen vervollständigen.",
  "ob.conv.recap.voiceBuilt":
    "Dein Stimmprofil ist gebaut. Entwürfe können nach dir klingen.",
  "ob.conv.recap.voiceSkipped":
    "Du hast das Stimmprofil übersprungen. Entwürfe nutzen eine neutrale Startstimme.",
  "ob.conv.recap.corpus":
    "Dein Korpus enthält bereits {words} deiner eigenen Wörter.",
  "ob.conv.recap.readTerminal":
    "Willkommen zurück. {host} ist gelesen: {count} belegte Funde, unten bereit.",
  "ob.conv.recap.readReading":
    "Willkommen zurück. Ich lese {host} noch. Seiten bisher: {pages}.",
  "ob.conv.recap.readFailed":
    "Willkommen zurück. Mein früherer Lesevorgang von {host} wurde nicht fertig. Nenn mir wieder eine Website oder erzähl es mir direkt.",
  "ob.conv.recap.readDeferred":
    "Willkommen zurück. Mein Lesevorgang von {host} pausiert gerade. Nenn mir wieder eine Website oder erzähl es mir direkt.",
  "ob.conv.connect.pick":
    "Wähle einen Anbieter, um genau zu sehen, was das Verbinden tut. Oder überspringe es und verbinde später in den Einstellungen.",
  "ob.conv.linkedin.cardBody":
    "Macht aus deinem Netzwerk Firmen und Kontakte, und meldet, wenn jemand aus deinem Netzwerk den Job wechselt.",
  "ob.conv.linkedin.limitsToggle": "Was Margince sehen kann und was nicht",
  "ob.conv.linkedin.scope1Lead": "Deine Kontaktliste \u2014",
  "ob.conv.linkedin.scope1Rest":
    "Name, Position, Unternehmen und das Datum der Vernetzung.",
  "ob.conv.linkedin.scope2Lead": "Sonst nichts.",
  "ob.conv.linkedin.scope2Rest":
    "Keine Nachrichten, keine Beitr\u00e4ge, keine Profilbesuche, keine Aktivit\u00e4ten.",
  "ob.conv.linkedin.scope3Lead": "Dein Netzwerk bleibt deins.",
  "ob.conv.linkedin.scope3Rest":
    "Es wird dir zugeordnet, nie dem Unternehmen, und beim Trennen wird es entfernt.",
  "ob.conv.linkedin.scope4Lead": "Niemand wird kontaktiert.",
  "ob.conv.linkedin.scope4Rest":
    "Das Verbinden verschickt keine Einladungen und keine Nachrichten \u2014 nie.",
  "ob.conv.linkedin.neverContacts":
    "Deine Kontakte werden nie zu CRM-Kontakten. Sie beantworten nur: Wer hier kennt sie schon?",
  "ob.conv.linkedin.profileLabel": "Deine LinkedIn-Profil-URL",
  "ob.conv.linkedin.profilePlaceholder": "https://www.linkedin.com/in/…",
  "ob.conv.linkedin.profileWhy":
    "So gehört das Netzwerk dir: „Anna kennt sie“, nie „das Unternehmen kennt sie“.",
  "ob.conv.linkedin.authorize": "Mit LinkedIn autorisieren",
  "ob.conv.linkedin.appPending":
    "Unsere LinkedIn-App wartet noch auf Freigabe, es synchronisiert nichts. Lade solange Connections.csv hoch.",
  "ob.conv.linkedin.skip": "LinkedIn vorerst \u00fcberspringen",
  "ob.conv.linkedin.connected":
    "LinkedIn autorisiert. Deine Kontakte werden synchronisiert, sobald die App freigegeben ist.",
  "ob.conv.linkedin.skipped":
    "LinkedIn \u00fcbersprungen. Du kannst es jederzeit in den Einstellungen verbinden.",
  "ob.conv.connect.skip": "Verbinden vorerst überspringen",

  // Die Setup-Leiste: fünf Stationen, je ein Wort. Lang genug, den Schritt zu
  // benennen, kurz genug, dass fünf davon bei 10px in eine Spalte passen.
  "ob.rail.read": "Lesen",
  "ob.rail.confirm": "Bestätigen",
  "ob.rail.voice": "Stimme",
  "ob.rail.ready": "Fertig",
  "ob.rail.connect": "Verbinden",

  // --- das Tor: der erste Screen nach der Anmeldung ----------------------
  // Eine Frage und sonst nichts. Niemand soll das ganze Werkzeug auf dem
  // ersten Screen treffen, also nennt das Tor, was es tut, was es den Leser
  // kostet (zwei Minuten) und wer entscheidet (er selbst) — und fragt dann
  // einmal.
  "ob.gate.title": "Hallo {name}, ich bin die Margince-KI.",
  "ob.gate.titleAnonymous": "Ich bin die Margince-KI.",
  "ob.gate.sub":
    "Ich lese deine Website und entwerfe dein Firmenprofil. Du gibst frei, bevor ich speichere. Zwei Minuten.",
  "ob.gate.trustToggle": "So funktioniert es",
  "ob.gate.trustBody":
    "Ich lese nur öffentliche Seiten. Nichts wird gespeichert, bevor du bestätigst, und ohne deine Freigabe geht nie etwas raus.",
  "ob.gate.field": "Deine Website-Adresse",
  "ob.gate.placeholder": "deinefirma.de",
  "ob.gate.submit": "Meine Website lesen",
  "ob.gate.altPrompt": "Keine Website zur Hand?",
  "ob.gate.altAction": "Die Angaben selbst eintragen",
  "ob.gate.invalidUrl":
    "Das sieht nicht wie eine Web-Adresse aus. Versuch es als deinefirma.de.",
  // Ein String für zwei Fehler, die für den Leser gleich aussehen: die
  // Anfrage kam nie an, oder das Lesen begann und wurde nicht fertig.
  // {detail} ist die Erklärung des Servers und kann leer sein — der Satz muss
  // also auch ohne sie tragen.
  "ob.gate.startFailed":
    "Ich konnte diese Website nicht lesen. {detail} Versuch eine andere Adresse, oder gib die Daten selbst ein.",
  // Ein aufgeschobenes Lesen ist vertagt, nicht kaputt: der Server kommt darauf
  // zurück. Der Satz sagt also, was stimmt, und nennt beide Türen, ohne dass
  // der Leser irgendetwas reparieren soll.
  "ob.gate.readPaused":
    "Dieses Lesen pausiert. {detail} Es läuft von selbst weiter — oder nenn mir eine andere Adresse.",

  // --- das Lese-Theater --------------------------------------------------
  // Sichtbar gemachtes Volumen. Die Schnittstelle liefert keinen Nenner für
  // die Seitenzahl, also ist jede Zahl hier ein offener Zähler — nie "14 von
  // 18", nie ein Balken mit bekanntem Ende, denn die Gesamtzahl zu erfinden
  // hieße, Daten zu erfinden.
  "ob.scan.title": "Ich lese {host}",
  "ob.scan.sub":
    "Jeder Fakt behält seine Quellseite, du kannst alles nachprüfen.",
  "ob.scan.doneTitle": "{host} gelesen",
  "ob.scan.doneSub":
    "{facts} Fakten und {fields} Profilfelder, jeweils mit der Seite, von der sie kommen. Ich öffne deine Durchsicht.",
  "ob.scan.phaseCrawling": "Ich hole Seiten",
  "ob.scan.phaseExtracting": "Ich ermittle, was du verkaufst",
  "ob.scan.phaseQueued": "In der Warteschlange, startet gleich",
  "ob.scan.phaseDeferred": "Vorerst pausiert",
  "ob.scan.pagesRead": "{pages} Seiten gelesen",
  "ob.scan.pagesSkipped": "{count} übersprungen",
  "ob.scan.factsSoFar": "{count} Fakten bisher",
  "ob.scan.stillReading": "lese noch",
  "ob.scan.pageStripLabel": "Bisher gelesene Seiten",
  "ob.scan.logLabel": "Die Seiten, die ich gerade durchgehe, neueste zuerst",
  "ob.scan.pageFetched": "{url} — gelesen",
  "ob.scan.pageSkipped": "{url} — übersprungen: {reason}",
  "ob.scan.pageFailed": "{url} — nicht lesbar: {reason}",
  "ob.scan.pageNoReason": "kein Grund erfasst",
  "ob.scan.pageStatusFetched": "gelesen",
  "ob.scan.pageStatusSkipped": "übersprungen: {reason}",
  "ob.scan.pageStatusFailed": "nicht lesbar: {reason}",
  "ob.scan.skipReason.robots": "die Seite hat mich gebeten, sie nicht zu lesen",
  "ob.scan.skipReason.offDomain": "sie liegt auf einer anderen Domain",
  "ob.scan.skipReason.pageCap":
    "ich hatte schon so viele Seiten gelesen, wie ein Read erlaubt",
  "ob.scan.skipReason.byteCap":
    "dieses Lesen hatte schon so viel Text aufgenommen, wie erlaubt ist",
  "ob.scan.skipReason.unreadable": "ich konnte die Seite nicht lesen",
  "ob.scan.transparency": "Transparenz",
  "ob.scan.costLine": "{calls} Aufrufe · {tokens} Tokens · {cost}",
  "ob.scan.costPending": "noch keine Modellaufrufe berechnet",
  "ob.scan.costUnpriced": " · nicht bepreiste Nutzung vorhanden",

  // --- das Live-Panel: was der Lauf abgedeckt hat und was nicht ----------
  "ob.live.stateDone": "fertig",
  "ob.live.stateNow": "läuft",
  "ob.live.stateWaiting": "wartet",
  "ob.live.review": "Prüfen",
  "ob.live.hide": "Ausblenden",
  "ob.live.countPages": "{read} gelesen · {skipped} übersprungen",
  "ob.live.cardCoverage": "Was ich gelesen und was ich übersprungen habe",
  "ob.live.coverageWarning": "Hinweis",
  "ob.live.coverageStopped": "Vorzeitig beendet",
  "ob.live.stoppedPageCap":
    "Ich habe das Seitenlimit für einen Lesevorgang erreicht. Es gibt also mehr auf deiner Website, das ich nicht geöffnet habe.",
  "ob.live.stoppedByteCap":
    "Ich habe das Größenlimit für einen Lesevorgang erreicht. Es gibt also mehr auf deiner Website, das ich nicht geöffnet habe.",
  "ob.live.stoppedBudget":
    "Ich habe das Budget für einen Lesevorgang erreicht. Es gibt also mehr auf deiner Website, das ich nicht geöffnet habe.",
  "ob.live.stoppedDeadline":
    "Mir ist die Zeit für einen Lesevorgang ausgegangen. Es gibt also mehr auf deiner Website, das ich nicht geöffnet habe.",
  "ob.live.coverageSkipped": "Übersprungen",
  "ob.live.coverageFailed": "Nicht lesbar",
  "ob.live.coverageClean":
    "Jede Seite, die ich versucht habe, kam zurück. Nichts wurde übersprungen, nichts ist fehlgeschlagen.",

  // --- Fakten: einen speichern, und die Obergrenze dafür ----------------
  "ob.facts.rowSave": "Diesen Fakt speichern: {fact}",
  "ob.facts.capReached":
    "Du kannst bis zu {max} Fakten speichern. Nimm einen heraus, um Platz für einen anderen zu machen.",

  // --- der Gegenwert: was zwei Minuten wirklich gebracht haben -----------
  // Zahlen, kein Applaus. Jede Zelle ist eine echte Zahl von der
  // Schnittstelle, und eine Zelle ohne Zahl sagt das, statt eine Null zu
  // zeigen, die wie ein Ergebnis aussieht.
  "ob.payoff.lead": "Vor wenigen Minuten war das eine leere Installation.",
  "ob.payoff.leadResumed": "Das hier hat als leere Installation angefangen.",
  "ob.payoff.factsRead": "Fakten gelesen",
  "ob.payoff.factsConfirmed": "Fakten bestätigt",
  "ob.payoff.peopleFound": "Personen gefunden",
  "ob.payoff.profileFields": "Profilfelder",
  "ob.payoff.voiceWords": "Wörter deiner Stimme",
  "ob.payoff.pagesRead": "Seiten gelesen",
  "ob.payoff.voiceNotTrained": "Stimme noch nicht trainiert",
  "ob.payoff.body":
    "Alles darin kannst du korrigieren, und jeder Wert zeigt weiter auf die Seite, von der er kommt.",
  "ob.payoff.defaults":
    "Ich warte auf dein Ja und überschreibe nie deine Eingaben. Beides in Einstellungen → Autonomie.",
  "ob.payoff.seats":
    "Es fehlen nur noch deine Kollegen. Sitze sind kostenpflichtig, deshalb legst du sie unter Einstellungen → Nutzer selbst an.",
  "ob.payoff.understood": "Verstanden",
  "ob.payoff.projects":
    "Wird aus einem Deal Arbeit, leg dafür ein Projekt an: Ein Projekt beginnt während des Deals und bleibt nach dem Gewinn bestehen, damit die Lieferung ihre eigene Chronik behält.",
  "ob.payoff.projectsLink": "Projekte ansehen",

  // --- die Übergabe in die App ------------------------------------------
  "ob.enter.cta": "Margince öffnen",
  "ob.enter.assembling": "Deine Organisation wird zusammengestellt",

  // --- das Zurücklesen des Postfachs -------------------------------------
  // Ein anderer Vorgang als das Verbinden, und der Text muss die beiden
  // getrennt halten: Verbinden erteilt Zugriff, das Zurücklesen verbraucht
  // Budget, um den Verlauf zu lesen. Es liest nur und schreibt nichts,
  // solange der Leser nicht zustimmt.
  "ob.backread.heading": "Wie weit soll ich zurücklesen?",
  "ob.backread.window3m": "3 Monate — aktueller Kontext",
  "ob.backread.window6m": "6 Monate — empfohlen",
  "ob.backread.window12m": "12 Monate — ganzer Vertriebszyklus",
  "ob.backread.window24m": "2 Jahre — die Beziehung, nicht nur der Deal",
  "ob.backread.window60m": "5 Jahre — alles, was das Postfach noch hat",
  "ob.backread.estimate": "Etwa {messages} Nachrichten in diesem Zeitraum.",
  "ob.backread.estimateHeuristic":
    "Aus dem Postfach geschätzt, noch nicht gezählt.",
  "ob.backread.estimateCost": "Ungefähr {cost} an Modellaufrufen.",
  "ob.backread.estimateFailed":
    "Ich konnte diesen Zeitraum nicht schätzen: {detail} Du kannst trotzdem starten oder einen anderen wählen.",
  "ob.backread.note":
    "Das Zurücklesen liest nur. Du siehst jede Person und Firma, bevor etwas geschrieben wird.",
  "ob.backread.start": "Verbinden und lesen",
  "ob.backread.startFailed":
    "Ich konnte das Zurücklesen nicht starten: {detail} Versuch es erneut, oder mach weiter und starte es später in den Einstellungen.",
  "ob.backread.running": "Ich lese dein Postfach",
  "ob.backread.runningNote":
    "Du kannst das laufen lassen und weiterarbeiten. Ich mache dort weiter, wo ich stehen geblieben bin.",
  "ob.backread.queued": "In der Warteschlange. Es startet gleich.",
  "ob.backread.progress": "{scanned} von etwa {total} Nachrichten",
  "ob.backread.progressNoTotal": "{scanned} Nachrichten bisher",
  "ob.backread.tallyMessages": "Nachrichten gelesen",
  "ob.backread.tallyCaptured": "behalten",
  "ob.backread.tallySkipped": "ignoriert",
  "ob.backread.tallyPeople": "Personen gefunden",
  "ob.backread.tallyCompanies": "Firmen gefunden",
  "ob.backread.doneHeading": "Das steckt darin.",
  "ob.backread.doneNote":
    "Noch ist nichts geschrieben. Alles, was ich gefunden habe, wartet im Eingang auf deine Prüfung.",
  "ob.backread.failed":
    "Das Zurücklesen wurde gestoppt: {detail} Deine Verbindung ist in Ordnung — du kannst es in den Einstellungen erneut starten.",
  "ob.backread.cancelled":
    "Ich habe das Lesen gestoppt. Es wurde nichts geschrieben.",
  "ob.backread.cancelledPartial":
    "Ich habe das Lesen gestoppt. Was schon erfasst wurde, bleibt erhalten — es wartet im Eingang auf dich.",
  "ob.backread.cancelFailed":
    "Ich konnte das Lesen nicht stoppen: {detail} Versuch es erneut — es läuft in der Zwischenzeit weiter.",
  "ob.backread.detailUnavailable": "Etwas ist unerwartet schiefgelaufen.",
  "ob.backread.cancel": "Lesen stoppen",
  "ob.backread.explore": "In der Zeit Margince erkunden",
  "ob.backread.skip": "Verlauf jetzt nicht lesen",

  "auth.title": "Margince",
  "auth.checking": "Deine Sitzung wird geprüft…",
  "auth.pageTitle": "Anmelden · Margince",
  "auth.loginTitle": "Bei Margince anmelden",
  // "eine Admin-Person", nicht "deine Administration": eine Administration ist
  // im Deutschen eine Stelle oder eine Tätigkeit, keine Person — der Rest des
  // Katalogs sagt durchgehend "Admin-Person". Und der zweite Satz nennt das Verb
  // statt des Nominalstils ("Eine Selbstregistrierung gibt es nicht").
  "auth.loginSub":
    "Konten legt eine Admin-Person an. Selbst registrieren kannst du dich nicht.",
  "auth.coreDisclosure": "Margince · KI-System",
  "auth.coreGreeting": "Hallo, ich bin Margince.",
  "auth.corePurpose": "Ich kümmere mich um die Arbeit rund um deine Arbeit.",
  // Gedankenstrich mit Leerzeichen, nicht der englische Geviertstrich ohne:
  // im Deutschen ist das die einzige Setzung, die nicht wie ein Bindestrich
  // zwischen zwei Wörtern liest.
  "auth.coreWork":
    "Ich halte dein CRM aktuell, erkenne, was Aufmerksamkeit braucht, und bereite den nächsten Schritt vor – damit du dich um Kunden kümmern kannst.",
  "auth.corePromise":
    "Und keine Sorge: Ich sende nie eine E-Mail oder Nachricht, ohne dich vorher zu fragen.",
  "auth.coreHandover": "Zuerst stelle ich sicher, dass du es wirklich bist …",
  "auth.coreConfigured": "Konfiguriert",
  "auth.coreUnconfigured": "KI nicht konfiguriert",
  // "auch ohne", nicht "weiterhin": "weiterhin" ist zeitlich und liest sich neben
  // dem Hinweis "KI nicht konfiguriert" wie "noch, aber nicht mehr lange".
  "auth.coreStillWorks": "Das CRM funktioniert auch ohne.",
  "auth.coreDevelopment": "Entwicklungs-KI",
  "auth.coreModeCloud": "Cloud-Routing",
  "auth.coreModeLocal": "lokales Routing",
  "auth.coreModeHybrid": "hybrides Routing",
  "auth.coreModeNone": "kein Modell-Routing",
  // Die Nachbarwerte sind alle Betriebsarten; "Modus" ist dafür das deutsche
  // Wort, "Pfad" die Übersetzung von "path".
  "auth.coreModeDevelopment": "Offline-Entwicklungsmodus",
  "auth.coreProviderAnthropic": "Anthropic",
  "auth.coreProviderGemini": "Gemini",
  "auth.coreProviderOllama": "Ollama",
  "auth.coreProviderOpenAI": "OpenAI",
  "auth.coreProviderCompatible": "kompatibler Anbieter",
  "auth.coreProviderVllm": "vLLM",
  "auth.email": "E-Mail",
  // Der lokale Teil einer Adresse ist nie ein Pronomen — "du@" ist "you@"
  // Zeichen für Zeichen. "beispiel.de" ist im Deutschen, was "example.com" im
  // Englischen ist, und genau das pinnt die Login-Spec §7.2.
  "auth.emailPlaceholder": "name@beispiel.de",
  "auth.password": "Passwort",
  "auth.passwordPlaceholder": "Passwort",
  "auth.passwordHint": "mindestens 12 Zeichen",
  "auth.showPassword": "Passwort anzeigen",
  "auth.hidePassword": "Passwort ausblenden",
  "auth.capsLock": "Feststelltaste ist an",
  "auth.continueWith": "Weiter mit {brand}",
  "auth.orDivider": "oder",
  "auth.legalProtected": "Der Zugang zu dieser Organisation ist beschränkt.",
  "auth.legalTerms": "Nutzungsbedingungen",
  "auth.legalPrivacy": "Datenschutz",
  "auth.signingIn": "Anmeldung läuft…",
  "auth.signIn": "Anmelden",
  "auth.failed": "Das hat nicht geklappt",
  "auth.errCredentials":
    "Die Anmeldung war nicht möglich. Prüfe E-Mail-Adresse und Passwort und versuche es erneut.",
  "auth.errRateLimited":
    "Zu viele Anmeldeversuche. Warte einen Moment und versuche es erneut.",
  "auth.errUnreachable":
    "Margince ist nicht erreichbar. Prüfe deine Verbindung und versuche es erneut.",
  "auth.retry": "Erneut versuchen",
  "auth.noticeSignedOut": "Du wurdest abgemeldet.",
  "auth.noticeSessionExpired":
    "Deine Sitzung ist abgelaufen. Melde dich erneut an, um fortzufahren.",
  "auth.noticeOidcFailed":
    "Die Anmeldung mit Google hat nicht geklappt. Falls du eingeladen wurdest, öffne den Link in deiner Einladungs-E-Mail, um dein Konto fertig einzurichten.",
  "auth.connectionTitle": "Margince ist nicht erreichbar",
  "auth.connectionBody":
    "Prüfe deine Verbindung und versuche es erneut. Besteht das Problem weiter, startet der Server womöglich gerade neu.",
  "auth.unavailableTitle": "Installation nicht bereit",
  // "Betreiber", nicht "Operator": ein Operator ist im Deutschen ein
  // mathematisches Zeichen oder eine Telefonvermittlung. Und eine Einrichtung
  // wird korrigiert, nicht repariert — repariert werden Geräte.
  "auth.unavailableBody":
    "Diese Margince-Installation kann dich noch nicht anmelden. Ein Betreiber muss die Einrichtung abschließen oder korrigieren.",
  "forcedPassword.pageTitle": "Passwort wählen",
  "forcedPassword.title": "Wähle dein eigenes Passwort",
  "forcedPassword.body":
    "Dieses Konto nutzt noch das Passwort, das dein Betreiber eingerichtet hat. Wähle eines, das nur du kennst, bevor du fortfährst.",
  "password.title": "Passwort",
  "password.body": "Ändere das Passwort, mit dem du dich anmeldest.",
  "password.current": "Aktuelles Passwort",
  "password.next": "Neues Passwort",
  "password.confirm": "Neues Passwort bestätigen",
  "password.hint": "Mindestens 12 Zeichen.",
  "password.tooShort": "Zu kurz. Verwende mindestens 12 Zeichen.",
  "password.mismatch": "Die beiden stimmen nicht überein.",
  "password.signsYouOut":
    "Die Änderung meldet dich überall ab, auch hier. Melde dich mit dem neuen Passwort erneut an.",
  "password.changing": "Passwort wird geändert…",
  "password.open": "Passwort ändern",
  "password.cancel": "Abbrechen",
  "password.submit": "Neues Passwort speichern",
  "password.done": "Passwort geändert. Melde dich mit dem neuen an.",
  "password.errorGeneric":
    "Das Passwort konnte nicht geändert werden. Versuche es erneut.",
  "setup.pageTitle": "Margince einrichten",
  "setup.title": "Diese Installation übernehmen",
  "setup.body":
    "Diese Margince-Installation hat noch keine Organisation. Dein Betreiber hat ein einmaliges Einrichtungs-Token aus der Token-Datei, die der Server beim ersten Start geschrieben hat.",
  "setup.token": "Einrichtungs-Token",
  "setup.tokenHint":
    "Aus der Token-Datei, die der Server beim ersten Start geschrieben hat — das Serverprotokoll nennt ihren Pfad und enthält das Token selbst, falls die Datei nicht geschrieben werden konnte.",
  "setup.organization": "Name der Organisation",
  "setup.baseCurrency": "Basiswährung",
  "setup.baseCurrencyHint":
    "Jeder Betrag im Produkt wird in diese Währung umgerechnet. Sie lässt sich in den Einstellungen ändern, aber nur solange noch kein Betrag dagegen umgerechnet wurde — es lohnt sich also, sie jetzt richtig zu setzen.",
  "setup.baseCurrencyMalformed":
    "Eine Währung besteht aus drei Buchstaben, zum Beispiel EUR, CHF oder USD.",
  "setup.baseLanguage": "Basissprache",
  "setup.baseLanguageHint":
    "Die Sprache, in der die KI schreibt, wenn das ganze Team mitliest. Jede Person wählt ihre eigene Anzeigesprache weiterhin selbst, und Antworten an Kunden folgen der Sprache des Gesprächs.",
  "setup.timezone": "Zeitzone für Auswertungen",
  "setup.timezoneHint":
    "IANA-Zonenname. Jeder Auswertungszeitraum wird darin berechnet — aus diesem Browser übernommen, also ändere ihn, wenn du nicht dort bist, wo das Team arbeitet.",
  "setup.adminName": "Dein Name",
  "setup.adminEmail": "Deine E-Mail-Adresse",
  "setup.adminPassword": "Passwort wählen",
  "setup.passwordHint": "Mindestens 12 Zeichen.",
  "setup.passwordShort": "Zu kurz. Verwende mindestens 12 Zeichen.",
  "setup.rootWarning":
    "Damit entsteht das Administratorkonto für die gesamte Installation. Es hat alle Berechtigungen, einschließlich der Verwaltung aller anderen.",
  "setup.claim": "Organisation anlegen",
  "setup.claiming": "Wird angelegt…",
  "setup.errorToken":
    "Dieses Einrichtungs-Token gilt nicht für diese Installation. Prüfe die Token-Datei, die das Serverprotokoll beim ersten Start nennt.",
  "setup.errorAlready":
    "Diese Installation hat bereits eine Organisation. Melde dich an oder bitte deinen Betreiber um ein Zurücksetzen.",
  "setup.errorFields":
    "Im Formular stimmt etwas nicht. Prüfe die Felder und versuche es erneut.",
  "setup.errorServer":
    "Margince konnte die Einrichtung nicht abschließen. Es wurde nichts angelegt. Versuche es gleich noch einmal; bei wiederholtem Fehlschlag prüfe das Serverprotokoll.",
  "setup.errorNetwork":
    "Margince war nicht erreichbar. Prüfe deine Verbindung und versuche es erneut.",
  "auth.forgotLink": "Passwort vergessen?",
  "auth.forgotTitle": "Passwort zurücksetzen",
  // "gibt", nicht "existiert" (Amtsdeutsch), und das Feld will eine Adresse, keine
  // E-Mail — die E-Mail ist die Nachricht. Was unterwegs ist, ist ebenfalls die
  // Nachricht, nicht der Link. Die Existenz des Kontos bleibt offen.
  "auth.forgotSub":
    "Gib deine E-Mail-Adresse ein. Wenn es dazu ein Konto gibt, schicken wir dir einen Link.",
  "auth.sendResetLink": "Link senden",
  "auth.forgotSentTitle": "Prüfe dein Postfach",
  "auth.forgotSentBody":
    "Wenn es zu dieser Adresse ein Konto gibt, ist die E-Mail unterwegs. Der Link läuft in einer Stunde ab.",
  "auth.resetTitle": "Neues Passwort wählen",
  "auth.resetSub": "Dein Link ist gültig. Wähle ein neues Passwort.",
  "auth.newPassword": "Neues Passwort",
  "auth.setNewPassword": "Neues Passwort speichern",
  // "bereits verwendet", nicht "verbraucht": ein Link wird verwendet, nicht
  // verbraucht wie Kraftstoff.
  "auth.resetFailed":
    "Dieser Link ist ungültig, bereits verwendet oder abgelaufen.",
  // "nicht akzeptiert": abgelehnt werden Anträge und Angebote, nicht Passwörter.
  // Ein anderes zu wählen IST der neue Versuch, also entfällt der Nachsatz.
  "auth.resetRejectedPassword":
    "Dieses Passwort wurde nicht akzeptiert. Wähle ein anderes.",
  // "speichern", nicht "setzen": gesetzt wird eine Variable, und "speichern" ist
  // genau das Verb, das auf dem Button darunter steht. Drei Sätze statt eines
  // Komma-Spleißes zwischen Aussage und Aufforderung.
  "auth.resetServerFailed":
    "Wir konnten dein neues Passwort gerade nicht speichern. Dein Link bleibt gültig. Versuche es gleich noch einmal.",
  // Nicht "setze … erneut": das liest sich als "zurücksetzen", und dieser Schritt
  // liegt hinter dem Zurücksetzen.
  "auth.resetRateLimited":
    "Zu viele Versuche. Warte einen Moment und speichere dein Passwort dann erneut.",
  "auth.requestNewLink": "Neuen Link anfordern",
  "auth.askAdminForNewLink":
    "Bitte deine Administratorin oder deinen Administrator um einen neuen Passwort-Link.",
  // "geändert", wie im Satz darunter: aktualisiert werden Daten, die veralten.
  "auth.resetDoneTitle": "Passwort geändert",
  // "beendet", nicht "abgemeldet": abmelden tut sich eine Person, eine Sitzung
  // wird beendet.
  "auth.resetDoneBody":
    "Dein Passwort ist geändert und alle anderen Sitzungen sind beendet. Melde dich mit dem neuen Passwort an.",
  "auth.backToLogin": "Zurück zur Anmeldung",
  "auth.signOut": "Abmelden",

  "client.back": "Zurück zu Margince",
  "client.title": "Margince neben deinem Postfach",
  "client.sub": "die Extension-Oberfläche — ohne Shell, kennt den Datensatz",
  "client.sender": "Absender",
  "client.lookup": "Nachschlagen",
  "client.open360": "360 öffnen",
  "client.unknown": "Noch nicht in deiner Organisation.",
  "client.unknownDetail":
    "Dieser Absender passt zu keinem Kontakt, den du sehen kannst. Von woanders wurde nichts geholt.",
  "client.createLead": "Als Lead erfassen",
  "client.isolation": "spricht nur mit DEINER Organisation",
  "client.attribution": "Jede Erfassung ist zugeordnet und prüfbar.",

  "book.title": "Termin buchen",
  "book.sub": "echte Verfügbarkeit aus dem verbundenen Kalender",
  "book.min15": "15 Min.",
  "book.min30": "30 Min.",
  "book.min60": "60 Min.",
  "book.attendee": "Teilnehmer-E-Mail",
  "book.welcomeBack": "Erkannt: {name}",
  "book.subject": "Termin über Margince",
  "book.confirmed": "Gebucht. Die Einladung ist unterwegs.",
  "book.failed": "Die Buchung ging nicht durch — es wurde nichts eingetragen.",
  "book.publicSub": "Slot auswählen — ganz ohne Konto",
  "book.name": "Dein Name",
  "book.email": "Deine E-Mail",
  "book.consentWording":
    "Ich bin einverstanden, dass mein Name und meine E-Mail gespeichert werden, um diesen Termin zu vereinbaren und nachzufassen.",

  "prefs.title": "Wähle, was du von uns hörst",
  "prefs.sub":
    "Jeder Zweck steht für sich — hier ist nicht alles oder nichts. Transaktionale Nachrichten lassen sich hier nicht abschalten, weil du sie brauchst; alles andere bestimmst du selbst.",
  "prefs.unsub.title": "Diese E-Mails nicht mehr erhalten?",
  "prefs.unsub.lead":
    "Ein Klick stoppt Nachrichten dieser Art an deine Adresse. Sonst \u00e4ndert sich nichts.",
  "prefs.unsub.loading":
    "Deine E-Mail-Einstellungen werden ge\u00f6ffnet \u2026",
  "prefs.unsub.afterTitle": "Was danach passiert",
  "prefs.unsub.afterBody":
    "Wir senden dir keine weiteren E-Mails dieser Art. Sicherheits- und Servicenachrichten, die du f\u00fcr einen von dir angeforderten Vorgang brauchst, bleiben davon unber\u00fchrt.",
  "prefs.unsub.confirm": "Diese E-Mails abbestellen",
  "prefs.unsub.busy": "Deine Entscheidung wird gespeichert \u2026",
  "prefs.unsub.seeAll": "Alle Einstellungen ansehen",
  "prefs.unsub.privacy":
    "Kein Login n\u00f6tig. Dieser pers\u00f6nliche Link gilt nur f\u00fcr deine E-Mail-Einstellungen \u2014 bitte teile ihn nicht.",
  "prefs.unsub.doneTitle": "Abbestellung best\u00e4tigt",
  "prefs.unsub.doneBody":
    "Du erh\u00e4ltst {label} nicht mehr von uns. Die \u00c4nderung gilt sofort.",
  "prefs.unsub.manage": "Einstellungen verwalten",
  "prefs.unsub.alreadyOff":
    "Diese E-Mails waren bereits abgeschaltet. Es wurde nichts ge\u00e4ndert.",
  "prefs.unsub.lockedTitle": "Diese Nachrichten lassen sich nicht abschalten",
  "prefs.unsub.lockedBody":
    "Sie geh\u00f6ren zu einem von dir angeforderten Vorgang \u2014 etwa ein neues Passwort oder eine Best\u00e4tigung, um die du gebeten hast.",
  "prefs.unsub.retry": "Erneut versuchen",
  "prefs.unsub.unknownPurpose":
    "Dieser Link nennt keine Art von E-Mail, die wir versenden. \u00d6ffne deine Einstellungen, um alles zu sehen.",
  "prefs.purpose.business_correspondence": "Persönliche Nachrichten",
  "prefs.purpose.marketing_email": "Produktneuigkeiten",
  "prefs.purpose.transactional": "Sicherheit & laufende Vorgänge",
  "prefs.sentVia": "Gesendet über Margince",
  "prefs.noObjection": "An — du hast dem nicht widersprochen",
  "prefs.optedOut": "Aus — du hast uns gebeten, damit aufzuhören",
  "prefs.invalidLink":
    "Dieser Link ist nicht mehr gültig. Präferenz-Links laufen ab oder können widerrufen werden — frag in einer aktuellen E-Mail nach einem neuen.",
  "buyer.opening": "Ihr Deal Room wird geöffnet …",
  "buyer.deadTitle": "Dieser Link funktioniert nicht mehr",
  "buyer.deadAskContact":
    "Bitten Sie Ihren Ansprechpartner um einen neuen Link.",
  "buyer.linkDead":
    "Der Link wurde bereits geöffnet, ist abgelaufen oder durch einen neueren ersetzt worden. Fordern Sie unten einen neuen Link an.",
  "buyer.noLink":
    "Öffnen Sie diese Seite über den Link, den Sie erhalten haben. Falls Sie ihn nicht mehr haben, fordern Sie unten einen neuen an.",
  "buyer.emailLabel": "Ihre E-Mail-Adresse",
  "buyer.emailHint": "Die Adresse, an die die Einladung ging.",
  "buyer.requestLink": "Neuen Link schicken",
  "buyer.linkRequested":
    "Falls diese Adresse eingeladen wurde, ist ein neuer Link unterwegs.",
  "buyer.pausedTitle": "Zugang pausiert",
  "buyer.pausedBody":
    "{steward} hat diesen Raum vorerst pausiert. Ihr Link bleibt gültig; sobald der Raum wieder geöffnet wird, können Sie weitermachen.",
  "buyer.expiredTitle": "Zugang beendet",
  "buyer.expiredBody":
    "Der Zugang zu diesem Raum ist abgelaufen. Wenden Sie sich an {steward} oder fordern Sie unten einen neuen Link an.",
  "buyer.eyebrow": "Ihr Deal Room",
  "buyer.contact": "Ihr Ansprechpartner: {steward}.",
  "buyer.closed":
    "Dieser Raum ist geschlossen; das Geteilte ist jetzt ein Protokoll.",
  "buyer.previewBanner":
    "Sie sehen diesen Raum als Vorschau, so wie ein Käufer ihn sieht. Sie können alles lesen und nichts ändern.",
  "buyer.previewReadOnly":
    "Eine Vorschau kann nicht schreiben. Schließen Sie diesen Tab, um zur Deal-Room-Seite zurückzukehren.",
  "buyer.closedNote": "Dieser Raum ist jetzt schreibgeschützt.",
  "buyer.stewardUnknown": "Ihr Ansprechpartner",
  "buyer.signOut": "Abmelden",
  "room.docs.title": "Dokumente",
  "room.docs.sub":
    "Was der Käufer lesen kann, mit dem Gespräch zu jedem Dokument darunter.",
  "room.docs.empty": "Noch keine Dokumente im Raum.",
  "room.docs.fileLabel": "Datei aus diesem Deal",
  "room.docs.fileHint":
    "Alles aus dem Dateibereich des Deals kann hinein: Uploads und die Dateien seiner E-Mails.",
  "room.docs.pickFile": "Datei wählen",
  "room.docs.noFiles": "Der Dateibereich des Deals ist leer",
  "room.docs.groupLabel": "Gruppe",
  "room.docs.add": "In den Raum legen",
  "room.docs.remove": "{title} aus dem Raum entfernen",
  "room.docs.group.commercial": "Kommerziell",
  "room.docs.group.legal": "Rechtliches",
  "room.docs.group.security_privacy": "Sicherheit & Datenschutz",
  "room.docs.group.delivery_operations": "Lieferung & Betrieb",
  "buyer.docs.title": "Dokumente",
  "buyer.docs.sub":
    "Was mit Ihnen geteilt wurde, mit dem Gespräch zu jedem Dokument darunter.",
  "buyer.docs.empty": "Noch keine Dokumente.",
  "buyer.docs.download": "{title} herunterladen",
  "buyer.docs.downloadFailed":
    "Der Download hat nicht begonnen. Versuchen Sie es erneut oder wenden Sie sich an Ihren Ansprechpartner.",
  "buyer.docs.downloadShort": "Herunterladen",
  "buyer.poweredBy": "Powered by",
  "buyer.poweredByMargince": "Powered by Margince",
  "threads.roomTitle": "Der Raum als Ganzes",
  "threads.roomSub": "Alles, was nicht ein einzelnes Dokument betrifft.",
  "threads.aboutThis_other": "{count} Threads zu diesem Dokument",
  "threads.aboutThis_one": "{count} Thread zu diesem Dokument",
  "threads.askAbout": "Zu diesem Dokument fragen",
  "threads.cancel": "Abbrechen",
  "threads.empty": "Noch nichts gesagt.",
  "threads.requiredChange": "Änderung nötig",
  "threads.resolved": "Erledigt",
  "threads.sideBuyer": "Käufer",
  "threads.sideSeller": "Anbieter",
  "threads.replyLabel": "Antwort",
  "threads.reply": "Antworten",
  "threads.resolve": "Erledigen",
  "threads.newLabel": "Neuer Thread",
  "threads.requireChangeLabel": "Dieses Dokument muss geändert werden",
  "threads.open": "Absenden",
  "threads.readOnly": "Ihr Zugang ist schreibgeschützt.",
  "deal360.blocker": "Was den Deal aufhält",
  "deal360.buyer": "Was der Käufer will",
  "deal360.verdict.live": "Aktiv",
  "deal360.verdict.drifting": "Schläft ein",
  "deal360.verdict.blocked": "Blockiert",
  "deal360.verdict.cold": "Kalt",
  "dealmail.title": "E-Mail",
  "dealmail.sub.reply":
    "Sie haben geschrieben, und noch hat niemand geantwortet.",
  "dealmail.sub.fresh": "Schreiben Sie den Beteiligten dieses Deals.",
  "dealmail.reply": "Antwort entwerfen",
  "dealmail.send": "E-Mail senden",
  "recordmail.title": "E-Mail",
  "recordmail.sub.reply": "Eine Antwort steht noch aus.",
  "recordmail.sub.fresh": "Schreiben Sie ihnen.",
  "recordmail.reply": "Antwort entwerfen",
  "recordmail.send": "E-Mail schreiben",
  "deal360.rewrite": "Neu schreiben",
  "deal360.readFull": "Vollständige Einschätzung lesen",
  "deal360.createTask": "Aufgabe anlegen",
  "deal360.openBrief": "Meeting-Briefing öffnen",
  "deal360.unreadable":
    "Dieses Briefing konnte nicht gelesen werden. Seite neu laden oder neu schreiben.",
  "prefs.rateLimited":
    "Gerade zu viele Versuche von hier aus. Warte eine Minute und lade neu.",
  "prefs.subscribed": "An — du hast danach gefragt",
  "prefs.alwaysOn": "immer an",
  "confirm.title": "Ihre Daten",
  "confirm.intro":
    "Ich bin Margince, die KI hinter diesem CRM. Hier steht alles, was wir über Sie gespeichert haben. Sie können es ändern oder uns bitten, es zu löschen.",
  "confirm.card.title": "Was wir gespeichert haben",
  "confirm.field.fullName": "Name",
  "confirm.field.title": "Position",
  "confirm.field.email": "E-Mail",
  "confirm.field.phone": "Telefon",
  "confirm.field.company": "Unternehmen",
  "confirm.field.none": "Nicht erfasst",
  "confirm.marketing.title": "Dürfen wir in Kontakt bleiben?",
  "confirm.marketing.ask":
    "Neuigkeiten ab und zu, etwa einmal im Monat. Sie entscheiden, ich halte mich daran.",
  "confirm.marketing.yes": "Ja, halten Sie mich auf dem Laufenden",
  "confirm.marketing.no": "Nein danke, nur meine Daten korrekt halten",
  "confirm.provenance.title": "Woher wir Ihre Daten haben",
  "confirm.provenance.empty": "Zur Herkunft ist nichts erfasst.",
  "confirm.provenance.line": "{field}: aus {source}, erfasst am {date}",
  "confirm.erasure.ask": "Meine Daten löschen",
  "confirm.erasure.staged": "Löschung angefragt. Zum Senden unten bestätigen.",
  "confirm.submit": "Bestätigen",
  "confirm.done.title": "Danke",
  "confirm.done.body":
    "Ich habe Ihre Antwort erfasst. Änderungen gehen an eine Person hier zur Übernahme, und dieser Link ist jetzt verbraucht.",
  "confirm.invalidLink":
    "Dieser Link ist nicht mehr gültig. Er wurde möglicherweise schon benutzt oder ist abgelaufen.",
  "prefs.lockedWhy":
    "Gehört zu etwas, das du angefordert hast, und bleibt deshalb an.",
  "prefs.confirmationNeededWhy":
    "Zum Einschalten nutze den Bestätigungslink aus unserer E-Mail. Ausschalten kannst du es hier jederzeit.",
  "prefs.notSaved": "Noch nicht gespeichert.",
  "prefs.savePending": "Ausstehend: {changes}.",
  "prefs.saveProof":
    "Wir speichern den genauen Wortlaut, den du gesehen hast, und einen Zeitstempel als Nachweis — danach gilt er für jeden künftigen Versand.",
  "prefs.save": "Einstellungen speichern",
  "prefs.discard": "Verwerfen",
  "prefs.partialSave":
    "Beim Speichern ist etwas schiefgelaufen. Einige deiner Entscheidungen wurden möglicherweise schon übernommen — wir haben deinen aktuellen Stand neu geladen, damit du genau siehst, wo du stehst.",
  "prefs.wording.business_correspondence":
    "„Schick mir Antworten und direkte Nachrichten zu unseren Gesprächen.“",
  "prefs.wording.transactional":
    "„Schick mir, was ich für einen von mir angeforderten Vorgang brauche.“",
  "prefs.wordingGeneric": "„Schick mir {label}.“",
  "prefs.wording.marketing_email":
    "„Schick mir Produkt-Updates und gelegentliche Marketing-E-Mails.“",
  "prefs.wording.events": "„Schick mir Einladungen zu Events und Webinaren.“",
  "prefs.unsubscribeAll": "Alles abschalten, was ich abschalten kann",
  "prefs.unsubscribeAllHint":
    "Das schaltet jede Zeile oben aus, die ein anklickbares Kästchen hat. Zeilen mit IMMER AN bleiben an — die brauchst du für Dinge, die du selbst angefordert hast.",
  "prefs.oneClickDone":
    "Erledigt — du bekommst keine Marketing-E-Mails mehr von uns. Das gilt sofort für jede Kampagne.",
  "prefs.oneClickAlreadyOff": "Nichts zu tun — das war bereits abgeschaltet.",
  "prefs.undo": "Rückgängig — Marketing weiter erhalten",
  "prefs.undoExplicit":
    "Ein erneutes Abonnieren ist eine ausdrückliche Zustimmung — wir schalten es nicht stillschweigend wieder ein. Speichere unten, um deine Zustimmung festzuhalten, oder verwirf.",

  "auto.tier.runs": "läuft",
  "auto.tier.approval": "Freigabe",
  "auto.sub":
    'Eine Regel mit "läuft" handelt selbstständig. Eine mit "Freigabe" wandert in den Freigabe-Eingang.',
  "auto.readOnly":
    "Nur-Lese-Ansicht — du hast keine Berechtigung, Automatisierungen zu ändern.",
  "auto.catalog": "Starter-Bibliothek",
  "auto.catalogSub": "die geschlossene Menge an Automatisierungstypen",
  "auto.instances": "Eingerichtete Automatisierungen",
  "auto.use": "Vorlage verwenden",
  "auto.name": "Name",
  "auto.create": "Anlegen",
  "auto.createdPaused":
    "Pausiert angelegt — es läuft nichts, bis du aktivierst.",
  "auto.delete": "Löschen",
  "auto.statusEnabled": "aktiv",
  "auto.statusPaused": "pausiert",
  "auto.dateField.placeholder": "Datumsfeld auswählen",
  "auto.dateField.needsObject":
    "Wähle zuerst ein Objekt aus, um dessen Datumsfelder anzuzeigen.",
  "auto.dateField.empty": "Dieses Objekt hat noch keine aktiven Datumsfelder.",
  "auto.dateField.loadError":
    "Die Datumsfelder dieses Objekts konnten nicht geladen werden. Erneut versuchen.",
  "auto.enabledFor": "{name} ist aktiv",
  "auto.rowActions": "Aktionen für {name}",
  "auto.withheld":
    "Die eingerichteten Automatisierungen sind ausgeblendet — deine Rolle darf sie nicht lesen.",
  "auto.deleteTitle": "Diese Automatisierung löschen?",
  "auto.deleteBody":
    "„{name}“ wird mitsamt den Einstellungen endgültig entfernt. Wenn sie nur nicht mehr auslösen soll, schalte sie stattdessen aus.",

  "auto.runs.open": "Läufe",
  "auto.runs.title": "Laufverlauf",
  "auto.runs.filterAll": "Alle",
  "auto.runs.filterFired": "Ausgelöst",
  "auto.runs.filterFailed": "Fehlgeschlagen",
  "auto.runs.filterBlocked": "Blockiert",
  "auto.runs.filterSkipped": "Übersprungen",
  "auto.runs.filterQueued": "Zur Freigabe eingereiht",
  "auto.runs.empty": "Diese Automatisierung wurde noch nicht ausgelöst.",
  "auto.runs.emptyFiltered": "Keine Läufe mit diesem Ergebnis.",
  "auto.runs.needsApproval": "Freigabe erforderlich",
  "auto.runs.why": "Warum",
  "auto.runs.target": "Ziel",
  "auto.runs.result": "Ergebnis",
  "auto.runs.reason": "Grund",
  "auto.runs.outcomeFired": "ausgelöst",
  "auto.runs.outcomeFailed": "fehlgeschlagen",
  "auto.runs.outcomeBlocked": "blockiert",
  "auto.runs.outcomeSkipped": "übersprungen",
  "auto.runs.outcomeQueued": "eingereiht",

  "auto.preview.open": "Vorschau",
  "auto.preview.title": "Probelauf – Reichweite",
  "auto.preview.window": "Zeitfenster",
  "auto.preview.window7": "7 T",
  "auto.preview.window30": "30 T",
  "auto.preview.window90": "90 T",
  "auto.preview.matchesNow": "Treffer jetzt: {n}",
  "auto.preview.wouldFire": "Würde auslösen: ~{n} / {days} T",
  "auto.preview.notComputable": "Rückblickende Schätzung nicht möglich",
  "auto.preview.hidden": "{n} ausgeblendet – kein Zugriff",
  "auto.preview.explainer":
    "Ein reiner Probelauf – es werden keine Datensätze geändert und nichts gesendet.",

  "strength.title": "Beziehungsstärke",
  "strength.score": "Score {score}/100",
  "strength.bucket.none": "Ruhend",
  "strength.bucket.weak": "Schwach",
  "strength.bucket.moderate": "Warm",
  "strength.bucket.strong": "Stark",
  "strength.factor.recency": "Aktualität",
  "strength.factor.frequency": "Häufigkeit",
  "strength.factor.reciprocity": "Reziprozität",
  "strength.factor.direction": "Richtung",
  "strength.lastInteraction": "Letzte Interaktion: {when}",
  "strength.none": "Noch keine Interaktionen",
  "strength.inout": "{in} eingehend · {out} ausgehend (90 Tage)",
  "strength.computedFrom": "Berechnet aus {count} Aktivitäten",

  // Die Beziehungsgraph-Karten (ADR-0078). Die Kollegen-Stufen sind die von
  // PO-F-3b und unterscheiden sich bewusst von denen der arbeitsbereichsweiten
  // Karte: beide messen Verschiedenes und dürfen nicht vergleichbar wirken.
  "network.title": "Wer kennt diese Person bei uns",
  "network.empty":
    "Niemand hier hat bisher erfassten Kontakt zu dieser Person.",
  "network.interactions": "{count} Interaktionen (90 Tage)",
  "network.neverSpoken": "Kein erfasster Kontakt",
  "network.bucket.none": "Kein Kontakt",
  "network.bucket.weak": "Schwach",
  "network.bucket.moderate": "Mittel",
  "network.bucket.strong": "Stark",
  "coverage.engaged": "Im Austausch",
  "coverage.quiet": "Kein beidseitiger Kontakt",
  "coverage.seatWithheld": "Ein Kontakt, den Sie nicht lesen dürfen",
  "coverage.daysSinceTouch": "{days} Tage",
  "coverage.risk.single_threaded_theirs": "Nur ein Kontakt",
  "coverage.risk.single_threaded_ours": "Von einer Person getragen",
  "coverage.risk.coverage_gap": "Kein engagierter Fürsprecher",
  "coverage.risk.champion_left": "Fürsprecher hat gekündigt",
  "coverage.risk.stakeholder_left": "Stakeholder hat gekündigt",
  "coverage.risk.going_cold": "Wird kalt",

  "cf.title": "Benutzerdefinierte Felder",
  "cf.formSection": "Benutzerdefinierte Felder",
  "cf.subtitle":
    "Füge einem vorhandenen Objekt ein einfaches typisiertes Feld hinzu — zur Laufzeit, ohne Entwickler, ohne Deploy. Neue Objekte und Beziehungen laufen weiterhin über Code.",
  "cf.object": "Objekt",
  "cf.obj.deal": "Deal",
  "cf.obj.organization": "Firma",
  "cf.obj.person": "Kontakt",
  "cf.obj.lead": "Lead",
  "cf.listLabel": "Felder auf {object}",
  "cf.col.field": "Feld",
  "cf.col.type": "Typ",
  "cf.col.addedBy": "Hinzugefügt von",
  "cf.addedByYou": "Du",
  "cf.addedByAdmin": "Admin",
  "cf.empty.deal":
    "Noch keine benutzerdefinierten Felder auf Deal. Füge eines hinzu, wenn du etwas erfasst, das wir nicht mitgeliefert haben.",
  "cf.empty.organization":
    "Noch keine benutzerdefinierten Felder auf Firma. Füge eines hinzu, wenn du etwas erfasst, das wir nicht mitgeliefert haben.",
  "cf.empty.person":
    "Noch keine benutzerdefinierten Felder auf Kontakt. Die Kernfelder decken den Kontaktdatensatz ab; füge eines hinzu, wenn du mehr erfasst.",
  "cf.empty.lead":
    "Noch keine benutzerdefinierten Felder auf Lead. Ein Feld, das du hier hinzufügst, erscheint auch, sobald ein Lead zu einem Kontakt befördert wird.",
  "cf.type.text": "Text",
  "cf.type.number": "Zahl",
  "cf.type.date": "Datum",
  "cf.type.currency": "Währung",
  "cf.type.picklist": "Auswahlliste",
  "cf.type.boolean": "Ja / Nein",
  "cf.builder.addTo": "Feld zu {object} hinzufügen",
  "cf.builder.open": "Feld hinzufügen",
  "cf.builder.noCode": "ohne Code",
  "cf.builder.intro":
    "Ein neues Feld ist eine echte Spalte auf der bestehenden Tabelle — es filtert, erscheint in Berichten, Exporten und in der API wie jedes Kernfeld. Es ist kein neues Objekt.",
  "cf.label": "Bezeichnung",
  "cf.apiKey": "API-Schlüssel",
  "cf.apiKeyHint":
    "Automatisch abgeleitet, unveränderlich sobald live. Mit cf_ präfixiert, damit er nie mit einem Kernfeld kollidiert.",
  "cf.typeLabel": "Typ",
  "cf.currencyCode": "Währungscode",
  "cf.currencyHint":
    "Dreibuchstabiger ISO-4217-Code (z. B. EUR, USD). Geld wird auf den Cent genau gespeichert.",
  "cf.options": "Optionen",
  "cf.addOption": "Option hinzufügen",
  "cf.removeOption": "Option entfernen",
  "cf.optionPlaceholder": "Optionsbezeichnung",
  "cf.lastOptionBlocked": "Eine Auswahlliste braucht mindestens eine Option",
  "cf.gate.title": "Ein Feld hinzuzufügen ist bestätigungspflichtig.",
  "cf.gate.body":
    "Bei Bestätigung wird es zu einer Live-Spalte auf jedem {object} — auf der 360, in Suche & Filtern, Listen, Export und der API. Das Hinzufügen wird im Audit-Trail festgehalten.",
  "cf.refuse.title":
    "Das sieht nach einem neuen Objekt oder einer Beziehung aus, nicht nach einem Feld.",
  "cf.refuse.body":
    "Dieser Builder fügt nur einfache Felder zu bestehenden Datensätzen hinzu. Ein neues Objekt, eine Verknüpfung zwischen Objekten oder ein berechneter Roll-up ist eine strukturelle Änderung — sie kommt als geprüfte Änderung an Margince in einer neuen Version, gemacht von Menschen, nicht vom Produkt, das seinen eigenen Code bearbeitet.",
  "cf.refuse.route":
    "Leite es über den Entwicklungsweg — deine eigenen Entwickler, einen Implementierungspartner oder Margince-Services.",
  "cf.confirm": "Bestätigen & Feld hinzufügen",
  "cf.writing": "wird geschrieben…",
  "cf.added":
    'Feld "{label}" hinzugefügt — live auf 360, Filtern, Export & API',
  "cf.edit": "Bezeichnung bearbeiten",
  "cf.archive": "Feld archivieren",
  "cf.archived":
    '"{label}" archiviert — aus neuen Datensätzen ausgeblendet, in Audit & Historie behalten (umkehrbar)',
  "cf.renamePrompt": "Neue Bezeichnung",
  "cf.renamed": 'Umbenannt in "{label}"',
  "cf.audit.title": "Letzte Feldänderungen",
  "cf.audit.empty": "Noch keine Änderungen an benutzerdefinierten Feldern.",
  "cf.audit.footer":
    "Jedes Hinzufügen / Bearbeiten / Archivieren wird dauerhaft im Audit-Log festgehalten.",
  "cf.noPermission":
    "Du hast nur Lesezugriff auf benutzerdefinierte Felder — Anlegen, Bearbeiten und Archivieren sind hier nicht deine Sache.",
  "cf.retired": "Archiviert",
  // "Allgemein" statt "Organisation" für den ersten Eintrag: die Gruppen-
  // überschrift darüber sagt das Wort schon, und eine Zeile, die ihre eigene
  // Überschrift wiederholt, benennt nichts.
  "settings.tab.account": "Konto",
  "settings.tab.voice": "Schreibstimme",
  "settings.tab.agents": "Agenten",
  "settings.tab.connections": "Verbindungen",
  "settings.tab.general": "Allgemein",
  "settings.tab.users": "Benutzer & Teams",
  "settings.tab.extensions": "Erweiterungen",
  "settings.tab.integrations": "Integrationen",
  "settings.tab.capture": "Erfassung",
  "settings.tab.data-model": "Datenmodell",
  "settings.tab.ai": "KI",
  "settings.tab.knowledge": "Wissen",
  "corpusAsk.title": "Ihre Dokumente fragen",
  "corpusAsk.sub":
    "Eine Frage in eigenen Worten, beantwortet ausschließlich aus einer Dokumentensammlung dieser Organisation. Was die Sammlung nicht abdeckt, wird abgelehnt statt geraten, und jeder Satz nennt die Textstelle, auf der er beruht.",
  "corpusAsk.whichSet": "Welche Sammlung",
  "corpusAsk.question": "Ihre Frage",
  "corpusAsk.submit": "Fragen",
  "corpusAsk.byModel": "Aus den Textstellen unten geschrieben",
  "corpusAsk.atLine": "Zeile {line}, Spalte {column}",
  "corpusAsk.byPassages":
    "Die Textstellen selbst — niemand hat eine Zusammenfassung geschrieben",
  "corpusAsk.notReady":
    "Diese Sammlung ist noch nicht fertig eingelesen — {embedded} von {total} Abschnitten sind durchsuchbar. An Ihrer Frage liegt es nicht; versuchen Sie es gleich noch einmal.",
  "corpusAsk.retrievalUnavailable":
    "Es wurde nichts durchsucht: in dieser Installation ist kein Suchindex eingerichtet, die Dokumente konnten also nicht angesehen werden. Das ist eine Frage der Einrichtung und liegt nicht an Ihrer Frage.",
  "corpusAsk.unreviewed":
    "Die Suche hat diese Textstellen als die zu Ihrer Frage passendsten gefunden. Gelesen hat sie niemand, also hat auch niemand beurteilt, ob sie die Frage beantworten.",
  "corpusAsk.notCovered.title": "Von dieser Sammlung nicht abgedeckt",
  "corpusAsk.notCovered.body":
    "{name} wurde vollständig durchsucht und enthält nichts, was nah genug daran wäre. Die Sammlung deckt ab:",
  "knowledge.title": "Dokumentensammlungen",
  "knowledge.sub":
    "Textbestände, zu denen diese Organisation befragt werden kann. Eine Antwort stammt ausschließlich aus dem, was hier abgelegt ist; eine Frage, die sie nicht abdecken, wird abgelehnt statt geraten.",
  "knowledge.withheld":
    "Welche Dokumentensammlungen es gibt, dürfen Sie nicht sehen.",
  "knowledge.coverage":
    "{documents} Dokumente · {embedded} von {total} Abschnitten durchsuchbar",
  "knowledge.reindexing":
    "Diese Sammlung wird nach einer Änderung der Textindizierung neu gelesen. Eine Frage meldet bis dahin, dass sie nicht bereit ist; verloren ist nichts.",
  "knowledge.showDocuments": "Dokumente anzeigen",
  "knowledge.hideDocuments": "Dokumente ausblenden",
  "knowledge.documents": "Dokumente",
  "knowledge.noDocuments": "Hier ist noch nichts abgelegt.",
  "knowledge.archive": "Sammlung archivieren",
  "knowledge.archiveConfirm.title": "Diese Dokumentensammlung archivieren?",
  "knowledge.archiveConfirm.body":
    "Die Sammlung und alles darin sind nicht mehr durchsuchbar. Zerstört wird nichts.",
  "knowledge.deleteDocument": "Löschen",
  "knowledge.deleteConfirm.title": "Dieses Dokument löschen?",
  "knowledge.deleteConfirm.body":
    "Die Datei, der daraus gewonnene Text und der darauf aufgebaute Suchindex werden zerstört. Das lässt sich nicht rückgängig machen.",
  "knowledge.ingest.queued": "Wartet auf Verarbeitung",
  "knowledge.ingest.running": "Wird gelesen",
  "knowledge.ingest.done": "Durchsuchbar",
  "knowledge.ingest.failed": "Konnte nicht gelesen werden",
  "knowledge.upload.label": "Dokument hinzufügen",
  "knowledge.upload.hint":
    "Reiner Text, Markdown, CSV oder JSON. Für PDFs oder Word-Dateien gibt es hier keinen Leser; sie werden abgelehnt statt leer abgelegt.",
  "knowledge.upload.empty": "Textdatei hierher ziehen oder auswählen",
  "knowledge.upload.submit_other": "{count} Dokumente hinzufügen",
  "knowledge.upload.refused": "{filename} wurde nicht hinzugefügt: {message}",
  "knowledge.upload.submit_one": "Dokument hinzufügen",
  "knowledge.new.title": "Neue Dokumentensammlung",
  "knowledge.new.name": "Name",
  "knowledge.new.topic": "Was diese Sammlung abdeckt",
  "knowledge.new.topicHint":
    "Schreiben Sie einen Satz, kein Schlagwort. Er wird demjenigen zitiert, dessen Frage diese Sammlung nicht abdeckt — also im ungeduldigsten Moment gelesen.",
  "knowledge.new.submit": "Sammlung anlegen",
  "settings.tab.privacy": "Datenschutz & Audit",
  "settings.tab.capture-activity": "Erfassungsaktivität",
  "captureActivity.title": "Erfassungsaktivität",
  "captureActivity.sub":
    "Was aus Ihrer Post der letzten 24 Stunden geworden ist. Die Absender, die Sie ausschließen, stehen darüber.",
  "captureActivity.scope.label": "Wessen Aktivität",
  "captureActivity.outcomes": "Ergebnisse",
  "captureActivity.messages": "Nachrichten",
  "captureActivity.scope.mine": "Meine",
  "captureActivity.scope.workspace": "Gemeinsame Kanäle",
  "captureActivity.scopeNote":
    "Gezählt ab dem Moment, in dem ein Connector eine Nachricht an dieses CRM übergibt. Was ein Connector auf seiner Seite gefiltert hat — eine Chat-Reaktion, eine Mail-Regel — ist nicht enthalten. Umfasst Nachrichten; Lead-Erfassung wird hier nicht gezeigt.",
  "captureActivity.filtered":
    "{shown} von {total} {outcome} in diesem Zeitraum.",
  "captureActivity.openTrace": "Jeden Schritt dieser Nachricht ansehen",
  "captureActivity.emptyFiltered":
    "keine der geladenen Zeilen passt — laden Sie mehr, um den Rest des Zeitraums zu erreichen",
  "captureActivity.loadMore": "Mehr laden",
  "captureActivity.empty":
    "keine Erfassungsaktivität in den letzten 24 Stunden",
  "captureActivity.payloadsOff":
    "Diese Installation speichert weder den Absender einer Nachricht noch ihren Betreff. Die Zeilen unten nennen daher nur die Entscheidung.",
  "captureActivity.contentNone": "kein Absender erfasst",
  "captureActivity.outcome.captured": "Erfasst",
  "captureActivity.outcome.internal": "Als intern verworfen",
  "captureActivity.outcome.suppressed": "Kein Kontakt angelegt",
  "captureActivity.outcome.deferred": "Wartet auf Beurteilung",
  "captureActivity.outcome.fault": "Ableitung fehlgeschlagen",
  "captureActivity.reason.internal_only":
    "alle Beteiligten waren auf Ihren eigenen Domains",
  "captureActivity.reason.deferral_capped":
    "das Limit offener Fragen war erreicht, es kommt keine Beurteilung",
  "captureActivity.reason.noise_prior":
    "eine frühere Beurteilung stufte diesen Absender als Rauschen ein, die Nachricht wird archiviert",
  "captureActivity.reason.decided_prior":
    "über diesen Absender wurde bereits entschieden, es wird kein Kontakt angelegt",
  "captureActivity.reason.no_granting_human":
    "die Verbindung nannte kein Mitglied, für das gehandelt werden kann",
  "captureActivity.reason.invisible_incumbent":
    "sie traf auf einen Datensatz außerhalb Ihrer Sicht",
  "captureActivity.reason.derivation_failed":
    "der Kontaktschritt schlug fehl; die Nachricht selbst ist unberührt",
  "captureActivity.reason.no_counterparty":
    "kein Absender, den dieses CRM erfassen konnte",
  "captureActivity.reason.role_mailbox":
    "ein Sammelpostfach, keine Person — gespeichert, aber kein Kontakt angelegt",
  "captureActivity.reason.private_thread":
    "ein privater Austausch — für Sie gespeichert, aber kein Kontakt angelegt",
  "captureActivity.reason.transactional_infra":
    "der Absender ist Mail-Infrastruktur, kein Unternehmen, mit dem Sie arbeiten",
  "captureActivity.reason.transactional_prefix":
    "der Absender wirkt wie ein automatischer Versender, keine Person",
  "captureActivity.outcome.deferred_capped": "Nicht eingereiht",
  "captureActivity.outcome.deferred_sent": "Zur Entscheidung vorgelegt",
  "captureActivity.resolution.pending": "wartet noch",
  "captureActivity.resolution.unsure": "an die Prüfliste gesendet",
  "captureActivity.resolution.real": "als echter Kontakt beurteilt",
  "captureActivity.resolution.noise": "als Rauschen beurteilt",
  "captureActivity.resolution.rejected": "von einer Person abgelehnt",
  "captureActivity.resolution.suppressed": "unterdrückt",
  "pipeline.title": "Wie diese Nachricht verarbeitet wurde",
  "pipeline.sub":
    "Jeder Schritt der Erfassung, in der Reihenfolge, in der diese Nachricht sie durchlaufen hat.",
  "pipeline.payloadsOff":
    "Zu keinem Schritt sind Absender oder Betreff gespeichert: diese Installation hat die Inhaltserfassung deaktiviert.",
  "pipeline.transport": "Übertragen über",
  "pipeline.unavailable":
    "die Verarbeitungsschritte dieser Nachricht konnten nicht gelesen werden",
  "pipeline.status.done": "Erledigt",
  "pipeline.status.skipped": "Übersprungen",
  "pipeline.status.pending": "Wartet",
  "pipeline.status.failed": "Fehlgeschlagen",
  "pipeline.status.not_applicable": "Nicht zutreffend",
  "pipeline.status.unknown": "Nicht feststellbar",
  "pipeline.reason.record_not_available":
    "der Eintrag zu diesem Schritt wird nicht mehr aufbewahrt, oder er ist nicht für Sie lesbar — ist der Eintrag fort, lässt sich beides nicht mehr unterscheiden",
  "pipeline.status.not_reported": "Hier nicht ausgewiesen",
  "pipeline.subject.message": "zu dieser Nachricht",
  "pipeline.subject.sender": "zum Absender, nicht nur zu dieser Nachricht",
  "pipeline.subject.domain": "zur Domain des Absenders",
  "pipeline.subject.thread": "zum gesamten Gespräch",
  "pipeline.stage.connector_filter": "Filterung im Konnektor",
  "pipeline.stage.ingress_gate": "Zugangsprüfung",
  "pipeline.stage.erasure_check": "Löschprüfung",
  "pipeline.stage.internal_drop": "Prüfung auf rein interne Nachricht",
  "pipeline.stage.activity_write": "In der Chronik gespeichert",
  "pipeline.stage.tier_ladder": "Kontaktentscheidung",
  "pipeline.stage.person_create": "Kontakt angelegt",
  "pipeline.stage.verdict": "Absenderurteil",
  "pipeline.stage.company_triage": "Firmenprüfung",
  "pipeline.stage.attention_label": "Aufmerksamkeits-Label",
  "pipeline.stage.material_events": "Gesprächsauswertung",
  "pipeline.stage.claim_extraction": "Zusagen und offene Punkte",
  "pipeline.reason.internal_only":
    "alle Beteiligten lagen auf Ihren eigenen Domains",
  "pipeline.reason.invisible_incumbent":
    "sie passte zu einem Datensatz außerhalb Ihrer Sicht",
  "pipeline.reason.transactional_infra":
    "der Absender ist Mail-Infrastruktur, kein Unternehmen, mit dem Sie arbeiten",
  "pipeline.reason.transactional_prefix":
    "der Absender wirkt wie ein automatischer Versender, nicht wie eine Person",
  "pipeline.reason.deferral_capped":
    "die Grenze offener Fragen war erreicht, ein Urteil kommt nicht mehr",
  "pipeline.reason.noise_prior":
    "ein früheres Urteil hat diesen Absender als Rauschen eingestuft",
  "pipeline.reason.decided_prior":
    "über diesen Absender war bereits entschieden",
  "pipeline.reason.no_counterparty":
    "kein Absender, den dieses CRM erfassen konnte",
  "pipeline.reason.role_mailbox":
    "ein Sammelpostfach, keine Person — gespeichert, aber kein Kontakt angelegt",
  "pipeline.reason.private_thread":
    "ein privater Austausch — für Sie gespeichert, aber kein Kontakt angelegt",
  "pipeline.reason.no_granting_human":
    "die Verbindung nennt kein Mitglied, in dessen Namen gehandelt wird",
  "pipeline.reason.derivation_failed":
    "der Kontaktschritt ist fehlgeschlagen; die Nachricht selbst ist unberührt",
  "pipeline.reason.not_linked_yet":
    "mit dieser Nachricht ist noch kein Kontakt verknüpft",
  "pipeline.reason.no_contact_intended":
    "die Kontaktentscheidung ergab, dass keiner anzulegen war",
  "pipeline.reason.awaiting_verdict": "der Absender wartet noch auf ein Urteil",
  "pipeline.reason.verdict_reached": "für diesen Absender liegt ein Urteil vor",
  "pipeline.reason.no_open_question":
    "zu diesem Absender gab es keine offene Frage",
  "pipeline.reason.transport_not_read":
    "dieser Schritt liest nur E-Mail, und die Nachricht kam über einen anderen Kanal",
  "pipeline.reason.sender_undecided":
    "der Absender wartet noch auf ein Urteil, daher wird die Nachricht zurückgehalten",
  "pipeline.reason.archived": "die Nachricht ist archiviert",
  "pipeline.reason.not_connector_captured":
    "die Nachricht wurde nicht von einem Konnektor erfasst",
  "pipeline.reason.awaiting_batch":
    "sie ist zulässig und wartet auf den nächsten Durchlauf",
  "pipeline.reason.labelled": "die Nachricht wurde gelabelt",
  "pipeline.reason.not_comparable":
    "was ein Konnektor auf seiner eigenen Seite filtert, wird hier nicht gezählt — die Zahlen bedeuten je Konnektor Verschiedenes",
  "pipeline.reason.connector_side_defect":
    "Zugangsfehler sind ein Fehler der Verbindung, nicht einer einzelnen Nachricht",
  "pipeline.reason.would_restore_erased":
    "dies auszuweisen würde Daten wiederherstellen, die eine Löschung entfernt hat",
  "pipeline.reason.no_writer_yet": "diesen Schritt gibt es noch nicht",
  "pipeline.reason.not_reported_yet":
    "dieser Schritt läuft, wird hier aber noch nicht ausgewiesen",
  "settings.tab.maintenance": "Wartung",
  "settings.tab.license": "Lizenz",
  "license.card.title": "Lizenz und Sitzplätze",
  "license.state.licensed": "Lizenziert",
  "license.state.uncapped": "Lizenziert, ohne Sitzplatzgrenze",
  "license.state.unlicensed": "Keine Lizenz konfiguriert",
  "license.state.refused": "Lizenz abgelehnt",
  "license.absent.title": "Diese Installation hat keine Lizenz",
  "license.absent.body":
    "Alles funktioniert weiter, nichts ist begrenzt. Konfiguriere ein Lizenz-Token im Deployment, wenn Sitzplätze gegen eine Zusage gezählt werden sollen.",
  "license.refused.title": "Die Lizenz dieser Installation wurde abgelehnt",
  "license.refused.body":
    "Das Token im Deployment wurde vorgelegt und abgelehnt. Alles funktioniert weiter und unbegrenzt, bis es ersetzt wird — prüfe das Token und die Uhr der Installation.",
  "license.seats.title": "Plätze",
  "license.seats.used": "Belegte Sitzplätze",
  "license.seats.granted": "Gewährte Sitzplätze",
  "license.seats.uncapped": "Keine Grenze",
  "license.meter.label": "{used} von {granted} Sitzplätzen belegt",
  "license.over.title": "Sie überschreiten Ihre Sitzplatzgrenze",
  "license.over.body":
    "{used} Sitzplätze sind belegt, die Lizenz gewährt {granted}. Niemand verliert den Zugang und kein Sitzplatz wird entzogen — aber es kann kein neues Mitglied eingeladen werden, solange die Grenze überschritten ist. Deaktivieren Sie ein Mitglied oder erhöhen Sie die Grenze.",
  "license.holder.title": "Lizenziert für",
  "license.holder.org": "Organisation",
  "license.holder.contact": "Kontakt",
  "license.holder.installation": "Installation",
  "license.holder.validUntil": "Gültig bis",
  "license.holder.expiredOn": "Abgelaufen am",
  "license.holder.id": "Lizenz-ID",
  "license.grace.title": "Diese Lizenz ist abgelaufen",
  "license.grace.body":
    "Die Lizenz ist am {expiry} abgelaufen. Sie funktioniert noch, für einen begrenzten Zeitraum. Erneuern Sie die Lizenz, damit die Installation in Betrieb bleibt.",
  "license.renewal.title": "Diese Lizenz braucht eine Erneuerung",
  "license.renewal.body":
    "Die Lizenz läuft am {expiry} ab. Vor diesem Datum ändert sich nichts.",
  "license.counting":
    "Volle Sitzplätze, die weder deaktiviert noch gesperrt sind, Agenten eingeschlossen. Lesende Sitzplätze sind unbegrenzt und werden nie gezählt. Gegen diese Zahl wird ein neues Mitglied zugelassen.",
  "settings.group.you": "Persönlich",
  "settings.group.admin": "Admin-Einstellungen",
  "settings.rates.fxTitle": "Währungskurse",
  "settings.rates.fxIntro":
    "Wechselkurse, die Fremdwährungsbeträge in deine Basiswährung umrechnen. Neue Kurse gelten ab heute oder später; vergangene Kurse werden nie geändert.",
  "settings.rates.fxWithheld":
    "Nur ein Admin oder Ops sieht die Währungskurse. Auf ihnen beruht jede Umrechnung in dieser Installation, deshalb werden sie nicht breiter gezeigt.",
  "settings.rates.modelWithheld":
    "Nur ein Admin oder Ops sieht, was die Modelle kosten. Die Preise sind Betriebsinformationen und werden deshalb nicht breiter gezeigt.",
  "settings.rates.readOnly":
    "Nur-Lese-Ansicht — du hast keine Berechtigung, Kurse zu ändern.",
  "settings.rates.fxTableLabel": "Geltende Kurse",
  "settings.rates.fxAdd": "Kurs setzen",
  "settings.rates.fxEmpty": "Noch keine Währungskurse.",
  "settings.rates.fxModalTitle": "Währungskurs setzen",
  "settings.rates.rateToBase": "Kurs (zur Basiswährung)",
  "settings.rates.modelTitle": "KI-Modellkosten",
  "settings.rates.modelIntro":
    "Preise je Modell in USD pro 1 Mio. Token zur Schätzung der KI-Kosten. Nur zur Transparenz — Preise ändern das Modell-Routing nie.",
  "settings.rates.modelTableLabel": "Geltende Preise",
  "settings.rates.modelAdd": "Modellpreis hinzufügen",
  "settings.rates.modelEmpty": "Noch keine Modellpreise.",
  "settings.rates.modelModalTitle": "Modellpreis setzen",
  "settings.rates.setRate": "Speichern",
  "settings.rates.refresh": "Von Quellen aktualisieren",
  "settings.rates.refreshEnqueued":
    "Aktualisierung angefordert — etwaige Vorschläge erscheinen im Posteingang.",
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
    "Deine persönliche Schreibstimme. Sie prägt Entwürfe, die für dich gemacht werden, bleibt privat und lernt nur aus Quellen, die du hinzufügst.",
  "settings.voice.readOnly":
    "Nur-Lese-Ansicht — du hast keine Berechtigung, deine Voice DNA zu ändern.",
  "settings.voice.emptyBody":
    "Füge ein paar Texte hinzu, die du geschrieben hast, und baue daraus deine Voice DNA. Das dauert etwa eine Minute.",
  "settings.voice.status.collecting": "Sammelt",
  "settings.voice.status.ready": "Bereit",
  "settings.voice.status.stale": "Neuaufbau nötig",
  "settings.voice.bandThin": "dünn",
  "settings.voice.bandGood": "gut",
  "settings.voice.bandRich": "dicht",
  "settings.voice.bandSharp": "scharf",
  "settings.voice.version": "Version {n}",
  "settings.voice.derivedLabel": "Deine abgeleitete Stimme",
  "settings.voice.derivedEmpty":
    "Noch nicht gebaut — füg Proben hinzu und bau, um deine abgeleitete Stimme zu sehen.",
  "settings.voice.personalityLabel": "Deine Vorgaben",
  "settings.voice.personalityPlaceholder":
    "Notizen dazu, wie du klingen willst — genau so behalten, wie du sie schreibst; das Modell überschreibt das nie.",
  "settings.voice.savePreferences": "Vorgaben speichern",
  "settings.voice.corpusLabel": "Schreibproben",
  "settings.voice.corpusRowLabel": "Aktuell in deinem Korpus",
  "settings.voice.meter": "{count} von {target} W\u00f6rtern",
  "settings.voice.register.email": "E-Mail",
  "settings.voice.register.social": "Social",
  "settings.voice.register.long_form": "Langform",
  "settings.voice.register.spoken": "gesprochen",
  "settings.voice.register.general": "allgemein",
  "settings.voice.bandDrop":
    "Das Entfernen stuft deine Stimme von {from} auf {to} zur\u00fcck. Zum Best\u00e4tigen Entfernen erneut ausl\u00f6sen.",
  "voice.insights.avoidLabel": "Was deine Stimme vermeidet",
  "voice.insights.voiceScore": "Stimm-Treffer {pct}%",
  "voice.insights.next.addTranscript":
    "F\u00fcge ein Gespr\u00e4chs- oder Meeting-Transkript hinzu \u2014 gesprochene Worte sind dein st\u00e4rkstes Signal.",
  "voice.insights.next.addEmail":
    "F\u00fcge gesendete E-Mails hinzu \u2014 sie sind die wichtigste Quelle daf\u00fcr, wie du beruflich schreibst.",
  "voice.insights.next.addWords":
    "F\u00fcge etwa {count} weitere W\u00f6rter hinzu, um das scharfe Band zu erreichen.",
  "voice.insights.next.atTarget":
    "Dein Korpus ist am Ziel; halte ihn mit gelegentlichen neuen Texten frisch.",
  "voice.status.active": "aktiv",
  "voice.status.candidate": "wartet auf Pr\u00fcfung",
  "voice.status.superseded": "abgel\u00f6st",
  "voice.status.rejected": "abgelehnt",
  "voice.classification.routine": "routinem\u00e4\u00dfige \u00c4nderung",
  "voice.classification.material": "wesentliche \u00c4nderung",
  "voice.outcome.autoActivated": "automatisch aktiviert",
  "voice.outcome.reviewRequired": "Pr\u00fcfung erforderlich",
  "voice.outcome.manuallyActivated": "von dir aktiviert",
  "voice.outcome.rejected": "abgelehnt",
  "voice.outcome.rollback": "wiederhergestellt",
  "voice.history.versionRow": "v{n} \u00b7",
  "voice.history.loadMore": "\u00c4ltere Eintr\u00e4ge anzeigen",
  "voice.insights.provenance": "Aus deinem Korpus gebaut \u00b7 v{n}",
  "voice.insights.statWords": "W\u00f6rter: {count}",
  "voice.insights.statSources": "Quellen: {count}",
  "voice.insights.statSentence": "\u2248{count} W\u00f6rter pro Satz",
  "voice.insights.thinkingLabel": "Wie du denkst",
  "voice.insights.movesLabel":
    "Deine Signature Moves \u2014 in deinen eigenen Worten",
  "voice.insights.samplesLabel": "Beispielentw\u00fcrfe in deiner Stimme",
  "voice.insights.draftOnly": "nur Entwurf \u2014 wird nie gesendet",
  "voice.insights.disclosure":
    "KI-gest\u00fctzte Entw\u00fcrfe; jeder Versand bleibt eine menschliche Entscheidung.",
  "voice.insights.nextBestLabel": "So wird sie besser:",
  "voice.candidate.title":
    "Eine neue Voice-Version (v{n}) ist fertig — lies sie, bevor du sie verwendest.",
  "voice.candidate.whatItIs":
    "Das hat der Build aus deinen Proben gelernt. Sie ist noch nicht im Einsatz: Es wird nichts in dieser Stimme entworfen, bevor du sie auswählst.",
  "voice.candidate.reviewLabel":
    "Was diese Version über deinen Schreibstil sagt",
  "voice.candidate.concernsLabel":
    "Warum sie auf dich wartet, statt von allein aktiv zu werden",
  "voice.candidate.applyHint":
    "Wenn sie sich nach dir liest, verwende sie. Wenn nicht, behalte deine aktuelle Stimme und füge mehr eigene Texte hinzu — der nächste Build lernt aus dem, was in deinem Korpus steht.",
  "voice.candidate.reason.malformed":
    "Die Prüfung, die eine neue Stimme bewertet, konnte einige der Beispielentwürfe nicht lesen. Die Bewertung beruht daher auf weniger Beispielen als üblich.",
  "voice.candidate.reason.lowScore":
    "Beispielentwürfe in dieser Stimme erreichten {score} im Vergleich zu deinen eigenen Texten — unter den {floor}, die diese Installation verlangt, um eine Stimme selbstständig zu aktivieren.",
  "voice.candidate.reason.hardFailures":
    "{n} Formulierungen, die diese Stimme vermeiden soll, sind in den Beispielentwürfen geblieben.",
  "voice.candidate.reason.rulesRemoved":
    "{n} Regeln darüber, was zu vermeiden ist, sind gegenüber deiner vorherigen Version weggefallen.",
  "voice.candidate.apply": "Diese Version verwenden",
  "voice.candidate.reject": "Meine aktuelle Stimme behalten",
  "voice.history.label": "Versionen und Lernen",
  "voice.history.empty":
    "Noch keine Versionen \u2014 baue zuerst deine Stimme.",
  "voice.history.deltasLabel": "Was sich ge\u00e4ndert hat",
  "voice.history.deltasEmpty":
    "Noch nichts zu vergleichen \u2014 ab deinem zweiten Build steht hier eine \u00c4nderung.",
  "voice.history.deltaRow": "v{from} \u2192 v{to}",
  "voice.history.learning":
    "Lernt kontinuierlich \u2014 erstellte Entw\u00fcrfe: {drafted} \u00b7 vor dem Senden bearbeitet: {edited} \u00b7 abgelehnt: {rejected}.",
  "voice.history.rollback": "Version {n} wiederherstellen",
  "settings.voice.corpusEmpty": "Noch keine Proben.",
  "settings.voice.excluded": "ausgeschlossen",
  "settings.voice.removeSource": "Probe entfernen",
  "settings.voice.addSource": "Schreibproben hinzufügen",
  "settings.voice.addFirstLabel": "Deine erste Schreibprobe",
  "settings.voice.dropHint":
    "Dateien hier ablegen oder auswählen. .txt, .md, .vtt, .srt oder .json, gern mehrere auf einmal.",
  "settings.voice.dropEmpty":
    "Deine Texte hier ablegen oder klicken, um Dateien auszuwählen",
  "settings.voice.whyToggle": "Warum das wichtig ist",
  "settings.voice.whyBody":
    "Margince entwirft E-Mails für dich in deinen eigenen Worten, und nichts wird gesendet, bevor du es freigibst. Es lernt Ton, Rhythmus und Formulierungen aus deinen eigenen Texten, und von niemandem sonst. Deine Proben bleiben privat.",
  "settings.voice.worksTitle": "Was am besten funktioniert",
  "settings.voice.worksEmails":
    "Gesendete E-Mails, als .txt oder .md gespeichert. Sie zeigen, wie du schreibst, wenn du etwas willst.",
  "settings.voice.worksDocs":
    "Angebote, Posts und alles andere, was du selbst geschrieben hast.",
  "settings.voice.worksTranscripts":
    "Transkripte von Calls oder Meetings (.vtt, .srt, .json oder ein Textexport). Ich frage, welche Sprecherin oder welcher Sprecher du bist, und behalte nur deine eigenen Beiträge.",
  "settings.voice.worksNot":
    "Lass weg, was andere geschrieben haben, und Entwürfe, die eine KI für dich gemacht hat. Sie würden ihr die Stimme von jemand anderem beibringen.",
  "settings.voice.floorNote":
    "Mindestens {min} Wörter für einen ersten Build. Darunter kopiert das Modell nur Formulierungen.",
  "settings.voice.floorLabel":
    "Fortschritt bis zum ersten Build ({min} Wörter)",
  "settings.voice.floorProgress":
    "{words} von {min} Wörtern bis zum ersten Build",
  "settings.voice.speakerQuestion":
    "„{name}“ ist ein Gespräch. Wer davon bist du?",
  "settings.voice.speakerWhy":
    "Nur deine eigenen Beiträge werden behalten. Die Worte aller anderen werden verworfen.",
  "settings.voice.speakerDetail": "{words} Wörter, {turns} Beiträge",
  "settings.voice.speakerConfirm": "Das bin ich",
  "settings.voice.speakerDismiss": "Datei überspringen",
  "settings.voice.noticeKept":
    "{name}: {kept} von {total} Wörtern behalten. Nur deine Beiträge zählen.",
  "settings.voice.noticeAdded": "{name}: {words} Wörter hinzugefügt.",
  "settings.voice.noticeSkippedType":
    "{name} wurde übersprungen – lesbar sind nur Textdateien.",
  "settings.voice.noticeSkippedEmpty":
    "{name} wurde übersprungen – die Datei enthält keinen Text.",
  "settings.voice.noticeDismissed":
    "{name} wurde übersprungen – nichts darin ließ sich dir zuordnen.",
  "settings.voice.noticeAskQueueFull":
    "{name} wurde nicht hinzugefügt – beantworte zuerst die offenen Fragen oben und füge die Datei dann erneut hinzu.",
  "settings.voice.noticeFailed":
    "{name} konnte nicht hinzugefügt werden: {detail}",
  "settings.voice.noticeUnexpected": "{name} konnte nicht hinzugefügt werden.",
  "settings.voice.refusalUnattributed":
    "{name} ist ein Gespräch, und nichts darin ließ sich dir zuordnen – deshalb wurde nichts übernommen.",
  "settings.voice.refusalSpeaker":
    "Diese Person kommt in {name} nicht vor, es wurde nichts übernommen.",
  "settings.voice.refusalUnsupported":
    "{name} liegt in einem Format vor, das nicht gelesen werden kann.",
  "settings.voice.buildsTitle": "Builds",
  "settings.voice.buildRowLabel": "Aus deinen Proben bauen",
  "settings.voice.building": "Baue…",
  "settings.voice.buildRunning":
    "Deine Voice DNA wird gerade gebaut — das dauert etwa eine Minute. Du kannst die Seite verlassen, der Build läuft weiter.",
  "settings.voice.rebuild": "Voice DNA neu bauen",
  "settings.voice.buildFirst": "Meine Voice DNA bauen",
  "settings.voice.buildNeedsWords":
    "Noch etwa {n} Wörter, dann kann ich deine erste Voice DNA bauen. Darunter liegt zu wenig von deinem Schreiben vor, um ehrlich etwas daraus zu lernen.",
  "settings.voice.buildProvisional":
    "Genug, um daraus zu bauen. Etwa {n} Wörter mehr geben dem Build ein vollständigeres Bild davon, wie du schreibst.",
  "settings.voice.buildStatus.succeeded": "Voice DNA aktualisiert.",
  "settings.voice.buildStatus.failed":
    "Der Aufbau ist nicht fertig geworden — versuch es noch mal.",
  "settings.voice.buildStatus.deferred":
    "In der Warteschlange — sie wird gleich fertig und aktualisiert sich automatisch.",
  "settings.voice.buildStatus.pending":
    "Wird noch gebaut — das kann einen Moment dauern; es aktualisiert sich hier, sobald es fertig ist.",
  "extAccess.title": "Erweiterungen & Zugriff",
  "extAccess.sub":
    "Was jede zusammengesetzte Erweiterungseinheit in diese Installation eingebracht hat und welche Rolle sie nutzen darf. Nur für Admins.",
  "extAccess.adminOnly": "Der Erweiterungszugriff steht nur Admins offen.",
  "extAccess.readOnly":
    "Ihr Sitzplatz liest diese Seite. Eine Berechtigung zu ändern erfordert einen vollen Sitzplatz.",
  "extAccess.empty":
    "In diese Installation ist keine Erweiterungseinheit eingebunden.",
  "extAccess.version": "Version {version}",
  "extAccess.openUnit": "Seite {name} öffnen",
  "extAccess.noPage":
    "{name} ist in die API eingebunden, aber dieser Build der App hat keine Seite dafür — die App ist vermutlich älter als der Server.",
  "extAccess.brings.heading": "Was diese Einheit mitbringt",
  "extAccess.brings.objects": "Berechtigungsobjekte",
  "extAccess.brings.routes": "Routen",
  "extAccess.brings.jobs": "Hintergrundjobs",
  "extAccess.brings.none": "Keine",
  "extAccess.noObjects":
    "Diese Einheit registriert keine Berechtigungsobjekte — es gibt nichts zu vergeben.",
  "extAccess.roleColumn": "Rolle",
  "extAccess.action.read": "Lesen",
  "extAccess.action.create": "Anlegen",
  "extAccess.action.update": "Ändern",
  "extAccess.action.delete": "Löschen",
  "extAccess.matrixCaption": "Wer darf was mit {object}",
  "extAccess.cell": "{role} darf {object} {action}",
  "extAccess.versionSkew":
    "Jemand anderes hat diese Rolle geändert, während Sie sie ansahen — Ihre Änderung wurde nicht übernommen. Oben stehen jetzt die aktuellen Berechtigungen; nehmen Sie die Änderung erneut vor, wenn Sie sie weiterhin wollen.",
  "extAccess.systemRole": "Eingebaute Rolle",
  "extAccess.nobodyReads":
    "Keine Rolle darf {object} lesen — jedes Mitglied sieht dort eine leere Seite, wo diese Erweiterung stehen sollte. Vergeben Sie unten mindestens einer Rolle das Leserecht.",
  "users.empty": "Noch keine Benutzer.",
  "users.adminOnly": "Benutzer verwalten können nur Admins.",
  "users.inviteTitle": "Benutzer einladen",
  "users.teamsLabel": "Teams",
  "users.noTeamsYet": "Noch keine Teams.",
  "users.teamMembersLabel": "Wer in diesem Team ist",
  "users.teamMembersAdminOnly":
    "Die Mitgliedschaft ist nur für Admins sichtbar.",
  "users.teamNobodyToAdd": "Noch keine Benutzer zum Hinzufügen.",
  "users.teamsTitle": "Teams",
  "users.teamsSub":
    "Benannte Gruppen, mit denen Sie Datensätze teilen können. Die Mitgliedschaft allein gewährt den meisten Rollen weiterhin keinen Zugriff — Ausnahme ist die Teamleitung: wird sie einem Team hinzugefügt, kann sie dessen Datensätze lesen und bearbeiten, ohne dass eine Freigabe eingerichtet wird.",
  "users.teamsAdminOnly": "Teams verwalten können nur Admins.",
  "users.deactivated": "{name} deaktiviert",
  "users.reactivated": "{name} reaktiviert",
  "users.roleSaved": "Rolle für {name} geändert",
  "users.teamArchived": "Team „{name}“ archiviert",
  "users.teamRestored": "Team „{name}“ wiederhergestellt",
  "users.archiveTeam": "Team {name} archivieren",
  "users.newTeamLabel": "Neues Team",
  "users.newTeamOpen": "Neues Team",
  "users.teamNameLabel": "Teamname",
  "users.newTeamPlaceholder": "z. B. DACH Sales",
  "users.createTeam": "Team anlegen",
  "users.access.title": "Das sieht dieser Benutzer",
  "users.access.identity":
    "Liest alle Kontakte, Firmen, Leads und Deals der Organisation.",
  "users.access.writesAll": "Bearbeitet alle Datensätze.",
  "users.access.writesTeam":
    "Bearbeitet eigene Datensätze und die der Teams {teams}.",
  "users.access.writesTeamNone":
    "Bearbeitet nur eigene Datensätze — noch keinem Team zugeordnet.",
  "users.access.writesOwn": "Bearbeitet nur eigene Datensätze.",
  "users.access.none": "kein Zugriff",
  "users.access.read": "lesen",
  "users.access.write": "schreiben",
  "users.access.delete": "löschen",
  "users.access.object.person": "Kontakte",
  "users.access.object.organization": "Firmen",
  "users.access.object.lead": "Leads",
  "users.access.object.deal": "Deals",
  "users.access.object.project": "Projekte",
  "users.access.mask": "{field} ist ausgeblendet {when}.",
  "users.access.maskAlways": "immer",
  "users.access.maskOutside":
    "bei Datensätzen, die nicht bearbeitet werden dürfen",
  "users.inviteSub":
    "Jemanden zu dieser Installation hinzufügen und die Rolle wählen, mit der sie oder er startet.",
  "users.membersTitle": "Benutzer",
  "users.membersSub":
    "Alle mit einem Platz in dieser Installation, deaktivierte Konten eingeschlossen.",
  "users.memberCount_one": "{count} Benutzer",
  "users.memberCount_other": "{count} Benutzer",
  "users.teamMemberCount_one": "{count} Mitglied",
  "users.teamMemberCount_other": "{count} Mitglieder",
  "users.emailLabel": "E-Mail des neuen Benutzers",
  "users.nameLabel": "Vollständiger Name des neuen Benutzers",
  "users.emailPlaceholder": "name@firma.de",
  "users.namePlaceholder": "Vollständiger Name",
  "users.deactivateConfirmTitle": "{name} deaktivieren?",
  "users.deactivateConfirmBody":
    "Die Person wird überall abgemeldet und ihre Agent-Pässe werden sofort widerrufen. Du kannst sie später reaktivieren, aber sie muss sich dann neu anmelden.",
  "users.deactivateAgentConfirmBody":
    "Das ist die Agent-Identität dieser Organisation. Wird sie deaktiviert, laufen alle Jobs ohne Person dahinter nicht mehr — Erweiterungen eingeschlossen — bis du sie reaktivierst. Kein Mensch verliert Zugriff: Sie meldet sich nirgends an.",
  "users.agentSeat": "Agent",
  "users.agentSeatRole": "Handelt mit einem Pass, nicht mit einer Rolle",
  "users.roleLabel": "Rolle für den neuen Benutzer",
  "users.inviteOpen": "Benutzer einladen",
  "users.invite": "Einladen",
  "users.setRole": "Rolle setzen…",
  "users.setRoleFor": "Rolle für {name} setzen",
  "users.rowActions": "Aktionen für {name}",
  "users.rolesHeld": "Hat {roles}. Eine auszuwählen ersetzt alle",
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
    "Sende diesen Link über einen vertrauenswürdigen Kanal an das Mitglied. Er funktioniert einmal und wird nur jetzt angezeigt. Nach dem Schließen kannst du in der Mitgliederzeile einen neuen erstellen.",
  "users.link.urlLabel": "Passwort-Link",
  "users.link.copy": "Link kopieren",
  "users.link.copied": "Kopiert",
  "users.link.copyFailed":
    "Automatisches Kopieren nicht möglich. Markiere den Link und kopiere ihn.",
  "users.link.expires": "Gültig bis {when}.",
  "users.link.failed":
    "Das Mitglied wurde angelegt, der Link jedoch nicht. Ohne Link kann es sich nicht anmelden.",
  "users.link.offline":
    "Server nicht erreichbar. Prüfe deine Verbindung und versuche es erneut.",
  "users.link.retry": "Erneut versuchen",
  "users.link.done": "Fertig",
  "settings.companyReadOnly":
    "Nur-Lese-Ansicht — das Unternehmensprofil zu ändern braucht Schreibrechte auf die Organisation.",
  "settings.companyTitle": "Was Margince über dein Unternehmen weiß",
  "settings.companySub":
    "Halte den gemeinsamen Geschäftskontext für Entwürfe, Angebote, Suche und gesteuerte Agenten aktuell. Jede Aussage bleibt mit Quelle und Urheber verbunden.",
  "settings.companyTrust":
    "Nur bestätigtes Wissen — Website-Texte werden nie zu Anweisungen.",
  "settings.companyConfirmed": "bestätigte Aussagen",
  "settings.companyMark": "Firmenzeichen",
  "settings.companyMarkPresent":
    "Wird überall dort gezeigt, wo diese Firma auftaucht, auch oben in der Seitenleiste.",
  "settings.companyMarkNone":
    "Noch kein Zeichen, deshalb stehen die Initialen dafür. Ein Website-Auslesen füllt das, oder Sie laden hier eines hoch.",
  "settings.companyMarkAdd": "Zeichen hinzufügen",
  "settings.companyMarkReplace": "Ersetzen",
  "settings.companyMarkRemove": "Entfernen",
  "settings.companyMarkPick": "Firmenzeichen",
  "settings.companyMarkHint":
    "PNG, JPEG, GIF, WebP, ICO oder SVG. Beim Hochladen wird es quadratisch zugeschnitten und verkleinert; ein selbst gewähltes Zeichen bleibt, bis Sie es entfernen.",
  "settings.companyMarkEmpty": "Bild hierher ziehen oder Datei auswählen",
  "settings.companyWebsite": "Öffentliche Unternehmenswebsite",
  "settings.companyWebsiteHint":
    "Die öffentliche Website, von der jede Website-Lesung ausgeht.",
  "settings.companySourceTitle": "Woher wir es lesen",
  "settings.companyRefreshRow": "Website erneut lesen",
  "settings.companyRefreshHint":
    "Wir rufen deine öffentlichen Seiten ab und schlagen Änderungen vor. Nichts landet im Profil, bevor du es geprüft und übernommen hast.",
  "settings.companyEdit": "Bearbeiten",
  "settings.companyEditField": "{field} bearbeiten",
  "settings.companyWebsiteRequired":
    "Füge vor der Aktualisierung eine Unternehmenswebsite hinzu.",
  "settings.companyRefresh": "Von Website aktualisieren",
  "settings.companyEssentials": "Die drei Grundlagen",
  "settings.companyPositioning": "Positionierung, Käufer und Vertrieb",
  "settings.companyIdentity": "Identität und rechtliche Angaben",
  "settings.companySave": "Firmenkontext speichern",
  "settings.companySaved": "Gespeichert",
  "settings.companyRefreshUnreadable":
    "Wir haben den Stand dieses Website-Lesevorgangs verloren. Starte die Aktualisierung erneut.",
  "settings.companyRefreshStale":
    "Der Website-Vorschlag hat sich geändert. Prüfe den neuen Vergleich vor dem Übernehmen.",
  "settings.companyRefreshReview": "Website-Vergleich",
  "settings.companyRefreshReady": "Änderungen prüfen",
  "settings.companyRefreshReading": "Website wird gelesen und belegt…",
  "settings.companyCoverage": "Seitenabdeckung",
  "settings.companyResolveAll":
    "Wähle für jeden Konflikt mit menschlichen Angaben eine Entscheidung.",
  "settings.companyApplyRefresh": "Ausgewählte Änderungen übernehmen",
  "settings.companySelectChange": "Änderung „{field}“ auswählen",
  "settings.companyClass.new": "Neu",
  "settings.companyClass.machine_change": "Website geändert",
  "settings.companyClass.human_conflict": "Entscheidung nötig",
  "settings.companyClass.unchanged": "Unverändert",
  "settings.companyResolution.keep_current": "Aktuellen Wert behalten",
  "settings.companyResolution.accept_proposal": "Website übernehmen",
  "settings.companyResolution.useValueFor":
    "Wert, der für {field} bleiben soll",
  "settings.companyResolution.use_value": "Meinen bearbeiteten Wert nutzen",
  "settings.companyManualKicker": "Private, manuelle Einrichtung",
  "settings.companyManualTitle": "Gib Margince die Grundlagen",
  "settings.companyManualSub":
    "Das Lesen der Website ist in dieser Rollout-Stufe nicht aktiviert. Diese drei Antworten reichen für einen nützlichen Firmenkontext — ohne Modellaufruf und ohne externe Anfrage.",
  "settings.companyCreateWorkspace": "Firmenkontext erstellen",
  "product.title": "Produkte",
  "product.readOnly": "Nur-Lese-Ansicht — du darfst Produkte nicht ändern.",
  "product.settingsSub":
    "Rate-Card-Einträge, auf deren Grundlage Angebotspositionen einen Snapshot erstellen.",
  "product.new": "Neues Produkt",
  "product.edit": "Produkt bearbeiten",
  "product.archive": "Produkt archivieren",
  "product.archiveConfirm":
    "Dieses Produkt archivieren? Bestehende Angebotszeilen behalten ihren Snapshot.",
  "product.name": "Name",
  "product.sku": "SKU",
  "product.description": "Beschreibung",
  "product.unit": "Einheit",
  "product.unitPrice": "Stückpreis",
  "product.currency": "Währung",
  "product.taxRate": "Standard-Steuersatz %",
  "product.active": "Aktiv",
  "product.activeFilter": "Nur aktive",
  "product.activeFilterAll": "Alle",
  "product.inactive": "Inaktiv",
  "product.archived": "Archiviert",

  "template.title": "Angebotsvorlagen",
  "template.readOnly":
    "Nur-Lese-Ansicht — du darfst Angebotsvorlagen nicht ändern.",
  "template.settingsSub": "Markenkonforme DE/EN-PDF-Layouts für Angebote.",
  "template.new": "Neue Vorlage",
  "template.edit": "Vorlage bearbeiten",
  "template.archive": "Vorlage archivieren",
  "template.archiveConfirm":
    "Diese Vorlage archivieren? Angebote, die sie referenzieren, fallen auf die Standardvorlage der Sprache zurück.",
  "template.name": "Name",
  "template.locale": "Sprache",
  "template.isDefault": "Standard für Sprache",
  "template.header": "Kopftext",
  "template.footer": "Fußtext",
  "template.localeFilter": "Sprache",
  "template.localeFilterAll": "Alle Sprachen",
  "template.localeDE": "Deutsch (DE)",
  "template.localeEN": "Englisch (US)",

  "tools.title": "Agenten-Werkzeuge",
  "tools.sub":
    "Die geregelte Oberfläche, die ein Passport aufrufen kann — dieselbe Liste, die ein MCP-Client sieht.",
  "tools.egress": "ruft nach außen",
  "tools.scopeAll": "Alle Passports",
  "tools.inventory": "Alle {count} Werkzeuge",
  "tools.scopeLabel": "Auf einen Passport eingrenzen",
  "tools.scopedTo": "Erreichbar durch {label}",
  "tools.unreachable": "Bereich nicht gewährt",

  "aiusage.title": "KI-Nutzung & Budget",
  "aiusage.withheld":
    "Nur ein Betreiber sieht, was die KI-Laufzeit ausgegeben hat. Die Zahlen umfassen die ganze Installation und werden deshalb nicht breiter gezeigt.",
  "aiusage.sub":
    "Ihre eigene Rechnung sichtbar — nach Aufgabe und Stufe, in Tokens.",
  "aiusage.budget": "{spent} von {budget} Tokens · {pct}%",
  "aiusage.budgetMeter": "Verbrauchtes Monats-Tokenbudget",
  "aiusage.band.normal": "normal",
  "aiusage.band.degraded": "Sparmodus",
  "aiusage.band.queued": "Budget erreicht — Hintergrund-KI wartet",
  "aiusage.band.unknown": "Unbekannter Budgetstatus",
  "aiusage.col.task": "Aufgabe",
  "aiusage.col.tier": "Stufe",
  "aiusage.col.calls": "Aufrufe",
  "aiusage.col.cached": "Aus Cache",
  "aiusage.col.tokensIn": "Tokens ein",
  "aiusage.col.tokensOut": "Tokens aus",
  "aiusage.col.cost": "Geschätzte Kosten",
  "aiusage.costNote": "Kosten sind Schätzungen zu den konfigurierten Tarifen.",
  "aiusage.monthLabel": "Monat",
  "aiusage.spendLabel": "Verbrauch nach Aufgabe",
  "aiusage.days.show": "Tage anzeigen",
  "aiusage.empty": "Keine KI-Aufrufe in diesem Zeitraum.",
  "aiusage.prevMonth": "Vorheriger Monat",
  "aiusage.nextMonth": "Nächster Monat",

  "aibanner.degraded": "KI läuft im Sparmodus.",
  "aibanner.queued": "KI-Budget erreicht — Hintergrund-KI wartet.",
  "aibanner.unknown": "Der KI-Budgetstatus ist unbekannt.",
  "aibanner.link": "Nutzung anzeigen",
  "aibanner.dismiss": "Schließen",

  "aicalls.title": "KI-Aufrufprotokoll",
  "aicalls.withheld":
    "Nur ein Betreiber liest die Aufrufspur. Sie verzeichnet jeden Modellaufruf dieser Installation und wird deshalb nicht breiter gezeigt.",
  "aicalls.sub":
    "Jeder Modellaufruf — Routing, Tokens, Wiederholungen und erfasste Nutzdaten.",
  "aicalls.col.detail": "Detail",
  "aicalls.expandCall": "Versuchsverlauf für {task} um {when} anzeigen",
  "aicalls.col.when": "Zeitpunkt",
  "aicalls.col.task": "Aufgabe",
  "aicalls.col.model": "Modell",
  "aicalls.col.tokens": "Tokens",
  "aicalls.col.latency": "Latenz",
  "aicalls.ms": "{value} ms",
  "aicalls.badge.cacheHit": "Cache-Treffer",
  "aicalls.badge.degraded": "reduziert",
  "aicalls.badge.retries": "Wiederholung ×{count}",
  "aicalls.callsLabel": "Letzte Aufrufe",
  "aicalls.filter.all": "Alle Aufgaben",
  "aicalls.loadMore": "Mehr laden",
  "aicalls.empty": "Noch keine KI-Aufrufe aufgezeichnet.",
  "aicalls.detail.identity":
    "{served} über {provider} bereitgestellt (konfiguriert: {configured})",
  "aicalls.detail.source": "Quelle der Modellidentität: {source}",
  "aicalls.detail.context": "Eingebetteter Kontext: {scopes}",
  "aicalls.detail.contextNone": "Kein Unternehmenskontext eingebettet",
  "aicalls.detail.attempts": "Versuche",
  "aicalls.detail.request": "Anfrage-Nutzdaten",
  "aicalls.detail.response": "Antwort-Nutzdaten",
  "aicalls.payload.off":
    "Nutzdatenerfassung ist aus — ai.capture_payloads: true in margince.yaml aktiviert die Aufzeichnung von Anfrage und Antwort.",
  "aicalls.payload.none": "Für diesen Aufruf wurden keine Nutzdaten erfasst.",

  "aiexport.button": "Als Zertifizierungsszenario exportieren",
  "aiexport.title": "Lauf als Zertifizierungsszenario exportieren",
  "aiexport.nameLabel": "Szenarioname",
  "aiexport.checklist":
    "Geheimnisse wurden bei der Erfassung entfernt. Personenbezogene Daten NICHT — prüfen und entfernen Sie PII und ersetzen Sie anschließend sanitized_by, bevor Sie die Datei in den Korpus übernehmen.",
  "aiexport.copy": "YAML kopieren",
  "aiexport.copied": "Kopiert",
  "aiexport.download": ".yaml herunterladen",
  "aiexport.copyFailed":
    "Kopieren fehlgeschlagen — Vorschau verwenden oder Datei herunterladen.",
  "aiexport.close": "Schließen",
  "aiexport.previewLabel": "Szenariovorschau",
  "aiexport.responseLabel": "Modellantwort",

  "countdown.daysHours": "{days}d {hours}h",
  "countdown.hoursMinutes": "{hours}h {minutes}m",
  "countdown.minutesSeconds": "{minutes}m {seconds}s",
  "countdown.expired": "Abgelaufen",

  "installationSettings.orgTitle": "Installation",
  "installationSettings.orgSub":
    "Wie diese Installation heißt und in welcher Zeitzone jede Auswertungsperiode berechnet wird.",
  "installationSettings.currencyTitle": "Währung",
  "installationSettings.currencySub":
    "Die eine Währung, in die jede Auswertung alle Beträge umrechnet.",
  "installationSettings.name": "Name der Organisation",
  "installationSettings.nameHint":
    "Wird überall dort angezeigt, wo das Produkt Ihre Organisation benennt.",
  "installationSettings.timezone": "Zeitzone für Auswertungen",
  "installationSettings.timezoneHint":
    "IANA-Zonenname (zum Beispiel Europe/Berlin). Die Uhr Ihrer Organisation: Periodengrenzen aller Auswertungen werden darin berechnet, und jedes Datum eines Datensatzes — Abschlusstermine, Rechnungstage, Verlaufsüberschriften — wird darin angezeigt, damit ein Datum für das ganze Team gleich lautet. Unabhängig von Ihrer eigenen Anzeigezeitzone.",
  "installationSettings.fiscalYearStart": "Geschäftsjahr beginnt",
  "installationSettings.fiscalYearStartHint":
    "Der Monat, in dem Ihr Geschäftsjahr beginnt. Auswertungen gruppieren nach diesem Jahr und Quartal — ein Jahr, das nicht im Januar beginnt, wird mit beiden Kalenderjahren benannt, die es umfasst, etwa FY2026/27. Eine Änderung benennt alle Auswertungen sofort neu, und eine gespeicherte Ansicht mit Periodenfilter fragt danach andere Monate ab.",
  "installationSettings.baseCurrency": "Basiswährung",
  "installationSettings.baseCurrencyHint":
    "ISO-4217-Code, in den alle Beträge für Auswertungen umgerechnet werden. Änderbar, bis der erste Betrag dagegen umgerechnet wurde.",
  "installationSettings.baseCurrencyLocked":
    "Gesperrt: Es wurden bereits Beträge gegen diese Währung umgerechnet — eine Änderung würde jede darauf aufbauende Auswertung neu bedeuten.",
  "installationSettings.baseLanguage": "Basissprache",
  "installationSettings.baseLanguageHint":
    "Die Sprache, in der die KI schreibt, wenn das ganze Team mitliest. Ihre eigene Anzeigesprache ist davon getrennt, und Antworten an Kunden folgen weiterhin der Sprache des Gesprächs.",
  "installationSettings.readOnly":
    "Nur ein Admin oder Ops kann diese Einstellungen ändern.",
  "installationSettings.edit": "Ändern",
  "installationSettings.editField": "{field} ändern",
  "installationSettings.save": "Speichern",
  "signInMethods.title": "Anmeldemethoden",
  "signInMethods.sub":
    "Wie sich Personen hier anmelden können. Die Liste zeigt, wofür diese Installation Zugangsdaten hat — eine Methode lässt sich abschalten, aber keine hinzufügen.",
  "signInMethods.password": "E-Mail und Passwort",
  "signInMethods.passwordAlways":
    "Immer verfügbar. Jedes Konto ist so erreichbar, und genau das macht das Abschalten der anderen unbedenklich.",
  "signInMethods.passwordReason":
    "Die Anmeldung mit Passwort lässt sich nicht abschalten. Sie hält eine Installation zugänglich.",
  "signInMethods.providerHint":
    "Diesen Anbieter auf der Anmeldeseite anbieten. Beim Abschalten brechen auch laufende Anmeldungen ab; bestehende Sitzungen bleiben unberührt.",
  "signInMethods.noneConfigured":
    "Für diese Installation ist kein externer Anbieter konfiguriert, daher steht außer dem Passwort nichts zur Auswahl.",
  "oauthApp.google.title": "Google-App",
  "oauthApp.google.sub":
    "Postfächer werden über eine eigene Google-OAuth-App verbunden, und die Anmeldung mit Google läuft ebenfalls darüber. Dabei werden die Zugangsdaten Ihrer Organisation verwendet und nicht unsere.",
  "oauthApp.google.absent":
    "Aus keiner Quelle ist eine App verfügbar. Gmail und Kalender lassen sich nicht verbinden, und die Anmeldung mit Google kann nicht angeboten werden.",
  "oauthApp.google.redirectSub":
    "Tragen Sie jede der folgenden URIs beim OAuth-Client in der Google Console ein. Fehlt eine, scheitert die Zustimmung mit redirect_uri_mismatch, ohne zu nennen, welche URI falsch war.",
  "oauthApp.google.clientIdPlaceholder":
    "000000000000-xxxx.apps.googleusercontent.com",
  "oauthApp.google.removeConfirmTitle": "Google-App entfernen?",
  "oauthApp.google.removeConfirmBody":
    "Das Client-Secret lässt sich nicht wieder auslesen. Nach dem Entfernen müssen beide Hälften erneut aus der Google-Konsole eingetragen werden. Gmail- und Kalender-Verbindungen laufen über diese App. Microsoft- und IMAP-Postfächer sind nicht betroffen. Die Ersteinrichtung fragt wieder danach.",
  "oauthApp.microsoft.title": "Microsoft-App",
  "oauthApp.microsoft.sub":
    "Outlook-Postfächer und -Kalender werden über eine eigene Entra-App-Registrierung verbunden, und die Anmeldung mit Microsoft läuft ebenfalls darüber. Dabei werden die Zugangsdaten Ihrer Organisation verwendet und nicht unsere.",
  "oauthApp.microsoft.absent":
    "Aus keiner Quelle ist eine App verfügbar. Outlook-Mail und -Kalender lassen sich nicht verbinden, und die Anmeldung mit Microsoft kann nicht angeboten werden.",
  "oauthApp.microsoft.redirectSub":
    "Tragen Sie jede der folgenden URIs in der Entra-App-Registrierung unter Authentifizierung als Web-Plattform ein. Fehlt eine, scheitert die Zustimmung mit AADSTS50011, ohne zu nennen, welche URI falsch war.",
  "oauthApp.microsoft.clientIdPlaceholder":
    "00000000-0000-0000-0000-000000000000",
  "oauthApp.microsoft.removeConfirmTitle": "Microsoft-App entfernen?",
  "oauthApp.microsoft.removeConfirmBody":
    "Das Client-Secret lässt sich nicht wieder auslesen. Nach dem Entfernen müssen beide Hälften erneut aus dem Entra-Portal eingetragen werden. Outlook-Mail- und -Kalender-Verbindungen laufen über diese App. Google- und IMAP-Postfächer sind nicht betroffen. Die Ersteinrichtung fragt wieder danach.",
  "oauthApp.configured": "In Verwendung: {clientId}",
  "oauthApp.fromEnvironment":
    "Aus der Konfiguration dieser Installation in Verwendung: {clientId}. Eine hier gespeicherte App ersetzt sie, solange eine gespeichert ist.",
  "oauthApp.pinnedToDirectory": "An das Verzeichnis {tenant} gebunden.",
  "oauthApp.replaceHint":
    "Ein neues Paar ersetzt das hinterlegte. Bereits bestehende Verbindungen laufen weiter, bis sie neu verbunden werden.",
  "oauthApp.store": "App hinterlegen",
  "oauthApp.replace": "App ersetzen",
  "oauthApp.remove": "App entfernen",
  "oauthApp.redirectCopied": "Kopiert",
  "oauthApp.redirectCopy": "URI für {purpose} kopieren",
  "oauthApp.redirect.mailbox_connect": "Postfach",
  "oauthApp.redirect.calendar_connect": "Kalender",
  "oauthApp.redirect.sign_in": "Anmeldung",
  "oauthApp.redirectTitle": "Autorisierte Weiterleitungs-URIs",
  "oauthApp.clientId": "Client-ID",
  "oauthApp.clientSecret": "Client-Secret",
  "oauthApp.tenant": "Verzeichnis-ID (Mandant)",
  "oauthApp.tenantHint":
    "Optional. Bindet die App an ein einzelnes Entra-Verzeichnis, sodass nur dessen Mitglieder zustimmen können. Leer lassen, um jede Organisation zuzulassen.",
  "oauthApp.tenantPlaceholder": "00000000-0000-0000-0000-000000000000",
  "firstRun.continue": "Weiter",
  "firstRun.ai.title": "Modellanbieter wählen",
  "firstRun.ai.sub":
    "Margince stellt keine eigene Inferenz bereit und arbeitet über Ihr Anbieterkonto. Alles davon lässt sich später unter Einstellungen → KI ändern.",
  "firstRun.ai.provider": "Anbieter",
  "firstRun.ai.key": "API-Schlüssel",
  "firstRun.ai.keyHint":
    "Einmal gesendet und im Schlüsseltresor versiegelt. Ist stattdessen {envVar} in der Umgebung gesetzt, liest der Server ihn von dort.",
  "firstRun.ai.chatModel": "Modell",
  "firstRun.ai.modelHint":
    "Ein Ausgangspunkt. Die angezeigten Preise gelten je Million Token, Eingabe → Ausgabe; jede Modell-ID, die Ihr Anbieter bedient, ist möglich.",
  "firstRun.ai.embedModel": "Embedding-Modell",
  "aiSettings.sub":
    "Wohin der Text dieser Installation geht und was er kostet.",
  "aiSettings.tabs": "Welcher Teil der KI-Einstellungen offen ist",
  "aiSettings.tab.routing": "Routing",
  "aiSettings.tab.providers": "Anbieter",
  "aiSettings.tab.automations": "Automatisierungen",
  "aiSettings.tab.usage": "Verbrauch",
  "aiSettings.tab.logs": "Protokoll",
  "aiSettings.withheld": "Nicht für Sie einsehbar",
  "aiSettings.unread": "Konnte nicht gelesen werden",
  "aiSettings.pending": "Wird gelesen…",
  "aiSettings.spend.label": "Verbrauch diesen Monat",
  "aiSettings.spend.value": "{spent} von {budget} Token",
  "aiSettings.spend.estimated": "≈ {amount} geschätzt",
  "aiSettings.providers.label": "Anbieter",
  "aiSettings.providers.value": "{count} mit Schlüssel",
  "aiSettings.providers.missing": "{count} gebunden, ohne Schlüssel",
  "aiSettings.providers.lastCall": "letzter Aufruf {elapsed}",
  "aiSettings.discardTitle": "Änderungen am Routing verwerfen?",
  "aiSettings.discardBody":
    "Die geänderten Bindungen sind nicht gespeichert. Wer diesen Tab verlässt, verwirft sie.",
  "aiSettings.discard": "Verwerfen",
  "elapsed.justNow": "gerade eben",
  "elapsed.minutes": "vor {minutes} Min.",
  "elapsed.hours": "vor {hours} Std.",
  "elapsed.days": "vor {days} T.",
  "aiRouting.lane.local_small": "Massen-Klassifikation auf eigener Hardware",
  "aiRouting.lane.cheap_cloud":
    "Alltag — Anreicherung, Zusammenfassungen, Triage",
  "aiRouting.lane.premium": "Alles, was ein Kunde liest",
  "aiRouting.lane.frontier": "Das schwierigste Denken, sparsam eingesetzt",
  "aiRouting.lane.local_large":
    "Schwerere Arbeit, die die eigenen Hosts nicht verlässt",
  "aiRouting.lane.embeddings": "Suche und Retrieval über die eigenen Daten",
  "aiRouting.lanes.title": "Routing-Bahnen",
  "aiRouting.lanes.sub":
    "Günstigste zuerst. Eine Aufgabe wählt die Bahn, die Bahn wählt das Modell.",
  "aiRouting.priceSheet": "Preisliste",
  "aiRouting.provider.label": "Anbieter",
  "aiRouting.change": "Ändern",
  "aiRouting.done": "Fertig",
  "aiRouting.noKey": "kein Schlüssel",
  "aiRouting.unpriced": "kein Preis",
  "aiRouting.effect":
    "Gespeicherte Bindungen erreichen jeden Prozess innerhalb einer Minute, ohne Neustart.",
  "aiProviderKeys.title": "Anbieter-Schlüssel",
  "aiProviderKeys.sub":
    "Die Zugangsdaten, mit denen diese Installation die Modellanbieter aufruft. Ein Schlüssel wird im Schlüsseltresor versiegelt und nie wieder angezeigt — ersetze ihn, wenn du ihn ändern willst.",
  "aiProviderKeys.keyless": "kein Schlüssel nötig",
  "aiProviderKeys.field": "API-Schlüssel",
  "aiProviderKeys.save": "Schlüssel speichern",
  "aiProviderKeys.adminOnly":
    "Nur Admin oder Ops können Anbieter-Zugangsdaten ändern.",
  "aiProviderKeys.configured": "eingerichtet",
  "aiProviderKeys.absent": "nicht gesetzt",
  "aiProviderKeys.configuredHint":
    "Im Schlüsseltresor versiegelt. Er kann nicht ausgelesen werden — füge einen neuen ein, um ihn zu ersetzen. Er kann auch über {envVar} ankommen.",
  "aiProviderKeys.absentHint":
    "Für diesen Anbieter liegen keine Zugangsdaten vor, ein daran gebundenes Modell kann also nicht aufgerufen werden. Sie können auch über {envVar} ankommen.",
  "aiProviderKeys.addPlaceholder": "API-Schlüssel einfügen",
  "aiProviderKeys.replacePlaceholder": "Neuen Schlüssel zum Ersetzen einfügen",
  "aiProviderKeys.add": "Hinzufügen",
  "aiProviderKeys.replace": "Ersetzen",
  "aiProviderKeys.removeConfirmTitle": "Den {provider}-Schlüssel entfernen?",
  "aiProviderKeys.removeConfirmBody":
    "Die Zugangsdaten werden aus dem Schlüsseltresor gelöscht und lassen sich nicht wiederherstellen — sie sind nie auslesbar, es gibt also keine Kopie. Jede an diesen Anbieter gebundene KI-Strecke steht, bis ein neuer Schlüssel eingefügt wird.",
  "aiProviderKeys.withheld":
    "Nur wer die Modellbindung ändern darf, sieht, für welche Anbieter ein Schlüssel vorliegt.",
  "aiProviderKeys.remove": "Entfernen",
  "aiRouting.withheld":
    "Nur wer die Modellbindung ändern darf, sieht, welche Modelle diese Installation verwendet.",
  "aiRouting.title": "Modell-Routing",
  "aiRouting.sheetAsOf":
    "Die Modelllisten sind die Preisliste mit Stand {date}. Jede neuere ID, die Ihr Anbieter bedient, funktioniert ebenfalls — einfach eintippen.",
  "aiRouting.sheetUnknown":
    "Die Modelllisten stammen aus der Preisliste, die Sie nicht einsehen dürfen. Jede ID, die Ihr Anbieter bedient, funktioniert — einfach eintippen.",
  "aiRouting.sub":
    "Welches Modell welche Stufe bedient. Änderungen wirken ohne Neustart; jeder Prozess übernimmt sie innerhalb einer Minute.",
  "aiRouting.unbound":
    "Diese Installation hat keine Modelle gebunden, daher sind ihre KI-Funktionen aus. Die erste Bindung deklariert eine Bereitstellung unter seeds.ai_routing in margince.yaml.",
  "aiRouting.profile.card": "Bereitstellungsprofil",
  "aiRouting.profile.label": "Standort",
  "aiRouting.profile.help":
    "Wo die Inferenz läuft. Souverän bedeutet kein Datenabfluss: nur Modelle auf eigenen Hosts — abgelehnt beim Speichern, nicht erst beim ersten Aufruf.",
  "aiRouting.profile.eu_hosted": "In der EU gehostet",
  "aiRouting.profile.sovereign": "Souverän (kein Datenabfluss)",
  "aiRouting.profile.cloud_frontier": "Cloud-Frontier",
  "aiRouting.dimensions.label": "Vektorbreite",
  "aiRouting.dimensions.help":
    "Leer lassen für den Standard des Anbieters. Ein Wert außerhalb von 1 bis 2000 wird abgelehnt.",
  "aiRouting.baseUrl.placeholder": "https://openrouter.ai/api",
  "aiRouting.baseUrl.label": "Host",
  "aiRouting.baseUrl.help":
    "Die Host-Wurzel des Anbieters, ohne Versionssegment. Der Adapter hängt /v1 an. Für openai_compatible erforderlich, das keinen eigenen Standard hat.",
  "aiRouting.models.noKey":
    "Nur die Preisliste — dieser Anbieter hat keinen Schlüssel und kann nicht gefragt werden, was er bedient. Jede ID, die er bedient, funktioniert trotzdem: eintippen.",
  "aiRouting.models.noEndpoint":
    "Nur die Preisliste — tragen Sie oben den Host ein, dann kann dieser Anbieter gefragt werden. Jede ID, die er bedient, funktioniert trotzdem: eintippen.",
  "aiRouting.models.profileForbids":
    "Nur die Preisliste — dieses Bereitstellungsprofil erlaubt es nicht, diesen Anbieter zu erreichen.",
  "aiRouting.models.notPublished":
    "Nur die Preisliste — dieser Anbieter veröffentlicht keine Modellliste.",
  "aiRouting.models.unreachable":
    "Nur die Preisliste — dieser Anbieter hat nicht geantwortet. Jede ID, die er bedient, funktioniert trotzdem: eintippen.",
  "aiRouting.model.label": "Modell",
  "aiRouting.model.help":
    "Aufgeführt sind die Modelle, für die diese Installation Preise kennt — je Million Token, Eingabe → Ausgabe. Jede andere ID, die Ihr Anbieter bedient, funktioniert ebenfalls — einfach eintippen.",
  "aiRouting.save": "Routing speichern",
  "aiRouting.saving": "Bindung wird gespeichert…",
  "aiRouting.saved": "Routing gespeichert. Jeder Prozess bedient es jetzt.",
  "aiRouting.adminOnly": "Nur Admin oder Ops können das Modell-Routing ändern.",
  "autonomy.title": "Was sich von selbst erledigt",
  "autonomy.sub":
    "Kleine Korrekturen, die du bisher von Hand bestätigt hast. Schalte eine ein, und sie wird sofort übernommen – die Änderung und ein Rückgängig warten auf deinem Tag.",
  "autonomy.noneDecidedYet":
    "Darüber hast du noch nichts entschieden. Was in dieser Liste landet, hängt von den Datensätzen ab, die dir gehören, und von der Arbeit, die dein Team an dich weiterleitet. Ohne beides bleibt sie leer. Die Schalter entscheiden trotzdem, was passiert, sobald etwas auftaucht.",
  "autonomy.noRecord": "Darüber hast du noch nicht entschieden.",
  "autonomy.record":
    "Bisher: {clean} wie vorgeschlagen übernommen, {edited} nach einer Änderung, {rejected} abgelehnt.",
  "autonomy.kind.close_date_correction.label": "Abschlussdaten",
  "autonomy.kind.close_date_correction.help":
    "Das Abschlussdatum eines Deals verschiebt sich durch das, was in einem Gespräch gesagt oder in einer Mail geschrieben wurde.",
  "autonomy.kind.org_name_promotion.label": "Firmennamen",
  "autonomy.kind.org_name_promotion.help":
    "Ein unter seiner Domain erfasstes Unternehmen übernimmt den Namen, den seine eigene Website nennt.",
  "autonomy.kind.lifecycle_change.label": "Lebenszyklus-Phasen",
  "autonomy.kind.lifecycle_change.help":
    "Ein Unternehmen wechselt die Phase aufgrund dessen, was mit ihm geschehen ist. Das kann auch ändern, wer das Konto sieht und welche Automationen laufen.",
  "captureSettings.title": "Anreicherung",
  "captureSettings.sub":
    "Wie erfasste Unternehmen und Kontakte nach ihrer Erstellung angereichert werden.",
  "captureSettings.autoEnrich.label":
    "Erfasste Unternehmen automatisch anreichern",
  "captureSettings.autoEnrich.help":
    "Wenn aktiviert, erhält jedes aus erfassten E-Mails erstellte Unternehmen automatisch ein Web-Dossier — seine Website wird gelesen und sein Profil ausgefüllt. Läuft unter einem Tageslimit.",
  "captureSettings.signatureEnrich.label": "Kontaktdaten aus E-Mails auswerten",
  "captureSettings.signatureEnrich.help":
    "Wenn aktiv, übernimmt Margince, was ein Kontakt in E-Mails an Sie unter seinem eigenen Namen angibt — in der Signatur und auf einer angehängten Visitenkarte. Position, Telefonnummer, Adresse, Firma. Das geschieht innerhalb von Minuten nach Eingang der E-Mail. Nichts wird erschlossen: Was die E-Mail nicht nennt, wird nicht geschrieben. Das ist die Voreinstellung der Organisation; ein Postfach mit eigener Einstellung behält sie.",
  "captureSettings.adminOnly":
    "Nur ein Administrator oder Ops kann dies ändern.",

  "ownDomains.companyTitle": "Unternehmens-Domains",
  "captureExclusions.title": "Nicht erfassen",
  "captureExclusions.sub":
    "Adressen und Domains, deren Nachrichten gar nicht erst ins CRM gelangen. Eigene Regeln gelten nur für die Postfächer, die Sie selbst verbunden haben; Regeln der Organisation gelten für alle.",
  "captureExclusions.notRetroactive":
    "Wirkt ab der nächsten Nachricht. Bereits erfasste Nachrichten bleiben.",
  "captureExclusions.current": "Geltende Regeln",
  "captureExclusions.empty": "Keine Ausschlüsse.",
  "ownerIdentities.title": "Ihre weiteren Adressen",
  "ownerIdentities.sub":
    "Adressen, die auch Sie sind: ein Alias zum Senden, eine private Domain, die Sie lesen, eine Adresse, von der Sie weiterleiten. Post zwischen Ihren eigenen Adressen ist keine Korrespondenz mit jemandem — sie wird nicht erfasst und wird nie ein Kontakt.",
  "ownerIdentities.add": "Adresse hinzufügen",
  "ownerIdentities.addLabel": "Eine weitere Adresse als Ihre eigene angeben",
  "ownerIdentities.addDescription":
    "Nur Ihre. Kolleginnen und Kollegen sehen nie, was Sie hier eintragen.",
  "ownerIdentities.current": "Angegeben",
  "ownerIdentities.notRetroactive":
    "Gilt ab der nächsten Nachricht. Bereits erfasste Post bleibt, und ein aus einem Alias entstandener Kontakt bleibt, bis Sie ihn zusammenführen oder entfernen.",
  "ownerIdentities.empty": "Sie haben keine weiteren Adressen angegeben.",
  "ownerIdentities.remove": "Diese Adresse zurückziehen",
  "ownerIdentities.added": "Adresse hinzugefügt.",
  "ownerIdentities.confirm": "Hinzufügen",
  "ownerIdentities.kindLabel": "Was geben Sie an?",
  "ownerIdentities.kind.address": "Eine Adresse",
  "ownerIdentities.kind.domain": "Eine ganze Domain",
  "ownerIdentities.valueLabel": "Adresse oder Domain",
  "ownerIdentities.addressPlaceholder": "sie@beispiel.de",
  "ownerIdentities.domainPlaceholder": "beispiel.de",
  "captureExclusions.scope.user": "Nur ich",
  "captureExclusions.scope.workspace": "Ganze Organisation",
  "captureExclusions.kind.address": "Adresse",
  "captureExclusions.kind.domain": "Domain",
  "captureExclusions.scopeLabel": "Gilt für",
  "captureExclusions.kindLabel": "Art",
  "captureExclusions.addLabel": "Adresse oder Domain ausschließen",
  "captureExclusions.placeholder.address": "name@beispiel.de",
  "captureExclusions.placeholder.domain": "beispiel.de",
  "captureExclusions.add": "Ausschließen",
  "captureExclusions.addOpen": "Neuer Ausschluss",
  "captureExclusions.remove": "{value} wieder erfassen",
  "ownDomains.title": "Eigene E-Mail-Domains",
  "ownDomains.sub":
    "Die Domains, die zu diesem Unternehmen gehören. Schreiben sich Kolleg:innen untereinander, wird diese Nachricht nicht gespeichert. Auch nicht für dich.",
  "ownDomains.curatedTitle": "Hier verwaltet",
  "ownDomains.irreversible":
    "Eine Domain hier einzutragen wirkt ab der nächsten Nachricht. Wird sie später entfernt, wird ab diesem Zeitpunkt wieder erfasst. Was übersprungen wurde, solange sie eingetragen war, liefert kein Postfach ein zweites Mal. Bereits erfasste E-Mails bleiben.",
  "ownDomains.fromCompany": "Aus dem Unternehmensprofil. Dort zu ändern:",
  "ownDomains.openCompany": "Unternehmensprofil öffnen",
  "ownDomains.empty":
    "Keine weiteren Domains eingetragen. Trag eine ein, wenn dein Unternehmen unter mehr als einer Domain schreibt.",
  "ownDomains.confirmed": "bestätigt",
  "ownDomains.candidate":
    "aus einem verbundenen Postfach, noch nicht bestätigt",
  "ownDomains.add": "Hinzufügen",
  "ownDomains.addOpen": "Domain hinzufügen",
  "ownDomains.addLabel": "Eigene Domain hinzufügen",
  "ownDomains.placeholder": "beispiel.de",
  "ownDomains.remove": "{domain} entfernen",

  "webhooks.title": "Webhooks",
  "webhooks.readOnly":
    "Nur-Lese-Ansicht — nur ein Admin oder Ops kann Abonnements ändern.",
  "webhooks.sub":
    "Ausgehende Abonnements, die signierte HTTP-POSTs für ausgewählte Ereignisse empfangen.",
  "webhooks.new": "Neues Abonnement",
  "webhooks.notConfigured":
    "Ausgehende Webhooks sind auf dieser Installation nicht aktiviert — zuerst muss ein Signaturschlüssel konfiguriert werden.",
  "webhooks.state.active": "Aktiv",
  "webhooks.state.paused": "Pausiert",
  "webhooks.updated": "Aktualisiert {date}",
  "webhooks.field.targetUrl": "Ziel-URL",
  "webhooks.field.eventTypes": "Ereignistypen",
  "webhooks.field.state": "Status",
  "webhooks.edit": "Bearbeiten",
  "webhooks.saveDone": "Webhook gespeichert",
  "webhooks.archiveDone": "Webhook archiviert",
  "webhooks.archive": "Archivieren",
  "webhooks.archiveConfirm":
    "Das Archivieren stoppt jede Zustellung für dieses Abonnement. Dies kann nicht rückgängig gemacht werden.",
  "webhooks.rotate": "Schlüssel rotieren",
  "webhooks.rotateConfirm.title": "Signaturschlüssel rotieren?",
  "webhooks.rotateConfirm.body":
    "Mit dem Bestätigen wird der aktuelle Schlüssel sofort ungültig und der neue Schlüssel danach einmalig angezeigt. Kopiere ihn und aktualisiere deinen Empfänger, sobald die Rotation abgeschlossen ist.",
  "webhooks.secret.title": "Signaturschlüssel",
  "webhooks.secret.warning":
    "Dieser Schlüssel wird nur einmal angezeigt und kann danach nicht erneut abgerufen werden. Speichere ihn jetzt — Zustellungen werden damit signiert.",
  "webhooks.secret.copy": "Kopieren",
  "webhooks.secret.copied": "Kopiert",
  "webhooks.secret.copyFailed":
    "Automatisches Kopieren fehlgeschlagen — bitte den Schlüssel manuell auswählen und kopieren.",
  "webhooks.secret.done": "Fertig",
  "webhooks.secret.leaveWarning":
    "Beim Verlassen wird die einzige Kopie dieses Secrets vernichtet. Kopieren Sie es zuerst.",

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
  "webhooks.deliveries.column.resolved": "Abgeschlossen / nächster Versuch",
  "webhooks.deliveries.status.pending": "Ausstehend",
  "webhooks.deliveries.status.delivered": "Zugestellt",
  "webhooks.deliveries.status.retrying": "Wird wiederholt",
  "webhooks.deliveries.status.dead_lettered": "Dead-Letter",
  "webhooks.deliveries.status.visibility_revoked":
    "Gestoppt — nicht mehr sichtbar",
  "webhooks.deliveries.replay": "Erneut zustellen",
  "webhooks.deliveries.replayConfirm.title":
    "Diese Zustellung erneut versuchen?",
  "webhooks.deliveries.replayConfirm.body":
    "Versucht die Zustellung sofort erneut, signiert mit dem aktuellen Schlüssel und einem neuen Zeitstempel. Es wird nicht auf den nächsten geplanten Versuch gewartet.",
  "reindexbanner.needed": "Neuindizierung erforderlich",
  "reindexbanner.link": "In den Einstellungen prüfen",

  "embedreindex.title": "Suchindex",
  "embedreindex.sub":
    "Der Neuindizierungsstatus des Embedding-Speichers — nur admin/ops, auch zum Ansehen.",
  "embedreindex.withheld":
    "Nur ein Admin oder Ops sieht den Suchindex. Ihn neu aufzubauen kostet Tokens für die ganze Installation, deshalb wird sein Status nicht breiter gezeigt.",
  "embedreindex.statusLabel": "Indexstatus",
  "embedreindex.reindexLabel": "Geändertes neu indexieren",
  "embedreindex.reindexHelp":
    "Bettet nur die Datensätze neu ein, deren Text sich seit dem letzten Durchlauf geändert hat.",
  "embedreindex.rebuildLabel": "Gesamten Index neu aufbauen",
  "embedreindex.rebuildHelp":
    "Bettet jeden Datensatz von Neuem ein. Dafür gedacht, wenn ein Lauf feststeckt oder das Embedding-Modell gewechselt hat.",
  "embedreindex.statusIdle": "Aktuell",
  "embedreindex.statusNeeded": "Neuindizierung erforderlich",
  "embedreindex.statusReembedding": "Neuindizierung läuft…",
  "embedreindex.lastProgress": "Letzter Fortschritt vor {duration}",
  "embedreindex.entitiesPending": "{count} Einträge ausstehend",
  "embedreindex.workspacePending": "{count} ausstehend",
  "embedreindex.reviewCta": "Prüfen & neu indizieren",
  "embedreindex.rebuildCta": "Index neu aufbauen",
  "embedreindex.confirmTitle": "Neuindizierung starten",
  "embedreindex.rebuildTitle": "Suchindex neu aufbauen",
  "embedreindex.confirmCta": "Neuindizierung starten",
  "embedreindex.rebuildConfirmCta": "Jetzt neu aufbauen",
  "embedreindex.previewLoading": "Umfang wird geschätzt…",
  "embedreindex.estimateEntities": "Neu einzubettende Einträge:",
  "embedreindex.estimateTokens": "Geschätzte KI-Tokens:",
  "embedreindex.estimateCost": "Geschätzte Kosten:",
  "embedreindex.estimateQualityHeuristic":
    "Heuristische Schätzung — eine kalte Arbeitsmengen-Untergrenze, kein beobachteter Verbrauch.",
  "embedreindex.utilizationTitle": "Budgetauswirkung",
  "embedreindex.impact.normal": "normal",
  "embedreindex.impact.degraded": "würde in den Sparmodus wechseln",
  "embedreindex.impact.queued": "würde in die Warteschlange gestellt",

  "consent.title": "Zugriff autorisieren",
  "consent.asks":
    "{client} kann in Margince als du handeln, mit dem unten angehakten Zugriff.",
  "consent.redirectsTo": "Margince sendet die Autorisierung zurück an {host}.",
  "consent.redirectsToLoopback":
    "Das ist eine Adresse auf diesem Computer, und diese Verbindung kann nicht belegen, welches Programm dort lauscht.",
  "consent.scopeNote.read": "sieht, was du siehst",
  "consent.scopeNote.draft": "bereitet Nachrichten zu deiner Prüfung vor",
  "consent.scopeNote.write":
    "erstellt, bearbeitet und archiviert Datensätze in deinem Namen",
  "consent.scopeNote.send":
    "sendet Nachrichten in deinem Namen, ohne vorher zu fragen",
  "consent.scopeNote.enrich":
    "verbraucht Anreicherungs-Guthaben — jeder Kauf fragt dich weiterhin zuerst",
  "consent.ceiling":
    "Nie mehr als deine eigenen Berechtigungen. Du kannst die Verbindung jederzeit unter Einstellungen → Agenten trennen.",
  "consent.pickOne": "Wähle mindestens eine aus, oder verweigere den Zugriff.",
  "consent.offline":
    "Sie bleibt verbunden, ohne erneut zu fragen, und erneuert den Zugriff, bis du ihn widerrufst.",
  "consent.approve": "Autorisieren",
  "consent.deny": "Zugriff verweigern",
  "consent.reentering": "Verbinde erneut…",
  "consent.backToApp": "Zurück zu Margince",
  "consent.staleTitle": "Diese Anfrage ist abgelaufen",
  "consent.staleBody":
    "Die Verbindungsanfrage ist nicht mehr gültig. Geh zurück zur App, die du verbinden wolltest, und starte erneut — ein Neuladen dieser Seite hilft nicht.",
  "consent.invalidTitle":
    "Diese Verbindungsanfrage konnte nicht abgeschlossen werden",
  "consent.invalidBody":
    "Diese Installation autorisiert die Anfrage in dieser Form nicht — die App ist hier möglicherweise nicht mehr registriert. Geh zurück zur App, die du verbinden wolltest, und starte erneut.",
  "person.thin.title": "Was wir bisher wissen",
  "person.thin.known":
    "Wir haben {what} zu {name}, aber niemand hier hat bisher einen erfassten Austausch mit ihnen.",
  "person.thin.remediation.capture":
    "Verbinden Sie das Postfach, das mit ihnen schreibt - dann fuellt sich diese Seite von selbst, jedes Feld mit seiner Quelle.",
  "person.thin.remediation.employer":
    "Hinterlegen Sie den Arbeitgeber, dann liest Margince dessen Website nach ihrer Rolle.",
  "person.thin.logFirst": "Ersten Kontakt erfassen",
  "person.enriched.title": "Was Margince gelesen hat",
  "person.enriched.sub":
    "Jeder Wert mit dem Text, aus dem er gelesen wurde. Eine Korrektur bleibt bestehen.",
  "person.enriched.field.title": "Position",
  "person.enriched.field.phone": "Telefon",
  "person.enriched.field.role": "Rolle",
  "person.enriched.field.linkedin": "LinkedIn",
  "person.enriched.field.org_name": "Unternehmen",
  "person.enriched.field.address": "Adresse",
  "person.enriched.field.website": "Website",
  "person.enriched.readFrom": "Gelesen aus {source} am {when}",
  "person.enriched.undo": "Rückgängig",
  "person.enriched.replaced": "Ersetzt „{was}“ — der ältere Stand.",
  "person.enriched.correctedByYou": "Von Ihnen korrigiert",
  "person.enriched.confirmed": "Best\u00e4tigt",
  "person.enriched.correct": "Korrigieren",
  "person.enriched.confirm": "Das stimmt",
  "person.enriched.save": "Korrektur speichern",
  "person.enriched.cancel": "Abbrechen",
  "person.graph.loading": "Das Netzwerk um diesen Kontakt wird gelesen \u2026",
  "person.graph.routeDirect": "{name} steht bereits im Austausch mit ihnen.",
  "person.graph.routeVia":
    "{name} steht im Austausch mit {through} im selben Unternehmen.",
  "person.graph.noRoute":
    "Bisher steht hier niemand im Austausch mit ihnen oder mit jemandem in ihrem Unternehmen.",
  "person.graph.noDirect": "Hier hat niemand mit ihnen korrespondiert.",
  "person.graph.recordWorksWith": "Festhalten: arbeitet mit {name} zusammen",
  "person.graph.noEdge": "Keine erfasste Korrespondenz mit {name}.",
  "person.graph.withColleague": "mit {name}",
  "person.graph.withContact": "mit diesem Kontakt",
  "person.graph.counts":
    "{total} Interaktionen in 90 Tagen \u00b7 {inbound} eingehend, {outbound} ausgehend",
  "person.graph.untitledMessage": "Ohne Titel",
  "person.graph.countsOnly":
    "Nur Z\u00e4hlwerte \u2014 die Nachrichten selbst bleiben in der Chronik.",
  "person.intro.routesTitle": "Wege hinein",
  "person.graph.droppedNote": "{count} weitere werden nicht angezeigt.",
  "person.graph.withheldDirect":
    "Einige Kolleginnen und Kollegen werden nicht angezeigt.",
  "person.graph.withheldAccount":
    "Einige Kontakte dieses Unternehmens werden nicht angezeigt.",
  "person.intro.askFirstName": "{name} um eine Vorstellung bitten",
  "person.intro.leadEyebrow": "Empfohlener Weg",
  "person.intro.leadRouteBadge": "Starker Weg",
  "person.intro.heroDirect": "kennt die Person direkt",
  "person.intro.heroIndirect": "erreicht sie über {through}",
  "person.intro.factReciprocal": "Wechselseitig",
  "person.intro.factOneSided": "Einseitig",
  "person.intro.factDirect": "Direkte Beziehung",
  "person.intro.factIndirect": "Über eine Kollegin",
  "person.intro.factReceipts": "{count} einsehbare Belege",
  "person.intro.stripPath": "Bester Weg",
  "person.intro.stripDirect": "Direkte Beziehung",
  "person.intro.stripVia": "Über {through}",
  "person.intro.stripNoPath": "Bisher erreicht hier niemand diese Person",
  "person.intro.stripWhyNow": "Warum jetzt",
  "person.intro.stripWhyNowSub": "Die jüngste Veränderung in dieser Beziehung",
  "person.intro.stripNoMoment": "Nichts Neues",
  "person.intro.stripNoMomentSub":
    "Keine jüngste Veränderung in dieser Beziehung",
  "person.intro.stripHandoff": "Status der Anfrage",
  "person.intro.handoffNotStarted": "Nicht begonnen",
  "person.intro.handoffNotStartedSub":
    "Es wurde noch keine Vorstellung erbeten",
  "person.intro.handoffOwner": "{name} ist am Zug",
  "person.intro.ownerColleague": "Ihre Kollegin",
  "person.intro.ownerYou": "Sie",
  "person.intro.ownerNobody": "niemand",
  "person.intro.relayTitle": "Stand der Vorstellung",
  "person.intro.relaySubOpen": "Wie weit die Übergabe ist.",
  "person.intro.relaySubNone": "Es ist keine Anfrage offen.",
  "person.intro.stepRoute": "Weg wählen",
  "person.intro.stepRoutePick": "wen Sie fragen",
  "person.intro.stepRequest": "Anfrage",
  "person.intro.stepNotSent": "nicht gesendet",
  "person.intro.stepAwaitingAnswer": "wartet auf Ihre Kollegin",
  "person.intro.stepIntroduction": "Vorstellung",
  "person.intro.stepNameDrop": "Name genannt",
  "person.intro.stepWaiting": "wartet",
  "person.intro.stepRecorded": "erfasst",
  "person.intro.stepReply": "Antwort",
  "person.intro.stepObserved": "aus erfasster Aktivität erkannt",
  "person.intro.stepDone": "Erledigt",
  "person.intro.stepCurrent": "Jetzt",
  "person.intro.stepPending": "Später",
  "person.intro.laneOurs": "Unser Team",
  "person.intro.laneTheirs": "Ihr Unternehmen",
  "person.intro.laneTarget": "Zielperson",
  "person.intro.useThisRoute": "Diesen Weg nutzen",
  "person.intro.mapRegion": "Wer diesen Kontakt erreicht, und über wen",
  "person.intro.edgeDirect": "{name} korrespondiert direkt mit ihr",
  "person.intro.edgeAccount": "arbeitet mit {name}",
  "person.intro.routesSub":
    "Bester zuerst. Nimm den, der sich wirklich nutzen lässt — der zweite steht hier, weil der erste nicht immer verfügbar ist.",
  "person.intro.best": "Bester Weg",
  "person.intro.alternative": "Alternative",
  "person.intro.evidenceTwoWay_one":
    "{total} Austausch in beide Richtungen in 90 Tagen · {when}",
  "person.intro.evidenceTwoWay_other":
    "{total} Austausche in beide Richtungen in 90 Tagen · {when}",
  "person.intro.evidenceOneSided_one":
    "{total} Kontakt in 90 Tagen, einseitig · {when}",
  "person.intro.evidenceOneSided_other":
    "{total} Kontakte in 90 Tagen, einseitig · {when}",
  "person.intro.whenToday": "letzter Kontakt heute",
  "person.intro.whenYesterday": "letzter Kontakt gestern",
  "person.intro.whenDays": "letzter Kontakt vor {days} Tagen",
  "person.intro.whenNever": "kein Kontakt in letzter Zeit",
  "person.intro.askTitle": "Um eine Vorstellung bei {name} bitten",
  "person.intro.cancel": "Abbrechen",
  "person.intro.askAction": "Vorstellung erbitten",
  "person.intro.askFailed": "Die Anfrage konnte nicht gespeichert werden.",
  "person.intro.reasonLabel": "Warum Sie fragen",
  "person.intro.reasonHint":
    "Das liest Ihre Kollegin, nicht der Kontakt. Sagen Sie, warum sich die Vorstellung lohnt.",
  "person.intro.valueLabel": "Was der Kontakt davon hat",
  "person.intro.valueHint":
    "Der Grund, warum der Kontakt dieses Gespräch wollen würde.",
  "person.intro.noteLabel": "Notiz zum Weiterleiten",
  "person.intro.noteHint":
    "Nur dieser Teil erreicht den Kontakt. Schreiben Sie ihn so, dass er unverändert weitergegeben werden kann.",
  "person.intro.nameDropAsk": "Um Erlaubnis bitten, den Namen zu nennen",
  "person.intro.fallbackLegend": "Falls abgelehnt wird",
  "person.intro.fallbackNone": "Nichts weiter",
  "person.intro.fallbackNoneHelp":
    "Die Anfrage wird geschlossen und Sie entscheiden selbst, wie es weitergeht.",
  "person.intro.fallbackNameDrop": "Stattdessen um Namensnennung bitten",
  "person.intro.fallbackNameDropHelp":
    "Sie würden sich selbst melden und die Kollegin erwähnen.",
  "person.intro.fallbackNextRoute": "Den nächsten Weg versuchen",
  "person.intro.fallbackNextRouteHelp":
    "Weiter zur nächsten Kollegin auf der Liste.",
  "person.intro.decideTitle": "Eine Vorstellung bei {name}",
  "person.intro.decideLegend": "Ihre Antwort",
  "person.intro.decideAction": "Antwort speichern",
  "person.intro.decideFailed": "Die Antwort konnte nicht gespeichert werden.",
  "person.intro.decideReasonLabel": "Möchten Sie etwas ergänzen",
  "person.intro.decideReasonHint":
    "Ihre Kollegin sieht das genau so, wie Sie es schreiben.",
  "person.intro.noteByModel": "Von Margince verfasst",
  "person.intro.nameDropRequested":
    "Es wurde außerdem gefragt, ob Ihr Name genannt werden darf.",
  "person.intro.answerAccept": "Ich stelle Sie vor",
  "person.intro.answerAcceptHelp": "Sie übernehmen die Vorstellung selbst.",
  "person.intro.answerNameDrop": "Sie dürfen meinen Namen nennen",
  "person.intro.answerNameDropHelp":
    "Die Kollegin meldet sich selbst und erwähnt Sie. Das ist keine Vorstellung und wird auch nirgends als eine erfasst.",
  "person.intro.answerSuggest": "Jemand anderen fragen",
  "person.intro.answerSuggestHelp":
    "Nennen Sie die Person, die besser helfen kann.",
  "person.intro.answerDecline": "Diesmal nicht",
  "person.intro.answerDeclineHelp":
    "Die Anfrage wird geschlossen. Sagen Sie gern, warum.",
  "person.intro.asksTitle": "Vorstellungen",
  "person.intro.asksSub":
    "Die Anfragen, an denen Sie beteiligt sind, neueste zuerst.",
  "person.intro.answerAction": "Antworten",
  "person.intro.completeIntroducedAction": "Als vorgestellt markieren",
  "person.intro.completeNameDroppedAction": "Als Namen verwendet markieren",
  "person.intro.completeFailed":
    "Das Ergebnis konnte nicht gespeichert werden.",
  "person.intro.withdrawAction": "Zurückziehen",
  "person.intro.withdrawFailed":
    "Die Anfrage konnte nicht zurückgezogen werden.",
  "person.intro.stateRequested": "Wartet auf Ihre Kollegin",
  "person.intro.stateAccepted": "Sie werden vorgestellt",
  "person.intro.stateNameDropApproved": "Sie dürfen den Namen nennen",
  "person.intro.stateSuggestOther": "Es wurde jemand anderes vorgeschlagen",
  "person.intro.stateDeclined": "Abgelehnt",
  "person.intro.stateIntroduced": "Vorgestellt",
  "person.intro.stateNameDropped": "Name genannt",
  "person.intro.stateReplied": "Es kam eine Antwort",
  "person.intro.stateExpired": "Keine Antwort in der Frist",
  "person.intro.stateCancelled": "Zurückgezogen",
  "person.intro.alreadyRequested": "Bereits angefragt",
  "person.intro.declined": "Früher abgelehnt",
  "person.intro.unavailable": "Nicht verfügbar",
  "person.network.momentsTitle": "Was sich zuletzt bewegt hat",
  "person.network.momentsSub":
    "Bewegungen in dieser Beziehung, aus den Nachrichten selbst.",
  "person.network.noMoments":
    "In dieser Beziehung hat sich zuletzt nichts bewegt.",
  "person.change.repliedAfterGap": "Antwort nach {days} stillen Tagen.",
  "person.change.wentQuiet": "Seit {days} Tagen ist nichts passiert.",
  "person.change.warmed": "Die Beziehung ist von {from} auf {to} gestiegen.",
  "person.change.cooled": "Die Beziehung ist von {from} auf {to} gefallen.",
  "person.band.none": "kein Kontakt",
  "person.band.weak": "schwach",
  "person.band.moderate": "mittel",
  "person.band.strong": "stark",
  "person.pulse.title": "Beziehung",
  "person.pulse.warmestIs": "{name} hat hier die engste Beziehung.",
  "person.pulse.nobodyYet":
    "Niemand hier hat bisher einen erfassten Austausch mit ihnen.",
  "person.pulse.lastInbound": "Letzte Nachricht von ihnen",
  "person.pulse.lastOutbound": "Letzte Nachricht von uns",
  "person.pulse.neverInbound": "nie",
  "person.pulse.neverOutbound": "nie",
  "person.pulse.why": "Wie das berechnet wird",
  "person.pulse.arithmetic":
    "Wert {score}/100 = 100 x Aktualitaet {recency} x Haeufigkeit {frequency} x Gegenseitigkeit {reciprocity}. Beim Lesen berechnet, nie gespeichert.",
  "person.identity.title": "Identitaet",
  "person.identity.emailDead":
    "Unzustellbar — Mails an diese Adresse kommen nicht an",
  "person.identity.email": "E-Mail",
  "person.identity.phone": "Telefon",
  "person.identity.currentRole": "Aktuelle Rolle",
  "person.identity.buyingRole": "Rolle im Kaufprozess",
  "person.career.title": "Fruehere Rollen",
  "person.consent.title": "Ausgehend-Schutz",
  "person.consent.allowed": "Erlaubt: {purposes}",
  "person.consent.noneGranted":
    "Kein Zweck ist eingewilligt, ausgehende Nachrichten bleiben blockiert.",
  "person.consent.blocked": "Blockiert: {purposes}",
  "person.network.title": "Wer kennt sie hier",
  "person.network.twoWay": "{count} beidseitige Kontakte in 90 Tagen",
  "person.network.oneSided": "{count} Kontakte in 90 Tagen, einseitig",
  "person.network.replied": "antwortete {when}",

  "person.page.loading": "Wird geladen…",
  "person.page.notOpened": "Dieser Kontakt konnte nicht geöffnet werden.",
  "person.page.buyingRole": "Rolle im Kauf",
  "person.page.owner": "Zuständig",
  "person.page.ownerUnassigned": "Nicht zugewiesen",
  "person.page.linkedin": "LinkedIn",
  "person.page.openProfile": "Profil öffnen",
  "person.rail.detailsTitle": "Details",
  "person.rail.archivedReadOnly":
    "Dieser Kontakt ist archiviert. Stelle ihn wieder her, um hier etwas zu ändern.",
  "person.rail.employmentVersionUnresolved":
    "Die aktuelle Version dieser Zeile konnte nicht zum Speichern zurückgelesen werden. Lade neu und versuche es erneut.",
  "person.rail.employmentTitle": "Unternehmen",
  "person.rail.noEmployment": "Keine Beschäftigung erfasst.",
  "person.rail.addEmployment": "Unternehmen hinzufügen",
  "person.rail.employer": "Arbeitgeber",
  "person.rail.allOrgsConnected":
    "Jeder Treffer ist bereits mit dieser Person verknüpft.",
  "person.rail.isCurrentEmployer": "Das ist der aktuelle Arbeitgeber",
  "person.rail.markEnded": "Als beendet markieren",
  "person.rail.removeEmploymentTitle":
    "Diese Unternehmensverbindung entfernen?",
  "person.rail.removeEmploymentBody":
    "Die Verbindung zu {org} und die daran hängende Historie verschwinden, und das lässt sich nicht rückgängig machen. {org} selbst bleibt. Wenn die Person einfach gegangen ist, markiere die Beschäftigung stattdessen als beendet.",
  "person.timeline.empty": "Mit ihnen wurde noch nichts erfasst.",
  "person.deals.empty": "Sie sind auf keinem Deal erfasst.",
  "person.deals.untitled": "Deal ohne Titel",
  "person.deals.noStage": "Noch keine Phase",
  "person.meetings.next": "Nächster Termin",
  "person.meetings.past": "Bisherige Termine",
  "person.meetings.noneBooked": "Mit ihnen ist nichts gebucht.",
  "person.meetings.noneLogged": "Es ist kein Termin mit ihnen erfasst.",
  "person.meetings.untitled": "Termin ohne Betreff",
  "person.meetings.participants": "Im Raum",
  "person.documents.empty": "Zu diesem Kontakt liegt keine Datei.",
  "person.research.empty": "Zu ihnen wurde noch nichts recherchiert.",
  "person.research.fields": "Belege der Anreicherung",
  "person.research.fieldsEmpty":
    "Noch trägt kein angereichertes Feld einen Beleg.",
  "person.research.capturedBy": "Erfasst von",
  "person.action.email": "E-Mail",
  "person.action.write": "Schreiben",
  "person.action.messageOn": "Über {transport} schreiben",
  "person.action.noTransport":
    "Keine Adresse und keine Unterhaltung, auf die sich antworten ließe.",
  "person.action.consentRefused":
    "Derzeit erlaubt kein Zweck, ihnen zu schreiben.",
  "person.action.call": "Anrufen",
  "person.action.meetings": "Termine ansehen",
  "person.action.addTask": "Aufgabe",
  "person.action.research": "Recherche",

  "person.strip.lastInbound": "Zuletzt eingehend",
  "person.strip.lastOutbound": "Zuletzt ausgehend",
  "person.strip.reciprocity": "Gegenseitigkeit",
  "person.strip.inOut": "{inbound} ein · {outbound} aus",
  "person.strip.nextMeeting": "Nächster Termin",
  "person.strip.never": "Nie",
  "person.strip.today": "Heute",
  "person.strip.yesterday": "Gestern",
  "person.strip.days": "vor {count} Tagen",
  "person.strip.noOpenDeal": "Kein offener Deal",
  "person.strip.noMeeting": "Keiner",
  "person.consent.allowedWord": "Erlaubt",
  "person.consent.blockedWord": "Gesperrt",
  "person.consent.unknownWord": "Unbekannt",

  "person.moment.rule.meeting_prep": "Termin steht an",
  "person.moment.rule.re_engaged": "Sie haben sich gemeldet",
  "person.moment.rule.job_change": "Neue Stelle",
  "person.moment.rule.overdue_promise": "Zusage überfällig",
  "person.moment.rule.gone_quiet": "Still geworden",
  "person.moment.rule.open_promise": "Offenes Versprechen",
  "person.moment.rule.role_change": "Rolle geändert",
  "person.moment.rule.public_signal": "Öffentlich gesagt",
  "person.moment.rule.missing_next_step": "Nichts geplant",
  "person.moment.rule.thin_relationship": "Nur ein Draht",
  "person.moment.rule.nothing_needed": "Nichts zu tun",
  "person.moment.evidence.activity": "Aus einem Austausch",
  "person.moment.evidence.task": "Aus einer Aufgabe",
  "person.moment.evidence.relationship_change":
    "Aus einer Änderung am Datensatz",
  "person.today.source_one": "{count} Beleg",
  "person.today.source_other": "{count} Belege",
  "person.today.updated": "Aktualisiert {when}",
  "person.today.freshToday": "heute",
  "person.today.freshYesterday": "gestern",
  "person.today.freshDaysAgo": "vor {count} Tagen",

  "person.brief.title": "Beziehungs-Briefing",
  "person.brief.reading": "Die Beziehung wird gelesen…",
  "person.brief.empty":
    "Es wurde noch nichts erfasst, woraus dieses Briefing geschrieben werden könnte.",
  "person.brief.sourceActivity": "Gespräch",
  "person.brief.sourceDeal": "Deal-Notizen",

  "person.matters.title": "Was {name} wichtig ist",
  "person.matters.priorities": "Prioritäten",
  "person.matters.objections": "Einwände",
  "person.matters.successCriteria": "Erfolgskriterien",
  "person.matters.absent": "Noch nichts erfasst",

  "person.commercial.title": "Offener Deal & Rolle im Kauf",
  "person.commercial.withheld":
    "Sie haben keinen Zugriff auf die Deals dieser Person.",
  "person.commercial.noDeal": "Kein offener Deal.",
  "person.commercial.closes": "Abschluss {date}",
  "person.commercial.committee": "Entscheidergremium",
  "person.commercial.openDeal": "Deal öffnen",

  "person.loops.title": "Zusagen & offene Punkte",
  "person.loops.empty":
    "In den erfassten Gesprächen wurde nichts zugesagt und nichts gefragt.",
  "person.loops.ours": "Sie",
  "person.loops.question": "Offene Frage",
  "person.loops.overdue": "{count} Tage überfällig",
  "person.loops.overdueUnderDay": "seit weniger als einem Tag überfällig",
  "person.loops.due": "fällig {when}",
  "person.loops.dueToday": "heute",
  "person.loops.dueTomorrow": "morgen",
  "person.loops.dueInDays": "in {count} Tagen",
  "person.loops.waiting": "wartet",
  "person.loops.open": "offen",
  "person.loops.atLeast": "mindestens {count}",

  "person.memory.title": "Gesprächsgedächtnis",
  "person.memory.empty": "Auf diesem Kanal wurde noch nichts erfasst.",
  "person.memory.all": "Alle",
  "person.memory.email": "E-Mail",
  "person.memory.meetings": "Termine",
  "person.memory.calls": "Anrufe",
  "person.memory.notes": "Notizen",
  "person.memory.channelEmail": "E-Mail",
  "person.memory.channelMeeting": "Termin",
  "person.memory.channelCall": "Anruf",
  "person.memory.channelNote": "Notiz",
  "person.memory.channelMessage": "Nachricht",
  "person.memory.channelTask": "Aufgabe",
  "person.memory.replied": "Beantwortet",
  "person.memory.unanswered": "Unbeantwortet",

  "person.rail.reviewFirst": "Erst prüfen",
  "person.rail.blocked": "Gesperrt",
  "person.rail.ready": "Bereit",
  "person.rail.pulseTitle": "Beziehungspuls",
  "person.rail.explain": "Erklären",
  "person.rail.direction": "Richtung",
  "person.rail.twoWay": "Beidseitig",
  "person.rail.oneSided": "Einseitig",
  "person.rail.lastReply": "Letzte Antwort",
  "person.rail.coverage": "Abdeckung",
  "person.rail.colleagues_one": "{count} Kollegin oder Kollege",
  "person.rail.colleagues_other": "{count} Kolleginnen und Kollegen",
  "person.rail.trend": "Tendenz",
  "person.rail.noInbound": "Nichts eingehend",
  "person.rail.cooling": "Kühlt ab",
  "person.rail.warming": "Wird wärmer",
  "person.rail.overall": "Gesamt",
  "person.rail.thin": "Dünn",
  "person.rail.atRisk": "Gefährdet",
  "person.rail.strong": "Stark",
  "person.rail.whoKnows": "Wer kennt {name}",
  "person.rail.nobodyYet": "Bisher hatte hier niemand Kontakt mit ihnen.",
  "person.rail.exchanges": "{count} Kontakte",
  "person.rail.signals": "Signale & Risiken",
  "person.rail.noSignals": "An dieser Beziehung fällt nichts auf.",
  "person.rail.noReplyDays": "Seit {count} Tagen keine Antwort",
  "person.rail.repliedDaysAgo": "Antwort vor {count} Tagen",
  "person.rail.singleThreaded": "Nur ein Kontakt in diesem Deal",
  "person.rail.noMeetingBooked": "Kein nächster Termin vereinbart",
  "person.rail.consentTitle": "Einwilligung & Kanäle",
  "person.rail.email": "E-Mail",
  "person.rail.phone": "Telefon",
  "person.rail.noEmailAddress": "Keine Adresse hinterlegt",
  "person.rail.noPhoneNumber": "Keine Nummer hinterlegt",
  "person.rail.channelNotDeliverable": "Nicht zustellbar",
  "person.rail.recentActivity": "Letzte Aktivität",
  "person.rail.nothingCaptured": "Noch nichts erfasst.",
  "person.rail.viewAllActivity": "Alle Aktivitäten ansehen",
  "person.drawer.close": "Schließen",
  "person.composer.title": "Follow-up entwerfen · {name}",
  "person.composer.to": "An",
  "person.composer.transport": "Versandweg",
  "person.composer.transportEmail": "E-Mail",
  "person.composer.toConversation": "Setzt Ihre {transport}-Unterhaltung fort",
  "person.composer.subject": "Betreff",
  "person.composer.bcc": "Bcc",
  "person.composer.bccPlaceholder":
    "Eine Adresse pro Zeile – sie erhalten die Nachricht, kein anderer Empfänger sieht sie",
  "person.composer.body": "Nachricht",
  "richtext.bold": "Fett",
  "richtext.italic": "Kursiv",
  "richtext.bulletList": "Aufzählung",
  "richtext.numberList": "Nummerierte Liste",
  "richtext.link": "Link",
  "richtext.linkPrompt":
    "Webadresse für diesen Link (leer lassen zum Entfernen)",
  "person.composer.drafting": "Entwurf wird geschrieben…",
  "person.composer.why": "Warum dieser Entwurf",
  "person.composer.consentUnknown":
    "Für diesen Kanal ist keine Einwilligungsentscheidung erfasst.",
  "person.composer.sendNote":
    "Mit dem Senden geht diese Nachricht aus Ihrem eigenen Postfach raus.",
  "person.composer.purpose": "Einwilligungszweck",
  "person.composer.blockedLead":
    "Unter diesem Zweck kann diese Nachricht nicht rausgehen.",
  "person.composer.blockedRewrite":
    "Eine Nachricht unter einem anderen Zweck muss auch diese Art von Nachricht SEIN — sie umzuetikettieren macht sie nicht dazu.",
  "person.composer.blockedRecordConsent":
    "Wenn Sie eine Rechtsgrundlage haben, erfassen Sie die Einwilligungsentscheidung am Kontakt.",
  "person.composer.consentPickPurpose":
    "Wählen Sie, wofür diese Nachricht ist — die Einwilligung gilt je Zweck.",
  "person.composer.intent": "Worum soll es gehen?",
  "person.composer.intentHint":
    "Optional — z. B. um einen Termin in der ersten Septemberwoche bitten",
  "person.composer.draftWithAi": "Mit KI entwerfen",
  "person.composer.intentAgenda":
    "eine Agenda für den anstehenden Termin vorschlagen",
  "person.composer.intentReply": "auf die letzte Nachricht antworten",
  "person.composer.intentCommitment": "einlösen, was wir zugesagt haben",
  "person.composer.intentFollowUp": "nachfassen — es ist still geworden",
  "person.composer.send": "Senden",
  "person.composer.sending": "Wird gesendet…",
  "person.composer.sent": "Gesendet",
  "person.composer.aiDisclosure":
    "KI-unterstützter Entwurf · jedes Wort prüfen",
  "person.research.title": "Tiefenrecherche · {name}",
  "person.research.publicOnly": "Nur öffentliche Quellen",
  "person.research.running": "Öffentliche Quellen werden gelesen…",
  // "Recherche-Anbieter", nicht "Datenanbieter": Letzteres ist das Wort für die
  // zugekauften Kontaktdaten (provider.profile.*), die direkt darüber stehen.
  "person.research.notConnected":
    "Es ist kein Recherche-Anbieter verbunden, also wurden keine öffentlichen Quellen zu dieser Person gelesen. Das ist unabhängig von zugekauften Kontaktdaten darüber — Margince recherchiert nie aus eigener Befugnis zu einer Person, und eine Tiefenrecherche braucht einen lizenzierten Anbieter mit eigener Rechtsgrundlage.",
  "person.research.staged":
    "Die Recherche ist vorgemerkt. Am Datensatz von {name} ändert sich nichts, bis Sie prüfen und speichern.",
  "person.research.stats":
    "{sources} Quellen gelesen · {claims} belegte Aussagen",
  "person.research.dismiss": "Verwerfen",
  "person.research.discard": "Verwerfen",
  "person.research.save": "{count} Aussagen prüfen & speichern",
  "person.research.evidenceOrOmit":
    "KI-unterstützt · nur mit Beleg · ausschließlich öffentliche Informationen",
  "person.meeting.title": "Meeting-Briefing",
  "person.meeting.brief": "Briefing öffnen",
  "person.meeting.empty": "Zu diesem Meeting ist noch nichts erfasst.",
  "person.meeting.loading": "Briefing wird zusammengestellt…",
  "person.meeting.assembledNow":
    "Soeben aus den aktuellen Daten zusammengestellt",
  "person.meeting.header": "Auf einen Blick",
  "person.meeting.what_changed": "Seit dem letzten Kontakt",
  "person.meeting.goal": "Ziel dieses Meetings",
  "person.meeting.attendees": "Teilnehmende",
  "person.meeting.commitments": "Offene Zusagen",
  "person.meeting.deal_state": "Stand des Deals",
  "person.meeting.risks": "Risiken und Warnsignale",
  "person.meeting.talking_points": "Vorgeschlagene Gesprächspunkte",
  "person.meeting.company_context": "Letztes Treffen",
  "person.meeting.objective": "Das angestrebte Ergebnis",
  "person.meeting.openWith": "Einstieg",
  "person.meeting.arc": "Der Verlauf des Kontakts",
  "person.meeting.arcSub":
    "Nur die Momente, die das heutige Gespräch verändern.",
  "person.meeting.close": "Das Meeting abschließen",
  "person.meeting.advance.minimum": "Mindestens",
  "person.meeting.advance.best": "Bestenfalls",
  "person.meeting.advance.fallback": "Rückfalloption",
  "person.meeting.unknowns": "Was der Datensatz nicht hergibt",
  "person.meeting.likelyAsks": "Womit zu rechnen ist",
  "person.meeting.beReady": "Darauf vorbereitet sein",
  "person.meeting.say": "Sagen",
  "person.meeting.show": "Zeigen",
  "person.meeting.avoid": "Vermeiden",
  "person.meeting.scenarios": "Falls das Gespräch anders läuft",
  "person.meeting.relevance.high": "Wahrscheinlich",
  "person.meeting.relevance.medium": "Möglich",
  "person.meeting.relevance.low": "Weniger wahrscheinlich",
  "person.meeting.coach.title": "Einen Punkt coachen",
  "person.meeting.coach.eyebrow": "Führungsansicht",
  "person.meeting.coach.listenFor": "Achte auf",
  "person.meeting.coach.watchFor": "Warnsignal",
  "person.meeting.coach.interveneIf": "Nur eingreifen, wenn",
  "person.meeting.coach.paths": "Mögliche Verläufe",
  "person.meeting.background": "Hintergrund und Quellen",
  "person.meeting.omittedSource": "Nicht in diesem Briefing",
  "person.meeting.preparedFor": "Vorbereitet für {name}",
  "person.meeting.preparedForAt": "Vorbereitet für {name} · {org}",

  "co.strip.healthSummary": "Zustand",
  "co.strip.healthSummary.failingOf": "{failing} von {rated} gefährdet",
  "co.strip.healthSummary.because": "{dimension} — {reason}",
  "co.strip.healthSummary.of": "{rated} von 3 bewertet",
  "today.source.suggestions": "die Empfehlungen",

  // Der Datenanbieter (ADR-0101). Zwei Oberflächen teilen sich diese
  // Begriffe — die Einstellungskarte und die Personenseite —, damit ein
  // Zustand überall gleich heißt.
  "provider.readOnly":
    "Nur-Lese-Ansicht — einen Anbieter zu verbinden kostet Geld und ist eine Admin- oder Ops-Aktion.",
  "provider.title": "Kontaktdaten",
  "provider.sub":
    "Geprüfte Kontaktdaten zu den Personen in deinem CRM zukaufen. Bezahlt wird beim Anbieter mit Guthaben; was davon hier verbraucht wurde, steht unten.",
  "provider.notConfigured":
    "In dieser Installation ist kein Datenanbieter verfügbar. Es wird nichts zugekauft, und es kann auch nichts zugekauft werden.",
  "provider.status.connected": "Verbunden",
  "provider.status.disconnected": "Nicht verbunden",
  "provider.status.validating": "Schlüssel wird geprüft …",
  "provider.status.invalidCredentials": "Schlüssel abgelehnt",
  "provider.status.insufficientCredits": "Kein Guthaben mehr",
  "provider.status.rateLimited": "Zu viele Anfragen",
  "provider.status.providerError": "Beim Anbieter klemmt es gerade",
  "provider.connect": "Verbinden",
  "provider.reconnect": "Schlüssel ersetzen",
  "provider.apiKey": "API-Schlüssel",
  "provider.apiKeyStored": "API-Schlüssel ersetzen",
  "provider.apiKeyReplaceHint":
    "Ein Schlüssel ist hinterlegt und aktiv. Er lässt sich nicht erneut anzeigen, deshalb bleibt dieses Feld leer — einen neuen nur einsetzen, wenn du ihn austauschen willst.",
  "provider.apiKeyReplacePlaceholder":
    "Neuen Schlüssel einsetzen, um den hinterlegten zu ersetzen",
  "provider.apiKeyHint":
    "Wird nach der Prüfung sofort versiegelt. Er wird nie wieder angezeigt und verlässt diese Installation nur Richtung Anbieter.",
  "provider.connectConfirm.title": "Datenanbieter verbinden?",
  "provider.connectConfirm.body":
    "Der Schlüssel wird beim Anbieter geprüft, bevor irgendetwas gespeichert wird. Ab dann kostet jede Anreicherung Guthaben.",
  "provider.disconnect": "Trennen",
  "provider.disconnectConfirm.title": "Verbindung trennen?",
  "provider.disconnectConfirm.body":
    "Neue Abfragen hören sofort auf, der Schlüssel wird vernichtet. Bereits gekaufte Daten bleiben an den Datensätzen — Trennen löscht nichts.",
  "provider.deleteData": "Gekaufte Daten löschen",
  "provider.deleteDataConfirm.title":
    "Alles löschen, was von diesem Anbieter stammt?",
  "provider.deleteDataConfirm.body":
    "Jeder Wert dieses Anbieters verschwindet von jedem Kontakt. Was du ausgegeben hast, bleibt in den Aufzeichnungen; die Daten nicht. Das lässt sich nicht rückgängig machen.",
  "provider.deleteDataConfirm.typed":
    "Zum Bestätigen den Namen des Anbieters eingeben",
  "provider.automaticLookup": "Kontakte automatisch nachschlagen",
  "provider.automaticLookupHint":
    "Jeder Kontakt wird einmal nachgeschlagen — für das, was die Verbindung auswählt und der Anbieter nicht berechnet, in der Regel den beruflichen Profil-Link, die aktuelle Rolle samt Arbeitgeber und den Werdegang. E-Mail-Adressen und Mobilnummern werden so nie gekauft: die kosten Guthaben und bleiben eine Entscheidung pro Kontakt.",
  "provider.automaticLookupJurisdiction":
    "Schalt das aus, wenn deine Kontakte einem Recht unterliegen, das den Handel mit Personendaten verbietet, etwa dem vietnamesischen. Der Knopf auf dem einzelnen Kontakt funktioniert weiterhin, damit die Entscheidung bei dem bleibt, der sie trifft.",
  "provider.buyable": "Kauf von {category} erlauben",
  "provider.buyableHint_one":
    "Dieser Schalter kauft nichts. Er stellt bei jedem Kontakt eine Schaltfläche bereit, zum Preis von {credits} Guthaben, damit jemand diese Angabe für eine einzelne Person kaufen kann.",
  "provider.buyableHint_other":
    "Dieser Schalter kauft nichts. Er stellt bei jedem Kontakt eine Schaltfläche bereit, zum Preis von {credits} Guthaben, damit jemand diese Angabe für eine einzelne Person kaufen kann.",
  "provider.buyableNeeds":
    "Der Anbieter sucht danach nur zusammen mit {prerequisite}. Einzeln lässt es sich nicht kaufen — erlauben Sie zuerst diese Angabe.",
  "provider.backlog": "Noch nachzuschlagen",
  "provider.backlogRemaining_one": "{count} Kontakt",
  "provider.backlogRemaining_other": "{count} Kontakte",
  "provider.backlogWorking":
    "Kontakte, die es beim Verbinden schon gab, werden nach und nach nachgeschlagen.",
  "provider.backlogPaused":
    "Zurzeit wird nichts nachgeschlagen: automatische Abfragen sind aus, das Tageslimit ist aufgebraucht, oder der Anbieter ist nicht nutzbar.",
  "provider.credits": "Restguthaben beim Anbieter",
  "provider.credits.none": "Der Anbieter hat uns noch keinen Stand genannt.",
  "provider.credits.notConnected":
    "Mit einem hinterlegten Schlüssel siehst du hier dein Guthaben beim Anbieter.",
  "provider.constraints": "Geltende Grenzen",
  "provider.spend": "Was wir verbraucht haben",
  "provider.spend.hint":
    "Unsere eigene Aufzeichnung dessen, was die Anreicherung gekostet hat. Nicht die Rechnung des Anbieters — dieselben Guthaben lassen sich auch über dessen App ausgeben, die beiden Zahlen dürfen also auseinandergehen.",
  "provider.spend.thisMonth": "Diesen Monat",
  "provider.spend.month": "Monat",
  "provider.spend.pool": "Kontingent",
  "provider.spend.chargedHead": "Guthaben",
  "provider.spend.heldHead": "Reserviert",
  "provider.spend.runsHead": "Abfragen",
  "provider.spend.none": "Es wurde noch nichts gekauft.",

  // Der Abschnitt auf der Personenseite. Die drei „nichts da"-Zustände sind
  // mit Absicht drei verschiedene Sätze: nur bei einem davon kann der Leser
  // etwas tun.
  "provider.profile.title": "Zugekaufte Kontaktdaten",
  "provider.profile.notConnected":
    "Es ist kein Datenanbieter verbunden, also wurde nichts gekauft.",
  "provider.profile.notEligible":
    "Für diesen Kontakt nicht zulässig — er hat widersprochen, oder der Datensatz ist archiviert.",
  "provider.profile.nothingToLookUp":
    "Es gibt nichts, womit sich dieser Kontakt nachschlagen ließe. Tragen Sie die LinkedIn-URL oder das Unternehmen ein, dann kann die Suche laufen.",
  "provider.profile.neverRun":
    "Diesen Kontakt hat noch niemand nachgeschlagen.",
  "provider.profile.queued": "In der Warteschlange",
  "provider.profile.inProgress": "Wird nachgeschlagen …",
  "provider.profile.working":
    "{provider} wird gefragt. Das dauert bis zu einer Minute.",
  "provider.profile.landing":
    "Antwort da. Sie wird in den Datensatz übernommen.",
  "provider.profile.completed": "Gefunden",
  "provider.profile.noMatch": "Der Anbieter hatte nichts zu diesem Kontakt.",
  "provider.profile.stale":
    "Früher gekauft. Der Anbieter ist nicht mehr verbunden, eine Auffrischung ist deshalb nicht möglich.",
  "provider.profile.invalidCredentials":
    "Der Anbieter hat unseren Schlüssel abgelehnt, die Abfrage lief nicht.",
  "provider.profile.insufficientCredits":
    "Nicht gekauft: das Guthabenbudget für diesen Monat ist aufgebraucht.",
  "provider.profile.rateLimited":
    "Nicht gekauft: der Anbieter hat uns gebremst.",
  "provider.profile.providerError":
    "Die letzte Abfrage kam nicht durch. Versuchen Sie es erneut, oder sehen Sie in den Einstellungen auf der Karte des Anbieters nach, wenn es wieder passiert.",
  "provider.profile.submissionUnknown":
    "Wie diese Abfrage ausgegangen ist, haben wir nie erfahren. Sie kann berechnet worden sein.",
  "provider.profile.claimsUnwritten":
    "Bezahlt, aber die Daten sind nie an diesem Datensatz angekommen. Niemand muss danach suchen — das hier ist die Lücke.",
  "provider.profile.enrichNow": "Kontakt nachschlagen · kostenlos",
  "provider.profile.recheck": "Erneut prüfen · kostenlos",
  "provider.profile.lookingUp": "Wir fragen den Anbieter. Das dauert kurz.",
  "provider.profile.emptyTitle": "Für diesen Kontakt wurde noch nichts gekauft",
  "provider.profile.emptyBody":
    "Eine Abfrage holt bei {provider} Angaben zu diesem Kontakt — je nachdem, was für diese Verbindung eingekauft werden soll. Sie kostet {provider}-Credits, und was zurückkommt steht hier neben dem Datensatz; überschrieben wird nichts, was jemand aus dem Team eingetragen hat.",
  "provider.profile.emails": "E-Mail-Adressen",
  "provider.profile.emailType.provider": "{type}, so vom Anbieter bezeichnet",
  "provider.profile.emailType.requested":
    "{type}, weil wir genau danach gefragt haben",
  "provider.profile.mobiles": "Mobilnummern",
  "provider.profile.confidence": "{percent} % Sicherheit",
  "provider.profile.linkedin": "LinkedIn",
  "provider.profile.employment": "Aktuelle Rolle",
  "provider.profile.jobHistory": "Frühere Rollen",
  "provider.profile.location": "Standort",
  "provider.profile.departments": "Bereiche",
  "provider.profile.seniorities": "Ebene",
  "provider.profile.notRequested": "Nie angefragt: {categories}.",
  "provider.profile.buy_one": "{category} kaufen · {credits} Credit",
  "provider.profile.buy_other": "{category} kaufen · {credits} Credits",
  "provider.profile.buyRebuys":
    "Im Preis ist {categories} erneut enthalten: Der Anbieter sucht ohne diese Angabe nicht danach und berechnet alles, was er zurückliefert.",
  "provider.freeTier.hint":
    "LinkedIn-Profil, aktuelle Rolle und Werdegang kosten keine Credits. Am besten anlassen: jeder neue Kontakt bekommt sie, ohne dass jemand entscheiden muss.",
  "provider.pricedTier.hint":
    "Werden nie automatisch gekauft. Jemand drückt bei einem einzelnen Kontakt auf den Knopf, und der Preis steht darauf.",
  "provider.profile.receiptAt": "Abgefragt am {at}.",
  "provider.profile.receipt":
    "Abgefragt am {at} · {asked} Angaben angefragt, {answered} zurückbekommen.",
  "provider.profile.noAnswer": "Angefragt, nichts gefunden: {categories}.",
  "provider.category.professionalEmail": "geschäftliche E-Mail",
  "provider.category.personalEmail": "private E-Mail",
  "provider.category.mobile": "Mobilnummer",
  "provider.category.linkedin": "LinkedIn-Profil",
  "provider.category.currentEmployment": "aktuelle Rolle",
  "provider.category.jobHistory": "frühere Rollen",

  // Der Filter-Baukasten (AC-filters-and-views-3/4).
  "filters.joinAll": "ALLE \u00b7 UND",
  "filters.joinAny": "BELIEBIGE \u00b7 ODER",
  "filters.joinLabel": "Wie diese Gruppe ihre Bedingungen verkn\u00fcpft",
  "filters.removeGroup": "Gruppe entfernen",
  "filters.addGroup": "Gruppe hinzuf\u00fcgen",
  "filters.addClause": "Bedingung hinzuf\u00fcgen",
  "filters.emptyGroup":
    "Noch keine Bedingungen \u2014 eine leere Gruppe trifft auf nichts zu, also f\u00fcgen Sie eine hinzu.",
  "filters.field": "Feld",
  "filters.choosePlaceholder": "Feld ausw\u00e4hlen",
  "filters.customBadge": "eigenes Feld",
  "filters.operator": "Operator",
  "filters.value": "Wert",
  "filters.values": "Werte",
  "filters.addValue": "Wert hinzuf\u00fcgen",
  "filters.removeClause": "Bedingung {field} entfernen",
  "filters.existsLabel": "Ob das Feld einen Wert hat",
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
  "filters.title": "Filter & Ansichten",
  "filters.subtitle":
    "Filter erstellen, Treffer beobachten und als Ansicht speichern.",
  "filters.objectLabel": "Welche Datens\u00e4tze gefiltert werden",
  "filters.tab.contacts": "Kontakte",
  "filters.tab.companies": "Firmen",
  "filters.tab.deals": "Gesch\u00e4fte",
  "filters.builderTitle": "Filter",
  "filters.dynamic": "Dynamisch \u2014 bei jedem Ereignis neu berechnet",
  "filters.matchContacts": "{count} Kontakte treffen zu",
  "filters.matchCompanies": "{count} Firmen treffen zu",
  "filters.matchDeals": "{count} Gesch\u00e4fte treffen zu",
  "filters.noFilterYet": "Bedingung hinzuf\u00fcgen, um die Treffer zu sehen",
  "filters.countUnavailable": "Anzahl nicht verf\u00fcgbar",
  "filters.loadingVocabulary": "Filterbare Felder werden geladen\u2026",
  "filters.noFields": "Keine filterbaren Felder f\u00fcr diesen Datensatztyp.",
  "filters.resultsTitle": "Passende Datens\u00e4tze",
  "filters.resultsCaption":
    "Die erste Seite der Treffer \u2014 genug, um den Filter zu pr\u00fcfen, nicht die gesamte Auswahl.",
  "filters.noMatches": "Keine Datens\u00e4tze entsprechen diesem Filter.",
  "filters.loadView": "Gespeicherten Filter laden",
  "filters.pickRecord": "Eintrag wählen",
  "filters.loadingRecords": "Auswahl wird geladen…",
  "filters.pickValue": "Wert wählen",
  "filters.exportCsv": "Als CSV exportieren",
  "filters.exportJson": "Als JSON exportieren",

  // Siehe en.ts: zwei Leser gleichzeitig — wer hinein will, und wer es
  // reparieren muss.
  "release.skewTitle": "Diese Installation wird gerade aktualisiert",
  "release.skewBody":
    "Die App in Ihrem Browser und der Server dahinter stammen aus unterschiedlichen Releases, deshalb funktioniert hier nichts verlässlich. Laden Sie neu, um die aktuelle Version zu holen. Bleibt diese Meldung, sagen Sie es der Person, die diese Installation betreibt: Jeder Teil davon muss dasselbe Release ausführen.",
  "release.skewVersions": "App {app} · Server {server}",
  "release.skewReload": "Neu laden",

  // Die Warteschlange hinter „später senden“. „Zurückziehen“ statt „löschen“
  // oder „abbrechen“: es wurde nichts übertragen und nichts erscheint in der
  // Chronik, also gibt es keinen Versand abzubrechen und keinen Datensatz zu
  // löschen — die Nachricht wird zurückgenommen, bevor sie geht.
  "nav.scheduled": "Geplante Nachrichten",
  "sched.sub":
    "Nachrichten, die Sie geschrieben haben und die noch nicht hinausgegangen sind. Nur Sie sehen sie.",
  "sched.empty": "Sie haben noch keine Nachricht geplant.",
  "sched.group.held": "Angehalten, wartet auf Sie",
  "sched.group.heldEmpty": "Nichts wurde angehalten.",
  "sched.group.waiting": "Wartet auf den Versand",
  "sched.group.waitingEmpty": "Nichts wartet auf den Versand.",
  "sched.group.closed": "Wartet nicht mehr",
  "sched.group.closedEmpty":
    "Bisher ist nichts hinausgegangen oder zurückgezogen worden.",
  "sched.status.scheduled": "Wartet",
  "sched.status.released": "Geht hinaus",
  "sched.status.sent": "Gesendet",
  "sched.status.cancelled": "Zurückgezogen",
  "sched.status.held": "Angehalten",
  "sched.held.consentWithdrawn":
    "Ein Empfänger hat seine Einwilligung zurückgezogen, nachdem Sie diese Nachricht geplant haben. Sie geht erst hinaus, wenn Sie ihm unter einem Zweck schreiben, dem er zugestimmt hat.",
  "sched.held.senderInactive":
    "Ihr Sitz oder Ihr Postfach hat sich nach der Planung geändert, deshalb kann die Nachricht nicht in Ihrem Namen gesendet werden.",
  "sched.held.missedWindow":
    "Der Zeitpunkt verstrich, während nichts lief, und ist jetzt zu spät für die Nachricht, die Sie geschrieben haben. Verschieben Sie sie oder ziehen Sie sie zurück.",
  "sched.held.timerExhausted":
    "Dem Auftrag, der diese Nachricht weckt, sind die Versuche ausgegangen. Verschieben Sie sie auf einen neuen Zeitpunkt, um es erneut zu versuchen.",
  "sched.held.sendRefused":
    "Eine Prüfung hat diese Nachricht bei Fälligkeit abgelehnt. Es wurde nichts gesendet.",
  "sched.inZone": "in der Zone {zone}",
  "sched.recipientsUnknown": "Kein Empfänger in dieser Nachricht",
  "sched.recipientsMore": "{first} und {count} weitere",
  "sched.move": "Zeitpunkt ändern",
  "sched.moveTo": "Neuer Zeitpunkt für „{subject}“",
  "sched.moveSave": "Verschieben",
  "sched.moveCancel": "Belassen",
  "sched.withdraw": "Zurückziehen",
  "sched.withdrawTitle": "Diese Nachricht zurückziehen?",
  "sched.withdrawBody":
    "„{subject}“ wird nicht gesendet, und nichts erscheint in der Chronik. Sie erneut zu schreiben heißt, sie von vorn zu verfassen.",
  "sched.withdrawConfirm": "Zurückziehen",
  "sched.skew":
    "Diese Liste ist nicht mehr aktuell: die Nachricht, auf die Sie eingewirkt haben, war schon hinausgegangen, zurückgezogen oder auf einen anderen Zeitpunkt verschoben. Lesen Sie die Liste erneut.",
  "sched.reload": "Erneut lesen",
  "nav.projects": "Projekte",
  "unit.projects": "Projekte",
  "companyProjects.title": "Projekte",
  "companyProjects.empty":
    "Ein Projekt ist die Arbeit, um die es in einem Deal geht. Dieses Unternehmen erscheint hier, sobald es an einem beteiligt ist — als Kunde, Partner oder Subunternehmer.",
  "projectCompanies.title": "Unternehmen",
  "projectCompanies.empty":
    "Ein Projekt ist Arbeit, die mehrere Unternehmen gemeinsam leisten — der Kunde und jeder Partner oder Subunternehmer, der liefert.",
  "projectCompanies.attach": "Unternehmen verknüpfen",
  "projectCompanies.detachTitle": "Dieses Unternehmen entfernen?",
  "projectCompanies.searchLabel": "Unternehmen nach Name suchen",
  "personProjects.title": "Projekte",
  "personProjects.empty":
    "Dieser Kontakt erscheint hier, sobald er an einer Lieferung beteiligt ist — als Sponsor, Ansprechpartner oder wer sonst daran arbeitet.",
  "projectRole.customer": "Kunde",
  "projectRole.partner": "Partner",
  "projectRole.subcontractor": "Subunternehmer",
  "personRole.sponsor": "Sponsor",
  "personRole.projectLead": "Projektleitung",
  "personRole.deliveryLead": "Lieferverantwortung",
  "personRole.expert": "Fachexperte",
  "personRole.user": "Anwender",
  "projectLinks.new": "Neues Projekt",
  "projectLinks.attach": "Projekt verknüpfen",
  "projectLinks.move": "Zu anderem Projekt verschieben",
  "projectLinks.detach": "Verknüpfung lösen",
  "projectLinks.detachConfirm": "Verknüpfung lösen",
  "projectLinks.detachNamed": "{name} lösen",
  "projectLinks.roleLabel": "Als",
  "projectLinks.detachTitle": "Verknüpfung zu diesem Projekt lösen?",
  "projectLinks.detachBody":
    "{name} bleibt unverändert. Nur die Verknüpfung zu diesem Datensatz endet — es wird nichts gelöscht.",
  "projectLinks.emptyTitle": "Noch keine Projekte",
  "projectLinks.searchLabel": "Projekte nach Name oder Kürzel suchen",
  "project.name": "Projektname",
  "project.keyMinted":
    "Margince vergibt jedem Projekt ein kurzes Kürzel. Steht [{key}] im Betreff einer E-Mail, wird sie diesem Projekt zugeordnet.",
  "project.company": "Unternehmen",
  "project.owner": "Verantwortlich",
  "project.ownerKeep": "Aktuelle Verantwortung behalten",
  "project.ownerMe": "Ich",
  "project.ownerUnassign": "Niemand",
  "project.assignOwner": "Einer Kolleg:in zuweisen",
  "project.assignOwnerTitle": "Einer Kolleg:in zuweisen",
  "project.assignOwnerSearch": "Kolleg:innen suchen",
  "project.assignOwnerNoneSelected": "Erst eine Kolleg:in auswählen",
  "project.assignOwnerDone": "{name} zugewiesen",
  "project.description": "Beschreibung",
  "project.targetEnd": "Geplantes Ende",
  "project.targetEndShort": "Ziel {date}",
  "project.new": "Neues Projekt",
  "project.edit": "Projekt bearbeiten",
  "project.archive": "Projekt archivieren",
  "project.archiveConfirm":
    "Durch das Archivieren verschwindet dieses Projekt aus der aktiven Liste und sein Kürzel wird frei. Dies kann in der Oberfläche nicht rückgängig gemacht werden.",
  "project.archivedReadOnly":
    "Dieses Projekt ist archiviert und nimmt keine Änderungen an.",
  "project.notYoursToChange":
    "Sie können dieses Projekt nicht ändern. Bitten Sie den Inhaber um Freigabe oder Ihre Administration um das Recht zum Bearbeiten.",
  "project.phaseLabel": "Phase",
  "project.filterPhaseAll": "Alle Phasen",
  "project.viewDelivering": "In Umsetzung",
  "project.phase.initiative": "Initiative",
  "project.phase.pursuing": "Im Vertrieb",
  "project.phase.delivering": "In Umsetzung",
  "project.phase.closed": "Abgeschlossen",
  "project.emptyTitle": "Noch keine Projekte",
  "project.emptyBody":
    "Ein Projekt ist das Vorhaben, um das es in einem Deal geht. Es beginnt während des Deals in der Phase Initiative und überlebt den Abschluss: Ist der Deal gewonnen, wird die Umsetzung hier verfolgt.",
  "project.emptyKey":
    "Jedes Projekt bekommt ein kurzes Kürzel. Jede E-Mail, die es in eckigen Klammern im Betreff trägt, wird dem Projekt automatisch zugeordnet.",
  "project.rollups.empty": "Noch keine Kennzahlen zu diesem Projekt.",
  "project.rollups.openValue": "Offenes Dealvolumen",
  "project.rollups.wonValue": "Gewonnenes Dealvolumen",
  "project.rollups.openCommitments": "Offene Zusagen",
  "project.rollups.lastActivity": "Letzte Aktivität",
  "project.rollups.never": "noch nichts",
  "project.rollups.activityCount": "Aktivitäten",
  "project.history.title": "Phasenverlauf",
  "project.history.empty": "Noch kein Phasenwechsel erfasst.",
  "project.history.current": "aktuell",
  "project.history.moved": "{from} → {to}",
  "project.history.born": "Gestartet in {phase}",
  "project.history.bySystem": "System",
  "project.deals.title": "Deals",
  "project.deals.empty":
    "Noch kein Deal nennt dieses Projekt. Ein Deal wählt sein Projekt in seinem eigenen Formular.",
  "project.deals.more": "Mehr Deals als hier gezeigt — öffne die Pipeline.",
  "project.stakeholders.title": "Beteiligte",
  "project.stakeholders.empty":
    "Noch niemand ist an diesem Projekt beteiligt. Beteiligte sind Personen mit einer Rolle hier — Sponsor, Projektleitung, Champion.",
  "project.stakeholders.add": "Beteiligten hinzufügen",
  "project.stakeholders.addHint":
    "Eine Rolle pro Person. Wer schon an diesem Projekt beteiligt ist, wechselt auf die hier gewählte Rolle.",
  "project.stakeholders.searchLabel": "Personen nach Namen suchen",
  "project.stakeholders.removeTitle": "Diese Person vom Projekt nehmen?",
  "project.stakeholders.removeConfirm":
    "{name} ist dann nicht mehr an diesem Projekt beteiligt. Die Aktivitäten bleiben erhalten.",
  "project.stakeholders.removeOne": "{name} vom Projekt nehmen",
  "project.role.sponsor": "Sponsor",
  "project.role.project_lead": "Projektleitung",
  "project.role.delivery_lead": "Umsetzungsleitung",
  "project.role.subject_matter_expert": "Fachexperte",
  "project.contracts.title": "Verträge",
  "project.contracts.empty":
    "Unter diesem Projekt ist kein Vertrag abgelegt. Ein Vertrag nennt sein Projekt beim Erfassen.",
  "project.documents.title": "Dokumente",
  "project.documents.empty":
    "An diesem Projekt hängt keine Datei. Dateien an seinen Deals bleiben bei den Deals.",
  "project.commitments.title": "Offene Zusagen",
  "project.commitments.empty":
    "Unter diesem Projekt ist keine offene Aufgabe abgelegt. Verknüpfte Aufgaben erscheinen hier, die nächste Fälligkeit zuerst.",
  "project.commitments.overdue": "überfällig",
  "project.timeline.empty":
    "Unter diesem Projekt ist noch nichts abgelegt. E-Mails mit dem Kürzel im Betreff und verknüpfte Aktivitäten erscheinen hier.",
  "project.advance.title": "Wechsel zu {phase}",
  "project.advance.confirm": "Wechseln",
  "project.advance.close": "Projekt abschließen",
  "project.advance.body":
    "Der Wechsel wird mit deiner Begründung im Phasenverlauf festgehalten.",
  "project.advance.closeBody":
    "Der Abschluss beendet die Umsetzung. Das Projekt kann später wieder geöffnet werden; die Begründung bleibt erhalten.",
  "project.advance.reason": "Begründung",
  "project.advance.reasonRequired":
    "Ein abgeschlossenes Projekt braucht eine Begründung.",
  "deal.project": "Projekt",
  "deal.projectNew": "Neues Projekt…",
  "deal.projectWithheld": "Projekt nicht sichtbar",
  "deal.projectNeedsCompany":
    "Wähle zuerst das Unternehmen des Deals — ein Projekt wird an einem Unternehmen gestartet.",
  "deal.projectUnnamed": "Projekt",
  "deal.startDeliveryTitle": "Umsetzung starten",
  "deal.startDelivery": "Umsetzung starten",
  "deal.startDeliveryAttached":
    "Dieser Deal hängt an {project}, das Projekt ist aber noch nicht in der Umsetzung. Jetzt wechseln?",
  "deal.startDeliveryBody":
    "Dieser Deal ist gewonnen und nennt kein Projekt. An {project} anhängen und das Projekt in die Umsetzung bringen?",

  // The Worklist's own words: the ranked queue, its dials, and the phrase
  // for every fact the server sends as a closed vocabulary.
  "worklist.loading": "Dein Tag wird gelesen…",
  "worklist.queue": "Was als Nächstes zu tun ist",
  "worklist.more": "Mehr anzeigen",
  "worklist.more.failed": "Konnte nicht mehr laden. Bitte erneut versuchen.",
  "worklist.summary":
    "{urgent} dringend · {due} fällig · {inPlay} in Arbeit · {lower} nachrangig — {total} insgesamt",
  "worklist.summary.noMiddle":
    "{urgent} dringend · {due} fällig · {lower} nachrangig — {total} insgesamt",
  "worklist.completeness": "{shown} von {considered} angezeigt",
  "worklist.completeness.bounded":
    "{shown} angezeigt · {sources} Quellen haben mehr",
  "worklist.clear": "Nichts wartet auf dich.",
  "worklist.clearOfWhatWasRead":
    "Unter den Quellen, die geantwortet haben, wartet nichts.",
  "worklist.partial": "{sources} — das ist nicht der ganze Tag.",
  "worklist.overdue": "Überfällig",
  "worklist.pair.ask": "Welcher Datensatz soll bleiben?",
  "worklist.pair.keep": "{name} behalten",
  "worklist.pair.notDuplicate": "Nicht dieselben",
  "worklist.pair.related": "{count} verknüpft",
  "worklist.pair.failed":
    "Paar konnte nicht entschieden werden. Bitte erneut versuchen.",
  "worklist.needsPrep": "Unvorbereitet",
  "worklist.pane.title": "Zu diesem Datensatz",
  "worklist.pane.openRow": "Zeigen, worum es bei {position}, {title}, geht",
  "worklist.pane.loading": "Datensatz wird gelesen…",
  "worklist.pane.nothing": "Noch nichts erfasst.",
  "worklist.pane.lastInbound": "Zuletzt geschrieben",
  "worklist.pane.lastOutbound": "Wir zuletzt geschrieben",
  "worklist.pane.never": "Nie",
  "worklist.focus.title": "Das als Nächstes",
  "worklist.nextup.title": "Und danach",
  "worklist.focus.verb.decide": "Entscheiden",
  "worklist.focus.verb.merge": "Paar prüfen",
  "worklist.focus.verb.complete": "Erledigen",
  "worklist.focus.verb.act": "Bearbeiten",
  "worklist.focus.verb.acknowledge": "Zur Kenntnis nehmen",
  "worklist.focus.verb.open": "Öffnen",
  "worklist.focus.verb.snooze": "Öffnen",
  "worklist.focus.verb.dismiss": "Öffnen",
  "worklist.focus.verb.set_aside": "Öffnen",
  "worklist.band.now": "Jetzt",
  "worklist.band.build_pipeline": "Pipeline aufbauen",
  "worklist.band.keep_momentum": "In Bewegung halten",
  "worklist.band.review": "Prüfen",
  "worklist.disposition.verb.snooze": "Später",
  "worklist.disposition.verb.not_mine": "Nicht meins",
  "worklist.disposition.verb.not_sales": "Kein Kunde",
  "worklist.disposition.done.snooze": "Morgen wieder auf deiner Liste.",
  "worklist.disposition.done.not_mine":
    "Von deiner Liste. Wer zuständig ist, sieht es weiterhin.",
  "worklist.disposition.done.not_sales": "Von allen Listen entfernt.",
  "worklist.disposition.undo": "Rückgängig",
  "worklist.disposition.undoFailed":
    "Das konnte nicht rückgängig gemacht werden. Die Nachricht ist weiterhin von deiner Liste.",
  "worklist.disposition.failed": "Das konnte nicht abgelegt werden.",
  "worklist.scope.label": "Wessen Arbeit",
  "worklist.scope.mine": "Meine",
  "worklist.scope.unassigned": "Ohne Zuständigkeit",
  "worklist.scope.team": "Mein Team",
  "worklist.scope.all": "Alle",
  "worklist.owner.label": "Wessen Liste",
  "worklist.manager.cancel": "Abbrechen",
  "worklist.owner.mine": "Mein eigener Tag",
  "worklist.owner.backToMine": "Zurück zu meinem Tag",
  "worklist.manager.reassign": "Neu zuweisen",
  "worklist.manager.reassignTo": "Übergeben an",
  "worklist.manager.reassignConfirm": "Übergeben",
  "worklist.manager.reassigned": "Übergeben.",
  "worklist.manager.reassignFailed": "Das konnte nicht übergeben werden.",
  "worklist.manager.coach": "Notiz hinterlassen",
  "worklist.manager.coachAbout": "Worum geht es",
  "worklist.manager.coachConfirm": "Notiz hinterlassen",
  "worklist.manager.coached": "Deine Notiz liegt auf der Liste.",
  "worklist.manager.coachFailed": "Die Notiz konnte nicht hinterlassen werden.",
  "worklist.manager.note": "Deine Notiz (optional)",
  "worklist.manager.kind.reply_aging": "Eine alternde Antwort",
  "worklist.manager.kind.next_step": "Der nächste Schritt eines Deals",
  "worklist.manager.kind.review_backlog": "Prüfarbeit",
  "worklist.manager.kind.general": "Etwas anderes",
  "worklist.board.title": "Wie es meinem Team geht",
  "worklist.board.loading": "Die Arbeit deines Teams wird gelesen…",
  "worklist.board.empty": "Bisher ist niemand mit dir in einem Team.",
  "worklist.board.member": "Wer",
  "worklist.board.waiting": "Warten auf Antwort",
  "worklist.board.atRisk": "Gefährdete Deals",
  "worklist.board.overdue": "Überfällig",
  "worklist.board.nobody": "Noch niemand",
  "worklist.board.truncated":
    "Es gibt mehr Arbeit, als hier gezählt werden konnte. Das sind Untergrenzen, keine Gesamtzahlen.",
  "worklist.readings.label": "Was heute auf dem Spiel steht",
  "worklist.readings.revenue": "Umsatz in Gefahr",
  "worklist.readings.revenue.detail": "Über die heute treibenden Deals",
  "worklist.readings.revenue.unpriced": "Kein gefährdeter Deal war bewertbar",
  "worklist.readings.replies": "Kundenantworten",
  "worklist.readings.replies.detail": "Kunden warten auf eine Antwort",
  "worklist.readings.prospecting": "Neugeschäft",
  "worklist.readings.prospecting.detail":
    "Neugeschäft, das eine erste Antwort schuldet",
  "worklist.readings.review": "Prüfung",
  "worklist.readings.review.detail":
    "Routinearbeit, die hinter einer Entscheidung wartet",
  "worklist.readings.truncated":
    "Es gibt mehr Arbeit, als hier gezählt werden konnte. Das sind Untergrenzen, keine Gesamtzahlen.",
  "worklist.hidden.title": "Was die Liste nicht zeigt",
  "worklist.hidden.loading": "Wird geprüft, was zurückgehalten wird…",
  "worklist.hidden.clear":
    "Es wird nichts zurückgehalten. Jeder wartende Kunde erreicht eine Liste.",
  "worklist.hidden.truncated":
    "Es gibt mehr Arbeit, als hier gezählt werden konnte. Das sind Untergrenzen, keine Gesamtzahlen.",
  "worklist.hidden.count": "{count} warten",
  "worklist.hidden.pastHorizon": "Zu alt für die Liste",
  "worklist.hidden.pastHorizon.detail":
    "Das hat niemand entschieden. Sie schrieben vor Monaten und bekamen nie eine Antwort.",
  "worklist.hidden.unlinked": "Keinem Datensatz zugeordnet",
  "worklist.hidden.unlinked.detail":
    "Meist kein Vertrieb. Manchmal ein Kunde, den niemand zuordnen konnte.",
  "worklist.hidden.notSales": "Als vertriebsfremd eingestuft",
  "worklist.hidden.notSales.detail":
    "Für die gesamte Organisation verborgen, und es hebt sich nicht auf.",
  "worklist.hidden.setAside": "Von Ihnen zurückgestellt",
  "worklist.hidden.setAside.detail":
    "Zurückgestellt oder als nicht Ihre markiert. Eine Zurückstellung kommt von selbst zurück.",
  "worklist.hidden.shown": "Die Liste selbst führt {count}.",
  "worklist.filter.label": "Art der Arbeit",
  "worklist.filter.all": "Alle",
  "worklist.filter.customer_waiting": "Kunde wartet",
  "worklist.filter.leads": "Leads",
  "worklist.filter.deals_at_risk": "Gefährdete Deals",
  "worklist.filter.meetings": "Termine",
  "worklist.filter.tasks": "Aufgaben",
  "worklist.filter.decisions": "Entscheidungen",
  "worklist.filter.system": "System",
  "worklist.category.customer_waiting": "Kunde wartet",
  "worklist.category.leads": "Lead",
  "worklist.category.deals_at_risk": "Deal gefährdet",
  "worklist.category.meetings": "Termin",
  "worklist.category.tasks": "Aufgabe",
  "worklist.category.decisions": "Entscheidung",
  "worklist.category.system": "System",
  "worklist.because.pinned": "Von dir angeheftet",
  "worklist.because.buyer_wrote_last": "Sie haben zuletzt geschrieben",
  "worklist.because.waiting_days": "wartet",
  "worklist.because.waiting_days.value_one": "wartet seit {value} Tag",
  "worklist.because.waiting_days.value_other": "wartet seit {value} Tagen",
  "worklist.because.overdue": "überfällig",
  "worklist.because.due_today": "heute fällig",
  "worklist.because.closing_soon": "hat ein Abschlussdatum",
  "worklist.because.expected_revenue": "ein offener Deal hängt daran",
  "worklist.because.expected_revenue.value": "Wert {value}",
  "worklist.because.material": "über dem üblichen offenen Deal",
  "worklist.because.material.value":
    "Wert {value}, über dem üblichen offenen Deal",
  "worklist.because.below_material": "unter dem üblichen offenen Deal",
  "worklist.because.below_material.value":
    "Wert {value}, unter dem üblichen offenen Deal",
  "worklist.because.quiet_days": "still geworden",
  "worklist.because.quiet_days.value_one": "seit {value} Tag still",
  "worklist.because.quiet_days.value_other": "seit {value} Tagen still",
  "worklist.because.no_champion": "kein Fürsprecher",
  "worklist.because.promised": "du hast es zugesagt",
  "worklist.because.approved_and_failed":
    "du hast zugestimmt, es lief aber nicht",
  "worklist.because.blocks_customer_work": "ein Kunde wartet darauf",
  "worklist.because.routine": "Routinepflege",
  "worklist.because.repeated_failure": "dasselbe schlägt immer wieder fehl",
  "worklist.because.legal_deadline": "eine gesetzliche Frist läuft",
  "worklist.because.meeting_soon": "beginnt gleich",
  "worklist.because.meeting_unprepared": "nichts vorbereitet",
  "worklist.because.response_overdue": "Antwort überfällig",
  "worklist.because.response_due_soon": "Antwort bald fällig",
  "worklist.because.response_due_soon.value": "Antwort fällig bis {value}",
  "worklist.because.unassigned": "niemand zuständig",
  "worklist.because.stale": "wartet schon lange",
  "worklist.above.pin": "Über dem Nächsten, weil du es angeheftet hast.",
  "worklist.above.level": "Über dem Nächsten, weil es dringlichere Arbeit ist.",
  "worklist.above.deadline": "Über dem Nächsten wegen des Datums.",
  "worklist.above.deadline.pair": "Über dem Nächsten: {mine} gegen {theirs}.",
  "worklist.above.expected_revenue":
    "Über dem Nächsten wegen des erwarteten Umsatzes.",
  "worklist.above.expected_revenue.pair":
    "Über dem Nächsten: {mine} gegen {theirs}.",
  "worklist.above.waiting_days": "Über dem Nächsten wegen der Wartezeit.",
  "worklist.above.waiting_days.pair":
    "Über dem Nächsten: {mine} gegen {theirs}.",
  "worklist.above.relationship":
    "Über dem Nächsten wegen der engeren Beziehung.",
  "worklist.above.crowded":
    "Über dem Nächsten, weil davon viele gleichzeitig anstehen.",
  "worklist.consequence.buyer_waits": "Wenn du nichts tust, warten sie weiter.",
  "worklist.consequence.promise_breaks":
    "Wenn du nichts tust, brichst du eine Zusage.",
  "worklist.consequence.deal_drifts":
    "Wenn du nichts tust, treibt der Deal weiter ab.",
  "worklist.consequence.deal_slips_past_close":
    "Wenn du nichts tust, verstreicht das vereinbarte Datum.",
  "worklist.consequence.meeting_unprepared":
    "Wenn du nichts tust, gehst du unvorbereitet hinein.",
  "worklist.consequence.task_slips": "Wenn du nichts tust, bleibt es liegen.",
  "worklist.consequence.work_blocked":
    "Wenn du nichts tust, bleibt die Arbeit blockiert.",
  "worklist.consequence.customer_never_received":
    "Wenn du nichts tust, erhält der Kunde es nie.",
  "worklist.consequence.you_believe_it_happened":
    "Wenn du nichts tust, glaubst du weiter, es sei passiert.",
  "worklist.consequence.legal_deadline_missed":
    "Wenn du nichts tust, verstreicht eine gesetzliche Frist.",
  "worklist.consequence.mailbox_blind":
    "Wenn du nichts tust, fehlt dieser Seite weiter, was nicht ankommt.",
  "worklist.consequence.data_drifts": "Wenn du nichts tust, driften die Daten.",
  "worklist.untitled.approval": "Eine Entscheidung wartet",
  "worklist.untitled.dedupe_candidate": "Zwei Datensätze sehen gleich aus",
  "worklist.untitled.task": "Eine Aufgabe",
  "worklist.untitled.brief_item": "Die Nacht hat das herausgesucht",
  "worklist.untitled.conversation_claim": "Eine Zusage von dir",
  "worklist.untitled.customer_waiting": "Jemand wartet auf Antwort",
  "worklist.untitled.lead_response": "Ein Lead",
  "worklist.untitled.deal_at_risk": "Ein Deal treibt ab",
  "worklist.untitled.meeting": "Ein Termin",
  "worklist.untitled.relationship_decay": "Eine Beziehung schläft ein",
  "worklist.untitled.failed_approval": "Etwas Zugestimmtes lief nicht",
  "worklist.untitled.dsr": "Eine offene Datenschutzanfrage",
  "worklist.untitled.sync_health":
    "Die CRM-Synchronisierung braucht Aufmerksamkeit",
  "worklist.untitled.capture_health":
    "Eine Postfachverbindung braucht Aufmerksamkeit",
  "worklist.untitled.ai_work_health": "KI-Arbeit braucht einen Blick",
  "worklist.untitled.bounce": "Eine E-Mail kam nicht an",
  "worklist.untitled.undelivered": "Eine E-Mail wurde nie gesendet",
  "worklist.untitled.automation_run": "Eine Regel hat nicht funktioniert",
  "worklist.untitled.notice": "Ein Hinweis für dich",
  "worklist.untitled.introduction_request":
    "Ein Kollege bittet dich um eine Vorstellung",
  "worklist.verb.decide": "Entscheiden",
  "worklist.verb.merge": "Zusammenführen",
  "worklist.verb.open": "Öffnen",
  "worklist.verb.complete": "Öffnen",
  "worklist.verb.snooze": "Öffnen",
  "worklist.verb.acknowledge": "Verstanden",
  "worklist.verb.acknowledgeFailed":
    "Das konnte nicht als gelesen markiert werden.",
  "worklist.source.failed": "{source} konnte nicht gelesen werden",
  "worklist.source.withheld": "{source} ist für dein Konto nicht sichtbar",
  "worklist.untitled.generic": "Etwas braucht dich",
  "worklist.batch.likely_automated": "{count} vermutlich automatische Absender",
  "worklist.batch.company_match": "{count} Adressen bei bekannten Firmen",
  "worklist.batch.uncertain_contact": "{count} Adressen zu entscheiden",
  "worklist.batch.duplicates": "{count} mögliche Dubletten",
  "worklist.batch.held_draft": "{count} Entwürfe warten auf Freigabe",
  "worklist.untitled.batch": "Eine Gruppe Routineentscheidungen",
  "worklist.verb.review_batch": "Durchsehen",
  "worklist.verb.draft_reply": "Zum Antworten öffnen",
  // Wo der Editor wirklich aufgeht, ist das Verb die HANDLUNG.
  "worklist.verb.draft_reply_now": "Antwort entwerfen",
  // Eine ERSTE Nachricht, keine Antwort auf eine bestehende.
  "worklist.verb.draft_email": "Zum Schreiben öffnen",
  "worklist.verb.draft_email_now": "E-Mail entwerfen",
  "worklist.deal.closes": "Abschluss {date}",
  "worklist.batch.system_incident": "{cause} ist {count}-mal fehlgeschlagen",
  "worklist.batch.unnamedCause": "Etwas",
  "person.readings.title": "Wo dieser Kontakt steht",
  "person.readings.move": "Wer ist am Zug",
  "person.readings.yourMove": "Du",
  "person.readings.theirMove": "Der Kontakt",
  "person.readings.quiet": "Verstummt",
  "person.readings.neverSpoke": "Noch nie gesprochen",
  "person.readings.lastFromThem": "zuletzt von ihnen: {when}",
  "person.readings.neverReplied": "bisher nichts von ihnen",
  "person.readings.promises": "Offene Zusagen",
  "person.readings.nothingOwed": "nichts offen",
  "person.readings.onTime": "noch nichts überfällig",
  "person.readings.deal": "Deals, die sie entscheiden",
  "person.readings.openDeals": "Deals öffnen",
  "person.readings.openMeetings": "Termine öffnen",
  "deal360.brief": "Was dieser Deal ist",
  "deal.strip.openHistory": "Zum Verlauf",
  "deal.strip.lastTouch": "Letzter Kontakt",
  "lead.standing.qualified": "Qualifiziert",
  "lead.standing.qualifiedOn":
    "Qualifiziert am {at}. Dieser Lead ist jetzt ein Kontakt.",
  "lead.standing.qualifiedUndated": "Dieser Lead ist jetzt ein Kontakt.",
  "lead.standing.closed": "Geschlossen",
  "lead.standing.closedFor":
    "Geschlossen: {reason}. Der Datensatz bleibt als Spur.",
  "lead.standing.closedUnreasoned":
    "Geschlossen. Der Datensatz bleibt als Spur.",
  "lead.standing.yourMove": "Du bist am Zug",
  "lead.standing.noResponse": "Niemand hat diesen Lead bisher beantwortet.",
  "lead.standing.theirMove": "Der Lead ist am Zug",
  "lead.standing.answeredOn":
    "Wir haben am {at} geantwortet. Bisher kam nichts zurück.",
  "lead.standing.inMotion": "In Bewegung",
  "lead.standing.engagedBecause":
    "Der Lead hat geantwortet, oder ein Termin steht im Kalender.",
  "lead.standing.rests.promoted": "Zu einem Kontakt befördert.",
  "lead.standing.rests.closed": "Disqualifiziert, kein Grund erfasst.",
  "lead.standing.rests.ladder": "Lead-Leiter",
  "lead.standing.rests.record": "Lead-Datensatz",
  "lead.standing.rests.captured": "Erfasst am {at}.",
  "lead.standing.rests.noResponse": "Keine erste Antwort erfasst.",
  "lead.standing.rests.engaged": "Engagement erfasst am {at}.",
  "lead.readings.title": "Wo dieser Lead steht",
  "lead.readings.firstResponse": "Erste Antwort",
  "lead.readings.noClock": "kein Antwortziel gesetzt",
  "lead.readings.owed": "Steht aus",
  "lead.today.answer": "{name} antworten",
  "lead.today.answerMeta": "Erste Antwort steht aus",
  "lead.today.nextTask": "Nächste Aufgabe",
  "lead.readings.answered": "Beantwortet",
  "lead.standing.dueBy":
    "Noch hat niemand geantwortet. Die erste Antwort ist bis {at} fällig.",
  "lead.standing.overdueSince":
    "Noch hat niemand geantwortet. Die erste Antwort war am {at} fällig.",
} as const satisfies Record<MessageKey, string>;
