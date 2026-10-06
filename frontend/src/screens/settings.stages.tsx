// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The stage ladder inside one pipeline: its shape, the open stages a reader
// puts in order by hand, the closing pair that always comes last, and the verbs
// that change a stage.
//
// Split from settings.pipelines.tsx beside it because a stage answers to its
// pipeline and not the other way round: this file receives the pipeline it
// draws and never reaches for the catalog around it.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, Lock } from "lucide-react";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import {
  Badge,
  Button,
  Disclosure,
  EmptyState,
  SectionHeader,
} from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Meter } from "../design-system/readings";
import { SortableList } from "../design-system/sortablelist";
import { StageStrip } from "../design-system/stagestrip";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { type CreateField, CreateRecordModal } from "./create";
import { EditAction } from "./edit";
import { StageExitCriteria } from "./settings.exitcriteria";
import { useStageOrder } from "./settings.pipelineorder";

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];
type T = ReturnType<typeof useT>;

// Coerces a form value down to the trimmed string these forms always produce.
// Exported because the pipeline form beside this one maps its own body the same
// way, and this file is the leaf of the two.
export function str(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}

// Narrows the form's free-text semantic into the Stage enum without a cast; an
// unrecognized value falls back to "open" rather than shipping a bad literal.
function stageSemantic(v: unknown): Stage["semantic"] {
  switch (v) {
    case "won":
      return "won";
    case "lost":
      return "lost";
    default:
      return "open";
  }
}

// What create and edit share. No position: a stage's place is set by where it
// is dragged, never typed.
function mapStageBody(v: Record<string, unknown>) {
  return {
    name: str(v.name),
    semantic: stageSemantic(v.semantic),
    win_probability: v.win_probability ? Number(str(v.win_probability)) : 0,
  };
}

function stageFields(t: T): CreateField[] {
  return [
    { key: "name", label: "stage.name", required: true },
    {
      key: "semantic",
      label: "stage.semantic",
      type: "select",
      required: true,
      options: [
        { value: "open", label: t("stage.semOpen") },
        { value: "won", label: t("stage.semWon") },
        { value: "lost", label: t("stage.semLost") },
      ],
    },
    { key: "win_probability", label: "stage.winProb", type: "number" },
  ];
}

/** The ladder in position order. */
export function ladderOf(pipeline: Pipeline): Stage[] {
  return [...(pipeline.stages ?? [])].sort((a, b) => a.position - b.position);
}

// A new stage is created at the end of the ladder; the server puts an open one
// in front of the closing pair in the same write.
async function createStage(
  input: Readonly<{ values: Record<string, string>; pipeline: Pipeline }>,
) {
  const last = Math.max(0, ...ladderOf(input.pipeline).map((s) => s.position));
  const { data, error } = await api.POST("/stages", {
    body: {
      ...mapStageBody(input.values),
      pipeline_id: input.pipeline.id,
      position: last + 1,
    },
  });
  if (error) {
    throwProblem(error);
  }
  return data;
}

function StageCreate({ pipeline }: Readonly<{ pipeline: Pipeline }>) {
  const t = useT();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: createStage,
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      toast.show(t("stage.added"));
    },
  });
  return (
    <>
      <Button
        data-testid={`new-stage-${pipeline.id}`}
        onClick={() => setOpen(true)}
      >
        {t("stage.new")}
      </Button>
      <CreateRecordModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("stage.new")}
        fields={stageFields(t)}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
        onSubmit={(values) => mutation.mutate({ values, pipeline })}
      />
    </>
  );
}

