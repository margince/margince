// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { Switch } from "../design-system/switch";
import { formatDateTime, formatNumber } from "../format/format";
import { useNow } from "../format/now";
import { formatRelativeTime } from "../format/relativetime";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, usePlural, useT } from "../i18n";
import { AutomationInspectors, OUTCOME_LOOK } from "./automationdetail";
import { DeleteAutomationAction } from "./automations.delete";
import { AutomationDialog } from "./automations.form";
import { RulePausedReason } from "./automations.lists";
import { recipeSentence } from "./automations.recipe";
import { problemMessageOf, unwrap } from "./common";
import "./automations.css";

// Configured automations as one table, rendered from the Automation wire schema
// alone: no origin field exists on the wire, so authorship cannot change a row.

type Automation = components["schemas"]["Automation"];
type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Inspecting = Readonly<{ runs: boolean; preview: boolean }>;

const CLOSED: Inspecting = { runs: false, preview: false };
const RUNS_ONLY: Inspecting = { runs: true, preview: false };

// The row's detail holds whichever inspectors are open; its chevron opens runs.
function anyOpen(inspecting: Inspecting | undefined): boolean {
  return inspecting !== undefined && (inspecting.runs || inspecting.preview);
}
const RELATIVE_TICK_MS = 60_000;

/** Puts a dialog away while keeping what it showed, so it can animate out. */
export function shut<T extends { open: boolean }>(prior: T | null): T | null {
  return prior === null ? null : { ...prior, open: false };
}

// The row identity travels as a variable: a closed-over `version` can be older
// than the control pressed, and its If-Match then refuses a write nobody raced.
type AutomationPatch = {
  id: string;
  version: number | undefined;
  body: {
    name?: string;
    params?: Record<string, unknown>;
    status?: "enabled" | "paused";
  };
};

async function patchAutomation({ id, version, body }: AutomationPatch) {
  return unwrap(
    await api.PATCH("/automations/{id}", {
      params: { path: { id }, ...ifMatch(requireVersion(version)) },
      body,
    }),
  );
}

type Editing = { automation: Automation; seq: number; open: boolean };

export function ConfiguredAutomations({
  automations,
  entryFor,
  canViewRuns,
  canEdit,
  canDelete,
  focusAfterDelete,
}: Readonly<{
  automations: Automation[];
  entryFor: (key: string) => CatalogEntry | undefined;
  canViewRuns: boolean;
  canEdit: boolean;
  canDelete: boolean;
  focusAfterDelete?: () => HTMLElement | null;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const now = useNow(RELATIVE_TICK_MS);
  const [inspecting, setInspecting] = useState<
    Readonly<Record<string, Inspecting>>
  >({});
  const [editing, setEditing] = useState<Editing | null>(null);
  // An edit form whose grant was revoked could only end in a 403.
  useEffect(() => {
    if (!canEdit) {
      setEditing(shut);
    }
  }, [canEdit]);
  // Settled, not only succeeded: a refusal such as version_skew means the list
  // is stale. Only a fresh read gives the next write the right version.
  const edit = useMutation({
    mutationFn: patchAutomation,
    onSuccess: () => setEditing(shut),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["automations"] }),
  });
  const toggle = (id: string, panel: keyof Inspecting) =>
    setInspecting((prior) => {
      const was = prior[id] ?? CLOSED;
      return { ...prior, [id]: { ...was, [panel]: !was[panel] } };
    });
  const columns = instanceColumns({
    t,
    now,
    entryFor,
    verbs: (automation) => (
      <AutomationVerbs
        automation={automation}
        canEdit={canEdit && entryFor(automation.key) !== undefined}
        canViewRuns={canViewRuns}
        canDelete={canDelete}
        inspecting={inspecting[automation.id] ?? CLOSED}
        focusAfterDelete={focusAfterDelete}
        onToggle={(panel) => toggle(automation.id, panel)}
        onEdit={() => {
          edit.reset();
          setEditing((prior) => ({
            automation,
            seq: (prior?.seq ?? 0) + 1,
            open: true,
          }));
        }}
      />
    ),
    status: (automation) => (
      <AutomationStatus automation={automation} canEdit={canEdit} />
    ),
  });
  const editingEntry = editing && entryFor(editing.automation.key);
  return (
    <>
      <DataTable
        bleed
        fold
        label={t("auto.instances")}
        columns={columns}
        rows={automations}
        rowKey={(automation) => automation.id}
        rowTestId={(automation) => `automation-${automation.id}`}
        detail={
          canViewRuns
            ? {
                header: t("auto.runs.title"),
                toggleLabel: (automation) =>
                  t("auto.runsFor", { name: automation.name }),
                expanded: new Set(
                  automations
                    .filter((one) => anyOpen(inspecting[one.id]))
                    .map((one) => one.id),
                ),
                onToggle: (id) =>
                  setInspecting((prior) => ({
                    ...prior,
                    [id]: anyOpen(prior[id]) ? CLOSED : RUNS_ONLY,
                  })),
                render: (automation) => {
                  const open = inspecting[automation.id] ?? CLOSED;
                  return (
                    <AutomationInspectors
                      automationId={automation.id}
                      runsOpen={open.runs}
                      previewOpen={open.preview}
                      canConfigure={canViewRuns}
                    />
                  );
                },
              }
            : undefined
        }
      />
      {editing && editingEntry && (
        <AutomationDialog
          key={editing.seq}
          entry={editingEntry}
          open={editing.open}
          initialName={editing.automation.name}
          initialParams={editing.automation.params}
          submitLabel={t("trust.save")}
          pending={edit.isPending}
          refusal={<ErrorLine error={edit.error} />}
          onSubmit={(name, params) => {
            // The version as the list now holds it, not as it stood at open.
            const current =
              automations.find((one) => one.id === editing.automation.id) ??
              editing.automation;
            edit.mutate({
              id: current.id,
              version: current.version,
              body: { name, params },
            });
          }}
          onClose={() => setEditing(shut)}
        />
      )}
    </>
  );
}

