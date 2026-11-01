// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import {
  EntityRef,
  RosterFailedLine,
  rosterOwnerName,
  useRoster,
} from "../../components/entityref";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { type FilterPill, FilterPills } from "../../design-system/filterpills";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { formatDate } from "../../format/format";
import { useNow } from "../../format/now";
import { viewerZone } from "../../format/timezone";
import { type Translator, useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useMemberName } from "../../lib/membernames";
import { problemMessageOf } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { ControllerMailDialog } from "./controllermail";
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
import { NoticeDuty } from "./noticeduty";
import { ExcuseModal } from "./noticeexcuse";
import { SourceReviewDialog } from "./noticesource";

// NoticeCasesCard is the disclosure-duty queue: who we obtained without asking,
// whether anybody has told them, and by when we must.
export function NoticeCasesCard({
  caseId,
}: Readonly<{ caseId?: string }> = {}) {
  return caseId ? (
    <NoticeDuty key={caseId} caseId={caseId} />
  ) : (
    <NoticeQueueCard />
  );
}

function NoticeQueueCard() {
  const t = useT();
  // The only clock touching rendering, so a test drives a deadline through it.
  const nowMs = useNow(60_000);
  const [facet, setFacet] = useState<NoticeFacet>("owed");
  const [excusing, setExcusing] = useState<NoticeCase | null>(null);
  const [reviewing, setReviewing] = useState<NoticeCase | null>(null);
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

  const facetPills = useMemo((): FilterPill<NoticeFacet>[] => {
    const labels: Record<NoticeFacet, string> = {
      owed: t("notice.facetOwed"),
      all: t("notice.facetAll"),
      review: t("notice.source.title"),
    };
    return NOTICE_FACETS.map((value) => ({ value, label: labels[value] }));
  }, [t]);

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
            <FilterPills
              pills={facetPills}
              value={facet}
              onChange={(value) => {
                setReviewing(null);
                setFacet(value);
              }}
              label={t("notice.facetLabel")}
            />
          </div>
        )}
        {body}
      </PanelBody>
      {reviewing && (
        <SourceReviewDialog
          key={reviewing.id}
          duty={reviewing}
          open
          onClose={() => setReviewing(null)}
        />
      )}
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
              onReview={setReviewing}
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
      <SendNoticeDialog
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

// Sending reaches the contact's own inbox, so the rep reads the mail first.
function SendNoticeDialog({
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
  return (
    <ControllerMailDialog
      contactId={duty?.contact_id ?? ""}
      kind="privacy_notice"
      open={duty !== null}
      onClose={onClose}
      onSent={(issued) => {
        onSent();
        toast.show(
          issued.queued
            ? t("notice.sentTo", { address: issued.delivered_to })
            : t("noticeDuty.notSent", { address: issued.delivered_to }),
          { tone: issued.queued ? "success" : "warning" },
        );
      }}
    />
  );
}

type DutyVerbs = Readonly<{
  canWork: boolean;
  canSend: boolean;
  assignPending: boolean;
  onAssign: (id: string, owner: string) => void;
  onExcuse: (row: NoticeCase) => void;
  onSend: (row: NoticeCase) => void;
  onReview: (row: NoticeCase) => void;
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
          <span
            title={
              row.rule === "unassessed"
                ? undefined
                : noticeRuleHint(row.rule, t)
            }
          >
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
          <span>
            {row.due_at
              ? row.acquisition?.deadline_is_upper_bound
                ? t("notice.latestBy", {
                    date: formatDate(row.due_at, locale, tz),
                  })
                : formatDate(row.due_at, locale, tz)
              : t("notice.source.noDeadline")}
          </span>
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
    <CellStack>
      <Select
        aria-label={t("notice.ownerOf", { duty: dutyName(row, t) })}
        options={options}
        value={owner ?? ""}
        placeholder={t("notice.unassigned")}
        onChange={(next) => onAssign(row.id, next)}
        disabled={assignPending || roster.isPending}
        name={`notice-assign-${row.id}`}
      />
      <RosterFailedLine kind="user" roster={roster} />
    </CellStack>
  );
}

function DutyMenu({
  row,
  canWork,
  canSend,
  onExcuse,
  onSend,
  onReview,
}: Readonly<{ row: NoticeCase }> & DutyVerbs) {
  const t = useT();
  // A contact this reader cannot see is one the send route would refuse.
  const sendable =
    canSend &&
    row.contact_name != null &&
    !isNoticeResolved(row.state) &&
    (row.allowed_routes ?? []).includes("privacy_notice");
  const excusable = canWork && mayExcuse(row.state);
  const reviewable = canWork && row.state === "source_review";
  if (!sendable && !excusable && !reviewable) {
    return null;
  }
  return (
    <OverflowMenu label={t("table.rowActions", { name: dutyName(row, t) })}>
      {reviewable && (
        <Button onClick={() => onReview(row)}>
          {t("notice.source.review")}
        </Button>
      )}
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
