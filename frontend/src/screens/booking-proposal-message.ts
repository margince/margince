import { formatDateTime } from "../format/format";
import type { Locale, useT } from "../i18n";

// The proposal email is written once here, because the composer that creates
// a proposal and the contact tab that resends one must send the same words.
export function proposalEmailBody(
  t: ReturnType<typeof useT>,
  proposal: Readonly<{
    description: string;
    options: readonly { start: string }[];
    url: string;
  }>,
  locale: Locale,
  zone: string,
) {
  const times = proposal.options
    .map((slot) => formatDateTime(slot.start, locale, zone))
    .join("\n");
  return [
    t("scheduling.proposalGreeting"),
    proposal.description,
    times,
    `${t("scheduling.proposalChoose")}\n${proposal.url}`,
    t("scheduling.proposalReply"),
  ]
    .filter((part) => part.trim() !== "")
    .join("\n\n");
}
