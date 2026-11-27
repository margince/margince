// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The verbs that change one stage: its create and edit forms, its row menu,
// and its removal.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import {
  type CreateConflict,
  type CreateField,
  CreateRecordModal,
} from "../../components/create";
import { fieldRefusals } from "../../components/create.refusals";
import { EditAction } from "../../components/edit";
import {
  translationFields,
  translationsFromValues,
  translationValues,
} from "../../components/labeltranslations";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useLastSubject } from "../../design-system/presence";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { STAGE_CHANCES_KEY } from "../../lib/activitykeys";
import { inPositionOrder } from "../../lib/pipelines.queries";
import {
  isVersionSkew,
  isVersionSkewOf,
  percentLimit,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { winProbabilityFault } from "../../lib/wholenumber";

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];
type T = ReturnType<typeof useT>;

// Exported because the pipeline form maps its body the same way, and this file
// is the leaf it imports.
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
    name_translations: translationsFromValues(STAGE_NAME, v),
    semantic: stageSemantic(v.semantic),
    win_probability: v.win_probability ? Number(str(v.win_probability)) : 0,
  };
}

// The form keys the stage's name in further languages travel under.
const STAGE_NAME = "name";

// `fixedType` is the type a stage must keep because it is its pipeline's only
// Won or Lost stage; the server refuses the change too.
function stageFields(
  t: T,
  locale: Locale,
  fixedType?: Stage["semantic"],
): CreateField[] {
  const chance = { label: t("stage.winProb"), limit: percentLimit(locale) };
  return [
    {
      key: "name",
      label: "stage.name",
      required: true,
      maxLength: TEXT_LIMITS.label,
    },
    ...translationFields(STAGE_NAME),
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
      ...(fixedType && {
        hint: t("stage.lastClosing"),
        validate: (value: string) =>
          value === fixedType ? undefined : t("stage.lastClosing"),
      }),
    },
    {
      key: "win_probability",
      label: "stage.winProb",
      type: "number",
      limit: chance.limit,
      // A blank would be stored as 0%, which the forecast reads as a stage that
      // never wins. A closing stage's chance is fixed by its type.
      requiredWhen: (values) => stageSemantic(values.semantic) === "open",
      validate: (value) => winProbabilityFault(value, locale, chance),
    },
  ];
}

function stageNameConflict(t: T): CreateConflict {
  return {
    field: "name",
    code: "stage_name_taken",
    sentence: t("stage.nameTaken"),
  };
}

// The closing stages a pipeline cannot do without: the only live one of a kind.
export function onlyClosingStages(ladder: readonly Stage[]): Set<string> {
  const only = new Set<string>();
  for (const kind of ["won", "lost"] as const) {
    const ofKind = ladder.filter((stage) => stage.semantic === kind);
    const [sole] = ofKind;
    if (sole && ofKind.length === 1) only.add(sole.id);
  }
  return only;
}

export function ladderOf(pipeline: Pipeline): Stage[] {
  return inPositionOrder(pipeline.stages ?? []);
}

// A new stage is created at the end of the ladder. The server moves an open
// one in front of the closing pair in the same write.
async function createStage(
  input: Readonly<{ values: Record<string, string>; pipeline: Pipeline }>,
) {
  const last = Math.max(0, ...ladderOf(input.pipeline).map((s) => s.position));
  return unwrap(
    await api.POST("/stages", {
      body: {
        ...mapStageBody(input.values),
        pipeline_id: input.pipeline.id,
        position: last + 1,
      },
    }),
  );
}

