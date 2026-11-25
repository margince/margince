// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Inbox, Plus } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { RosterFailedLine, useRoster } from "../../components/roster";
import { Button, EmptyState } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { OverflowMenu } from "../../design-system/overflowmenu";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryStates } from "../../design-system/querystates";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { statusLabel, statusTone } from "../../lib/connector-status";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import {
  ChangeSharedMailboxDialog,
  ConnectSharedMailboxDialog,
  SHARED_MAILBOXES_KEY,
  SharedMailboxHistoryDialog,
  VISIBILITY_LABEL,
} from "./sharedmailboxes.forms";

// The installation's shared mailboxes (sales@, an operations desk): mailboxes
// no single user owns, each read by one team. Every call behind this card is
// admin-only, so a reader without the role sees why rather than an error.

type SharedMailbox = components["schemas"]["SharedMailbox"];
type Team = components["schemas"]["Team"];

// The team roster's entries, typed as teams: the shared roster cache holds
// seats and teams under one entry type.
function isTeam(entry: { id: string }): entry is Team {
  return "name" in entry;
}

type SharedMailboxesResult =
  | { allowed: false }
  | { allowed: true; items: SharedMailbox[] };

function useSharedMailboxes() {
  return useQuery({
    queryKey: SHARED_MAILBOXES_KEY,
    queryFn: async (): Promise<SharedMailboxesResult> => {
      const { data, error, response } = await api.GET("/shared-mailboxes");
      if (response.status === 403) {
        return { allowed: false };
      }
      if (error) {
        throwProblem(error);
      }
      return { allowed: true, items: data.items };
    },
    // The server narrows earlier mail one batch every ten seconds.
    refetchInterval: (query) => {
      const answer = query.state.data;
      return answer?.allowed && answer.items.some((m) => m.history_narrowing)
        ? 10_000
        : false;
    },
  });
}

const PROVIDER_LABEL: Record<SharedMailbox["provider"], MessageKey> = {
  imap: "firstRun.platform.imap",
  graph: "firstRun.platform.microsoft",
  gmail: "firstRun.platform.google",
  test_mailbox: "firstRun.platform.imap",
};

type RowDialog = {
  kind: "change" | "history" | "disconnect";
  mailbox: SharedMailbox;
} | null;

