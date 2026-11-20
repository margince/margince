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
import { useSinglePress } from "../../design-system/presslatch";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { type Departure, fromHere } from "../../lib/navigatefrom";
import { throwProblem, unwrap } from "../../lib/problem";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { ReportComposition, useComposition } from "./reporting.composition";
import { useCreateReport } from "./reporting.create";
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
  // `from` is where the reader pressed Save, for a caller that moves them on.
  onSaved: (report: ReportingReport, from: Departure) => void;
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
  const createReport = useCreateReport();
  const singlePress = useSinglePress();
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
      if (!existing) return createReport(body);
      const response = await api.PATCH("/analytics/reports/{id}", {
        params: {
          path: { id: existing.id },
          ...ifMatch(existing.version),
        },
        body,
      });
      if (response.error) throwProblem(response.error);
      return response.data;
    },
    onMutate: fromHere,
    onSuccess: async (report, _vars, from) => {
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
      onSaved(report, from);
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
  // Save stays pressable; a press with something missing names it instead.
  const [attempted, setAttempted] = useState(false);
  const nameMissing = name.trim() === "";
  const refusal = saveRefusal(selection, metrics);
  const submit = (asDraft: boolean) => {
    if (write.isPending) return;
    if (nameMissing || refusal !== undefined) {
      setAttempted(true);
      return;
    }
    save(asDraft);
  };
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
        // The press names a missing name itself; a native bubble would pre-empt it.
        noValidate
        onSubmit={singlePress((event) => {
          event.preventDefault();
          submit(draftOpen);
        })}
      >
        <Field
          label={t("reporting.name")}
          required
          error={
            attempted && nameMissing ? t("record.fieldRequired") : undefined
          }
        >
          {(field) => (
            <TextInput
              {...field}
              value={name}
              disabled={draftOpen}
              onChange={(event) => setName(event.target.value)}
              maxLength={TEXT_LIMITS.label}
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
        {attempted && refusal !== undefined && (
          <ErrorLine inline>{t(refusal)}</ErrorLine>
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
            disabled={write.isPending || metadataChanged}
            onClick={() => submit(true)}
          >
            {t("reporting.saveDraft")}
          </Button>
        )}
        <Button type="submit" form={formId} disabled={write.isPending}>
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
  return saveRefusal(selection, metrics) !== undefined;
}

// Why a report cannot be saved as it stands, or nothing when it can.
function saveRefusal(
  selection: ReportingSelection,
  metrics: readonly unknown[],
): "reporting.needsChart" | "reporting.needsInterval" | undefined {
  if (metrics.length === 0) return "reporting.needsChart";
  if (selection.period !== "custom") return undefined;
  return !selection.interval ||
    selection.interval.start_at >= selection.interval.end_at
    ? "reporting.needsInterval"
    : undefined;
}
