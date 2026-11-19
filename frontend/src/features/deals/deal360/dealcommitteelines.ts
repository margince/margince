// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import type { Translator } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { committeeContacts } from "./dealcommittee";

type DealCoverage = components["schemas"]["DealCoverage"];
type DealCoverageLine = components["schemas"]["DealCoverageLine"];

type Way = "wrote" | "replied" | "met";

const WAY_WORDS: Record<Way, MessageKey> = {
  wrote: "deal.committee.lines.wrote",
  replied: "deal.committee.lines.replied",
  met: "deal.committee.lines.met",
};

export type CommitteeLineRow = Readonly<{
  key: string;
  contactId: string;
  // The one sentence a reader of the text alone gets for this pair.
  text: string;
}>;

export type CommitteeUnreached = Readonly<{
  contactId: string;
  text: string;
}>;

/** newest picks the pair's latest event and which way it went. */
function newest(line: DealCoverageLine): { way: Way; at: string } | undefined {
  const events: { way: Way; at: string }[] = [];
  if (line.last_outbound_at) {
    events.push({ way: "wrote", at: line.last_outbound_at });
  }
  if (line.last_inbound_at) {
    events.push({ way: "replied", at: line.last_inbound_at });
  }
  if (line.last_met_at) {
    events.push({ way: "met", at: line.last_met_at });
  }
  return events.sort((a, b) => Date.parse(b.at) - Date.parse(a.at))[0];
}

/**
 * Which colleague talks to which buyer seat, and the seats no colleague has
 * reached. Undefined when the read carries no `lines`: that read was withheld
 * from this reader, so no seat may be called unreached from it.
 */
export function committeeLines(
  coverage: DealCoverage | undefined,
  t: Translator,
  when: (at: string) => string,
):
  | Readonly<{ lines: CommitteeLineRow[]; unreached: CommitteeUnreached[] }>
  | undefined {
  if (!coverage?.lines || coverage.sections_omitted.length > 0) {
    return undefined;
  }
  const colleagues = new Map(
    coverage.our_side.map((colleague) => [
      colleague.user_id,
      colleague.display_name,
    ]),
  );
  const seats = committeeContacts(coverage.stakeholders);
  const nameOf = (seat: (typeof seats)[number]) =>
    seat.contact_name ?? t("coverage.seatWithheld");
  const lines: CommitteeLineRow[] = [];
  const reached = new Set<string>();
  for (const seat of seats) {
    const own = coverage.lines
      .filter((line) => line.contact_id === seat.contact_id)
      .flatMap((line) => {
        const last = newest(line);
        const colleague = colleagues.get(line.user_id);
        return last && colleague ? [{ line, last, colleague }] : [];
      })
      .sort((a, b) => Date.parse(b.last.at) - Date.parse(a.last.at));
    for (const { line, last, colleague } of own) {
      reached.add(seat.contact_id);
      lines.push({
        key: `${line.user_id}:${seat.contact_id}`,
        contactId: seat.contact_id,
        text: t("deal.committee.lines.pair", {
          colleague,
          contact: nameOf(seat),
          how: t(WAY_WORDS[last.way], { date: when(last.at) }),
        }),
      });
    }
  }
  const unreached = seats
    .filter((seat) => !reached.has(seat.contact_id))
    .map((seat) => ({
      contactId: seat.contact_id,
      text: t("deal.committee.lines.unreached", { contact: nameOf(seat) }),
    }));
  return { lines, unreached };
}
