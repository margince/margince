import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel } from "../../design-system/panel";
import { formatDate, formatMoney, formatNumber } from "../../format/format";
import {
  type Locale,
  type PluralBase,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];
type FinanceInvoice = components["schemas"]["FinanceInvoice"];

type Cells = {
  t: ReturnType<typeof useT>;
  plural: ReturnType<typeof usePlural>;
  locale: Locale;
  zone: string;
};

/**
 * RecentInvoices is the latest invoices as their own card under the finance
 * figures. It shows every date the server sent, so each row is checkable
 * against the invoice itself. Absent when the summary carries none.
 */
export function RecentInvoices({
  summary,
}: Readonly<{ summary: FinanceSummary }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const invoices = summary.recent_invoices ?? [];
  if (invoices.length === 0) {
    return null;
  }
  return (
    <Panel
      title={t("finance.recentInvoices")}
      footer={
        summary.truncated ? (
          <span className="t-caption">{t("finance.moreInvoices")}</span>
        ) : undefined
      }
    >
      <DataTable
        bleed
        fold
        label={t("finance.recentInvoices")}
        columns={invoiceColumns({ t, plural, locale, zone })}
        rows={invoices}
        rowKey={(invoice) => invoice.id}
      />
    </Panel>
  );
}

function invoiceColumns(cells: Cells): DataTableColumn<FinanceInvoice>[] {
  const { t, locale, zone } = cells;
  return [
    {
      key: "number",
      header: t("finance.col.invoice"),
      grow: true,
      render: (invoice) => invoice.number ?? t("finance.unnumbered"),
    },
    {
      key: "issued",
      header: t("finance.col.issued"),
      render: (invoice) => formatDate(invoice.issued_at, locale, zone),
    },
    {
      key: "amount",
      header: t("finance.col.amount"),
      align: "end",
      render: (invoice) =>
        formatMoney(invoice.gross_minor, invoice.currency, locale),
    },
    {
      key: "due",
      header: t("finance.col.due"),
      // When it was settled rides with the due day, not in its own column.
      // An unpaid invoice has no date to put there.
      render: (invoice) => (
        <span>
          {invoice.due_at ? formatDate(invoice.due_at, locale, zone) : ""}
          {invoice.paid_at && (
            <span className="t-caption">
              {invoice.due_at ? " · " : ""}
              {t("finance.paidOn", {
                when: formatDate(invoice.paid_at, locale, zone),
              })}
            </span>
          )}
        </span>
      ),
    },
    {
      key: "status",
      header: t("finance.col.status"),
      fold: "end",
      render: (invoice) => <InvoiceStatus invoice={invoice} cells={cells} />,
    },
  ];
}

// How late, and whether it was, as one reading. "Paid" and "paid 22 days
// late" are different facts about a customer. On time says nothing the status
// does not.
function InvoiceStatus({
  invoice,
  cells,
}: Readonly<{ invoice: FinanceInvoice; cells: Cells }>) {
  const { t, plural, locale } = cells;
  const late = invoice.days_late != null && invoice.days_late > 0;
  return (
    <Badge tone={STATUS_TONE[invoice.status]}>
      {late && invoice.days_late != null
        ? plural(daysLateBase(invoice.status), invoice.days_late, {
            days: formatNumber(invoice.days_late, locale),
          })
        : t(STATUS_LABEL[invoice.status])}
    </Badge>
  );
}

// One day late is one day late, not "1 days late" — and WHICH lateness it is
// depends on whether the invoice was eventually paid, which is the only thing
// this function decides. How the count picks a form is the plural helper's
// business.
function daysLateBase(status: FinanceInvoice["status"]): PluralBase {
  return status === "paid" ? "finance.paidDaysLate" : "finance.overdueDays";
}

const STATUS_LABEL: Record<FinanceInvoice["status"], MessageKey> = {
  draft: "finance.status.draft",
  open: "finance.status.open",
  partially_paid: "finance.status.partiallyPaid",
  paid: "finance.status.paid",
  overdue: "finance.status.overdue",
  disputed: "finance.status.disputed",
  credited: "finance.status.credited",
  void: "finance.status.void",
};

const STATUS_TONE: Record<
  FinanceInvoice["status"],
  "success" | "warning" | "danger" | undefined
> = {
  draft: undefined,
  open: undefined,
  partially_paid: "warning",
  paid: "success",
  overdue: "danger",
  disputed: "warning",
  credited: undefined,
  void: undefined,
};

// Where the figures came from and when. Both are the card's own honesty: a
// reader looking at money needs to know which system said so, and how long
// ago — and `offline_demo` says outright that these are demonstration data.
// Which accounting source the figures came from and how fresh they are — the
// qualification every number in the panel inherits, so it sits beside the
// panel's name rather than after its last row.
//
// Null when nothing is connected: there is no source to name, and the offer to
// connect one is an ACTION, which the panel places with its other actions
// instead of in the line that reports provenance.
