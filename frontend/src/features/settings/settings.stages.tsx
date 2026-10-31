// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The stage ladder inside one pipeline, as one table. The open stages come
// first, in the order a reader sets, and the closing pair always last.
//
// Split from settings.pipelines.tsx beside it because a stage answers to its
// pipeline and not the other way round: this file receives the pipeline it
// draws and never reaches for the catalog around it.

import { ArrowDown, Lock } from "lucide-react";
import { useId, useRef, useState } from "react";
import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { PanelBody, PanelGroupHead } from "../../design-system/panel";
import { useLastSubject } from "../../design-system/presence";
import { Meter } from "../../design-system/readings";
import { foldForMatch } from "../../format/collate";
import { formatNumber } from "../../format/format";
import { type PluralTranslator, useLocale, usePlural, useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { useCanWrite, useWriteRefusal } from "../../lib/capability";
import { useMe } from "../../lib/session";
import {
  CriterionCreateDialog,
  StageExitCriteria,
} from "./settings.exitcriteria";
import { useStageOrder } from "./settings.pipelineorder";
import { useStageChances } from "./settings.stagechances";
import {
  ladderOf,
  StageCreate,
  StageMenu,
  StageRemoveConfirm,
  type StageVerbs,
  useStageRemoval,
} from "./settings.stageverbs";

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];
type T = ReturnType<typeof useT>;

// A badge only where the name leaves the outcome unsaid: "Won" needs no [Won].
function ClosingName({ stage }: Readonly<{ stage: Stage }>) {
  const t = useT();
  const outcome =
    stage.semantic === "won" ? t("stage.semWon") : t("stage.semLost");
  const said = foldForMatch(stage.name.trim()) === foldForMatch(outcome);
  return (
    <span className="stage-name">
      {stage.name}
      {!said && (
        <Badge tone={stage.semantic === "won" ? "success" : "danger"}>
          {outcome}
        </Badge>
      )}
    </span>
  );
}

