// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { WriteRefused } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { leadWriteKeys } from "../../lib/leadkeys";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type { LeadWriter } from "./leads";

type Lead = components["schemas"]["Lead"];

// What the page's write refused, stated once for both of them.
//
// Two mutations, one sentence: the patch every field goes through, and the
// claim that takes an unowned lead. They are alternatives — a pick is one or
// the other — so whichever refused is the one to name.
//
// Spelled once because two readers ask it: the notice below, to say so, and the
// band around it, to decide whether it has anything to draw at all. Two
// spellings is how a band comes to be drawn around a notice that renders
// nothing.
function writeRefusal(writer: LeadWriter) {
  return writer.patch.error ?? writer.claim.error;
}

// The one sentence this page says about the lead taking no changes, with the
// closure it names. Both terminal states archive the row, so keying this off
// archived_at alone told every promoted lead it had been disqualified; the
// status says which. A live lead that is somebody else's prints the writer's
// own sentence instead.
//
// Every control the closure refuses points at the element that carries it by
// id. A screen reader reaches it from each of them without the sentence being
// printed beside all six.
export function terminalSentence(
  lead: Lead,
  writer: LeadWriter,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!writer.readOnlyReason) {
    return undefined;
  }
  if (!lead.archived_at) {
    return writer.readOnlyReason;
  }
  return lead.status === "promoted"
    ? t("lead.terminalPromoted")
    : t("lead.terminalDisqualified");
}

// The band under the strip: a write the lead refused, and the sentence that
// says a live lead is not this reader's to change. A closed lead says so in
// the verdict of How it stands, so its sentence stays hidden (the actions
// carry it for assistive technology). The way back out of a disqualification
// is in More.
//
// RecordView draws the band's element and its two intervals for anything it is
// handed, so this answers `undefined` rather than a fragment that renders
// nothing when there is nothing to say: a fragment is truthy, and every live
// lead paid a step of dead air between its strip and its columns for one.
export function leadBand({
  lead,
  writer,
  reasonId,
  t,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  reasonId: string;
  t: ReturnType<typeof useT>;
}>): ReactNode | undefined {
  const refusal = writeRefusal(writer);
  const sentence = lead.archived_at
    ? undefined
    : terminalSentence(lead, writer, t);
  if (!refusal && !sentence) {
    return undefined;
  }
  return (
    <>
      {/* The page's one write serves both columns and every tab, so what it
          REFUSES is stated where both are visible. In the ladder panel this
          reached only the Overview tab, and a rail write refused while the
          reader was on History said nothing at all. */}
      <WriteRefused titleKey="lead.writeRefused" error={refusal} />
      {sentence && <p id={reasonId}>{sentence}</p>}
    </>
  );
}

/**
 * ReopenAction puts a disqualified lead back on the open ladder.
 *
 * The page could say "Disqualified: <reason>" and could not offer the way
 * back. A judgement that somebody is not worth pursuing is exactly the kind
 * that changes — the budget arrives, the champion returns — and with no way
 * back the operator re-keys the lead, which loses its history and its score
 * along with its reason.
 *
 * No reason field, unlike the demote beside it. The demote asks for one
 * because it unwinds a contact and a reader of that trail needs to know why;
 * reopening restores a status the trail already holds, and there is nothing a
 * caller could say that the server does not read for itself.
 */
export function ReopenAction({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const id = lead.id;
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const reopen = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/leads/{id}/reopen", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      for (const key of leadWriteKeys(id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      setOpen(false);
    },
  });
  const close = () => {
    setOpen(false);
    reopen.reset();
  };
  // The way back is offered only on a disqualification a reader may write. The
  // promoted closure is the demote's to reverse, and a reader who cannot
  // change this lead cannot reopen it either.
  if (
    !lead.archived_at ||
    lead.status !== "disqualified" ||
    lead.writable === false
  ) {
    return null;
  }
  return (
    <>
      <Button onClick={() => setOpen(true)}>{t("lead.reopen")}</Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("lead.reopenDialog")}
        confirmLabel={t("lead.reopenConfirm")}
        onConfirm={() => reopen.mutate()}
        pending={reopen.isPending}
        error={reopen.isError ? problemMessageOf(reopen.error, t) : undefined}
      >
        <p className="t-body">{t("lead.reopenExplain")}</p>
      </ConfirmModal>
    </>
  );
}
