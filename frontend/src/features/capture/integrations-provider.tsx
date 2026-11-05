// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./integrations-provider.css";
import type { components } from "../../api/schema";
import { EmptyState } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useProviderConnections } from "../../lib/provider-status";
import { useMe } from "../../lib/session";
// Disconnect and delete-data are separate decisions, so both sit behind the
// overflow: beside the connect button, a misclick would destroy paid-for data.
import { PricedCategoryRows } from "./integrations-provider-categories";
import { CredentialRow } from "./integrations-provider-credential";
import { LookupPostureRow } from "./integrations-provider-posture";

export type ProviderConnection = components["schemas"]["ProviderConnection"];

export function ProviderCard() {
  const t = useT();
  const query = useProviderConnections();
  // Every seat reads this card — the balances and the spend are a rep's
  // explanation for a dated value on a contact record — while connecting and
  // destroying are admin/ops. The two answers are computed HERE, once, so the
  // posture line below and the affordances inside cannot disagree about who
  // may do what.
  //
  // Connect asks for `create` alone, not the create-or-update an upsert would:
  // the PUT replaces an existing credential, but the server admits it on
  // `create` whichever it turns out to be, so a reader holding only `update`
  // would be shown a button that can only 403.
  const me = useMe();
  const canConnect = useCanWrite("integrations", "create");
  const canDestroy = useCanWrite("integrations", "delete");
  // The configuration PATCH is its own verb (integrations/update.go), so the
  // auto-enrich switch asks for that rather than borrowing either answer above.
  const canEdit = useCanWrite("integrations", "update");
  // Gated on the probe having ANSWERED, so a reader who does hold the grants
  // never sees this flash while /me is in flight.
  const readOnly = me.isSuccess && !canConnect && !canDestroy && !canEdit;
  return (
    <Panel title={t("provider.title")}>
      <PanelBody>
        <PanelIntro>{t("provider.sub")}</PanelIntro>
        {/* Outside the gate: its empty and error branches replace their
            children, and those are the states where an unexplained absence of
            controls reads as a bug. */}
        {readOnly && <PanelIntro>{t("provider.readOnly")}</PanelIntro>}
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("provider.title")}>
        {(result) =>
          // The server lists every registered provider, connected or not, so
          // an empty list means no adapter is compiled in, as a 501 does.
          result.notConfigured || result.connections.length === 0 ? (
            <PanelBody>
              <EmptyState>{t("provider.notConfigured")}</EmptyState>
            </PanelBody>
          ) : (
            <PanelBody>
              <SettingList>
                <LookupPostureRow canEdit={canEdit} />
                {result.connections.map((connection) => (
                  <ProviderConnectionRows
                    key={connection.provider}
                    connection={connection}
                    canConnect={canConnect}
                    canDestroy={canDestroy}
                    canEdit={canEdit}
                  />
                ))}
              </SettingList>
            </PanelBody>
          )
        }
      </QueryGate>
    </Panel>
  );
}

function ProviderConnectionRows({
  connection,
  canConnect,
  canDestroy,
  canEdit,
}: Readonly<{
  connection: ProviderConnection;
  canConnect: boolean;
  canDestroy: boolean;
  canEdit: boolean;
}>) {
  const t = useT();
  return (
    <>
      <CredentialRow
        connection={connection}
        canConnect={canConnect}
        canDestroy={canDestroy}
      />
      <LookupBacklogRow connection={connection} />
      <PricedCategoryRows connection={connection} canEdit={canEdit} />
      {/* Both readings need a key: without one there is no balance to ask
          for and nothing has been spent. */}
      {connection.credential_present && (
        <>
          <SettingRow
            label={t("provider.credits")}
            layout="stack"
            control={<CreditsReading connection={connection} />}
          />
          <SettingRow
            label={t("provider.spend")}
            description={t("provider.spend.hint")}
            layout="stack"
            control={<SpendReading connection={connection} />}
          />
        </>
      )}
    </>
  );
}

// What THIS installation consumed. The hint on the row above says why it can
// legitimately differ from the provider's own invoice: the same credits are
// spendable through their app, so neither figure is a check on the other.
type SpendMonth = NonNullable<ProviderConnection["spend"]>["months"][number];

