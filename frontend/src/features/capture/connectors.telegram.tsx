// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, Send } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { statusLabel, statusTone } from "../../lib/connector-status";
import {
  problemCode,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "../../lib/problem";
import { useMe } from "../../lib/session";
import { TelegramConnectForm } from "./telegram-connect-form";

// The workspace's messaging bot: one bot for the whole company, read from
// /channel-connections and gated on its own three grants. It sits on
// Integrations beside the other company wiring, not among a member's mailboxes.

type ChannelConnection = components["schemas"]["ChannelConnection"];

type ChannelConnectionsResult = {
  // GET /channel-connections answers 503 when this deployment serves no
  // messaging channels or has no credential store to seal a bot token in. That
  // is a documented feature-off state, treated like the mail card's 501
  // not_implemented rather than as an error card.
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

// One live bot as a row. Which bot it is sits on the left and whether it is live
// on the right. Then come whichever of the two verbs that change it this reader
// holds.
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
  const tone = statusTone(connection.status);
  return (
    // The wrapper names the row for its tests; the hairline between rows is
    // the list's, drawn between any two children.
    <div data-testid="telegram-connection">
      <ProviderRow
        mark={<Send />}
        name={t("connectors.provTelegram")}
        account={`@${connection.channelLabel}`}
        status={{
          tone: tone ?? "default",
          label: t(statusLabel(connection.status)),
        }}
        action={
          onEdit || onDisconnect ? (
            <>
              {onEdit && (
                <Button aria-haspopup="dialog" onClick={onEdit}>
                  <RefreshCw aria-hidden /> {t("connectors.telegramEditToken")}
                </Button>
              )}
              {onDisconnect && (
                <Button
                  variant="ghost"
                  aria-haspopup="dialog"
                  onClick={onDisconnect}
                >
                  {t("connectors.disconnect")}
                </Button>
              )}
            </>
          ) : undefined
        }
      />
    </div>
  );
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
  // No bot and no way to add one leaves the card nothing to show or offer.
  if (readOnly && connections.length === 0) return null;

  return (
    <Panel
      title={t("connectors.telegramTitle")}
      // The connect verb in the header, the shape every connections card takes;
      // as a zero-state row it named the card's own subject twice.
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
        <QueryStates query={query} pendingLabel={t("connectors.telegramTitle")}>
          {query.data?.notConfigured && (
            <EmptyState
              plate
              title={t("connectors.telegramNotConfiguredTitle")}
            >
              {t("connectors.telegramNotConfigured")}
            </EmptyState>
          )}
          {/* The connect verb is in the header, so the plate only says what a
              bot here would do. */}
          {live && connections.length === 0 && (
            <EmptyState plate title={t("connectors.telegramEmptyTitle")}>
              {t("connectors.telegramEmptyNote")}
            </EmptyState>
          )}
          {connections.length > 0 && (
            <ProviderList>
              {connections.map((connection) => (
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
              ))}
            </ProviderList>
          )}
        </QueryStates>
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
