// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The Account coverage section: the accounts at risk in the chosen
// population, in three views the server judges by the framework's thresholds.
// A row opens a drawer with the facts behind its verdict and the records.

import { useQuery } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Button, SegmentedControl } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { FilterBar } from "../../design-system/filterbar";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import {
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
  formatDateTime,
} from "../../format/format";
import { type Locale, usePlural, useT } from "../../i18n";
import { useMemberNames } from "../../lib/membernames";
import { unwrap } from "../../lib/problem";
import type { AnalyticsScope } from "./analytics.context";

export type AccountCoverage = components["schemas"]["AccountCoverage"];
export type AccountCoverageRow = components["schemas"]["AccountCoverageRow"];
type View = components["schemas"]["AccountCoverage"]["view"];

const VIEWS: readonly View[] = ["single_threaded", "untouched", "silent"];

export function useAccountCoverage(scope: AnalyticsScope, view: View) {
  return useQuery({
    queryKey: ["analytics-account-coverage", scope.kind, scope.id, view],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/account-coverage", {
          params: {
            query: {
              scope_kind:
                scope.kind === "managed_teams" ? undefined : scope.kind,
              scope_id: scope.id,
              view,
            },
          },
        }),
      ),
  });
}

export function AccountCoverageView({
  scope,
  scopeControl,
  locale,
  timezone,
}: Readonly<{
  scope: AnalyticsScope;
  scopeControl: ReactNode;
  locale: Locale;
  timezone: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const [view, setView] = useState<View>("single_threaded");
  const [open, setOpen] = useState<AccountCoverageRow | null>(null);
  const query = useAccountCoverage(scope, view);
  const labels: Record<View, string> = {
    single_threaded: t("analytics.accounts.singleThreaded"),
    untouched: t("analytics.accounts.untouched"),
    silent: t("analytics.accounts.silent"),
  };
  return (
    <>
      <FilterBar
        label={t("analytics.filters")}
        caption={
          query.data ? thresholdsCaption(query.data, locale, t) : undefined
        }
      >
        {scopeControl}
        <SegmentedControl
          options={VIEWS}
          value={view}
          onChange={(next) => {
            setView(next);
            setOpen(null);
          }}
          labels={labels}
          counts={query.data ? countsOf(query.data) : undefined}
          label={t("analytics.accounts.views")}
        />
      </FilterBar>
      <QueryGate
        query={query}
        pendingLabel={t("analytics.accounts.pending")}
        pendingLines={4}
      >
        {(coverage) => (
          <Panel title={labels[view]}>
            <PanelBody>
              <p className="t-caption">
                {viewPurpose(view, coverage, locale, t)}
              </p>
              {!coverage.activity_readable ? (
                <Callout
                  tone="info"
                  title={t("analytics.accounts.noActivityGrant")}
                />
              ) : (
                <CoverageTable
                  view={view}
                  rows={coverage.rows}
                  locale={locale}
                  timezone={timezone}
                  onOpen={setOpen}
                />
              )}
              {coverage.truncated && (
                <p className="t-caption">{t("analytics.accounts.truncated")}</p>
              )}
              {coverage.withheld !== undefined &&
                coverage.withheld > 0 &&
                view === "single_threaded" && (
                  <p className="t-caption">
                    {plural("analytics.accounts.withheld", coverage.withheld, {
                      count: formatAnalyticsNumber(coverage.withheld, locale),
                    })}
                  </p>
                )}
            </PanelBody>
          </Panel>
        )}
      </QueryGate>
      {open && (
        <EvidenceDrawer
          view={view}
          row={open}
          locale={locale}
          timezone={timezone}
          onClose={() => setOpen(null)}
        />
      )}
    </>
  );
}

function countsOf(coverage: AccountCoverage): Partial<Record<View, number>> {
  const counts: Partial<Record<View, number>> = {
    single_threaded: coverage.counts.single_threaded,
  };
  if (coverage.counts.untouched !== undefined)
    counts.untouched = coverage.counts.untouched;
  if (coverage.counts.silent !== undefined)
    counts.silent = coverage.counts.silent;
  return counts;
}

type Translator = ReturnType<typeof useT>;

/** The thresholds in use, printed so a reader knows what "at risk" meant. */
function thresholdsCaption(
  coverage: AccountCoverage,
  locale: Locale,
  t: Translator,
): string {
  const limits = coverage.thresholds;
  return t("analytics.accounts.thresholds", {
    seats: formatAnalyticsNumber(limits.max_engaged_seats ?? 1, locale),
    untouched: formatAnalyticsNumber(limits.untouched_days ?? 21, locale),
    silent: formatAnalyticsNumber(limits.silent_days ?? 30, locale),
    won: formatAnalyticsNumber(limits.won_within_days ?? 180, locale),
  });
}

function viewPurpose(
  view: View,
  coverage: AccountCoverage,
  locale: Locale,
  t: Translator,
): string {
  const limits = coverage.thresholds;
  switch (view) {
    case "single_threaded":
      return t("analytics.accounts.singleThreadedPurpose", {
        seats: formatAnalyticsNumber(limits.max_engaged_seats ?? 1, locale),
      });
    case "untouched":
      return t("analytics.accounts.untouchedPurpose", {
        days: formatAnalyticsNumber(limits.untouched_days ?? 21, locale),
      });
    case "silent":
      return t("analytics.accounts.silentPurpose", {
        days: formatAnalyticsNumber(limits.silent_days ?? 30, locale),
        won: formatAnalyticsNumber(limits.won_within_days ?? 180, locale),
      });
  }
}

function CoverageTable({
  view,
  rows,
  locale,
  timezone,
  onOpen,
}: Readonly<{
  view: View;
  rows: readonly AccountCoverageRow[];
  locale: Locale;
  timezone: string;
  onOpen: (row: AccountCoverageRow) => void;
}>) {
  const t = useT();
  const ownerNames = useMemberNames(
    rows.flatMap((row) => {
      const owner = row.deal?.owner_id ?? row.company.owner_id;
      return owner ? [owner] : [];
    }),
  );
  if (rows.length === 0) {
    return <p className="t-caption">{t("analytics.accounts.none")}</p>;
  }
  const account = {
    key: "account",
    header: t("analytics.accounts.account"),
    render: (row: AccountCoverageRow) => row.company.name,
  };
  const owner = {
    key: "owner",
    header: t("analytics.accounts.owner"),
    render: (row: AccountCoverageRow) => {
      const id = row.deal?.owner_id ?? row.company.owner_id;
      return id ? (ownerNames.get(id) ?? "—") : "—";
    },
  };
  const columns =
    view === "single_threaded"
      ? [
          {
            key: "deal",
            header: t("analytics.accounts.deal"),
            render: (row: AccountCoverageRow) => row.deal?.name ?? "",
          },
          account,
          {
            key: "amount",
            header: t("reporting.dealValue"),
            render: (row: AccountCoverageRow) =>
              formatAnalyticsMoneyOrAbsent(
                row.deal?.amount_minor,
                row.deal?.currency,
                locale,
              ),
          },
          {
            key: "seats",
            header: t("analytics.accounts.engagedSeats"),
            render: (row: AccountCoverageRow) =>
              t("analytics.accounts.seatsOf", {
                engaged: formatAnalyticsNumber(row.engaged_seats ?? 0, locale),
                named: formatAnalyticsNumber(row.named_seats ?? 0, locale),
              }),
          },
          owner,
        ]
      : [
          account,
          view === "untouched"
            ? {
                key: "open",
                header: t("analytics.accounts.openDeals"),
                render: (row: AccountCoverageRow) =>
                  formatAnalyticsNumber(row.open_deals ?? 0, locale),
              }
            : {
                key: "won",
                header: t("analytics.accounts.wonDeal"),
                render: (row: AccountCoverageRow) => row.won_deal?.name ?? "",
              },
          {
            key: "last",
            header: t("analytics.accounts.lastExchange"),
            render: (row: AccountCoverageRow) =>
              lastExchange(row, locale, timezone, t),
          },
          owner,
        ];
  return (
    <DataTable
      label={t("analytics.accounts.views")}
      columns={columns}
      rows={[...rows]}
      rowKey={(row) => row.deal?.id ?? row.company.id}
      onRowClick={onOpen}
    />
  );
}

/** The newest exchange either way as a date, or that there was none. */
function lastExchange(
  row: AccountCoverageRow,
  locale: Locale,
  timezone: string,
  t: Translator,
): string {
  const newest = [row.last_inbound, row.last_outbound]
    .filter((moment): moment is string => moment !== undefined)
    .sort()
    .at(-1);
  if (!newest) return t("analytics.accounts.never");
  return row.days_silent === undefined
    ? formatDateTime(newest, locale, timezone)
    : t("analytics.accounts.daysAgo", {
        date: formatDateTime(newest, locale, timezone),
        days: formatAnalyticsNumber(row.days_silent, locale),
      });
}

function EvidenceDrawer({
  view,
  row,
  locale,
  timezone,
  onClose,
}: Readonly<{
  view: View;
  row: AccountCoverageRow;
  locale: Locale;
  timezone: string;
  onClose: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const titleId = useId();
  const facts = evidenceFacts(view, row, locale, timezone, t, plural);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="modal-title">
          {row.deal?.name ?? row.company.name}
        </Heading>
        <p className="t-caption">{t("analytics.accounts.evidencePurpose")}</p>
      </DrawerHead>
      <DrawerBody>
        <ul className="assessment-lines">
          {facts.map((fact) => (
            <li key={fact}>{fact}</li>
          ))}
        </ul>
        <div className="form-actions">
          {row.deal && (
            <EntityRef kind="deal" id={row.deal.id} name={row.deal.name} />
          )}
          {row.won_deal && (
            <EntityRef
              kind="deal"
              id={row.won_deal.id}
              name={row.won_deal.name}
            />
          )}
          <EntityRef
            kind="company"
            id={row.company.id}
            name={row.company.name}
          />
          <Button variant="ghost" onClick={onClose}>
            {t("common.close")}
          </Button>
        </div>
      </DrawerBody>
    </Modal>
  );
}

/** The facts the verdict rests on, one sentence each. */
function evidenceFacts(
  view: View,
  row: AccountCoverageRow,
  locale: Locale,
  timezone: string,
  t: Translator,
  plural: ReturnType<typeof usePlural>,
): string[] {
  const facts: string[] = [];
  if (view === "single_threaded") {
    facts.push(
      t("analytics.accounts.factSeats", {
        engaged: formatAnalyticsNumber(row.engaged_seats ?? 0, locale),
        named: formatAnalyticsNumber(row.named_seats ?? 0, locale),
      }),
    );
    if (row.deal?.stage) {
      facts.push(t("analytics.accounts.factStage", { stage: row.deal.stage }));
    }
    return facts;
  }
  if (view === "untouched") {
    facts.push(
      plural("analytics.accounts.factOpen", row.open_deals ?? 0, {
        count: formatAnalyticsNumber(row.open_deals ?? 0, locale),
      }),
    );
  } else if (row.won_deal?.closed_at) {
    facts.push(
      t("analytics.accounts.factWon", {
        deal: row.won_deal.name,
        date: formatDateTime(row.won_deal.closed_at, locale, timezone),
      }),
    );
  }
  facts.push(
    t("analytics.accounts.factInbound", {
      date: row.last_inbound
        ? formatDateTime(row.last_inbound, locale, timezone)
        : t("analytics.accounts.never"),
    }),
    t("analytics.accounts.factOutbound", {
      date: row.last_outbound
        ? formatDateTime(row.last_outbound, locale, timezone)
        : t("analytics.accounts.never"),
    }),
  );
  return facts;
}