function instanceColumns({
  t,
  now,
  entryFor,
  verbs,
  status,
}: Readonly<{
  t: Translator;
  now: number;
  entryFor: (key: string) => CatalogEntry | undefined;
  verbs: (automation: Automation) => ReactNode;
  status: (automation: Automation) => ReactNode;
}>): DataTableColumn<Automation>[] {
  return [
    {
      key: "name",
      header: t("auto.name"),
      fold: "title",
      render: (automation) => {
        const entry = entryFor(automation.key);
        return (
          <CellStack>
            <span>{automation.name}</span>
            {entry && (
              <span className="t-caption">{recipeSentence(entry, t)}</span>
            )}
          </CellStack>
        );
      },
    },
    {
      key: "mode",
      header: t("auto.colMode"),
      render: (automation) => <Mode tier={entryFor(automation.key)?.tier} />,
    },
    {
      key: "lastRun",
      header: t("auto.colLastRun"),
      render: (automation) => <LastRun automation={automation} now={now} />,
    },
    {
      key: "runs",
      header: t("auto.colRuns30"),
      align: "end",
      // "Never" in the last-run cell already says it: no count beside it.
      render: (automation) =>
        automation.last_run_at ? (
          <RunCount count={automation.runs_last_30_days ?? 0} />
        ) : null,
    },
    {
      key: "status",
      header: t("auto.colStatus"),
      fold: "end",
      render: status,
    },
    {
      key: "verbs",
      header: t("auto.colActions"),
      headerHidden: true,
      fold: "end",
      render: verbs,
    },
  ];
}

// Running by itself is the normal mode, so only a screen reader hears it.
function Mode({ tier }: Readonly<{ tier?: CatalogEntry["tier"] }>) {
  const t = useT();
  if (!tier) {
    return null;
  }
  return tier === "auto_execute" ? (
    <span className="sr-only">{t("auto.modeAuto")}</span>
  ) : (
    <Badge tone="warning">{t("auto.tier.approval")}</Badge>
  );
}

