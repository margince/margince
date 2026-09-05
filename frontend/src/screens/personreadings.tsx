import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { StatCard } from "../design-system/atoms";
import { FactList } from "../design-system/factlist";
import { ReadingsGrid } from "../design-system/readingsgrid";
import {
  calendarDaysBetween,
  formatDayMonth,
  formatMoneyCompact,
  formatNumber,
  relativeDays,
} from "../format/format";
import { daysPast } from "../format/lateness";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { buyingRoleLabel } from "./companypeople/summary";
import { owedPromises, owedPromisesTruncated } from "./personowed";
import { daysSinceInbound, isQuiet } from "./personquiet";
import type { PersonTab } from "./persontab";

// The contact's four readings, in the cards every record page draws them in
// (ReadingsGrid): the same shape the account's readings row takes, so a rep who
// reads both records reads them the same way.
//
// Four, and these four, in the order a reader asks them: whose move it is and
// how long it has been theirs; what we owe them; what they decide; when we next
// see them. Consent is NOT one of them any more — it is a fact about what we
// may send, which the header's Write verb states where it is acted on and the
// rail states beside the channels it governs; a slot of five it read as one
// more score.
//
// Every slot distinguishes three states, and the difference matters: a value,
// a fact that there is none ("None", "No open deal"), and a section the caller
// may not read. Only the last renders as withheld — "no open deal" is an
// answer.

type Person360 = components["schemas"]["Person360"];

// How many days without a reply before the relationship reads as quiet. The
// same span the rail's own overall reading turns on (personrail.tsx), so the
// card and the rail cannot disagree about when silence became a problem.

