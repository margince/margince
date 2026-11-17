// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useEffectEvent, useId } from "react";
import { AddToShortlistAction } from "../../components/addtoshortlist";
import { ArchiveAction } from "../../components/archive";
import { OwnerPick } from "../../components/ownerpick";
import { useRailText } from "../../components/railfield";
import { Button } from "../../design-system/button";
import { useClipboardCopy } from "../../design-system/clipboardcopy";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import type { DealCardActions } from "../../design-system/dealcard";
import {
  FieldEditDialog,
  useFieldEditor,
} from "../../design-system/fieldeditdialog";
import { ListPopover } from "../../design-system/listpopover";
import { useCloseOverflowMenu } from "../../design-system/overflowmenu";
import { type useOwnToast, useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { appHref } from "../../lib/recordaddress";
import { routeHash } from "../../lib/router";
import type { Deal, Stage } from "./deals";
import { dealCloseDateField } from "./deals.fields";
import { archiveDeal, openDealEdit, saveDealEdit } from "./deals.write";

type CardVerbs = Pick<DealCardActions, "onSummary" | "onEmail" | "onAddTask">;
type RefusalSlot = ReturnType<typeof useOwnToast>;

// A board card's "…" items: the moves a drag makes, for a reader with no
// pointer to drag with, then the card's verbs and the record's.
export function DealCardMenu({
  deal,
  stages,
  verbs,
  onAdvance,
  copyRefusal,
}: Readonly<{
  deal: Deal;
  /** The pipeline's stages, in position order. */
  stages: readonly Stage[];
  verbs: CardVerbs;
  onAdvance: (dealId: string, stageId: string) => void;
  /** The board's one slot for a refused copy, so refusals never stack. */
  copyRefusal: RefusalSlot;
}>) {
  const t = useT();
  // An archived deal takes no change, so it keeps only what reads it. A seat
  // that may not change this deal is offered none of its writes, as its page.
  const live = !deal.archived_at;
  const canWrite = useCanWriteRecord("deal", deal);
  return (
    <>
      {live && canWrite && (
        <>
          <DealMoves deal={deal} stages={stages} onAdvance={onAdvance} />
          <DealCloseDate deal={deal} />
          <DealOwnerPick deal={deal} />
        </>
      )}
      {verbs.onSummary && (
        <Button onClick={verbs.onSummary}>{t("deal.card.summaryItem")}</Button>
      )}
      {verbs.onEmail && (
        <Button onClick={verbs.onEmail}>{t("compose.writeEmail")}</Button>
      )}
      {verbs.onAddTask && (
        <Button onClick={verbs.onAddTask}>{t("log.addTask")}</Button>
      )}
      <CopyDealLink dealId={deal.id} refusal={copyRefusal} />
      {live && (
        <>
          <AddToShortlistAction entityType="deal" entityId={deal.id} />
          {canWrite && <ArchiveDeal deal={deal} />}
        </>
      )}
    </>
  );
}

// A closed deal keeps its moves refused, as its page does; it is reopened there.
function DealMoves({
  deal,
  stages,
  onAdvance,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  onAdvance: (dealId: string, stageId: string) => void;
}>) {
  const t = useT();
  const reasonId = useId();
  const closeMenu = useCloseOverflowMenu();
  const refused = deal.status === "open" ? undefined : reasonId;
  // A pipeline with two won stages has no one stage "won" means.
  const closers = (["won", "lost"] as const).flatMap((semantic) => {
    const matching = stages.filter((stage) => stage.semantic === semantic);
    const only = matching.length === 1 ? matching[0] : undefined;
    return only && only.id !== deal.stage_id ? [{ semantic, stage: only }] : [];
  });
  const here = stages.findIndex((stage) => stage.id === deal.stage_id);
  const following = here < 0 ? undefined : stages[here + 1];
  const next = closers.some(({ stage }) => stage.id === following?.id)
    ? undefined
    : following;
  const move = (stageId: string) => onAdvance(deal.id, stageId);
  return (
    <>
      {refused && (
        <p id={reasonId} className="t-caption">
          {t("deal.card.closedReopen")}
        </p>
      )}
      {next && (
        <Button reasonId={refused} onClick={() => move(next.id)}>
          {t("deal.card.moveTo", { stage: next.name })}
        </Button>
      )}
      <ListPopover
        label={t("deals.bulkStage")}
        title={t("deals.bulkStage")}
        searchLabel={t("deals.bulkStagePick")}
        reasonId={refused}
        options={stages.map((stage) => ({ id: stage.id, name: stage.name }))}
        selected={deal.stage_id ?? undefined}
        onPick={(stage, done) => {
          done();
          closeMenu();
          if (stage.id !== deal.stage_id) {
            move(stage.id);
          }
        }}
      />
      {closers.map(({ semantic, stage }) => (
        <Button
          key={semantic}
          reasonId={refused}
          onClick={() => move(stage.id)}
        >
          {t(semantic === "won" ? "deal.card.markWon" : "deal.card.markLost")}
        </Button>
      ))}
    </>
  );
}

// The record page's own one-field save: its write, its refusals, its reads made stale.
function DealCloseDate({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const write = useRailText(
    {
      kind: "deal",
      record: openDealEdit(deal),
      fields: [dealCloseDateField],
      canEdit: true,
      save: (values, _rows, opened) =>
        saveDealEdit(opened, values, deal.masked_fields ?? []),
    },
    dealCloseDateField,
  );
  const { open, editing } = useFieldEditor(
    write.canEdit,
    write.onEditingChange,
  );
  return (
    <>
      <Button onClick={() => editing(true)}>
        {t("deal.card.changeCloseDate")}
      </Button>
      <FieldEditDialog
        open={open}
        label={t(dealCloseDateField.label)}
        value={write.stored}
        onClose={() => editing(false)}
        onSave={write.onSave}
        onDirtyChange={write.onDirtyChange}
      >
        {({ control, draft, setDraft, saving }) => (
          <DateInput
            {...control}
            value={isoOrBlank(draft)}
            readOnly={saving}
            onChange={(event) => setDraft(event.target.value)}
          />
        )}
      </FieldEditDialog>
    </>
  );
}

function DealOwnerPick({ deal }: Readonly<{ deal: Deal }>) {
  const closeMenu = useCloseOverflowMenu();
  return (
    <OwnerPick
      record={openDealEdit(deal)}
      assign={(ownerId, opened) =>
        saveDealEdit(opened, { owner_id: ownerId }, deal.masked_fields ?? [])
      }
      invalidate="deals"
      recordKey="deal"
      onAssigned={closeMenu}
    />
  );
}

// The menu closes on the press, so both answers go to a toast, and a refusal
// stays until it is read. One refusal at a time: a new press withdraws it.
function CopyDealLink({
  dealId,
  refusal,
}: Readonly<{ dealId: string; refusal: RefusalSlot }>) {
  const t = useT();
  const toast = useToast();
  const link = `${globalThis.location.origin}${appHref(routeHash({ screen: "deals", id: dealId }))}`;
  const clipboard = useClipboardCopy(
    link,
    {
      copy: t("deal.card.copyLink"),
      copied: t("deal.card.linkCopied"),
      remedy: t("deal.card.copyFailed"),
    },
    () => toast.show(t("deal.card.linkCopied")),
  );
  const failed = clipboard.notice !== null;
  const refuse = useEffectEvent(() =>
    refusal.show(t("deal.card.copyFailed"), { tone: "danger", sticky: true }),
  );
  useEffect(() => {
    if (failed) {
      refuse();
    }
  }, [failed]);
  return (
    <Button
      onClick={() => {
        refusal.withdraw();
        clipboard.copy();
      }}
    >
      {t("deal.card.copyLink")}
    </Button>
  );
}

// Nothing to do once archived: the board stays, and the "deals" read the
// archive invalidates drops the card.
function ArchiveDeal({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  return (
    <ArchiveAction
      requires={{ object: "deal", action: "delete" }}
      archivable={deal.archivable ?? false}
      refusedReason={t("deal.archiveRefused")}
      label={t("deal.archive")}
      confirmText={t("deal.archiveConfirm")}
      archivedMessage={t("record.archiveDone", { name: deal.name })}
      archive={() => archiveDeal(deal)}
      invalidate="deals"
      recordKey="deal"
      onArchived={() => undefined}
    />
  );
}
