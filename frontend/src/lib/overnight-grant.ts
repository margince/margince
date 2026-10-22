// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "@composition/schema";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { unwrap } from "./problem";
import { readStored, removeStored, STORAGE_KEYS, writeStored } from "./storage";

// The rep's standing answer to "may an agent work as me overnight?", read and
// written once for the two places that ask it: onboarding and Settings.

export type MyAgentGrant = components["schemas"]["MyAgentGrant"];

/** The agent this question is about. The morning brief is the one a rep
 * recognises by name, and the sweep that feeds it runs under the same answer. */
export const MORNING_BRIEF = "morning_brief" as const;

const GRANTS_KEY = ["agent-grants"] as const;

// A real OAuth "allow" leaves the page entirely for the provider's consent
// screen, so React state does not survive the trip back. The rep's answer to
// the overnight question has to, or an opt-out is silently reversed: they
// untick the box, click Google, come back, and the step grants anyway because
// the component remounted at its preselected default.
//
// Same mechanism and same scope as the OAuth attempt mark beside it
// (`onboarding-connect-panels.tsx`): sessionStorage, belonging to the ONE tab
// that started the trip.

/** Remembers the rep's answer across the OAuth round trip. */
export function rememberOvernightChoice(wanted: boolean): void {
  writeStored(STORAGE_KEYS.overnightChoice, wanted ? "yes" : "no");
}

/** The remembered answer, or undefined when this tab has none. Undefined is
 * distinct from `false`: it means "not answered here", which is what makes the
 * preselected default correct rather than an override of an opt-out. */
export function rememberedOvernightChoice(): boolean | undefined {
  const stored = readStored(STORAGE_KEYS.overnightChoice);
  return stored === null ? undefined : stored === "yes";
}

/** Drops the mark once the step has acted on it, so a later visit to
 * onboarding in the same tab starts from the default rather than from an
 * answer given to a question that has already been settled. */
export function forgetOvernightChoice(): void {
  removeStored(STORAGE_KEYS.overnightChoice);
}

/** Reads every scheduled agent's standing answer for the signed-in rep. */
export function useAgentGrants() {
  return useQuery({
    queryKey: GRANTS_KEY,
    queryFn: async () => {
      const data = unwrap(await api.GET("/me/agent-grants"));
      return data.data;
    },
  });
}

/** Writes one answer. Granting mints the rep's own credential server-side;
 * withdrawing revokes it. Neither is a field this client could send. */
export function useSetAgentGrant() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (granted: boolean) => {
      return unwrap(
        await api.PUT("/me/agent-grants/{spec}", {
          params: { path: { spec: MORNING_BRIEF } },
          body: { granted },
        }),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: GRANTS_KEY });
      toast.show(t("settings.saved"));
    },
  });
}

/** Picks the morning brief's answer out of the list. */
export function morningBriefGrant(
  grants: readonly MyAgentGrant[],
): MyAgentGrant | undefined {
  return grants.find((grant) => grant.spec === MORNING_BRIEF);
}
