// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { useT } from "../../i18n";
import { problemCodeOf, refusalSentences, unwrap } from "../../lib/problem";

type Purpose = components["schemas"]["ConsentPurpose"];
type Change = { mode: "rename" | "archive"; label: string };

export function PurposeActions({ purpose }: Readonly<{ purpose: Purpose }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<Change["mode"] | null>(null);
  const [label, setLabel] = useState(purpose.label);
  const change = useMutation({
    mutationFn: async (input: Change) => {
      const params = { path: { id: purpose.id } };
      if (input.mode === "archive") {
        unwrap(await api.DELETE("/consent-purposes/{id}", { params }));
      } else {
        unwrap(
          await api.PATCH("/consent-purposes/{id}", {
            params,
            body: { label: input.label.trim() },
          }),
        );
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["consent-purposes"] });
      setMode(null);
    },
  });
  const refusal = refusalSentences(change.error, t, {
    label: t("privacy.purposeLabel"),
  });
  const error = change.isError
    ? problemCodeOf(change.error) === "consent_purpose_in_use"
      ? t("privacy.purposeInUse")
      : refusal.rest
    : null;
  function open(next: Change["mode"]) {
    change.reset();
    setLabel(purpose.label);
    setMode(next);
  }
  return (
    <>
      <OverflowMenu label={t("table.actions")}>
        <Button onClick={() => open("rename")} aria-haspopup="dialog">
          {t("privacy.purposeRename")}
        </Button>
        <Button onClick={() => open("archive")} aria-haspopup="dialog">
          {t("record.archive")}
        </Button>
      </OverflowMenu>
      <ConfirmModal
        open={mode !== null}
        onClose={() => setMode(null)}
        title={
          mode === "archive"
            ? t("privacy.purposeArchiveTitle", { label: purpose.label })
            : t("privacy.purposeRenameTitle", { label: purpose.label })
        }
        intent="form"
        pending={change.isPending}
        confirmLabel={
          mode === "archive" ? t("record.archive") : t("record.save")
        }
        confirmVariant={mode === "archive" ? "danger" : "primary"}
        confirmDisabled={mode === "rename" && label.trim() === ""}
        onConfirm={() => {
          if (mode) change.mutate({ mode, label });
        }}
        error={error}
      >
        <p>{t("privacy.purposeAppendOnly")}</p>
        {mode === "rename" && (
          <Field
            label={t("privacy.purposeLabel")}
            error={refusal.byField.get("label")}
          >
            {(control) => (
              <TextInput
                {...control}
                value={label}
                required
                onChange={(event) => {
                  setLabel(event.target.value);
                  change.reset();
                }}
              />
            )}
          </Field>
        )}
      </ConfirmModal>
    </>
  );
}
