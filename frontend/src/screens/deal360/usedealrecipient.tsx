// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The address a first message from the deal page is offered.
//
// Two steps, because a deal reaches an address through a person: pick the seat
// (dealRecipientSeat), then read that person for the address the rest of the
// product writes to (format/primaryEmail). Neither half decides anything on its
// own — the seat rule is the deal's, the address rule is shared with the
// drafter and every screen.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { primaryEmail } from "../../format/primaryemail";
import { throwProblem } from "../common";
import { dealRecipientSeat } from "./dealrecipient";
import type { useDealCoverage } from "./usedealcoverage";

/**
 * useDealRecipientAddress answers who a fresh mail on this deal is addressed
 * to, or undefined when there is nobody to offer.
 *
 * It rides the `["person", id]` cache the person page and the composer already
 * fetch under, so a reader who has opened that contact pays nothing here and
 * one who has not pays a single read — and only once the coverage answered with
 * a seat worth reading.
 *
 * Undefined while the reads are in flight, when coverage is WITHHELD, and for a
 * deal whose stakeholders carry no live address. The composer fills an empty
 * field only, so undefined leaves the reader typing the address exactly as they
 * do today rather than waiting on anything.
 */
export function useDealRecipientAddress(
  coverage: ReturnType<typeof useDealCoverage>,
): string | undefined {
  // Withheld is not empty: a caller without the relationship grant is served no
  // seats at all, and picking "the first of none" would quietly become "this
  // deal has nobody on it" for exactly the readers who cannot check.
  const seat = coverage.withheld
    ? undefined
    : dealRecipientSeat(coverage.coverage?.stakeholders);
  const person = useQuery({
    queryKey: ["person", seat?.person_id],
    queryFn: async () => {
      const { data, error } = await api.GET("/people/{id}", {
        params: { path: { id: seat?.person_id as string } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: seat?.person_id != null,
  });
  return primaryEmail(person.data?.emails);
}
