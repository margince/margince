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
import { problemMessageOf, unwrap } from "../../lib/problem";

type CustomField = components["schemas"]["CustomField"];
type CustomFieldList = components["schemas"]["CustomFieldListResponse"];
type ActivityKind = NonNullable<
  components["schemas"]["RenameCustomFieldRequest"]["activity_kinds"]
>[number];

// Every kind the server takes, so a stored kind the form does not log survives.
const ACTIVITY_KINDS: readonly ActivityKind[] = [
  "call",
  "email",
  "meeting",
  "message",
  "note",
  "task",
];

/** The chosen values as the wire's kind list, in the server's order. */
export function activityKindsOf(values: readonly string[]): ActivityKind[] {
  return ACTIVITY_KINDS.filter((kind) => values.includes(kind));
}

// The types the Log activity form logs, the ones an admin narrows a field to.
const LOGGED_KINDS = ["note", "task", "call", "meeting"] as const;

const KIND_LABEL = {
  note: "log.kindNote",
  task: "log.kindTask",
  call: "log.kindCall",
  meeting: "log.kindMeeting",
} as const;

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
      options={LOGGED_KINDS.map((kind) => ({
        value: kind,
        label: t(KIND_LABEL[kind]),
      }))}
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
