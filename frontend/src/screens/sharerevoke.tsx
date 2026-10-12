// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemCodeOf, problemMessageOf, throwProblem } from "./common";

// The share screen's refusal wording and its revoke dialog.

type RecordGrant = components["schemas"]["RecordGrant"];

// Marks a 403 `approval_required` from the grant and revoke routes. The screen
// then says the change waits for approval, not the raw problem detail.
export class ApprovalRequiredError extends Error {}

// Three refusals get the screen's own sentence. A seat_tier_insufficient names
// the recipient's licence, not the actor's permission. Every other refusal
// reads best in the server's words.
export function shareRefusalMessage(
  error: unknown,
  t: ReturnType<typeof useT>,
): string {
  if (error instanceof ApprovalRequiredError) {
    return t("share.approvalRequired");
  }
  if (problemCodeOf(error) === "seat_tier_insufficient") {
    return t("share.seatCeiling");
  }
  if (problemCodeOf(error) === "version_skew") {
    return t("share.versionSkew");
  }
  return problemMessageOf(error, t);
}

// The version the reader saw rides the revoke, so a grant re-asserted since
// (a new level, expiry or reason) is refused instead of removed unseen.
export function RevokeGrantDialog({
  grant,
  grantsKey,
  onClose,
  returnFocusTo,
}: {
  grant: RecordGrant | null;
  grantsKey: readonly unknown[];
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const revoke = useMutation({
    mutationFn: async (held: RecordGrant) => {
      const { error } = await api.DELETE("/record-grants/{id}", {
        params: { path: { id: held.id }, ...ifMatch(held.version) },
      });
      if (error) {
        if (error.code === "approval_required") {
          throw new ApprovalRequiredError();
        }
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: grantsKey });
      onClose();
    },
    onError: (error) => {
      if (problemCodeOf(error) === "version_skew") {
        queryClient.invalidateQueries({ queryKey: grantsKey });
      }
    },
  });

  return (
    <ConfirmModal
      open={grant !== null}
      onClose={() => {
        onClose();
        revoke.reset();
      }}
      title={t("share.revoke")}
      confirmLabel={t("share.revoke")}
      confirmVariant="danger"
      returnFocusTo={returnFocusTo}
      onConfirm={() => {
        if (grant) {
          revoke.mutate(grant);
        }
      }}
      pending={revoke.isPending}
      error={revoke.isError ? shareRefusalMessage(revoke.error, t) : null}
    >
      <p>{t("share.revokeConfirm")}</p>
    </ConfirmModal>
  );
}
