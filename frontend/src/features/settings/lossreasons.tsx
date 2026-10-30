// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { Button, EmptyState, OverflowMenu } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyCode } from "../../design-system/keyedname";
import { NameDialog, nameRefusal } from "../../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import {
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { useCanWrite } from "../../lib/capability";
import {
  choosableLossReasons,
  type LegacyLossReason,
  type LossReason,
  useApplyLegacyLossReasons,
  useCreateLossReason,
  useDecideLegacyLossReason,
  useLegacyLossReasons,
  useLossReasons,
  useUpdateLossReason,
} from "../../lib/lossreasons.queries";
import { problemMessageOf } from "../../lib/problem";
import { DealsWithheld } from "./acquisitionsources";
import { usePageReadOnly } from "./settingsnav";

function reasonColumns({
  t,
  plural,
  locale,
  canEdit,
  refusedFor,
  onActive,
  onRename,
}: Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  canEdit: boolean;
  refusedFor: (reason: LossReason) => string | undefined;
  onActive: (reason: LossReason, active: boolean) => void;
  onRename: (reason: LossReason) => void;
}>): DataTableColumn<LossReason>[] {
  return [
    {
      key: "name",
      header: t("lossReasons.colReason"),
      grow: true,
      render: (reason) => reason.label,
    },
    {
      key: "deals",
      header: t("lossReasons.colDeals"),
      align: "end",
      render: (reason) =>
        typeof reason.deal_count === "number" ? (
          formatNumber(reason.deal_count, locale)
        ) : (
          <DealsWithheld />
        ),
      folded: (reason) =>
        typeof reason.deal_count === "number"
          ? plural("lossReasons.deals", reason.deal_count, {
              count: formatNumber(reason.deal_count, locale),
            })
          : undefined,
    },
    {
      key: "active",
      header: t("leadSources.colActive"),
      fold: "end",
      render: (reason) => (
        <Switch
          describedBy={refusedFor(reason)}
          label={t("lossReasons.activeFor", { label: reason.label })}
          labelHidden
          checked={reason.active}
          disabled={!canEdit}
          onChange={(active) => onActive(reason, active)}
        />
      ),
    },
    ...(canEdit
      ? [
          {
            key: "verbs",
            header: t("leadSources.colActions"),
            headerHidden: true,
            fold: "end" as const,
            render: (reason: LossReason) => (
              <span className="cell-actions">
                <OverflowMenu
                  label={t("table.rowActions", { name: reason.label })}
                >
                  <Button onClick={() => onRename(reason)}>
                    {t("leadSources.rename")}
                  </Button>
                </OverflowMenu>
              </span>
            ),
          },
        ]
      : []),
  ];
}

type Naming = { mode: "create" } | { mode: "rename"; reason: LossReason };

