// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Send } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { KeyedName } from "../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import {
  problemCode,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../lib/problem";
import { useMe } from "../lib/session";
import { statusLabel, statusTone } from "./connector-status";
import { TelegramConnectForm } from "./telegram-connect-form";

// The workspace's messaging bot, as its own file. It shares the connections
// card with the mail roster and nothing else: a different endpoint, a different
// scope — one bot for the workspace against a roster per user — and its own
// three grants. Only ConnectorsCard, which renders the pair, spans both.

type ChannelConnection = components["schemas"]["ChannelConnection"];

type ChannelConnectionsResult = {
  // GET /channel-connections answers 503 when this deployment serves no
  // messaging channels, or has no credential store to seal a bot token in — a
  // calm, documented feature-off state, mirroring the mail card's 501
  // not_implemented treatment above rather than an error card.
  notConfigured: boolean;
  data: ChannelConnection[];
};

function useChannelConnections() {
  return useQuery({
    queryKey: ["channel-connections"],
    queryFn: async (): Promise<ChannelConnectionsResult> => {
      const { data, error, response } = await api.GET("/channel-connections");
      if (
        response.status === 503 &&
        (problemCode(error) === "channel_connections_not_configured" ||
          problemCode(error) === "channel_credentials_not_configured")
      ) {
        return { notConfigured: true, data: [] };
      }
      if (error) {
        throwProblem(error);
      }
      return { notConfigured: false, data: data.data };
    },
  });
}

// One live bot as a row: its handle, a badge only when it is not live, and
// whichever of the two verbs this reader holds.
function TelegramConnectionRow({
  connection,
  onEdit,
  onDisconnect,
}: Readonly<{
  connection: ChannelConnection;
  onEdit?: () => void;
  onDisconnect?: () => void;
}>) {
  const t = useT();
  const handle = `@${connection.channelLabel}`;
  return (
    <SettingRow
      testId="telegram-connection"
      label={
        <span className="connector-head">
          <KeyedName name={t("connectors.provTelegram")} code={handle} />
          {connection.status !== "connected" && (
            <Badge tone={statusTone(connection.status)}>
              {t(statusLabel(connection.status))}
            </Badge>
          )}
        </span>
      }
      control={
        (onEdit || onDisconnect) && (
          <span className="cell-actions">
            <OverflowMenu label={t("table.rowActions", { name: handle })}>
              {onEdit && (
                <Button aria-haspopup="dialog" onClick={onEdit}>
                  {t("connectors.telegramEditToken")}
                </Button>
              )}
              {onDisconnect && (
                <Button
                  variant="danger"
                  aria-haspopup="dialog"
                  onClick={onDisconnect}
                >
                  {t("connectors.disconnect")}
                </Button>
              )}
            </OverflowMenu>
          </span>
        )
      }
    />
  );
}

// Everything the Telegram panel shows INSTEAD of its rows. Split out so the
// panel function keeps one return and its hooks stay unconditional.
function TelegramNotice({
  query,
}: Readonly<{ query: ReturnType<typeof useChannelConnections> }>) {
  const t = useT();
  if (query.isPending) {
    return <p>{t("connectors.loading")}</p>;
  }
  if (query.isError) {
    return (
      <Callout tone="danger" kind="outcome" title={t("connectors.loadFailed")}>
        {problemMessageOf(query.error, t)}
      </Callout>
    );
  }
  if (query.data.notConfigured) {
    return (
      <EmptyState>
        <p>{t("connectors.telegramNotConfigured")}</p>
      </EmptyState>
    );
  }
  return null;
}

