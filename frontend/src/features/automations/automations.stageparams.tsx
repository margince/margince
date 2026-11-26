// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { FieldControl } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { MultiSelect, Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";

export function StageParamSelect({
  kind,
  pipelineId,
  value,
  onChange,
  control,
}: Readonly<{
  kind: "pipeline" | "stages";
  pipelineId: string;
  value: string;
  onChange: (value: string) => void;
  control: FieldControl;
}>) {
  const t = useT();
  const pipelines = usePipelines();
  if (pipelines.isError) {
    return <ErrorLine>{t("auto.stages.loadError")}</ErrorLine>;
  }
  if (kind === "pipeline") {
    return (
      <Select
        {...control}
        value={value}
        onChange={onChange}
        options={(pipelines.data ?? [])
          .filter((pipeline) => !pipeline.archived_at)
          .map((pipeline) => ({ value: pipeline.id, label: pipeline.name }))}
        placeholder={t("auto.stages.choosePipeline")}
      />
    );
  }
  const stages =
    pipelines.data?.find((pipeline) => pipeline.id === pipelineId)?.stages ??
    [];
  const values: unknown = value === "" ? [] : JSON.parse(value);
  const selected = Array.isArray(values)
    ? values.filter((item): item is string => typeof item === "string")
    : [];
  return (
    <MultiSelect
      {...control}
      values={selected}
      onChange={(next) => onChange(JSON.stringify(next))}
      options={stages
        .filter((stage) => !stage.archived_at)
        .map((stage) => ({ value: stage.id, label: stage.name }))}
      disabled={!pipelineId}
      placeholder={t("auto.stages.chooseStages")}
    />
  );
}
