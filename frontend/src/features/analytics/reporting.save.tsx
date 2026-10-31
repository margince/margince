import "./reporting.css";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import {
  Button,
  Checkbox,
  Disclosure,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { throwProblem, unwrap } from "../../lib/problem";
import { CardPreview } from "./reporting.cardpreview";
import { ReportingFilters } from "./reporting.filters";
import {
  blocksOf,
  cardChoices,
  keptCards,
  layoutOf,
  type ReportingLayout,
} from "./reporting.layout";
import { LayoutEditor } from "./reporting.layouteditor";
import type { ReportingReport, ReportingSelection } from "./reporting.model";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";
import { reportRevisionsKey } from "./reporting.versions";

export function SaveReportingDialog({
  selection: initialSelection,
  report,
  onClose,
  onSaved,
}: Readonly<{
  selection: ReportingSelection;
  report?: ReportingReport;
  onClose: () => void;
  onSaved: (report: ReportingReport) => void;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const [selection, setSelection] = useState(initialSelection);
  const title = useId();
  const formId = useId();
  const queryClient = useQueryClient();
  const [customizing, setCustomizing] = useState(!!report);
  const [name, setName] = useState(report?.name ?? "");
  const [description, setDescription] = useState(report?.description ?? "");
  const [audience, setAudience] = useState<"private" | "team" | "workspace">(
    report?.audience ?? "private",
  );
  const [metrics, setMetrics] = useState(selection.metrics);
  const [layout, setLayout] = useState(layoutOf(selection));
  const catalog = useReportingCatalog();
  // Each selected metric's charts, from the catalog once it has loaded and from
  // the report's own blocks until then.
  const metricBlocks = new Map(
    (catalog.data?.metrics ?? [])
      .filter((metric) => metrics.includes(metric.id))
      .map((metric) => [metric.id, metric.blocks] as const),
  );
  const drawable = (card: ReportingLayout[number]["cards"][number]) => {
    const kind = card.kind;
    if (kind === "question" || catalog.data === undefined) return true;
    return card.metric
      ? (metricBlocks.get(card.metric)?.includes(kind) ?? false)
      : [...metricBlocks.values()].some((blocks) => blocks.includes(kind));
  };
  const kept = keptCards(layout, drawable);
  const write = useMutation({
    mutationFn: async ({
      body,
      existing,
      asDraft,
    }: {
      body: components["schemas"]["ReportingReportInput"];
      existing?: { id: string; version: number };
      asDraft?: boolean;
    }) => {
      if (existing && asDraft) {
        return unwrap(
          await api.PUT("/analytics/reports/{id}/draft", {
            params: { path: { id: existing.id }, ...ifMatch(existing.version) },
            body: { selection: body.selection },
          }),
        );
      }
      const response = existing
        ? await api.PATCH("/analytics/reports/{id}", {
            params: {
              path: { id: existing.id },
              ...ifMatch(existing.version),
            },
            body,
          })
        : await api.POST("/analytics/reports", { body });
      if (response.error) throwProblem(response.error);
      return response.data;
    },
    onSuccess: async (report) => {
      await queryClient.invalidateQueries({ queryKey: ["reporting-reports"] });
      await queryClient.invalidateQueries({
        queryKey: ["reporting-report", report.id],
      });
      await queryClient.invalidateQueries({
        queryKey: ["reporting-live", report.id],
      });
      await queryClient.invalidateQueries({
        queryKey: reportRevisionsKey(report.id),
      });
      onSaved(report);
    },
  });
  // An open draft takes every edit until it is published or discarded, so a
  // direct save is not offered beside it.
  const draftOpen = report?.draft_revision !== undefined;
  const save = (asDraft: boolean) =>
    write.mutate({
      existing: report ? { id: report.id, version: report.version } : undefined,
      asDraft,
      body: {
        name,
        description,
        audience,
        audience_team_id: audience === "team" ? selection.scope.id : undefined,
        selection: {
          ...selection,
          metrics,
          layout: kept,
          blocks: blocksOf(kept),
        },
      },
    });
  const metadataChanged = renamedOrReshared(report, {
    name,
    description,
    audience,
  });
  const incomplete =
    write.isPending || !name.trim() || unsaveable(selection, metrics);
  return (
    <Modal open onClose={onClose} labelledBy={title} intent="form">
      <Heading size="large" id={title} className="t-h2 modal-title">
        {t(report ? "reporting.edit" : "reporting.save")}
      </Heading>
      {draftOpen && <p className="t-caption">{t("reporting.draftEditNote")}</p>}
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          save(draftOpen);
        }}
      >
        <Field label={t("reporting.name")} required>
          {(field) => (
            <TextInput
              {...field}
              value={name}
              disabled={draftOpen}
              onChange={(event) => setName(event.target.value)}
              maxLength={160}
            />
          )}
        </Field>
        <Field label={t("reporting.description")}>
          {(field) => (
            <Textarea
              {...field}
              value={description}
              disabled={draftOpen}
              onChange={(event) => setDescription(event.target.value)}
              rows={3}
            />
          )}
        </Field>
        <p className="t-caption">
          {selection.scope.label} · {t(`reporting.${selection.period}`)}
        </p>
        <Field label={t("reporting.audience")}>
          {(field) => (
            <Select
              {...field}
              value={audience}
              disabled={draftOpen}
              options={[
                { value: "private", label: t("reporting.private") },
                ...(selection.scope.kind === "team"
                  ? [{ value: "team", label: t("reporting.team") }]
                  : []),
                ...(selection.scope.kind === "workspace"
                  ? [{ value: "workspace", label: t("reporting.workspace") }]
                  : []),
              ]}
              onChange={(value) => {
                if (
                  value === "private" ||
                  value === "team" ||
                  value === "workspace"
                )
                  setAudience(value);
              }}
            />
          )}
        </Field>
        <Button
          variant="link"
          aria-expanded={customizing}
          onClick={() => setCustomizing(!customizing)}
        >
          {t("reporting.customize")}
        </Button>
        {customizing && (
          <div className="form-stack">
            <ReportingFilters
              selection={selection}
              onChange={(next) => {
                setSelection(next);
                if (
                  next.scope.kind !== selection.scope.kind ||
                  next.scope.id !== selection.scope.id
                )
                  setAudience("private");
              }}
            />
            <fieldset className="form-stack">
              <legend>{t("reporting.metrics")}</legend>
              {(
                catalog.data?.metrics.map((metric) => metric.id) ??
                selection.metrics
              ).map((metric) => (
                <Checkbox
                  key={metric}
                  label={metricName(metric)}
                  // Which charts a metric brings is in the catalog, so the
                  // choice waits for it rather than keep cards it cannot judge.
                  disabled={catalog.data === undefined}
                  checked={metrics.includes(metric)}
                  onChange={(event) =>
                    setMetrics(
                      event.target.checked
                        ? [...metrics, metric]
                        : metrics.filter((candidate) => candidate !== metric),
                    )
                  }
                />
              ))}
            </fieldset>
            <Disclosure summary={t("reporting.blocks")}>
              <LayoutEditor
                layout={kept}
                choices={cardChoices(kept, metricBlocks)}
                onChange={setLayout}
                preview={(choice) => (
                  <CardPreview
                    selection={{ ...selection, metrics }}
                    choice={choice}
                  />
                )}
              />
            </Disclosure>
          </div>
        )}
        {metadataChanged && !draftOpen && (
          <p className="t-caption">{t("reporting.draftKeepsName")}</p>
        )}
        <ErrorLine error={write.error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        {report && !draftOpen && (
          <Button
            variant="ghost"
            disabled={incomplete || metadataChanged}
            onClick={() => save(true)}
          >
            {t("reporting.saveDraft")}
          </Button>
        )}
        <Button type="submit" form={formId} disabled={incomplete}>
          {t(draftOpen ? "reporting.saveDraft" : "reporting.save")}
        </Button>
      </div>
    </Modal>
  );
}

// A draft holds charts alone, so a renamed, redescribed or re-shared report is
// saved directly rather than lost in a draft.
function renamedOrReshared(
  report: ReportingReport | undefined,
  typed: Readonly<{ name: string; description: string; audience: string }>,
): boolean {
  return (
    report !== undefined &&
    (typed.name.trim() !== report.name ||
      typed.description.trim() !== (report.description ?? "") ||
      typed.audience !== report.audience)
  );
}

function unsaveable(
  selection: ReportingSelection,
  metrics: readonly unknown[],
): boolean {
  if (metrics.length === 0) return true;
  if (selection.period !== "custom") return false;
  return (
    !selection.interval ||
    selection.interval.start_at >= selection.interval.end_at
  );
}
