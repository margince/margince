// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which activity types an activity field shows on. None chosen is every type,
// which is what the server stores as null.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { MultiSelect } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, unwrap } from "../../lib/problem";

type CustomField = components["schemas"]["CustomField"];
type CustomFieldList = components["schemas"]["CustomFieldListResponse"];
type ActivityKind = NonNullable<
  components["schemas"]["RenameCustomFieldRequest"]["activity_kinds"]
>[number];

// Every kind the contract takes, by its label. Keyed by the generated kind
// union, so a kind the contract adds fails the build until it has a label.
const KIND_LABEL = {
  note: "timeline.kind.note",
  task: "timeline.kind.task",
  call: "timeline.kind.call",
  meeting: "timeline.kind.meeting",
  email: "timeline.kind.email",
  message: "timeline.kind.message",
} as const satisfies Record<ActivityKind, MessageKey>;

function isActivityKind(value: string): value is ActivityKind {
  return Object.hasOwn(KIND_LABEL, value);
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
  disabled?: boolean;
  id?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
}>) {
  const t = useT();
  return (
    <MultiSelect
      {...aria}
      options={Object.keys(KIND_LABEL)
        .filter(isActivityKind)
        .map((kind) => ({ value: kind, label: t(KIND_LABEL[kind]) }))}
      values={values}
      onChange={onChange}
      disabled={disabled}
      placeholder={t("cf.activityKindsEvery")}
    />
  );
}

/**
 * The write of a field's activity types from the settings table. Each change
 * writes, as a setting switch does; the saved row goes into the list so the
 * next change sends the version this one left.
 */
export function useSetActivityKinds() {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { field: CustomField; kinds: string[] }) =>
      unwrap(
        await api.PATCH("/custom-fields/{id}", {
          params: {
            path: { id: input.field.id },
            header: input.field.version
              ? { "If-Match": String(input.field.version) }
              : undefined,
          },
          body: { activity_kinds: activityKindsOf(input.kinds) },
        }),
      ),
    onSuccess: (saved, input) => {
      queryClient.setQueryData<CustomFieldList>(
        ["custom-fields", input.field.object],
        (old) =>
          old && {
            ...old,
            data: old.data.map((one) => (one.id === saved.id ? saved : one)),
          },
      );
      queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
      toast.show(t("cf.activityKindsSaved", { label: input.field.label }));
    },
    onError: (error) => {
      toast.show(problemMessageOf(error, t), { tone: "danger" });
    },
  });
}
