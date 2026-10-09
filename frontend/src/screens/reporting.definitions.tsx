import "./reporting.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { useCanWrite } from "../app/capability";
import {
  Button,
  Checkbox,
  Disclosure,
  Field,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useAnalyticsContext } from "./analytics.context";
import { AnalyticsScopePicker } from "./analytics.scope";
import { QueryGate, unwrap } from "./common";
import { metricLabel } from "./reporting.model";

type Framework = components["schemas"]["ReportingFramework"];
export function ReportingDefinitions() {
  const { locale } = useLocale();
  const t = useT();
  const [section, setSection] = useState("definitions");
  const canPublish = useCanWrite("reporting_framework", "update");
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/metrics"));
    },
  });
  const framework = useQuery({
    enabled: canPublish && section === "setup",
    queryKey: ["reporting-framework"],
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/framework"));
    },
  });
  return (
    <>
      {canPublish && (
        <SegmentedControl
          label={t("reporting.settings")}
          options={["definitions", "setup"]}
          value={section}
          onChange={setSection}
          labels={{
            definitions: t("reporting.definitions"),
            setup: t("reporting.framework"),
          }}
        />
      )}
      {section === "definitions" && (
        <Panel title={t("reporting.definitions")}>
          <PanelBody>
            <QueryGate
              query={catalog}
              pendingLabel={t("reporting.definitions")}
            >
              {(catalog) => (
                <>
                  {catalog.metrics.map((metric) => (
                    <Disclosure
                      key={metric.id}
                      summary={metricLabel(metric.id, t)}
                    >
                      <p>{metric.definition}</p>
                      <p className="t-caption">{metric.attribution}</p>
                      <p className="t-caption">{metric.incomplete_policy}</p>
                      <p className="t-caption">
                        {t("reporting.revision", {
                          revision: String(metric.version),
                        })}{" "}
                        ·{" "}
                        {metric.unit === "count"
                          ? t("reporting.countUnit")
                          : metric.unit === "days"
                            ? t("reporting.daysUnit")
                            : metric.unit === "money"
                              ? t("reporting.amountUnit")
                              : "%"}{" "}
                        ·{" "}
                        {t(
                          metric.temporal_basis === "event_period"
                            ? "reporting.period"
                            : "reporting.currentState",
                        )}
                      </p>
                    </Disclosure>
                  ))}
                </>
              )}
            </QueryGate>
          </PanelBody>
        </Panel>
      )}
      {section === "setup" && canPublish && (
        <Panel title={t("reporting.framework")}>
          <PanelBody>
            <QueryGate
              query={framework}
              pendingLabel={t("reporting.framework")}
            >
              {(framework) => (
                <>
                  <p>
                    {t("reporting.revision", {
                      revision: formatNumber(framework.revision, locale),
                    })}{" "}
                    · {t(`reporting.${framework.definition.template}`)}
                  </p>
                  <PanelIntro>{t("reporting.prospective")}</PanelIntro>
                  <FrameworkEditor
                    key={framework.version}
                    framework={framework}
                  />
                </>
              )}
            </QueryGate>
          </PanelBody>
        </Panel>
      )}
    </>
  );
}

