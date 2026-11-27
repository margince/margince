// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Why one task is on the list, written from that task's own records.
//
// The server writes it once per reader and keeps it until the task, its
// message or the relationship moves. So it is asked for only when the details
// open. Facts are what happened, the assessment is what waiting costs, and the
// recommendation is the next step. A section the server left unsaid is not
// drawn, because no sentence here may fit any other task.

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import { type BriefSentence, SentenceList } from "../../components/citations";
import { AiPending } from "../../design-system/aipending";
import { AiReveal } from "../../design-system/aireveal";
import { useT } from "../../i18n";
import { useSourceOpener } from "../../lib/opensource";
import { unwrap } from "../../lib/problem";

export function useTaskExplanation(taskId: string) {
  const t = useT();
  return useQuery({
    queryKey: ["task-explanation", taskId],
    queryFn: async () =>
      unwrap(
        await api.GET("/activities/{id}/task-explanation", {
          params: { path: { id: taskId } },
        }),
        t,
      ),
    // The answer is cached server-side on its inputs, so a reopen within the
    // session needs no second round trip.
    staleTime: 5 * 60 * 1000,
  });
}

export type TaskExplanationParts = Readonly<{
  happened: BriefSentence[];
  stakes: BriefSentence[];
  next: BriefSentence[];
}>;

/** The explanation's sentences, by the section each belongs to. */
export function explanationParts(
  sentences: readonly BriefSentence[],
): TaskExplanationParts {
  return {
    happened: sentences.filter((s) => (s.nature ?? "fact") === "fact"),
    stakes: sentences.filter((s) => s.nature === "assessment"),
    next: sentences.filter((s) => s.nature === "recommendation"),
  };
}

/** The sections a task's details draw, in the panel's own section shape. */
export function TaskExplanationSections({
  taskId,
  section,
  todo,
}: Readonly<{
  taskId: string;
  section: (heading: string, children: ReactNode) => ReactNode;
  // What the panel draws under What to do besides the step: the row's verbs.
  todo?: ReactNode;
}>) {
  const t = useT();
  const explanation = useTaskExplanation(taskId);
  const openerFor = useSourceOpener();
  const onOpenEmail = (id: string) => openerFor({ id, kind: "email" })?.();
  if (explanation.isPending) {
    return section(
      t("worklist.why.happened"),
      <AiPending label={t("worklist.why.writing")} lines={3} />,
    );
  }
  const parts = explanationParts(explanation.data?.sentences ?? []);
  const said = (sentences: BriefSentence[]) => (
    <AiReveal text={sentences.map((s) => s.text).join(" ")}>
      <SentenceList sentences={sentences} onOpenEmail={onOpenEmail} />
    </AiReveal>
  );
  return (
    <>
      {parts.happened.length > 0 &&
        section(t("worklist.why.happened"), said(parts.happened))}
      {parts.stakes.length > 0 &&
        section(t("worklist.why.consequence"), said(parts.stakes))}
      {(parts.next.length > 0 || todo) &&
        section(
          t("worklist.why.todo"),
          <>
            {parts.next.length > 0 && said(parts.next)}
            {todo}
          </>,
        )}
    </>
  );
}
