// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf } from "../../lib/problem";
import {
  type AgentClientMethod,
  useAgentClients,
  useSaveClientMethods,
} from "./agent-clients.queries";

const MCP_AUTHORIZATION_SPEC =
  "https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization";

// Each switch's row in the read, the settings field that writes it, and the
// section of the MCP specification that defines the method.
const METHODS = [
  {
    method: "metadata_document",
    field: "metadata_document_clients",
    label: "agentClients.methods.published",
    description: "agentClients.methods.publishedHint",
    spec: "agentClients.methods.publishedSpec",
    specHref: `${MCP_AUTHORIZATION_SPEC}#client-id-metadata-documents`,
  },
  {
    method: "dynamic",
    field: "dynamic_client_registration",
    label: "agentClients.methods.dynamic",
    description: "agentClients.methods.dynamicHint",
    spec: "agentClients.methods.dynamicSpec",
    specHref: `${MCP_AUTHORIZATION_SPEC}#dynamic-client-registration`,
  },
] as const satisfies readonly {
  method: string;
  field: string;
  label: MessageKey;
  description: MessageKey;
  spec: MessageKey;
  specHref: string;
}[];

function MethodDescription({
  method,
  row,
}: Readonly<{ method: AgentClientMethod; row: (typeof METHODS)[number] }>) {
  const t = useT();
  return (
    <>
      {t(row.description)}
      {!method.deployment_allows &&
        ` ${t("agentClients.methods.deploymentOff")}`}{" "}
      <OffsiteLink href={row.specHref}>{t(row.spec)}</OffsiteLink>
    </>
  );
}

/**
 * Which ways an AI tool may get a client without an admin. The deployment file
 * can hold a way off; the admin's switch then shows off and cannot turn it on.
 */
export function AgentClientMethodsCard() {
  const t = useT();
  const canRead = useCan("agent_client", "read");
  const canManage = useCanWrite("agent_client", "update");
  const clients = useAgentClients(canRead);
  const save = useSaveClientMethods();
  if (!canRead) {
    return null;
  }
  const registration = {
    ...clients,
    data: clients.data?.pages[0]?.registration,
  };
  return (
    <Panel title={t("agentClients.methods.title")}>
      <PanelBody>
        <PanelIntro>{t("agentClients.methods.sub")}</PanelIntro>
        {save.error && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("agentClients.methods.saveFailed")}
          >
            {problemMessageOf(save.error, t)}
          </Callout>
        )}
      </PanelBody>
      <QueryGate
        query={registration}
        pendingLabel={t("agentClients.methods.title")}
      >
        {(current) => (
          <SettingList bleed="settings">
            {METHODS.map((row) => {
              const method = current[row.method];
              return (
                <SettingRow
                  key={row.method}
                  label={t(row.label)}
                  description={<MethodDescription method={method} row={row} />}
                  control={(control) => (
                    <Switch
                      label={t(row.label)}
                      labelHidden
                      checked={method.deployment_allows && method.setting}
                      describedBy={control["aria-describedby"]}
                      pending={save.isPending}
                      disabled={!canManage || !method.deployment_allows}
                      onChange={(next) =>
                        save.mutate(
                          row.field === "dynamic_client_registration"
                            ? { dynamic_client_registration: next }
                            : { metadata_document_clients: next },
                        )
                      }
                    />
                  )}
                />
              );
            })}
            <SettingRow
              label={t("agentClients.methods.admin")}
              description={t("agentClients.methods.adminHint")}
              control={<Badge>{t("agentClients.methods.alwaysOn")}</Badge>}
            />
          </SettingList>
        )}
      </QueryGate>
      <PanelNotices
        readOnly={canManage ? false : t("agentClients.methods.readOnly")}
      />
    </Panel>
  );
}
