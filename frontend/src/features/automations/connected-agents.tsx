// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import type { components } from "../../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ScopeChips, scopeChipLabel } from "../../design-system/passportselect";
import { QueryStates } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import "./connected-agents.css";
import "../settings/settings-agents.css";
import { usePassportRevoke } from "../../components/passport-revoke";
import { usePassports } from "../../lib/passports.queries";
import { PassportUsageLine, usePassportUsageDetail } from "./agentusage";

// GET /passports lists a connected client's credential beside the minted
// passports; `connection` tells them apart, never the `oauth:` label prefix.

type PassportSummary = components["schemas"]["PassportSummary"];
type Connection = PassportSummary & {
  connection: NonNullable<PassportSummary["connection"]>;
};

// A connection ends when revoked, or when its credential runs out and its grant
// cannot renew. A renewable one past its expiry is only between credentials.
type ConnectionState = Readonly<{
  revoked: boolean;
  renewing: boolean;
  lapsed: boolean;
  ended: boolean;
}>;

function connectionStateOf(passport: Connection, now: number): ConnectionState {
  const revoked = passport.revoked_at != null;
  const expired =
    !revoked &&
    passport.expires_at != null &&
    Date.parse(passport.expires_at) <= now;
  // Renewing, not ended: the client repairs this itself on its next call.
  const renewing = expired && passport.connection.renewable;
  const lapsed = expired && !passport.connection.renewable;
  return { revoked, renewing, lapsed, ended: revoked || lapsed };
}

type Translate = ReturnType<typeof useT>;

function stateBadge(state: ConnectionState, t: Translate): ReactNode {
  if (state.renewing) {
    return <Badge>{t("agents.renewing")}</Badge>;
  }
  if (!state.ended) {
    return null;
  }
  return (
    <Badge tone="danger">
      {t(state.revoked ? "agents.disconnected" : "agents.lapsed")}
    </Badge>
  );
}

// A lapsed connection has lost its credential but not its grant, so it ends the
// grant. Each verb names its client: the dialog's confirm names only the act.
function EndVerb({
  connection,
  state,
  onEnd,
}: Readonly<{
  connection: Connection;
  state: ConnectionState;
  onEnd: () => void;
}>) {
  const t = useT();
  const client = connection.connection.client_name;
  if (state.ended && !state.lapsed) {
    return null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: client })}>
        <Button
          variant={state.ended ? undefined : "danger"}
          aria-label={t(
            state.ended ? "agents.revokeGrantNamed" : "agents.disconnectNamed",
            { client },
          )}
          aria-haspopup="dialog"
          onClick={onEnd}
        >
          {t(state.ended ? "agents.revokeGrantOpen" : "agents.disconnectOpen")}
        </Button>
      </OverflowMenu>
    </span>
  );
}

// Focusable because the disconnect confirm hands focus back to a client name.
function ClientName({
  connection,
  state,
  caption,
}: Readonly<{
  connection: Connection;
  state: ConnectionState;
  caption: string;
}>) {
  const t = useT();
  const name = connection.connection.client_name;
  return (
    <span className="agents-name" data-connection={connection.id} tabIndex={-1}>
      <span className="agents-name-line">
        <span
          className={
            state.ended ? "agents-name-text agents-ended" : "agents-name-text"
          }
          title={name}
        >
          {name}
        </span>
        {stateBadge(state, t)}
      </span>
      <span className="t-caption agents-fold-caption" aria-hidden="true">
        {caption}
      </span>
    </span>
  );
}

const EXPIRED: MessageKey = "agents.expiredOn";
const RENEWS: MessageKey = "agents.renewsBy";
const EXPIRES: MessageKey = "settings.passportExpiresOn";

// connected_at is a record date; the expiry is the holder's own deadline. A
// renewing or revoked row shows no expiry: its badge carries the state.
function useConnectionDates() {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const connected = (connection: Connection) =>
    formatDate(connection.connection.connected_at, locale, recordZone);
  // A renewable connection outlives its credential, so its date is when the
  // client must next renew, never when it stops working.
  const deadline = (connection: Connection, state: ConnectionState) => {
    if (connection.expires_at == null || state.revoked || state.renewing) {
      return null;
    }
    const date = formatDate(connection.expires_at, locale, viewerZone());
    if (state.lapsed) {
      return { date, phrase: EXPIRED };
    }
    return { date, phrase: connection.connection.renewable ? RENEWS : EXPIRES };
  };
  // The column's heading already says Expires; the folded caption has none.
  const expires = (connection: Connection, state: ConnectionState) => {
    const due = deadline(connection, state);
    if (due == null) {
      return "";
    }
    return due.phrase === EXPIRES
      ? due.date
      : t(due.phrase, { date: due.date });
  };
  const caption = (connection: Connection, state: ConnectionState) => {
    const due = deadline(connection, state);
    return [
      t("agents.connectedOn", { date: connected(connection) }),
      due && t(due.phrase, { date: due.date }),
    ]
      .filter(Boolean)
      .join(" · ");
  };
  return { connected, expires, caption };
}

