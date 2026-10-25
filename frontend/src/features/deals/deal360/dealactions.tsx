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
import { CalendarPlus, CheckSquare, FileText, Phone } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import { ArchiveAction } from "../../../components/archive";
import { ExtensionEntries } from "../../../components/extensionentries";
import { LogActivityAction } from "../../../components/logactivity.action";
import { searchCompanyContacts } from "../../../components/meetingattendees";
import { Button, OverflowMenu } from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { ErrorLine } from "../../../design-system/errorline";
import { VerbWord } from "../../../design-system/iconaction";
import { ListPopover } from "../../../design-system/listpopover";
import { useT } from "../../../i18n";
import { dealRecordKeys } from "../../../lib/activitykeys";
import { useCanWrite } from "../../../lib/capability";
import { unwrap } from "../../../lib/problem";
import { navigate } from "../../../lib/router";
import { useMe } from "../../../lib/session";
import { RecordEmailVerb } from "../../records/recordemail";
import { ShareAction } from "../../records/share";
import { CorrectCloseDayAction } from "./correctcloseday";
import { useDealCoverage } from "./usedealcoverage";
import { useDealRecipientAddress } from "./usedealrecipient";
import "./deal360.css";
import { AddToShortlistAction } from "../../../components/addtoshortlist";
import { RequestLegalReviewAction } from "./deallegaltab";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// The shared Email verb every record header carries.
function DealEmailVerb({
  deal,
  disabledReasonId,
}: Readonly<{ deal: Deal; disabledReasonId?: string }>) {
  // The same coverage read the readings band and the coverage card already
  // make, served from one cache entry — so asking here costs no request.
  const coverage = useDealCoverage(deal.id, !deal.archived_at);
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
      return unwrap(
        await api.POST("/deals/{id}/advance", {
          params: {
            path: { id: dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { to_stage_id: input.toStageId, status: "open" },
        }),
        t,
      );
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
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("deal.reopenPick")}
        confirmLabel={t("deal.reopenConfirm")}
        confirmDisabled={!stageId}
        pending={reopen.isPending}
        onConfirm={() => {
          if (stageId) {
            reopen.mutate({ toStageId: stageId, version: dealVersion });
          }
        }}
      >
        <div className="d360-reopen-stages">
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
        <ErrorLine error={reopen.error} />
      </ConfirmModal>
    </>
  );
}

export function DealActions({
  deal,
  openStages,
  refusedReasonId,
  onEditDetails,
}: Readonly<{
  deal: Deal;
  openStages: Stage[];
  // Opens the drawer that holds every field of the deal and its editors.
  onEditDetails: () => void;
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
  const [drawer, setDrawer] = useState<"log" | "task" | "call" | null>(null);
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
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={t("log.kindCall")}
        onClick={() => setDrawer("call")}
      >
        <Phone aria-hidden="true" />
        <VerbWord word={t("record.verb.call")} label={t("log.kindCall")} />
      </Button>
      <DealBookMeeting
        companyId={deal.company_id ?? undefined}
        allowed={logGrantKnown && canLog && !refusedByArchive}
        pending={logPending}
        reasonId={logRefused}
      />
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={t("log.title")}
        onClick={() => setDrawer("log")}
      >
        <FileText aria-hidden="true" />
        <VerbWord word={t("record.verb.log")} label={t("log.title")} />
      </Button>
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={t("log.addTask")}
        onClick={() => setDrawer("task")}
      >
        <CheckSquare aria-hidden="true" />
        <VerbWord word={t("record.verb.task")} label={t("log.addTask")} />
      </Button>
      <ExtensionEntries
        slot="record_header.deal"
        record={{ type: "deal", id: deal.id }}
      />
      {drawer && (
        <LogActivityAction
          entityType="deal"
          entityId={deal.id}
          askedKind={drawer === "log" ? undefined : drawer}
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
      <OverflowMenu
        label={t("record.moreActions")}
        word={t("record.verb.more")}
      >
        {/* Worded rather than a bare pencil: among named verbs the square
            would be the one row naming nothing. */}
        <Button onClick={onEditDetails}>{t("deal.editDetails")}</Button>
        <ShareAction
          recordType="deal"
          recordId={deal.id}
          disabledReasonId={refusedReasonId}
        />
        <AddToShortlistAction entityType="deal" entityId={deal.id} />
        <RequestLegalReviewAction deal={deal} />
        {/* Reopen answers a CLOSED deal, so an open one has no reason to be
            told about it — absent, not refused. An archived closed deal keeps
            it, refused: the reader came asking whether this can come back. */}
        {(deal.status === "won" || deal.status === "lost") && (
          <>
            <CorrectCloseDayAction
              deal={deal}
              disabledReasonId={refusedReasonId}
            />
            <ReopenAction
              dealId={deal.id}
              dealVersion={deal.version}
              openStages={openStages}
              disabledReasonId={refusedReasonId}
            />
          </>
        )}
        <ArchiveAction
          requires={{ object: "deal", action: "delete" }}
          disabledReasonId={refusedReasonId}
          label={t("deal.archive")}
          confirmText={t("deal.archiveConfirm")}
          archivedMessage={t("record.archiveDone", { name: deal.name })}
          archive={async () => {
            return unwrap(
              await api.DELETE("/deals/{id}", {
                params: {
                  path: { id: deal.id },
                  ...ifMatch(requireVersion(deal.version)),
                },
              }),
            );
          }}
          invalidate="deals"
          recordKey="deal"
          onArchived={() => navigate({ screen: "deals" })}
        />
      </OverflowMenu>
    </>
  );
}

// Meet books with one of the account's contacts, through that contact's own
// booking screen. A deal on no company has nobody to book with yet.
function DealBookMeeting({
  companyId,
  allowed,
  pending,
  reasonId,
}: Readonly<{
  companyId?: string;
  allowed: boolean;
  pending: boolean;
  reasonId?: string;
}>) {
  const t = useT();
  const face = (
    <>
      <CalendarPlus aria-hidden="true" />
      <VerbWord
        word={t("record.verb.meet")}
        label={t("scheduling.bookContact")}
      />
    </>
  );
  if (!allowed || !companyId) {
    return (
      <Button
        disabled={pending || !companyId}
        reasonId={reasonId}
        title={t("scheduling.bookContact")}
      >
        {face}
      </Button>
    );
  }
  return (
    <ListPopover
      label={face}
      title={t("scheduling.bookContact")}
      searchLabel={t("scheduling.bookPickContact")}
      search={(term) => searchCompanyContacts(companyId, term)}
      onPick={(contact, done) => {
        done();
        navigate({ screen: "book", id: `contact-${contact.id}` });
      }}
    />
  );
}
