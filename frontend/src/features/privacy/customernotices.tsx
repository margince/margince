// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  TextInput,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { ChoiceList } from "../../design-system/choicelist";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Textarea } from "../../design-system/field";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { dueInstant, todayIn } from "../../format/calendarday";
import { isoOrBlank } from "../../format/datepattern";
import { formatDate } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import "./privacy.css";

type CustomerNotice = components["schemas"]["ExistingCustomerNotice"];

const NOTICES_KEY = ["existing-customer-notices"] as const;

type Draft = Readonly<{
  entity: string;
  group: string;
  channel: string;
  text: string;
  from: ISODate | "";
  until: ISODate | "";
  purposes: string[];
}>;

// The day a reader picks starts at midnight in their own zone; the end day is
// inclusive on screen and exclusive on the wire, so it is sent as the next
// day's first second.
function startOf(day: string, zone: string): string {
  return dueInstant(day, zone, "00:00");
}
function afterEndOf(day: string, zone: string): string {
  return new Date(
    new Date(dueInstant(day, zone)).getTime() + 1000,
  ).toISOString();
}

function useMarketingPurposes() {
  return useQuery({
    queryKey: ["consent-purposes"],
    queryFn: async () => unwrap(await api.GET("/consent-purposes")),
    select: (page) => page.data.filter((p) => p.class === "marketing"),
  });
}

function DeclareForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const zone = useViewerZone();
  const queryClient = useQueryClient();
  const formId = useId();
  const entities = useLegalEntities();
  const purposes = useMarketingPurposes();
  const live = (entities.data ?? []).filter((e) => !e.archived_at);
  const fallbackEntity = live.find((e) => e.is_default)?.id ?? "";
  const [draft, setDraft] = useState<Draft>({
    entity: "",
    group: "",
    channel: "",
    text: "",
    from: "",
    until: "",
    purposes: [],
  });
  const entity = draft.entity || fallbackEntity;
  const declare = useMutation({
    mutationFn: async (d: Draft) =>
      unwrap(
        await api.POST("/existing-customer-notices", {
          body: {
            legal_entity_id: entity,
            customer_group: d.group.trim(),
            collection_channel: d.channel.trim(),
            notice_text: d.text.trim(),
            effective_from: startOf(d.from, zone),
            effective_until: d.until ? afterEndOf(d.until, zone) : null,
            purpose_ids: d.purposes,
          },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: NOTICES_KEY });
      onDone();
    },
  });
  function edit(next: Partial<Draft>) {
    setDraft((was) => ({ ...was, ...next }));
    if (declare.isError) declare.reset();
  }
  const ready =
    entity !== "" &&
    draft.group.trim() !== "" &&
    draft.channel.trim() !== "" &&
    draft.text.trim() !== "" &&
    draft.from !== "" &&
    draft.purposes.length > 0;
  const marketing = purposes.data ?? [];
  return (
    <>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready && !declare.isPending) declare.mutate(draft);
        }}
      >
        {live.length > 1 && (
          <Field label={t("privacy.customerNoticeEntity")}>
            {(control) => (
              <Select
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                value={entity}
                onChange={(value) => edit({ entity: value })}
                options={live.map((e) => ({
                  value: e.id,
                  label: e.display_name,
                }))}
              />
            )}
          </Field>
        )}
        <Field
          label={t("privacy.customerNoticeGroup")}
          hint={t("privacy.customerNoticeGroupHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              maxLength={500}
              value={draft.group}
              onChange={(event) => edit({ group: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={t("privacy.customerNoticeChannel")}
          hint={t("privacy.customerNoticeChannelHint")}
        >
          {(control) => (
            <TextInput
              {...control}
              maxLength={200}
              value={draft.channel}
              onChange={(event) => edit({ channel: event.target.value })}
            />
          )}
        </Field>
        <Field label={t("privacy.customerNoticeText")}>
          {(control) => (
            <Textarea
              {...control}
              rows={4}
              maxLength={4000}
              value={draft.text}
              onChange={(event) => edit({ text: event.target.value })}
            />
          )}
        </Field>
        <Field label={t("privacy.customerNoticeFrom")}>
          {(control) => (
            <DateInput
              {...control}
              value={draft.from}
              max={isoOrBlank(todayIn(zone)) || undefined}
              onChange={(event) => edit({ from: event.target.value })}
            />
          )}
        </Field>
        <Field
          label={t("privacy.customerNoticeUntil")}
          hint={t("privacy.customerNoticeUntilHint")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={draft.until}
              min={draft.from || undefined}
              onChange={(event) => edit({ until: event.target.value })}
            />
          )}
        </Field>
        {marketing.length === 0 && purposes.isSuccess ? (
          <p className="t-caption">{t("privacy.customerNoticeNoPurposes")}</p>
        ) : (
          <ChoiceList
            multiple
            legend={t("privacy.customerNoticePurposes")}
            value={draft.purposes}
            onChange={(next) => edit({ purposes: next })}
            choices={marketing.map((p) => ({ value: p.id, label: p.label }))}
          />
        )}
        <ErrorLine error={declare.error} />
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!ready || declare.isPending}
        >
          {t("privacy.customerNoticeSave")}
        </Button>
      </div>
    </>
  );
}

function WithdrawForm({
  notice,
  onDone,
}: Readonly<{ notice: CustomerNotice; onDone: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const formId = useId();
  const [reason, setReason] = useState("");
  const withdraw = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/existing-customer-notices/{id}/withdraw", {
          params: { path: { id: notice.id } },
          body: { reason: reason.trim() },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: NOTICES_KEY });
      onDone();
    },
  });
  return (
    <>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (reason.trim() !== "" && !withdraw.isPending) withdraw.mutate();
        }}
      >
        <p>{t("privacy.customerNoticeWithdrawBody")}</p>
        <Field label={t("privacy.customerNoticeWithdrawReason")}>
          {(control) => (
            <Textarea
              {...control}
              rows={3}
              maxLength={500}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
        <ErrorLine error={withdraw.error} />
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={reason.trim() === "" || withdraw.isPending}
        >
          {t("privacy.customerNoticeWithdraw")}
        </Button>
      </div>
    </>
  );
}

