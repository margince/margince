// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { navigate } from "../app/router";
import { Button } from "../design-system/atoms";
import { useOwnToast } from "../design-system/toast";
import { useT } from "../i18n";
import {
  ProblemError,
  problemExistingId,
  problemMessageOf,
  unwrap,
  useMe,
} from "./common";
import { contactLeadKey, LEAD_LIST_KEY } from "./leadkeys";

type Lead = components["schemas"]["Lead"];

/**
 * "Work as a lead": a contact the CRM already holds becomes the one an
 * opportunity is worked through, without retyping them. The lead is filled
 * from the contact on the server (`contact_id`), owned by the reader who asked,
 * and opened. A contact already worked through a live lead offers that lead
 * instead, and a refusal naming one opens it rather than failing.
 */
export function WorkAsLeadAction({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const me = useMe();
  const canCreate = useCanWrite("lead", "create");
  const toast = useOwnToast();
  const queryClient = useQueryClient();
  const worked = useQuery({
    queryKey: contactLeadKey(contactId),
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/leads", {
          params: { query: { from_contact_id: contactId, limit: 1 } },
        }),
      );
      return data.data[0] ?? null;
    },
  });
  const create = useMutation({
    mutationFn: async ({
      contact,
      owner,
    }: Readonly<{
      contact: string;
      owner: string | undefined;
    }>): Promise<Lead> => {
      const { data, error } = await api.POST("/leads", {
        body: {
          contact_id: contact,
          owner_id: owner,
          status: "new",
          source: "manual",
        },
      });
      if (error) {
        throw new ProblemError(error, t);
      }
      return data;
    },
    onSuccess: (lead) => {
      void queryClient.invalidateQueries({ queryKey: LEAD_LIST_KEY });
      navigate({ screen: "leads", id: lead.id });
    },
    onError: (error) => {
      const existing =
        error instanceof ProblemError ? problemExistingId(error.problem) : null;
      if (existing) {
        navigate({ screen: "leads", id: existing.id });
        return;
      }
      toast.show(problemMessageOf(error, t));
    },
  });
  const open = worked.data;
  if (open) {
    return (
      <Button onClick={() => navigate({ screen: "leads", id: open.id })}>
        {t("contact.action.openLead")}
      </Button>
    );
  }
  if (!canCreate) {
    return null;
  }
  return (
    <Button
      disabled={create.isPending || !me.data || worked.isPending}
      onClick={() =>
        create.mutate({ contact: contactId, owner: me.data?.user.id })
      }
    >
      {t("contact.action.workAsLead")}
    </Button>
  );
}
