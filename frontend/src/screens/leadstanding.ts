import type { components } from "../api/schema";
import { formatDateAbbrev, formatDateTime } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { Grounding, StandingTone } from "./record360";

type Lead = components["schemas"]["Lead"];

// Where a lead stands, as the call at the head of its page.
//
// A lead has no server verdict the way a deal has its standing card and a
// contact its moment. What it has is a small set of FACTS the server already
// decides — the ladder status, whether a first response went out and whether
// it was on time, how the lead was closed — and every call here is one of
// those facts said in a word, with the fact under it as what the call rests
// on. Nothing is inferred from tone or from dates the server did not judge:
// "your move" on a new lead is the server's own first-response clock, not this
// page's reading of a silence.

export type LeadStanding = {
  label: string;
  tone: StandingTone;
  // The one sentence the call rests on.
  because: string;
  restsOn: Grounding[];
};

export function leadStanding(
  lead: Lead,
  t: Translator,
  locale: Locale,
  zone: string,
): LeadStanding {
  const when = (at: string) => formatDateAbbrev(at, locale, zone);
  // The first-response target is set in hours, so its deadline is an instant
  // and prints with its time — the same precision the readings card gives it.
  const instant = (at: string) => formatDateTime(at, locale, zone);
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
  // An open lead nobody has answered: the move is ours, and how loudly it is
  // ours is the server's own first-response clock.
  if (!lead.first_response_at) {
    return {
      label: t("lead.standing.yourMove"),
      tone: slaTone(lead.sla_state),
      because: unansweredBecause(lead, t, instant),
      restsOn: [
        {
          key: "captured",
          quote: t("lead.standing.rests.captured", {
            at: when(lead.created_at),
          }),
          from: t("lead.standing.rests.record"),
        },
        {
          key: "response",
          quote: t("lead.standing.rests.noResponse"),
          from: t("lead.standing.rests.record"),
        },
      ],
    };
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

// How loud an unanswered lead is: the server's own three-state clock. No
// clock at all — an installation with no first-response target — is simply our
// move, without alarm.
function slaTone(state: Lead["sla_state"]): StandingTone {
  switch (state) {
    case "breached":
      return "danger";
    case "at_risk":
      return "warn";
    default:
      return "accent";
  }
}

function unansweredBecause(
  lead: Lead,
  t: Translator,
  instant: (at: string) => string,
): string {
  if (!lead.sla_deadline_at) {
    return t("lead.standing.noResponse");
  }
  return lead.sla_state === "breached"
    ? t("lead.standing.overdueSince", { at: instant(lead.sla_deadline_at) })
    : t("lead.standing.dueBy", { at: instant(lead.sla_deadline_at) });
}
