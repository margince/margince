// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "./common";

type PassportSummary = components["schemas"]["PassportSummary"];

// The reader's own Agent Seat Passports. The server lists only the passports
// minted on the reader's behalf, an administrator included: which agents act
// for a human is that human's personal data.
export function usePassports() {
  return useQuery({
    queryKey: ["passports"],
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