function SpendReading({
  connection,
}: Readonly<{ connection: ProviderConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  const months = connection.spend?.months ?? [];
  if (months.length === 0) {
    return <p className="t-caption">{t("provider.spend.none")}</p>;
  }
  // The series only carries months that had spend, so its newest entry is not
  // necessarily this one. Last month's total must not read as the current bill.
  const now = new Date();
  const current = `${now.getUTCFullYear()}-${String(now.getUTCMonth() + 1).padStart(2, "0")}-01`;
  const figure = (value: number) => formatNumber(value, locale);
  const columns: DataTableColumn<SpendMonth>[] = [
    {
      key: "month",
      header: t("provider.spend.month"),
      render: (month) =>
        month.month === current ? t("provider.spend.thisMonth") : month.month,
    },
    { key: "pool", header: t("provider.spend.pool"), render: (m) => m.pool },
    {
      key: "charged",
      header: t("provider.spend.chargedHead"),
      align: "end",
      render: (m) => figure(m.charged_credits),
    },
    // Never folded into the charge: the platform does not know whether held
    // credits were spent, so this is the figure a human reconciles by hand.
    {
      key: "held",
      header: t("provider.spend.heldHead"),
      align: "end",
      render: (m) => figure(m.held_credits),
    },
    {
      key: "runs",
      header: t("provider.spend.runsHead"),
      align: "end",
      render: (m) => figure(m.runs),
    },
  ];
  return (
    // Not `bleed`: the table answers a stacked settings row, so it keeps the
    // row's text column instead of spanning the panel.
    <div className="provider-reading">
      <DataTable
        columns={columns}
        rows={months}
        rowKey={(month) => `${month.month}-${month.pool}`}
        label={t("provider.spend")}
      />
    </div>
  );
}

// What the PROVIDER says is left — their number, never ours. A customer may
// spend the same credits through the provider's own app, so this is a reading
// of their ledger and the card never presents it as our accounting.
function CreditsReading({
  connection,
}: Readonly<{ connection: ProviderConnection }>) {
  const t = useT();
  // Iterated, never hardcoded to email/mobile: the pool names are the
  // PROVIDER's own vocabulary, and a second provider meters different ones.
  // A pool whose balance is null is a pool we have no reading for — the
  // disconnect clears the number with the credential that fetched it. Rendering
  // it as 0 would assert an empty account, which is a different claim from
  // "we do not know" and the one thing this block must never say by accident.
  const pools = Object.entries(connection.credits?.pools ?? {}).filter(
    ([, balance]) => balance !== null && balance !== undefined,
  );
  if (pools.length === 0) {
    // Two different silences. With no key we never asked, and saying the
    // provider "has not told us" would blame them for our own empty state.
    return (
      <p className="t-caption">
        {connection.credential_present
          ? t("provider.credits.none")
          : t("provider.credits.notConnected")}
      </p>
    );
  }
  const highest = Math.max(1, ...pools.map(([, balance]) => balance ?? 0));
  // A pool is one reading inside the row, not a section of its own: FactList's
  // `dt` is the row label the grid was built for. The bar rides in `note`,
  // under the figure it qualifies — and it carries the pool's name itself,
  // since a role="meter" takes no accessible name from the text beside it.
  const facts: Fact[] = pools.map(([pool, balance]) => ({
    key: pool,
    term: pool,
    value: balance ?? 0,
    note: <Meter value={balance ?? 0} max={highest} label={pool} flat />,
  }));
  return (
    <div className="provider-reading">
      <FactList className="provider-pools" facts={facts} numeric />
      {(connection.effective_constraints ?? []).length > 0 && (
        <p className="provider-block-hint">
          {t("provider.constraints")}:{" "}
          {(connection.effective_constraints ?? []).join(", ")}
        </p>
      )}
    </div>
  );
}

// How much of the installation is still waiting to be looked up.
//
// The count alone would be a number that sometimes stops moving for reasons
// nobody on this screen can see: the posture switched off, the day's ceiling
// spent, the provider not usable. So the server answers both, and the row says
// which of the two the reader is looking at — a figure that is not falling is
// explained rather than read as a stuck sweep.
//
// Absent only at zero. NOT hidden while disconnected: "the provider is not
// usable" is one of the three causes the paused sentence names, and an
// installation that disconnects with a thousand contacts pending still has
// them pending. Hiding the count in the state its own copy explains would be
// the row disappearing exactly when it had something to say.
function LookupBacklogRow({
  connection,
}: Readonly<{ connection: ProviderConnection }>) {
  const t = useT();
  const plural = usePlural();
  // The reader's own notation: a five-figure backlog is the ordinary case on a
  // real installation, and 12345 reads as a serial number.
  const { locale } = useLocale();
  const backlog = connection.lookup_backlog;
  if (!backlog || backlog.remaining === 0) {
    return null;
  }
  return (
    <SettingRow
      label={t("provider.backlog")}
      description={
        backlog.paused
          ? t("provider.backlogPaused")
          : t("provider.backlogWorking")
      }
      control={() => (
        <span className="provider-backlog-count">
          {plural("provider.backlogRemaining", backlog.remaining, {
            count: formatNumber(backlog.remaining, locale),
          })}
        </span>
      )}
    />
  );
}
