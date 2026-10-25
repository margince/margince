// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { FALLBACK_RECORD_ZONE } from "./timezone";

// A screen that names a zone has decided something, and the decision is the
// part that has to be reviewable: whose calendar does this date belong to, the
// company's or the reader's? Spelled at the call site, that question gets
// answered once per screen by whoever is passing through, and the two answers
// drift apart — `timezone.ts` exists because the same product carried a fixed
// `Europe/Berlin` on a credential expiry (a personal deadline, wrong for every
// reader outside that zone) and a hand-inlined browser lookup on a record date
// (right for nobody, since two colleagues then quote different days for the
// same fact).
//
// So the rule is that a zone is NAMED in one file. `useRecordZone()` and
// `viewerZone()` carry the purposes; a call site picks one and reads as the
// judgement it is. This gate holds the negative half: nothing outside that
// module spells ANY zone out again — not the record zone, and not a second one
// chosen because it looked harmless.
//
// Since the record zone became the INSTALLATION's answer rather than a
// constant, there is a second way to pin it silently, and this gate holds that
// half too: importing `FALLBACK_RECORD_ZONE`. It is a real zone name that a
// screen could use and that would look right on the machine of whoever wrote
// it, while every installation that configured something else got Berlin — the
// exact defect the hook exists to end. Only the record-zone module itself may
// name it.
//
// Every half is derived from the tree — each `.ts`/`.tsx` under `src/`, read
// through the parser, matched against the IANA zone shape and the two spellings
// of the browser lookup — so a screen added tomorrow is gated the day it lands,
// and so is a zone nobody has typed here yet. The one hand-maintained list is
// `pinnedZones` below, and every entry states why that file is allowed to name a
// zone. It cannot rot unnoticed: an entry that has stopped covering a real site
// fails.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");

// The one home for the rule. It names both zones because that is its job.
const zoneModule = join(here, "timezone.ts");

// The IANA areas every zone name starts with. Anchoring on them is what keeps
// `"screens/settings"` and `"design-system/composed"` out of the findings while
// `"Asia/Ho_Chi_Minh"` stays in: a bare `Word/Word` shape would report half the
// module paths in the tree and be turned off within a week.
const IANA_AREAS =
  "Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Etc|Europe|Indian|Pacific";

// A string literal that is ENTIRELY a zone name — the shape a call site passes
// to a formatter. ANY zone, not the record zone alone: `formatDate(x, locale,
// "UTC")` is the same judgement made silently, and so is a hand-picked
// `"America/Los_Angeles"`, so a gate that knew one name would wave the rest
// through. An i18n message that mentions a zone to explain the setting to a
// reader is prose inside a longer sentence, not a call site, and does not match.
const ZONE_LITERAL = new RegExp(
  `(['"\`])(?:UTC|(?:${IANA_AREAS})/[A-Za-z0-9_+-]+(?:/[A-Za-z0-9_+-]+)?)\\1`,
  "g",
);

// The viewer's own zone, read by hand. `viewerZone()` is the same lookup plus
// the UTC fallback for a runtime that reports no zone.
const INLINE_VIEWER_LOOKUP = /resolvedOptions\(\)\s*\.\s*timeZone/g;

// The lookup itself, whether or not `.timeZone` follows on the same line. This
// is what stops the rule being sidestepped by reading the options object into a
// local first, and it is the shape a test's zone spy is built from — which is
// why a suite that pretends to be elsewhere is an exemption below.
const RESOLVED_OPTIONS = /Intl\.DateTimeFormat\(\)\s*\.\s*resolvedOptions\(\)/g;

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" || entry.name === "dist"
        ? []
        : sourceFiles(path);
    }
    if (path === fileURLToPath(import.meta.url) || path === zoneModule) {
      // This gate spells out the very patterns it hunts for, and the module IS
      // the permitted site. A sweep that read either would vouch for itself.
      return [];
    }
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

