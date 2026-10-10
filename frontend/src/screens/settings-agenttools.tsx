// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { dotTier } from "../app/autonomy";
import { Badge, EmptyState, SearchField } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { IconAction } from "../design-system/iconaction";
import { KeyedName } from "../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import {
  PassportSelect,
  scopeChipLabel,
} from "../design-system/passportselect";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { AutonomyDot } from "../design-system/trust";
import { useT } from "../i18n";
import { QueryStates, unwrap } from "./common";
import { usePassports } from "./passports.queries";
import "./settings-agents.css";

type AgentTool = components["schemas"]["AgentTool"];

// The governed surface an MCP client sees. A passport picked in the selector
// strikes every tool its scopes do not reach.
export function AgentToolsCard() {
  const t = useT();
  const [passportId, setPassportId] = useState("");
  const [query, setQuery] = useState("");
  const tools = useQuery({
    queryKey: ["agent-tools"],
    queryFn: async () => unwrap(await api.GET("/agent-tools")),
  });
  const passports = (usePassports().data?.data ?? []).filter(
    (passport) => passport.connection == null,
  );
  // A connection's credential was minted from a consent screen, never picked
  // from a list, so only the passports this human minted are offered.
  const live = passports.filter((passport) => passport.revoked_at == null);
  // A revoked passport drops out of the options, and the filter with it, so
  // the inventory never stays scoped to a choice no longer on offer.
  const scopeId = live.some((passport) => passport.id === passportId)
    ? passportId
    : "";
  const granted = new Set(
    live.find((passport) => passport.id === scopeId)?.scopes ?? [],
  );
  const reachable = (tool: AgentTool) =>
    !scopeId || tool.required_scope == null || granted.has(tool.required_scope);
  const all = tools.data?.data ?? [];
  const shown = matching(all, query);

  return (
    <Panel title={t("tools.title")}>
      <PanelBody>
        <PanelIntro>{t("tools.sub")}</PanelIntro>
      </PanelBody>
      {/* Kept once a passport was ever minted, revoked included, so revoking
          the chosen one does not pull the selector away mid-use. */}
      {passports.length > 0 && (
        <SettingList bleed="settings">
          <SettingRow
            label={t("tools.scopeLabel")}
            control={
              <PassportSelect
                options={live.map((passport) => ({
                  id: passport.id,
                  label: t("tools.scopedTo", { label: passport.label }),
                  scopes: passport.scopes,
                }))}
                value={scopeId}
                onChange={setPassportId}
                allowEmpty
                emptyLabel={t("tools.scopeAll")}
                ariaLabel={t("tools.scopeAll")}
              />
            }
          />
        </SettingList>
      )}
      {all.length > 0 && (
        <PanelBody>
          <SearchField
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("tools.search")}
            aria-label={t("tools.search")}
          />
        </PanelBody>
      )}
      {shown.length > 0 ? (
        <ToolTable tools={shown} reachable={reachable} />
      ) : (
        <PanelBody>
          <QueryStates query={tools} pendingLabel={t("tools.title")}>
            <EmptyState>
              {all.length > 0 ? t("tools.noMatch") : t("common.empty")}
            </EmptyState>
          </QueryStates>
        </PanelBody>
      )}
    </Panel>
  );
}

function matching(
  tools: readonly AgentTool[],
  query: string,
): readonly AgentTool[] {
  const needle = query.trim().toLowerCase();
  if (needle === "") {
    return tools;
  }
  return tools.filter((tool) =>
    [tool.title, tool.name, tool.description].some((text) =>
      text.toLowerCase().includes(needle),
    ),
  );
}

function ToolTable({
  tools,
  reachable,
}: Readonly<{
  tools: readonly AgentTool[];
  reachable: (tool: AgentTool) => boolean;
}>) {
  const t = useT();
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set());
  const toggle = (tool: AgentTool) =>
    setOpen((current) => {
      const next = new Set(current);
      if (!next.delete(tool.name)) {
        next.add(tool.name);
      }
      return next;
    });
  const idPrefix = useId();
  const descriptionId = (tool: AgentTool) => `${idPrefix}-${tool.name}`;
  const columns: DataTableColumn<AgentTool>[] = [
    {
      key: "tool",
      header: t("tools.colTool"),
      grow: true,
      fold: "title",
      render: (tool) => (
        <CellStack>
          <span className={reachable(tool) ? undefined : "agents-ended"}>
            <KeyedName name={tool.title} code={tool.name} />
          </span>
          {/* The text an agent selects the tool by, governance clause included. */}
          <span
            id={descriptionId(tool)}
            className={
              open.has(tool.name)
                ? "t-caption tools-description"
                : "t-caption tools-description tools-description-clamped"
            }
          >
            {tool.description}
          </span>
        </CellStack>
      ),
    },
    {
      key: "tier",
      header: t("tools.colTier"),
      render: (tool) => <AutonomyDot tier={dotTier(tool.tier)} withLabel />,
    },
    {
      key: "access",
      header: t("tools.colScope"),
      render: (tool) => (
        <CellStack>
          <span className="agents-scopes">
            {tool.required_scope && (
              <Badge>{scopeChipLabel(t, tool.required_scope)}</Badge>
            )}
            {tool.egress && <Badge tone="warning">{t("tools.egress")}</Badge>}
          </span>
          {!reachable(tool) && (
            <span className="t-caption">{t("tools.unreachable")}</span>
          )}
        </CellStack>
      ),
    },
    {
      key: "detail",
      header: t("tools.colDetail"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (tool) => (
        <IconAction
          onClick={() => toggle(tool)}
          label={t("tools.descriptionToggle", { tool: tool.title })}
          icon={<ChevronDown aria-hidden className="expander-chevron" />}
          disclosure={{
            expanded: open.has(tool.name),
            controls: descriptionId(tool),
          }}
        />
      ),
    },
  ];
  return (
    <DataTable
      bleed
      fold
      label={t("tools.title")}
      columns={columns}
      rows={[...tools]}
      rowKey={(tool) => tool.name}
      rowTestId={(tool) => `tool-${tool.name}`}
      onRowClick={toggle}
    />
  );
}
