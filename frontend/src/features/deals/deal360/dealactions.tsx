// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This deal's VERBS — split out of deals.tsx, which is at its own line
// ceiling and may not grow, for the same readability reason DealOverviewPane
// and the deal's committee card were split out before it: the record-view render
// callback stays under the complexity ceiling.
//
// An archived deal's verbs stay, refused, and point at the page's sentence
// about the archive (STATE-4a). A deal this reader may not write is refused by
// permission, not state, so the verbs that change it are left out.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarPlus, FileText, Phone } from "lucide-react";
import { useCallback, useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { ifMatch, requireVersion } from "../../../api/version";
import {
  type ActivityGate,
  useActivityGate,
} from "../../../components/activitygate";
import { ArchiveAction } from "../../../components/archive";
import { ExtensionEntries } from "../../../components/extensionentries";
import { LogActivityAction } from "../../../components/logactivity.action";
import { searchCompanyContacts } from "../../../components/meetingattendees";
import { Button, OverflowMenu } from "../../../design-system/atoms";
import { tipText, VerbWord } from "../../../design-system/iconaction";
import { ListPopover } from "../../../design-system/listpopover";
import { useToast } from "../../../design-system/toast";
import { useLocale, useT } from "../../../i18n";
import { dealStageMoveKeys } from "../../../lib/activitykeys";
import { useCanWriteRecord } from "../../../lib/capability";
import { unwrap } from "../../../lib/problem";
import { navigate } from "../../../lib/router";
import { useShortcut } from "../../../lib/shortcuts";
import { RecordEmailVerb } from "../../records/recordemail";
import { ShareAction } from "../../records/share";
import { advanceNote } from "../dealprobability";
import { archiveDeal } from "../deals.write";
import { CorrectCloseDayAction } from "./correctcloseday";
import { useDealCoverage } from "./usedealcoverage";
import { useDealRecipientAddress } from "./usedealrecipient";
import "./deal360.css";
import { AddToShortlistAction } from "../../../components/addtoshortlist";
import { RequestLegalReviewAction } from "./deallegaltab";
import { MovePipelineAction } from "./movepipeline";
import { ReopenModal } from "./reopenmodal";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// The shared Email verb every record header carries.
function DealEmailVerb({
  deal,
  disabledReasonId,
  reasonTip,
  disabled,
  keyShortcuts,
  open,
  onOpenChange,
}: Readonly<{
  deal: Deal;
  disabledReasonId?: string;
  reasonTip?: string;
  disabled?: boolean;
  keyShortcuts?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}>) {
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
      reasonTip={reasonTip}
      disabled={disabled}
      keyShortcuts={keyShortcuts}
      open={open}
      onOpenChange={onOpenChange}
    />
  );
}

// The sentence the header states when the seat may not log. An archive is
// the page's sentence, so only its id reaches here.
function activityRefusal(
  t: ReturnType<typeof useT>,
  refusedByArchive: string | undefined,
  activity: ActivityGate,
): string | undefined {
  return !refusedByArchive && activity.refusalKey
    ? t(activity.refusalKey)
    : undefined;
}

