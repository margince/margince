// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A screen that names a zone has decided whose calendar a date belongs to, the
// company's or the reader's. `timezone.ts` names zones in one place;
// `useCompanyZone()` and `browserZone()` carry the purposes, and a call site
// picks one.
//
// This rule holds the negative half: nothing outside that module spells a zone
// again, by literal or by the browser lookup. Importing
// `FALLBACK_COMPANY_ZONE` is a second way to pin one, and only the record-zone
// module may do it.
//
// The one hand list is `pinnedZones`, each entry with its reason. An entry
// that stops covering a real site fails.

import ts from "typescript";
import { parseSource } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import type { CensusFile, CensusRule } from "../tree";

// The IANA areas every zone name starts with. Anchoring on them keeps module
// paths like `"screens/settings"` out of the findings.
const IANA_AREAS =
  "Africa|America|Antarctica|Arctic|Asia|Atlantic|Australia|Etc|Europe|Indian|Pacific";

// A string literal that is entirely a zone name, any zone. Prose in an i18n
// message that mentions a zone is a longer sentence and does not match.
export const ZONE_LITERAL = new RegExp(
  `(['"\`])(?:UTC|(?:${IANA_AREAS})/[A-Za-z0-9_+-]+(?:/[A-Za-z0-9_+-]+)?)\\1`,
  "g",
);

// The viewer's own zone, read by hand instead of through `browserZone()`.
const INLINE_VIEWER_LOOKUP = /resolvedOptions\(\)\s*\.\s*timeZone/g;

// The lookup itself, so reading the options into a local first still counts.
const RESOLVED_OPTIONS = /Intl\.DateTimeFormat\(\)\s*\.\s*resolvedOptions\(\)/g;

