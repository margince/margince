// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import {
  Badge,
  Button,
  Disclosure,
  EmptyState,
  OverflowMenu,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { ScopeChips, scopeChipLabel } from "../design-system/passportselect";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, QueryStates, unwrap } from "./common";
import "./connected-agents.css";
import "./settings-agents.css";
import { usePassports } from "./passports.queries";

// A client that connects over MCP is issued its own credential at token
// exchange. GET /passports lists it beside the minted passports; `connection`
// tells the two apart, never the `oauth:` label prefix a human can also type.

type PassportSummary = components["schemas"]["PassportSummary"];
type Connection = PassportSummary & {
  connection: NonNullable<PassportSummary["connection"]>;
};

// The MCP URL as the SERVER states it, read from the RFC 9728 document the
// connector serves. It is `--public-base-url` + /mcp, the value clients are
// required to match exactly, so a guide built from it is the command that
// actually works — the SPA's own origin would only coincide with it.
//
// A 404 is not a failure here: the connector is off for this installation
// (`mcp.connector_enabled`), and the whole route group is absent. That is worth
// saying plainly instead of printing four commands that cannot connect.
type ConnectorState =
  | { readonly enabled: true; readonly url: string }
  | { readonly enabled: false };

async function fetchConnectorState(): Promise<ConnectorState> {
  const response = await fetch("/.well-known/oauth-protected-resource", {
    headers: { accept: "application/json" },
  });
  if (response.status === 404) {
    return { enabled: false };
  }
  if (!response.ok) {
    throw new Error(`discovery answered ${response.status}`);
  }
  const document: unknown = await response.json();
  const resource =
    typeof document === "object" &&
    document !== null &&
    "resource" in document &&
    typeof document.resource === "string"
      ? document.resource
      : "";
  if (resource === "") {
    throw new Error("the discovery document names no resource");
  }
  return { enabled: true, url: resource };
}

// One command per client, because "point your agent at the URL" is exactly the
// instruction contacts cannot act on. All four reach the same place: the client
// registers itself, and the consent screen asks which of the five scopes to
// grant.
//
// Antigravity is the odd one out only in shape — it has no add command, so its
// step is the config file its docs name. The OAuth handshake is identical. That
// caveat is a statement about ONE client, so it travels with that client as its
// row's `note` rather than as a loose paragraph under all four, where it read as
// a footnote to the whole guide.
const CONNECT_GUIDES: readonly Readonly<{
  id: string;
  name: string;
  command: (url: string) => string;
  note?: MessageKey;
}>[] = [
  {
    id: "claude",
    name: "Claude Code",
    command: (url: string) => `claude mcp add --transport http margince ${url}`,
  },
  {
    id: "codex",
    name: "Codex",
    command: (url: string) =>
      `codex mcp add margince --url ${url}\ncodex mcp login margince`,
  },
  {
    id: "gemini",
    name: "Gemini CLI",
    command: (url: string) => `gemini mcp add --transport http margince ${url}`,
  },
  {
    id: "antigravity",
    name: "Antigravity",
    // ~/.gemini/config/mcp_config.json — `serverUrl` specifically: Antigravity
    // rejects the `url`/`httpUrl` spellings its siblings accept.
    command: (url: string) =>
      `{ "mcpServers": { "margince": { "serverUrl": "${url}" } } }`,
    note: "agents.connectAntigravityPath",
  },
];

// Reference rather than a decision, so it reads last and closed. One client is
// one stacked row: a 60-character command in a right-hand column is unreadable.
function ConnectGuide() {
  const t = useT();
  const state = useQuery({
    queryKey: ["mcp-connector-state"],
    queryFn: fetchConnectorState,
  });
  return (
    <QueryGate query={state} pendingLabel={t("agents.connected")}>
      {(connector) =>
        connector.enabled ? (
          <>
            <PanelIntro>{t("agents.connectSteps")}</PanelIntro>
            <SettingList>
              {CONNECT_GUIDES.map((guide) => (
                <SettingRow
                  key={guide.id}
                  layout="stack"
                  // A product's own name, never translated — the same reason the
                  // command is not: both are typed exactly as they are shown.
                  label={guide.name}
                  description={guide.note && t(guide.note)}
                  control={
                    <code className="agents-guide-command">
                      {guide.command(connector.url)}
                    </code>
                  }
                />
              ))}
            </SettingList>
          </>
        ) : (
          // Nothing to do and nothing to set: the disclosure's one row states the
          // fact and what still holds despite it. A row rather than two loose
          // paragraphs, so the sentence a reader who opened this arrives at sits
          // on the same beat as the rows they came from.
          <SettingList>
            <SettingRow
              label={t("agents.connectorOff")}
              description={t("agents.connectorOffDetail")}
              control={null}
            />
          </SettingList>
        )
      }
    </QueryGate>
  );
}

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
      <OverflowMenu label={t("agents.rowActions", { client })}>
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