// Reopens a won/lost deal back to an open-semantic stage — the same advance
// mutation shape the board drag uses, with status:"open" forced. Split out
// of DealActions for the same readability reason as the other header actions.
function ReopenAction({
  dealId,
  dealVersion,
  openStages,
  pipelineRetired,
  disabledReasonId,
}: Readonly<{
  dealId: string;
  // The version the header this button sits in was rendered from, so the reopen
  // pins the deal the reader was looking at. Stated by the caller rather than
  // read here: this action holds no query of its own to read a fresh one from,
  // and a fresh one would be the wrong answer anyway.
  dealVersion: number | undefined;
  openStages: Stage[];
  // A retired pipeline takes no stage, so a closed deal on it cannot reopen.
  pipelineRetired?: boolean;
  // The id of the sentence saying why this reopen is refused, when it is.
  // STATE-4a: a control blocked by the record's STATE rather than by a
  // permission stays visible and says why, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const reopen = useMutation({
    mutationKey: ["deal-edit", dealId],
    // Stage and version both ride the variables: a version read out of the
    // closure would be the one from the render before this dialog opened, and a
    // reopen that pins the wrong version either fails for no reason the reader
    // can see or lands on a deal somebody else has since moved.
    mutationFn: async (input: {
      toStage: Stage;
      version: number | undefined;
    }) => {
      return unwrap(
        await api.POST("/deals/{id}/advance", {
          params: {
            path: { id: dealId },
            ...ifMatch(requireVersion(input.version)),
          },
          body: { to_stage_id: input.toStage.id, status: "open" },
        }),
        t,
      );
    },
    onSuccess: (advanced, input) => {
      setOpen(false);
      toast.show(
        advanceNote(
          advanced,
          { toStage: input.toStage, reopens: true },
          t,
          locale,
        ),
      );
      for (const queryKey of dealStageMoveKeys(dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  if (pipelineRetired) {
    return null;
  }
  return (
    <>
      <Button
        reasonId={disabledReasonId}
        data-testid="reopen-open"
        onClick={() => setOpen(true)}
      >
        {t("deal.reopen")}
      </Button>
      <ReopenModal
        open={open}
        onClose={() => setOpen(false)}
        openStages={openStages}
        pending={reopen.isPending}
        error={reopen.error}
        onConfirm={(toStage) =>
          reopen.mutate({ toStage, version: dealVersion })
        }
      />
    </>
  );
}

function DealArchiveAction({
  deal,
  refusedReasonId,
}: Readonly<{ deal: Deal; refusedReasonId?: string }>) {
  const t = useT();
  return (
    <ArchiveAction
      requires={{ object: "deal", action: "delete" }}
      archivable={deal.archivable ?? false}
      refusedReason={t("deal.archiveRefused")}
      disabledReasonId={refusedReasonId}
      label={t("deal.archive")}
      confirmText={t("deal.archiveConfirm")}
      archivedMessage={t("record.archiveDone", { name: deal.name })}
      archive={() => archiveDeal(deal)}
      invalidate="deals"
      recordKey="deal"
      onArchived={(from) => from.closeOnto({ screen: "deals" })}
    />
  );
}

export function DealActions({
  deal,
  openStages,
  pipelineRetired,
  refusedReasonId,
  onEditDetails,
}: Readonly<{
  deal: Deal;
  openStages: Stage[];
  pipelineRetired?: boolean;
  // Opens the drawer that holds every field of the deal and its editors.
  onEditDetails: () => void;
  // The id of the page's one sentence about why this deal takes no changes —
  // archived, or not this caller's to write — and undefined while it does.
  // Every verb the sentence refuses points at that one element instead of
  // printing the same line four times.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const canWrite = useCanWriteRecord("deal", deal);
  const refusedByArchive = deal.archived_at ? refusedReasonId : undefined;
  // The log/task drawer's own state: one drawer with two doors, the same
  // shape companyheaderactions.tsx and leads.tsx keep for the identical pair.
  const [drawer, setDrawer] = useState<"log" | "task" | "call" | null>(null);
  const activity = useActivityGate();
  const logRefused = refusedByArchive ?? activity.refusedId;
  const logRefusal = activityRefusal(t, refusedByArchive, activity);
  const logPending = !refusedByArchive && activity.pending;
  const [writing, setWriting] = useState(false);
  const logOpen = !logPending && !logRefused;
  const logKeys = useShortcut(
    "record.logActivity",
    () => setDrawer("log"),
    logOpen,
  );
  const taskKeys = useShortcut(
    "record.addTask",
    () => setDrawer("task"),
    logOpen,
  );
  const writeKeys = useShortcut(
    "record.writeEmail",
    () => setWriting(true),
    logOpen,
  );
  return (
    <>
      {logRefusal && <p id={activity.sentenceId}>{logRefusal}</p>}
      {/* Mail first, then the hairline, then what the reader records about
          this deal: the same two groups, in the same order, that the contact,
          company and lead headers draw. A rep moving between records must not
          have to re-find the verb they just used on the last one. */}
      <DealEmailVerb
        deal={deal}
        disabledReasonId={logRefused}
        reasonTip={logRefusal}
        disabled={logPending}
        open={writing}
        onOpenChange={setWriting}
        keyShortcuts={writeKeys}
      />
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={tipText(t("log.kindCall"), logRefusal)}
        onClick={() => setDrawer("call")}
      >
        <Phone aria-hidden="true" />
        <VerbWord word={t("record.verb.call")} label={t("log.kindCall")} />
      </Button>
      <DealBookMeeting
        dealId={deal.id}
        companyId={deal.company_id ?? undefined}
        allowed={!logPending && !logRefused}
        pending={logPending}
        reasonId={logRefused}
        reasonTip={logRefusal}
      />
      <Button
        disabled={logPending}
        reasonId={logRefused}
        title={tipText(t("log.title"), logRefusal)}
        onClick={() => setDrawer("log")}
        aria-keyshortcuts={logKeys}
      >
        <FileText aria-hidden="true" />
        <VerbWord word={t("record.verb.log")} label={t("log.title")} />
      </Button>
      <ExtensionEntries
        slot="record_header.deal"
        record={{ type: "deal", id: deal.id }}
      />
      <ExtensionEntries slot="record_mark.deal" />
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
        {/* First: the most frequent verb in the menu. */}
        <Button
          disabled={logPending}
          reasonId={logRefused}
          onClick={() => setDrawer("task")}
          aria-keyshortcuts={taskKeys}
        >
          {t("log.addTask")}
        </Button>
        {/* Worded rather than a bare pencil: among named verbs the square
            would be the one row naming nothing. */}
        {canWrite && (
          <Button onClick={onEditDetails}>{t("deal.editDetails")}</Button>
        )}
        {canWrite && (
          <ShareAction
            recordType="deal"
            recordId={deal.id}
            disabledReasonId={refusedReasonId}
          />
        )}
        <MovePipelineAction deal={deal} />
        <AddToShortlistAction entityType="deal" entityId={deal.id} />
        <RequestLegalReviewAction deal={deal} />
        {/* Reopen answers a CLOSED deal, so an open one has no reason to be
            told about it — absent, not refused. An archived closed deal keeps
            it, refused: the reader came asking whether this can come back. */}
        {canWrite && (deal.status === "won" || deal.status === "lost") && (
          <>
            <CorrectCloseDayAction
              deal={deal}
              disabledReasonId={refusedReasonId}
            />
            <ReopenAction
              dealId={deal.id}
              dealVersion={deal.version}
              openStages={openStages}
              pipelineRetired={pipelineRetired}
              disabledReasonId={refusedReasonId}
            />
          </>
        )}
        {canWrite && !deal.archived_at && (
          <DealArchiveAction deal={deal} refusedReasonId={refusedReasonId} />
        )}
      </OverflowMenu>
    </>
  );
}

// Meet books with one of the account's contacts, through that contact's own
// booking screen. A deal on no company has nobody to book with yet.
function DealBookMeeting({
  dealId,
  companyId,
  allowed,
  pending,
  reasonId,
  reasonTip,
}: Readonly<{
  dealId: string;
  companyId?: string;
  allowed: boolean;
  pending: boolean;
  reasonId?: string;
  /** The sentence `reasonId` names, for the verb's tooltip. */
  reasonTip?: string;
}>) {
  const t = useT();
  // Held across renders, because a new search function drops the answer the
  // open panel has already listed.
  const search = useCallback(
    (term: string) => searchCompanyContacts(companyId ?? "", term),
    [companyId],
  );
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
    // A refusal the page already states wins; otherwise the missing company
    // is the one thing standing between the reader and a booking.
    const noCompany = !reasonId && !companyId;
    const why = noCompany ? t("deal.meetNeedsCompany") : undefined;
    return (
      <Button
        disabled={pending || !companyId}
        reasonId={reasonId}
        reason={why}
        title={tipText(t("scheduling.bookContact"), reasonId ? reasonTip : why)}
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
      search={search}
      listOnOpen
      onPick={(contact, done) => {
        done();
        navigate(
          { screen: "book", id: `contact-${contact.id}` },
          new Map([["deal", dealId]]),
        );
      }}
    />
  );
}
