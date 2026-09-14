// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The stage ladder inside one pipeline: the rows, and the three verbs that
// change them.
//
// Split from settings.pipelines.tsx beside it because a stage answers to its
// pipeline and not the other way round — this file names no pipeline type and
// imports nothing from there, so the direction is readable from the imports.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button, Disclosure } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { type CreateField, CreateRecordModal } from "./create";
import { EditAction } from "./edit";
import { StageExitCriteria } from "./settings.exitcriteria";

type Stage = components["schemas"]["Stage"];

// Coerces a form value (CreateAction's values are strings; EditAction's update
// callback widens to Record<string, unknown> so a screen COULD prefill
// non-string values) down to the trimmed string these forms always produce —
// mirrors deals.tsx's mapDealUpdate `str` helper, keeping create's and edit's
// transports on one map function without an `as` cast.
//
// Exported because the pipeline form beside this one maps its own body the same
// way, and this file is the leaf of the two: stages know nothing of pipelines,
// so the helper lives here and travels up rather than the reverse.
export function str(v: unknown): string {
  return typeof v === "string" ? v.trim() : "";
}

// Narrows the form's free-text semantic value into the Stage enum WITHOUT a
// cast (mirrors deals.tsx's forecastCategory) — an unrecognized value falls
// back to "open" rather than shipping a bad literal to the wire.
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

// UpdateStageRequest carries no pipeline_id (a stage never moves pipelines
// via this form) while CreateStageRequest requires one — so this returns
// only the fields the two requests share, and the create transport adds
// pipeline_id on top.
function mapStageBody(v: Record<string, unknown>) {
  return {
    name: str(v.name),
    position: v.position ? Number(str(v.position)) : 0,
    semantic: stageSemantic(v.semantic),
    win_probability: v.win_probability ? Number(str(v.win_probability)) : 0,
  };
}

function stageFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "name", label: "stage.name", required: true },
    { key: "position", label: "pipeline.position", type: "number" },
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

// Localized badge for a stage's semantic — open/won/lost each render as a
// short label rather than the raw enum value.
function stageSemanticLabel(
  semantic: Stage["semantic"],
  t: ReturnType<typeof useT>,
): string {
  if (semantic === "won") {
    return t("stage.semWon");
  }
  if (semantic === "lost") {
    return t("stage.semLost");
  }
  return t("stage.semOpen");
}

// Tone-less Badge shares the card-inset background it sits on (both resolve
// to var(--bgCard)) — the semantic pill needs an explicit tone to be visible.
function stageSemanticTone(
  semantic: Stage["semantic"],
): "success" | "danger" | "accent" {
  switch (semantic) {
    case "won":
      return "success";
    case "lost":
      return "danger";
    default:
      return "accent"; // open
  }
}

// The bespoke per-pipeline "new stage" trigger: CreateAction's testid
// (`new-record`) can't disambiguate multiple pipelines on one screen, so
// this composes the same Button + CreateRecordModal pieces directly rather
// than adding new form infra.
export function StageCreate({ pipelineId }: Readonly<{ pipelineId: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: async (values: Record<string, string>) => {
      const { data, error } = await api.POST("/stages", {
        body: { ...mapStageBody(values), pipeline_id: pipelineId },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
    },
  });
  return (
    <>
      <Button
        small
        data-testid={`new-stage-${pipelineId}`}
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
        onSubmit={(values) => mutation.mutate(values)}
      />
    </>
  );
}

// The removal half of the bounded stage surface. Both refusals are the
// server's — a stage still holding deals, and the terminal won/lost pair —
// so this asks and then shows what it was told rather than pre-judging
// from the row: the refusal names the deals standing in the way, which is
// the part an admin acts on, and a board read a minute ago would name the
// wrong ones.
function StageRemove({
  stage,
  returnFocusTo,
}: Readonly<{ stage: Stage; returnFocusTo: () => HTMLElement | null }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  // Removal is pipeline:delete, not the pipeline:update everything else on
  // this card runs on — the server gates it that way, so a principal who
  // may add and rename stages but not remove one is not shown a control
  // that could only ever answer 403. Read before the early return: the
  // hooks a render performs must not depend on the answer.
  const canRemove = useCanWrite("pipeline", "delete");
  const remove = useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE("/stages/{id}", {
        params: { path: { id: stage.id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      // The refetched pipelines FIRST, then the dialog: closing it hands
      // focus back to a list that must no longer hold this row.
      await queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      setOpen(false);
    },
  });
  // A refusal is about the workspace's state, not the dialog's: reopening
  // must ask again rather than reprint what the last attempt was told.
  //
  // REFUSED WHILE THE DELETE IS OUT, and that is not tidiness. ConfirmModal
  // hands Escape and a backdrop press to this, and reset() clears isPending
  // without cancelling the request already in flight — so a reader who pressed
  // Escape could reopen and confirm again, and the stage would be deleted
  // twice. The second one answers 404 against a row the first already took.
  const close = () => {
    if (remove.isPending) {
      return;
    }
    remove.reset();
    setOpen(false);
  };
  if (!canRemove) {
    return null;
  }
  return (
    <>
      {/* Ghost, not danger. The dialog behind it is where the danger lives —
          its confirm is the red one — and a trigger that shouts as loudly as
          the act it only ASKS about put six solid red buttons in one pipeline,
          which is the shout a reader stops reading. */}
      <Button
        small
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
        onConfirm={() => remove.mutate()}
        // The stage list, not the trigger: a successful removal unmounts
        // the row this button lives in, so there is nothing to hand focus
        // back to (design-system/confirmmodal).
        returnFocusTo={returnFocusTo}
      >
        <p className="t-caption">
          {t("stage.removeBody", { name: stage.name })}
        </p>
      </ConfirmModal>
    </>
  );
}

export function StageRow({
  stage,
  canEdit,
  t,
  returnFocusTo,
}: Readonly<{
  stage: Stage;
  canEdit: boolean;
  t: ReturnType<typeof useT>;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const { locale } = useLocale();
  return (
    // The four tracks (name, semantic badge, probability, edit) live in
    // settings.css, where they can have a phone breakpoint. Inline, the three
    // fixed tracks plus the Edit button left about 60px for a stage name that
    // could not wrap, and it painted straight over the badge beside it.
    <li className="stage-row">
      <span className="stage-name">{stage.name}</span>
      <Badge tone={stageSemanticTone(stage.semantic)}>
        {stageSemanticLabel(stage.semantic, t)}
      </Badge>
      <span className="t-mono t-caption">
        {formatNumber(stage.win_probability, locale)}%
      </span>
      {/* Each control carries its own verb — editing a stage is
          pipeline:update, removing one is pipeline:delete — so a role
          holding one without the other still sees the one it may use. */}
      <span className="stage-verbs">
        {canEdit && (
          <EditAction<Stage>
            label={t("stage.edit")}
            savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
            invalidate="pipelines"
            recordKey="stage"
            record={{
              id: stage.id,
              name: stage.name,
              position: String(stage.position),
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
        )}
        <StageRemove stage={stage} returnFocusTo={returnFocusTo} />
      </span>
      {/* The criteria sit UNDER the stage's own line rather than beside it:
          a stage carries three to six of them, each with a label, a key, two
          badges and two verbs, which is a list and not an answer that fits in
          a track. */}
      <div className="stage-criteria">
        <Disclosure summary={t("stage.criteria.title")}>
          <StageExitCriteria
            stageId={stage.id}
            semantic={stage.semantic}
            canEdit={canEdit}
          />
        </Disclosure>
      </div>
    </li>
  );
}
