import type { components } from "../../api/schema";
import { activityDay } from "../../format/activitywhen";
import type { Locale, useT } from "../../i18n";

type Lead = components["schemas"]["Lead"];
type PromoteLeadRequest = components["schemas"]["PromoteLeadRequest"];

export function triggerFor(lead: Lead): PromoteLeadRequest["trigger"] {
  return lead.qualification_evidence?.trigger ?? "human_qualify";
}

export function reasonSentence(
  lead: Lead,
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): string {
  const evidence = lead.qualification_evidence;
  if (!evidence) {
    return t("lead.qualify.reasonHuman");
  }
  const at = evidence.occurred_at
    ? activityDay(
        evidence.occurred_at,
        evidence.occurred_precision ?? "instant",
        locale,
        zone,
      )
    : "";
  switch (evidence.trigger) {
    case "meeting_held":
      return t("lead.qualify.reasonMeetingHeld", { at });
    case "meeting_booked":
      return t("lead.qualify.reasonMeetingBooked", { at });
    default:
      return t("lead.qualify.reasonReplied", { at });
  }
}
