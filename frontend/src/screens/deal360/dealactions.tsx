// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This deal's VERBS — split out of deals.tsx, which is at its own line
// ceiling and may not grow, for the same readability reason DealOverviewPane
// and the deal's committee card were split out before it: the record-view render
// callback stays under the complexity ceiling.
//
// An archived deal is read-only (no edit/archive/advance path exists
// server-side for a non-live row), so its verbs render REFUSED rather than
// missing: the page's one sentence about the archive says why, and each of
// them points at it (STATE-4a). A missing control says nothing about the
// deal, while a refused one names the reason.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CheckSquare, FileText } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { useCanWrite } from "../../app/capability";
import { navigate } from "../../app/router";
import { Button, Modal, OverflowMenu } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { dealRecordKeys } from "../activitykeys";
import { ArchiveAction } from "../archive";
import { problemMessageOf, throwProblem, useMe } from "../common";
import { LogActivityAction } from "../logactivity";
import { RecordEmailVerb } from "../recordemail";
import { ShareAction } from "../share";
import { useDealCoverage } from "./usedealcoverage";
import { useDealRecipientAddress } from "./usedealrecipient";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// The shared Email verb every record header carries.
function DealEmailVerb({
  deal,
  disabledReasonId,
}: Readonly<{ deal: Deal; disabledReasonId?: string }>) {
  // The same coverage read the readings band and the coverage card already
  // make, served from one cache entry — so asking here costs no request.
  const coverage = useDealCoverage(deal.id);
  const recordAddress = useDealRecipientAddress(coverage);
  return (
    <RecordEmailVerb
      entityType="deal"
      entityId={deal.id}
      // Who a FIRST message on this deal goes to: the champion, else somebody
      // the deal is actually in conversation with, else the first seat. Only
      // ever an offer — the composer fills an empty To field once and never
      // over what the reader typed.
      recordAddress={recordAddress}
      disabledReasonId={disabledReasonId}
    />
  );
}

