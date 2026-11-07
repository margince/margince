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
type RelationshipChange = components["schemas"]["ContactRelationshipChange"];

/** The call How it stands draws. */
export type ContactVerdict = Readonly<{
  word: string;
  sentence?: string;
  tone: ReadingTone;
  /** The change the sentence was read under: the newest one that agrees with
   *  the tone the sentence was chosen for. The ring's caption draws this one,
   *  so the caption can never point the other way from the sentence. */
  change?: RelationshipChange;
}>;

// The wire carries no tone for a rule. An owed thing that slipped is danger,
// an owed thing still on time is warning, a reply after silence is good news.
const MOMENT_TONE: Readonly<Record<ContactMomentRule, ReadingTone>> = {
  privacy_notice_due: "warning",
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
    const tone = MOMENT_TONE[moment.rule];
    const change = changeUnder(view, tone);
    return {
      word: t(`contact.moment.rule.${moment.rule}`),
      sentence: bandSentence(view, t, change),
      tone,
      change,
    };
  }
  const strength = view.strength;
  if (!strength) {
    return moment
      ? {
          word: t("contact.moment.rule.nothing_needed"),
          sentence: t("contact.verdict.nothingOwed"),
          tone: "neutral",
        }
      : null;
  }
  if (strength.bucket === "none") {
    return {
      word: bucketLabel(strength.bucket, t),
      sentence: t("contact.verdict.noExchange"),
      tone: "neutral",
    };
  }
  // A band-only reading is no judgement of direction, so its sentence takes
  // the newest change either way, and the caption takes the same one.
  const change = changeUnder(view, "neutral");
  return {
    word: bucketLabel(strength.bucket, t),
    sentence: bandSentence(view, t, change),
    tone: bandTone(strength.bucket),
    change,
  };
}

// Which way each change moved the relationship.
const CHANGE_DIRECTION: Readonly<
  Record<RelationshipChange["kind"], "better" | "worse">
> = {
  warmed: "better",
  replied_after_gap: "better",
  cooled: "worse",
  went_quiet: "worse",
};

// A change may stand under a heading only when it points the same way. Under
// "Gone quiet", a band that moved up is true and still reads as a
// contradiction, so the newest change that agrees is used instead.
function agreesWith(change: RelationshipChange, tone: ReadingTone): boolean {
  const direction = CHANGE_DIRECTION[change.kind];
  if (tone === "danger" || tone === "warning") return direction !== "better";
  if (tone === "success") return direction !== "worse";
  return true;
}

/** The newest change that may stand under a heading of this tone. The wire
 *  orders changes by consequence, not by time, so the newest is picked by
 *  `at`, not by position. */
export function changeUnder(
  view: Contact360,
  tone: ReadingTone,
): RelationshipChange | undefined {
  let newest: RelationshipChange | undefined;
  let newestAt = Number.NEGATIVE_INFINITY;
  for (const candidate of view.relationship_changes ?? []) {
    if (!agreesWith(candidate, tone)) continue;
    const at = Date.parse(candidate.at);
    if (newest === undefined || at > newestAt) {
      newest = candidate;
      newestAt = at;
    }
  }
  return newest;
}

function bandSentence(
  view: Contact360,
  t: Translator,
  change: RelationshipChange | undefined,
): string | undefined {
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

export function colleaguesInTouch(view: Contact360) {
  const colleagues = view.network?.colleagues ?? [];
  if (
    colleagues.some(
      (colleague) =>
        colleague.interactions_90d > 0 && colleague.two_way === undefined,
    )
  )
    return undefined;
  return colleagues.filter((colleague) => colleague.two_way === true);
}

// Colleagues who sent the contact a message and got no reply. Being copied on
// someone else's mail is not writing, so `sent_90d`, not `outbound_90d`.
function colleaguesWhoWrote(view: Contact360) {
  return (view.network?.colleagues ?? []).filter(
    (colleague) => colleague.two_way !== true && (colleague.sent_90d ?? 0) > 0,
  );
}

function reachDim(
  view: Contact360,
  t: Translator,
  locale: Locale,
): StandingDim | null {
  if (omitted(view, "network")) {
    return null;
  }
  const inTouch = colleaguesInTouch(view);
  const label = t("contact.standing.dim.reach");
  if (!inTouch)
    return {
      key: "reach",
      label,
      value: t("record.notShown"),
      tone: "neutral",
    };
  const wrote = colleaguesWhoWrote(view)
    .map((colleague) => colleague.display_name)
    .join(", ");
  if (inTouch.length === 0) {
    return {
      key: "reach",
      label,
      value: t(
        wrote
          ? "contact.standing.dim.reachOneWay"
          : "contact.standing.dim.reachNone",
      ),
      sub: wrote
        ? t("contact.standing.dim.wroteNoReply", { names: wrote })
        : undefined,
      tone: "neutral",
    };
  }
  const names = inTouch.map((colleague) => colleague.display_name).join(", ");
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
    sub: wrote
      ? `${names} · ${t("contact.standing.dim.alsoWrote", { names: wrote })}`
      : names,
    // One colleague in touch is a single thread, not coverage.
    tone: inTouch.length > 1 ? "success" : "warning",
  };
}
