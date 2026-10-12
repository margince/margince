import "./reporting.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import {
  Button,
  Checkbox,
  Disclosure,
  Field,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { throwProblem, unwrap } from "./common";
import { ReportingFilters } from "./reporting.filters";
import {
  blockLabel,
  metricLabel,
  type ReportingReport,
  type ReportingSelection,
} from "./reporting.model";

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
  const [selection, setSelection] = useState(initialSelection);
  const title = useId();
  const formId = useId();
  const queryClient = useQueryClient();
  const [customizing, setCustomizing] = useState(!!report);
  const [name, setName] = useState(report?.name ?? "");
  const [audience, setAudience] = useState<"private" | "team" | "workspace">(
    report?.audience ?? "private",
  );
  const [metrics, setMetrics] = useState(selection.metrics);
  const [blocks, setBlocks] = useState(selection.blocks);
  const catalog = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/metrics"));
    },
  });
  const allowedBlocks =
    catalog.data?.metrics
      .filter((metric) => metrics.includes(metric.id))
      .flatMap((metric) => metric.blocks) ?? selection.blocks;
  const selectedBlocks = blocks.filter((block) =>
    allowedBlocks.includes(block),
  );
  const moveBlock = (
    block: ReportingSelection["blocks"][number],
    direction: number,
  ) => {
    const ordered = [...selectedBlocks];
    const index = ordered.indexOf(block);
    const next = index + direction;
    if (index < 0 || next < 0 || next >= ordered.length) return;
    ordered.splice(index, 1);
    ordered.splice(next, 0, block);
    setBlocks(ordered);
  };
  const write = useMutation({
    mutationFn: async ({
      body,
      existing,
    }: {
      body: components["schemas"]["ReportingReportInput"];
      existing?: { id: string; version: number };
    }) => {
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
      onSaved(report);
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={title} intent="form">
      <Heading size="large" id={title} className="t-h2 modal-title">
        {t(report ? "reporting.edit" : "reporting.save")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          write.mutate({
            existing: report
              ? { id: report.id, version: report.version }
              : undefined,
            body: {
              name,
              audience,
              audience_team_id:
                audience === "team" ? selection.scope.id : undefined,
              selection: { ...selection, metrics, blocks: selectedBlocks },
            },
          });
        }}
      >
        <Field label={t("reporting.name")} required>
          {(field) => (
            <TextInput
              {...field}
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={160}
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
                  label={metricLabel(metric, t)}
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
              <fieldset className="form-stack">
                <legend>{t("reporting.blocks")}</legend>
                {[...new Set([...selectedBlocks, ...allowedBlocks])].map(
                  (block) => (
                    <div key={block} className="reporting-block-choice">
                      <Checkbox
                        key={block}
                        label={blockLabel(block, t)}
                        checked={selectedBlocks.includes(block)}
                        onChange={(event) =>
                          setBlocks(
                            event.target.checked
                              ? [...selectedBlocks, block]
                              : blocks.filter(
                                  (candidate) => candidate !== block,
                                ),
                          )
                        }
                      />
                      {selectedBlocks.includes(block) && (
                        <span className="reporting-block-order">
                          <Button
                            variant="ghost"
                            disabled={selectedBlocks.indexOf(block) === 0}
                            onClick={() => moveBlock(block, -1)}
                            aria-label={`${t("reporting.moveUp")}: ${blockLabel(block, t)}`}
                          >
                            <ArrowUp aria-hidden="true" />
                          </Button>
                          <Button
                            variant="ghost"
                            disabled={
                              selectedBlocks.indexOf(block) ===
                              selectedBlocks.length - 1
                            }
                            onClick={() => moveBlock(block, 1)}
                            aria-label={`${t("reporting.moveDown")}: ${blockLabel(block, t)}`}
                          >
                            <ArrowDown aria-hidden="true" />
                          </Button>
                        </span>
                      )}
                    </div>
                  ),
                )}
              </fieldset>
            </Disclosure>
          </div>
        )}
        <ErrorLine error={write.error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={
            write.isPending ||
            !name.trim() ||
            metrics.length === 0 ||
            (selection.period === "custom" &&
              (!selection.interval ||
                selection.interval.start_at >= selection.interval.end_at))
          }
        >
          {t("reporting.save")}
        </Button>
      </div>
    </Modal>
  );
}
