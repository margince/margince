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
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { statusLabel, statusTone } from "../../lib/connector-status";
import { labelFor } from "../../lib/labelfor";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import {
  ChangeSharedMailboxDialog,
  ConnectSharedMailboxDialog,
  SHARED_MAILBOXES_KEY,
  SharedMailboxHistoryDialog,
  VISIBILITY_LABEL,
} from "./sharedmailboxes.forms";

// The installation's shared mailboxes (sales@, an operations desk): mailboxes
// no person owns, each read by one team. Every call behind this card is
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
  const connected = mailbox.status !== "disconnected";
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
        action={
          <Button aria-haspopup="dialog" onClick={() => onOpen("change")}>
            {t("sharedMailboxes.change")}
          </Button>
        }
        menu={
          connected && (
            <OverflowMenu
              label={t("table.rowActions", { name: mailbox.address })}
            >
              <Button aria-haspopup="dialog" onClick={() => onOpen("history")}>
                {t("sharedMailboxes.importHistory")}
              </Button>
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
