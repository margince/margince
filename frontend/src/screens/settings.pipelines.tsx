// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Settings -> Data model -> Pipelines: the ladders a deal moves through, the
// stages on each, and retiring or restoring one.
//
// Its own file rather than a sixth zone of settings.tsx, and the split is where
// it is because the whole of it hangs off one record type: a pipeline, its
// stages, and the three verbs that change them. Nothing outside reaches in
// except PipelinesCard, which is what the settings screen mounts.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { ArchiveAction } from "./archive";
import { QueryGate, throwProblem } from "./common";
import { CreateAction, type CreateField } from "./create";
import { EditAction } from "./edit";
import { StageCreate, StageRow, str } from "./settings.stages";

type Pipeline = components["schemas"]["Pipeline"];

// The 3 shared scalar fields between create and edit pipeline forms.
function pipelineFields(t: ReturnType<typeof useT>): CreateField[] {
  return [
    { key: "name", label: "pipeline.name", required: true },
    {
      key: "is_default",
      label: "pipeline.default",
      type: "select",
      required: true,
      options: [
        { value: "false", label: t("pipeline.notDefault") },
        { value: "true", label: t("pipeline.default") },
      ],
    },
    { key: "position", label: "pipeline.position", type: "number" },
  ];
}

function mapPipelineBody(v: Record<string, unknown>) {
  return {
    name: str(v.name),
    is_default: v.is_default === "true",
    position: v.position ? Number(str(v.position)) : 0,
  };
}