// Pointed out, never refused. A dip is usually a stage dragged one place too
// far, and sometimes a pipeline built that way.
function OpenName({
  stage,
  above,
}: Readonly<{ stage: Stage; above: Stage | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  if (above === undefined || stage.win_probability >= above.win_probability) {
    return <span className="stage-name">{stage.name}</span>;
  }
  return (
    <CellStack>
      <span className="stage-name">{stage.name}</span>
      <span className="stage-dip">
        <ArrowDown aria-hidden="true" />
        {t("stage.lowerThanAbove", {
          name: above.name,
          reading: `${formatNumber(above.win_probability, locale)}%`,
        })}
      </span>
    </CellStack>
  );
}

export type StageChances = ReturnType<typeof useStageChances>["data"];

// What the forecast makes of the typed chance: the recorded deals from this
// stage, and the chance it weights an open deal by.
export function StageChanceLine({
  stage,
  chances,
}: Readonly<{ stage: Stage; chances: StageChances }>) {
  const t = useT();
  const { locale } = useLocale();
  const chance = chances?.byStage.get(stage.id);
  if (!chances || !chance) return null;
  const pct = (value: number) => `${formatNumber(value, locale)}%`;
  const typed = pct(chance.typed_probability);
  let line: string;
  if (!chances.selfCorrecting) {
    line = t("stage.chance.off", { typed });
  } else if (chance.won_percent === undefined) {
    line = t("stage.chance.noEvidence", {
      typed,
      used: pct(chance.chance_in_use),
    });
  } else {
    line = t("stage.chance.withEvidence", {
      typed,
      count: formatNumber(chance.evidence_n, locale),
      won: pct(chance.won_percent),
      used: pct(chance.chance_in_use),
    });
  }
  return <span className="t-caption">{line}</span>;
}

type ColumnsOf = Readonly<{
  t: T;
  chances: StageChances | undefined;
  plural: PluralTranslator;
  locale: Locale;
  open: readonly Stage[];
  verbs: StageVerbs;
}>;

function criteriaCell(stage: Stage, { t, plural, locale }: ColumnsOf) {
  if (stage.semantic !== "open") return t("stage.endsDeal");
  const counts = stage.exit_criteria;
  if (counts === undefined) return "—";
  const parts = [
    counts.required > 0 &&
      plural("stage.criteriaRequired", counts.required, {
        count: formatNumber(counts.required, locale),
      }),
    counts.optional > 0 &&
      plural("stage.criteriaOptional", counts.optional, {
        count: formatNumber(counts.optional, locale),
      }),
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : t("stage.criteriaNoneFolded");
}

function criteriaFolded(stage: Stage, { plural, locale }: ColumnsOf) {
  const counts = stage.exit_criteria;
  if (stage.semantic !== "open" || counts === undefined) return undefined;
  const all = counts.required + counts.optional;
  if (all === 0) return undefined;
  return plural("stage.criteriaCount", all, {
    count: formatNumber(all, locale),
  });
}

function stageColumns(of: ColumnsOf): DataTableColumn<Stage>[] {
  const { t, locale, open, verbs, chances } = of;
  const reading = (stage: Stage) =>
    `${formatNumber(stage.win_probability, locale)}%`;
  return [
    {
      key: "step",
      header: t("stage.colStep"),
      align: "end",
      fold: "hide",
      render: (stage) =>
        stage.semantic === "open" ? (
          formatNumber(open.indexOf(stage) + 1, locale)
        ) : (
          <Lock
            className="stage-lock"
            role="img"
            aria-label={t("stage.fixedLast")}
          />
        ),
    },
    {
      key: "name",
      header: t("stage.colStage"),
      fold: "title",
      grow: true,
      render: (stage) =>
        stage.semantic === "open" ? (
          <OpenName stage={stage} above={open[open.indexOf(stage) - 1]} />
        ) : (
          <ClosingName stage={stage} />
        ),
    },
    {
      key: "odds",
      header: t("stage.winProb"),
      align: "end",
      render: (stage) => (
        <CellStack>
          <span className="stage-odds">
            <Meter
              value={stage.win_probability}
              max={100}
              label={t("stage.oddsOf", { name: stage.name })}
              dense
              flat
            />
            {reading(stage)}
          </span>
          {stage.semantic === "open" && (
            <StageChanceLine stage={stage} chances={chances} />
          )}
        </CellStack>
      ),
      folded: reading,
    },
    {
      key: "criteria",
      header: t("stage.criteria.title"),
      render: (stage) => criteriaCell(stage, of),
      folded: (stage) => criteriaFolded(stage, of),
    },
    {
      key: "verbs",
      header: t("table.actions"),
      headerHidden: true,
      align: "end",
      fold: "end",
      render: (stage) => <StageMenu stage={stage} verbs={verbs} />,
    },
  ];
}

/**
 * The ladder of one pipeline. A retired pipeline's ladder is read-only: the
 * server refuses every stage write on it, so this draws no verb it would refuse.
 */
export function StageLadderEditor({
  pipeline,
  canEdit,
}: Readonly<{ pipeline: Pipeline; canEdit: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const hintId = useId();
  const { locale } = useLocale();
  const order = useStageOrder();
  const writable = canEdit && !pipeline.archived_at;
  const canRemove = useCanWrite("pipeline", "delete") && !pipeline.archived_at;
  const refusal = useWriteRefusal("pipeline", "update", {
    seat: t("settings.seatRefused"),
    grant: t("stage.createRefused"),
  });
  // A /me still loading denies every verb without refusing any of them.
  const answered = useMe().isSuccess;
  const ladder = ladderOf(pipeline);
  const open = ladder.filter((stage) => stage.semantic === "open");
  const orders = writable && open.length > 1;
  const head = useRef<HTMLSpanElement>(null);
  const removal = useStageRemoval(
    ladder,
    () =>
      document.querySelector<HTMLElement>(
        // A refused New stage cannot take focus, so the heading does.
        `[data-testid="new-stage-${pipeline.id}"]:not(:disabled)`,
      ) ?? head.current,
  );
  const [adding, setAdding] = useState<Stage | null>(null);
  const added = useLastSubject(adding);
  const [openStage, setOpenStage] = useState<string | null>(null);
  const toggle = (key: string, keep = false) =>
    setOpenStage((current) => (current === key && !keep ? null : key));
  const expanded = new Set(openStage === null ? [] : [openStage]);
  const verbs: StageVerbs = {
    writable,
    canRemove,
    onAddCriterion: (stage) => {
      toggle(stage.id, true);
      setAdding(stage);
    },
    onRemove: removal.ask,
  };
  const label = t("stage.group");
  const chances = useStageChances();
  return (
    <>
      <PanelGroupHead
        level="h3"
        title={
          <span ref={head} tabIndex={-1}>
            {label}
          </span>
        }
        action={
          // A retired ladder takes no stage from anyone and says so above; a
          // live one keeps its verb, refused with why. One element in both
          // states, so a refusal landing mid-edit reaches the open dialog.
          !pipeline.archived_at &&
          (writable || answered) && (
            <>
              <StageCreate
                pipeline={pipeline}
                reason={writable ? undefined : refusal}
              />
              {orders && (
                <span id={hintId} className="sr-only">
                  {t("stage.orderHint")}
                </span>
              )}
            </>
          )
        }
      />
      {ladder.length === 0 ? (
        <PanelBody>
          <EmptyState plate title={t("stage.noneOpen")}>
            {t("stage.noneOpenNote")}
          </EmptyState>
        </PanelBody>
      ) : (
        <DataTable<Stage>
          bleed
          fold
          label={label}
          rows={ladder}
          rowKey={(stage) => stage.id}
          rowTestId={(stage) => `stage-${stage.id}`}
          columns={stageColumns({
            t,
            plural,
            locale,
            open,
            verbs,
            chances: chances.data,
          })}
          detail={{
            header: t("stage.criteriaDetail"),
            toggleLabel: (stage) =>
              t("stage.criteriaFor", { name: stage.name }),
            has: (stage) => stage.semantic === "open",
            expanded,
            onToggle: toggle,
            render: (stage) => (
              <StageExitCriteria
                stageId={stage.id}
                stageName={stage.name}
                canEdit={writable}
              />
            ),
          }}
          reorder={
            orders
              ? {
                  header: t("stage.orderHeader"),
                  hintId,
                  busy: order.pending,
                  can: (stage) => stage.semantic === "open",
                  labels: {
                    handle: (stage, position, count) =>
                      t("stage.handle", {
                        name: stage.name,
                        position: formatNumber(position, locale),
                        total: formatNumber(count, locale),
                      }),
                    moved: (stage, position, count) =>
                      t("stage.moved", {
                        name: stage.name,
                        position: formatNumber(position, locale),
                        total: formatNumber(count, locale),
                      }),
                  },
                  onReorder: (keys) => order.reorder(pipeline, keys),
                }
              : undefined
          }
        />
      )}
      {ladder.length > 0 &&
        (open.length === 0 || open.length === ladder.length) && (
          <PanelBody>
            <p className="t-caption stage-hint">
              {open.length === 0
                ? t("stage.noneOpenNote")
                : t("stage.closingMissing")}
            </p>
          </PanelBody>
        )}
      <CriterionCreateDialog
        stageId={added?.id ?? ""}
        open={adding !== null}
        onClose={() => setAdding(null)}
      />
      <StageRemoveConfirm removal={removal} />
    </>
  );
}
