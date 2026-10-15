import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { connectorsPollInterval } from "../lib/capture-progress";
import {
  problemCode,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../lib/problem";
import { AddConnectionDialog, ALL_PROVIDERS } from "./connectors.add";
import { OAuthOutcomeNote } from "./connectors.notices";
import { ConnectorRow, OAUTH_PROVIDERS } from "./connectors.row";
import { TelegramConnectorsPanel } from "./connectors.telegram";
import { ImapConnectForm } from "./imap-connect-form";
import "./connectors.css";

// Every field shown is a server fact from GET /connectors.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];

// Disconnecting deletes our stored credential and does not revoke the grant at
// the vendor, so the confirm names where to finish that. IMAP has no grant.
const OAUTH_DISCONNECT_NOTE: Partial<Record<Provider, MessageKey>> = {
  gmail: "connectors.disconnectBodyGoogleNote",
  gcal: "connectors.disconnectBodyGoogleNote",
  graph: "connectors.disconnectBodyMicrosoftNote",
  graphcal: "connectors.disconnectBodyMicrosoftNote",
};

export type ConnectorsResult = {
  // A 501 not_implemented is a deployment that never wired mail capture: a
  // documented feature-off state, never an error card.
  notConfigured: boolean;
  data: CaptureConnection[];
  providers: ProviderReadiness[];
  // Absent when none is configured, which is itself the answer.
  publicOrigin?: PublicOriginStatus;
};

type PublicOriginStatus =
  components["schemas"]["CaptureConnectionListResponse"]["public_origin"];

type ProviderReadiness = NonNullable<
  components["schemas"]["CaptureConnectionListResponse"]["providers"]
>[number];

type ConnectFailure = {
  provider: Provider | undefined;
  message: string;
} | null;

// One mutation serves the add dialog and every row's Reconnect. A provider is
// either on the roster or addable, so its variable says which surface reports.
function failureOwnedBy(
  failure: ConnectFailure,
  owner: readonly Provider[],
): string | null {
  if (!failure?.provider || !owner.includes(failure.provider)) {
    return null;
  }
  return failure.message;
}

// Readers share the connector query shape because its cache entry is shared.
export function useConnectors(
  options?: Readonly<{
    /** False holds the request without leaving the key: a reader that only
     *  wants the answer when it already exists still shares the one entry. */
    enabled?: boolean;
    refetchOnWindowFocus?: boolean | "always";
  }>,
) {
  return useQuery({
    ...options,
    queryKey: ["connectors"],
    enabled: options?.enabled ?? true,
    queryFn: async (): Promise<ConnectorsResult> => {
      const { data, error, response } = await api.GET("/connectors");
      if (response.status === 501 && problemCode(error) === "not_implemented") {
        return { notConfigured: true, data: [], providers: [] };
      }
      if (error) {
        throwProblem(error);
      }
      return {
        notConfigured: false,
        data: data.data,
        providers: data.providers ?? [],
        publicOrigin: data.public_origin,
      };
    },
    // Live while a mailbox imports: the orb, the capture chip and the import
    // dialog all draw the run from this read.
    refetchInterval: (query) => connectorsPollInterval(query.state.data),
  });
}

// A probe from inside cannot prove a recipient reaches the address, so only a
// failed one earns a badge.
function PublicOriginRow({
  status,
}: Readonly<{ status?: ConnectorsResult["publicOrigin"] }>) {
  const t = useT();
  if (!status) {
    return null;
  }
  return (
    <SettingRow
      testId="public-origin"
      label={t("connectors.originLabel")}
      description={status.origin}
      control={
        status.reachable === false && (
          <Badge tone="warning">{t("connectors.originUnreachable")}</Badge>
        )
      }
    />
  );
}

function MailRoster({
  rows,
  origin,
  pendingProvider,
  connectFailure,
  addId,
  onAdd,
  onReconnect,
  onDisconnect,
}: Readonly<{
  /** Absent until the roster is known. */
  rows: readonly CaptureConnection[] | undefined;
  origin: ConnectorsResult["publicOrigin"];
  pendingProvider: Provider | null;
  connectFailure: ConnectFailure;
  addId: string;
  /** Absent when every provider is already on the roster. */
  onAdd: (() => void) | undefined;
  onReconnect: (provider: Provider) => void;
  onDisconnect: (provider: Provider) => void;
}>) {
  const t = useT();
  if (rows === undefined && origin === undefined) {
    return null;
  }
  const listed = rows !== undefined && rows.length > 0;
  return (
    <SettingList
      bleed={listed ? "records" : "settings"}
      testId={listed ? "connector-roster" : undefined}
    >
      {rows?.length === 0 && (
        <SettingRow
          testId="connector-roster-empty"
          label={t("connectors.emptyLabel")}
          description={t("connectors.emptyHint")}
          control={
            onAdd && (
              <Button id={addId} onClick={onAdd}>
                {t("connectors.addOpen")}
              </Button>
            )
          }
        />
      )}
      {rows?.map((conn) => (
        <ConnectorRow
          key={conn.id}
          conn={conn}
          connectPending={pendingProvider === conn.provider}
          connectError={failureOwnedBy(connectFailure, [conn.provider])}
          onReconnect={() => onReconnect(conn.provider)}
          onDisconnect={() => onDisconnect(conn.provider)}
        />
      ))}
      <PublicOriginRow status={origin} />
    </SettingList>
  );
}

function useConnect(onNotConfigured: (provider: Provider | null) => void) {
  return useMutation({
    mutationFn: async (provider: Provider) => {
      onNotConfigured(null);
      const { data, error, response } = await api.POST(
        "/connectors/{provider}/connect",
        {
          params: { path: { provider } },
          body: { return_to: "settings" },
        },
      );
      // A provider this deployment never wired is a calm, named state.
      if (response.status === 501 && problemCode(error) === "not_implemented") {
        onNotConfigured(provider);
        return null;
      }
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      if (data?.authorize_url) {
        globalThis.location.assign(data.authorize_url);
      }
    },
  });
}

function useDisconnect(onDone: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (provider: Provider) => {
      unwrap(
        await api.POST("/connectors/{provider}/disconnect", {
          params: { path: { provider } },
        }),
      );
    },
    onSuccess: () => {
      onDone();
      void qc.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}

function RosterState({
  connectors,
}: Readonly<{ connectors: ReturnType<typeof useConnectors> }>) {
  const t = useT();
  return (
    <PanelBody>
      {connectors.isPending && <p>{t("connectors.loading")}</p>}
      {connectors.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("connectors.loadFailed")}
        >
          {problemMessageOf(connectors.error, t)}
        </Callout>
      )}
      {connectors.isSuccess && connectors.data.notConfigured && (
        <EmptyState>
          <p>{t("connectors.notConfigured")}</p>
        </EmptyState>
      )}
    </PanelBody>
  );
}

