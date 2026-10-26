// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { useTaskUpdate } from "../../components/taskactions";
import { useCompleteTask } from "../../components/taskcomplete";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { type WorklistItem, worklistKey } from "./worklist.queries";

// The question a promise task asks once we wrote to its recipient after the
// promise: was that mail the promise kept? The contact page asks the same
// question under the same key, so Not yet given here holds there too. The
// task is never closed without the reader's Done.
export function WorklistMayBeDone({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const queryClient = useQueryClient();
  const question = item.may_be_done;
  const refresh = [
    worklistKey,
    ["contact360", question?.contact_id ?? ""],
  ] as const;
  const task = useTaskUpdate(refresh);
  const complete = useCompleteTask(task);
  // The question rides in as the mutation's variables, so a press answers the
  // question the reader saw even when a newer one has rendered since.
  const notYet = useMutation({
    mutationFn: async (asked: NonNullable<WorklistItem["may_be_done"]>) => {
      unwrap(
        await api.POST("/contacts/{id}/moment/dismiss", {
          params: { path: { id: asked.contact_id } },
          body: {
            claim_key: asked.claim_key,
            evidence_fingerprint: asked.evidence_fingerprint,
          },
        }),
        t,
      );
    },
    onSuccess: (_, asked) =>
      Promise.all(
        [worklistKey, ["contact360", asked.contact_id]].map((queryKey) =>
          queryClient.invalidateQueries({ queryKey }),
        ),
      ),
  });
  if (!question) return null;
  const pending = task.isPending || notYet.isPending;
  return (
    <div className="worklist-may-be-done">
      <p className="t-caption">
        {t("worklist.mayBeDone.asked", {
          date: formatDate(question.wrote_at, locale, zone),
        })}
      </p>
      <span className="today-verb">
        <Button
          variant="primary"
          disabled={pending || !item.actions.includes("complete")}
          onClick={() => complete(item.id, item.version)}
        >
          {t("worklist.mayBeDone.done")}
        </Button>
      </span>
      <span className="today-verb">
        <Button
          variant="ghost"
          disabled={pending}
          onClick={() => notYet.mutate(question)}
        >
          {t("worklist.mayBeDone.notYet")}
        </Button>
      </span>
      {(task.error ?? notYet.error) && (
        <ErrorLine error={task.error ?? notYet.error} inline />
      )}
    </div>
  );
}
