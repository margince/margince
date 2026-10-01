// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";

type Automation = components["schemas"]["Automation"];

// Deleting an automation drops the rule entirely — the records it watched stop
// being watched — so it asks first, the way every other destructive verb in
// settings does. It owns its own mutation and its own staged state rather than
// borrowing the row's: the question, the refusal and the write are one thing,
// and keeping them together is what lets the row stay a row.
//
// The refusal stays IN the dialog, which is where the reader still is. A row
// that reported it underneath would report it behind the thing covering it.
export function DeleteAutomationAction({
  automation,
}: Readonly<{ automation: Automation }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [asking, setAsking] = useState(false);

  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE("/automations/{id}", {
        params: { path: { id: automation.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      setAsking(false);
      queryClient.invalidateQueries({ queryKey: ["automations"] });
    },
  });

  return (
    <>
      <Button
        variant="danger"
        disabled={remove.isPending}
        onClick={() => setAsking(true)}
      >
        {t("auto.delete")}
      </Button>
      <ConfirmModal
        open={asking}
        onClose={() => {
          setAsking(false);
          remove.reset();
        }}
        title={t("auto.deleteTitle")}
        confirmLabel={t("auto.delete")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => remove.mutate()}
      >
        <p>{t("auto.deleteBody", { name: automation.name })}</p>
      </ConfirmModal>
    </>
  );
}
