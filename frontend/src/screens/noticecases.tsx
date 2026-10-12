// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { api } from "../api/client";
import { useCan, useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
  SegmentedControl,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { formatDate } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, useT } from "../i18n";
import {
  LoadMoreButton,
  problemMessageOf,
  QueryGate,
  QueryStates,
  unwrap,
  useMe,
} from "./common";
import { EntityRef, rosterOwnerName, useRoster } from "./entityref";
import { useMemberName } from "./membernames";
import {
  acquisitionCaption,
  noticeRuleHint,
  noticeRuleLabel,
} from "./noticeacquisition";
import {
  isDefaultNoticeState,
  isNoticeOverdue,
  isNoticeResolved,
  mayAssign,
  mayExcuse,
  NOTICE_STATE_LABEL,
  type NoticeCase,
  noticeStateTone,
} from "./noticecases.logic";
import {
  NOTICE_FACETS,
  type NoticeFacet,
  useAssignDuty,
  useExcuseDuty,
  useNoticeQueue,
} from "./noticecases.queries";
import { ExcuseModal } from "./noticeexcuse";

// NoticeCasesCard is the disclosure-duty queue: who we obtained without asking,
// whether anybody has told them, and by when we must.
export function NoticeCasesCard() {
  const t = useT();
  // The only clock touching rendering, so a test drives a deadline through it.
  const nowMs = useNow(60_000);
  const [facet, setFacet] = useState<NoticeFacet>("owed");
  const [excusing, setExcusing] = useState<NoticeCase | null>(null);
  const [sending, setSending] = useState<NoticeCase | null>(null);
  const queryClient = useQueryClient();

  // `privacy_request:read`, which is what consent/noticeownership.go asks for.
  const canSee = useCan("privacy_request", "read");
  const canWork = useCanWrite("privacy_request", "update");
  // Sending writes the contact's consent record, a grant of its own.
  const canSend = useCanWrite("contact", "update");
  // The probe itself: capability predicates read false while /me is in flight.
  const me = useMe();

  const query = useNoticeQueue(facet, canSee);

  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["notice-cases"] });
  };
  const assign = useAssignDuty(invalidate);
  const excuse = useExcuseDuty(() => {
    invalidate();
    setExcusing(null);
  });

  const facetLabels = useMemo(
    (): Record<NoticeFacet, string> => ({
      owed: t("notice.facetOwed"),
      all: t("notice.facetAll"),
    }),
    [t],
  );

  let body: ReactNode = null;
  if (!canSee) {
    // Withheld rather than absent: an absent card would read as "no duties".
    body = (
      <QueryGate query={me} pendingLabel={t("notice.readOnlyForPrivacy")}>
        {() => <EmptyState>{t("notice.readOnlyForPrivacy")}</EmptyState>}
      </QueryGate>
    );
  } else if (!query.isSuccess || rows.length === 0) {
    body = (
      <QueryStates query={query} pendingLabel={t("notice.loading")}>
        <EmptyState>
          {facet === "owed" ? t("notice.emptyOwed") : t("common.empty")}
        </EmptyState>
      </QueryStates>
    );
  }

  return (
    <Panel title={t("notice.title")}>
      <PanelBody>
        <PanelIntro>{t("notice.sub")}</PanelIntro>
        {canSee && (
          <div className="filter-tabs">
            <SegmentedControl
              options={[...NOTICE_FACETS]}
              value={facet}
              onChange={setFacet}
              labels={facetLabels}
              label={t("notice.facetLabel")}
            />
          </div>
        )}
        {body}
      </PanelBody>
      <CardBoundary>
        {body === null && (
          <>
            <NoticeTable
              rows={rows}
              nowMs={nowMs}
              canWork={canWork}
              canSend={canSend}
              assignPending={assign.isPending}
              onAssign={(id, owner) => assign.mutate({ id, owner })}
              onExcuse={setExcusing}
              onSend={setSending}
            />
            {query.hasNextPage && (
              <PanelBody>
                <LoadMoreButton query={query} />
              </PanelBody>
            )}
          </>
        )}
      </CardBoundary>
      {/* Both write failures land on the card too: the modal can be dismissed
          while its submit is still in flight. */}
      <DutyRefusals
        assignError={assign.error}
        excuseError={excusing === null ? excuse.error : null}
      />
      <ExcuseModal
        // Keyed by the case, so a second duty mounts a fresh form rather than
        // carrying the first one's chosen kind and typed ground.
        key={excusing?.id ?? "none"}
        open={excusing !== null}
        onClose={() => setExcusing(null)}
        onConfirm={(state, note) => {
          if (excusing) {
            excuse.mutate({ id: excusing.id, state, note });
          }
        }}
        pending={excuse.isPending}
        error={excuse.isError ? problemMessageOf(excuse.error, t) : null}
      />
      <SendNoticeModal
        key={`send-${sending?.id ?? "none"}`}
        duty={sending}
        onClose={() => setSending(null)}
        onSent={invalidate}
      />
    </Panel>
  );
}