function FrameworkEditor({ framework }: Readonly<{ framework: Framework }>) {
  const t = useT();
  const client = useQueryClient();
  const scopes = useAnalyticsContext();
  const [definition, setDefinition] = useState(framework.definition);
  const [reason, setReason] = useState("");
  const pipelines = useQuery({
    queryKey: ["reporting-framework-pipelines"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/pipelines", {
          params: { query: {} },
        }),
      );
    },
  });
  const write = useMutation({
    mutationFn: async ({
      version,
      definition,
    }: {
      version: number;
      definition: Framework["definition"];
    }) => {
      return unwrap(
        await api.PUT("/analytics/framework", {
          params: { ...ifMatch(version) },
          body: definition,
        }),
      );
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["reporting-framework"] });
      await client.invalidateQueries({
        queryKey: ["reporting-overview-setup"],
      });
    },
  });
  const toggleStage = (
    pipelineId: string,
    stageId: string,
    checked: boolean,
  ) => {
    const current =
      definition.qualification.find((entry) => entry.pipeline_id === pipelineId)
        ?.stage_ids ?? [];
    const stages = checked
      ? [...new Set([...current, stageId])]
      : current.filter((id) => id !== stageId);
    setDefinition({
      ...definition,
      qualification: [
        ...definition.qualification.filter(
          (entry) => entry.pipeline_id !== pipelineId,
        ),
        ...(stages.length
          ? [{ pipeline_id: pipelineId, stage_ids: stages }]
          : []),
      ],
    });
  };
  return (
    <form
      className="reporting-dialog"
      onSubmit={(event) => {
        event.preventDefault();
        write.mutate({
          version: framework.version,
          definition: { ...definition, reason },
        });
      }}
    >
      <Field label={t("reporting.template")}>
        {(field) => (
          <Select
            {...field}
            value={definition.template}
            options={[
              { value: "sales", label: t("reporting.sales") },
              { value: "sdr", label: t("reporting.sdr") },
            ]}
            onChange={(value) => {
              if (value === "sales" || value === "sdr")
                setDefinition({ ...definition, template: value });
            }}
          />
        )}
      </Field>
      <QueryGate query={pipelines} pendingLabel={t("reporting.pipeline")}>
        {(result) => (
          <>
            {result.data.map((pipeline) => (
              <fieldset key={pipeline.id}>
                <legend>
                  {pipeline.name} · {t("reporting.qualification")}
                </legend>
                {(pipeline.stages ?? []).map((stage) => (
                  <Checkbox
                    key={stage.id}
                    label={stage.name}
                    checked={definition.qualification.some(
                      (entry) =>
                        entry.pipeline_id === pipeline.id &&
                        entry.stage_ids.includes(stage.id),
                    )}
                    onChange={(event) =>
                      toggleStage(pipeline.id, stage.id, event.target.checked)
                    }
                  />
                ))}
              </fieldset>
            ))}
          </>
        )}
      </QueryGate>
      <fieldset>
        <legend>{t("reporting.captureContexts")}</legend>
        <p>{t("reporting.captureHelp")}</p>
        {definition.capture_contexts.map((capture, index) => (
          <div
            key={`${capture.scope.kind}:${capture.scope.id}:${capture.pipeline_id}`}
            className="reporting-toolbar"
          >
            <AnalyticsScopePicker
              scopes={(scopes.data?.allowed_scopes ?? []).filter(
                (scope) => scope.kind === "team" || scope.kind === "workspace",
              )}
              selected={{
                ...capture.scope,
                label: capture.scope.label ?? t("history.field.scope"),
              }}
              onSelect={(scope) =>
                setDefinition({
                  ...definition,
                  capture_contexts: definition.capture_contexts.map(
                    (entry, i) => (i === index ? { ...entry, scope } : entry),
                  ),
                })
              }
            />
            <Field label={t("reporting.pipeline")}>
              {(field) => (
                <Select
                  {...field}
                  value={capture.pipeline_id ?? ""}
                  options={[
                    { value: "", label: t("reporting.allPipelines") },
                    ...(pipelines.data?.data ?? []).map((pipeline) => ({
                      value: pipeline.id,
                      label: pipeline.name,
                    })),
                  ]}
                  onChange={(value) =>
                    setDefinition({
                      ...definition,
                      capture_contexts: definition.capture_contexts.map(
                        (entry, i) =>
                          i === index
                            ? { ...entry, pipeline_id: value || undefined }
                            : entry,
                      ),
                    })
                  }
                />
              )}
            </Field>
            <Button
              variant="ghost"
              onClick={() =>
                setDefinition({
                  ...definition,
                  capture_contexts: definition.capture_contexts.filter(
                    (_, i) => i !== index,
                  ),
                })
              }
            >
              {t("reporting.removeCapture")}
            </Button>
          </div>
        ))}
        <Button
          variant="ghost"
          disabled={
            definition.capture_contexts.length >= 20 ||
            !scopes.data?.allowed_scopes.some(
              (scope) => scope.kind === "workspace" || scope.kind === "team",
            )
          }
          onClick={() => {
            const scope = scopes.data?.allowed_scopes.find(
              (scope) => scope.kind === "team" || scope.kind === "workspace",
            );
            if (scope)
              setDefinition({
                ...definition,
                capture_contexts: [...definition.capture_contexts, { scope }],
              });
          }}
        >
          {t("reporting.addCapture")}
        </Button>
      </fieldset>
      <Field label={t("reporting.reason")} required>
        {(field) => (
          <TextInput
            {...field}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
          />
        )}
      </Field>
      <ErrorLine error={write.error} />
      <Button type="submit" disabled={write.isPending || !reason.trim()}>
        {t("reporting.publish")}
      </Button>
    </form>
  );
}
