// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Settings -> Sales -> Pipelines: every pipeline the company runs, the one the
// reader has open, and that pipeline's ladder.
//
// The catalog stands ABOVE the pipeline it opens rather than beside it. Every
// settings page holds one measure, and a ladder row needs that width for its
// name, its odds and its verbs; a side-by-side split would take it away to
// show a list of a handful of names. Which pipeline is open lives in the
// address, so a reload or a shared link lands on the same one.

import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { useId } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useUrlParams } from "../app/urlstate";
import { Badge, Disclosure } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { SortableList } from "../design-system/sortablelist";
import { StageStrip } from "../design-system/stagestrip";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { QueryGate, unwrap } from "./common";
import { CreateAction, type CreateField } from "./create";
import { EditAction } from "./edit";
import { SETTINGS_PIPELINES, usePipelineOrder } from "./settings.pipelineorder";
import { MakeDefault, PipelineRetirement } from "./settings.pipelineverbs";
import { ladderOf, StageLadderEditor, str } from "./settings.stages";
import "./settings.pipelines.css";

type Pipeline = components["schemas"]["Pipeline"];
type T = ReturnType<typeof useT>;

// The address dial naming the open pipeline.
const PIPELINE_PARAM = "pipeline";

function createFields(t: T): CreateField[] {
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
  ];
}

const RENAME_FIELDS: CreateField[] = [
  { key: "name", label: "pipeline.name", required: true },
];

// The pipeline the address names, or the one a reader would expect without it:
// the default, then the first in use, then whatever there is.
function openPipeline(
  pipelines: readonly Pipeline[],
  asked: string | undefined,
): Pipeline | undefined {
  return (
    pipelines.find((each) => each.id === asked) ??
    pipelines.find((each) => each.is_default && !each.archived_at) ??
    pipelines.find((each) => !each.archived_at) ??
    pipelines[0]
  );
}

