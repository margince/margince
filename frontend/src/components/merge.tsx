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
import { useT } from "../i18n";
import { type RequiredGrant, useGrantRefused } from "../lib/capability";
import { navigate, type Route } from "../lib/router";
import "../design-system/querystates.css";

// The shared "Merge into…" affordance (P-2): a human direct call that folds
// this record (the source, A) into a picked survivor (B) — A is archived
// with merged_into_id=B, B keeps the id the rest of the CRM already points
// at. Contact and Company 360s have an identical merge shape (target_id body
// + If-Match precondition, survivor Contact/Company back), so this stays
// resource-agnostic: the screen supplies the search transport, the merge
// transport, and where the survivor's 360 lives.

export function MergeAction<Survivor extends { id: string }>({
  label,
  sourceId,
  sourceName,
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
  const headingId = useId();
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<RecordPickerCandidate | null>(null);
  const searchOthers = useCallback(
    async (q: string) =>
      (await searchTargets(q)).filter((candidate) => candidate.id !== sourceId),
    [searchTargets, sourceId],
  );

  const mutation = useMutation({
    mutationFn: (targetId: string) => merge(targetId),
    onSuccess: (survivor) => {
      queryClient.invalidateQueries({ queryKey: [invalidate] });
      queryClient.invalidateQueries({ queryKey: [recordKey, sourceId] });
      queryClient.invalidateQueries({ queryKey: [recordKey, survivor.id] });
      setOpen(false);
      navigate(survivorRoute(survivor.id));
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
            disabled={mutation.isPending}
          />
          {target && (
            <p>
              {target.detail
                ? t("merge.confirmDetail", {
                    source: sourceName,
                    target: target.name,
                    detail: target.detail,
                  })
                : t("merge.confirm", {
                    source: sourceName,
                    target: target.name,
                  })}
            </p>
          )}
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
