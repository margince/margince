// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
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
  useMe,
} from "./common";

type Lead = components["schemas"]["Lead"];

/**
 * "Work as a lead": a contact the CRM already holds becomes the person an
 * opportunity is worked through, without retyping them. The lead is filled
 * from the contact on the server (`contact_id`), owned by the reader who asked,
 * and opened. A lead already holding the contact's address is that same
 * person's lead, so the refusal that says so opens it rather than failing.
 */
export function WorkAsLeadAction({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const me = useMe();
  const canCreate = useCanWrite("lead", "create");
  const toast = useOwnToast();
  const create = useMutation({
    mutationFn: async (owner: string | undefined): Promise<Lead> => {
      const { data, error } = await api.POST("/leads", {
        body: {
          contact_id: contactId,
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
    onSuccess: (lead) => navigate({ screen: "leads", id: lead.id }),
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
  if (!canCreate) {
    return null;
  }
  return (
    <Button
      disabled={create.isPending || !me.data}
      onClick={() => create.mutate(me.data?.user.id)}
    >
      {t("contact.action.workAsLead")}
    </Button>
  );
}
