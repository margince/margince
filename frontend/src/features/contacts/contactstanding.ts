import type { components } from "../../api/schema";
import { bandTone, type ReadingTone } from "../../design-system/readingtone";
import type { StandingDim } from "../../design-system/standingdims";
import type { StrengthBand } from "../../design-system/strengthmeter";
import { formatNumber } from "../../format/format";
import { daysPast } from "../../format/lateness";
import {
  type Locale,
  type PluralBase,
  type Translator,
  translatePlural,
} from "../../i18n";
import { bucketLabel } from "../../lib/bucketlabel";
import { omitted } from "./contact360";
import { directionSentence } from "./contactrail";
import { changeSentence } from "./relationshipchange";

type Contact360 = components["schemas"]["Contact360"];
type ContactMomentRule = components["schemas"]["ContactMomentRule"];

/** The call How it stands draws. */
export type ContactVerdict = Readonly<{
  word: string;
  sentence?: string;
  tone: ReadingTone;
}>;

// The wire carries no tone for a rule. An owed thing that slipped is danger,
// an owed thing still on time is warning, a reply after silence is good news.
const MOMENT_TONE: Readonly<Record<ContactMomentRule, ReadingTone>> = {
  overdue_promise: "danger",
  gone_quiet: "danger",
  open_promise: "warning",
  missing_next_step: "warning",
  meeting_prep: "warning",
  job_change: "warning",
  public_signal: "warning",
  re_engaged: "success",
  thin_relationship: "neutral",
  nothing_needed: "neutral",
};

/**
 * contactVerdict is the one reading of how a contact stands. It is the moment
 * the server's rule ladder picked while it asks for something, else the
 * strength band the ring is drawn from. So the word and the ring cannot
 * disagree.
 * Null when both readings are withheld from this reader.
 */
export function contactVerdict(
  view: Contact360,
  t: Translator,
): ContactVerdict | null {
  const moment = view.moment;
  if (moment && moment.rule !== "nothing_needed") {
    // The Next bar above already says why now, so the sentence here is the
    // relationship's own reading and the two never print one line twice.
    return {
      word: t(`contact.moment.rule.${moment.rule}`),
      sentence: bandSentence(view, t),
      tone: MOMENT_TONE[moment.rule],
    };
  }
  const strength = view.strength;
  if (!strength) {
    return moment
      ? { word: t("contact.moment.rule.nothing_needed"), tone: "neutral" }
      : null;
  }
  if (strength.bucket === "none") {
    return { word: bucketLabel(strength.bucket, t), tone: "neutral" };
  }
  return {
    word: bucketLabel(strength.bucket, t),
    sentence: bandSentence(view, t),
    tone: bandTone(strength.bucket),
  };
}

function bandSentence(view: Contact360, t: Translator): string | undefined {
  const change = view.relationship_changes?.[0];
  if (change) {
    return changeSentence(change, t);
  }
  return omitted(view, "last_touch") ? undefined : directionSentence(view, t);
}

/** The three rows beside the ring. A withheld section draws no row, because
 *  the overview's coverage line already names it. */
export function contactDims(
  view: Contact360,
  t: Translator,
  locale: Locale,
): StandingDim[] {
  return [
    promisesDim(view, t, locale),
    exchangeDim(view, t, locale),
    reachDim(view, t, locale),
  ].filter((dim): dim is StandingDim => dim !== null);
}

// `claims` returns every open promise of ours uncapped, so the count is whole.
function promisesDim(
  view: Contact360,
  t: Translator,
  locale: Locale,
): StandingDim | null {
  if (omitted(view, "claims")) {
    return null;
  }
  const asOf = Date.parse(view.as_of);
  const open = (view.claims ?? []).filter(
    (claim) => claim.kind === "commitment_ours" && claim.status === "open",
  );
  const overdue = open.filter(
    (claim) =>
      claim.due_at != null && daysPast(Date.parse(claim.due_at), asOf).late,
  );
  const label = t("contact.standing.dim.promises");
  const count = (base: PluralBase, n: number) =>
    translatePlural(locale, base, n, { count: formatNumber(n, locale) });
  if (open.length === 0) {
    return {
      key: "promises",
      label,
      value: t("contact.standing.dim.promisesNone"),
      tone: "neutral",
    };
  }
  const named = (overdue[0] ?? open[0])?.body;
  if (overdue.length > 0) {
    return {
      key: "promises",
      label,
      value: count("standing.overdue", overdue.length),
      sub: [count("standing.ofOpen", open.length), named]
        .filter(Boolean)
        .join(" · "),
      tone: "danger",
    };
  }
  return {
    key: "promises",
    label,
    value: count("standing.open", open.length),
    sub: named,
    tone: "success",
  };
}

// The same 90-day window and the same server count the ring's score is from.
function exchangeDim(
  view: Contact360,
  t: Translator,
  locale: Locale,
): StandingDim | null {
  const inbound = view.strength?.inbound_90d;
  const outbound = view.strength?.outbound_90d;
  if (inbound === undefined || outbound === undefined) {
    return null;
  }
  return {
    key: "exchange",
    label: t("contact.standing.dim.exchange"),
    value: t("contact.standing.dim.exchangeValue", {
      in: formatNumber(inbound, locale),
      out: formatNumber(outbound, locale),
    }),
    sub: t("contact.standing.dim.window"),
    tone: exchangeTone(inbound, outbound, view.strength?.bucket),
  };
}

// Two-way traffic is green only once the ring agrees it is a real
// relationship; a weak score beside a green row would contradict it.
function exchangeTone(
  inbound: number,
  outbound: number,
  band: StrengthBand | undefined,
): ReadingTone {
  if (inbound > 0 && outbound > 0) {
    return band === "weak" ? "warning" : "success";
  }
  return inbound + outbound > 0 ? "warning" : "neutral";
}

function reachDim(
  view: Contact360,
  t: Translator,
  locale: Locale,
): StandingDim | null {
  if (omitted(view, "network")) {
    return null;
  }
  const inTouch = (view.network?.colleagues ?? []).filter(
    (colleague) => colleague.interactions_90d > 0,
  );
  const label = t("contact.standing.dim.reach");
  if (inTouch.length === 0) {
    return {
      key: "reach",
      label,
      value: t("contact.standing.dim.reachNone"),
      tone: "neutral",
    };
  }
  return {
    key: "reach",
    label,
    value: translatePlural(
      locale,
      "contact.standing.dim.inTouch",
      inTouch.length,
      {
        count: formatNumber(inTouch.length, locale),
      },
    ),
    sub: inTouch.map((colleague) => colleague.display_name).join(", "),
    // One colleague in touch is a single thread, not coverage.
    tone: inTouch.length > 1 ? "success" : "warning",
  };
}