// A zone named in a comment is not a call site. The parser says which spans
// are comments: `/*` sits inside path strings and `//` inside every URL.
// Comments are blanked, keeping newlines, so line numbers stay right.
export function code(path: string, source: string): string {
  const parsed = parseSource(path, source);
  const chars = source.split("");
  const blank = ({ pos, end }: ts.CommentRange): void => {
    for (let index = pos; index < end; index++) {
      if (chars[index] !== "\n") {
        chars[index] = " ";
      }
    }
  };
  // Every comment is leading or trailing trivia of one token, the end-of-file
  // token included.
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

// Files that may name a zone, each with its reason. A test or fixture that
// pins a zone proves something about a named zone, which `browserZone()`
// cannot stand in for.
const pinnedZones: { file: string; why: string }[] = [
  {
    file: "app/agentrail.month.test.tsx",
    why: "Proves the rail asks for the reader's month, not UTC's: only a reader zone already past midnight while UTC is not tells the two apart, and UTC is named to show the boundary is real.",
  },
  {
    file: "features/booking/meeting-availability.refusal.test.tsx",
    why: "Feeds the working-hours card the zone a host stores with their hours: the API sends that zone as data, so the fixture carries a real stored value.",
  },
  {
    file: "components/bulktask.test.tsx",
    why: "Proves the bulk writer mints the day's end on the RECORD's clock: only a named record zone that differs from the runner's and the fallback's tells that apart from the viewer's zone.",
  },
  {
    file: "components/followuptaskfields.test.tsx",
    why: "The hour the clocks skip exists only in a named zone with daylight saving; the runner's zone may have none, and then the field has nothing to warn about.",
  },
  {
    file: "mcp-apps/attention/main.test.ts",
    why: "The card prints a promise's day in the installation zone the answer names. Only named zones east and west of UTC show that a late-evening due date stays on its own day.",
  },
  {
    file: "components/logactivity.followup.test.tsx",
    why: "A callback time is a wall-clock time on the record's clock: the posted due_at only differs from UTC, and so only proves the conversion, under a named zone with an offset. A runner zone of UTC would pass a form that ignores the record zone.",
  },
  {
    file: "components/logactivity.openmeeting.test.tsx",
    why: "A date-only meeting is stored at noon UTC. Only a zone well east of UTC puts an instant from the evening before nearer the day's start than that noon, which is the case the closest-meeting pick must get right.",
  },
  {
    file: "features/privacy/noticesource.test.tsx",
    why: "Pins the acquisition calendar to prove that the submitted civil date converts to the intended UTC instant.",
  },
  {
    file: "features/analytics/analytics.overview.trajectory.test.ts",
    why: "Month ticks fall on the company's month starts. Only a zone whose local month starts on another UTC date tells that apart from UTC.",
  },
  {
    file: "features/analytics/reporting.questioncard.test.tsx",
    why: "A bound card shows the calendar its saved question keeps; that stored basis names a zone, and the note must print it as stored.",
  },
  {
    file: "features/analytics/reporting.charts.stories.tsx",
    why: "The story of a sent edition's bound card carries the calendar basis its question was kept in, which names a zone.",
  },
  {
    file: "features/settings/rates.test.tsx",
    why: "The model price form opens on the UTC day its server counts. Only a reader west of UTC, in the evening, makes that day differ from the reader's own.",
  },
  {
    file: "features/deals/deal360/dealwaiting.test.ts",
    why: "Current request age counts calendar days in the record zone; named zones on opposite sides of midnight prove that the count follows that zone.",
  },
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
    file: "format/format.calendar-days.test.ts",
    why: "A day count is a fact about a named zone: the same two instants are different numbers of calendar days in UTC+7, UTC-5 and UTC.",
  },
  {
    file: "lib/activitybody.test.ts",
    why: "A backdated entry's wire date must be the picked day in zones a day ahead of UTC at the moment of logging, so each case names the zone it is logged in.",
  },
  {
    file: "lib/worklistheld.test.ts",
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
    file: "features/brief/brief.taskcard.words.test.ts",
    why: "A task card counts the days a task is late in the zone it is given; the fixture pins UTC so the count does not move with the test machine.",
  },
  {
    file: "features/companies/companynextbar.late.test.tsx",
    why: "A promise 35 hours late is one elapsed day and two midnights only in a named zone; the badge and the sentence must both say two.",
  },
  {
    file: "features/contacts/contacttoday.late.test.tsx",
    why: "A promise 35 hours late is one elapsed day and two midnights only in a named zone; the contact's badge and sentence must both say two.",
  },
  {
    file: "features/deals/deals.columns.test.tsx",
    why: "The close column draws its day in the zone it is given; the fixture pins UTC so the expected day does not move with the test machine.",
  },
  {
    file: "features/deals/deal360/dealstanding.test.ts",
    why: "Days in a stage are counted in the record zone; the fixture pins UTC so the expected count does not move with the test machine.",
  },
  {
    file: "features/deals/dealfiles.test.tsx",
    why: "A file from a mail logged by its day is dated by that day; only a record zone at UTC+14 puts the stored noon on the next local date.",
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
    file: "components/connect-ai-tool.test.tsx",
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
    file: "features/worklist/taskduedate.test.tsx",
    why: "The picker's day and the instant it sends are asserted across a zone boundary, so the record zone has to be a NAMED one the fixture also computes its expectation from: the case is that a deadline reads as the day it was agreed on for a colleague elsewhere, and a zone taken off the runner would make the assertion true wherever the suite happened to run. It is provided through CompanyZoneProvider, the seam the product itself reads.",
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
    file: "testing/readerzone.testkit.ts",
    why: "It is the zone spy a suite uses to pretend the browser is elsewhere; it has to read the real options to replace only the zone.",
  },
  {
    file: "features/worklist/worklist.dueday.test.tsx",
    why: "The row's due day is asserted where the company's clock and the reader's disagree by a calendar day, which only two named zones can set up.",
  },
  {
    file: "features/worklist/worklist.briefverbs.test.tsx",
    why: "Tomorrow at 08:00 is asserted on a named reader zone against a browser pretending to be elsewhere; only named zones make the two disagree.",
  },
  {
    file: "lib/viewerzone.test.tsx",
    why: "The reader's zone is the subject: the suite names a reader zone, a company zone that disagrees with it by a calendar day, and a fixed offset the formatters refuse.",
  },
  {
    file: "features/worklist/taskactions.test.tsx",
    why: "The due line is asserted to the hour, and that hour exists only in a named zone; the suite names the reader's zone so the hour does not move with the machine it runs on.",
  },
  {
    file: "features/settings/account-settings.test.tsx",
    why: "The case picks a zone in the account row and asserts the exact IANA name the write carries; the name the user picked is the subject, so it has to be spelled.",
  },
  {
    file: "testing/settings.testkit.tsx",
    why: "The stub answers /me/working-hours with the SERVER's answer, and a contact who has chosen no zone is answered with the installation's — so the fixture has to name one, the way the analytics frames above do. Reading the runner's zone would make the fixture describe whichever machine ran it, and the card under it renders the name it is given.",
  },
  {
    file: "features/worklist/worklist.when.test.tsx",
    why: "The rule under test is which SIDE of the reader's own day a moment falls on — today's meeting shows a bare time, another day's shows the date too. Deciding that needs a zone whose offset is not zero: in UTC the fixture's instants land on the same calendar day under either rule, so every case would pass whichever branch ran. The zone is injected by mocking browserZone, which is the module this gate points callers at; naming it is what makes the expectation ('14:30', not '12:30') checkable at all.",
  },
  {
    file: "features/brief/brief.queue.test.tsx",
    why: "A snooze keeps the wall time the reader picked, and that time maps to a different instant on each side of a daylight-saving change. Only a named zone with daylight saving makes the expected instant the same on every runner.",
  },
  {
    file: "features/brief/brief.readings.test.tsx",
    why: "The case is that a date-only period reads as the SAME days for every colleague, and proving it needs three named zones at once: the installation's, which the tile must follow, and a viewer's on either side of UTC. A zone taken off the runner would decide whether the bug can appear at all — east of UTC a viewer-zone read prints the right days by luck, so the assertion would pass over code that is wrong for half the world. The installation's zone is supplied through CompanyZoneProvider, the seam the product itself reads.",
  },
  {
    file: "features/automations/approvalevidence.test.tsx",
    why: "The source date must use the configured record zone. The test supplies a named zone through CompanyZoneProvider and checks the date against it, independent of the browser zone.",
  },
  {
    file: "features/brief/brief.weekly.test.tsx",
    why: "The case asserts that the weekly's 'written at' renders in the INSTALLATION's zone, so it provides one through CompanyZoneProvider and formats its expectation against the same name. The alternative is naming FALLBACK_COMPANY_ZONE, which the arm below forbids and rightly: the component reads that same constant, so the assertion would hold however wrong the zone decision was.",
  },
  {
    file: "features/brief/brief.weekly.report.test.tsx",
    why: "The report's title, calendar week and next-report date are days in the installation's zone, so the case provides one through CompanyZoneProvider. Without it the expected dates would follow whichever runner's zone the case ran in.",
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
    file: "lib/companyzone.test.tsx",
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
    file: "format/timezone.test.ts",
    why: "Pins browserZone()'s answer and its UTC fallback by pretending to be in a named zone.",
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
    file: "components/logactivity.testkit.tsx",
    why: "The composer harness names the reader's zone its suites assert against, so a stored instant cut on any other clock fails rather than agreeing with itself on the default.",
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

// The modules allowed to import the fallback: the one that decides when it
// applies, its suite, and the suite proving the formatters accept it.
const FALLBACK_READERS: ReadonlySet<string> = new Set([
  "lib/companyzone.tsx",
  "lib/companyzone.test.tsx",
  "format/timezone.test.ts",
]);

const ZONE_MODULE = "src/format/timezone.ts";

const judged = (file: CensusFile) =>
  file.area === "src" && /\.tsx?$/.test(file.path) && file.rel !== ZONE_MODULE;

const fromSrc = (file: CensusFile) => file.rel.replace(/^src\//, "");

type Finding = { file: string; line: number; text: string };

// Blanking comments only takes matches away, so a file whose raw text names no
// zone needs no parse for its comments.
const namesAZone = (text: string) =>
  [ZONE_LITERAL, INLINE_VIEWER_LOOKUP, RESOLVED_OPTIONS].some(
    (pattern) => text.match(pattern) !== null,
  );

function zonesIn(file: CensusFile): Finding[] {
  if (!namesAZone(file.text)) return [];
  const lines = code(file.path, file.text).split("\n");
  return lines.flatMap((line, index) =>
    [
      ...line.matchAll(ZONE_LITERAL),
      ...line.matchAll(INLINE_VIEWER_LOOKUP),
      ...line.matchAll(RESOLVED_OPTIONS),
    ].map((hit) => ({ file: fromSrc(file), line: index + 1, text: hit[0] })),
  );
}

export const rule: CensusRule = {
  name: "zone-by-purpose",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) =>
    corpus.length > 200 ? [] : ["fewer than 200 source files were read"],
  across: (corpus) => {
    const all = corpus.flatMap(zonesIn);
    const pinned = new Set(pinnedZones.map(({ file }) => file));
    const covered = new Set(all.map(({ file }) => file));
    return [
      ...all
        .filter(({ file }) => !pinned.has(file))
        .map(({ file, line, text }) => `${file}:${line}: ${text}`)
        .sort(stable),
      ...corpus
        .filter(
          (file) =>
            file.text.includes("FALLBACK_COMPANY_ZONE") &&
            !FALLBACK_READERS.has(fromSrc(file)),
        )
        .map((file) => `${fromSrc(file)}: names FALLBACK_COMPANY_ZONE`),
      // An entry whose last pinned zone is gone is a standing permission.
      ...pinnedZones
        .filter(({ file }) => !covered.has(file))
        .map(({ file }) => `${file}: pinned, but names no zone`),
      ...pinnedZones
        .filter(({ why }) => why.trim().length < 40)
        .map(({ file }) => `${file}: pinned without a reason`),
    ];
  },
  advice:
    "A date on a record surface takes the record zone (useCompanyZone); a " +
    "moment the reader relates to their own clock takes browserZone(); a " +
    "date-only wire value takes the record zone. A fixture or test that must " +
    "pin a zone earns an entry in pinnedZones with its reason.",
};
