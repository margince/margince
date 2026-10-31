// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { rowsOf } from "../../api/rows";
import { Button, EmptyState, OverflowMenu } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { KeyCode } from "../../design-system/keyedname";
import { NameDialog, nameRefusal } from "../../design-system/namedialog";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { useCanWrite } from "../../lib/capability";
import { isOption } from "../../lib/options";
import {
  MEANING_LABELS,
  PARTNER_STAGE_MEANINGS,
  type PartnerStage,
  type PartnerStageMeaning,
  useCreatePartnerStage,
  useDeletePartnerStage,
  usePartnerStages,
  useSwapPartnerStages,
  useUpdatePartnerStage,
} from "../../lib/partnerstages.queries";
import { problemMessageOf } from "../../lib/problem";
import { usePageReadOnly } from "./settingsnav";

type RowVerbs = Readonly<{
  onRename: (stage: PartnerStage) => void;
  onMove: (stage: PartnerStage, by: -1 | 1) => void;
  onRemove: (stage: PartnerStage) => void;
}>;

function stageColumns({
  t,
  locale,
  canEdit,
  canRemove,
  stages,
  refusedFor,
  onPatch,
  verbs,
}: Readonly<{
  t: Translator;
  locale: Locale;
  canEdit: boolean;
  canRemove: boolean;
  stages: readonly PartnerStage[];
  refusedFor: (stage: PartnerStage) => string | undefined;
  onPatch: (
    stage: PartnerStage,
    body: { active?: boolean; meaning?: PartnerStageMeaning },
  ) => void;
  verbs: RowVerbs;
}>): DataTableColumn<PartnerStage>[] {
  const meaningOptions = PARTNER_STAGE_MEANINGS.map((meaning) => ({
    value: meaning,
    label: t(MEANING_LABELS[meaning]),
  }));
  return [
    {
      key: "name",
      header: t("partnerStages.colStage"),
      grow: true,
      render: (stage) => stage.label,
    },
    {
      key: "meaning",
      header: t("partnerStages.colMeaning"),
      // A partner standing on the stage would change meaning with it, so the
      // server keeps the meaning of a stage in use.
      render: (stage) => (
        <Select
          aria-label={t("partnerStages.meaningFor", { label: stage.label })}
          aria-describedby={refusedFor(stage)}
          value={stage.meaning}
          disabled={!canEdit || (stage.partner_count ?? 0) > 0}
          onChange={(value) => {
            if (
              isOption(value, PARTNER_STAGE_MEANINGS) &&
              value !== stage.meaning
            ) {
              onPatch(stage, { meaning: value });
            }
          }}
          options={meaningOptions}
        />
      ),
    },
    {
      key: "partners",
      header: t("partnerStages.colPartners"),
      align: "end",
      // Absent for a seat that may not read partners; the server then answers
      // an edit to a stage in use with a refusal instead.
      render: (stage) =>
        stage.partner_count === undefined
          ? ""
          : formatNumber(stage.partner_count, locale),
    },
    {
      key: "active",
      header: t("leadSources.colActive"),
      fold: "end",
      render: (stage) => (
        <Switch
          describedBy={refusedFor(stage)}
          label={t("partnerStages.activeFor", { label: stage.label })}
          labelHidden
          checked={stage.active}
          disabled={!canEdit}
          onChange={(active) => onPatch(stage, { active })}
        />
      ),
    },
    ...(canEdit || canRemove
      ? [
          {
            key: "verbs",
            header: t("leadSources.colActions"),
            headerHidden: true,
            fold: "end" as const,
            render: (stage: PartnerStage) => {
              const at = stages.findIndex((row) => row.id === stage.id);
              const removable =
                canRemove &&
                stage.system !== true &&
                (stage.partner_count ?? 0) === 0;
              return (
                <span className="cell-actions">
                  <OverflowMenu
                    label={t("table.rowActions", { name: stage.label })}
                  >
                    {canEdit && (
                      <Button onClick={() => verbs.onRename(stage)}>
                        {t("leadSources.rename")}
                      </Button>
                    )}
                    {canEdit && at > 0 && (
                      <Button onClick={() => verbs.onMove(stage, -1)}>
                        {t("partnerStages.moveUp")}
                      </Button>
                    )}
                    {canEdit && at < stages.length - 1 && (
                      <Button onClick={() => verbs.onMove(stage, 1)}>
                        {t("partnerStages.moveDown")}
                      </Button>
                    )}
                    {removable && (
                      <Button onClick={() => verbs.onRemove(stage)}>
                        {t("leadSources.remove")}
                      </Button>
                    )}
                  </OverflowMenu>
                </span>
              );
            },
          },
        ]
      : []),
  ];
}

type Naming = { mode: "create" } | { mode: "rename"; stage: PartnerStage };