// The workspace's messaging bot, as its own panel.
//
// A bot connects for the WHOLE workspace rather than per-user (Task 17,
// design §9.1/§9.2), and a send needs exactly one of them: with a second
// live bot the workspace can send nothing at all until an admin removes it.
// This panel is the only surface that can, so it must show every connection
// the list returns — a bot it hides is a bot nobody can disconnect. Every one
// of them is a row of its own for exactly that reason.
//
// Editing goes through the SAME TelegramConnectForm modal, whose PATCH takes
// the place of a disconnect-reconnect cycle (§9.2). The panel mounts one
// form instance, keyed to whichever row opened it.
export function TelegramConnectorsPanel() {
  const t = useT();
  const qc = useQueryClient();
  const query = useChannelConnections();
  const [connectOpen, setConnectOpen] = useState(false);
  const [editingConnection, setEditingConnection] =
    useState<ChannelConnection | null>(null);
  const [disconnecting, setDisconnecting] = useState<ChannelConnection | null>(
    null,
  );

  const disconnect = useMutation({
    mutationFn: async (connection: ChannelConnection) => {
      unwrap(
        await api.DELETE("/channel-connections/{id}", {
          params: { path: { id: connection.id } },
        }),
      );
    },
    onSuccess: () => {
      setDisconnecting(null);
      void qc.invalidateQueries({ queryKey: ["channel-connections"] });
    },
  });

  // Three grants, three verbs: the seeded matrix moves them together for
  // admin and ops, but an operator may narrow one alone.
  const canCreate = useCanWrite("channel_connection", "create");
  const canEdit = useCanWrite("channel_connection", "update");
  const canDisconnect = useCanWrite("channel_connection", "delete");
  // The probe, not just its answer: every grant reads false while /me is in
  // flight, and the read-only line must not flash at an admin on each load.
  const me = useMe();
  const live = query.isSuccess && !query.data.notConfigured;
  const connections = live ? query.data.data : [];
  // A deployment with no messaging channels withholds the verbs from every
  // seat, so the not-configured notice is the reason and this line stays quiet.
  const readOnly =
    me.isSuccess && live && !canCreate && !canEdit && !canDisconnect;
  const closeForms = () => {
    setConnectOpen(false);
    setEditingConnection(null);
  };

  return (
    <Panel title={t("connectors.telegramTitle")}>
      {(!live || readOnly) && (
        <PanelBody>
          {readOnly && (
            <PanelIntro>{t("connectors.telegramReadOnly")}</PanelIntro>
          )}
          <TelegramNotice query={query} />
        </PanelBody>
      )}
      {live && (
        <SettingList bleed={connections.length > 0 ? "records" : "settings"}>
          {connections.length === 0 ? (
            <SettingRow
              testId="telegram-empty"
              label={t("connectors.telegramEmptyLabel")}
              description={t("connectors.telegramSub")}
              control={
                canCreate && (
                  <Button
                    data-testid="telegram-connect"
                    onClick={() => setConnectOpen(true)}
                  >
                    <Send aria-hidden /> {t("connectors.telegramConnectCta")}
                  </Button>
                )
              }
            />
          ) : (
            connections.map((connection) => (
              <TelegramConnectionRow
                key={connection.id}
                connection={connection}
                onEdit={
                  canEdit ? () => setEditingConnection(connection) : undefined
                }
                onDisconnect={
                  canDisconnect ? () => setDisconnecting(connection) : undefined
                }
              />
            ))
          )}
        </SettingList>
      )}
      <TelegramConnectForm
        // Keyed to the row that opened it, so the form never carries one
        // connection's in-progress state onto another's rotation.
        key={editingConnection?.id ?? "new"}
        open={connectOpen || editingConnection !== null}
        connection={editingConnection ?? undefined}
        onClose={closeForms}
        onConnected={closeForms}
      />
      <ConfirmModal
        open={disconnecting !== null}
        onClose={() => setDisconnecting(null)}
        title={t("connectors.telegramDisconnectTitle")}
        confirmLabel={t("connectors.disconnect")}
        confirmVariant="danger"
        pending={disconnect.isPending}
        error={
          disconnect.isError ? problemMessageOf(disconnect.error, t) : null
        }
        onConfirm={() => {
          if (disconnecting) {
            disconnect.mutate(disconnecting);
          }
        }}
      >
        <p>{t("connectors.telegramDisconnectBody")}</p>
      </ConfirmModal>
    </Panel>
  );
}
