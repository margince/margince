// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button, Modal } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure } from "../../design-system/disclosure";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { companyTabRoute } from "../../lib/companytab";
import { financeProvenance } from "../../lib/financeprovenance";
import { problemCodeOf } from "../../lib/problem";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { useCustomerPayments } from "./brief.payments.queries";

type Payments = components["schemas"]["CustomerPayments"];
type PaymentInvoice = components["schemas"]["CustomerPaymentInvoice"];

export function CustomerPaymentsCard({
  enabled = true,
}: Readonly<{ enabled?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const titleId = useId();
  const triggerId = useId();
  const [open, setOpen] = useState(false);
  const {
    query,
    allowed,
    latest: summary,
    pages,
  } = useCustomerPayments(enabled);
  // A failed "Load more" keeps the pages already read, so only a failed first
  // read or refetch takes the card and its drawer down.
  const failed = query.isError && !query.isFetchNextPageError;
  if (
    !enabled ||
    !allowed ||
    query.isPending ||
    (query.isError && !summary) ||
    problemCodeOf(query.error) === "permission_denied"
  )
    return null;
  if (
    !failed &&
    (summary?.state === "no_connection" || summary?.linked_companies === 0)
  )
    return null;
  return (
    <Panel
      title={t("payments.title")}
      footer={
        summary?.last_synced_at && !failed ? (
          <span className="t-caption">
            {financeProvenance(
              t,
              summary.provider_name,
              formatDateTime(summary.last_synced_at, locale, zone),
            )}
          </span>
        ) : undefined
      }
    >
      <QueryStates
        query={{
          isPending: query.isPending,
          isError: failed,
          error: query.error,
          refetch: query.refetch,
        }}
        pendingLabel={t("finance.loading")}
      >
        {summary && (
          <PaymentOverview
            summary={summary}
            onRefresh={() => void query.refetch()}
            onReview={() => setOpen(true)}
            triggerId={triggerId}
          />
        )}
      </QueryStates>
      {open && !failed && (
        <Modal
          intent="drawer-reading"
          labelledBy={titleId}
          open={open}
          returnFocusTo={() => document.getElementById(triggerId)}
          onClose={() => setOpen(false)}
        >
          <DrawerHead>
            <Heading size="medium" id={titleId}>
              {t("payments.viewInvoices")}
            </Heading>
          </DrawerHead>
          <DrawerBody>
            <PaymentInvoices
              rows={pages
                .flatMap((page) => page.data)
                .filter(
                  (invoice) =>
                    invoice.days_overdue <= (summary?.long_overdue_days ?? 365),
                )}
            />
            {(summary?.long_overdue?.invoice_count ?? 0) > 0 && (
              <Disclosure
                summary={<LongOverdueSummary summary={summary} />}
              >
                <PaymentInvoices
                  rows={pages
                    .flatMap((page) => page.data)
                    .filter(
                      (invoice) =>
                        invoice.days_overdue >
                        (summary?.long_overdue_days ?? 365),
                    )}
                />
              </Disclosure>
            )}
            <ErrorLine error={query.isFetchNextPageError && query.error} />
            <LoadMoreButton query={query} />
          </DrawerBody>
        </Modal>
      )}
    </Panel>
  );
}

function PaymentOverview({
  summary,
  onRefresh,
  onReview,
  triggerId,
}: Readonly<{
  summary: Payments;
  onRefresh: () => void;
  onReview: () => void;
  triggerId: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      <PanelBody>
        <PaymentState summary={summary} onRefresh={onRefresh} />
        {summary.state !== "syncing" &&
          summary.last_synced_at &&
          (summary.invoice_count === 0 ? (
            <p>{t("payments.none")}</p>
          ) : (
            <>
              {summary.totals.map((total) => (
                <p className="brief-payments-total" key={total.currency}>
                  {t("payments.overdueAmount", {
                    amount: formatMoney(
                      total.amount_minor,
                      total.currency,
                      locale,
                    ),
                  })}
                </p>
              ))}
              <p className="t-caption">
                {plural("payments.invoices", summary.invoice_count, {
                  count: formatNumber(summary.invoice_count, locale),
                })}
                {" · "}
                {plural("payments.customers", summary.company_count, {
                  count: formatNumber(summary.company_count, locale),
                })}
              </p>
            </>
          ))}
      </PanelBody>
      {summary.customers.map((customer) => (
        <PanelRow key={`${customer.company_id}:${customer.currency}`}>
          <div className="brief-payments-customer">
            <AppLink
              href={routeHash(companyTabRoute(customer.company_id, "finance"))}
            >
              {customer.company_name}
            </AppLink>
            <span className="t-caption">
              {plural("payments.oldest", customer.oldest_days, {
                days: formatNumber(customer.oldest_days, locale),
              })}
            </span>
          </div>
          <span className="t-num">
            {formatMoney(customer.amount_minor, customer.currency, locale)}
          </span>
        </PanelRow>
      ))}
      {summary.invoice_count > 0 && (
        <PanelBody>
          <Button id={triggerId} variant="link" onClick={onReview}>
            {t("payments.viewInvoices")}
          </Button>
        </PanelBody>
      )}
      {(summary.long_overdue?.invoice_count ?? 0) > 0 && (
        <PanelBody>
          <Disclosure
            summary={<LongOverdueSummary summary={summary} />}
          >
            {summary.long_overdue?.totals.map((total) => (
              <p key={total.currency} className="t-num">
                {formatMoney(total.amount_minor, total.currency, locale)}
              </p>
            ))}
            <p>
              {summary.provider_name
                ? t("payments.checkWriteOff", {
                    provider: summary.provider_name,
                  })
                : t("payments.checkWriteOffUnknown")}
            </p>
          </Disclosure>
        </PanelBody>
      )}
    </>
  );
}

function PaymentState({
  summary,
  onRefresh,
}: Readonly<{ summary: Payments; onRefresh: () => void }>) {
  const t = useT();
  if (summary.state === "syncing")
    return (
      <>
        <p>{t("finance.syncing")}</p>
        <Button variant="link" onClick={onRefresh}>
          {t("payments.refresh")}
        </Button>
      </>
    );
  if (summary.state !== "stale" && summary.state !== "error") return null;
  return (
    <Callout
      kind="standing"
      title={t("payments.refresh")}
      tone="warning"
      actions={
        <Button variant="link" onClick={onRefresh}>
          {t("common.retry")}
        </Button>
      }
    >
      {t(
        summary.state === "stale"
          ? "payments.stale"
          : summary.last_synced_at
            ? "payments.syncFailed"
            : "payments.syncUnavailable",
      )}
    </Callout>
  );
}

function PaymentInvoices({ rows }: Readonly<{ rows: PaymentInvoice[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const columns: DataTableColumn<PaymentInvoice>[] = [
    {
      key: "customer",
      header: t("payments.customer"),
      grow: true,
      render: (invoice) => (
        <AppLink
          href={routeHash(companyTabRoute(invoice.company_id, "finance"))}
        >
          {invoice.company_name}
        </AppLink>
      ),
    },
    {
      key: "number",
      header: t("finance.col.invoice"),
      render: (invoice) => invoice.number ?? t("finance.unnumbered"),
    },
    {
      key: "due",
      header: t("finance.col.due"),
      render: (invoice) => formatDate(invoice.due_at, locale, zone),
    },
    {
      key: "open",
      header: t("payments.openAmount"),
      align: "end",
      render: (invoice) =>
        formatMoney(invoice.open_minor, invoice.currency, locale),
    },
  ];
  return (
    <DataTable
      fold
      label={t("payments.viewInvoices")}
      columns={columns}
      rows={rows}
      rowKey={(invoice) => invoice.invoice_id}
    />
  );
}

function LongOverdueSummary({ summary }: Readonly<{ summary?: Payments }>) {
 const t = useT(); const { locale } = useLocale();
 return <>
  {t("payments.longOverdue", { count: formatNumber(summary?.long_overdue?.invoice_count ?? 0, locale) })}
  {summary?.long_overdue?.totals.map((total) => <span className="t-num" key={total.currency}> · {formatMoney(total.amount_minor, total.currency, locale)}</span>)}
 </>;
}
