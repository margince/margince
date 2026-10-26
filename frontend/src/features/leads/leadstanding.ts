import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import { activityDay, type OccurredPrecision } from "../../format/activitywhen";
import { formatDateAbbrev } from "../../format/format";
import { leadIdentityName } from "../../format/leadname";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Lead = components["schemas"]["Lead"];

// Lead standing reports recorded status and responses; a missing response
// does not establish that somebody asked us for anything.

export type LeadStanding = Readonly<{
  word: string;
  // The one sentence the call rests on.
  sentence: string;
  tone: ReadingTone;
}>;

type Evidence = NonNullable<Lead["qualification_evidence"]>;

const EVIDENCE_SENTENCE: Record<Evidence["trigger"], MessageKey> = {
  inbound_reply: "lead.standing.repliedOn",
  meeting_booked: "lead.standing.meetingBookedFor",
  meeting_held: "lead.standing.meetingHeldOn",
};

// The first-response clock as ONE fact, read by the call and by the readings
// card alike: the server runs a target only when it has both a deadline and a
// verdict on it, and a deadline without a verdict is not a clock either surface
// may be late against. Exported so the two cannot read the same row apart.
export function firstResponseClock(
  lead: Lead,
): { deadline: string; state: NonNullable<Lead["sla_state"]> } | undefined {
  if (!lead.sla_deadline_at || !lead.sla_state) {
    return undefined;
  }
  return { deadline: lead.sla_deadline_at, state: lead.sla_state };
}

// The terminal badge a lead earns (null = live/open, no badge). Keying the
// label off the ENDING rather than a bare archived_at is what stops a promoted
// lead reading "Disqualified".
//
// The merge is read first because it is the ending the ladder does not record:
// it archives the lead, points it at the survivor and leaves `status` exactly
// where it stood. Read from the status alone, a lead merged away mid
// conversation wears no badge at all and every surface draws it as open work.
//
// Exhaustive over the four statuses below: a new value is a compile error
// here, not a silently-unlabelled row.
export function terminalBadge(
  lead: Pick<Lead, "status" | "merged_into_id">,
): { label: MessageKey; tone: "warning" } | null {
  if (lead.merged_into_id) {
    return { label: "lead.merged", tone: "warning" };
  }
  switch (lead.status) {
    case "disqualified":
      return { label: "lead.disqualified", tone: "warning" };
    case "promoted":
      return { label: "record.archived", tone: "warning" };
    case "new":
    case "contacted":
    case "engaged":
      return null;
  }
}

export function leadStanding(
  lead: Lead,
  t: Translator,
  locale: Locale,
  zone: string,
  record: string,
): LeadStanding {
  const when = (at: string) => formatDateAbbrev(at, locale, zone);
  // Merged away is read first: the merge archives the loser and leaves
  // `status` where it stood, so every branch below would call it open work.
  if (lead.merged_into_id) {
    return {
      word: t("lead.standing.merged"),
      sentence: t("lead.standing.mergedBecause"),
      tone: "neutral",
    };
  }
  if (lead.status === "promoted") {
    return {
      word: t("lead.standing.qualified"),
      sentence: lead.promoted_at
        ? t("lead.standing.qualifiedOn", { at: when(lead.promoted_at) })
        : t("lead.standing.qualifiedUndated"),
      tone: "success",
    };
  }
  if (lead.status === "disqualified") {
    // Neutral, not a failure colour: closing a lead is ordinary work.
    return {
      word: t("lead.standing.closed"),
      sentence: lead.disqualify_reason
        ? t("lead.standing.closedFor", { reason: lead.disqualify_reason })
        : t("lead.standing.closedUnreasoned"),
      tone: "neutral",
    };
  }
  const engaged = lead.status === "engaged";
  if (engaged) {
    return {
      word: lead.first_response_at
        ? t("lead.standing.inMotion")
        : t("lead.status.engaged"),
      sentence: engagedSentence(lead, t, (at, precision) =>
        activityDay(at, precision, locale, record, formatDateAbbrev),
      ),
      tone: "success",
    };
  }
  if (!lead.first_response_at) {
    return {
      word: t(`lead.status.${lead.status}`),
      sentence: t("lead.standing.noResponse"),
      tone: "neutral",
    };
  }
  return {
    word: t("lead.standing.theirMove"),
    sentence: t("lead.standing.answeredOn", {
      at: when(lead.first_response_at),
    }),
    tone: "success",
  };
}

// The signal that moved the lead to Engaged, dated on the record's clock, a
// day entry by its own date. A lead set by hand keeps the general sentence.
function engagedSentence(
  lead: Lead,
  t: Translator,
  when: (at: string, precision: OccurredPrecision) => string,
): string {
  const evidence = lead.qualification_evidence;
  if (!evidence?.occurred_at) {
    return t("lead.standing.engagedBecause");
  }
  return t(EVIDENCE_SENTENCE[evidence.trigger], {
    name: leadIdentityName(lead) || t("lead.unnamed"),
    at: when(evidence.occurred_at, evidence.occurred_precision ?? "instant"),
  });
}