function NoticeTable({
  notices,
  canWrite,
  onWithdraw,
}: Readonly<{
  notices: CustomerNotice[];
  canWrite: boolean;
  onWithdraw: (notice: CustomerNotice) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const entities = useLegalEntities();
  const purposes = useMarketingPurposes();
  const entityName = (id: string) =>
    entities.data?.find((e) => e.id === id)?.display_name ?? "";
  const purposeName = (id: string) =>
    purposes.data?.find((p) => p.id === id)?.label ?? "";
  const day = (iso: string) => formatDate(iso, locale, zone);
  const period = (n: CustomerNotice) =>
    n.effective_until
      ? t("privacy.customerNoticeRange", {
          from: day(n.effective_from),
          until: day(
            new Date(
              new Date(n.effective_until).getTime() - 1000,
            ).toISOString(),
          ),
        })
      : t("privacy.customerNoticeOngoing", { from: day(n.effective_from) });
  const columns: DataTableColumn<CustomerNotice>[] = [
    {
      key: "group",
      header: t("privacy.customerNoticeGroup"),
      render: (n) => (
        <CellStack>
          <span>{n.customer_group}</span>
          <span className="t-caption">{n.collection_channel}</span>
        </CellStack>
      ),
    },
    {
      key: "entity",
      header: t("privacy.customerNoticeEntity"),
      render: (n) => entityName(n.legal_entity_id),
    },
    {
      key: "period",
      header: t("privacy.customerNoticePeriod"),
      render: (n) =>
        n.withdrawn_at ? (
          <CellStack>
            <span>{period(n)}</span>
            <Badge tone="warning">{t("privacy.customerNoticeWithdrawn")}</Badge>
          </CellStack>
        ) : (
          period(n)
        ),
    },
    {
      key: "purposes",
      header: t("privacy.customerNoticePurposes"),
      render: (n) => n.purpose_ids.map(purposeName).filter(Boolean).join(", "),
    },
  ];
  if (canWrite) {
    columns.push({
      key: "withdraw",
      header: t("privacy.customerNoticeWithdraw"),
      headerHidden: true,
      render: (n) =>
        n.withdrawn_at ? null : (
          <Button aria-haspopup="dialog" onClick={() => onWithdraw(n)}>
            {t("privacy.customerNoticeWithdraw")}
          </Button>
        ),
    });
  }
  return (
    <DataTable
      label={t("privacy.customerNotices")}
      columns={columns}
      rows={notices}
      rowKey={(n) => n.id}
    />
  );
}

/** The collection-notice declarations the existing-customer route reads. */
export function CustomerNoticesCard() {
  const t = useT();
  const canRead = useCan("consent_config", "read");
  const canWrite = useCanWrite("consent_config", "create");
  const declareTitle = useId();
  const withdrawTitle = useId();
  const [declaring, setDeclaring] = useState(false);
  const [withdrawing, setWithdrawing] = useState<CustomerNotice | null>(null);
  const query = useQuery({
    queryKey: NOTICES_KEY,
    queryFn: async () =>
      unwrap(await api.GET("/existing-customer-notices")).data,
    enabled: canRead,
  });
  if (!canRead) return null;
  const notices = query.data ?? [];
  return (
    <Panel
      title={t("privacy.customerNotices")}
      titleAction={
        canWrite ? (
          <Button aria-haspopup="dialog" onClick={() => setDeclaring(true)}>
            {t("privacy.customerNoticeDeclare")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("privacy.customerNoticesSub")}</PanelIntro>
        {(!query.isSuccess || notices.length === 0) && (
          <QueryStates
            query={query}
            pendingLabel={t("privacy.customerNoticesLoading")}
          >
            <EmptyState>{t("privacy.customerNoticesEmpty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      <CardBoundary>
        {query.isSuccess && notices.length > 0 && (
          <NoticeTable
            notices={notices}
            canWrite={canWrite}
            onWithdraw={setWithdrawing}
          />
        )}
      </CardBoundary>
      <Modal
        open={declaring}
        onClose={() => setDeclaring(false)}
        labelledBy={declareTitle}
        intent="form"
      >
        <Heading size="large" id={declareTitle} className="t-h2 modal-title">
          {t("privacy.customerNoticeDeclare")}
        </Heading>
        {declaring && <DeclareForm onDone={() => setDeclaring(false)} />}
      </Modal>
      <Modal
        open={withdrawing !== null}
        onClose={() => setWithdrawing(null)}
        labelledBy={withdrawTitle}
        intent="form"
      >
        <Heading size="large" id={withdrawTitle} className="t-h2 modal-title">
          {t("privacy.customerNoticeWithdrawTitle")}
        </Heading>
        {withdrawing && (
          <WithdrawForm
            notice={withdrawing}
            onDone={() => setWithdrawing(null)}
          />
        )}
      </Modal>
    </Panel>
  );
}