export function SharedMailboxesCard() {
  const t = useT();
  const query = useSharedMailboxes();
  const allowed = query.data?.allowed === true;
  const roster = useRoster("team", allowed);
  const teams = (roster.data ?? []).filter(isTeam);
  const [connecting, setConnecting] = useState(false);
  const [dialog, setDialog] = useState<RowDialog>(null);
  const teamName = (id: string) =>
    teams.find((team) => team.id === id)?.name ??
    t("sharedMailboxes.unknownTeam");
  const items = query.data?.allowed ? query.data.items : [];
  return (
    <Panel
      title={t("sharedMailboxes.title")}
      titleAction={
        allowed && (
          <Button onClick={() => setConnecting(true)} aria-haspopup="dialog">
            <Plus aria-hidden /> {t("sharedMailboxes.connect")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("sharedMailboxes.intro")}</PanelIntro>
        <QueryStates query={query} pendingLabel={t("sharedMailboxes.title")}>
          {query.data?.allowed === false && (
            <EmptyState plate title={t("sharedMailboxes.adminOnlyTitle")}>
              {t("sharedMailboxes.adminOnly")}
            </EmptyState>
          )}
          {allowed && items.length === 0 && (
            <EmptyState plate title={t("sharedMailboxes.emptyTitle")}>
              {t("sharedMailboxes.empty")}
            </EmptyState>
          )}
          {items.length > 0 && (
            <ProviderList>
              {items.map((mailbox) => (
                <SharedMailboxRow
                  key={mailbox.id}
                  mailbox={mailbox}
                  teamName={teamName(mailbox.reader_team_id)}
                  onOpen={(kind) => setDialog({ kind, mailbox })}
                />
              ))}
            </ProviderList>
          )}
        </QueryStates>
        <RosterFailedLine kind="team" roster={roster} />
      </PanelBody>
      {connecting && (
        <ConnectSharedMailboxDialog
          teams={teams}
          onClose={() => setConnecting(false)}
        />
      )}
      {dialog?.kind === "change" && (
        <ChangeSharedMailboxDialog
          mailbox={dialog.mailbox}
          teams={teams}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "history" && (
        <SharedMailboxHistoryDialog
          mailbox={dialog.mailbox}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === "disconnect" && (
        <DisconnectDialog
          mailbox={dialog.mailbox}
          onClose={() => setDialog(null)}
        />
      )}
    </Panel>
  );
}

function SharedMailboxRow({
  mailbox,
  teamName,
  onOpen,
}: Readonly<{
  mailbox: SharedMailbox;
  teamName: string;
  onOpen: (kind: "change" | "history" | "disconnect") => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const connected = mailbox.status !== "disconnected";
  const unread = mailbox.unread_period;
  const narrowing = mailbox.history_narrowing;
  // A span no sync read is what the row needs an admin for, so the import
  // that recovers it takes the visible verb.
  const importFirst = connected && unread != null;
  const change = (
    <Button aria-haspopup="dialog" onClick={() => onOpen("change")}>
      {t("sharedMailboxes.change")}
    </Button>
  );
  const importHistory = (
    <Button aria-haspopup="dialog" onClick={() => onOpen("history")}>
      {t("sharedMailboxes.importHistory")}
    </Button>
  );
  return (
    <div data-testid="shared-mailbox">
      <ProviderRow
        mark={<Inbox />}
        name={mailbox.address}
        account={t(labelFor(PROVIDER_LABEL, mailbox.provider))}
        status={{
          tone: statusTone(mailbox.status) ?? "default",
          label: t(statusLabel(mailbox.status)),
        }}
        facts={t("sharedMailboxes.readBy", {
          team: teamName,
          visibility: t(labelFor(VISIBILITY_LABEL, mailbox.mail_visibility)),
        })}
        reason={
          [
            unread &&
              t("captureHealth.unreadPeriodFolded", {
                from: formatDateTime(unread.from, locale, zone),
                to: formatDateTime(unread.to, locale, zone),
              }),
            narrowing &&
              plural(
                "connectors.mailPosture.historyNarrowing",
                narrowing.remaining,
                {
                  count: formatNumber(narrowing.remaining, locale),
                },
              ),
          ]
            .filter(Boolean)
            .join(" ") || undefined
        }
        action={importFirst ? importHistory : change}
        menu={
          connected && (
            <OverflowMenu
              label={t("table.rowActions", { name: mailbox.address })}
            >
              {importFirst ? change : importHistory}
              <Button
                aria-haspopup="dialog"
                onClick={() => onOpen("disconnect")}
              >
                {t("connectors.disconnect")}
              </Button>
            </OverflowMenu>
          )
        }
      />
    </div>
  );
}

function DisconnectDialog({
  mailbox,
  onClose,
}: Readonly<{ mailbox: SharedMailbox; onClose: () => void }>) {
  const t = useT();
  const qc = useQueryClient();
  const disconnect = useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.POST("/shared-mailboxes/{id}/disconnect", {
          params: { path: { id } },
        }),
      );
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SHARED_MAILBOXES_KEY });
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("sharedMailboxes.disconnectTitle", { address: mailbox.address })}
      confirmLabel={t("connectors.disconnect")}
      confirmVariant="danger"
      pending={disconnect.isPending}
      error={disconnect.isError ? problemMessageOf(disconnect.error, t) : null}
      onConfirm={() => disconnect.mutate(mailbox.id)}
    >
      <p>{t("sharedMailboxes.disconnectBody")}</p>
    </ConfirmModal>
  );
}
