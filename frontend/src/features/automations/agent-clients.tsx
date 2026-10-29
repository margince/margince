// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  OverflowMenu,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyedName } from "../../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { useLastSubject } from "../../design-system/presence";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import { formatDate, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import { AgentClientExperimentsCard } from "./agent-client-experiments";
import { AgentClientMethodsCard } from "./agent-client-methods";
import {
  AgentClientValuesDialog,
  RegisterAgentClientDialog,
} from "./agent-client-register";
import {
  type AgentClient,
  type AgentClientSecret,
  useAgentClients,
  useDeleteAgentClient,
  useRotateAgentClientSecret,
  useSetAgentClientDisabled,
} from "./agent-clients.queries";
import { AgentConnectionsCard } from "./agent-token-settings";
import "./agent-clients.css";

type Rights = Readonly<{ update: boolean; delete: boolean }>;

// How each client came to exist, keyed by the wire value so a new value fails
// to compile here instead of falling through.
const REGISTRATION: Record<
  AgentClient["registered_by"],
  Readonly<{ label: MessageKey; deleteBody: MessageKey }>
> = {
  admin: {
    label: "agentClients.by.admin",
    deleteBody: "agentClients.delete.bodyAdmin",
  },
  dcr: {
    label: "agentClients.by.dcr",
    deleteBody: "agentClients.delete.bodyDcr",
  },
  cimd: {
    label: "agentClients.by.cimd",
    deleteBody: "agentClients.delete.bodyCimd",
  },
};

// A rotate needs a secret to replace; only a client registered by hand holds one.
function rotates(client: AgentClient): boolean {
  return client.confidential;
}