// One pipeline in the catalog: its name and standing, how long its ladder is,
// and the ladder's shape, so two pipelines are told apart before either opens.
function PipelineChoice({
  pipeline,
  open,
  onOpen,
}: Readonly<{ pipeline: Pipeline; open: boolean; onOpen: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const ladder = ladderOf(pipeline);
  const openCount = ladder.filter((stage) => stage.semantic === "open").length;
  return (
    <button
      type="button"
      className="pipeline-choice"
      aria-current={open ? "true" : undefined}
      onClick={onOpen}
    >
      <span className="pipeline-choice-head">
        <span className="pipeline-choice-name">{pipeline.name}</span>
        {pipeline.is_default && !pipeline.archived_at && (
          <Badge tone="success">{t("pipeline.default")}</Badge>
        )}
        <span className="t-caption">
          {plural("pipeline.openStageCount", openCount, {
            count: formatNumber(openCount, locale),
          })}
        </span>
      </span>
      <StageStrip
        compact
        label={pipeline.name}
        steps={ladder.map((stage) => ({
          key: stage.id,
          name: stage.name,
          probability: stage.win_probability,
          reading: "",
          outcome: stage.semantic === "open" ? undefined : stage.semantic,
        }))}
      />
    </button>
  );
}

// The pipelines themselves: those in use, in the order they are offered, and
// the retired ones apart, since they hold no place in that order.
function CatalogList({
  pipelines,
  openId,
  onOpen,
  hintId,
}: Readonly<{
  pipelines: readonly Pipeline[];
  openId: string | undefined;
  onOpen: (id: string) => void;
  hintId: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const canEdit = useCanWrite("pipeline", "update");
  const order = usePipelineOrder();
  const inUse = pipelines.filter((each) => !each.archived_at);
  const retired = pipelines.filter((each) => each.archived_at);
  const place = (position: number, count: number) => ({
    position: formatNumber(position, locale),
    total: formatNumber(count, locale),
  });
  return (
    <>
      {canEdit && inUse.length > 1 && (
        <p id={hintId} className="t-caption stage-hint">
          {t("pipeline.orderHint")}
        </p>
      )}
      <SortableList
        label={t("pipeline.inUse")}
        items={inUse.map((pipeline) => ({ key: pipeline.id, pipeline }))}
        selectedKey={openId}
        hintId={canEdit ? hintId : undefined}
        busy={order.pending}
        onReorder={
          canEdit ? (keys) => order.reorder(pipelines, keys) : undefined
        }
        labels={{
          handle: (item, position, count) =>
            t("pipeline.handle", {
              name: item.pipeline.name,
              ...place(position, count),
            }),
          moved: (item, position, count) =>
            t("pipeline.moved", {
              name: item.pipeline.name,
              ...place(position, count),
            }),
        }}
        renderItem={(item) => (
          <PipelineChoice
            pipeline={item.pipeline}
            open={item.key === openId}
            onOpen={() => onOpen(item.key)}
          />
        )}
      />
      {retired.length > 0 && (
        <Disclosure
          summary={plural("pipeline.retiredGroup", retired.length, {
            count: formatNumber(retired.length, locale),
          })}
          open={retired.some((each) => each.id === openId) || undefined}
        >
          <ul className="pipeline-retired">
            {retired.map((pipeline) => (
              <li key={pipeline.id}>
                <PipelineChoice
                  pipeline={pipeline}
                  open={pipeline.id === openId}
                  onOpen={() => onOpen(pipeline.id)}
                />
              </li>
            ))}
          </ul>
        </Disclosure>
      )}
    </>
  );
}

// The catalog panel draws its title and its verbs whatever state the list is
// in: the page is named by it, and a loading or empty list is still this page.
function PipelineCatalog({
  query,
  openId,
  onOpen,
}: Readonly<{
  query: UseQueryResult<Pipeline[]>;
  openId: string | undefined;
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const hintId = useId();
  const canCreate = useCanWrite("pipeline", "create");
  const canEdit = useCanWrite("pipeline", "update");
  const inUse = (query.data ?? []).filter((each) => !each.archived_at);
  const lastPosition = Math.max(0, ...inUse.map((each) => each.position));
  return (
    <Panel
      title={t("settings.pipelines")}
      titleAction={
        canCreate && (
          <CreateAction<Pipeline>
            label={t("pipeline.new")}
            invalidate="pipelines"
            screen="settings"
            stay
            onCreated={(created) => onOpen(created.id)}
            // A new pipeline lands at the end of the catalog and already holds
            // the two ways out, so its ladder is open stages plus the close.
            create={async (values) => {
              return unwrap(
                await api.POST("/pipelines", {
                  body: {
                    name: str(values.name),
                    is_default: values.is_default === "true",
                    position: lastPosition + 1,
                    stages: [
                      {
                        name: t("stage.semWon"),
                        semantic: "won",
                        position: 1,
                        win_probability: 100,
                      },
                      {
                        name: t("stage.semLost"),
                        semantic: "lost",
                        position: 2,
                        win_probability: 0,
                      },
                    ],
                  },
                }),
              );
            }}
            fields={createFields(t)}
          />
        )
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.pipelinesSub")}</PanelIntro>
        {!canCreate && !canEdit && (
          <PanelIntro>{t("settings.pipelinesReadOnly")}</PanelIntro>
        )}
        <QueryGate
          pendingLabel={t("settings.pipelines")}
          query={query}
          empty={(pipelines) => pipelines.length === 0}
        >
          {(pipelines) => (
            <CatalogList
              pipelines={pipelines}
              openId={openId}
              onOpen={onOpen}
              hintId={hintId}
            />
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// The open pipeline: its standing and verbs in the head, its ladder, and the
// way to retire it at the foot, where a destructive verb waits last.
function PipelineDetail({ pipeline }: Readonly<{ pipeline: Pipeline }>) {
  const t = useT();
  // Renaming, reordering stages and making default are pipeline:update;
  // retiring is pipeline:delete; putting one back is update, because
  // restoring is not a deletion.
  const canEdit = useCanWrite("pipeline", "update");
  const canRetire = useCanWrite("pipeline", "delete");
  const retired = Boolean(pipeline.archived_at);
  const retireBlockedId = useId();
  return (
    <Panel
      title={pipeline.name}
      titleAction={
        <span className="pipeline-standing">
          {retired && <Badge>{t("pipeline.retired")}</Badge>}
          {pipeline.is_default && !retired && (
            <Badge tone="success">{t("pipeline.default")}</Badge>
          )}
          {canEdit && !retired && !pipeline.is_default && (
            <MakeDefault pipeline={pipeline} />
          )}
          {canEdit && !retired && (
            <EditAction<Pipeline>
              label={t("pipeline.rename")}
              savedMessage={(saved) =>
                t("record.saveDone", { name: saved.name })
              }
              invalidate="pipelines"
              recordKey="pipeline"
              record={{ id: pipeline.id, name: pipeline.name }}
              fields={RENAME_FIELDS}
              update={async (values) => {
                return unwrap(
                  await api.PATCH("/pipelines/{id}", {
                    params: { path: { id: pipeline.id } },
                    body: { name: str(values.name) },
                  }),
                );
              }}
            />
          )}
        </span>
      }
    >
      <PanelBody className="pipeline-detail">
        {retired && <PanelIntro>{t("pipeline.retiredNote")}</PanelIntro>}
        <StageLadderEditor pipeline={pipeline} canEdit={canEdit} />
      </PanelBody>
      {(retired ? canEdit : canRetire) && (
        <SettingList bleed="settings">
          <SettingRow
            label={retired ? t("pipeline.restore") : t("pipeline.retire")}
            description={
              retired ? undefined : (
                <>
                  {t("pipeline.retireNote")}
                  {/* Why the verb is refused sits in the description, which
                      wraps; the control column is sized to its content. */}
                  {pipeline.is_default && (
                    <>
                      {" "}
                      <span id={retireBlockedId}>
                        {t("pipeline.retireBlocked")}
                      </span>
                    </>
                  )}
                </>
              )
            }
            control={
              <span className="pipeline-standing">
                <PipelineRetirement
                  pipeline={pipeline}
                  canRetire={canRetire}
                  canRestore={canEdit}
                  blockedReasonId={retireBlockedId}
                />
              </span>
            }
          />
        </SettingList>
      )}
    </Panel>
  );
}

// Settings → Pipelines. The list is readable by everyone and only the write
// affordances are gated, with the server the RBAC authority. Three of the five
// seeded roles hold pipeline READ and no write verb, so for most readers this
// is the read-only case rather than an edge of it.
export function PipelinesCard() {
  const [params, setParams] = useUrlParams();
  const query = useQuery({
    queryKey: SETTINGS_PIPELINES,
    queryFn: async () => {
      const data = unwrap(
        await api.GET("/pipelines", {
          params: { query: { include_archived: true } },
        }),
      );
      return data.data;
    },
  });
  const open = (id: string) => {
    const next = new Map(params);
    next.set(PIPELINE_PARAM, id);
    setParams(next);
  };
  const opened = openPipeline(query.data ?? [], params.get(PIPELINE_PARAM));
  return (
    <div className="settings-pipelines">
      <PipelineCatalog query={query} openId={opened?.id} onOpen={open} />
      {opened && <PipelineDetail pipeline={opened} />}
    </div>
  );
}
