// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which activity types an activity field shows on, and which pipelines a deal
// field shows in. None chosen is every type or pipeline, which is what the
// server stores as null.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { MultiSelect } from "../../design-system/select";
import { ACTIVITY_KIND_LABEL } from "../../design-system/timelineentry";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { customFieldsQuery } from "../../lib/customfields.query";
import { labelFor } from "../../lib/labelfor";
import { usePipelines } from "../../lib/pipelines.queries";
import { problemMessageOf, unwrap } from "../../lib/problem";

type CustomField = components["schemas"]["CustomField"];
type ActivityKind = NonNullable<
  components["schemas"]["RenameCustomFieldRequest"]["activity_kinds"]
>[number];

function isActivityKind(value: string): value is ActivityKind {
  return Object.hasOwn(ACTIVITY_KIND_LABEL, value);
}

/** The chosen values as the wire's kind list, without a word it does not take. */
export function activityKindsOf(values: readonly string[]): ActivityKind[] {
  return values.filter(isActivityKind);
}

export function ActivityKindsSelect({
  values,
  onChange,
  disabled,
  ...aria
}: Readonly<{
  values: readonly string[];
  onChange: (next: string[]) => void;
}> &
  SelectAria) {
  const t = useT();
  return (
    <MultiSelect
      {...aria}
      options={Object.keys(ACTIVITY_KIND_LABEL)
        .filter(isActivityKind)
        .map((kind) => ({
          value: kind,
          label: t(labelFor(ACTIVITY_KIND_LABEL, kind)),
        }))}
      values={values}
      onChange={onChange}
      disabled={disabled}
      placeholder={t("cf.activityKindsEvery")}
    />
  );
}

type SelectAria = Readonly<{
  disabled?: boolean;
  id?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
}>;

/** The live pipelines a deal field can be narrowed to. */
export function PipelinesSelect({
  values,
  onChange,
  ...aria
}: Readonly<{
  values: readonly string[];
  onChange: (next: string[]) => void;
}> &
  SelectAria) {
  const t = useT();
  const pipelines = usePipelines().data ?? [];
  return (
    <MultiSelect
      {...aria}
      options={pipelines.map((pipeline) => ({
        value: pipeline.id,
        label: pipeline.name,
      }))}
      values={values}
      onChange={onChange}
      placeholder={t("cf.pipelinesEvery")}
    />
  );
}

/**
 * The write of one scope setting of a field from the settings table. Each change
 * writes, as a setting switch does; the saved row goes into the list so the
 * next change sends the version this one left.
 */
function useSetFieldScope<V extends { field: CustomField }>(
  bodyOf: (input: V) => components["schemas"]["RenameCustomFieldRequest"],
  saved: "cf.activityKindsSaved" | "cf.pipelinesSaved",
) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: V) =>
      unwrap(
        await api.PATCH("/custom-fields/{id}", {
          params: {
            path: { id: input.field.id },
            ...ifMatch(requireVersion(input.field.version)),
          },
          body: bodyOf(input),
        }),
      ),
    onSuccess: (row, input) => {
      queryClient.setQueryData(
        customFieldsQuery(input.field.object).queryKey,
        (old) =>
          old && {
            ...old,
            data: old.data.map((one) => (one.id === row.id ? row : one)),
          },
      );
      queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
      toast.show(t(saved, { label: input.field.label }));
    },
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger" });
    },
  });
}

/** The write of a field's activity types from the settings table. */
export function useSetActivityKinds() {
  return useSetFieldScope(
    (input: { field: CustomField; kinds: string[] }) => ({
      activity_kinds: activityKindsOf(input.kinds),
    }),
    "cf.activityKindsSaved",
  );
}

/** The write of a deal field's pipelines from the settings table. */
export function useSetPipelines() {
  return useSetFieldScope(
    (input: { field: CustomField; pipelineIds: string[] }) => ({
      pipeline_ids: input.pipelineIds,
    }),
    "cf.pipelinesSaved",
  );
}
