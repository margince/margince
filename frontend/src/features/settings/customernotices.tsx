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
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { ErrorLine } from "../../design-system/errorline";
import { Textarea } from "../../design-system/field";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { formatDate } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { useLegalEntities } from "../../lib/legalentities.queries";
import { blankRequired, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { DeclareForm } from "./customernotices.declare";
import { NOTICES_KEY, useMarketingPurposes } from "./customernotices.queries";

type CustomerNotice = components["schemas"]["ExistingCustomerNotice"];

function WithdrawForm({
  notice,
  onDone,
}: Readonly<{ notice: CustomerNotice; onDone: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const [reason, setReason] = useState("");
  const reasonLabel = t("privacy.customerNoticeWithdrawReason");
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
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          const blank = blankRequired(locale, {
            reason: [reasonLabel, reason],
          });
          if (!faults.show(blank) && !withdraw.isPending) withdraw.mutate();
        }}
      >
        <p>{t("privacy.customerNoticeWithdrawBody")}</p>
        <Field label={reasonLabel} required error={faults.of("reason")}>
          {(control) => (
            <Textarea
              {...control}
              rows={3}
              maxLength={500}
              value={reason}
              onChange={(event) => {
                setReason(event.target.value);
                faults.edited("reason");
              }}
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
          pending={withdraw.isPending}
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
          from: day(n.covers_from),
          until: day(
            new Date(
              new Date(n.effective_until).getTime() - 1000,
            ).toISOString(),
          ),
        })
      : t("privacy.customerNoticeOngoing", { from: day(n.covers_from) });
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