// Fired and queued are the ordinary ends of a run; only a failure or a block
// earns a badge.
function RunOutcome({
  outcome,
}: Readonly<{ outcome: Automation["last_run_outcome"] }>) {
  const t = useT();
  if (!outcome) {
    return null;
  }
  const look = OUTCOME_LOOK[outcome];
  return outcome === "failed" || outcome === "blocked" ? (
    <Badge tone={look.tone}>{t(look.word)}</Badge>
  ) : (
    <span className="t-caption">{t(look.word)}</span>
  );
}

function LastRun({
  automation,
  now,
}: Readonly<{ automation: Automation; now: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const at = automation.last_run_at;
  if (!at) {
    return <span className="t-caption">{t("auto.lastRunNever")}</span>;
  }
  return (
    <CellStack>
      <time dateTime={at} title={formatDateTime(at, locale, viewerZone())}>
        {formatRelativeTime(at, locale, new Date(now))}
      </time>
      <RunOutcome outcome={automation.last_run_outcome} />
    </CellStack>
  );
}

// Folded onto a caption line the heading is gone, so the figure takes its unit.
function RunCount({ count }: Readonly<{ count: number }>) {
  const { locale } = useLocale();
  const plural = usePlural();
  const figure = formatNumber(count, locale);
  return (
    <>
      <span className="auto-runs-figure">{figure}</span>
      <span className="auto-runs-sentence">
        {plural("auto.runsInWindow", count, { count: figure })}
      </span>
    </>
  );
}

// Flipping the switch is the write; a seat without update reads a badge.
function AutomationStatus({
  automation,
  canEdit,
}: Readonly<{ automation: Automation; canEdit: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const flip = useMutation({
    mutationFn: patchAutomation,
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["automations"] }),
  });
  const enabled = automation.status === "enabled";
  return (
    <CellStack>
      {canEdit ? (
        <Switch
          label={t("auto.enabledFor", { name: automation.name })}
          labelHidden
          checked={enabled}
          pending={flip.isPending}
          onChange={(next) =>
            flip.mutate({
              id: automation.id,
              version: automation.version,
              body: { status: next ? "enabled" : "paused" },
            })
          }
        />
      ) : (
        <Badge tone={enabled ? "success" : "warning"}>
          {enabled ? t("auto.statusEnabled") : t("auto.statusPaused")}
        </Badge>
      )}
      <RulePausedReason automation={automation} />
      {/* A refused flip moves nothing on screen, so this line is its only report. */}
      {flip.isError && <ErrorLine>{problemMessageOf(flip.error, t)}</ErrorLine>}
    </CellStack>
  );
}

// Edit, the two inspectors and Delete, each on its own grant. The inspectors
// are reads (automation:read), so they never hide behind the write grant.
function AutomationVerbs({
  automation,
  canEdit,
  canViewRuns,
  canDelete,
  inspecting,
  focusAfterDelete,
  onToggle,
  onEdit,
}: Readonly<{
  automation: Automation;
  canEdit: boolean;
  canViewRuns: boolean;
  canDelete: boolean;
  inspecting: Inspecting;
  focusAfterDelete?: () => HTMLElement | null;
  onToggle: (panel: keyof Inspecting) => void;
  onEdit: () => void;
}>) {
  const t = useT();
  if (!canEdit && !canViewRuns && !canDelete) {
    return null;
  }
  return (
    <OverflowMenu label={t("auto.rowActions", { name: automation.name })}>
      {canEdit && <Button onClick={onEdit}>{t("trust.edit")}</Button>}
      {canViewRuns && (
        <>
          <Button
            variant={inspecting.runs ? "primary" : "ghost"}
            aria-pressed={inspecting.runs}
            onClick={() => onToggle("runs")}
          >
            {t("auto.runs.open")}
          </Button>
          <Button
            variant={inspecting.preview ? "primary" : "ghost"}
            aria-pressed={inspecting.preview}
            onClick={() => onToggle("preview")}
          >
            {t("auto.preview.open")}
          </Button>
        </>
      )}
      {canDelete && (
        <DeleteAutomationAction
          automation={automation}
          focusAfterDelete={focusAfterDelete}
        />
      )}
    </OverflowMenu>
  );
}
