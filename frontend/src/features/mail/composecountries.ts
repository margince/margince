// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { skipToken, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

/** A country the sender stated for one recipient; `null` is one they cleared. */
export type RecipientCountry = components["schemas"]["RecipientCountry"];
export type RecipientCountryReading =
  components["schemas"]["RecipientCountryReading"];

// Mirrors the server's canonical address: trimmed, lower case.
const canonical = (address: string) => address.trim().toLowerCase();

/** The sender's statement about one address, or undefined for none. */
export function statedFor(
  stated: readonly RecipientCountry[],
  address: string,
): RecipientCountry | undefined {
  return stated.find((entry) => entry.address === canonical(address));
}

/**
 * The statements about addresses the message carries, or undefined when there
 * are none. The server ignores the rest, and a recipient taken off the message
 * takes their country with them.
 */
export function countriesOnMessage(
  stated: readonly RecipientCountry[],
  addresses: readonly string[],
): RecipientCountry[] | undefined {
  const held = new Set(addresses.map(canonical));
  const kept = stated.filter((entry) => held.has(entry.address));
  return kept.length > 0 ? kept : undefined;
}

export function sameCountries(
  a: readonly RecipientCountry[] | undefined,
  b: readonly RecipientCountry[] | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  return (
    left.length === right.length &&
    left.every((entry) => {
      const other = statedFor(right, entry.address);
      return other !== undefined && other.country === entry.country;
    })
  );
}

/** The countries the sender stated on the message being written. */
export function useRecipientCountries(seed?: readonly RecipientCountry[]) {
  const [stated, setStated] = useState<readonly RecipientCountry[]>(seed ?? []);
  const without = (list: readonly RecipientCountry[], address: string) =>
    list.filter((entry) => entry.address !== canonical(address));
  return {
    stated,
    /** States a country, or clears it with `null`. */
    state: (address: string, country: string | null) =>
      setStated((now) => [
        ...without(now, address),
        { address: canonical(address), country },
      ]),
    /** Goes back to the record's country. */
    forget: (address: string) => setStated((now) => without(now, address)),
    restore: (list: readonly RecipientCountry[] | undefined) =>
      setStated(list ?? []),
  };
}

export type RecipientCountries = ReturnType<typeof useRecipientCountries>;

/**
 * The country the record states for one address, read when its chip is
 * opened. A plain read, not a send check: writing a message asks nothing.
 */
export function useRecordCountry(address: string, open: boolean) {
  const query = useQuery({
    queryKey: ["compose-recipient-country", canonical(address)],
    queryFn: open
      ? async () => {
          const answer = unwrap(
            await api.POST("/recipient-countries", {
              body: { addresses: [address] },
            }),
          );
          return answer.recipients[0];
        }
      : skipToken,
    staleTime: 60_000,
  });
  return { reading: query.data, settled: query.isSuccess || query.isError };
}