function DutyRefusals({
  assignError,
  excuseError,
}: Readonly<{ assignError: unknown; excuseError: unknown }>) {
  if (!assignError && !excuseError) {
    return null;
  }
  return (
    <PanelBody>
      <ErrorLine error={assignError} />
      <ErrorLine error={excuseError} />
    </PanelBody>
  );
}

// Sending reaches the contact's own inbox, so it asks first.
function SendNoticeModal({
  duty,
  onClose,
  onSent,
}: Readonly<{
  duty: NoticeCase | null;
  onClose: () => void;
  onSent: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const send = useMutation({
    mutationFn: async (contactId: string) => {
      return unwrap(
        await api.POST("/contacts/{id}/consent/privacy-notice", {
          params: { path: { id: contactId } },
        }),
      );
    },
    onSuccess: (issued) => {
      onSent();
      onClose();
      toast.show(
        issued.queued
          ? t("notice.sentTo", { address: issued.delivered_to })
          : t("noticeDuty.notSent", { address: issued.delivered_to }),
        { tone: issued.queued ? "success" : "warning" },
      );
    },
  });
  return (
    <ConfirmModal
      open={duty !== null}
      onClose={onClose}
      title={t("notice.sendTitle")}
      confirmLabel={t("notice.sendConfirm")}
      onConfirm={() => duty && send.mutate(duty.contact_id)}
      pending={send.isPending}
      error={send.isError ? problemMessageOf(send.error, t) : null}
    >
      <p>{t("notice.sendBody", { contact: duty?.contact_name ?? "" })}</p>
    </ConfirmModal>
  );
}

type DutyVerbs = Readonly<{
  canWork: boolean;
  canSend: boolean;
  assignPending: boolean;
  onAssign: (id: string, owner: string) => void;
  onExcuse: (row: NoticeCase) => void;
  onSend: (row: NoticeCase) => void;
}>;

function NoticeTable({
  rows,
  nowMs,
  ...verbs
}: Readonly<{ rows: NoticeCase[]; nowMs: number }> & DutyVerbs) {
  const t = useT();
  const { locale } = useLocale();
  // A statutory deadline, read on the viewer's own calendar day.
  const tz = viewerZone();
  const columns: DataTableColumn<NoticeCase>[] = [
    {
      key: "contact",
      header: t("notice.contact"),
      grow: true,
      render: (row) => (
        <CellStack>
          {row.contact_name ? (
            <EntityRef
              kind="contact"
              id={row.contact_id}
              name={row.contact_name}
            />
          ) : (
            // A hidden contact and an erased one both arrive nameless.
            <span className="t-caption">{t("notice.recordUnavailable")}</span>
          )}
          <span className="t-caption">
            {acquisitionCaption(row.acquisition, t, locale, tz)}
          </span>
        </CellStack>
      ),
    },
    {
      key: "duty",
      header: t("notice.duty"),
      render: (row) => (
        <CellStack>
          <span title={noticeRuleHint(row.rule, t)}>
            {noticeRuleLabel(row.rule, t)}
          </span>
          {!isDefaultNoticeState(row.state) && (
            <Badge tone={noticeStateTone(row.state)}>
              {t(NOTICE_STATE_LABEL[row.state])}
            </Badge>
          )}
          {row.resolution_note && (
            <span className="t-caption">{row.resolution_note}</span>
          )}
        </CellStack>
      ),
    },
    {
      key: "due",
      header: t("notice.due"),
      render: (row) => (
        <CellStack>
          <span>{formatDate(row.due_at, locale, tz)}</span>
          {isNoticeOverdue(row.due_at, row.state, nowMs) && (
            <Badge tone="danger">{t("notice.overdue")}</Badge>
          )}
        </CellStack>
      ),
    },
    {
      key: "owner",
      header: t("notice.owner"),
      render: (row) => <OwnerCell row={row} {...verbs} />,
    },
  ];
  if (verbs.canWork || verbs.canSend) {
    columns.push({
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (row) => <DutyMenu row={row} {...verbs} />,
    });
  }
  return (
    <DataTable
      label={t("notice.title")}
      bleed
      fold
      columns={columns}
      rows={rows}
      rowKey={(row) => row.id}
    />
  );
}

function dutyName(row: NoticeCase, t: Translator): string {
  const rule = noticeRuleLabel(row.rule, t);
  return row.contact_name
    ? t("notice.dutyFor", { rule, contact: row.contact_name })
    : rule;
}

function OwnerCell({
  row,
  canWork,
  assignPending,
  onAssign,
}: Readonly<{ row: NoticeCase }> & DutyVerbs) {
  const t = useT();
  const editable = canWork && mayAssign(row.state);
  // Only a seat that can act on the roster fetches it.
  const roster = useRoster("user", editable);
  const offered = useMemo(
    () =>
      (roster.data ?? []).flatMap((entry) =>
        "display_name" in entry
          ? [{ value: entry.id, label: entry.display_name }]
          : [],
      ),
    [roster.data],
  );
  const owner = row.owner_user_id ?? null;
  const unoffered =
    editable && owner !== null && !offered.some((o) => o.value === owner);
  const ownerName = useMemberName(unoffered ? owner : null);
  if (!editable) {
    return owner ? (
      <EntityRef kind="user" id={owner} />
    ) : (
      <span className="t-caption">{t("notice.unassigned")}</span>
    );
  }
  // An owner the roster does not offer still names the face, or an assigned
  // duty would read as unassigned.
  const options = unoffered
    ? [
        {
          value: owner,
          label: rosterOwnerName(owner, ownerName, t, t("ref.notInRoster")),
          disabled: true,
        },
        ...offered,
      ]
    : offered;
  return (
    <Select
      aria-label={t("notice.ownerOf", { duty: dutyName(row, t) })}
      options={options}
      value={owner ?? ""}
      placeholder={t("notice.unassigned")}
      onChange={(next) => onAssign(row.id, next)}
      disabled={assignPending || roster.isPending}
      name={`notice-assign-${row.id}`}
    />
  );
}

function DutyMenu({
  row,
  canWork,
  canSend,
  onExcuse,
  onSend,
}: Readonly<{ row: NoticeCase }> & DutyVerbs) {
  const t = useT();
  // A contact this reader cannot see is one the send route would refuse.
  const sendable =
    canSend &&
    row.contact_name != null &&
    !isNoticeResolved(row.state) &&
    (row.allowed_routes ?? []).includes("privacy_notice");
  const excusable = canWork && mayExcuse(row.state);
  if (!sendable && !excusable) {
    return null;
  }
  return (
    <OverflowMenu label={t("table.rowActions", { name: dutyName(row, t) })}>
      {sendable && (
        <Button aria-haspopup="dialog" onClick={() => onSend(row)}>
          {t("noticeDuty.sendNotice")}
        </Button>
      )}
      {excusable && (
        <Button aria-haspopup="dialog" onClick={() => onExcuse(row)}>
          {t("notice.excuse")}
        </Button>
      )}
    </OverflowMenu>
  );
}