// The mail half: the roster of mailboxes this member captures from, and the
// header verb that adds another.
function MailConnectorsPanel() {
  const t = useT();
  const [pendingDisconnect, setPendingDisconnect] = useState<Provider | null>(
    null,
  );
  const [imapConnectOpen, setImapConnectOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [notConfigured501, setNotConfigured501] = useState<Provider | null>(
    null,
  );
  const connectors = useConnectors();
  const connect = useConnect(setNotConfigured501);
  const disconnect = useDisconnect(() => setPendingDisconnect(null));

  const notConfigured = connectors.data?.notConfigured ?? false;
  const rows = (connectors.data?.data ?? []).filter(
    (c) => c.status !== "disconnected",
  );
  const disconnectNoteKey = pendingDisconnect
    ? OAUTH_DISCONNECT_NOTE[pendingDisconnect]
    : undefined;
  const present = new Set(rows.map((r) => r.provider));
  const addable = ALL_PROVIDERS.filter((p) => !present.has(p));
  const connectFailure = connect.isError
    ? {
        provider: connect.variables,
        message: problemMessageOf(connect.error, t),
      }
    : null;
  // Until the roster answers, `addable` says "all of them" and would offer a
  // mailbox the reader already has.
  const rosterKnown = connectors.isSuccess && !notConfigured;
  const offerAdd = rosterKnown && addable.length > 0;
  const pendingProvider = connect.isPending ? connect.variables : null;
  const titleId = useId();
  const headAddId = useId();
  const rowAddId = useId();
  // Adding a mailbox moves the verb between the empty row and the head.
  const backToAdd = () =>
    document.getElementById(headAddId) ??
    document.getElementById(rowAddId) ??
    document.getElementById(titleId);

  return (
    <Panel
      title={
        <span id={titleId} tabIndex={-1}>
          {t("connectors.title")}
        </span>
      }
      // On an empty roster the verb rides the one row that says so.
      titleAction={
        offerAdd && rows.length > 0 ? (
          <Button id={headAddId} onClick={() => setAddOpen(true)}>
            {t("connectors.addOpen")}
          </Button>
        ) : undefined
      }
    >
      {!rosterKnown && <RosterState connectors={connectors} />}
      <OAuthOutcomeNote />
      <MailRoster
        rows={rosterKnown ? rows : undefined}
        origin={connectors.data?.publicOrigin}
        pendingProvider={pendingProvider}
        connectFailure={connectFailure}
        addId={rowAddId}
        onAdd={offerAdd ? () => setAddOpen(true) : undefined}
        onReconnect={(provider) =>
          OAUTH_PROVIDERS.has(provider)
            ? connect.mutate(provider)
            : setImapConnectOpen(true)
        }
        onDisconnect={setPendingDisconnect}
      />
      <AddConnectionDialog
        open={addOpen && offerAdd}
        onClose={() => setAddOpen(false)}
        returnFocusTo={backToAdd}
        addable={addable}
        pendingProvider={pendingProvider}
        notConfigured501={notConfigured501}
        connectError={failureOwnedBy(connectFailure, addable)}
        onConnect={(p) => connect.mutate(p)}
        onImap={() => {
          setNotConfigured501(null);
          // Closed before the IMAP form opens: two overlays deep, Escape and
          // the focus restore answer to the wrong layer.
          setAddOpen(false);
          setImapConnectOpen(true);
        }}
      />
      <ConfirmModal
        open={pendingDisconnect !== null}
        onClose={() => setPendingDisconnect(null)}
        title={t("connectors.disconnectTitle")}
        confirmLabel={t("connectors.disconnect")}
        confirmVariant="danger"
        pending={disconnect.isPending}
        error={
          disconnect.isError ? problemMessageOf(disconnect.error, t) : null
        }
        onConfirm={() => {
          if (pendingDisconnect !== null) {
            disconnect.mutate(pendingDisconnect);
          }
        }}
      >
        <p>{t("connectors.disconnectBody")}</p>
        {disconnectNoteKey && (
          <p className="t-caption">{t(disconnectNoteKey)}</p>
        )}
      </ConfirmModal>
      <ImapConnectForm
        open={imapConnectOpen}
        onClose={() => setImapConnectOpen(false)}
        onConnected={() => setImapConnectOpen(false)}
      />
    </Panel>
  );
}

export function ConnectorsCard() {
  return (
    <>
      <MailConnectorsPanel />
      <TelegramConnectorsPanel />
    </>
  );
}
