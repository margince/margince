// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import type { OccurredPrecision } from "../../../format/activitywhen";
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

type Newest = Readonly<{ way: Way; at: string; precision: OccurredPrecision }>;

/** newest picks the pair's latest event and which way it went. */
function newest(line: DealCoverageLine): Newest | undefined {
  const events: Newest[] = [];
  if (line.last_outbound_at) {
    events.push({
      way: "wrote",
      at: line.last_outbound_at,
      precision: line.last_outbound_precision ?? "instant",
    });
  }
  if (line.last_inbound_at) {
    events.push({
      way: "replied",
      at: line.last_inbound_at,
      precision: line.last_inbound_precision ?? "instant",
    });
  }
  if (line.last_met_at) {
    events.push({
      way: "met",
      at: line.last_met_at,
      precision: line.last_met_precision ?? "instant",
    });
  }
  return events.sort((a, b) => Date.parse(b.at) - Date.parse(a.at))[0];
}

/**
 * Which colleague talks to which buyer seat, and the seats no colleague has
 * reached, as the server states them. Undefined when the read carries no
 * `lines`: that read was withheld from this reader.
 */
export function committeeLines(
  coverage: DealCoverage | undefined,
  t: Translator,
  when: (at: string, precision: OccurredPrecision) => string,
):
  | Readonly<{ lines: CommitteeLineRow[]; unreached: CommitteeUnreached[] }>
  | undefined {
  if (!coverage?.lines || coverage.sections_omitted.length > 0) {
    return undefined;
  }
  const seats = committeeContacts(coverage.stakeholders);
  const nameOf = (seat: (typeof seats)[number]) =>
    seat.contact_name ?? t("coverage.seatWithheld");
  const lines: CommitteeLineRow[] = [];
  for (const seat of seats) {
    const own = coverage.lines
      .filter((line) => line.contact_id === seat.contact_id)
      .flatMap((line) => {
        const last = newest(line);
        return last ? [{ line, last }] : [];
      })
      .sort((a, b) => Date.parse(b.last.at) - Date.parse(a.last.at));
    for (const { line, last } of own) {
      lines.push({
        key: `${line.user_id}:${seat.contact_id}`,
        contactId: seat.contact_id,
        text: t("deal.committee.lines.pair", {
          colleague: line.colleague_name,
          contact: nameOf(seat),
          how: t(WAY_WORDS[last.way], { date: when(last.at, last.precision) }),
        }),
      });
    }
  }
  // Only the server can prove nobody reached a seat. Any other state, or none,
  // says nothing: a withheld exchange is not silence.
  const unreached = seats
    .filter((seat) => seat.reach === "not_reached")
    .map((seat) => ({
      contactId: seat.contact_id,
      text: t("deal.committee.lines.unreached", { contact: nameOf(seat) }),
    }));
  return { lines, unreached };
}