export function StageCreate({
  pipeline,
  reason,
}: Readonly<{ pipeline: Pipeline; reason?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();
  const fields = stageFields(t, locale);
  const mutation = useMutation({
    mutationFn: createStage,
    onSuccess: () => {
      setOpen(false);
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      queryClient.invalidateQueries({ queryKey: STAGE_CHANCES_KEY });
      toast.show(t("stage.added"));
    },
  });
  const refused = fieldRefusals(
    mutation.error,
    fields,
    t,
    stageNameConflict(t),
  );
  return (
    <>
      <Button
        data-testid={`new-stage-${pipeline.id}`}
        reason={reason}
        onClick={() => setOpen(true)}
      >
        {t("stage.new")}
      </Button>
      <CreateRecordModal
        // Closes when a refusal lands mid-edit, as CreateAction's does.
        open={open && reason === undefined}
        onClose={() => setOpen(false)}
        title={t("stage.new")}
        fields={fields}
        pending={mutation.isPending}
        error={mutation.isError ? refused.rest : null}
        fieldRefusals={refused.byField}
        refusedAt={mutation.isError ? mutation.submittedAt : undefined}
        onSubmit={(values) => mutation.mutate({ values, pipeline })}
      />
    </>
  );
}

// Invalidating the pipelines re-reads the whole ladder: a save that moves one
// stage renumbers its siblings, and each sibling's version moves with it.
function StageEdit({
  stage,
  fixedType,
}: Readonly<{ stage: Stage; fixedType: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  return (
    <EditAction<Stage>
      labelled
      label={t("stage.edit")}
      savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
      invalidate="pipelines"
      recordKey="stage"
      record={{
        id: stage.id,
        version: stage.version,
        name: stage.name,
        ...translationValues(STAGE_NAME, stage.name_translations),
        semantic: stage.semantic,
        win_probability: String(stage.win_probability),
      }}
      fields={stageFields(t, locale, fixedType ? stage.semantic : undefined)}
      conflict={stageNameConflict(t)}
      update={async (values, _rows, opened) => {
        const answer = await api.PATCH("/stages/{id}", {
          params: {
            path: { id: stage.id },
            ...ifMatch(requireVersion(opened?.version)),
          },
          body: mapStageBody(values),
        });
        // A stale version means the ladder moved; the refusal says so, and
        // the next opening of this form starts from the re-read row.
        if (isVersionSkew(answer.error)) {
          queryClient.invalidateQueries({ queryKey: ["pipelines"] });
        }
        return unwrap(answer);
      }}
    />
  );
}

export type StageVerbs = Readonly<{
  writable: boolean;
  canRemove: boolean;
  /** Ids of the stages that are their pipeline's only Won or only Lost stage. */
  onlyOfKind: ReadonlySet<string>;
  onAddCriterion: (stage: Stage) => void;
  onRemove: (stage: Stage) => void;
}>;

// The closing pair takes no criteria and no removal: the server refuses both.
export function StageMenu({
  stage,
  verbs,
}: Readonly<{ stage: Stage; verbs: StageVerbs }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const open = stage.semantic === "open";
  const removes = verbs.canRemove && open;
  if (!verbs.writable && !removes) {
    return null;
  }
  const deals = stage.deal_count ?? 0;
  return (
    <span className="cell-actions">
      <OverflowMenu label={t("table.rowActions", { name: stage.name })}>
        {verbs.writable && (
          <StageEdit stage={stage} fixedType={verbs.onlyOfKind.has(stage.id)} />
        )}
        {verbs.writable && open && (
          <Button
            aria-haspopup="dialog"
            onClick={() => verbs.onAddCriterion(stage)}
          >
            {t("stage.criteria.add")}
          </Button>
        )}
        {removes && (
          <Button
            aria-haspopup="dialog"
            reason={
              deals > 0
                ? plural("stage.occupied", deals, {
                    count: formatNumber(deals, locale),
                  })
                : undefined
            }
            onClick={() => verbs.onRemove(stage)}
          >
            {t("stage.remove")}
          </Button>
        )}
      </OverflowMenu>
    </span>
  );
}

// Removal is pipeline:delete, not pipeline:update. The server also refuses a
// stage holding deals the reader cannot see, so its refusal shows in the dialog.
export function useStageRemoval(
  ladder: readonly Stage[],
  // Takes focus when no row is left with a menu to hand it to.
  landing: () => HTMLElement | null,
) {
  const queryClient = useQueryClient();
  const [removing, setRemoving] = useState<Stage | null>(null);
  const asked = useLastSubject(removing);
  // Null until the confirm is pressed: a dismissed dialog hands focus back to
  // its opener.
  const nextMenus = useRef<readonly string[] | null>(null);
  const remove = useMutation({
    mutationFn: async (target: Stage) => {
      unwrap(
        await api.DELETE("/stages/{id}", {
          params: {
            path: { id: target.id },
            ...ifMatch(requireVersion(target.version)),
          },
        }),
      );
    },
    onSuccess: async () => {
      // The refetched ladder first, then the dialog: closing it hands focus to
      // a row that must already be drawn.
      await queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      void queryClient.invalidateQueries({ queryKey: STAGE_CHANCES_KEY });
      setRemoving(null);
    },
    // Held pending until the re-read lands, so a second press cannot resend
    // the stale version.
    onError: (error) =>
      isVersionSkewOf(error)
        ? queryClient.invalidateQueries({ queryKey: ["pipelines"] })
        : undefined,
  });
  return {
    asked,
    remove,
    open: removing !== null,
    ask: (stage: Stage) => {
      remove.reset();
      nextMenus.current = null;
      setRemoving(stage);
    },
    close: () => {
      nextMenus.current = null;
      setRemoving(null);
    },
    confirm: () => {
      if (!removing) return;
      const at = ladder.findIndex((stage) => stage.id === removing.id);
      nextMenus.current = [
        ...ladder.slice(at + 1),
        ...ladder.slice(0, Math.max(at, 0)).reverse(),
      ].map((stage) => stage.id);
      remove.mutate(ladder[at] ?? removing);
    },
    // The removed row took its menu with it; the nearest row's menu takes focus.
    focusAfter: () => {
      if (nextMenus.current === null) return null;
      for (const id of nextMenus.current) {
        const menu = document.querySelector<HTMLElement>(
          `[data-testid="stage-${id}"] .overflow-menu > button`,
        );
        if (menu) return menu;
      }
      return landing();
    },
  };
}

export function StageRemoveConfirm({
  removal,
}: Readonly<{ removal: ReturnType<typeof useStageRemoval> }>) {
  const t = useT();
  const { remove } = removal;
  return (
    <ConfirmModal
      open={removal.open}
      onClose={removal.close}
      title={t("stage.removeTitle")}
      confirmLabel={t("stage.removeConfirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.isError ? problemMessageOf(remove.error, t) : null}
      onConfirm={removal.confirm}
      returnFocusTo={removal.focusAfter}
    >
      <p>{t("stage.removeBody", { name: removal.asked?.name ?? "" })}</p>
    </ConfirmModal>
  );
}
