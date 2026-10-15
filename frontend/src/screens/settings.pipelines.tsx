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
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { CreateAction, type CreateField } from "../components/create";
import { EditAction } from "../components/edit";
import { useArrivalFocus } from "../design-system/arrivalfocus";
import { Badge, Disclosure, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryStates } from "../design-system/querystates";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { SortableList } from "../design-system/sortablelist";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { unwrap } from "../lib/problem";
import { useUrlParams } from "../lib/urlstate";
import { LegalReviewSettingsCard } from "./legalreviewsettings";
import { SETTINGS_PIPELINES, usePipelineOrder } from "./settings.pipelineorder";
import { MakeDefault, PipelineRetirement } from "./settings.pipelineverbs";
import { StageLadderEditor } from "./settings.stages";
import { ladderOf, str } from "./settings.stageverbs";
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

// One pipeline in the catalog: its name, its standing and how many open stages
// it has, so two pipelines are told apart before either opens.
function PipelineChoice({
  pipeline,
  open,
  onOpen,
}: Readonly<{ pipeline: Pipeline; open: boolean; onOpen: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const openCount = ladderOf(pipeline).filter(
    (stage) => stage.semantic === "open",
  ).length;
  return (
    <button
      type="button"
      className="pipeline-choice"
      aria-current={open ? "true" : undefined}
      onClick={onOpen}
    >
      <span className="pipeline-choice-name">{pipeline.name}</span>
      {pipeline.is_default && !pipeline.archived_at && (
        <Badge tone="success">{t("pipeline.default")}</Badge>
      )}
      <span className="t-caption">
        {plural("pipeline.openStageCount", openCount, {
          count: formatNumber(openCount, locale),
        })}
      </span>
    </button>
  );
}

// The pipelines in use, in the order they are offered.
function CatalogList({
  pipelines,
  openId,
  onOpen,
}: Readonly<{
  pipelines: readonly Pipeline[];
  openId: string | undefined;
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const hintId = useId();
  const canEdit = useCanWrite("pipeline", "update");
  const order = usePipelineOrder();
  const inUse = pipelines.filter((each) => !each.archived_at);
  const orders = canEdit && inUse.length > 1;
  const place = (position: number, count: number) => ({
    position: formatNumber(position, locale),
    total: formatNumber(count, locale),
  });
  return (
    <>
      <SortableList
        label={t("pipeline.inUse")}
        items={inUse.map((pipeline) => ({ key: pipeline.id, pipeline }))}
        selectedKey={openId}
        hintId={orders ? hintId : undefined}
        busy={order.pending}
        onReorder={
          orders ? (keys) => order.reorder(pipelines, keys) : undefined
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
      {orders && (
        <span id={hintId} className="sr-only">
          {t("pipeline.orderHint")}
        </span>
      )}
    </>
  );
}

// The retired pipelines, apart and folded: they hold no place in the order.
function RetiredPipelines({
  retired,
  openId,
  onOpen,
}: Readonly<{
  retired: readonly Pipeline[];
  openId: string | undefined;
  onOpen: (id: string) => void;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Disclosure
      summary={plural("pipeline.retiredGroup", retired.length, {
        count: formatNumber(retired.length, locale),
      })}
      open={retired.some((each) => each.id === openId) || undefined}
    >
      <PanelBody>
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
      </PanelBody>
    </Disclosure>
  );
}

// "New pipeline": in the catalog's head, or in the one pipeline's head while
// there is no catalog to hold it.
function NewPipeline({
  pipelines,
  onOpen,
}: Readonly<{
  pipelines: readonly Pipeline[];
  onOpen: (id: string) => void;
}>) {
  const t = useT();
  const canCreate = useCanWrite("pipeline", "create");
  if (!canCreate) return null;
  const inUse = pipelines.filter((each) => !each.archived_at);
  const lastPosition = Math.max(0, ...inUse.map((each) => each.position));
  return (
    <CreateAction<Pipeline>
      label={t("pipeline.new")}
      invalidate="pipelines"
      screen="settings"
      stay
      onCreated={(created) => onOpen(created.id)}
      // A new pipeline lands last and already holds the two ways out. Its
      // ladder is open stages plus the close.
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
  );
}

// The catalog draws its title and verb in every state. A loading or empty list
// is still where a pipeline is chosen and created.
function PipelineCatalog({
  query,
  openId,
  onOpen,
  onCreated,
}: Readonly<{
  query: UseQueryResult<Pipeline[]>;
  openId: string | undefined;
  onOpen: (id: string) => void;
  onCreated: (id: string) => void;
}>) {
  const t = useT();
  const pipelines = query.data ?? [];
  const inUse = pipelines.filter((each) => !each.archived_at);
  const retired = pipelines.filter((each) => each.archived_at);
  return (
    <Panel
      title={t("pipeline.catalog")}
      titleAction={<NewPipeline pipelines={pipelines} onOpen={onCreated} />}
    >
      {(!query.isSuccess || inUse.length > 0 || retired.length === 0) && (
        <PanelBody>
          <QueryStates query={query} pendingLabel={t("pipeline.catalog")}>
            {pipelines.length === 0 ? (
              <EmptyState plate title={t("pipeline.none")}>
                {t("pipeline.noneNote")}
              </EmptyState>
            ) : (
              <CatalogList
                pipelines={pipelines}
                openId={openId}
                onOpen={onOpen}
              />
            )}
          </QueryStates>
        </PanelBody>
      )}
      {retired.length > 0 && (
        <RetiredPipelines retired={retired} openId={openId} onOpen={onOpen} />
      )}
    </Panel>
  );
}

// The open pipeline: its standing and verbs in the head, its ladder, and the
// way to retire it at the foot, where a destructive verb waits last.
function PipelineDetail({
  pipeline,
  create,
  arrived,
}: Readonly<{ pipeline: Pipeline; create: ReactNode; arrived: boolean }>) {
  const t = useT();
  // The second pipeline brings the catalog and takes "New pipeline" off this
  // head, so focus would fall to the page.
  const title = useArrivalFocus<HTMLSpanElement>(arrived);
  // Renaming, reordering stages and making default are pipeline:update;
  // retiring is pipeline:delete; putting one back is update, because
  // restoring is not a deletion.
  const canEdit = useCanWrite("pipeline", "update");
  const canRetire = useCanWrite("pipeline", "delete");
  const retired = Boolean(pipeline.archived_at);
  const retireBlockedId = useId();
  return (
    <Panel
      title={
        <span ref={title} tabIndex={-1}>
          {pipeline.name}
        </span>
      }
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
          {create}
        </span>
      }
    >
      {retired && (
        <PanelBody>
          <PanelIntro>{t("pipeline.retiredNote")}</PanelIntro>
        </PanelBody>
      )}
      <StageLadderEditor pipeline={pipeline} canEdit={canEdit} />
      {(retired ? canEdit : canRetire) && (
        <SettingList bleed="settings">
          <SettingRow
            label={retired ? t("pipeline.restore") : t("pipeline.retire")}
            description={
              retired ? undefined : (
                <span id={retireBlockedId}>
                  {pipeline.is_default
                    ? t("pipeline.retireBlocked")
                    : t("pipeline.retireNote")}
                </span>
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
  const [created, setCreated] = useState<string>();
  const open = (id: string) => {
    const next = new Map(params);
    next.set(PIPELINE_PARAM, id);
    setParams(next);
  };
  const openCreated = (id: string) => {
    open(id);
    setCreated(id);
  };
  const pipelines = query.data ?? [];
  const opened = openPipeline(pipelines, params.get(PIPELINE_PARAM));
  // One pipeline leaves nothing to choose between, so it stands alone and
  // carries the catalog's one verb in its own head.
  const alone = query.isSuccess && pipelines.length === 1;
  return (
    <div className="settings-pipelines">
      {!alone && (
        <PipelineCatalog
          query={query}
          openId={opened?.id}
          onOpen={open}
          onCreated={openCreated}
        />
      )}
      {opened && (
        <PipelineDetail
          pipeline={opened}
          arrived={opened.id === created}
          create={
            alone && <NewPipeline pipelines={pipelines} onOpen={openCreated} />
          }
        />
      )}
      {/* Whether a stage may wait for legal is a pipeline question. */}
      <LegalReviewSettingsCard />
    </div>
  );
}