function RowVerbs({
  client,
  rights,
  onRotate,
  onToggle,
  onDelete,
}: Readonly<{
  client: AgentClient;
  rights: Rights;
  onRotate: () => void;
  onToggle: () => void;
  onDelete: () => void;
}>) {
  const t = useT();
  if (!rights.update && !rights.delete) {
    return null;
  }
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: client.client_name })}>
        {rights.update && rotates(client) && (
          <Button aria-haspopup="dialog" onClick={onRotate}>
            {t("agentClients.rotate.open")}
          </Button>
        )}
        {rights.update && (
          <Button onClick={onToggle}>
            {t(
              client.disabled ? "agentClients.enable" : "agentClients.disable",
            )}
          </Button>
        )}
        {rights.delete && (
          <Button variant="danger" aria-haspopup="dialog" onClick={onDelete}>
            {t("agentClients.delete.open")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}

function useColumns(
  rights: Rights,
  verbs: Readonly<{
    rotate: (client: AgentClient) => void;
    toggle: (client: AgentClient) => void;
    remove: (client: AgentClient) => void;
  }>,
): DataTableColumn<AgentClient>[] {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return [
    {
      key: "client",
      header: t("agentClients.col.client"),
      grow: true,
      fold: "title",
      render: (client) => (
        <span
          className="agentclient-name"
          data-agent-client={client.id}
          tabIndex={-1}
        >
          <KeyedName name={client.client_name} code={client.client_id} />
        </span>
      ),
    },
    {
      key: "registered",
      header: t("agentClients.col.registered"),
      render: (client) => (
        <Badge>{t(REGISTRATION[client.registered_by].label)}</Badge>
      ),
    },
    {
      key: "users",
      header: t("agentClients.col.users"),
      align: "end",
      render: (client) => formatNumber(client.connected_users, locale),
      folded: (client) =>
        plural("agentClients.usersFolded", client.connected_users, {
          count: formatNumber(client.connected_users, locale),
        }),
    },
    {
      key: "lastUsed",
      header: t("agentClients.col.lastUsed"),
      render: (client) =>
        client.last_used_at
          ? formatDate(client.last_used_at, locale, viewerZone())
          : t("agentClients.neverUsed"),
    },
    {
      key: "state",
      header: t("agentClients.col.state"),
      fold: "end",
      render: (client) =>
        client.disabled ? (
          <Badge tone="warning">{t("agentClients.disabled")}</Badge>
        ) : null,
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (client) => (
        <RowVerbs
          client={client}
          rights={rights}
          onRotate={() => verbs.rotate(client)}
          onToggle={() => verbs.toggle(client)}
          onDelete={() => verbs.remove(client)}
        />
      ),
    },
  ];
}

/**
 * Every OAuth client an AI tool signs in through: the ones an admin registered,
 * the ones that registered themselves, and published identities.
 */
export function AgentClientsCard() {
  const t = useT();
  const canRead = useCan("agent_client", "read");
  const canCreate = useCanWrite("agent_client", "create");
  const rights: Rights = {
    update: useCanWrite("agent_client", "update"),
    delete: useCanWrite("agent_client", "delete"),
  };
  const list = useAgentClients(canRead);
  const toggle = useSetAgentClientDisabled();
  const [registering, setRegistering] = useState(false);
  const [rotating, setRotating] = useState<AgentClient | null>(null);
  const [deleting, setDeleting] = useState<AgentClient | null>(null);
  const columns = useColumns(rights, {
    rotate: setRotating,
    toggle: (client) =>
      toggle.mutate({ id: client.id, disabled: !client.disabled }),
    remove: setDeleting,
  });
  if (!canRead) {
    return null;
  }
  const clients = list.data?.pages.flatMap((page) => page.data) ?? [];
  const readOnly =
    !canCreate && !rights.update && !rights.delete
      ? t("agentClients.readOnly")
      : false;
  return (
    <Panel
      title={t("agentClients.title")}
      titleAction={
        canCreate ? (
          <Button aria-haspopup="dialog" onClick={() => setRegistering(true)}>
            {t("agentClients.register.open")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("agentClients.sub")}</PanelIntro>
        {!(list.isSuccess && clients.length > 0) && (
          <QueryStates query={list} pendingLabel={t("agentClients.title")}>
            <EmptyState>{t("agentClients.empty")}</EmptyState>
          </QueryStates>
        )}
      </PanelBody>
      {clients.length > 0 && (
        <DataTable
          bleed
          fold
          label={t("agentClients.title")}
          columns={columns}
          rows={clients}
          rowKey={(client) => client.id}
          rowTestId={(client) => `agent-client-${client.id}`}
        />
      )}
      {list.hasNextPage && (
        <PanelBody>
          <LoadMoreButton query={list} />
        </PanelBody>
      )}
      <PanelNotices
        readOnly={readOnly}
        refused={
          toggle.error
            ? { title: t("agentClients.toggleFailed"), error: toggle.error }
            : undefined
        }
      />
      {registering && (
        <RegisterAgentClientDialog onClose={() => setRegistering(false)} />
      )}
      <RotateSecret client={rotating} onClose={() => setRotating(null)} />
      <DeleteClient client={deleting} onClose={() => setDeleting(null)} />
    </Panel>
  );
}

// Focus lands on a row still listed, since the one acted on may be gone.
function anyClientRow(): HTMLElement | null {
  return document.querySelector<HTMLElement>("[data-agent-client]");
}

function RotateSecret({
  client,
  onClose,
}: Readonly<{ client: AgentClient | null; onClose: () => void }>) {
  const t = useT();
  const rotate = useRotateAgentClientSecret();
  const [rotated, setRotated] = useState<AgentClientSecret | null>(null);
  const shown = useLastSubject(client);
  const close = () => {
    setRotated(null);
    rotate.reset();
    onClose();
  };
  if (client !== null && rotated !== null) {
    return (
      <AgentClientValuesDialog
        titleKey="agentClients.rotate.resultTitle"
        clientId={client.client_id}
        secret={rotated.client_secret}
        scopes={rotated.scopes}
        onClose={close}
      />
    );
  }
  const name = shown?.client_name ?? "";
  return (
    <ConfirmModal
      open={client !== null}
      onClose={close}
      title={t("agentClients.rotate.title", { client: name })}
      confirmLabel={t("agentClients.rotate.confirm")}
      confirmVariant="danger"
      pending={rotate.isPending}
      error={rotate.error ? problemMessageOf(rotate.error, t) : null}
      onConfirm={() => {
        if (client !== null) {
          rotate.mutate(client.id, { onSuccess: setRotated });
        }
      }}
    >
      <p>{t("agentClients.rotate.body")}</p>
    </ConfirmModal>
  );
}

function DeleteClient({
  client,
  onClose,
}: Readonly<{ client: AgentClient | null; onClose: () => void }>) {
  const t = useT();
  const remove = useDeleteAgentClient();
  const shown = useLastSubject(client);
  const close = () => {
    remove.reset();
    onClose();
  };
  const name = shown?.client_name ?? "";
  return (
    <ConfirmModal
      open={client !== null}
      onClose={close}
      title={t("agentClients.delete.title", { client: name })}
      confirmLabel={t("agentClients.delete.confirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.error ? problemMessageOf(remove.error, t) : null}
      returnFocusTo={anyClientRow}
      onConfirm={() => {
        if (client !== null) {
          remove.mutate(client.id, { onSuccess: close });
        }
      }}
    >
      <p>
        {t(
          shown === null
            ? "agentClients.delete.bodyAdmin"
            : REGISTRATION[shown.registered_by].deleteBody,
          { client: name },
        )}
      </p>
    </ConfirmModal>
  );
}

/** Settings › AI › MCP: everything about AI tools that connect over MCP. */
export function McpCards() {
  return (
    <>
      <AgentClientMethodsCard />
      <AgentClientExperimentsCard />
      <AgentClientsCard />
      <AgentConnectionsCard />
    </>
  );
}
