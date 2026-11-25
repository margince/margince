// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Briefcase } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { RosterFailedLine, useRoster } from "../../components/roster";
import { Button, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCanWrite } from "../../lib/capability";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { isTeam, VISIBILITY_LABEL } from "../settings/sharedmailboxes.forms";
import { useChannelConnections } from "./connectors.telegram";
import {
  ApproveBusinessDialog,
  BusinessVisibilityDialog,
} from "./connectors.telegrambusiness.dialogs";

// The Telegram Business accounts whose owners linked the workspace's bot inside
// Telegram. A link waits here until an admin approves it as a company account;
// nothing from it is stored before that. It sits under the bot panel because a
// link can only exist while that bot is connected.

type BusinessConnection = components["schemas"]["TelegramBusinessConnection"];

const LIST_KEY = ["telegram-business-connections"] as const;

const STATUS: Readonly<
  Record<
    BusinessConnection["status"],
    { tone: "default" | "success" | "warning"; label: MessageKey }
  >
> = {
  pending_approval: {
    tone: "warning",
    label: "connectors.tgbiz.statusPending",
  },
  connected: { tone: "success", label: "connectors.tgbiz.statusConnected" },
  disconnected: { tone: "default", label: "connectors.tgbiz.statusOff" },
};

function useBusinessConnections(enabled: boolean) {
  return useQuery({
    queryKey: LIST_KEY,
    enabled,
    queryFn: async (): Promise<BusinessConnection[]> => {
      const { data, error } = await api.GET(
        "/channel-connections/telegram-business",
      );
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

// Who reads an approved account: its reader team and the seat's visibility.
// A member's own account has no team and holds each chat.
function readers(
  connection: BusinessConnection,
  teamName: string,
  t: ReturnType<typeof useT>,
): string | null {
  const { mailVisibility } = connection;
  if (mailVisibility === "classified") {
    return t("connectors.tgbiz.ownAccount");
  }
  if (!connection.readerTeamId || !mailVisibility) {
    return null;
  }
  return t("sharedMailboxes.readBy", {
    team: teamName,
    visibility: t(labelFor(VISIBILITY_LABEL, mailVisibility)),
  });
}

function BusinessRow({
  connection,
  teamName,
  onApprove,
  onVisibility,
  onDisconnect,
}: Readonly<{
  connection: BusinessConnection;
  teamName: string;
  onApprove?: () => void;
  onVisibility?: () => void;
  onDisconnect?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const status = STATUS[connection.status];
  const approve = connection.status === "pending_approval" && onApprove;
  const readBy = readers(connection, teamName, t);
  const changeReaders =
    connection.status === "connected" &&
    connection.readerTeamId &&
    onVisibility;
  return (
    <div data-testid="telegram-business-connection">
      <ProviderRow
        mark={<Briefcase />}
        name={connection.ownerLabel}
        status={{ tone: status.tone, label: t(status.label) }}
        facts={
          <>
            {t("connectors.tgbiz.linked", {
              date: formatDate(connection.connectedAt, locale, zone),
            })}
            {connection.approvedAt
              ? ` · ${t("connectors.tgbiz.approvedOn", {
                  date: formatDate(connection.approvedAt, locale, zone),
                })}`
              : null}
            {readBy ? ` · ${readBy}` : null}
          </>
        }
        reason={
          connection.status === "disconnected"
            ? t("connectors.tgbiz.offReason")
            : undefined
        }
        action={
          approve || changeReaders || onDisconnect ? (
            <>
              {approve && (
                <Button aria-haspopup="dialog" onClick={onApprove}>
                  {t("connectors.tgbiz.approve")}
                </Button>
              )}
              {changeReaders && (
                <Button aria-haspopup="dialog" onClick={onVisibility}>
                  {t("connectors.tgbiz.changeReaders")}
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

// The panel withdraws itself unless a Telegram bot is connected and the reader
// may approve or disconnect: the list answers 403 to anyone else.
export function TelegramBusinessPanel() {
  const t = useT();
  const qc = useQueryClient();
  const canApprove = useCanWrite("channel_connection", "update");
  const canDisconnect = useCanWrite("channel_connection", "delete");
  const bots = useChannelConnections();
  const bot = bots.data?.data.find(
    (connection) =>
      connection.provider === "telegram" && connection.status === "connected",
  );
  const allowed = canApprove || canDisconnect;
  const query = useBusinessConnections(Boolean(bot) && allowed);
  const roster = useRoster("team", Boolean(bot) && allowed);
  const teams = (roster.data ?? []).filter(isTeam);
  const teamName = (id: string | null | undefined) =>
    teams.find((team) => team.id === id)?.name ??
    t("sharedMailboxes.unknownTeam");
  const [approving, setApproving] = useState<BusinessConnection | null>(null);
  const [changing, setChanging] = useState<BusinessConnection | null>(null);
  const [disconnecting, setDisconnecting] = useState<BusinessConnection | null>(
    null,
  );

  const refresh = () => qc.invalidateQueries({ queryKey: LIST_KEY });
  const disconnect = useMutation({
    mutationFn: async (connection: BusinessConnection) => {
      unwrap(
        await api.DELETE("/channel-connections/telegram-business/{id}", {
          params: { path: { id: connection.id } },
        }),
      );
    },
    onSuccess: () => {
      setDisconnecting(null);
      void refresh();
    },
  });

  if (!bot || !allowed) {
    return null;
  }
  const connections = query.data ?? [];
  const closeDisconnect = () => {
    setDisconnecting(null);
    disconnect.reset();
  };
  const owner = disconnecting?.ownerLabel ?? "";
  return (
    <Panel title={t("connectors.tgbiz.title")}>
      <PanelBody>
        <PanelIntro>{t("connectors.tgbiz.intro")}</PanelIntro>
        <p>{t("connectors.tgbiz.stepsTitle")}</p>
        <ol>
          <li>{t("connectors.tgbiz.step1")}</li>
          <li>{t("connectors.tgbiz.step2", { username: bot.channelLabel })}</li>
          <li>{t("connectors.tgbiz.step3")}</li>
        </ol>
        <QueryStates query={query} pendingLabel={t("connectors.tgbiz.title")}>
          {query.isSuccess && connections.length === 0 && (
            <EmptyState plate title={t("connectors.tgbiz.emptyTitle")}>
              {t("connectors.tgbiz.emptyNote")}
            </EmptyState>
          )}
          {connections.length > 0 && (
            <ProviderList>
              {connections.map((connection) => (
                <BusinessRow
                  key={connection.id}
                  connection={connection}
                  teamName={teamName(connection.readerTeamId)}
                  onApprove={
                    canApprove ? () => setApproving(connection) : undefined
                  }
                  onVisibility={
                    canApprove ? () => setChanging(connection) : undefined
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
        <RosterFailedLine kind="team" roster={roster} />
      </PanelBody>
      {approving && (
        <ApproveBusinessDialog
          connection={approving}
          teams={teams}
          onClose={() => setApproving(null)}
          onDone={() => {
            setApproving(null);
            void refresh();
          }}
          onSwitchedOff={() => void refresh()}
        />
      )}
      {changing && (
        <BusinessVisibilityDialog
          connection={changing}
          onClose={() => setChanging(null)}
          onDone={() => {
            setChanging(null);
            void refresh();
          }}
        />
      )}
      <ConfirmModal
        open={disconnecting !== null}
        onClose={closeDisconnect}
        title={t("connectors.tgbiz.disconnectTitle", { owner })}
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
        <p>{t("connectors.tgbiz.disconnectBody")}</p>
      </ConfirmModal>
    </Panel>
  );
}
