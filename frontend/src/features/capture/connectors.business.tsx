// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageCircle } from "lucide-react";
import { useState } from "react";
import { isTeam, VISIBILITY_LABEL } from "../../components/readerteam";
import { RosterFailedLine, useRoster } from "../../components/roster";
import { Button, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { statusLabel, statusTone } from "../../lib/connector-status";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf } from "../../lib/problem";
import {
  type BusinessAccount,
  type BusinessProvider,
  businessTitles,
  disconnectBusinessAccount,
  listBusinessAccounts,
} from "./connectors.businessapi";
import {
  BusinessReadersDialog,
  ConnectBusinessDialog,
} from "./connectors.businessdialogs";

export function BusinessAccountsPanel({
  provider,
}: Readonly<{ provider: BusinessProvider }>) {
  const t = useT();
  const qc = useQueryClient();
  const canConnect = useCanWrite("channel_connection", "create");
  const canChange = useCanWrite("channel_connection", "update");
  const canDisconnect = useCanWrite("channel_connection", "delete");
  const allowed = canChange;
  const queryKey = ["business-accounts", provider];
  const query = useQuery({
    queryKey,
    enabled: allowed,
    queryFn: () => listBusinessAccounts(provider),
  });
  const roster = useRoster("team", allowed);
  const teams = (roster.data ?? []).filter(isTeam);
  const [connecting, setConnecting] = useState(false);
  const [changing, setChanging] = useState<BusinessAccount | null>(null);
  const [disconnecting, setDisconnecting] = useState<BusinessAccount | null>(
    null,
  );
  const disconnect = useMutation({
    mutationFn: (account: BusinessAccount) =>
      disconnectBusinessAccount(provider, account.id),
    onSuccess: async () => {
      setDisconnecting(null);

      await qc.invalidateQueries({ queryKey });
    },
  });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey });
  };
  if (!allowed) return null;
  return (
    <Panel
      title={t(businessTitles[provider])}
      actions={
        canConnect && query.isSuccess && !query.data.notConfigured ? (
          <Button aria-haspopup="dialog" onClick={() => setConnecting(true)}>
            {t("connectors.business.connect")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("connectors.business.intro")}</PanelIntro>
        <QueryStates query={query} pendingLabel={t(businessTitles[provider])}>
          {query.data?.notConfigured && (
            <EmptyState
              plate
              title={t("connectors.telegramNotConfiguredTitle")}
            >
              {t("connectors.telegramNotConfigured")}
            </EmptyState>
          )}
          {query.isSuccess &&
            !query.data.notConfigured &&
            query.data.data.length === 0 && (
              <EmptyState>{t("connectors.business.empty")}</EmptyState>
            )}
          <ProviderList>
            {(query.data?.data ?? []).map((account) => (
              <BusinessAccountRow
                key={account.id}
                account={account}
                teamName={
                  teams.find((team) => team.id === account.readerTeamId)
                    ?.name ?? t("sharedMailboxes.unknownTeam")
                }
                onReaders={
                  canChange && account.status === "connected"
                    ? () => setChanging(account)
                    : undefined
                }
                onDisconnect={
                  canDisconnect && account.status !== "disconnected"
                    ? () => {
                        disconnect.reset();
                        setDisconnecting(account);
                      }
                    : undefined
                }
              />
            ))}
          </ProviderList>
        </QueryStates>
        <RosterFailedLine kind="team" roster={roster} />
      </PanelBody>
      {connecting && (
        <ConnectBusinessDialog
          provider={provider}
          teams={teams}
          onClose={() => {
            setConnecting(false);
            refresh();
          }}
          onDone={() => {
            setConnecting(false);
            refresh();
          }}
        />
      )}
      {changing && (
        <BusinessReadersDialog
          provider={provider}
          account={changing}
          onClose={() => setChanging(null)}
          onDone={() => {
            setChanging(null);
            refresh();
          }}
        />
      )}
      <ConfirmModal
        open={disconnecting !== null}
        onClose={() => setDisconnecting(null)}
        title={t("connectors.business.disconnectTitle", {
          name: disconnecting?.name ?? "",
        })}
        confirmLabel={t("connectors.disconnect")}
        confirmVariant="danger"
        pending={disconnect.isPending}
        error={
          disconnect.isError ? problemMessageOf(disconnect.error, t) : null
        }
        onConfirm={() => {
          if (disconnecting) disconnect.mutate(disconnecting);
        }}
      >
        <p>{t("connectors.business.disconnectHelp")}</p>
      </ConfirmModal>
    </Panel>
  );
}

function BusinessAccountRow({
  account,
  teamName,
  onReaders,
  onDisconnect,
}: Readonly<{
  account: BusinessAccount;
  teamName: string;
  onReaders?: () => void;
  onDisconnect?: () => void;
}>) {
  const t = useT();
  return (
    <ProviderRow
      mark={<MessageCircle />}
      name={account.name}
      status={{
        tone: statusTone(account.status) ?? "default",
        label: t(
          account.status === "reauth_required"
            ? "connectors.business.reauth"
            : statusLabel(account.status),
        ),
      }}
      facts={t("sharedMailboxes.readBy", {
        team: teamName,
        visibility: t(
          labelFor(VISIBILITY_LABEL, account.mailVisibility ?? "held"),
        ),
      })}
      reason={
        account.status === "reauth_required"
          ? t("connectors.business.reauthHelp")
          : undefined
      }
      action={
        onReaders ? (
          <Button aria-haspopup="dialog" onClick={onReaders}>
            {t("connectors.tgbiz.changeReaders")}
          </Button>
        ) : undefined
      }
      menu={
        onDisconnect ? (
          <OverflowMenu label={t("table.rowActions", { name: account.name })}>
            <Button aria-haspopup="dialog" onClick={onDisconnect}>
              {t("connectors.disconnect")}
            </Button>
          </OverflowMenu>
        ) : undefined
      }
    />
  );
}
