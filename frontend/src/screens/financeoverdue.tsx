import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Meter } from "../design-system/readings";
import { formatMoney, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { medianDaysLabel } from "./company360";
import { amountOf, FinanceFigure } from "./companyfinance";

// The overdue reading: the one figure on the finance panel that asks for a
// decision, and everything the panel knows about why it is that size.
//
// Its own file because it is the only part of the card a reader ACTS on, and
// because what it must not say is as decided as what it says — a figure
// withheld on a finished relationship, and a note beside the dash saying which
// of the two absences this is.

type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];

// Overdue money, at the size of the decision it asks for, with everything the
// panel knows about WHY it is that size underneath: what share of the open
// balance it is, how late this customer usually pays, and the two halves of
// the open balance drawn against each other.
//
// The hero figure is drawn whatever the rest of the panel can say. The
// sentence and the bar under it each appear only when their own inputs
// arrived, so a customer with no settled invoices gets the figure and nothing
// invented beneath it.
export function OverdueLead({
  summary,
}: Readonly<{ summary: FinanceSummary }>) {
  const t = useT();
  const { locale } = useLocale();
  const split = openSplitOf(summary);
  return (
    <div className="fin-lead">
      <FinanceFigure
        label={t("finance.overdue")}
        value={amountOf(summary.overdue, locale)}
        hero
        // An overdue figure is a call to action, and the server withholds it
        // once the relationship has ended rather than sending a number that
        // would read as a collection to make. A bare dash here would say "we
        // could not compute it", which is a different claim — so the one case
        // the reader can act on is named.
        absentNote={
          summary.overdue?.amount_minor == null && summary.open_balance
            ? t("finance.overdueRelationshipEnded")
            : undefined
        }
      />
      <FinanceLede summary={summary} split={split} />
      {split && (
        <>
          <Meter
            value={split.overdue}
            max={split.open}
            label={t("finance.overdueShareLabel")}
            tone="danger"
            restTone="accent"
          />
          {/* The bar's two colours, named. Without this the reader is left to
              infer which half is which from the tones alone, and the tones are
              what a reader who cannot distinguish them needs the words for. */}
          <p className="fin-legend">
            <Badge tone="danger">
              {t("finance.legendOverdue", {
                amount: formatMoney(split.overdue, split.currency, locale),
              })}
            </Badge>
            <Badge tone="accent">
              {t("finance.legendOpen", {
                amount: formatMoney(split.open, split.currency, locale),
              })}
            </Badge>
          </p>
        </>
      )}
    </div>
  );
}

// What the overdue figure MEANS, in whichever clauses the data supports: its
// share of the open balance, and how late this customer settles.
//
// Each clause is a sentence and is terminated here rather than in the catalog,
// because `medianDaysLabel` is also read on its own elsewhere on the record,
// where a trailing full stop mid-line would be wrong. One terminator, spelled
// once, so the two clauses cannot end differently from each other.
function FinanceLede({
  summary,
  split,
}: Readonly<{ summary: FinanceSummary; split: OpenSplit | null }>) {
  const t = useT();
  const { locale } = useLocale();
  const clauses: string[] = [];
  if (split) {
    clauses.push(
      t("finance.shareOfOpen", {
        percent: formatNumber(
          Math.round((split.overdue / split.open) * 100),
          locale,
        ),
      }),
    );
  }
  if (summary.median_days_after_due != null) {
    clauses.push(medianDaysLabel(summary.median_days_after_due, locale, t));
  }
  if (clauses.length === 0) {
    return null;
  }
  return (
    <p className="fin-lede">{clauses.map((one) => `${one}.`).join(" ")}</p>
  );
}

// The open balance as its two halves in ONE currency: what is overdue, and
// the whole it is part of.
//
// Null unless the halves make a proportion at all. Different currencies is a
// proportion of nothing. Nothing open would put a full bar over "€0 open",
// which reads as an account entirely in arrears rather than one that owes us
// nothing. And overdue ABOVE open is not a share that has run high, it is two
// figures that contradict each other: the mirror says money is late that it
// also says is not outstanding.
//
// The third case is refused rather than clamped because clamping picks a
// winner between two figures the panel cannot choose between, and picks it
// silently — a bar pinned at 100% beside a sentence reading "115% of
// everything open" is the same disagreement with a coat of paint. The overdue
// figure above still draws: it is what the mirror reported, and it is the one
// number a reader can act on. What refuses is only the claim about a
// relationship between the two.
type OpenSplit = { overdue: number; open: number; currency: string };

function openSplitOf(summary: FinanceSummary): OpenSplit | null {
  const open = summary.open_balance;
  const overdue = summary.overdue;
  if (
    open?.amount_minor == null ||
    !open.currency ||
    overdue?.amount_minor == null ||
    open.currency !== overdue.currency ||
    open.amount_minor <= 0 ||
    overdue.amount_minor > open.amount_minor
  ) {
    return null;
  }
  return {
    overdue: overdue.amount_minor,
    open: open.amount_minor,
    currency: open.currency,
  };
}
