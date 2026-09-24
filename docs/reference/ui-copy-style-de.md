# UI copy: German

German is translated from the English catalog, `frontend/src/i18n/en.ts`, and
[ui-copy-style.md](ui-copy-style.md) binds it too. This page states only what
German adds or changes: address, grammar, mechanics, inclusive language and one
German word per concept. It is the page the next German author reads, and the
one a Vietnamese translator reads to know which German choices are deliberate.

Part of it is mechanical. `frontend/src/i18n/copy-style-de.test.ts` holds the
mechanics and `frontend/src/i18n/address-register.test.ts` holds the address,
over `de.ts` and every extension `de.json`.
[What the gate holds](#what-the-gate-holds) lists both. Everything else is the
author's judgement, and a reviewer reads a German change against this page.

## Source and claim strength

- **The English value is the source of meaning.** When the English and this
  page disagree about a fact, the English wins; when German grammar and English
  syntax disagree, German wins. Translate what a value means and does, not its
  words.
- **Natural German word order.** Never mirror English syntax. Resolve English
  noun chains, and avoid heavy nominalisation: "Speichern fehlgeschlagen", not
  "Durchführung der Speicherung nicht erfolgreich".
- **Nothing added, nothing dropped.** No fact, feature, promise or limitation
  the English does not carry.
- **Claim strength is kept exactly.** Legal, privacy, consent, retention,
  licence and AI-notice text keeps every qualifier: can and may (kann, darf),
  designed to (ausgelegt auf, dafür gedacht), intended to (vorgesehen), subject
  to (vorbehaltlich, abhängig von), where applicable (soweit anwendbar),
  typically (in der Regel). "Designed to support compliance" never becomes
  "garantiert". Accuracy beats elegance there.
- **Product and brand names stay as they are**: Margince, Gradion, Google
  Workspace, Microsoft 365. So do URLs, code, identifiers and placeholders.

## Address

The product addresses its user as **du**, and only as du: du, dich, dir, dein,
deine, deinem, deinen, deiner, deines. Never Sie, Ihr or Ihnen to the user.

- A du-pronoun is **capitalised only at a sentence start**. "Prüf deine
  Eingaben", never "Prüf Deine Eingaben".
- **Address nobody by default**, as the English does. Labels, titles,
  statuses, table headers, menu items and most help text are impersonal. Use du
  only where the English says "you" or "your", or where an error or
  confirmation tells the reader what to do next.
- **Four families keep Sie**, and they are the only Sie in the catalog:
  `privacynotice.`, `confirm.`, `buyer.` and `prefs.`. Each is read by someone
  outside the installation: a stranger reading the privacy notice, a contact
  answering a consent question, a buyer in a Deal Room, a visitor choosing what
  may still be sent. Outbound mail to a contact is written in Sie, and the
  consent question is published by the server in that wording, so a screen in
  du would ask one question and record another.
- **Never mix** du, Sie and neutral address inside one surface.
- "Mein" and "Meine" only as a **scope label** naming the reader's own records
  ("Meine Deals"), mirroring "My deals". "Mir" only as the **object of a
  picker** ("Mir zuweisen", "Nur mir"), mirroring "Assign to me".

## Never ich, never wir

Margince is software and never speaks as ich or wir. "Wir konnten nicht
speichern" becomes "Änderungen wurden nicht gespeichert". The exemptions mirror
the English page:

- `ob.conv.`: the agent speaking in the onboarding conversation bubble says
  "ich" in short, factual sentences.
- `prefs.wording.`, `prefs.wordingGeneric`, `directSend.acknowledge`,
  `book.consentWording`: a consent statement is the reader's own sentence and
  may say ich and wir.
- `privacynotice.`: the data controller speaks as "wir", as in law. That
  exemption covers wir, uns and unser only, never ich.

A value that means the reader's company ("waiting on us") follows the current
English, rendered neutrally: "Wartet auf Antwort".

## Grammar by slot

| Slot | Shape | Example |
|---|---|---|
| Button, menu item, fragment hint | Infinitive, verb last, no article | Änderungen speichern, Deal anlegen |
| Label, heading, tab, table header | Noun phrase, no article, no period | Abschlussdatum |
| Instruction as a full sentence | du imperative, the short form is fine (prüf, versuch, wähl, gib) | Melde dich erneut an. |
| Statement | Impersonal; passive where the actor is the system | Änderungen wurden nicht gespeichert. |
| Status | Adjective or past participle | Gesendet, Überfällig |
| Success | Noun and past participle, no period | Phase hinzugefügt |
| In progress | Passive or present with one ellipsis character | Wird gespeichert…, Deals werden geladen… |
| Error | What did not happen, then the one action | Änderungen wurden nicht gespeichert. Prüf die Verbindung und versuch es erneut. |
| Warning | What will happen, then what it costs | Beim Entfernen dieser Phase wandern 12 Deals nach „Qualifizierung“. |
| Confirmation | Title asks and names the object; primary button repeats verb and object; secondary is Abbrechen | Phase löschen? / Phase löschen |
| Empty state | One fact, one action | Noch keine Deals. Leg den ersten Deal an. |
| Permission denied | The fact and who can change it | Nur Admins können das Kontingent bearbeiten. |

A confirmation button is never "Ja", "Nein", "OK" or a bare "Bestätigen".
Sentence case follows German orthography: nouns capitalised, everything else
lowercase, never title case.

## Mechanics

- **No long dashes.** No en dash, no em dash, no double hyphen. Rewrite with a
  comma, a period, a colon or parentheses. The hyphen only joins compounds:
  E-Mail, KI-nativ, CRM-Datensatz. A compound with an English word takes a
  hyphen (Deal-Wert, Pipeline-Phase, Follow-up-Entwurf) unless the closed form
  is established.
- **Quotes** are „…“ (U+201E opening, U+201C closing). The apostrophe is ’.
  Never a straight " or ', never » « and never English “…” or ”.
- **Ellipsis** is the one character …, never three dots.
- **No exclamation marks. Never "bitte".** One space between words; an edge
  space only where the English value has the same one.
- **No abbreviations**: z. B., d. h., usw., u. a., ggf., bzgl., bzw., inkl.,
  ca. Write them out (zum Beispiel, das heißt, gegebenenfalls, bezüglich,
  inklusive, etwa) or cut them; "beziehungsweise" is usually "oder".
  Established acronyms stay: CRM, API, KI, DSGVO, PDF, CSV, UTC, IMAP, DNS, USt.
- **No apostrophe contractions**: "gibt es", never "gibt’s".
- **Percent** takes a no-break space (U+00A0) between number and sign: "30 %",
  "{pct} %". In a JSON catalog write the character itself or ` `.
- **Numbers** written as text use a period for thousands and a comma for
  decimals: 1.234,50. Ranges use "bis": "1 bis 4", "von 1 bis
  1.000.000.000.000".
- **Dates and times** come from the formatting layer through a placeholder and
  are never spelled in a string. Relative time: "gerade eben", "vor {n}
  Minuten", "gestern", "in {n} Tagen".
- **Spelling** is standard German as written in Germany and Austria, with ß,
  after the current Duden.
- **Placeholders and plural arms** are frozen. A `{count}` without plural arms
  must read at 0, 1 and many, so it takes the "Label: {count}" shape
  ("Gewonnene Deals: {count}") rather than a noun that inflects.

## Inclusive language

- **Groups** are named with nominalised participle plurals, the same way every
  time: Nutzende, Mitarbeitende, Vertriebsmitarbeitende, Teilnehmende.
  "Admins" is already neutral.
- **A single human** is written around, never with a gendered noun: Nutzerkonto,
  Teammitglied, "wer …".
- **No Gendersternchen, colon, underscore, Binnen-I or slash form**: never
  Nutzer\*innen, Nutzer:innen, NutzerInnen or Nutzer/-innen.
- **Kunde** is acceptable where it means the customer company. For a human
  record the word is Kontakt.

## Anglicisms

Keep the English terms German B2B readers use: Deal, Pipeline, Forecast, Lead,
Follow-up, Dashboard, Workflow, Tag, Commit, Best Case, Legal Hold, Buying
Center, Passport, Connector, Deal Room, Thread, Worklist, API, Webhook, Token
(model tokens only), Embedding. Everything else is German: Einstellungen,
Suche, hochladen, herunterladen, Termin, Aufgabe, Bericht, speichern.

## Length

The English ceilings apply, with a German compound counted as one word: 3 words
for a button, label, menu item, tab or badge; 6 for a title or heading; one
sentence of about 100 characters for a hint; two sentences of about 200
characters for a message. Legal text is exempt. A value over its ceiling says
why; meaning is never truncated to fit.

## Vocabulary

One German word per concept, the same word on every screen. The Never column
lists the words the gate retires, plus some it leaves to the reviewer because
they are ordinary German in another sense (Geschäft, Stufe, Posteingang).

| Concept | German | Never |
|---|---|---|
| The tenant and the company record | Unternehmen | Firma, Firmen, Workspace, Arbeitsbereich, Account, Accounts, Mandant, Konto for the tenant, and the retired company nouns in [record-vocabulary.md](record-vocabulary.md) |
| A human record | Kontakt | Ansprechpartner as the record noun, and the retired record nouns in [record-vocabulary.md](record-vocabulary.md) |
| Sales object | Deal, Deals | Opportunity, Opportunities, Verkaufschance, Verkaufschancen, Geschäft |
| Ordered stages | Pipeline | Trichter, Funnel |
| A pipeline step | Phase (Deal-Phase, Pipeline-Phase) | Stufe |
| Forecast object | Forecast | Prognose, Prognosen (the verb "prognostiziert" is fine) |
| Manager's forecast number | Einschätzung | Call, Commit (the per-deal category) |
| The reader's queue | Worklist | Arbeitsliste, Warteschlange, Posteingang |
| Daily digest | Morgenbericht | Briefing, Digest, Tagesbericht |
| Agent decisions awaiting a human | Freigaben (one Freigabe) | Entscheidungen |
| Approve, approval | freigeben, Freigabe | genehmigen, genehmigt, Genehmigung, Genehmigungen |
| A classifier's result | Ergebnis, Prüfung | Urteil, Urteile, Verdikt |
| Something a party said they would do | Zusage | Versprechen |
| Ownership of a deal | zuständig (label "Zuständig") | Owner, Besitzer, Inhaber |
| Legal hold | Legal Hold | Aufbewahrungssperre |
| Buying committee | Buying Center | Einkaufskomitee, Kaufgremium |
| The running Margince system | Installation | Deployment, Instanz, Bereitstellung |
| Administrator | Admin, Admins ("Administrator" only in licence text) | Administrator, Administratoren, Verwalter |
| Users | Nutzende; for one, Nutzerkonto or a rephrase | Nutzer, Nutzern, Benutzer, Benutzern, Anwender, Anwendern |
| Staff | Mitarbeitende, Vertriebsmitarbeitende | Mitarbeiter, Mitarbeitern, Vertriebsmitarbeiter |
| Agent credential | Passport | Token, Schlüssel (except API-Schlüssel) |
| Mail or calendar link | Connector | Integration |
| Buyer-facing deal page | Deal Room | Deal-Room, Portal |
| The mail thread of a record | Thread | Spine, Konversation |
| Import of mailbox history | Import des Postfachverlaufs | Backread |
| Full read of a web page | Seite vollständig lesen | Deep Read |
| Capture intake step | Eingangsprüfung | Zulassungsprüfung |
| Audit log | Audit-Log | Audit-Trail, Prüfprotokoll |
| Artificial intelligence | KI (KI-generiert, KI-Agent) | AI |
| Sign in | anmelden, abmelden | einloggen, eingeloggt |
| Retry | Erneut versuchen | Nochmal, nochmal |
| Upload, download | hochladen, herunterladen | uploaden, downloaden |
| Email | E-Mail, E-Mails | Email, Emails, eMail |
| Mailbox | Postfach | Mailbox |
| Meeting | Termin | Meeting, Meetings |
| Task | Aufgabe | Task, Tasks |
| Tag | Tag, Tags | Schlagwort |
| Report | Bericht | Report, Reports |
| AI quota | Kontingent | Allowance, Budget |
| Licensed seat | Platz (Nutzerplatz) | Seat, Seats |
| Consent (the legal concept) | Einwilligung | Zustimmung |
| Data protection | Datenschutz, DSGVO | Privatsphäre, GDPR |
| Loading a record | Lädt…, Wird geladen… | Wird gelesen… |

Terms that carry over unchanged, so no second word is needed: Lead, Commit,
Best Case, Follow-up, Entwurf, Notiz, Verlauf, Datensatz, Feldhistorie,
Aufbewahrung, Lizenz, Angebot, Abschlussdatum, Deal-Wert.

## What the gate holds

`frontend/src/i18n/copy-style-de.test.ts` checks every value of `de.ts` and of
every extension `de.json`, one test per rule, and lists each offender as its
source file, key and value. Placeholders are removed before the word rules run.

| Rule | What fails |
|---|---|
| No long dashes | An en dash, an em dash or `--` |
| Straight quotes | Any straight `"`; a straight `'` between two letters |
| German quotes | » « ” or “ anywhere except as the closing mark of a „…“ pair; an unclosed „ |
| Ellipsis | Three dots in a row |
| Spacing | Two spaces in a row |
| No exclamation marks | Any `!` |
| Never "bitte" | The whole word, in any case |
| Abbreviations | z. B., d. h., usw., u. a., ggf., bzgl., bzw., inkl., ca., with or without the inner space |
| No contractions | ’s or 's straight after a letter |
| Percent | A digit or a closing `}` followed by `%`, or by an ordinary space and `%` |
| Inclusive forms | A Gendersternchen, colon, underscore, slash or hyphen before "innen", or a Binnen-I |
| Never ich | ich, Ich, mich, Mich, outside `ob.conv.` and the consent statements |
| Never wir | wir, uns, unser in lower or leading capital, outside `privacynotice.` and the consent statements |
| Retired vocabulary | A word the test lists from the Vocabulary table's Never column, as a whole word, case-sensitive, outside the keys it names with a reason |

A last test fails a retired word the Vocabulary table above does not name, so
the table and the gate cannot drift apart.

`frontend/src/i18n/address-register.test.ts` holds the address: no Sie, Ihr or
Ihnen outside the four outsider families, and no capitalised du-pronoun
mid-sentence. A sentence opening with "Sie" as she or they is rephrased, since
the capital cannot be read off the page.

Neither can see the rest of this page: word order, tone, sentence case,
articles, periods, slot grammar, claim strength, du used where nobody needed
addressing, a neutral value beside a du value on one surface, number format and
ranges, spelled dates, gendered singulars other than the listed nouns, a
retired word inside a closed compound (Firmenkontingent), mein or mir used as
the product's voice, length ceilings and message shapes. Those are the
author's and the reviewer's judgement.