// A zone named in a COMMENT is not a call site. The invariant a screen states
// in prose — "pinned to Europe/Berlin it told a reader in another zone a
// different day" — is exactly what this rule wants written down, so a gate
// that punished it would delete its own explanation.
//
// Which spans ARE comments comes from the TypeScript parser rather than from a
// pattern over the text, because a comment opener is not something a pattern can
// settle. `/*` occurs inside ordinary string literals (`"/dist/mcp-apps/*.html"`)
// and inside line comments that quote a path (`// … /v1/public/*`); `//` occurs
// inside every URL; and a scan that pairs any of those with the next terminator
// blanks every line between the two, leaving the gate reading none of the real
// code it swallowed. Blindness in a gate is worse than a false finding: a
// finding gets looked at, and a blind spot reports success.
//
// Blanked rather than deleted, keeping the newlines: the findings below carry
// line numbers, and a comment that collapsed to one line would report every hit
// under it at the wrong place — a gate that names the wrong line sends the next
// reader to innocent code.
function code(path: string, source: string): string {
  const parsed = parseSource(path, source);
  const chars = source.split("");
  const blank = ({ pos, end }: ts.CommentRange): void => {
    for (let index = pos; index < end; index++) {
      if (chars[index] !== "\n") {
        chars[index] = " ";
      }
    }
  };
  // Every comment is trivia of exactly one token, so walking to the tokens
  // reaches all of them, including any at the end of the file — those belong to
  // the end-of-file token. Leading AND trailing: the parser calls a comment that
  // shares a line with the code before it trailing, and reports it under that
  // name only.
  const strip = (node: ts.Node): void => {
    ts.getLeadingCommentRanges(source, node.pos)?.forEach(blank);
    ts.getTrailingCommentRanges(source, node.pos)?.forEach(blank);
    for (const child of node.getChildren(parsed)) {
      strip(child);
    }
  };
  strip(parsed);
  return chars.join("");
}

