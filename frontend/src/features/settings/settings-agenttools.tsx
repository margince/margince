// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Badge,
  EmptyState,
  OptionCount,
  SearchField,
} from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Disclosure, DisclosureGroup } from "../../design-system/disclosure";
import { KeyCode } from "../../design-system/keyedname";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import {
  PassportSelect,
  scopeChipLabel,
} from "../../design-system/passportselect";
import { QueryStates } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { AutonomyDot } from "../../design-system/trust";
import { useT } from "../../i18n";
import { dotTier } from "../../lib/autonomy";
import { usePassports } from "../../lib/passports.queries";
import { unwrap } from "../../lib/problem";
import "./settings-agents.css";
import { holdsEveryWord, searchWords } from "../../format/collate";

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
  // A revoked passport drops out of the options, and the filter with it. The
  // inventory never stays scoped to a choice no longer on offer.
  const scopeId = live.some((passport) => passport.id === passportId)
    ? passportId
    : "";
  const granted = new Set(
    live.find((passport) => passport.id === scopeId)?.scopes ?? [],
  );
  const reachable = (tool: AgentTool) =>
    !scopeId || tool.required_scope == null || granted.has(tool.required_scope);
  const all = tools.data?.data ?? [];
  const words = searchWords(query);
  const shown = matching(all, words);
  // A row found by its key alone shows the key, or nothing on it says why.
  const keyHit = (tool: AgentTool) =>
    words.length > 0 &&
    holdsEveryWord(words, [tool.name]) &&
    !holdsEveryWord(words, [tool.title, tool.description]);

  return (
    <Panel title={t("tools.title")}>
      <PanelBody>
        <PanelIntro>{t("tools.sub")}</PanelIntro>
        {all.length > 0 && (
          <SearchField
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("tools.search")}
            aria-label={t("tools.search")}
          />
        )}
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
      <PanelBody>
        {shown.length > 0 ? (
          <ToolGroups
            key={words.length === 0 ? "browse" : "search"}
            groups={groupByScope(shown)}
            searching={words.length > 0}
            reachable={reachable}
            keyHit={keyHit}
          />
        ) : (
          <QueryStates query={tools} pendingLabel={t("tools.title")}>
            <EmptyState>
              {all.length > 0 ? t("tools.noMatch") : t("common.empty")}
            </EmptyState>
          </QueryStates>
        )}
      </PanelBody>
    </Panel>
  );
}

function matching(
  tools: readonly AgentTool[],
  words: readonly string[],
): readonly AgentTool[] {
  return tools.filter((tool) =>
    holdsEveryWord(words, [tool.title, tool.name, tool.description]),
  );
}

// The order a passport's scopes widen in, so the groups read from the least
// a tool may do to the most.
const SCOPE_ORDER: readonly string[] = [
  "read",
  "draft",
  "write",
  "send",
  "enrich",
];

type ToolGroup = Readonly<{
  scope: string | null;
  tools: readonly AgentTool[];
}>;

function scopeRank(scope: string | null): number {
  if (scope === null) {
    return -1;
  }
  const rank = SCOPE_ORDER.indexOf(scope);
  return rank === -1 ? SCOPE_ORDER.length : rank;
}

function groupByScope(tools: readonly AgentTool[]): readonly ToolGroup[] {
  const byScope = new Map<string | null, AgentTool[]>();
  for (const tool of tools) {
    const scope = tool.required_scope ?? null;
    byScope.set(scope, [...(byScope.get(scope) ?? []), tool]);
  }
  return [...byScope]
    .map(([scope, members]) => ({ scope, tools: members }))
    .sort((a, b) => scopeRank(a.scope) - scopeRank(b.scope));
}

// Browsing opens one scope at a time; a search opens every group it matched in.
function ToolGroups({
  groups,
  searching,
  reachable,
  keyHit,
}: Readonly<{
  groups: readonly ToolGroup[];
  searching: boolean;
  reachable: (tool: AgentTool) => boolean;
  keyHit: (tool: AgentTool) => boolean;
}>) {
  const t = useT();
  const sections = groups.map((group) => {
    const label =
      group.scope === null
        ? t("tools.noScope")
        : scopeChipLabel(t, group.scope);
    return (
      <Disclosure
        key={group.scope ?? ""}
        open={searching ? true : undefined}
        summary={
          <span className="tools-group">
            {label}
            <OptionCount count={group.tools.length} />
          </span>
        }
      >
        <ToolTable
          label={label}
          tools={group.tools}
          reachable={reachable}
          keyHit={keyHit}
        />
      </Disclosure>
    );
  });
  return searching ? sections : <DisclosureGroup>{sections}</DisclosureGroup>;
}

function ToolTable({
  label,
  tools,
  reachable,
  keyHit,
}: Readonly<{
  label: string;
  tools: readonly AgentTool[];
  reachable: (tool: AgentTool) => boolean;
  keyHit: (tool: AgentTool) => boolean;
}>) {
  const t = useT();
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set());
  const toggle = (name: string) =>
    setOpen((current) => {
      const next = new Set(current);
      if (!next.delete(name)) {
        next.add(name);
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
          <span className="tools-name-line">
            <span className={reachable(tool) ? undefined : "agents-ended"}>
              {tool.title}
            </span>
            {tool.egress && <Badge tone="warning">{t("tools.egress")}</Badge>}
          </span>
          {!reachable(tool) && (
            <span className="t-caption">{t("tools.unreachable")}</span>
          )}
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
            {open.has(tool.name) && (
              <span className="tools-key">
                <KeyCode code={tool.name} />
              </span>
            )}
          </span>
          {!open.has(tool.name) && keyHit(tool) && (
            <span className="tools-key">
              <KeyCode code={tool.name} />
            </span>
          )}
        </CellStack>
      ),
    },
    {
      key: "tier",
      header: t("tools.colTier"),
      render: (tool) => <AutonomyDot tier={dotTier(tool.tier)} withLabel />,
    },
  ];
  return (
    <DataTable
      fold
      label={label}
      columns={columns}
      rows={[...tools]}
      rowKey={(tool) => tool.name}
      rowTestId={(tool) => `tool-${tool.name}`}
      detail={{
        header: t("tools.colDetail"),
        toggleLabel: (tool) =>
          t("tools.descriptionToggle", { tool: tool.title }),
        expanded: open,
        onToggle: toggle,
        controls: descriptionId,
      }}
    />
  );
}
