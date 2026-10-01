import "./reporting.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { useCan } from "../app/capability";
import { navigate } from "../app/router";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Select } from "../design-system/select";
import { INTL_LOCALE } from "../format/format";
import { useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import type { ReportingReport } from "./reporting.model";

type Schedule = components["schemas"]["ReportingSchedule"];
type Input = components["schemas"]["ReportingScheduleInput"];
export function ReportingScheduleDialog({
  report,
  schedule,
  timezone,
  onClose,
}: Readonly<{
  report: ReportingReport;
  schedule?: Schedule;
  timezone: string;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const id = useId();
  const client = useQueryClient();
  const [definition, setDefinition] = useState<Input>(
    schedule?.definition ?? {
      report_revision: report.revision,
      frequency: "weekly",
      day: 1,
      local_time: "09:00",
      enabled: false,
    },
  );
  const canRetention = useCan("retention_policy", "read");
  const readiness = useQuery({
    queryKey: ["reporting-catalog"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/metrics");
      if (error) throwProblem(error);
      return data;
    },
  });
  const ready = readiness.data?.schedule_ready === true;
  const write = useMutation({
    mutationFn: async ({
      reportId,
      previous,
      input,
    }: {
      reportId: string;
      previous?: Schedule;
      input: Input;
    }) => {
      const result = previous
        ? await api.PATCH("/analytics/schedules/{id}", {
            params: {
              path: { id: previous.id },
              ...ifMatch(previous.version),
            },
            body: input,
          })
        : await api.POST("/analytics/reports/{id}/schedules", {
            params: { path: { id: reportId } },
            body: input,
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    onSuccess: async () => {
      await client.invalidateQueries({
        queryKey: ["reporting-schedules", report.id],
      });
      await client.invalidateQueries({ queryKey: ["reporting-reports"] });
      onClose();
    },
  });
  const dayValue = String(definition.day);
  const days = Array.from(
    { length: definition.frequency === "weekly" ? 7 : 31 },
    (_, index) => ({
      value: String(index + 1),
      label:
        definition.frequency === "weekly"
          ? new Intl.DateTimeFormat(INTL_LOCALE[locale], {
              weekday: "long",
              timeZone: "UTC",
            }).format(new Date(Date.UTC(2026, 0, 5 + index)))
          : String(index + 1),
    }),
  );
  return (
    <Modal open onClose={onClose} labelledBy={id}>
      <form
        className="reporting-dialog"
        onSubmit={(event) => {
          event.preventDefault();
          write.mutate({
            reportId: report.id,
            previous: schedule,
            input: { ...definition, enabled: true },
          });
        }}
      >
        <Heading id={id} as="h2" size="medium">
          {t("reporting.schedule")} · {report.name}
        </Heading>
        <p>
          {report.selection.scope.label} · {t(`reporting.${report.audience}`)} ·{" "}
          {timezone}
        </p>
        <p className="t-caption">{t("reporting.scheduleResult")}</p>
        {readiness.isSuccess && !ready && (
          <p role="status">{t("reporting.scheduleSetup")}</p>
        )}
        {readiness.isSuccess && !ready && canRetention && (
          <Button
            variant="link"
            onClick={() => navigate({ screen: "settings", id: "retention" })}
          >
            {t("reporting.retentionSettings")}
          </Button>
        )}
        <ErrorLine error={readiness.error} />
        {definition.report_revision !== report.revision && (
          <div>
            <p>{t("reporting.pinnedSettings")}</p>
            <Button
              variant="link"
              onClick={() =>
                setDefinition({
                  ...definition,
                  report_revision: report.revision,
                })
              }
            >
              {t("reporting.useCurrentSettings")}
            </Button>
          </div>
        )}
        <Field label={t("reporting.frequency")}>
          {(field) => (
            <Select
              {...field}
              value={definition.frequency}
              options={[
                { value: "weekly", label: t("reporting.weekly") },
                { value: "monthly", label: t("reporting.monthly") },
              ]}
              onChange={(value) => {
                if (value === "weekly" || value === "monthly")
                  setDefinition({ ...definition, frequency: value, day: 1 });
              }}
            />
          )}
        </Field>
        <Field label={t("reporting.day")}>
          {(field) => (
            <Select
              {...field}
              value={dayValue}
              options={days}
              onChange={(value) =>
                setDefinition({ ...definition, day: Number(value) })
              }
            />
          )}
        </Field>
        <Field label={t("reporting.time")} required>
          {(field) => (
            <TextInput
              {...field}
              type="time"
              value={definition.local_time}
              onChange={(event) =>
                setDefinition({ ...definition, local_time: event.target.value })
              }
            />
          )}
        </Field>
        <ErrorLine error={write.error} />
        <div className="reporting-dialog-actions">
          <Button variant="ghost" onClick={onClose}>
            {t("reporting.cancel")}
          </Button>
          <Button
            variant="ghost"
            disabled={write.isPending}
            onClick={() =>
              write.mutate({
                reportId: report.id,
                previous: schedule,
                input: { ...definition, enabled: false },
              })
            }
          >
            {t("reporting.savePaused")}
          </Button>
          <Button type="submit" disabled={write.isPending || !ready}>
            {t(
              schedule?.definition.enabled
                ? "reporting.updateSchedule"
                : "reporting.activateSchedule",
            )}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