// Retiring a pipeline, and putting one back.
//
// Both verbs live on the row rather than behind the card's create button,
// because what they act on is THIS ladder and a reader deciding to retire one
// is looking at the one they mean.
//
// The default pipeline is refused by the server (`default_pipeline_not_archivable`)
// and the control says so instead of letting the reader find out by pressing
// it: STATE-4a — a control blocked by the record's STATE rather than by
// permission stays visible and disabled WITH the reason, because the reason is
// the information. The remedy is one the reader can act on from this same row,
// so naming it is not a dead end.
function PipelineRetirement({
  pipeline,
  canRetire,
  canRestore,
  t,
}: Readonly<{
  pipeline: Pipeline;
  canRetire: boolean;
  canRestore: boolean;
  t: ReturnType<typeof useT>;
}>) {
  const queryClient = useQueryClient();
  const toast = useToast();
  const blockedId = useId();
  const restore = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST("/pipelines/{id}/restore", {
        params: { path: { id: pipeline.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (restored) => {
      queryClient.invalidateQueries({ queryKey: ["pipelines"] });
      toast.show(t("pipeline.restored", { name: restored.name }));
    },
  });

  if (pipeline.archived_at) {
    if (!canRestore) {
      return null;
    }
    return (
      <Button
        small
        onClick={() => restore.mutate()}
        disabled={restore.isPending}
      >
        {t("pipeline.restore")}
      </Button>
    );
  }
  if (!canRetire) {
    return null;
  }
  return (
    <>
      <ArchiveAction
        label={t("pipeline.retire")}
        confirmText={t("pipeline.retireConfirm", { name: pipeline.name })}
        // The version travels as If-Match: retiring a pipeline somebody else
        // has just renamed or made default is a decision taken about a record
        // the reader was not looking at. requireVersion rather than a fallback,
        // because an unpinned DELETE is last-write-wins and this one cannot be
        // taken back by the reader who lost the race.
        archive={async () => {
          const { error } = await api.DELETE("/pipelines/{id}", {
            params: {
              path: { id: pipeline.id },
              ...ifMatch(requireVersion(pipeline.version)),
            },
          });
          if (error) {
            throwProblem(error);
          }
          return pipeline;
        }}
        invalidate="pipelines"
        recordKey="pipeline"
        archivedMessage={t("pipeline.retired.done", { name: pipeline.name })}
        onArchived={() => {}}
        disabledReasonId={pipeline.is_default ? blockedId : undefined}
      />
      {pipeline.is_default && (
        // t-caption, the same treatment Button gives a reason it renders
        // itself: ArchiveAction takes only the id of a sentence the page
        // already owns, so the sentence is written here and must read the
        // same as one the design system would have drawn.
        <span id={blockedId} className="t-caption">
          {t("pipeline.retireBlocked")}
        </span>
      )}
    </>
  );
}

// One pipeline as one row: its name on the left, and under it what the pipeline
// IS — default or not — the verbs that change it, and the stage ladder itself.
//
// Stacked, because the ladder IS the subject rather than an answer to a question
// that fits beside it: three to six stages, each carrying a name, a semantic
// badge, a probability and two verbs of its own. The pipeline's own name is the
// row's LABEL, which is what puts it at the same x as every other naming on the
// page — it used to be an inner heading, drawn one step larger than the card
// title above it.
function PipelineRow({
  pipeline,
  canEdit,
  canRetire,
  t,
}: Readonly<{
  pipeline: Pipeline;
  canEdit: boolean;
  // Retiring is pipeline:DELETE, a different verb from everything else this
  // row offers — the same split stage removal already makes. Putting one back
  // is pipeline:update, so it rides canEdit: restoring is not a deletion and
  // a seat that may rename a pipeline may un-retire one.
  canRetire: boolean;
  t: ReturnType<typeof useT>;
}>) {
  const stageList = useRef<HTMLUListElement>(null);
  const stages = [...(pipeline.stages ?? [])].sort(
    (a, b) => a.position - b.position,
  );
  return (
    <SettingRow
      label={pipeline.name}
      layout="stack"
      control={
        <div className="form-stack settingrow-measure">
          {/* What this pipeline IS, and the verbs that change it, above the
              ladder they act on. */}
          <div className="pipeline-standing">
            {/* Retired is the leading fact about a pipeline that has one: a
                reader scanning this list needs to know which ladders are still
                offered before anything else about them, and the default badge
                beside it would otherwise be the only mark on the row. */}
            {pipeline.archived_at && (
              // No tone: retiring a pipeline is a deliberate choice somebody
              // made, not a status going badly, and Badge's tones say how a
              // thing is going.
              <Badge>{t("pipeline.retired")}</Badge>
            )}
            <Badge tone={pipeline.is_default ? "success" : undefined}>
              {pipeline.is_default
                ? t("pipeline.default")
                : t("pipeline.notDefault")}
            </Badge>
            {canEdit && (
              <>
                <EditAction<Pipeline>
                  label={t("pipeline.edit")}
                  savedMessage={(saved) =>
                    t("record.saveDone", { name: saved.name })
                  }
                  invalidate="pipelines"
                  recordKey="pipeline"
                  record={{
                    id: pipeline.id,
                    name: pipeline.name,
                    is_default: String(pipeline.is_default),
                    position: String(pipeline.position),
                  }}
                  fields={pipelineFields(t)}
                  update={async (values) => {
                    const { data, error } = await api.PATCH("/pipelines/{id}", {
                      params: { path: { id: pipeline.id } },
                      body: mapPipelineBody(values),
                    });
                    if (error) {
                      throwProblem(error);
                    }
                    return data;
                  }}
                />
                <StageCreate pipelineId={pipeline.id} />
              </>
            )}
            <PipelineRetirement
              pipeline={pipeline}
              canRetire={canRetire}
              canRestore={canEdit}
              t={t}
            />
          </div>
          {/* tabIndex -1 so a removal can hand focus to the list it changed:
              the row's own Remove button is gone by then, and focus dropped to
              <body> leaves a screen-reader user at the top of the document. */}
          <ul ref={stageList} tabIndex={-1} className="stage-rows">
            {stages.map((stage) => (
              <StageRow
                key={stage.id}
                stage={stage}
                canEdit={canEdit}
                t={t}
                returnFocusTo={() => stageList.current}
              />
            ))}
          </ul>
        </div>
      }
    />
  );
}

// D-8: Settings → Pipelines config. Reads via the SAME ["pipelines","all"]
// key the deals screen's plural selector uses (an array shape, distinct
// from DealScreen's single-pipeline ["pipelines"] cache entry) — any
// mutation here invalidates the ["pipelines"] prefix, so both shapes stay
// fresh. The list itself is readable by everyone; only the write affordances are
// gated, and the server stays the RBAC authority. Three of the five seeded roles
// hold pipeline READ and no write verb at all, so for most readers this card is
// the read-only case rather than an edge of it — which is why it states that
// posture once instead of leaving a reader to infer it from absent buttons.
export function PipelinesCard() {
  const t = useT();
  // Adding a pipeline is pipeline:create. Everything else here — renaming a
  // pipeline, adding a stage, editing one, reordering — is pipeline:update,
  // including the stage CREATE affordance: a stage is not its own RBAC object,
  // so adding one is an update to the pipeline that owns it.
  const canCreate = useCanWrite("pipeline", "create");
  const canEdit = useCanWrite("pipeline", "update");
  // Retiring one is pipeline:delete. The card's read-only line below still
  // turns on the two verbs that draw most of it; a seat holding delete alone
  // sees the retire verb and nothing else, which is what it holds.
  const canRetire = useCanWrite("pipeline", "delete");
  // Its OWN cache key, and the reason is the whole point of retiring one. The
  // deal board, the stage-automation picker and the lead qualifier all read
  // ["pipelines","all"], and a retired pipeline must not appear in any of them
  // — so this card cannot widen that entry to include archived rows without
  // putting retired pipelines back in the three places a retirement removes
  // them from. A longer key still matches the ["pipelines"] prefix every
  // mutation here invalidates, so both shapes stay fresh.
  const query = useQuery({
    queryKey: ["pipelines", "all", "including-retired"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: { include_archived: true } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  return (
    <Panel
      title={t("settings.pipelines")}
      // Adding a pipeline is four inputs committed together, so the header
      // keeps the verb and the dialog keeps the form. It sits in the header
      // band rather than as a trailing row: a row's label would repeat the
      // button beside it, and a card-level create verb is the header's job
      // everywhere else on these pages. Absent without the create grant,
      // exactly as each Edit verb is — the read-only posture is stated once
      // below.
      titleAction={
        canCreate && (
          <CreateAction
            label={t("pipeline.new")}
            invalidate="pipelines"
            screen="settings"
            create={async (values) => {
              const { data, error } = await api.POST("/pipelines", {
                body: { ...mapPipelineBody(values), stages: [] },
              });
              if (error) {
                throwProblem(error);
              }
              return data;
            }}
            fields={pipelineFields(t)}
          />
        )
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("settings.pipelinesSub")}</p>
        {/* Said once, at the top, rather than annotating each absent control —
            the rule in design-system/README.md. A reader holding one of the two
            verbs can see for themselves which controls they got. */}
        {!canCreate && !canEdit && (
          <p className="settings-panel-sub">
            {t("settings.pipelinesReadOnly")}
          </p>
        )}
        <SettingList>
          <QueryGate
            pendingLabel={t("settings.pipelines")}
            query={query}
            empty={(pipelines) => pipelines.length === 0}
          >
            {(pipelines) =>
              pipelines.map((pipeline) => (
                <PipelineRow
                  key={pipeline.id}
                  pipeline={pipeline}
                  canEdit={canEdit}
                  canRetire={canRetire}
                  t={t}
                />
              ))
            }
          </QueryGate>
        </SettingList>
      </PanelBody>
    </Panel>
  );
}
