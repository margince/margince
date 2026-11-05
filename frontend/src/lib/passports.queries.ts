// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./problem";

export type PassportSummary = components["schemas"]["PassportSummary"];

// The reader's own Agent Seat Passports. The server lists only the passports
// minted on the reader's behalf, an administrator included: which agents act
// for a human is that human's personal data. A connection is made in another
// app, so a screen waiting for one reads again when the reader comes back.
export function usePassports({
  enabled = true,
  refreshOnReturn = false,
}: Readonly<{ enabled?: boolean; refreshOnReturn?: boolean }> = {}) {
  return useQuery({
    queryKey: ["passports"],
    enabled,
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      return unwrap(await api.GET("/passports"));
    },
  });
}

/**
 * passportName is what a reader can recognise a passport by, or undefined. A
 * connection is named by the client the human approved. The server answers its
 * raw client id when that registration is gone, and a connection's label
 * carries the client id too, so neither stands in. A minted passport is named
 * by the label its human typed, unless it is blank.
 */
export function passportName(passport: PassportSummary): string | undefined {
  const { connection } = passport;
  if (connection) {
    const name = connection.client_name.trim();
    return name && name !== connection.client_id ? name : undefined;
  }
  return passport.label.trim() || undefined;
}

// A connection ends when revoked, or when its credential runs out and its grant
// cannot renew. A renewable one past its expiry is only between credentials.
export type ConnectionState = Readonly<{
  revoked: boolean;
  renewing: boolean;
  lapsed: boolean;
  ended: boolean;
}>;

export function connectionStateOf(
  passport: PassportSummary & {
    connection: NonNullable<PassportSummary["connection"]>;
  },
  now: number,
): ConnectionState {
  const revoked = passport.revoked_at != null;
  const expired =
    !revoked &&
    passport.expires_at != null &&
    Date.parse(passport.expires_at) <= now;
  // Renewing, not ended: the client repairs this itself on its next call.
  const renewing = expired && passport.connection.renewable;
  const lapsed = expired && !passport.connection.renewable;
  return { revoked, renewing, lapsed, ended: revoked || lapsed };
}

// The AI tool a reader connected by signing in, while it has not ended. A
// passport minted by hand is a credential for a script, so it never counts.
export function liveConnection(
  passports: readonly PassportSummary[],
  now: number,
): PassportSummary | undefined {
  return passports.find(
    ({ connection, ...passport }) =>
      connection !== undefined &&
      !connectionStateOf({ ...passport, connection }, now).ended,
  );
}

/**
 * usePassportName names a passport the reader may see, and nothing else. A
 * colleague's passport is not in the reader's list, so it resolves to
 * undefined and the chip says "An agent" rather than printing its id.
 */
export function usePassportName(passportId: string): string | undefined {
  const passport = usePassports().data?.data.find(
    (row) => row.id === passportId,
  );
  return passport ? passportName(passport) : undefined;
}
