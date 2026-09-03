// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { EntityRef } from "../entityref";

type Seat = components["schemas"]["DealCoverageSeat"];

/**
 * One stakeholder seat's person: a link to them, or the sentence saying this
 * reader may not know who they are.
 *
 * ONE component because three cards on the deal record drew this cell — the
 * coverage strip's receipt, the seats rail and the committee map's accessible
 * list — and all three printed `person_name` as text, so the only surface
 * naming the people on a deal offered no way to open any of them. Three copies
 * also drifted the way copies do: two of them said "A contact you cannot read"
 * and the third said "A stakeholder you cannot see", which is one fact in two
 * sentences on one record.
 *
 * The withheld case is deliberately NOT a link. `person_id` is always on the
 * wire and only the NAME is withheld, so an href could be built — and it would
 * offer a route this reader may not take, to a record the API answers 404 for
 * precisely so that its existence stays hidden. A link there would be an
 * existence oracle drawn as a courtesy.
 */
export function SeatPerson({ seat }: Readonly<{ seat: Seat }>) {
  const t = useT();
  if (!seat.person_name) {
    return <>{t("coverage.seatWithheld")}</>;
  }
  return (
    <EntityRef kind="person" id={seat.person_id} name={seat.person_name} />
  );
}
