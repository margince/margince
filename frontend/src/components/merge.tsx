// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { type RequiredGrant, useGrantRefused } from "../lib/capability";
import { fromHere } from "../lib/navigatefrom";
import type { Route } from "../lib/router";
import "../design-system/querystates.css";

// The shared "Merge into…" affordance (P-2): a human direct call that folds
// this record (the source, A) into a picked survivor (B) — A is archived
// with merged_into_id=B, B keeps the id the rest of the CRM already points
// at. Contact and Company 360s have an identical merge shape (target_id body
// + If-Match precondition, survivor Contact/Company back), so this stays
// resource-agnostic: the screen supplies the search transport, the merge
// transport, and where the survivor's 360 lives.

export type MergeSide = Readonly<{ name: string; detail?: string }>;

// Each side by its name and its detail line, so the question asked before a
// merge and the answer after it tell two records that share a name apart.
function mergeSideLabel(t: ReturnType<typeof useT>, side: MergeSide): string {
  return side.detail
    ? t("merge.recordDetail", { name: side.name, detail: side.detail })
    : side.name;
}

// The question asked before a merge, or the answer after it.
export function mergeSentence(
  t: ReturnType<typeof useT>,
  key: "merge.confirm" | "merge.done",
  source: MergeSide,
  target: MergeSide,
): string {
  return t(key, {
    source: mergeSideLabel(t, source),
    target: mergeSideLabel(t, target),
  });
}

export function MergeAction<Survivor extends { id: string }>({
  label,
  sourceId,
  sourceName,
  sourceDetail,
  searchTargets,
  merge,
  invalidate,
  recordKey,
  survivorRoute,
  disabledReasonId,
  requires,
}: Readonly<{
  label: string;
  sourceId: string;
  sourceName: string;
  // The same detail line the picker draws under each candidate.
  sourceDetail?: string;
  // May return the source row; it is dropped here, because a record is never
  // a merge target for itself.
  searchTargets: (q: string) => Promise<RecordPickerCandidate[]>;
  // POSTs the merge; the screen attaches ifMatch(sourceVersion) itself, so
  // this stays agnostic of the source record's shape. Returns the surviving
  // record.
  merge: (targetId: string) => Promise<Survivor>;
  invalidate: string;
  recordKey: string;
  survivorRoute: (targetId: string) => Route;
  // Why this merge is unavailable, when it is: the id of an element on the
  // page already carrying the sentence. STATE-4a settles the absent-vs-
  // disabled question by CAUSE — a merge blocked by the source record's
  // STATE, an archived row the server will not fold into anything, stays
  // visible and disabled WITH the reason, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
  // The grant the merge needs; required for the reason `ArchiveAction` gives.
  requires: RequiredGrant;
}>) {
  const t = useT();
  const refused = useGrantRefused(requires);
  const queryClient = useQueryClient();
  const toast = useToast();
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<RecordPickerCandidate | null>(null);
  const source = { name: sourceName, detail: sourceDetail };
  const searchOthers = useCallback(
    async (q: string) =>
      (await searchTargets(q)).filter((candidate) => candidate.id !== sourceId),
    [searchTargets, sourceId],
  );

  const mutation = useMutation({
    mutationFn: (targetId: string) => merge(targetId),
    onMutate: fromHere,
    // Closed onto the survivor: Back must not open the record merged away.
    onSuccess: (survivor, _targetId, from) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      queryClient.invalidateQueries({ queryKey: [recordKey, sourceId] });
      queryClient.invalidateQueries({ queryKey: [recordKey, survivor.id] });
      setOpen(false);
      if (target) {
        toast.show(mergeSentence(t, "merge.done", source, target));
      }
      from.closeOnto(survivorRoute(survivor.id));
    },
  });

  const close = () => {
    setOpen(false);
    setTarget(null);
    mutation.reset();
  };

  return (
    <>
      <Button
        reasonId={disabledReasonId}
        reason={refused ? t("record.actionRefused") : undefined}
        onClick={() => setOpen(true)}
        data-testid="merge-record"
      >
        {label}
      </Button>
      <Modal open={open} onClose={close} labelledBy={headingId} intent="form">
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {label}
        </Heading>
        <div className="form-stack">
          <p>{t("merge.pickTarget")}</p>
          <RecordPicker
            label={t("merge.searchPlaceholder")}
            searchTargets={searchOthers}
            onPick={setTarget}
            selected={target}
            // The server refuses a survivor the caller cannot write.
            // An omitted flag means not writable, so such a row says so here.
            unavailableReason={(candidate) =>
              candidate.writable !== true
                ? t("merge.targetReadOnly")
                : undefined
            }
            disabled={mutation.isPending}
          />
          {target && <p>{mergeSentence(t, "merge.confirm", source, target)}</p>}
          <ErrorLine error={mutation.error} />
        </div>
        <div className="actions">
          <Button onClick={close} disabled={mutation.isPending}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="danger"
            disabled={!target}
            pending={mutation.isPending}
            onClick={() => {
              if (target) {
                mutation.mutate(target.id);
              }
            }}
            data-testid="merge-confirm"
          >
            {t("merge.submit")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
