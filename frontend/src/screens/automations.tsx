import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, ChevronRight, Zap } from "lucide-react";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { EmptyState } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { IconAction } from "../design-system/iconaction";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { Chip } from "../design-system/readings";
import { useTruncationTooltip } from "../design-system/tooltip";
import { type Translator, useT } from "../i18n";
import { AutomationDialog } from "./automations.form";
import { ConfiguredAutomations, shut } from "./automations.instances";
import { triggerLabel } from "./automations.recipe";
import { type QueryLike, QueryStates, unwrap, useMe } from "./common";
import "./automations.css";

// The automations editor (B-EP09.15): a management UI over the CLOSED
// catalog (E15/ADR-0035). The anti-DSL invariant of features/10 §1 holds by
// construction — every form field derives from the catalog entry's
// params_schema plus the instance name; there is no free-form rule body and
// no user-defined trigger anywhere on this surface, and a test pins that.

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];
type Automation = components["schemas"]["Automation"];
// A template staged for the create dialog. It OUTLIVES the close — hence `open`
// as a field, and `seq`, which re-keys the form so every open re-seeds it.
type StagedTemplate = { entry: CatalogEntry; seq: number; open: boolean };

// One section of Settings → AI: the page owns the reading column and the h1.
export function AutomationsAdmin() {
  const t = useT();
  const queryClient = useQueryClient();
  const [staged, setStaged] = useState<StagedTemplate | null>(null);
  // Grants come from the session (/v1/me); until they arrive every predicate
  // is false, so the section shows no mutation affordance until one is confirmed.
  const me = useMe();
  const canViewRuns = useCan("automation", "read");
  const canCreate = useCanWrite("automation", "create");
  const canEdit = useCanWrite("automation", "update");
  const canDelete = useCanWrite("automation", "delete");
  // The create form must not survive the grant that opened it, or Create
  // becomes a button that can only 403.
  useEffect(() => {
    if (!canCreate) {
      setStaged(shut);
    }
  }, [canCreate]);

  // Not gated: ListAutomationCatalog asks for no object grant.
  const catalog = useQuery({
    queryKey: ["automation-catalog"],
    queryFn: async () => {
      return unwrap(await api.GET("/automations/catalog"));
    },
  });

  // Gated on the grant the server demands (AutomationStore.List requires
  // automation:read), so a seat without it is not shown a Retry that cannot work.
  const instances = useQuery({
    queryKey: ["automations"],
    enabled: canViewRuns,
    queryFn: async () => {
      return unwrap(
        await api.GET("/automations", {
          params: { query: { limit: 50 } },
        }),
      );
    },
  });

  const create = useMutation({
    mutationFn: async (input: {
      key: string;
      name: string;
      params: Record<string, unknown>;
    }) => {
      return unwrap(
        await api.POST("/automations", {
          body: input,
        }),
      );
    },
    onSuccess: () => {
      setStaged(shut);
      queryClient.invalidateQueries({ queryKey: ["automations"] });
    },
  });

  // Reset on OPEN, not on close: the closing dialog still shows its refusal.
  const stage = (entry: CatalogEntry) => {
    create.reset();
    setStaged((prior) => ({ entry, seq: (prior?.seq ?? 0) + 1, open: true }));
  };

  const entryFor = (key: string): CatalogEntry | undefined =>
    catalog.data?.data.find((entry) => entry.key === key);

  return (
    <div data-automations-admin>
      <Panel title={t("nav.automations")}>
        <PanelBody>
          <PanelIntro>{t("auto.sub")}</PanelIntro>
          {/* Bound to `update`, the grant the row's Switch asks for. */}
          {me.isSuccess && !canEdit && (
            <PanelIntro>{t("auto.readOnly")}</PanelIntro>
          )}
        </PanelBody>
        <PanelGroupHead title={t("auto.instances")} level="h3" />
        <InstancesSection
          instances={instances}
          me={me}
          entryFor={entryFor}
          canViewRuns={canViewRuns}
          canEdit={canEdit}
          canDelete={canDelete}
        />
        <PanelGroupHead title={t("auto.catalog")} level="h3" />
        <StarterLibrary catalog={catalog} canCreate={canCreate} onUse={stage} />
        {/* On the card: by the time it is true the dialog is gone. */}
        {create.isSuccess && (
          <PanelBody>
            <p role="status">{t("auto.createdPaused")}</p>
          </PanelBody>
        )}
        {staged && (
          <AutomationDialog
            key={staged.seq}
            open={staged.open}
            entry={staged.entry}
            initialName={staged.entry.name}
            submitLabel={t("auto.create")}
            pending={create.isPending}
            refusal={<ErrorLine error={create.error} />}
            onSubmit={(name, params) =>
              create.mutate({ key: staged.entry.key, name, params })
            }
            onClose={() => setStaged(shut)}
          />
        )}
      </Panel>
    </div>
  );
}

