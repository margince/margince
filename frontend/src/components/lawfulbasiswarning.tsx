// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "../lib/problem";

// The warning before a send to a recipient whose country has no
// legitimate-interest basis. It applies when the record holds neither their
// consent nor an agreement with them. It does not block: the person sending
// confirms it, and the server records that confirmation under their name.

type Preview = components["schemas"]["SendAuthorizationPreview"];

/** The addresses the preview marks as having no lawful basis on record. */
export function lawfulBasisMissing(preview: Preview | undefined): string[] {
  return (preview?.recipients ?? [])
    .filter((recipient) => recipient.lawful_basis_missing === true)
    .map((recipient) => recipient.address);
}

/** The standing warning, drawn wherever the preview is. */
export function LawfulBasisCallout({
  preview,
}: Readonly<{ preview: Preview | undefined }>) {
  const t = useT();
  const missing = lawfulBasisMissing(preview);
  if (missing.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="danger"
      kind="standing"
      live="status"
      title={t("lawfulBasis.title")}
    >
      <p className="t-body">{t("lawfulBasis.body")}</p>
      <p className="t-caption">{missing.join(", ")}</p>
    </Callout>
  );
}

/**
 * Holds a send until the person confirms the warning for every recipient it
 * applies to. `gate(proceed)` runs `proceed` at once when nothing is missing,
 * else after the confirmation is recorded. `dialog` is drawn by the caller.
 */
export function useLawfulBasisGate(preview: Preview | undefined) {
  const t = useT();
  const missing = lawfulBasisMissing(preview);
  const [pending, setPending] = useState<(() => void) | null>(null);
  const record = useMutation({
    mutationFn: async (addresses: string[]) => {
      const answered = await api.POST("/emails/lawful-basis-confirmations", {
        body: { addresses },
      });
      if (!answered.response.ok) {
        throwProblem(answered.error ?? { title: t("lawfulBasis.failed") });
      }
      return answered.data;
    },
  });
  const close = () => {
    setPending(null);
    record.reset();
  };
  const gate = (proceed: () => void) => {
    if (missing.length === 0) {
      proceed();
      return;
    }
    setPending(() => proceed);
  };
  const dialog = (
    <ConfirmModal
      open={pending !== null}
      onClose={close}
      title={t("lawfulBasis.title")}
      confirmLabel={t("lawfulBasis.confirm")}
      confirmVariant="danger"
      pending={record.isPending}
      error={record.error ? problemMessageOf(record.error, t) : null}
      onConfirm={() => {
        record.mutate(missing, {
          onSuccess: () => {
            const proceed = pending;
            close();
            proceed?.();
          },
        });
      }}
    >
      <Callout tone="danger" kind="standing" title={t("lawfulBasis.body")}>
        <p className="t-caption">{missing.join(", ")}</p>
      </Callout>
    </ConfirmModal>
  );
  return { missing, gate, dialog };
}
