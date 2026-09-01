# Verzeichnis von Verarbeitungstätigkeiten und Datenschutz-Folgenabschätzung

> Vorlage. Jede Zeile nennt den Code-Pfad, der die Zusage durchsetzt — damit
> prüfbar ist, ob das Verzeichnis das Produkt beschreibt oder eine Absicht.

**Verantwortlicher:** [Firma, Anschrift] · **Stand:** [Datum]

## Teil A — Verzeichnis nach Art. 30 DSGVO

### A.1 Erfassung dienstlicher E-Mail-Korrespondenz

| Feld | Angabe |
| --- | --- |
| Zweck | Dokumentation geschäftlicher Vorgänge im CRM |
| Betroffene | Beschäftigte; deren Korrespondenzpartner |
| Datenkategorien | Absender, Empfänger, Betreff, Text, Anhänge, Zeitpunkt |
| Rechtsgrundlage | Art. 6 Abs. 1 lit. b, f DSGVO; § 26 Abs. 1 BDSG; bei Privatnutzung zusätzlich lit. a |
| Empfänger | keine Übermittlung an Dritte |
| Löschfrist | Handelsbriefe [Frist]; übrige [Frist] |
| Durchsetzung | `capture/sinkactivity.go` schreibt jede Nachricht mit der Sichtbarkeit, die das Postfach verlangt |

### A.2 Zurückhalten erfasster Nachrichten

| Feld | Angabe |
| --- | --- |
| Zweck | Vertraulichkeit privater und beraterlicher Korrespondenz |
| Rechtsgrundlage | Art. 5 Abs. 1 lit. c DSGVO (Datenminimierung); § 26 Abs. 1 BDSG |
| Besonderheit | **Fällt die Klassifikation aus, bleibt alles zurückgehalten.** Nicht verfügbar oder ohne Budget bedeutet zurückgehalten, nie freigegeben |
| Durchsetzung | `activities/audiencerecompute.go` leitet die Sichtbarkeit als das Strengste ab, was ein erfassendes Postfach verlangt; `platform/auth` prüft sie bei jedem Lesen |

### A.3 Automatisierte Klassifikation von Absendern und Threads

| Feld | Angabe |
| --- | --- |
| Zweck | Entscheidung, ob ein Absender ein Geschäftskontakt und ein Thread gewöhnliche Geschäftskorrespondenz ist |
| Rechtsgrundlage | Art. 6 Abs. 1 lit. f DSGVO |
| Modellbetrieb | lokal, siehe `docs/reference/ai-egress.md` |
| Menschliches Eingreifen | Die Seite „Absender“ zeigt jede Entscheidung; eine Korrektur ist endgültig und wird von der Maschine nicht überschrieben |
| Besonderheit | **Nicht verfügbar oder ohne Budget bedeutet zurückgehalten.** Ein Ausfall führt nie zu einer Freigabe |
| Durchsetzung | `compose/captureverdict.go`, `compose/confidentialityverdict.go`; die Vorrangregel in `capture/senderoverride.go` |

### A.4 Vernichtung auf Verlangen der beschäftigten Person

| Feld | Angabe |
| --- | --- |
| Zweck | Entfernen privater Korrespondenz aus dem CRM |
| Rechtsgrundlage | Art. 17 DSGVO; Art. 6 Abs. 1 lit. c für die Ausnahmen |
| Umfang | Text, Originalnachricht, Anhänge samt Dateien, Vektoren, Zustellkopien |
| Ausnahme | Handelsbriefe innerhalb der gesetzlichen Frist werden **nicht** vernichtet und als übersprungen ausgewiesen |
| Durchsetzung | `privacy/purge.go` über dieselben Executoren wie die Aufbewahrungssteuerung; die Frist als `correspondenceFloorPredicate`, einmal geschrieben und von einem Gate gehalten |

## Teil B — Datenschutz-Folgenabschätzung nach Art. 35 DSGVO

### B.1 Warum eine DSFA erforderlich ist

Systematische Verarbeitung von Beschäftigtendaten in großem Umfang, teils
automatisiert bewertet, in einem Verhältnis struktureller Unterlegenheit.

### B.2 Risiken und Abhilfen

| Risiko | Abhilfe | Durchsetzung |
| --- | --- | --- |
| Kolleginnen lesen private Korrespondenz | Voreinstellung `classified`; nur gewöhnliche Threads werden freigegeben | `capture/birthdecision.go` |
| Eine Administratorin liest zurückgehaltene Inhalte | Die Zugriffsprüfung kennt keine Administratorausnahme | `platform/auth/inheritedscope.go` |
| Der Klassifikator irrt und gibt Vertrauliches frei | Genau eine Antwort öffnet; alle übrigen halten. Öffnende Antworten unter der Schwelle gelten als unsicher und halten | `compose/confidentialityverdictkinds.go` |
| Ein Ausfall führt zur Freigabe | Kein Modell, kein Budget, keine Antwort — alles bleibt zurückgehalten | dieselbe Datei; kein `default`-Zweig |
| Inhalte verlassen die Infrastruktur | Beide Vertraulichkeitsaufgaben laufen lokal | `docs/reference/ai-egress.md`, erzeugt |
| Leistungskontrolle über Metadaten | Verwertungsverbot in der Betriebsvereinbarung | vertraglich, nicht technisch |
| Ein Kollege veröffentlicht, was ein anderer zurückhält | Die Sichtbarkeit ist das Strengste über alle erfassenden Postfächer | `activities/audiencerecompute.go` |

### B.3 Verbleibendes Risiko

**Ein neuer Absender ist zurückgehalten, nicht abwesend.** Seine erste Nachricht
wird gespeichert, bis eine Entscheidung fällt.

**Der Klassifikator irrt.** Die asymmetrische Schwelle verschiebt Fehler in
Richtung Zurückhalten, beseitigt sie aber nicht.

**Metadaten bleiben aussagekräftig.** Wer mit wem wann korrespondiert, ist auch
ohne Inhalt eine Aussage. Dagegen hilft nur Ziffer 4 der Betriebsvereinbarung.

**Das Produkt prüft nichts hiervon.** Es verbindet ein Postfach unabhängig davon,
ob dieses Verzeichnis existiert.