// connected_at is a record date; the expiry is the holder's own deadline. A
// renewing or revoked row shows no expiry: its badge carries the state.
function useConnectionDates() {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const connected = (connection: Connection) =>
    formatDate(connection.connection.connected_at, locale, recordZone);
  const expires = (connection: Connection, state: ConnectionState) =>
    connection.expires_at != null && !state.revoked && !state.renewing
      ? formatDate(connection.expires_at, locale, viewerZone())
      : "";
  const caption = (connection: Connection, state: ConnectionState) => {
    const deadline = expires(connection, state);
    return [
      t("agents.connectedOn", { date: connected(connection) }),
      deadline &&
        t(state.lapsed ? "agents.expiredOn" : "settings.passportExpiresOn", {
          date: deadline,
        }),
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
  const columns: DataTableColumn<Connection>[] = [
    {
      key: "client",
      header: t("agents.colClient"),
      grow: true,
      fold: "title",
      render: (connection) => (
        <ClientName
          connection={connection}
          state={stateOf(connection)}
          caption={dates.caption(connection, stateOf(connection))}
        />
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
    />
  );
}

// The soonest moment at which some row's status would change if nothing else
// happened — the earliest still-future expiry among the live connections.
// Null when nothing is pending, which is the ordinary case.
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

// Re-render when a credential passes its expiry, because THIS list derives a
// status from the clock and nothing else would notice. The app disables
// refetchOnWindowFocus (main.tsx), so a settings tab left open would otherwise
// keep reporting a connection as live indefinitely — the status is computed at
// render, and without this nothing schedules another one.
//
// One timer at the nearest expiry, not a poll: the boundary is known exactly,
// so waking for it is enough and waking every N seconds to check would be
// waste. Re-running on `until` means each crossing schedules the next.
function useClockAt(until: number | null) {
  const [, setTick] = useState(0);
  useEffect(() => {
    if (until == null) {
      return;
    }
    // setTimeout saturates past ~24.8 days (its delay is a signed 32-bit ms
    // value) and would fire IMMEDIATELY, spinning. A passport lifetime reaches
    // 30 days, so the far ones are simply not scheduled: nobody holds a tab
    // open that long, and the next mount recomputes anyway.
    const delay = until - Date.now();
    if (delay <= 0 || delay > 0x7fffffff) {
      return;
    }
    const timer = globalThis.setTimeout(() => setTick((n) => n + 1), delay);
    return () => globalThis.clearTimeout(timer);
  }, [until]);
}

// Disconnect goes through the connection's grant: revoking the passport alone
// would be undone by the client's next refresh.
export function ConnectedAgentsCard() {
  const t = useT();
  const [confirmId, setConfirmId] = useState<string | null>(null);
  // Where the disconnect confirm hands focus: ending a connection removes its
  // row, so focus lands on what the list still holds.
  const emptyRegion = useRef<HTMLDivElement | null>(null);
  const list = usePassports();

  const disconnect = useMutation({
    mutationFn: async (id: string) => {
      unwrap(await api.DELETE("/passports/{id}", { params: { path: { id } } }));
    },
    onSuccess: async () => {
      // The refetched list first, so focus never lands on a row already gone.
      await list.refetch();
      setConfirmId(null);
    },
  });

  // Read here rather than in the render prop: the expiry timer is a hook.
  const connections = (list.data?.data ?? []).filter(
    (passport): passport is Connection => Boolean(passport.connection),
  );
  const now = Date.now();
  useClockAt(nextExpiry(connections, now));

  return (
    <Panel title={t("agents.connected")}>
      <PanelBody>
        <PanelIntro>{t("agents.connectedSub")}</PanelIntro>
      </PanelBody>
      {list.isSuccess && connections.length > 0 ? (
        <ConnectionTable
          connections={connections}
          now={now}
          onEnd={setConfirmId}
        />
      ) : (
        <PanelBody>
          <div ref={emptyRegion} tabIndex={-1}>
            <QueryStates query={list} pendingLabel={t("agents.connected")}>
              {/* Not the generic empty state: beside a connect guide, "nothing
                  here" reads as a failed load. */}
              <EmptyState>{t("agents.noneConnected")}</EmptyState>
            </QueryStates>
          </div>
        </PanelBody>
      )}
      <PanelBody>
        {/* Open by itself only once the read has answered with nothing, so a
            reader with a connection never sees it flash open. */}
        <Disclosure
          summary={t("agents.connectHow")}
          open={list.isSuccess && connections.length === 0 ? true : undefined}
        >
          <ConnectGuide />
        </Disclosure>
      </PanelBody>
      <ConfirmModal
        open={confirmId != null}
        onClose={() => {
          setConfirmId(null);
          disconnect.reset();
        }}
        title={t("agents.disconnect")}
        confirmLabel={t("agents.disconnect")}
        confirmVariant="danger"
        onConfirm={() => confirmId && disconnect.mutate(confirmId)}
        pending={disconnect.isPending}
        error={disconnect.error ? problemMessageOf(disconnect.error, t) : null}
        returnFocusTo={() =>
          document.querySelector<HTMLElement>("[data-connection]") ??
          emptyRegion.current
        }
      >
        <p>{t("agents.disconnectConfirm")}</p>
      </ConfirmModal>
    </Panel>
  );
}