// Reopens a won/lost deal back to an open-semantic stage — the same advance
// mutation shape the board drag uses, with status:"open" forced. Split out
// of DealActions for the same readability reason as the other header actions.
function ReopenAction({
  dealId,
  dealVersion,
  openStages,
  disabledReasonId,
}: Readonly<{
  dealId: string;
  // The version the header this button sits in was rendered from, so the reopen
  // pins the deal the reader was looking at. Stated by the caller rather than
  // read here: this action holds no query of its own to read a fresh one from,
  // and a fresh one would be the wrong answer anyway.
  dealVersion: number | undefined;
  openStages: Stage[];
  // The id of the sentence saying why this reopen is refused, when it is.
  // STATE-4a: a control blocked by the record's STATE rather than by a
  // permission stays visible and says why, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [stageId, setStageId] = useState<string | null>(null);
  const reopen = useMutation({
    mutationKey: ["deal-edit", dealId],
    // Stage and version both ride the variables: a version read out of the
    // closure would be the one from the render before this dialog opened, and a
    // reopen that pins the wrong version either fails for no reason the reader
    // can see or lands on a deal somebody else has since moved.
    mutationFn: async (input: {
      toStageId: string;
      version: number | undefined;
    }) => {
      const { data, error } = await api.POST("/deals/{id}/advance", {
        params: {
          path: { id: dealId },
          ...ifMatch(requireVersion(input.version)),
        },
        body: { to_stage_id: input.toStageId, status: "open" },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      setOpen(false);
      for (const queryKey of dealRecordKeys(dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  return (
    <>
      <Button
        reasonId={disabledReasonId}
        data-testid="reopen-open"
        onClick={() => setOpen(true)}
      >
        {t("deal.reopen")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy="reopen-title"
      >
        <p className="t-sub" id="reopen-title">
          {t("deal.reopenPick")}
        </p>
        <div
          style={{
            display: "flex",
            gap: "var(--space-2)",
            flexWrap: "wrap",
            margin: "var(--space-3) 0",
          }}
        >
          {openStages.map((s) => (
            <Button
              key={s.id}
              aria-pressed={stageId === s.id}
              data-testid={`reopen-stage-${s.id}`}
              onClick={() => setStageId(s.id)}
            >
              {s.name}
            </Button>
          ))}
        </div>
        {reopen.isError && (
          <p
            // The sentence arrives after the press, so it is announced: a
            // reader who cannot see the dialog change otherwise learns the
            // reopen failed only by tabbing back over it.
            role="alert"
            style={{ color: "var(--dangerText)" }}
          >
            {problemMessageOf(reopen.error, t)}
          </p>
        )}
        <div className="actions">
          <Button onClick={() => setOpen(false)}>{t("deals.cancel")}</Button>
          <Button
            variant="primary"
            data-testid="reopen-confirm"
            // A write in flight is `pending`, never `disabled`: the two mean
            // different things and Button keeps the focused control reachable
            // for the first. Spelled as disabled, the browser drops focus from
            // the button the reader just pressed.
            disabled={!stageId}
            pending={reopen.isPending}
            onClick={() => {
              if (stageId) {
                reopen.mutate({ toStageId: stageId, version: dealVersion });
              }
            }}
          >
            {t("deal.reopenConfirm")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

export function DealActions({
  deal,
  openStages,
  refusedReasonId,
}: Readonly<{
  deal: Deal;
  openStages: Stage[];
  // The id of the page's one sentence about why this deal takes no changes —
  // archived, or not this caller's to write — and undefined while it does.
  // Every verb the sentence refuses points at that one element instead of
  // printing the same line four times.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const refusedByArchive = deal.archived_at ? refusedReasonId : undefined;
  // The log/task drawer's own state: one drawer with two doors, the same
  // shape companyheaderactions.tsx and leads.tsx keep for the identical pair.
  const [drawer, setDrawer] = useState<"log" | "task" | null>(null);
  // useCanWrite, not useCan: the two log verbs issue a POST, and a read seat
  // is refused before RBAC is consulted, the rule every other record header
  // states for the identical verb.
  const me = useMe();
  const canLog = useCanWrite("activity", "create");
  const logRefusedId = useId();
  // A guard that has not answered yet refuses nothing: claiming a refusal
  // `/me` has not decided is worse than a control that is briefly quiet.
  const logGrantKnown = me.data?.authorization !== undefined;
  const logRefused =
    refusedByArchive ?? (logGrantKnown && !canLog ? logRefusedId : undefined);
  const logPending = !refusedByArchive && !logGrantKnown;
  return (
    <>
      {!refusedByArchive && logGrantKnown && !canLog && (
        <p id={logRefusedId}>{t("record.logActivityRefused")}</p>
      )}
      {/* Mail first, then the hairline, then what the reader records about
          this deal: the same two groups, in the same order, that the contact,
          company and lead headers draw. A rep moving between records must not
          have to re-find the verb they just used on the last one. */}
      <DealEmailVerb deal={deal} disabledReasonId={refusedByArchive} />
      {/* A hairline between reaching the record and recording what happened
          to it: two groups of verbs, not one toolbar. */}
      <span className="record-actions-sep" aria-hidden="true" />
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("log")}
      >
        <FileText aria-hidden="true" /> {t("log.title")}
      </Button>
      {/* Keeps its words rather than a glyph alone: a tick box is the mark
          for COMPLETING a task, so squaring this one would name the opposite
          of what it does. */}
      <Button
        disabled={logPending}
        reasonId={logRefused}
        onClick={() => setDrawer("task")}
      >
        <CheckSquare aria-hidden="true" /> {t("log.addTask")}
      </Button>
      {drawer && (
        <LogActivityAction
          entityType="deal"
          entityId={deal.id}
          askedKind={drawer === "task" ? "task" : undefined}
          triggerLabel={drawer === "task" ? "log.addTask" : undefined}
          openOnMount
          onClose={() => setDrawer(null)}
        />
      )}
      {/* Behind the overflow, every verb but the mail: editing a deal,
          handing a link to somebody outside the workspace, reopening a
          closed one and archiving it each want a whole line rather than a
          place in a row — the header carries identity and the one verb a
          reader reaches for. Archive goes last, farthest from the press
          that opened the menu. */}
      <OverflowMenu label={t("record.moreActions")}>
        {/* Worded rather than a bare pencil: among named verbs the square
            would be the one row naming nothing. */}
        <ShareAction
          recordType="deal"
          recordId={deal.id}
          disabledReasonId={refusedReasonId}
        />
        {/* Reopen answers a CLOSED deal, so an open one has no reason to be
            told about it — absent, not refused. An archived closed deal keeps
            it, refused: the reader came asking whether this can come back. */}
        {(deal.status === "won" || deal.status === "lost") && (
          <ReopenAction
            dealId={deal.id}
            dealVersion={deal.version}
            openStages={openStages}
            disabledReasonId={refusedReasonId}
          />
        )}
        <ArchiveAction
          disabledReasonId={refusedReasonId}
          label={t("deal.archive")}
          confirmText={t("deal.archiveConfirm")}
          archivedMessage={t("record.archiveDone", { name: deal.name })}
          archive={async () => {
            const { data, error } = await api.DELETE("/deals/{id}", {
              params: {
                path: { id: deal.id },
                ...ifMatch(requireVersion(deal.version)),
              },
            });
            if (error) {
              throwProblem(error);
            }
            return data;
          }}
          invalidate="deals"
          recordKey="deal"
          onArchived={() => navigate({ screen: "deals" })}
        />
      </OverflowMenu>
    </>
  );
}
