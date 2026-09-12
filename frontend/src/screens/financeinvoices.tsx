import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, TableScroll } from "../design-system/atoms";
import { formatDate, formatMoney, formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];
type FinanceInvoice = components["schemas"]["FinanceInvoice"];

export function RecentInvoices({
  summary,
}: Readonly<{ summary: FinanceSummary }>) {
  const t = useT();
  const { locale } = useLocale();
  const invoices = summary.recent_invoices ?? [];
  if (invoices.length === 0) {
    return null;
  }
  return (
    // Four columns, and nothing is dropped to get there: the issue, due and
    // settlement dates read as one life of one invoice rather than three
    // columns a reader has to line up by eye. It is still every date the
    // server sent, which is what an invoice is checkable against. The table
    // scrolls sideways inside its panel rather than widening the record page:
    // `TableScroll` is the one spelling of that box, the same one DataTable
    // puts every list it draws inside (atoms.tsx).
    <TableScroll label={t("finance.recentInvoices")}>
      <table className="table fin-table">
        <thead>
          <tr>
            <th>{t("finance.col.invoice")}</th>
            <th>{t("finance.col.dates")}</th>
            <th className="fin-col-amount">{t("finance.col.amount")}</th>
            <th className="fin-col-status">{t("finance.col.status")}</th>
          </tr>
        </thead>
        <tbody>
          {invoices.map((invoice) => (
            <InvoiceRow key={invoice.id} invoice={invoice} locale={locale} />
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}

function InvoiceRow({
  invoice,
  locale,
}: Readonly<{
  invoice: FinanceInvoice;
  locale: ReturnType<typeof useLocale>["locale"];
}>) {
  const t = useT();
  const plural = usePlural();
  const recordZone = useRecordZone();
  const late = invoice.days_late != null && invoice.days_late > 0;
  return (
    // A row the customer still owes past its due date carries the tint, so the
    // rows that need chasing are findable without reading the status column of
    // every one of them. A settled invoice that was paid late is history, not
    // work, and takes no tint however late it was.
    <tr className={late && invoice.status !== "paid" ? "fin-row-late" : ""}>
      <td className="fin-cell-id">
        {invoice.number ?? t("finance.unnumbered")}
      </td>
      <td className="fin-cell-dates">
        {formatDate(invoice.issued_at, locale, recordZone)} →{" "}
        {invoice.due_at ? formatDate(invoice.due_at, locale, recordZone) : "—"}
        {/* When it was actually settled, appended rather than given a column
            of its own: an unpaid invoice has no date to put there, and a
            column of dashes states nothing the status does not. */}
        {invoice.paid_at && (
          <span className="fin-cell-paid">
            {" · "}
            {t("finance.paidOn", {
              when: formatDate(invoice.paid_at, locale, recordZone),
            })}
          </span>
        )}
      </td>
      <td className="fin-col-amount">
        {formatMoney(invoice.gross_minor, invoice.currency, locale)}
      </td>
      {/* HOW late, and whether it was, as ONE reading. "Paid" and "paid 22
          days late" are different facts about a customer, and splitting them
          across a badge and a caption made the reader assemble the sentence.
          Zero and negative say nothing worth a line: on time is what the
          status alone already says. */}
      <td className="fin-col-status">
        <Badge tone={STATUS_TONE[invoice.status]} quiet>
          {late && invoice.days_late != null
            ? plural(daysLateBase(invoice.status), invoice.days_late, {
                days: formatNumber(invoice.days_late, locale),
              })
            : t(STATUS_LABEL[invoice.status])}
        </Badge>
      </td>
    </tr>
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
  "success" | "warn" | "danger" | undefined
> = {
  draft: undefined,
  open: undefined,
  partially_paid: "warn",
  paid: "success",
  overdue: "danger",
  disputed: "warn",
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