export function PersonReadings({
  view,
  onOpenTab,
}: Readonly<{
  view: Person360;
  // The tab each reading is a reading OF. Optional, because a surface that
  // draws these outside the record page has no tab strip to send anybody to.
  onOpenTab?: (tab: PersonTab) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const omitted = new Set(view.sections_omitted ?? []);
  return (
    <ReadingsGrid label={t("person.readings.title")} testId="person-readings">
      <MoveCard
        view={view}
        withheld={omitted.has("last_touch")}
        locale={locale}
        t={t}
      />
      <PromisesCard
        view={view}
        withheld={omitted.has("claims")}
        locale={locale}
        t={t}
      />
      <DealCard
        view={view}
        withheld={omitted.has("commercial")}
        locale={locale}
        zone={zone}
        onOpen={onOpenTab && (() => onOpenTab("deals"))}
        t={t}
      />
      <MeetingCard
        view={view}
        withheld={omitted.has("next_meeting")}
        locale={locale}
        zone={zone}
        onOpen={onOpenTab && (() => onOpenTab("meetings"))}
        t={t}
      />
    </ReadingsGrid>
  );
}

type Translate = ReturnType<typeof useT>;

// A reading the caller's grants withheld says so, and carries no tone: there
// is no verdict to colour, and "not shown" painted amber would state a problem
// with the contact out of a permission boundary.
function withheldCard(label: string, t: Translate) {
  return <StatCard label={label} value={t("record.notShown")} />;
}

// Whose move it is, read from the two directions. They are separate facts and
// the card keeps them separate: the value says who wrote last and how long
// ago, the detail says what the other side did.
//
// Quiet is a claim about THEM and only them: an account we wrote to yesterday
// and one we have not heard from in a month can share a last-touch date, and
// only the second is a relationship going cold.
function MoveCard({
  view,
  withheld,
  locale,
  t,
}: Readonly<{
  view: Person360;
  withheld: boolean;
  locale: Locale;
  t: Translate;
}>) {
  const label = t("person.readings.move");
  if (withheld) {
    return withheldCard(label, t);
  }
  const inbound = view.last_inbound_at ?? null;
  const outbound = view.last_outbound_at ?? null;
  // Every "how long ago" is measured from the instant the read describes, so
  // the card agrees with the thread beside it and does not drift while a tab
  // is left open.
  const asOf = new Date(view.as_of);
  const theirs = inbound && (!outbound || inbound > outbound);
  const basis = (
    <FactList
      facts={[
        {
          key: "in",
          term: t("person.strip.lastInbound"),
          value: relativeDays(inbound, t, locale, asOf),
        },
        {
          key: "out",
          term: t("person.strip.lastOutbound"),
          value: relativeDays(outbound, t, locale, asOf),
        },
        ...reciprocityFact(view, t, locale),
      ]}
    />
  );
  const basisProps = {
    basis,
  };
  if (!inbound && !outbound) {
    return (
      <StatCard
        label={label}
        value={t("person.readings.neverSpoke")}
        {...basisProps}
      />
    );
  }
  if (theirs) {
    // They wrote last, so the move is ours. The detail says how long we have
    // let it stand.
    return (
      <StatCard
        label={label}
        value={t("person.readings.yourMove")}
        detail={t("person.readings.lastFromThem", {
          when: relativeDays(inbound, t, locale, asOf),
        })}
        tone="warn"
        dot
        {...basisProps}
      />
    );
  }
  // We wrote last. Quiet once the silence has outlasted the span the rail
  // calls at risk — counted from their last word by the rail's own rule, or
  // from ours when they have never written: a contact first written to
  // yesterday is awaited, not going cold. Otherwise simply their move.
  const silence =
    daysSinceInbound(view) ??
    (outbound ? calendarDaysBetween(new Date(outbound), asOf) : null);
  const quiet = silence !== null && isQuiet(silence);
  return (
    <StatCard
      label={label}
      value={
        quiet ? t("person.readings.quiet") : t("person.readings.theirMove")
      }
      detail={
        inbound
          ? t("person.readings.lastFromThem", {
              when: relativeDays(inbound, t, locale, asOf),
            })
          : t("person.readings.neverReplied")
      }
      tone={quiet ? "warn" : undefined}
      dot={quiet}
      {...basisProps}
    />
  );
}

// How the conversation has run each way, as counts and not a score: a
// standalone number here would be the composite verdict the face
// deliberately does not carry. Counted off the timeline page the read
// carries, so the tally is of what the page holds; absent when the activity
// section was withheld, since a count of nothing would read as silence.
function reciprocityFact(
  view: Person360,
  t: Translate,
  locale: Locale,
): { key: string; term: string; value: string }[] {
  if ((view.sections_omitted ?? []).includes("activities")) {
    return [];
  }
  const rows = view.activities?.data ?? [];
  const inbound = rows.filter((row) => row.direction === "inbound").length;
  const outbound = rows.filter((row) => row.direction === "outbound").length;
  return [
    {
      key: "exchanged",
      term: t("person.strip.reciprocity"),
      value: t("person.strip.inOut", {
        inbound: formatNumber(inbound, locale),
        outbound: formatNumber(outbound, locale),
      }),
    },
  ];
}

// What WE owe them: the open commitments on our side, and how late the oldest
// is. Only ours — theirs are the commitments card's to list, and a count that
// folded the two sides together would ask the reader to chase a promise that
// is not theirs to keep.
function PromisesCard({
  view,
  withheld,
  locale,
  t,
}: Readonly<{
  view: Person360;
  withheld: boolean;
  locale: Locale;
  t: Translate;
}>) {
  // The rest of this card's words arrive as props, the way every card in this
  // file takes them. The plural rule cannot: which form a count takes is the
  // READER's locale's answer, so it is read here rather than threaded through
  // a parent that would have to hand it to one of six siblings.
  const plural = usePlural();
  const label = t("person.readings.promises");
  if (withheld) {
    return withheldCard(label, t);
  }
  const asOf = Date.parse(view.as_of);
  // BOTH places a promise is written down, which is the rule the headline
  // above this card already applies. Counting claims alone put "0 · nothing
  // owed" directly under "You owe them" — the same page contradicting itself
  // about the same person, on a record whose only open promise was a task.
  const ours = owedPromises(view);
  if (ours.length === 0) {
    return (
      <StatCard
        label={label}
        value={formatNumber(0, locale)}
        numeric
        detail={t("person.readings.nothingOwed")}
      />
    );
  }
  // The most overdue promise decides the card's tone: one late promise is the
  // thing the reader came to be told about, however many are on time.
  const lateness = ours.flatMap((owed) => {
    const late = owed.dueAt ? daysPast(Date.parse(owed.dueAt), asOf) : null;
    return late?.late ? [late.days] : [];
  });
  const worst = Math.max(0, ...lateness);
  const overdue = lateness.length > 0;
  return (
    <StatCard
      label={label}
      // A FLOOR where the server holds more than it sent: next_steps is a
      // page, and a card reporting its length as the total states a number it
      // did not measure.
      value={
        owedPromisesTruncated(view)
          ? t("person.loops.atLeast", {
              count: formatNumber(ours.length, locale),
            })
          : formatNumber(ours.length, locale)
      }
      numeric
      detail={
        overdue
          ? worst > 0
            ? plural("person.loops.overdue", worst, {
                count: formatNumber(worst, locale),
              })
            : t("person.loops.overdueUnderDay")
          : t("person.readings.onTime")
      }
      tone={overdue ? "danger" : undefined}
      dot={overdue}
      basis={
        <FactList
          facts={ours.map((owed) => ({
            key: owed.key,
            term: t("person.loops.ours"),
            value: owed.body,
            note: owed.note,
          }))}
        />
      }
    />
  );
}

// What they decide: the open deal they sit on, its figure, and where it is.
function DealCard({
  view,
  withheld,
  locale,
  zone,
  onOpen,
  t,
}: Readonly<{
  view: Person360;
  withheld: boolean;
  locale: Locale;
  zone: string;
  onOpen?: () => void;
  t: Translate;
}>) {
  const label = t("person.readings.deal");
  if (withheld) {
    return withheldCard(label, t);
  }
  const door = { openLabel: t("person.readings.openDeals"), onOpen };
  const deal = view.commercial?.deal;
  if (!deal) {
    return (
      <StatCard {...door} label={label} value={t("person.strip.noOpenDeal")} />
    );
  }
  const priced =
    deal.amount_minor != null && deal.currency
      ? { amountMinor: deal.amount_minor, currency: deal.currency }
      : undefined;
  const parts = [
    deal.stage ?? undefined,
    deal.close_date
      ? t("person.commercial.closes", {
          date: formatDayMonth(deal.close_date, locale, zone),
        })
      : undefined,
  ].filter((part): part is string => Boolean(part));
  return (
    <StatCard
      {...door}
      label={label}
      // The figure leads where there is one; the deal's name is the reading
      // when nobody has priced it, because "—" over a real deal reads as a
      // deal that failed to load.
      value={
        priced
          ? formatMoneyCompact(priced.amountMinor, priced.currency, locale)
          : deal.title
      }
      numeric={priced !== undefined}
      detail={priced ? [deal.title, ...parts].join(" · ") : parts.join(" · ")}
      basis={
        view.commercial?.role ? (
          <FactList
            facts={[
              {
                key: "role",
                term: t("person.page.buyingRole"),
                value: buyingRoleLabel(view.commercial.role, t),
              },
            ]}
          />
        ) : undefined
      }
    />
  );
}

// When we next see them. The record's zone, which is what the meetings tab
// renders the same field in — rendered in the reader's own instead, the card
// and the tab beside it name different days for one meeting whenever the
// reader is not in that zone.
function MeetingCard({
  view,
  withheld,
  locale,
  zone,
  onOpen,
  t,
}: Readonly<{
  view: Person360;
  withheld: boolean;
  locale: Locale;
  zone: string;
  onOpen?: () => void;
  t: Translate;
}>) {
  const label = t("person.strip.nextMeeting");
  if (withheld) {
    return withheldCard(label, t);
  }
  const door = { openLabel: t("person.readings.openMeetings"), onOpen };
  const meeting = view.next_meeting;
  if (!meeting) {
    return (
      <StatCard {...door} label={label} value={t("person.strip.noMeeting")} />
    );
  }
  return (
    <StatCard
      {...door}
      label={label}
      value={formatDayMonth(meeting.starts_at, locale, zone)}
      detail={meeting.subject ?? undefined}
    />
  );
}

// The verdict word and its tone are read from the SERVER's verdict key, never
// from the rendered word: a translated label must not change how the slot is
// coloured. Read by the rail's consent rows, which is where the verdict is
// drawn now that the readings row no longer carries it.
export function consentWord(
  verdict: string | undefined,
  t: ReturnType<typeof useT>,
): string {
  switch (verdict) {
    case "allowed":
      return t("person.consent.allowedWord");
    case "blocked":
      return t("person.consent.blockedWord");
    default:
      return t("person.consent.unknownWord");
  }
}