// Files that legitimately name a zone, each with the reason it does. A test or
// a fixture that PINS a zone is proving something about a named zone, which is
// the one thing `viewerZone()` cannot stand in for: a suite whose expected
// output moved with the machine it ran on would assert nothing.
const pinnedZones: { file: string; why: string }[] = [
  {
    file: "features/analytics/analytics.reports.stories.tsx",
    why: "The frame line names the installation zone as Intl's long name plus offset, and only a fixed zone makes that expected text independent of the machine.",
  },
  {
    file: "features/analytics/reporting.filters.test.tsx",
    why: "The custom period marks today in the workspace zone; only a zone behind every viewer's own makes that day differ from the machine's.",
  },
  {
    file: "features/companies/company360-strip.test.tsx",
    why: "The last-contact day count follows the zone its date is drawn in; the case pins UTC+7 where a UTC count is a day off.",
  },
  {
    file: "format/calendar-days-zone.test.ts",
    why: "The gate's seeded snippets and its message name the literal zone it forbids a day count to pass.",
  },
  {
    file: "format/format.calendar-days.test.ts",
    why: "A day count is a fact about a named zone: the same two instants are different numbers of calendar days in UTC+7, UTC-5 and UTC.",
  },
  {
    file: "lib/activitybody.test.ts",
    why: "A backdated entry's wire date must be the picked day in zones a day ahead of UTC at the moment of logging, so each case names the zone it is logged in.",
  },
  {
    file: "features/worklist/worklist.held.test.ts",
    why: "A date-only meeting reads on the record's clock and a timed one on the reader's, and the two only differ between named zones far apart.",
  },
  {
    file: "features/leads/leadstanding.test.ts",
    why: "A date-only signal reads on the record's clock and the other dates on the reader's, and the two only differ between named zones far apart.",
  },
  {
    file: "features/leads/leadstandingparts.test.ts",
    why: "A date-only signal dates its rung on the record's clock, which differs from the reader's only between named zones far apart.",
  },
  {
    file: "features/automations/approvalpreview.conflicts.test.ts",
    why: "A date-only source's midday falls on the day before west of the record zone, so the case names a zone east of UTC.",
  },
  {
    file: "format/activitywhen.test.ts",
    why: "A date-only entry's noon UTC lands on another local date in the zones furthest from UTC, so the case names those zones.",
  },
  {
    file: "features/companies/companynextbar.test.ts",
    why: "The days-late count takes the zone its date is drawn in; the fixture pins UTC so the expected count does not move with the test machine.",
  },
  {
    file: "features/deals/deal360/dealstanding.test.ts",
    why: "Days in a stage are counted in the record zone; the fixture pins UTC so the expected count does not move with the test machine.",
  },
  {
    file: "features/capture/connectors.historybaseline.stories.tsx",
    why: "A mailbox history baseline stores the zone its owner picked the date in; the fixture carries that stored zone.",
  },
  {
    file: "features/capture/connectors.historybaseline.test.tsx",
    why: "A mailbox history baseline stores the zone its owner picked the date in; the fixture carries that stored zone.",
  },
  {
    file: "features/automations/approvalpreview.test.ts",
    why: "The preview must use the card's supplied date formatter and zone in every locale, independent of the test machine's zone.",
  },
  {
    file: "testing/reporting.fixtures.ts",
    why: "The report API fixture carries the installation timezone so schedule and frozen-period rendering remain independent of the browser zone.",
  },
  {
    file: "features/analytics/reporting.schedule.tsx",
    why: "Weekday names are generated from fixed Monday-through-Sunday UTC instants; these are locale labels, not event or record dates.",
  },
  {
    file: "features/automations/connect-ai-tool.test.tsx",
    why: "The admin guide dates a calendar day; a viewer at UTC+14 is where a day handled as an instant would print the next one.",
  },
  {
    file: "testing/book.testkit.ts",
    why: "The booking API fixture supplies the host’s effective working-hours timezone independently of the viewer’s zone, so the form can show the actual calendar policy.",
  },
  {
    file: "features/companies/companyhealthreason.test.tsx",
    why: "Asserts that a booked meeting's day is named in the record's zone; the day boundary is only checkable against zones the test chose.",
  },
  {
    file: "features/booking/booking-guest-month.test.ts",
    why: "A month is read in the guest's zone, and only a zone with a fall-back clock change makes it longer than the server's 31-day bound.",
  },
  {
    file: "features/booking/booking-picker.test.ts",
    why: "The week grid's empty working days depend on the host's zone against the reader's, so the test pins both to make them disagree.",
  },
  {
    file: "features/companies/companies.thread.test.tsx",
    why: "The account page's email drawer must date a message in the record zone, so the test pins one fourteen hours off UTC that a drawer dated in the viewer's zone cannot match.",
  },
  {
    file: "features/contacts/contactmeetings.test.tsx",
    why: "The next meeting's end comes from its calendar invitation, and the test names the record zone it renders that span in, so the expected span is stated rather than inherited.",
  },
  {
    file: "features/worklist/worklist.meetingoutcome.test.tsx",
    why: "A meeting moved to another day keeps its time of day in the record zone, so the test names two zones whose noon is not the meeting's start, east and west of UTC.",
  },
  {
    file: "features/booking/book.test.tsx",
    why: "The guest deliberately selects Bangkok and the test verifies that its slot labels follow that chosen zone rather than the browser default.",
  },
  {
    file: "features/ai/ai-admin.tsx",
    why: "The allowance period resets on the server’s UTC month boundary; the card explicitly labels this operational clock UTC rather than presenting it as a record or viewer date.",
  },
  {
    file: "features/worklist/worklist.reader.test.ts",
    why: "Pins different viewer and record zones to prove a delayed notification dates the original change in the viewer’s zone.",
  },
  {
    file: "features/sign-in/installation-settings.regional.stories.tsx",
    why: "The fixture supplies the contract-required installation timezone alongside regional settings.",
  },
  {
    file: "format/preferences.test.tsx",
    why: "Regional notation must preserve explicit timezone and midnight fixtures.",
  },

  {
    file: "features/worklist/worklist.leadfacts.test.ts",
    why: "Proves a date-only provisional close keeps its day east and west of UTC.",
  },
  {
    file: "features/worklist/taskduedate.test.tsx",
    why: "The picker's day and the instant it sends are asserted across a zone boundary, so the record zone has to be a NAMED one the fixture also computes its expectation from: the case is that a deadline reads as the day it was agreed on for a colleague elsewhere, and a zone taken off the runner would make the assertion true wherever the suite happened to run. It is provided through RecordZoneProvider, the seam the product itself reads.",
  },
  {
    file: "features/worklist/taskduedate.guard.test.ts",
    why: "The guard's subject is dueInstant, which now TAKES the zone a deadline is resolved in — so a call to it cannot be written without naming one. UTC would prove nothing here: the case that matters is a day's last second landing before midnight on the very clock that minted it, and a zero offset makes the wire value and the wall clock identical whichever rule ran.",
  },
  {
    file: "features/booking/meeting-availability.hours.test.tsx",
    why: "The stub answers /me/working-hours with the SERVER's answer, and a contact who has chosen no zone is answered with the installation's — so the fixture has to name one, exactly as settings.testkit.tsx below does. The card renders the name it is given and no case asserts a rendered instant; reading the runner's zone would make the fixture describe whichever machine ran it.",
  },
  {
    file: "testing/settings.testkit.tsx",
    why: "The stub answers /me/working-hours with the SERVER's answer, and a contact who has chosen no zone is answered with the installation's — so the fixture has to name one, the way the analytics frames above do. Reading the runner's zone would make the fixture describe whichever machine ran it, and the card under it renders the name it is given.",
  },
  {
    file: "features/worklist/worklist.when.test.tsx",
    why: "The rule under test is which SIDE of the reader's own day a moment falls on — today's meeting shows a bare time, another day's shows the date too. Deciding that needs a zone whose offset is not zero: in UTC the fixture's instants land on the same calendar day under either rule, so every case would pass whichever branch ran. The zone is injected by mocking viewerZone, which is the module this gate points callers at; naming it is what makes the expectation ('14:30', not '12:30') checkable at all.",
  },
  {
    file: "features/brief/brief.readings.test.tsx",
    why: "The case is that a date-only period reads as the SAME days for every colleague, and proving it needs three named zones at once: the installation's, which the tile must follow, and a viewer's on either side of UTC. A zone taken off the runner would decide whether the bug can appear at all — east of UTC a viewer-zone read prints the right days by luck, so the assertion would pass over code that is wrong for half the world. The installation's zone is supplied through RecordZoneProvider, the seam the product itself reads.",
  },
  {
    file: "features/automations/approvalevidence.test.tsx",
    why: "The source date must use the configured record zone. The test supplies a named zone through RecordZoneProvider and checks the date against it, independent of the browser zone.",
  },
  {
    file: "features/brief/brief.weekly.test.tsx",
    why: "The case asserts that the weekly's 'written at' renders in the INSTALLATION's zone, so it provides one through RecordZoneProvider and formats its expectation against the same name. The alternative is naming FALLBACK_RECORD_ZONE, which the arm below forbids and rightly: the component reads that same constant, so the assertion would hold however wrong the zone decision was.",
  },
  {
    file: "features/brief/brief.weekly.report.test.tsx",
    why: "The report's title, calendar week and next-report date are days in the installation's zone, so the case provides one through RecordZoneProvider. Without it the expected dates would follow whichever runner's zone the case ran in.",
  },
  {
    file: "features/analytics/analytics.forecast.test.tsx",
    why: "The stubbed readings carry the installation zone the SERVER sends — the frame is the server's answer, not the reader's setting. A zone read off the runner would make the fixture describe whichever machine ran it.",
  },
  {
    file: "features/analytics/analytics.stories.tsx",
    why: "The stub answers a report with the frame a real result carries, and the frame's whole point is that the zone comes from the SERVER rather than the reader. A zone read off the runner would draw a different as-of caption on every machine the catalog builds on.",
  },
  {
    file: "features/analytics/analytics.testkit.tsx",
    why: "Same: the stubbed report result carries the installation zone the server sends, and the caption assertion is about that zone reaching the screen unchanged. Reading the runner's zone would make the assertion about the machine.",
  },
  {
    file: "features/records/recordconversations.test.tsx",
    why: "The component takes the zone as a required prop and these cases assert which GROUPS render and which badges they carry — no date rendering is asserted. The zone is the shape being satisfied, not a rendering under test.",
  },
  {
    file: "design-system/composed.thread.test.tsx",
    why: "The gutter case asserts that a row's time is drawn in the RECORD's zone rather than the instant's — '14:22' for a 12:22Z instant — which needs a zone whose offset is not zero, and the component takes it as a required prop. The other cases pass UTC as the shape being satisfied and assert who wrote what, not a rendering of the clock.",
  },
  {
    file: "features/records/historyreversalrow.stories.tsx",
    why: "The story hand-renders a member row and passes the formatter's required zone directly — a story has no installation read to take it from, and a zone read off the runner would draw a different date column on every machine the catalog builds on.",
  },
  {
    file: "components/pageaside.stories.tsx",
    why: "The story hand-renders a RecordView around the details pane and passes the view's required zone; nothing in it renders a date, so the zone is the shape being satisfied, not a rendering under review.",
  },
  {
    file: "features/records/recordconversations.stories.tsx",
    why: "The story passes the component's required zone prop directly — a story has no installation read to take it from, and a zone read off the runner would draw a different date column on every machine the catalog builds on.",
  },
  {
    file: "features/records/historyfielddiff.test.tsx",
    why: "The component under test takes a reading context whose zone is required, and these cases assert the CURRENCY scaling — no value here renders a date. The zone is the shape being satisfied, not a rendering being asserted.",
  },
  {
    file: "features/records/history.timeline.test.tsx",
    why: "The adapter under test takes a reading context whose zone is required, and none of these rows carries a timestamp for it to render — the zone is the SHAPE being satisfied, not a rendering being asserted, so it is pinned rather than read from a provider these unit calls do not have.",
  },
  {
    file: "features/records/recordchronology.test.tsx",
    why: "Same shape, one level up: the hook takes the reading context whole, and these cases assert which ROWS survive a narrowed read rather than how any date renders. A zone read off the runner would make the fixture differ by machine while proving nothing about the zone.",
  },
  {
    file: "lib/historyvalues.test.ts",
    why: "The zone IS the input under test: a stored timestamp is asserted to render at the RECORD's zone rather than at UTC, which needs a zone east of UTC to stand in. Read off the runner's own zone the suite would assert a different rendering on every machine.",
  },
  {
    file: "features/ai/aiusage.test.tsx",
    why: "The reader's zone IS the input under test: the card is asserted at an instant where the reader's calendar and UTC's name different months, which needs one zone east of UTC to stand in and UTC itself to return to afterwards. A suite that read the runner's own zone would be asking a different question on every machine.",
  },
  {
    file: "App.stories.tsx",
    why: "The whole-app smoke story serves an installation-settings response, and the contract makes `timezone` required — a settled answer without one is a server this bundle does not match, which the app reports as an error and the render gate then fails on. The zone is the SERVER's answer being stubbed, not a zone this story picks.",
  },
  {
    file: "lib/recordzone.test.tsx",
    why: "Proves what the record zone does with what the server sends it, which needs a configured zone to hand it and an unrenderable one to refuse — neither is a zone this code picks, both are the input under test.",
  },
  {
    file: "design-system/recordtimeline.test.tsx",
    why: "The timeline's from/to filter cuts a picked day at the INSTALLATION's boundaries; asserting that against the fallback would agree with itself whichever zone the code read, so the suite names a zone that is not the fallback.",
  },
  {
    file: "components/recordlist.test.tsx",
    why: "createdColumn now takes the zone it renders dates in; this suite asserts what the column SORTS on, so it hands over a named zone to satisfy the signature and the zone itself carries none of the claim.",
  },
  {
    file: "testing/mefixture.ts",
    why: "The offline `me` fixture stands in for a real installation's stored settings, and its company timezone is one of those settings — a value on the wire, not a zone this code picks.",
  },
  {
    file: "design-system/pipelineboard.stories.tsx",
    why: "PipelineBoard takes the record's zone as a prop for its cards' close dates; the story has to hand it a named one, and a zone read off the runner would draw a different date on every machine the catalog builds on.",
  },
  {
    file: "design-system/recordview.stories.tsx",
    why: "RecordView hands its zone to TimelineRow as a prop; the story has to hand it a named one to show what the row renders.",
  },
  {
    file: "design-system/timelinelist.stories.tsx",
    why: "TimelineList and GroupedTimelineList take their zone as a prop; the story has to hand them a named one to show the day and time each row renders.",
  },
  {
    file: "design-system/composed.test.tsx",
    why: "Same prop, asserted: the row's rendered time is only checkable against a zone the test chose.",
  },
  {
    file: "design-system/composed.timelinetext.test.tsx",
    why: "RecordView requires its zone prop; these suites assert a body's links, and a fixed zone keeps the rows they render identical on every runner.",
  },
  {
    file: "design-system/dealcard.stories.tsx",
    why: "DealCard takes the record's zone as a required prop for its close date; the story has to hand it a named one, and a zone read off the runner would draw a different date on every machine the catalog builds on.",
  },
  {
    file: "features/deals/dealpipelineboard.stories.tsx",
    why: "DealPipelineBoard passes the record's zone through to PipelineBoard for its cards' close dates; the story has to hand it a named one, and a zone read off the runner would draw a different date on every machine the catalog builds on.",
  },
  {
    file: "features/deals/dealpipelineboard.test.tsx",
    why: "Same prop, satisfied rather than asserted: the suite is about the cards' verbs, and the zone only fills the board's required signature.",
  },
  {
    file: "design-system/select.stories.tsx",
    why: "The zone picker's option list — IANA names as DATA the control lists, not a zone anything is formatted in.",
  },

  {
    file: "format/format.dates.ts",
    why: "monthName reads a UTC-minted date back in UTC to name a month. The zone is not a rendering choice: the instant is midnight on the 1st, so any reader's clock behind UTC lands on the previous month, and this returned December for January in America/New_York.",
  },
  {
    file: "features/sign-in/fiscalyear.test.ts",
    why: "Proves that trap is real before proving monthName avoids it — reading the same UTC instant on a named western clock, which is the only way to show it lands in the previous month.",
  },
  {
    file: "format/calendarday.test.ts",
    why: "middayInstant and calendarDay take a zone and are proven by naming two of them; a machine-dependent zone would make the expectations unwritable.",
  },
  {
    file: "format/daysuntil.test.ts",
    why: "The zone IS the input under test: the whole case is that a record's date and the count beside it name the same day, which only shows up where the reader's calendar and UTC's disagree — so it needs one named zone east of UTC, one west, and UTC itself to count against. A zone read off the runner would decide whether the defect can appear at all, and the suite would pass over code that is wrong for half the world.",
  },
  {
    file: "format/format.test.ts",
    why: "formatDate's whole contract is that it attaches the zone it is given, which needs a named zone on both sides of the assertion.",
  },
  {
    file: "format/one-locale.test.ts",
    why: "The locale gate's sibling rule. It carries the browser-zone lookup as a fixture line, to prove it does NOT report the one shape that constructs a formatter to read a zone rather than to render — the carve-out that keeps the two gates from claiming one rule each.",
  },
  {
    file: "format/timezone.test.ts",
    why: "Pins viewerZone()'s answer and its UTC fallback by pretending to be in a named zone.",
  },
  {
    file: "features/voice-knowledge/adddocument.test.tsx",
    why: "Installation-settings fixture: the company timezone the document form reads back.",
  },
  {
    file: "features/settings/audit.test.tsx",
    why: "Pins an audit line to the company's clock by reading it from a zone whose calendar day is already the next one — a claim that needs both the zone pretended in and the day compared against.",
  },
  {
    file: "features/companies/company-context.test.tsx",
    why: "The signed-in `me` fixture carries the reader's own `timezone` — a settings value arriving on the wire, not a zone this screen picks.",
  },
  {
    file: "features/companies/companytasks.test.tsx",
    why: "Proves a task's deadline reads the same day on the row and in the detail modal, which only says anything from a zone where the record's clock has already turned over.",
  },
  {
    file: "features/automations/connected-agents.test.tsx",
    why: "Proves the record/viewer split on one row, which only works by naming both zones and a boundary-straddling instant.",
  },
  {
    file: "features/projects/contractform.currency.test.tsx",
    why: "Installation-settings fixture: the company timezone the contract form reads back.",
  },
  {
    file: "features/projects/contractform.customseed.test.tsx",
    why: "The same installation-settings fixture, behind the contract form's custom-field seeding cases.",
  },
  {
    file: "features/deals/dealbulk.stories.tsx",
    why: "Same `me` fixture behind the bulk-edit story: the reader's stored `timezone` is wire data the story has to supply.",
  },
  {
    file: "features/deals/deals-company.test.tsx",
    why: "Same `me` fixture behind the company-scoped board: a stored user setting, not a formatting decision.",
  },
  {
    file: "features/deals/deals-views.test.tsx",
    why: "Same `me` fixture behind the saved-views suite: a stored user setting, not a formatting decision.",
  },
  {
    file: "features/deals/deals.testkit.tsx",
    why: "Same `me` fixture across the deal suites it serves: a stored user setting, not a formatting decision.",
  },
  {
    file: "features/sign-in/installation-settings.stories.tsx",
    why: "The Admin settings card DISPLAYS the configured company timezone; the story needs a configured value.",
  },
  {
    file: "features/sign-in/installation-settings.test.tsx",
    why: "Asserts that same card shows the configured zone verbatim — the one screen where the zone name is the content.",
  },
  {
    file: "features/onboarding/onboarding-conversation/basis-act.stories.tsx",
    why: "The basis act prefills the installation's reporting zone from the same settings record; the story needs a configured value.",
  },
  {
    file: "features/onboarding/onboarding-conversation/basis-act.test.tsx",
    why: "Asserts the reporting zone is prefilled verbatim and left out of the patch when unchanged — the zone name is the content.",
  },
  {
    file: "features/capture/integrations-provider.test.tsx",
    why: "The signed-in `me` fixture behind the provider screen carries the reader's stored `timezone` as wire data.",
  },
  {
    file: "features/leads/leads.test.tsx",
    why: "Installation-settings fixture backing the leads screen's settings read.",
  },
  {
    file: "components/logactivity.test.tsx",
    why: "Reads back the calendar day an entry files under — backdated or logged at the moment — which needs both a named zone to pretend to be in and a named zone to compare the filing against.",
  },
  {
    file: "features/contacts/contactfiles.test.tsx",
    why: "Installation-settings fixture backing the contact-files read.",
  },
  {
    file: "lib/privacy.logic.test.ts",
    why: "endOfDayInZone is a zone-taking function and is proven by naming one.",
  },
  {
    file: "features/privacy/privacy.test.tsx",
    why: "Pretends to be in a zone behind UTC to prove a DSR deadline mints and reads back on the same calendar day; the claim is unwritable without naming that zone.",
  },
  {
    file: "features/privacy/privacy.dsr.test.tsx",
    why: "Pretends to be in a zone behind UTC to prove a DSR opened there gets its deadline on the day the reader chose.",
  },
  {
    file: "features/privacy/privacy.deadline.test.tsx",
    why: "Pretends to be in a zone behind UTC to prove an earlier deadline mints at the end of the chosen day there and never passes an hour-exact deadline.",
  },
  {
    file: "features/privacy/privacy.assignee.test.tsx",
    why: "Its `User` fixtures carry the member's own stored `timezone`, which is a field on the wire rather than a zone this code renders in.",
  },
  {
    file: "features/mail/scheduledsends.stories.tsx",
    why: "A scheduled send carries the zone the human picked it in; the story needs one that is NOT the host's, so it picks whichever of two named zones differs.",
  },
  {
    file: "features/mail/scheduledsends.test.tsx",
    why: "Same pair, asserted: the row must state the picked zone when it differs from the reader's.",
  },
  {
    file: "features/mail/scheduledtimeline.test.ts",
    why: "Day boundaries are asserted on a named zone that sits ahead of UTC, so a message late in UTC is already the next day on the reader's wall clock.",
  },
  {
    file: "features/settings/users-admin.stories.tsx",
    why: "Installation-settings fixtures backing the users-admin story rows.",
  },
  {
    file: "design-system/standingtimeline.test.tsx",
    why: "A day entry dated today is stored at noon UTC, which is later than a late evening in UTC+14; only that zone shows the line still draws it.",
  },
  {
    file: "features/contacts/contactmemory.test.tsx",
    why: "A day entry's noon UTC is the next date in UTC+14; only that zone shows the card prints the date the entry names.",
  },
  {
    file: "features/companies/company360lasttouch.stories.tsx",
    why: "A day entry's noon UTC is the next date in UTC+14; only that zone shows the tile counts from and names the date the entry holds.",
  },
  {
    file: "features/contacts/contactmeetings.held.stories.tsx",
    why: "A day entry's noon UTC is the next date in UTC+14; only that zone shows the table dates a held meeting by the date it names.",
  },
];

