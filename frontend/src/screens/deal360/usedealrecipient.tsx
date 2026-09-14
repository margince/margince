// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The address a first message from the deal page is offered.
//
// Two steps, because a deal reaches an address through a contact: pick the seat
// (dealRecipientSeat), then read that contact for the address the rest of the
// product writes to. Neither half decides anything on its own — the seat rule
// is the deal's, and the address is the server's `primary_email`, which every
// screen and the drafter read rather than choose again.

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { throwProblem } from "../common";
import { dealRecipientSeat } from "./dealrecipient";
import type { useDealCoverage } from "./usedealcoverage";

/**
 * useDealRecipientAddress answers who a fresh mail on this deal is addressed
 * to, or undefined when there is nobody to offer.
 *
 * It rides the `["contact", id]` cache the contact page and the composer already
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
  const contact = useQuery({
    queryKey: ["contact", seat?.contact_id],
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}", {
        params: { path: { id: seat?.contact_id as string } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    enabled: seat?.contact_id != null,
  });
  return contact.data?.primary_email ?? undefined;
}
