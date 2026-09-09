// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { leadWriteKeys } from "./leadkeys";
import type { LeadWriter } from "./leads";

type Lead = components["schemas"]["Lead"];

// What the page's write refused, stated once for both of them.
//
// Two mutations, one sentence: the patch every field goes through, and the
// claim that takes an unowned lead. They are alternatives — a pick is one or
// the other — so whichever refused is the one to name.
//
// Spelled once because two readers ask it: the callout below, to say so, and
// the band around the callout, to decide whether it has anything to draw at
// all. Two spellings is how a band comes to be drawn around a callout that
// renders nothing.
function writeRefusal(writer: LeadWriter) {
  return writer.patch.error ?? writer.claim.error;
}

function LeadWriteRefusal({ writer }: Readonly<{ writer: LeadWriter }>) {
  const t = useT();
  const error = writeRefusal(writer);
  if (!error) {
    return null;
  }
  return (
    <Callout tone="danger" kind="outcome" title={t("lead.writeRefused")}>
      {problemMessageOf(error, t)}
    </Callout>
  );
}

// The band under the header: what the lead has to say about itself as a whole
// — a write it refused, the sentence that says it is closed, and the way back
// out of a disqualification.
//
// RecordView draws the band's element and its two intervals for anything it is
// handed, so this answers `undefined` rather than a fragment that renders
// nothing when there is nothing to say: a fragment is truthy, and every live
// lead paid a step of dead air between its strip and its columns for one.
export function leadBand({
  lead,
  writer,
  reasonId,
  id,
  t,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  reasonId: string;
  id: string;
  t: ReturnType<typeof useT>;
}>): ReactNode | undefined {
  // The way back, beside the sentence that says the lead is closed. Offered
  // only on a disqualification a reader may WRITE: the promoted closure is the
  // demote's to reverse, and a reader who cannot change this lead cannot
  // reopen it either.
  const reopenable =
    Boolean(lead.archived_at) &&
    lead.status === "disqualified" &&
    lead.writable !== false;
  if (!writeRefusal(writer) && !writer.readOnlyReason && !reopenable) {
    return undefined;
  }
  return (
    <>
      {/* The page's one write serves both columns and every tab, so what it
          REFUSES is stated where both are visible. In the ladder panel this
          reached only the Overview tab, and a rail write refused while the
          reader was on History said nothing at all. */}
      <LeadWriteRefusal writer={writer} />
      {/* Stated ONCE for the page. Every control the closure refuses points at
          this element by id, so a screen reader reaches it from each of them
          without the sentence being printed beside all six. */}
      {writer.readOnlyReason && (
        <p id={reasonId} className="t-caption">
          {/* Which closure, not merely THAT it is closed. Both terminal states
              archive the row, so keying this off archived_at alone told every
              promoted lead it had been disqualified — invisible until ADR-0119
              stopped the page redirecting away before anyone could read it. A
              live lead that is somebody else's prints the writer's own sentence
              instead. */}
          {lead.archived_at
            ? lead.status === "promoted"
              ? t("lead.terminalPromoted")
              : t("lead.terminalDisqualified")
            : writer.readOnlyReason}
        </p>
      )}
      {reopenable && <ReopenAction id={id} />}
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
 * because it unwinds a person and a reader of that trail needs to know why;
 * reopening restores a status the trail already holds, and there is nothing a
 * caller could say that the server does not read for itself.
 */
function ReopenAction({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const reopen = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/leads/{id}/reopen", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
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
  return (
    <>
      <Button small onClick={() => setOpen(true)}>
        {t("lead.reopen")}
      </Button>
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
