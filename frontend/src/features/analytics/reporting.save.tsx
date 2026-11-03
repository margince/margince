import "./reporting.css";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { Button, Field, Textarea, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { throwProblem, unwrap } from "../../lib/problem";
import { ReportComposition, useComposition } from "./reporting.composition";
import { blocksOf } from "./reporting.layout";
import type { ReportingReport, ReportingSelection } from "./reporting.model";
import { reportRevisionsKey } from "./reporting.versions";

export function SaveReportingDialog({
  selection: initialSelection,
  report,
  note,
  onClose,
  onSaved,
}: Readonly<{
  selection: ReportingSelection;
  report?: ReportingReport;
  // What saving does not change, said where the reader decides to save.
  note?: string;
  onClose: () => void;
  onSaved: (report: ReportingReport) => void;
}>) {
  const t = useT();
  const title = useId();
  const formId = useId();
  const queryClient = useQueryClient();
  const [customizing, setCustomizing] = useState(!!report);
  const [name, setName] = useState(report?.name ?? "");
  const [description, setDescription] = useState(report?.description ?? "");
  const [audience, setAudience] = useState<"private" | "team" | "workspace">(
    report?.audience ?? "private",
  );
  const composition = useComposition(initialSelection);
  const { metrics, kept } = composition;
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
  const selection = composition.selection;
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
          metrics: [...metrics],
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
      {note && <p className="t-caption">{note}</p>}
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
          <ReportComposition
            composition={composition}
            onScopeChange={() => setAudience("private")}
          />
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

export function unsaveable(
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
