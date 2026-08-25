import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import {
  formatDayMonth,
  formatMoneyCompact,
  formatNumber,
  relativeDays,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";

// The relationship state strip (concept §5.3): six facts that change how a
// reader interprets everything below them.
//
// The two DIRECTIONS are separate slots and never folded into one "last
// touch". Which way the last message went is the whole question — a contact we
// mailed a fortnight ago with no reply and one who wrote to us this morning
// have the same last-touch date and opposite meanings.
//
// Every slot distinguishes three states, and the difference matters: a value,
// a fact that there is none ("None", "Never"), and a section the caller may
// not read. Only the last renders as withheld — "no open deal" is an answer.

type Person360 = components["schemas"]["Person360"];

export function PersonStrip({
  view,
  consentVerdict,
}: Readonly<{
  view: Person360;
  consentVerdict: string | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const omitted = new Set(view.sections_omitted ?? []);
  // A withheld slot says so. Rendering it empty would read as "there is none",
  // which is a claim about the record rather than about the reader's grants —
  // and a withheld reading carries no tone, because there is no verdict to
  // colour.
  const reading = (value: string, withheld: boolean) =>
    withheld ? t("record.notShown") : value;
  const consentIsShown = !omitted.has("consent");
  return (
    <StatStrip testId="person-strip">
      <StatCard
        label={t("person.strip.lastInbound")}
        value={reading(
          relativeDays(view.last_inbound_at, t, locale),
          omitted.has("last_touch"),
        )}
      />
      <StatCard
        label={t("person.strip.lastOutbound")}
        value={reading(
          relativeDays(view.last_outbound_at, t, locale),
          omitted.has("last_touch"),
        )}
      />
      <StatCard
        label={t("person.strip.reciprocity")}
        value={reading(reciprocity(view, t, locale), omitted.has("activities"))}
      />
      <StatCard
        label={t("person.strip.openDeal")}
        value={reading(openDeal(view, t, locale), omitted.has("commercial"))}
      />
      <StatCard
        label={t("person.strip.nextMeeting")}
        value={reading(
          nextMeeting(view, t, locale, recordZone),
          omitted.has("next_meeting"),
        )}
      />
      <StatCard
        label={t("person.strip.consent")}
        value={reading(consentWord(consentVerdict, t), !consentIsShown)}
        tone={consentIsShown ? consentTone(consentVerdict) : undefined}
        dot={consentIsShown}
      />
    </StatStrip>
  );
}

// Counts, not a score. A standalone number here would be the composite verdict
// the face deliberately does not carry (ADR-0096 D1).
function reciprocity(
  view: Person360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const rows = view.activities?.data ?? [];
  let inbound = 0;
  let outbound = 0;
  for (const row of rows) {
    if (row.direction === "inbound") {
      inbound += 1;
    }
    if (row.direction === "outbound") {
      outbound += 1;
    }
  }
  return t("person.strip.inOut", {
    inbound: formatNumber(inbound, locale),
    outbound: formatNumber(outbound, locale),
  });
}

function openDeal(
  view: Person360,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string {
  const deal = view.commercial?.deal;
  if (!deal) {
    return t("person.strip.noOpenDeal");
  }
  if (deal.amount_minor == null || !deal.currency) {
    return deal.title;
  }
  // The design system's glance formatter, which company360 already uses for
  // the same job on the account page. It reads the scale off the CURRENCY and
  // abbreviates in the reader's own conventions — German goes to "Mio." where
  // English goes to "m", and a locale-blind table said "k" to everyone.
  return formatMoneyCompact(deal.amount_minor, deal.currency, locale);
}

function nextMeeting(
  view: Person360,
  t: ReturnType<typeof useT>,
  locale: Locale,
  recordZone: string,
): string {
  const meeting = view.next_meeting;
  if (!meeting) {
    return t("person.strip.noMeeting");
  }
  // The record's zone, which is what persontabs.tsx renders the same field in.
  // Rendered in the reader's own instead, the strip and the tab beside it name
  // different days for one meeting whenever the reader is not in that zone.
  return formatDayMonth(meeting.starts_at, locale, recordZone);
}

// The verdict word and its tone are read from the SERVER's verdict key, never
// from the rendered word: a translated label must not change how the slot is
// coloured.
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

// A verdict slot is coloured in both directions: allowed reads as allowed and
// blocked reads as blocked. An absent verdict is neither — nobody has judged
// it yet, so it takes no tone rather than borrowing one.
function consentTone(
  verdict: string | undefined,
): "good" | "danger" | undefined {
  if (verdict === "allowed") {
    return "good";
  }
  return verdict === "blocked" ? "danger" : undefined;
}