// Without the read grant the section says it is WITHHELD: an empty list would
// claim the workspace runs no automations. Behind /me, so it is a settled denial.
function InstancesSection({
  instances,
  me,
  entryFor,
  canViewRuns,
  canEdit,
  canDelete,
}: Readonly<{
  instances: QueryLike<{ data: Automation[] }>;
  me: QueryLike<unknown>;
  entryFor: (key: string) => CatalogEntry | undefined;
  canViewRuns: boolean;
  canEdit: boolean;
  canDelete: boolean;
}>) {
  const t = useT();
  const rows = instances.data?.data;
  if (canViewRuns && rows !== undefined && rows.length > 0) {
    return (
      <ConfiguredAutomations
        automations={rows}
        entryFor={entryFor}
        canViewRuns={canViewRuns}
        canEdit={canEdit}
        canDelete={canDelete}
      />
    );
  }
  return (
    <PanelBody>
      <QueryStates
        query={canViewRuns ? instances : me}
        pendingLabel={t("auto.instances")}
      >
        <EmptyState>
          {canViewRuns ? t("common.empty") : t("auto.withheld")}
        </EmptyState>
      </QueryStates>
    </PanelBody>
  );
}

// Every seat may read the catalog; only the create grant makes a row open.
function StarterLibrary({
  catalog,
  canCreate,
  onUse,
}: Readonly<{
  catalog: QueryLike<{ data: CatalogEntry[] }>;
  canCreate: boolean;
  onUse: (entry: CatalogEntry) => void;
}>) {
  const t = useT();
  const entries = catalog.data?.data;
  if (entries === undefined || entries.length === 0) {
    return (
      <PanelBody>
        <QueryStates query={catalog} pendingLabel={t("auto.catalog")}>
          <EmptyState>{t("common.empty")}</EmptyState>
        </QueryStates>
      </PanelBody>
    );
  }
  return (
    <DataTable
      bleed
      fold
      label={t("auto.catalog")}
      columns={libraryColumns(t, canCreate ? onUse : undefined)}
      rows={entries}
      rowKey={(entry) => entry.key}
      rowTestId={(entry) => `template-${entry.key}`}
      onRowClick={canCreate ? onUse : undefined}
    />
  );
}

function libraryColumns(
  t: Translator,
  onUse: ((entry: CatalogEntry) => void) | undefined,
): DataTableColumn<CatalogEntry>[] {
  const columns: DataTableColumn<CatalogEntry>[] = [
    {
      key: "name",
      header: t("auto.name"),
      fold: "title",
      grow: true,
      render: (entry) => (
        <CellStack>
          <span>{entry.name}</span>
          {entry.description && <OneLine text={entry.description} />}
        </CellStack>
      ),
    },
    {
      key: "trigger",
      header: t("auto.runs.why"),
      render: (entry) => (
        <Chip
          dense
          icon={entry.trigger.startsWith("clock:") ? CalendarClock : Zap}
        >
          {triggerLabel(entry.trigger, t)}
        </Chip>
      ),
    },
  ];
  if (onUse === undefined) {
    return columns;
  }
  return [
    ...columns,
    {
      key: "use",
      header: t("auto.use"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (entry) => (
        <IconAction
          variant="ghost"
          icon={<ChevronRight aria-hidden />}
          label={t("auto.use")}
          onClick={() => onUse(entry)}
        />
      ),
    },
  ];
}

// One line, cut short; the whole text on hover and focus.
function OneLine({ text }: Readonly<{ text: string }>) {
  const tip = useTruncationTooltip<HTMLSpanElement>(text);
  return (
    <span className="t-caption auto-oneline" ref={tip.ref} {...tip.trigger}>
      {text}
      {tip.tip}
    </span>
  );
}