// Mounted afresh for each opening, so a refused write never greets the next one.
function StageNameDialog({
  naming,
  nextOrder,
  onClose,
}: Readonly<{ naming: Naming; nextOrder: number; onClose: () => void }>) {
  const t = useT();
  const create = useCreatePartnerStage();
  const rename = useUpdatePartnerStage();
  const renaming = naming.mode === "rename" ? naming.stage : null;
  const refused = renaming
    ? {
        problem: rename.isError ? problemMessageOf(rename.error, t) : null,
        nameProblem: null,
      }
    : nameRefusal(create.error, t, "partnerStages.duplicate");
  const done = { onSuccess: onClose };
  return (
    <NameDialog
      open
      onClose={onClose}
      title={t(
        renaming ? "partnerStages.renameTitle" : "partnerStages.addTitle",
      )}
      label={t("acqSources.addLabel")}
      initial={renaming?.label ?? ""}
      hint={renaming && <KeyCode code={renaming.key} />}
      confirmLabel={t(
        renaming ? "leadSources.renameSave" : "partnerStages.addConfirm",
      )}
      pending={(renaming ? rename : create).isPending}
      problem={refused.problem}
      nameProblem={refused.nameProblem}
      onSave={(label) => {
        if (renaming) {
          rename.mutate({ id: renaming.id, body: { label } }, done);
        } else {
          // A new stage joins the end of the ladder as a candidate; its
          // meaning is changed in its row until a partner stands on it.
          create.mutate(
            { label, meaning: "candidate", sort_order: nextOrder },
            done,
          );
        }
      }}
    />
  );
}

function RemoveStageDialog({
  stage,
  onClose,
}: Readonly<{ stage: PartnerStage | null; onClose: () => void }>) {
  const t = useT();
  const remove = useDeletePartnerStage();
  return (
    <ConfirmModal
      open={stage !== null}
      onClose={() => {
        remove.reset();
        onClose();
      }}
      title={t("partnerStages.removeTitle")}
      confirmLabel={t("leadSources.remove")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.isError ? problemMessageOf(remove.error, t) : null}
      onConfirm={() => {
        if (stage) {
          remove.mutate(stage.id, { onSuccess: onClose });
        }
      }}
    >
      <p>{t("partnerStages.removeBody", { label: stage?.label ?? "" })}</p>
    </ConfirmModal>
  );
}

// The installation's partner ladder. A built-in stage is never removed, and a
// stage a partner stands on is retired through its switch instead.
export function PartnerStagesCard() {
  const t = useT();
  const { locale } = useLocale();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const canRemove = useCanWrite("custom_field", "delete");
  const readOnlyPage = usePageReadOnly("partners");
  const refusal =
    !readOnlyPage && !canEdit && canCreate
      ? ("leadSources.addOnlyTitle" as const)
      : undefined;
  const query = usePartnerStages();
  const update = useUpdatePartnerStage();
  const swap = useSwapPartnerStages();
  const refusalId = useId();
  const [naming, setNaming] = useState<Naming | null>(null);
  const [openings, setOpenings] = useState(0);
  const [removing, setRemoving] = useState<PartnerStage | null>(null);
  const stages = rowsOf(query.data);
  const open = (next: Naming) => {
    setOpenings((count) => count + 1);
    setNaming(next);
  };
  const failure = update.error ?? swap.error;
  const columns = stageColumns({
    t,
    locale,
    canEdit,
    canRemove,
    stages,
    refusedFor: (stage) =>
      update.isError && update.variables?.id === stage.id
        ? refusalId
        : undefined,
    onPatch: (stage, body) => update.mutate({ id: stage.id, body }),
    verbs: {
      onRename: (stage) => open({ mode: "rename", stage }),
      onMove: (stage, by) => {
        const at = stages.findIndex((row) => row.id === stage.id);
        const neighbour = stages[at + by];
        if (neighbour) swap.mutate({ moving: stage, neighbour });
      },
      onRemove: setRemoving,
    },
  });
  const lastOrder = stages.reduce(
    (max, stage) => Math.max(max, stage.sort_order),
    0,
  );
  return (
    <Panel
      title={t("partnerStages.title")}
      titleAction={
        canCreate && (
          <Button onClick={() => open({ mode: "create" })}>
            {t("partnerStages.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("partnerStages.sub")}</PanelIntro>
      </PanelBody>
      {query.isSuccess && stages.length > 0 ? (
        <DataTable
          bleed
          fold
          label={t("partnerStages.title")}
          columns={columns}
          rows={[...stages]}
          rowKey={(stage) => stage.id}
          rowTestId={(stage) => `partner-stage-${stage.key}`}
        />
      ) : (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("partnerStages.loading")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      <PanelNotices
        readOnly={refusal && t(refusal)}
        refused={
          failure
            ? {
                title: t("leadSources.notSaved"),
                error: failure,
                id: refusalId,
              }
            : undefined
        }
      />
      {naming && (
        <StageNameDialog
          key={openings}
          naming={naming}
          nextOrder={lastOrder + 10}
          onClose={() => setNaming(null)}
        />
      )}
      <RemoveStageDialog stage={removing} onClose={() => setRemoving(null)} />
    </Panel>
  );
}
