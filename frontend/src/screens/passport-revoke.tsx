// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type ReactNode, useRef, useState } from "react";
import { api } from "../api/client";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, unwrap } from "./common";
import { usePassports } from "./passports.queries";

/**
 * Ending a passport or a connection: DELETE /passports/{id} behind a confirm.
 * The list refetches before the dialog closes, so focus returns to a row that
 * already reads as ended; `focusAfter` names that row by the id confirmed.
 */
export function usePassportRevoke({
  verb,
  question,
  focusAfter,
}: Readonly<{
  verb: MessageKey;
  question: MessageKey;
  focusAfter: (id: string) => HTMLElement | null;
}>): Readonly<{ ask: (id: string) => void; confirm: ReactNode }> {
  const t = useT();
  const list = usePassports();
  const [pendingId, setPendingId] = useState<string | null>(null);
  // Read as the dialog closes, when `pendingId` is already back to null.
  const askedId = useRef("");
  const revoke = useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/passports/{id}", { params: { path: { id } } }));
    },
    onSuccess: async () => {
      await list.refetch();
      setPendingId(null);
    },
  });
  const confirm = (
    <ConfirmModal
      open={pendingId != null}
      onClose={() => {
        setPendingId(null);
        revoke.reset();
      }}
      title={t(verb)}
      confirmLabel={t(verb)}
      confirmVariant="danger"
      onConfirm={() => pendingId && revoke.mutate(pendingId)}
      pending={revoke.isPending}
      error={revoke.error ? problemMessageOf(revoke.error, t) : null}
      returnFocusTo={() => focusAfter(askedId.current)}
    >
      <p>{t(question)}</p>
    </ConfirmModal>
  );
  return {
    ask: (id) => {
      askedId.current = id;
      setPendingId(id);
    },
    confirm,
  };
}
