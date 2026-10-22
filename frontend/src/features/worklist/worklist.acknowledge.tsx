// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import { useNoticeRead } from "../../components/taskactions";
import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { type WorklistItem, worklistKey } from "./worklist.queries";

// The rows the reader takes off their list by saying they have seen them: a
// notice, and an approved change that did not run.

/**
 * Saying the failure has been seen, so it leaves this reader's list. It names
 * the failure the reader saw: one recorded since is refused, and stays.
 */
export function useApprovalFailureAcknowledge(
  invalidateKeys: readonly QueryKey[],
) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (seen: { id: string; failedAt: string }) => {
      unwrap(
        await api.POST("/approvals/{id}/failure-acknowledgement", {
          params: { path: { id: seen.id } },
          body: { failed_at: seen.failedAt },
        }),
        t,
      );
    },
    // Settled either way: a refusal means a newer failure, which the list
    // must show.
    onSettled: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

/**
 * Approving the change once more, which runs the work it released again. The
 * server accepts this only for an approval whose work never ran.
 */
export function useApprovalRetry(invalidateKeys: readonly QueryKey[]) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.POST("/approvals/{id}/approve", {
          params: { path: { id } },
        }),
        t,
      );
    },
    // Settled either way: a retry that fails again moves the failure's time,
    // and the list must show the new one.
    onSettled: () => {
      for (const queryKey of invalidateKeys) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
}

// A failed approval's verbs. The row's id is the approval's, which both
// endpoints take, and `occurred_at` is the failure the reader is shown. Each
// button is disabled only by the other's request: `disabled` would override
// its own `pending` and drop the keyboard focus.
export function FailedApprovalAnswer({
  item,
}: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const acknowledge = useApprovalFailureAcknowledge([worklistKey]);
  const retry = useApprovalRetry([worklistKey, ["approvals"]]);
  const failedAt = item.occurred_at;
  return (
    <>
      {item.actions.includes("retry") ? (
        <Button
          pending={retry.isPending}
          disabled={acknowledge.isPending}
          onClick={() =>
            retry.mutate(item.id, {
              onSuccess: () => toast.show(t("worklist.verb.approvalRetried")),
              onError: () =>
                toast.show(t("worklist.verb.approvalRetryFailed"), {
                  tone: "danger",
                }),
            })
          }
        >
          {t("worklist.verb.retry")}
        </Button>
      ) : null}
      {failedAt ? (
        <Button
          pending={acknowledge.isPending}
          disabled={retry.isPending}
          onClick={() =>
            acknowledge.mutate(
              { id: item.id, failedAt },
              {
                onError: () =>
                  toast.show(t("worklist.verb.acknowledgeFailed"), {
                    tone: "danger",
                  }),
              },
            )
          }
        >
          {t("worklist.verb.acknowledge")}
        </Button>
      ) : null}
    </>
  );
}

// A notice's one verb: the reader has seen it, and it leaves the lane.
// Settled here rather than routed as a link. The mutation is the notice's own
// read endpoint, and no other screen answers "I've seen this".
export function NoticeAcknowledge({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const toast = useToast();
  const acknowledge = useNoticeRead([worklistKey]);
  return (
    <Button
      pending={acknowledge.isPending}
      onClick={() =>
        acknowledge.mutate(id, {
          // A rejected read leaves the button idle, which looks like a click
          // that did nothing. Without the toast the reader has no reason to
          // try again.
          onError: () =>
            toast.show(t("worklist.verb.acknowledgeFailed"), {
              tone: "danger",
            }),
        })
      }
    >
      {t("worklist.verb.acknowledge")}
    </Button>
  );
}
