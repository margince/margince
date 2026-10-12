import type { components } from "../api/schema";
import { formatDateAbbrev } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Grounding, StandingTone } from "./record360";

type Lead = components["schemas"]["Lead"];

// Lead standing reports recorded status and responses; a missing response
// does not establish that somebody asked us for anything.

export type LeadStanding = {
  label: string;
  tone: StandingTone;
  // The one sentence the call rests on.
  because: string;
  restsOn: Grounding[];
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
): LeadStanding {
  const when = (at: string) => formatDateAbbrev(at, locale, zone);
  // Merged away is read FIRST, because it is the one ending the ladder does
  // not record: the merge archives the loser and points it at the survivor
  // and leaves `status` exactly where it stood. A lead merged away while it
  // was being worked therefore still says `contacted`, and every branch below
  // would call it open work — or, once something did call it terminal, call
  // it disqualified, which says a human judged the prospect not worth
  // pursuing rather than that it was the same prospect as another one.
  if (lead.merged_into_id) {
    return {
      label: t("lead.standing.merged"),
      tone: "unknown",
      because: t("lead.standing.mergedBecause"),
      restsOn: [
        {
          key: "merged",
          quote: t("lead.standing.rests.merged"),
          from: t("lead.standing.rests.record"),
        },
      ],
    };
  }
  if (lead.status === "promoted") {
    return {
      label: t("lead.standing.qualified"),
      tone: "calm",
      because: lead.promoted_at
        ? t("lead.standing.qualifiedOn", { at: when(lead.promoted_at) })
        : t("lead.standing.qualifiedUndated"),
      restsOn: [
        {
          key: "promoted",
          quote: t("lead.standing.rests.promoted"),
          from: t("lead.standing.rests.ladder"),
        },
      ],
    };
  }
  if (lead.status === "disqualified") {
    return {
      label: t("lead.standing.closed"),
      tone: "unknown",
      because: lead.disqualify_reason
        ? t("lead.standing.closedFor", { reason: lead.disqualify_reason })
        : t("lead.standing.closedUnreasoned"),
      restsOn: [
        {
          key: "closed",
          quote: lead.disqualify_reason ?? t("lead.standing.rests.closed"),
          from: t("lead.standing.rests.ladder"),
        },
      ],
    };
  }
  if (!lead.first_response_at) {
    return unansweredStanding(lead.status, t);
  }
  // Answered. Engaged means they came back or a meeting is on the calendar;
  // contacted means the ball is with them.
  const engaged = lead.status === "engaged";
  return {
    label: engaged ? t("lead.standing.inMotion") : t("lead.standing.theirMove"),
    tone: "accent",
    because: engaged
      ? t("lead.standing.engagedBecause")
      : t("lead.standing.answeredOn", { at: when(lead.first_response_at) }),
    restsOn: [
      {
        key: "response",
        quote: t("lead.sla.answeredAt", { at: when(lead.first_response_at) }),
        from: t("lead.standing.rests.record"),
      },
      ...(engaged && lead.qualification_evidence?.occurred_at
        ? [
            {
              key: "evidence",
              quote: t("lead.standing.rests.engaged", {
                at: when(lead.qualification_evidence.occurred_at),
              }),
              from: t("lead.standing.rests.ladder"),
            },
          ]
        : []),
    ],
  };
}

function unansweredStanding(
  status: "new" | "contacted" | "engaged",
  t: Translator,
): LeadStanding {
  const engaged = status === "engaged";
  const label = t(`lead.status.${status}`);
  return {
    label,
    tone: engaged ? "accent" : "unknown",
    because: t(
      engaged ? "lead.standing.engagedBecause" : "lead.standing.noResponse",
    ),
    restsOn: [
      { key: "status", quote: label, from: t("lead.standing.rests.ladder") },
    ],
  };
}