// The reasons a closer picks from when a deal is lost. No delete: a code a lost
// deal carries must stay resolvable, so the switch retires it from the picker.
export function LossReasonsCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  // A page the seat only reads says so itself; the card names only the
  // partial case it leaves unsaid.
  const readOnlyPage = usePageReadOnly("lossreasons");
  const refusal =
    !readOnlyPage && !canEdit && canCreate
      ? ("leadSources.addOnlyTitle" as const)
      : undefined;
  const query = useLossReasons();
  const create = useCreateLossReason();
  const update = useUpdateLossReason();
  const rename = useUpdateLossReason();
  const refusalId = useId();
  const [naming, setNaming] = useState<Naming>({ mode: "create" });
  const [namingOpen, setNamingOpen] = useState(false);
  const reasons = rowsOf(query.data);
  const write = naming.mode === "rename" ? rename : create;
  const refused =
    naming.mode === "rename"
      ? {
          problem: rename.isError ? problemMessageOf(rename.error, t) : null,
          nameProblem: null,
        }
      : nameRefusal(create.error, t, "lossReasons.duplicate");
  const open = (next: Naming) => {
    create.reset();
    rename.reset();
    setNaming(next);
    setNamingOpen(true);
  };
  const columns = reasonColumns({
    t,
    plural,
    locale,
    canEdit,
    refusedFor: (reason) =>
      update.isError && update.variables?.id === reason.id
        ? refusalId
        : undefined,
    onActive: (reason, active) =>
      update.mutate({ id: reason.id, body: { active } }),
    onRename: (reason) => open({ mode: "rename", reason }),
  });
  const done = { onSuccess: () => setNamingOpen(false) };
  return (
    <Panel
      title={t("lossReasons.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => open({ mode: "create" })}>
            {t("lossReasons.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("lossReasons.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && reasons.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("lossReasons.title")}
          columns={columns}
          rows={[...reasons]}
          rowKey={(reason) => reason.id}
          rowTestId={(reason) => `loss-reason-${reason.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("lossReasons.loading")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelNotices
        readOnly={refusal && t(refusal)}
        refused={
          update.isError
            ? {
                title: t("leadSources.notSaved"),
                error: update.error,
                id: refusalId,
              }
            : undefined
        }
      />
      <NameDialog
        open={namingOpen}
        onClose={() => setNamingOpen(false)}
        title={t(
          naming.mode === "rename"
            ? "lossReasons.renameTitle"
            : "lossReasons.addTitle",
        )}
        label={t("acqSources.addLabel")}
        initial={naming.mode === "rename" ? naming.reason.label : ""}
        hint={naming.mode === "rename" && <KeyCode code={naming.reason.key} />}
        confirmLabel={t(
          naming.mode === "rename"
            ? "leadSources.renameSave"
            : "lossReasons.addConfirm",
        )}
        pending={write.isPending}
        problem={refused.problem}
        nameProblem={refused.nameProblem}
        onSave={(label) => {
          if (naming.mode === "rename") {
            rename.mutate({ id: naming.reason.id, body: { label } }, done);
          } else {
            create.mutate({ label }, done);
          }
        }}
      />
    </Panel>
  );
}

// The notes of deals lost before the list existed. An administrator picks the
// reason each note counts under; applying writes those reasons onto the deals
// in the background. The notes themselves are never changed.
export function LegacyLossReasonsCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canDecide = useCanWrite("custom_field", "update");
  const canApply = useCanWrite("data_task", "update");
  const legacy = useLegacyLossReasons(canDecide);
  const reasons = choosableLossReasons(useLossReasons().data);
  const decide = useDecideLegacyLossReason();
  const apply = useApplyLegacyLossReasons();
  const refusalId = useId();
  if (!canDecide) {
    return null;
  }
  const rows = legacy.data?.data ?? [];
  const decided = rows.some((row) => row.mapping);
  const columns: DataTableColumn<LegacyLossReason>[] = [
    {
      key: "note",
      header: t("lossReasons.colNote"),
      grow: true,
      render: (row) => row.text,
    },
    {
      key: "deals",
      header: t("lossReasons.colDeals"),
      align: "end",
      render: (row) => formatNumber(row.deal_count, locale),
    },
    {
      key: "reason",
      header: t("lossReasons.colMapTo"),
      render: (row) => (
        <Select
          aria-label={t("lossReasons.mapFor", { note: row.text })}
          disabled={decide.isPending}
          placeholder={t("lossReasons.pick")}
          value={row.mapping?.reason_key ?? ""}
          onChange={(reasonKey) =>
            decide.mutate({ textHash: row.text_hash, reasonKey })
          }
          options={reasons.map((reason) => ({
            value: reason.key,
            label: reason.label,
          }))}
        />
      ),
    },
  ];
  return (
    <Panel
      title={t("lossReasons.legacyTitle")}
      titleAction={
        canApply && (
          <Button
            disabled={!decided || apply.isPending}
            onClick={() => apply.mutate()}
          >
            {t("lossReasons.apply")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("lossReasons.legacySub")}</PanelIntro>
        {legacy.data && legacy.data.distinct_total > rows.length && (
          <p className="t-caption">
            {plural("lossReasons.remaining", legacy.data.distinct_total, {
              shown: formatNumber(rows.length, locale),
              count: formatNumber(legacy.data.distinct_total, locale),
            })}
          </p>
        )}
        {apply.isSuccess && (
          <p className="t-caption" role="status">
            {plural("lossReasons.applied", apply.data.deals, {
              count: formatNumber(apply.data.deals, locale),
            })}
          </p>
        )}
      </PanelBody>
      {rows.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("lossReasons.legacyTitle")}
          columns={columns}
          rows={rows}
          rowKey={(row) => row.text_hash}
        />
      ) : (
        <PanelBody>
          <QueryStates query={legacy} pendingLabel={t("lossReasons.loading")}>
            <EmptyState>{t("lossReasons.legacyEmpty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelNotices
        refused={
          decide.isError || apply.isError
            ? {
                title: t("leadSources.notSaved"),
                error: decide.error ?? apply.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}
