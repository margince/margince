// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Send } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { problemCode, problemMessageOf, throwProblem, useMe } from "./common";
import { statusLabel, statusTone } from "./connector-status";
import { ConnectionIdentity } from "./connectors.identity";
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

// One live bot as a row: which bot it is on the left, whether it is live on the
// right, and whichever of the two verbs that change it this reader holds.
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
  return (
    <SettingRow
      testId="telegram-connection"
      label={
        <ConnectionIdentity
          icon={Send}
          name={t("connectors.provTelegram")}
          account={`@${connection.channelLabel}`}
        />
      }
      value={
        <Badge tone={statusTone(connection.status)}>
          {t(statusLabel(connection.status))}
        </Badge>
      }
      control={
        (onEdit || onDisconnect) && (
          <div className="connector-actions">
            {onEdit && (
              <Button onClick={onEdit}>
                <RefreshCw aria-hidden /> {t("connectors.telegramEditToken")}
              </Button>
            )}
            {onDisconnect && (
              <Button variant="ghost" onClick={onDisconnect}>
                {t("connectors.disconnect")}
              </Button>
            )}
          </div>
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
      const { error } = await api.DELETE("/channel-connections/{id}", {
        params: { path: { id: connection.id } },
      });
      if (error) {
        throwProblem(error);
      }
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
    <Panel
      title={t("connectors.telegramTitle")}
      // The connect verb in the header, the same shape the mail card next to it
      // takes: as the zero state's row it was labelled "Telegram" under a card
      // titled "Telegram bot" — the card's own subject, said twice, with the
      // act beside it.
      titleAction={
        canCreate &&
        live &&
        connections.length === 0 && (
          <Button
            data-testid="telegram-connect"
            onClick={() => setConnectOpen(true)}
          >
            <Send aria-hidden /> {t("connectors.telegramConnectCta")}
          </Button>
        )
      }
    >
      <PanelBody>
        {/* In the BODY, not `Panel`'s `sub`. A description in the header band
            raises that band's own height, so this card's title sat lower than
            every sibling's on the tab and the whole page lost its beat over one
            sentence. Read here it is also the first thing under the title
            rather than a second line competing with it. */}
        <PanelIntro>{t("connectors.telegramSub")}</PanelIntro>
        {readOnly && (
          <PanelIntro>{t("connectors.telegramReadOnly")}</PanelIntro>
        )}
        <TelegramNotice query={query} />
        {live && (
          <SettingList>
            {connections.length === 0 ? (
              // What the card is FOR, in the roster's own place: which bot is
              // carrying messages. The verb that changes it is in the header.
              <SettingRow
                label={t("connectors.telegramRosterLabel")}
                layout="stack"
                control={
                  <EmptyState>{t("connectors.telegramEmpty")}</EmptyState>
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
                    canDisconnect
                      ? () => setDisconnecting(connection)
                      : undefined
                  }
                />
              ))
            )}
          </SettingList>
        )}
      </PanelBody>
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
