import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, EmptyState, StatCard } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Panel, PanelBody } from "../design-system/panel";
import { Row } from "../design-system/stack";
import { StatStrip } from "../design-system/statstrip";
import { stable } from "../format/collate";
import { formatMoney, formatNumber, INTL_LOCALE } from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { CommissionDecision, decisionsFor } from "./commissiondecide";
import { QueryGate, throwProblem } from "./common";
import { EntityRef } from "./entityref";
import "./partnercommissions.css";

// What a partner has earned, on the partner's own company page.
//
// The margin tier one screen up is what the arrangement SAYS; this is what it
// has actually produced. Showing the tier without ever showing the money is how
// a number nobody can check ends up in a contract.

type CommissionEntry = components["schemas"]["CommissionEntry"];
type CommissionStatus = CommissionEntry["status"];

// Each ledger state and how it reads. The tones say what a reader needs at a
// glance: money still owed, money agreed, money gone, money taken back.
const STATUS_LABELS: Record<CommissionStatus, MessageKey> = {
  accrued: "commission.status.accrued",
  approved: "commission.status.approved",
  paid: "commission.status.paid",
  void: "commission.status.void",
};

// Accrued is the one that still needs a decision, so it leads; approved and
// paid are both settled and read the same; void is the exception a reader must
// not skim past.
const STATUS_TONES: Record<CommissionStatus, "accent" | "success" | "warning"> =
  {
    accrued: "accent",
    approved: "success",
    paid: "success",
    void: "warning",
  };

// The whole ledger, followed page by page.
//
// Reading page one and stopping would under-report what a partner earned the
// moment they pass one page of entries — and it would do it silently, which is
// the worst way for a money figure to be wrong. The panel totals nothing today,
// but a list that claims to be the ledger has to be the ledger.
async function fetchPartnerCommissions(
  companyId: string,
): Promise<CommissionEntry[]> {
  const entries: CommissionEntry[] = [];
  let cursor: string | undefined;
  do {
    const { data, error } = await api.GET("/commissions", {
      params: {
        query: { partner_company_id: companyId, limit: 50, cursor },
      },
    });
    if (error) {
      throwProblem(error);
    }
    entries.push(...(data?.data ?? []));
    cursor = data?.page?.has_more
      ? (data.page.next_cursor ?? undefined)
      : undefined;
  } while (cursor);
  return entries;
}

/**
 * outstandingByCurrency totals what is still OWED, per currency.
 *
 * Accrued and approved are both money this partner has not been paid; paid is
 * settled and void is money that came back, so neither counts. Never summed
 * ACROSS currencies — the schema's own summary row says why, and two currencies
 * added together is a number that means nothing.
 *
 * Derived from the entries already on screen rather than from
 * GET /commissions/summary: that endpoint answers for every partner at once and
 * this panel is about one, so a second request would fetch the whole ledger to
 * show a subset of what is already here — and the two could disagree while one
 * of them was stale.
 */
export function outstandingByCurrency(
  entries: CommissionEntry[],
): Array<{ currency: string; amountMinor: number; entryCount: number }> {
  const totals = new Map<string, { amountMinor: number; entryCount: number }>();
  for (const entry of entries) {
    if (entry.status !== "accrued" && entry.status !== "approved") {
      continue;
    }
    const running = totals.get(entry.currency);
    // The COUNT travels with the sum, because the card states both and a second
    // pass over the same rows to find it is a second answer to one question.
    totals.set(entry.currency, {
      amountMinor: (running?.amountMinor ?? 0) + entry.amount_minor,
      entryCount: (running?.entryCount ?? 0) + 1,
    });
  }
  return [...totals.entries()]
    .map(([currency, total]) => ({ currency, ...total }))
    .sort((a, b) => stable(a.currency, b.currency));
}

/**
 * PartnerCommissions lists what this partner earned, newest first.
 *
 * A reversal keeps its own row rather than being folded into the entry it
 * cancels: the ledger's whole premise is that what it recorded stays recorded,
 * and a partner asking "what happened to that one" needs to see both halves.
 */
export function PartnerCommissions({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: ["partner-commissions", companyId],
    queryFn: () => fetchPartnerCommissions(companyId),
  });

  return (
    <Panel title={t("commission.panelTitle")}>
      <QueryGate query={query} pendingLabel={t("commission.panelTitle")}>
        {(entries) =>
          entries.length === 0 ? (
            <PanelBody>
              <EmptyState>{t("commission.none")}</EmptyState>
            </PanelBody>
          ) : (
            <PanelBody>
              {/* What this ledger is and is not: it records decisions, and the
                  money leaves through the finance system. On the panel, because
                  it is true of every row and every total under it. */}
              <p className="t-sub">{t("commission.decide.settledElsewhere")}</p>
              <OutstandingStrip entries={entries} locale={locale} />
              <CommissionLedger
                entries={entries}
                locale={locale}
                companyId={companyId}
              />
            </PanelBody>
          )
        }
      </QueryGate>
    </Panel>
  );
}

/**
 * OutstandingStrip is the one figure somebody running the programme opens this
 * panel for: what is still owed.
 *
 * One slot per currency, because they are not addable. Nothing is drawn when
 * everything is settled — a strip reading "0" is a slot spent saying there is
 * nothing to say, and the ledger below already shows the entries are paid.
 */