function ConnectionTable({
  connections,
  now,
  onEnd,
}: Readonly<{
  connections: readonly Connection[];
  now: number;
  onEnd: (id: string) => void;
}>) {
  const t = useT();
  const dates = useConnectionDates();
  const stateOf = (connection: Connection) =>
    connectionStateOf(connection, now);
  const usageDetail = usePassportUsageDetail<Connection>(
    (connection) => connection.connection.client_name,
  );
  const columns: DataTableColumn<Connection>[] = [
    {
      key: "client",
      header: t("agents.colClient"),
      grow: true,
      fold: "title",
      render: (connection) => (
        <span className="agents-name-usage">
          <ClientName
            connection={connection}
            state={stateOf(connection)}
            caption={dates.caption(connection, stateOf(connection))}
          />
          <PassportUsageLine passport={connection} />
        </span>
      ),
    },
    {
      key: "scopes",
      header: t("settings.passportColScopes"),
      render: (connection) => (
        <span className="agents-scopes">
          <ScopeChips
            labels={connection.scopes.map((scope) => scopeChipLabel(t, scope))}
          />
        </span>
      ),
    },
    {
      key: "connected",
      header: t("agents.colConnected"),
      fold: "hide",
      render: (connection) => dates.connected(connection),
    },
    {
      key: "expires",
      header: t("settings.passportColExpires"),
      fold: "hide",
      render: (connection) => dates.expires(connection, stateOf(connection)),
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (connection) => (
        <EndVerb
          connection={connection}
          state={stateOf(connection)}
          onEnd={() => onEnd(connection.id)}
        />
      ),
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("agents.connected")}
      columns={columns}
      rows={[...connections]}
      rowKey={(connection) => connection.id}
      rowTestId={(connection) => `connection-${connection.id}`}
      detail={usageDetail}
    />
  );
}

// The earliest still-future expiry among the live connections, or null.
function nextExpiry(
  connections: readonly Connection[],
  now: number,
): number | null {
  const upcoming = connections
    .filter((c) => c.revoked_at == null && c.expires_at != null)
    .map((c) => Date.parse(c.expires_at as string))
    .filter((at) => at > now);
  return upcoming.length > 0 ? Math.min(...upcoming) : null;
}

// The clock this list reads, moved on when a credential passes its expiry:
// the status is derived at render, and window focus refetches nothing.
function useExpiryClock(connections: readonly Connection[]): number {
  const [now, setNow] = useState(() => Date.now());
  const until = nextExpiry(connections, now);
  useEffect(() => {
    if (until == null) {
      return;
    }
    const delay = until - Date.now();
    // Past ~24.8 days setTimeout fires at once; a tab is not left open that long.
    if (delay > 0x7fffffff) {
      return;
    }
    if (delay <= 0) {
      setNow(Date.now());
      return;
    }
    const timer = globalThis.setTimeout(() => setNow(Date.now()), delay);
    return () => globalThis.clearTimeout(timer);
  }, [until]);
  return now;
}

// Disconnect goes through the connection's grant: revoking the passport alone
// would be undone by the client's next refresh.
export function ConnectedAgentsCard() {
  const t = useT();
  // Focus returns to the ended connection's row while it is still listed, and
  // otherwise to what the list still holds.
  const emptyRegion = useRef<HTMLDivElement | null>(null);
  const list = usePassports();
  const disconnect = usePassportRevoke({
    verb: "agents.disconnect",
    question: "agents.disconnectConfirm",
    focusAfter: (id) =>
      document.querySelector<HTMLElement>(`[data-connection="${id}"]`) ??
      document.querySelector<HTMLElement>("[data-connection]") ??
      emptyRegion.current,
  });

  // Read here rather than in the render prop: the expiry timer is a hook.
  const connections = (list.data?.data ?? []).filter(
    (passport): passport is Connection => Boolean(passport.connection),
  );
  const now = useExpiryClock(connections);

  return (
    <Panel title={t("agents.connected")}>
      <PanelBody>
        <PanelIntro>{t("agents.connectedSub")}</PanelIntro>
        {!(list.isSuccess && connections.length > 0) && (
          <div ref={emptyRegion} tabIndex={-1} className="agents-empty">
            <QueryStates query={list} pendingLabel={t("agents.connected")}>
              {/* Not the generic empty state: beside a connect guide, "nothing
                  here" reads as a failed load. */}
              <EmptyState>{t("agents.noneConnected")}</EmptyState>
            </QueryStates>
          </div>
        )}
      </PanelBody>
      {list.isSuccess && connections.length > 0 && (
        <ConnectionTable
          connections={connections}
          now={now}
          onEnd={disconnect.ask}
        />
      )}
      {disconnect.confirm}
    </Panel>
  );
}