// Removal is pipeline:delete, not the pipeline:update the rest of this ladder
// runs on. Both refusals are the server's (a stage still holding deals, and the
// closing pair), so this asks and then shows what it was told: the refusal
// names the deals in the way, which is the part an admin acts on.
function StageRemove({
  stage,
  returnFocusTo,
}: Readonly<{ stage: Stage; returnFocusTo: () => HTMLElement | null }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const canRemove = useCanWrite("pipeline", "delete");
  const remove = useMutation({
    mutationFn: async (target: Stage) => {
      const { error } = await api.DELETE("/stages/{id}", {
        params: { path: { id: target.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // The refetched ladder FIRST, then the dialog: closing it hands focus
      // back to a list that must no longer hold this row.
      await queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      setOpen(false);
    },
  });
  const close = () => {
    remove.reset();
    setOpen(false);
  };
  if (!canRemove) {
    return null;
  }
  return (
    <>
      <Button
        data-testid={`remove-stage-${stage.id}`}
        onClick={() => setOpen(true)}
      >
        {t("stage.remove")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("stage.removeTitle")}
        confirmLabel={t("stage.removeConfirm")}
        confirmVariant="danger"
        pending={remove.isPending}
        error={remove.isError ? problemMessageOf(remove.error, t) : null}
        onConfirm={() => remove.mutate(stage)}
        returnFocusTo={returnFocusTo}
      >
        <p>{t("stage.removeBody", { name: stage.name })}</p>
      </ConfirmModal>
    </>
  );
}

function StageEdit({ stage }: Readonly<{ stage: Stage }>) {
  const t = useT();
  return (
    <EditAction<Stage>
      label={t("stage.edit")}
      savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
      invalidate="pipelines"
      recordKey="stage"
      record={{
        id: stage.id,
        name: stage.name,
        semantic: stage.semantic,
        win_probability: String(stage.win_probability),
      }}
      fields={stageFields(t)}
      update={async (values) => {
        const { data, error } = await api.PATCH("/stages/{id}", {
          params: { path: { id: stage.id } },
          body: mapStageBody(values),
        });
        if (error) {
          throwProblem(error);
        }
        return data;
      }}
    />
  );
}

// One open stage: its step number, its name, how likely a deal standing here is
// to close won, and its verbs, with the exit criteria folded under it.
function OpenStageRow({
  stage,
  step,
  above,
  canEdit,
  returnFocusTo,
}: Readonly<{
  stage: Stage;
  step: number;
  above: Stage | undefined;
  canEdit: boolean;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const reading = `${formatNumber(stage.win_probability, locale)}%`;
  // Pointed out, never refused: a ladder whose odds dip is usually a stage
  // dragged one place too far, and sometimes a pipeline built that way on
  // purpose. The reader decides which.
  const dips =
    above !== undefined && stage.win_probability < above.win_probability;
  return (
    <div className="stage-row">
      <span className="stage-step t-num" aria-hidden="true">
        {formatNumber(step, locale)}
      </span>
      <span className="stage-name">
        {stage.name}
        {dips && (
          <span className="stage-dip">
            <ArrowDown aria-hidden="true" />
            {t("stage.lowerThanAbove", {
              name: above.name,
              reading: `${formatNumber(above.win_probability, locale)}%`,
            })}
          </span>
        )}
      </span>
      <span className="stage-odds">
        <Meter
          value={stage.win_probability}
          max={100}
          label={t("stage.oddsOf", { name: stage.name })}
          dense
          flat
        />
        <span className="t-num">{reading}</span>
      </span>
      <span className="stage-verbs">
        {canEdit && <StageEdit stage={stage} />}
        <StageRemove stage={stage} returnFocusTo={returnFocusTo} />
      </span>
      <div className="stage-criteria">
        <Disclosure summary={t("stage.criteria.title")}>
          <StageExitCriteria
            stageId={stage.id}
            semantic={stage.semantic}
            canEdit={canEdit}
          />
        </Disclosure>
      </div>
    </div>
  );
}

function ClosingStages({
  closing,
  canEdit,
}: Readonly<{ closing: readonly Stage[]; canEdit: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  if (closing.length === 0) {
    return <EmptyState>{t("stage.closingMissing")}</EmptyState>;
  }
  return (
    <ul className="stage-closing">
      {closing.map((stage) => (
        <li key={stage.id} className="stage-row is-closing">
          <Lock className="stage-lock" aria-hidden="true" />
          <span className="stage-name">
            {stage.name}
            <Badge tone={stage.semantic === "won" ? "success" : "danger"}>
              {stage.semantic === "won"
                ? t("stage.semWon")
                : t("stage.semLost")}
            </Badge>
          </span>
          <span className="stage-odds t-num">
            {formatNumber(stage.win_probability, locale)}%
          </span>
          <span className="stage-verbs">
            {canEdit && <StageEdit stage={stage} />}
          </span>
        </li>
      ))}
    </ul>
  );
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
  const { locale } = useLocale();
  const hintId = useId();
  const list = useRef<HTMLDivElement>(null);
  const order = useStageOrder();
  const writable = canEdit && !pipeline.archived_at;
  const ladder = ladderOf(pipeline);
  const open = ladder.filter((stage) => stage.semantic === "open");
  const closing = ladder.filter((stage) => stage.semantic !== "open");
  const closingIds = closing.map((stage) => stage.id);
  return (
    <>
      <StageStrip
        label={t("pipeline.flow")}
        empty={t("stage.noneOpen")}
        steps={[...open, ...closing].map((stage) => ({
          key: stage.id,
          name: stage.name,
          probability: stage.win_probability,
          reading: `${formatNumber(stage.win_probability, locale)}%`,
          outcome: stage.semantic === "open" ? undefined : stage.semantic,
        }))}
      />
      <SectionHeader
        title={t("stage.openGroup")}
        level={3}
        actions={writable && <StageCreate pipeline={pipeline} />}
      />
      {writable && open.length > 1 && (
        <p id={hintId} className="t-caption stage-hint">
          {t("stage.orderHint")}
        </p>
      )}
      {/* tabIndex -1 so a removal can hand focus to the list it changed: the
          row's own Remove button is gone by then. */}
      <div ref={list} tabIndex={-1}>
        {open.length === 0 ? (
          <EmptyState plate title={t("stage.noneOpen")}>
            {t("stage.noneOpenNote")}
          </EmptyState>
        ) : (
          <SortableList
            label={t("stage.openGroup")}
            items={open.map((stage) => ({ key: stage.id, stage }))}
            hintId={writable ? hintId : undefined}
            busy={order.pending}
            onReorder={
              writable
                ? (keys) => order.reorder(pipeline, [...keys, ...closingIds])
                : undefined
            }
            labels={{
              handle: (item, position, count) =>
                t("stage.handle", {
                  name: item.stage.name,
                  position: formatNumber(position, locale),
                  total: formatNumber(count, locale),
                }),
              moved: (item, position, count) =>
                t("stage.moved", {
                  name: item.stage.name,
                  position: formatNumber(position, locale),
                  total: formatNumber(count, locale),
                }),
            }}
            renderItem={(item, position) => (
              <OpenStageRow
                stage={item.stage}
                step={position}
                above={open[open.indexOf(item.stage) - 1]}
                canEdit={writable}
                returnFocusTo={() => list.current}
              />
            )}
          />
        )}
      </div>
      <SectionHeader title={t("stage.closingGroup")} level={3} />
      <p className="t-caption stage-hint">{t("stage.closingNote")}</p>
      <ClosingStages closing={closing} canEdit={writable} />
    </>
  );
}
