// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { unwrap } from "../lib/problem";
import { invalidateRecord } from "../lib/recordwritekeys";
import { contactCasesKey, worklistKey } from "../lib/workkeys";

export type ContactCase = components["schemas"]["ContactCase"];
export type ContactCaseMember = components["schemas"]["ContactCaseMember"];
export type ContactCaseOutcome = components["schemas"]["ContactCaseOutcome"];
export type ContactCaseDecision =
  components["schemas"]["ContactCaseDecisionRequest"];

export function contactCaseKey(contactId: string) {
  return [...contactCasesKey, contactId] as const;
}

/**
 * The case that holds this contact, or null when it is in none. A 404 is the
 * normal answer for a contact with no duplicates, so it is data and not a
 * failure.
 */
export function useContactCase(contactId: string, enabled = true) {
  return useQuery({
    queryKey: contactCaseKey(contactId),
    enabled,
    queryFn: async (): Promise<ContactCase | null> => {
      const answer = await api.GET("/dedupe/contact-cases/{contact_id}", {
        params: { path: { contact_id: contactId } },
      });
      if (answer.response.status === 404) return null;
      return unwrap(answer);
    },
  });
}

type DecisionInput = Readonly<{ anchorId: string; body: ContactCaseDecision }>;

export function useContactCasePreview() {
  return useMutation({
    mutationFn: async ({ anchorId, body }: DecisionInput) =>
      unwrap(
        await api.POST("/dedupe/contact-cases/{contact_id}/preview", {
          params: { path: { contact_id: anchorId } },
          body,
        }),
      ),
  });
}

export function useContactCaseMerge() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ anchorId, body }: DecisionInput) =>
      unwrap(
        await api.POST("/dedupe/contact-cases/{contact_id}/merge", {
          params: { path: { contact_id: anchorId } },
          body,
        }),
      ),
    onSuccess: (outcome, { body }) =>
      invalidateAfterMerge(client, body, outcome),
  });
}

// Every member page and list that showed a merged-away contact, plus the
// Worklist rows and every cached case, since one merge reshapes all of them.
function invalidateAfterMerge(
  client: QueryClient,
  body: ContactCaseDecision,
  outcome: ContactCaseOutcome,
): Promise<unknown> {
  const touched = new Set([...body.member_ids, outcome.survivor_id]);
  return Promise.all([
    client.invalidateQueries({ queryKey: worklistKey }),
    client.invalidateQueries({ queryKey: ["contacts"] }),
    client.invalidateQueries({ queryKey: contactCasesKey }),
    ...[...touched].map((id) => invalidateRecord(client, "contact", id)),
  ]);
}
