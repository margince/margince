// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

type PassportSummary = components["schemas"]["PassportSummary"];

// The reader's own Agent Seat Passports. The server lists only the passports
// minted on the reader's behalf, an administrator included: which agents act
// for a human is that human's personal data.
export function usePassports() {
  return useQuery({
    queryKey: ["passports"],
    queryFn: async () => {
      const { data, error } = await api.GET("/passports");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// A connection is named by the client the human approved, a minted passport by
// the label its human typed.
export function passportName(passport: PassportSummary): string {
  return passport.connection?.client_name ?? passport.label;
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
  return passport && passportName(passport);
}
