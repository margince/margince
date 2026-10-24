import type { components } from "../../api/schema";
import {
  HEALTH_DIMENSION_LABEL,
  HEALTH_RATING_LABEL,
  LIFECYCLE_LABELS,
} from "../../components/companylookups";
import { useFinanceSummary } from "../../components/financesummary";
import { StatCard } from "../../design-system/atoms";
import { FactList } from "../../design-system/factlist";
import { formatMoneyCompact } from "../../format/format";
import type { Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemCodeOf } from "../../lib/problem";
import type { Company360 } from "./company360";
import { WITHHELD_READING } from "./company360health";

type Health = NonNullable<Company360["health"]>;

// One rated dimension of the account's health: the rating, and the sentence it
// was read from. Named here because three readings carry it as their basis.
export type HealthDimension = NonNullable<Health["relationship"]>;

// The caveat on a figure that is shown but is not current, and undefined when
// it needs none. Stale, error and syncing say different things about whether
// anything is broken, so they never fold into one.
function staleDetailKey(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey | undefined {
  switch (state) {
    case "stale":
      // Beside a figure the caveat qualifies the figure (the last good one,
      // which may have moved since). The empty slot says it of the connection.
      return "co.strip.fin.notCurrent";
    case "error":
      return "co.strip.fin.errorFigure";
    case "syncing":
      // The first pass has not finished, so what is shown may be partial.
      return "co.strip.fin.syncing";
    default:
      return undefined;
  }
}

// The slot when there is no figure: the word it stands on and the reason under
// it. They are decided together, because each reason belongs in a different
// one of the two. A refusal and a read in flight are facts about the request
// and are the reading. A broken source explains itself underneath.
function financeAbsence({
  pending,
  withheld,
  failed,
  state,
  lifecycle,
}: Readonly<{
  pending: boolean;
  withheld: boolean;
  failed: boolean;
  state?: components["schemas"]["FinanceSummaryState"];
  lifecycle: StripLifecycle;
}>): Readonly<{ value: MessageKey; detail?: MessageKey }> {
  if (pending) {
    return { value: "co.strip.fin.loading" };
  }
  // Before any state is read: with no answer there is no state, and guessing
  // one from its absence is how a denial became setup advice.
  if (withheld) {
    return { value: WITHHELD_READING };
  }
  if (failed) {
    return { value: "co.strip.fin.error", detail: "co.strip.fin.errorWhy" };
  }
  if (state === "connected") {
    // A live, mapped source that produced no figure: nothing is broken and
    // nothing to set up, because we have never billed them. The account's own
    // stage under the word keeps it apart from one that never bought. It is
    // read rather than assumed, since a former customer comes down here too.
    return {
      value: "co.strip.fin.neverInvoiced",
      detail: LIFECYCLE_LABELS[lifecycle],
    };
  }
  return { value: "co.strip.fin.noFigure", detail: noFigureReason(state) };
}

// Why the source produced no figure. Naming the wrong state sends the reader
// to set up a connection they already have.
function noFigureReason(
  state?: components["schemas"]["FinanceSummaryState"],
): MessageKey {
  switch (state) {
    case "unmapped":
      return "co.strip.fin.unmapped";
    case "syncing":
      return "co.strip.fin.syncing";
    case "stale":
      return "co.strip.fin.staleFigure";
    case "error":
      return "co.strip.fin.errorFigure";
    default:
      // no_connection, and the read that never answered: both mean there is no
      // source, the one case the setup advice fits.
      return "co.strip.fin.noConnection";
  }
}

type StripLifecycle = NonNullable<
  Company360["state_strip"]
>["account"]["lifecycle"];

// The stages whose page leads with a money figure. A former customer's does:
// the trailing year is a fact about invoices, not about the account's stage.
// This is narrower than `hasFinance` (companyfinance.tsx), a different rule.
// That one asks whether an account could ever have been billed. It says yes
// for an unknown or disqualified stage, so the Finance tab hides no money.
const INVOICEABLE: ReadonlySet<StripLifecycle> = new Set<StripLifecycle>([
  "customer",
  "former_customer",
]);

/**
 * The customer row's one money slot: what this account has been invoiced over
 * the trailing year.
 *
 * One slot, not three. The strip is a glance and the Finance tab
 * (companyfinance.tsx) holds the detail, so open balance and the payment-habit
 * median are not here. Three slots on one figure bury the account's standing.
 *
 * One label on every branch, because a slot whose label moves is read twice.
 * "Not invoiced" under a twelve-month label is still true: nothing was
 * invoiced in any window.
 */
export function MoneyStat({
  companyId,
  locale,
  lifecycle,
  dimension,
  onOpen,
  t,
}: Readonly<{
  companyId: string;
  locale: Locale;
  // Whether there is a window to read at all, and the word for it when not.
  lifecycle: StripLifecycle;
  // The payment health reading, shown as this card's basis. The verdict on the
  // health card can then be checked against the money it was read from.
  dimension?: HealthDimension;
  // Handed to every shape this reading takes: a door on only one would make
  // the way out look like a property of the figure.
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  // The finance card and the payment health dimension run this same query.
  // So every money reading on one page agrees, and all but the first are free.
  const { data, isPending, isError, error } = useFinanceSummary(companyId);
  const slot = { label: t("co.strip.netInvoiced") } as const;
  const basis = dimension ? (
    <FactList
      facts={[
        {
          key: "payment",
          term: t(HEALTH_DIMENSION_LABEL.payment),
          value: t(HEALTH_RATING_LABEL[dimension.rating]),
          note: dimension.reason,
        },
      ]}
    />
  ) : undefined;
  // Never invoiced is a fact about the account, and it outranks every state of
  // the finance connection. A prospect on an install with no accounting must
  // not be told to connect one. A prospect on one that has it must not read as
  // though we had billed them and got nothing.
  if (!INVOICEABLE.has(lifecycle)) {
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t("co.strip.fin.neverInvoiced")}
        // Which stage, rather than "not a customer yet". Every stage that
        // reaches here answers why there is nothing to invoice. The record
        // page reads them all out of the one catalog.
        detail={t(LIFECYCLE_LABELS[lifecycle])}
      />
    );
  }
  // A refusal is not a failure, and neither is a setup gap. A reader whose role
  // cannot see finance must not be told to "connect your accounting". That
  // sends them to fix a permission on the one page that cannot fix it.
  const withheld = isError && problemCodeOf(error) === "permission_denied";
  const amount = data?.net_invoiced;
  const caveat = staleDetailKey(data?.state);
  // No figure is not €0, and the reasons there is none are not one reason.
  if (!amount || amount.amount_minor == null || !amount.currency) {
    const absence = financeAbsence({
      pending: isPending,
      withheld,
      failed: isError && !withheld,
      state: data?.state,
      lifecycle,
    });
    return (
      <StatCard
        onOpen={onOpen}
        {...slot}
        value={t(absence.value)}
        detail={absence.detail ? t(absence.detail) : undefined}
        basis={basis}
      />
    );
  }
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={formatMoneyCompact(amount.amount_minor, amount.currency, locale)}
      // One line, in the order a reader needs it. The caveat first: a figure
      // that is not current is shown with it, not withheld. Then lifetime,
      // which a second slot would cost a row. The provider last, in the line,
      // since a badge by the label makes the slot taller. Overdue and open
      // balance live on the Finance tab; a copy here would drift from it.
      detail={
        caveat
          ? t(caveat)
          : moneyPhrase(
              "co.strip.lifetimeOf",
              data?.net_invoiced_lifetime,
              locale,
              t,
            ) ||
            data?.provider ||
            undefined
      }
      basis={basis}
    />
  );
}

// One money figure as a phrase for the detail line, or nothing. Both halves
// are required and neither absence has a substitute. A figure with no currency
// cannot be rendered. A zero the server did not send would say the account
// owes us nothing, when in fact nobody has told us.
function moneyPhrase(
  key: "co.strip.lifetimeOf",
  amount:
    | { amount_minor?: number | null; currency?: string | null }
    | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string | undefined {
  return amount?.amount_minor != null && amount.currency
    ? t(key, {
        amount: formatMoneyCompact(
          amount.amount_minor,
          amount.currency,
          locale,
        ),
      })
    : undefined;
}