type Finding = { file: string; line: number; text: string };

function findings(): Finding[] {
  return sourceFiles(srcRoot).flatMap((path) => {
    const rel = relative(srcRoot, path).split("\\").join("/");
    const lines = code(path, readFileSync(path, "utf8")).split("\n");
    return lines.flatMap((line, index) => {
      const hits = [
        ...line.matchAll(ZONE_LITERAL),
        ...line.matchAll(INLINE_VIEWER_LOOKUP),
        ...line.matchAll(RESOLVED_OPTIONS),
      ];
      return hits.map((hit) => ({
        file: rel,
        line: index + 1,
        text: hit[0],
      }));
    });
  });
}

const pinnedFiles = new Set(pinnedZones.map(({ file }) => file));

describe("a zone is named in one module", () => {
  const all = findings();

  it("reads the tree it is meant to sweep", () => {
    // A miswired walk passes every assertion below by inspecting nothing.
    expect(sourceFiles(srcRoot).length).toBeGreaterThan(200);
    expect(all.length).toBeGreaterThan(pinnedZones.length);
  });

  it("sees the record zone it exists to protect", () => {
    // The pattern is derived from the IANA shape, not copied from the value of
    // the fallback — and the two must not drift, because the way that drift
    // ends is silent. Retune the fallback to something the pattern cannot see
    // and every arm below still passes, over a tree the sweep no longer reads.
    expect(`"${FALLBACK_RECORD_ZONE}"`.match(ZONE_LITERAL)).toEqual([
      `"${FALLBACK_RECORD_ZONE}"`,
    ]);
    // And it has to be a zone Intl accepts, or the shape matched a name that
    // formats nothing. This is load-bearing for the fallback specifically: it
    // is what a screen renders against when the installation's own zone cannot
    // be rendered, so a fallback that itself threw would turn a recoverable
    // disagreement between two zone databases into a blank application.
    expect(
      () => new Intl.DateTimeFormat("en", { timeZone: FALLBACK_RECORD_ZONE }),
    ).not.toThrow();
  });

  it("leaves the fallback zone importable by nothing but its own reader", () => {
    // The record zone is the installation's, served by a hook. The fallback is
    // what stands in when that answer cannot be rendered, and it is a REAL zone
    // name — so a screen that imported it would look correct to whoever wrote
    // it and would quietly show Berlin to every installation that configured
    // something else. That is the defect the hook exists to end, reintroduced
    // through the back door, and no zone LITERAL appears in such a file for the
    // sweep above to catch.
    //
    // Three permitted files, all of them about the fallback rather than users
    // of it. `lib/recordzone.tsx` decides when it applies, and its own suite
    // proves that decision. `format/timezone.test.ts` asserts the one property
    // the fallback must keep: the formatters accept it, since it renders when
    // the installation's own zone cannot. A test forbidden
    // from naming the value it asserts on could only check it against itself.
    const allowed = new Set([
      "lib/recordzone.tsx",
      "lib/recordzone.test.tsx",
      "format/timezone.test.ts",
    ]);
    const importers = sourceFiles(srcRoot)
      .filter((path) => /FALLBACK_RECORD_ZONE/.test(readFileSync(path, "utf8")))
      .map((path) => relative(srcRoot, path).split("\\").join("/"))
      .filter((file) => !allowed.has(file));
    expect(importers.sort()).toEqual([]);
  });

  it("sees a zone that is not the record zone", () => {
    // The gate's first spelling knew one name, so `formatDate(x, locale, "UTC")`
    // and a hand-picked `"America/Los_Angeles"` passed it untouched: two zone
    // decisions taken at a call site, neither reviewable. Both are the finding.
    const zones = ['"UTC"', "'America/Los_Angeles'", '"Asia/Ho_Chi_Minh"'];
    for (const zone of zones) {
      expect(zone.match(ZONE_LITERAL)).toEqual([zone]);
    }
    // A module path is the shape's near neighbour and is not a zone. Reporting
    // these is how a gate gets switched off rather than fixed.
    for (const path of [
      '"screens/settings"',
      '"../format/timezone"',
      '"/v1/me"',
    ]) {
      expect(path.match(ZONE_LITERAL)).toBeNull();
    }
  });

  it("reads the code a comment sits beside, and the code beneath one", () => {
    // A comment opener is not a lexical fact a pattern can settle: `/*` sits
    // inside path strings and inside line comments that quote a path, and `//`
    // sits inside every URL. Pair one of those with the next terminator and the
    // real code between them stops being swept — which is a gate reporting
    // success over lines it never read.
    const source = [
      'const glob = "/dist/mcp-apps/*.html";',
      "// the api client carves out /v1/public/* for this",
      'const hidden = "Europe/Berlin";',
      "/** A doc comment naming Europe/Berlin in prose. */",
      'const url = "https://example.test/x"; // trailing Europe/Berlin prose',
      'const shown = "Asia/Ho_Chi_Minh";',
    ].join("\n");
    const swept = code("sample.ts", source)
      .split("\n")
      .flatMap((line, index) =>
        [...line.matchAll(ZONE_LITERAL)].map(
          (hit) => `${index + 1}: ${hit[0]}`,
        ),
      );
    expect(swept).toEqual([`3: "Europe/Berlin"`, `6: "Asia/Ho_Chi_Minh"`]);
  });

  it("leaves no screen naming a zone of its own", () => {
    const loose = all
      .filter(({ file }) => !pinnedFiles.has(file))
      .map(({ file, line, text }) => `${file}:${line}: ${text}`);
    // Zero, not a ratchet. A date on a record surface takes RECORD_ZONE; a
    // moment the reader relates to their own clock takes viewerZone(); a
    // date-only wire value takes RECORD_ZONE, because there is no instant in
    // `2026-08-21` to localize and a zone behind UTC prints the day before.
    // A fixture or a test that must pin a zone earns an entry in
    // `pinnedZones` with the reason it needs one.
    expect(loose.sort()).toEqual([]);
  });

  it("keeps no exemption that has stopped covering a site", () => {
    // An allowlist is only as honest as its narrowest entry. Once the last
    // pinned zone in a file is gone, the entry is a standing permission
    // nobody needs — and the next screen that hard-codes a zone in that file
    // inherits the excuse.
    const covered = new Set(all.map(({ file }) => file));
    const idle = pinnedZones
      .filter(({ file }) => !covered.has(file))
      .map(({ file }) => file);
    expect(idle.sort()).toEqual([]);
  });

  it("states a reason for every exemption", () => {
    const unexplained = pinnedZones
      .filter(({ why }) => why.trim().length < 40)
      .map(({ file }) => file);
    // A reasonless entry is itself the finding: it records that somebody
    // wanted the gate quiet, not that the file has a case.
    expect(unexplained).toEqual([]);
  });
});