function OutstandingStrip({
  entries,
  locale,
}: Readonly<{ entries: CommissionEntry[]; locale: Locale }>) {
  const t = useT();
  const plural = usePlural();
  const outstanding = outstandingByCurrency(entries);
  if (outstanding.length === 0) {
    return null;
  }
  return (
    <div
      data-testid="commission-outstanding"
      className="partnercommissions-outstanding"
    >
      <StatStrip>
        {outstanding.map(({ currency, amountMinor, entryCount }) => (
          <StatCard
            key={currency}
            // A partner owed in two currencies is two slots, and on a phone
            // they read as rows rather than as two clipped boxes.
            narrow="row"
            label={t("commission.outstanding")}
            // In FULL, never compact. This is money somebody is OWED: the
            // compact form carries no fraction below ten thousand, so forty
            // cents outstanding would read "€0" — a partner told they are owed
            // nothing. The aggregates of deal value elsewhere abbreviate for
            // width; a balance does not.
            value={formatMoney(amountMinor, currency, locale)}
            // What the figure is made of. Where paying happens is true of the
            // whole panel rather than of this one currency's total, so it is
            // said once above the readings instead of on each of them.
            detail={plural("commission.outstandingDetail", entryCount, {
              count: formatNumber(entryCount, locale),
            })}
          />
        ))}
      </StatStrip>
    </div>
  );
}

function CommissionLedger({
  entries,
  locale,
  companyId,
}: Readonly<{
  entries: CommissionEntry[];
  locale: Locale;
  companyId: string;
}>) {
  const t = useT();
  // The object grant decides whether the verbs are drawn at all. Without this
  // a read-only seat sees Approve and Reverse on every row and learns from a
  // 403 that they were never theirs — a control nobody may press is not a
  // control, it is a promise the server breaks.
  //
  // WITHHELD, not absent: the column keeps its place and says the decision is
  // not this reader's. Dropping it would leave a reader unable to tell "there
  // is nothing to decide here" from "you may not decide it", which are
  // opposite facts that make the same shape on screen.
  //
  // The object grant is the half a client can know. Row scope is the server's
  // — a `read` share of the deal carries no authority over its partner's money
  // (decide.go's write probe) — so a grant-holder can still be refused, and
  // the dialog surfaces that refusal rather than pretending it cannot happen.
  const canDecide = useCanWrite("commission", "update");
  return (
    <div data-testid="commission-ledger">
      <DataTable
        label={t("commission.panelTitle")}
        rows={entries}
        rowKey={(entry) => entry.id}
        columns={[
          {
            // The deal leads: an entry's first question is "on what?", and a
            // ledger of bare figures cannot be reconciled against anything.
            // EntityRef resolves the name and links to the record, so a
            // partner's earnings are traceable to the deals that produced them.
            key: "deal",
            header: t("commission.column.deal"),
            render: (entry) => <EntityRef kind="deal" id={entry.deal_id} />,
          },
          {
            key: "amount",
            header: t("commission.column.amount"),
            render: (entry) =>
              formatMoney(entry.amount_minor, entry.currency, locale),
          },
          {
            key: "rate",
            header: t("commission.column.rate"),
            // Basis points render as the percentage a human agreed to: 1500 is
            // the tier's 15%, and nobody outside the schema thinks in bps.
            render: (entry) => formatRate(entry.rate_bps, locale),
          },
          {
            key: "basis",
            header: t("commission.column.basis"),
            render: (entry) =>
              formatMoney(entry.basis_amount_minor, entry.currency, locale),
          },
          {
            key: "status",
            header: t("commission.column.status"),
            render: (entry) => (
              <Badge tone={STATUS_TONES[entry.status]}>
                {t(STATUS_LABELS[entry.status])}
              </Badge>
            ),
          },
          {
            // Only what this row's state actually admits — decisionsFor
            // mirrors the store's legalTransitions, so a control here is one
            // the server will accept. A settled or reversed row offers
            // nothing and renders an empty cell rather than a disabled verb:
            // there is no precondition the reader could clear.
            key: "decision",
            header: t("commission.column.actions"),
            render: (entry) => {
              const decisions = decisionsFor(entry.status);
              if (decisions.length === 0) {
                // Terminal or settled: there is genuinely nothing to decide,
                // and that is a different fact from being refused.
                return null;
              }
              if (!canDecide) {
                return (
                  <span data-testid="commission-withheld">
                    {t("commission.decide.withheld")}
                  </span>
                );
              }
              return (
                <Row gap="2" wrap={false}>
                  {decisions.map((decision) => (
                    <CommissionDecision
                      key={decision}
                      entry={entry}
                      decision={decision}
                      companyId={companyId}
                    />
                  ))}
                </Row>
              );
            },
          },
        ]}
      />
    </div>
  );
}

/**
 * formatRate renders basis points as a percentage in the reader's locale.
 *
 * Trailing zeros are dropped so the common whole-percent tiers read as "15%"
 * rather than "15.00%", while a rate that genuinely carries a fraction still
 * shows it. Through Intl rather than string arithmetic: a German reader writes
 * "12,5 %", and a hand-built string would hand them a decimal point.
 */
export function formatRate(rateBps: number, locale: Locale): string {
  const percent = rateBps / 100;
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "percent",
    maximumFractionDigits: Number.isInteger(percent) ? 0 : 2,
  }).format(percent / 100);
}
