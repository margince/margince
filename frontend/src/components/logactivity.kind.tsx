// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { type ActivityDraft, isActivityKind } from "../lib/activitybody";

/**
 * The Type picker of Log activity. A task is left out where the form completes
 * one, and a message is offered only where a transport is registered to name.
 */
export function LogKindField({
  value,
  offerTask,
  offerMessage,
  onChange,
}: Readonly<{
  value: ActivityDraft["kind"];
  offerTask: boolean;
  offerMessage: boolean;
  onChange: (kind: ActivityDraft["kind"]) => void;
}>) {
  const t = useT();
  const options = [
    { value: "note", label: t("log.kindNote") },
    ...(offerTask ? [{ value: "task", label: t("log.kindTask") }] : []),
    { value: "call", label: t("log.kindCall") },
    { value: "meeting", label: t("log.kindMeeting") },
    ...(offerMessage
      ? [{ value: "message", label: t("log.kindMessage") }]
      : []),
  ];
  return (
    <Field label={t("log.kind")}>
      {(control) => (
        <Select
          {...control}
          options={options}
          value={value}
          // The clamp narrows the type and validates nothing. The Select only
          // hands back the values above; anything else falls back to the
          // ordinary kind rather than a refusal.
          onChange={(next) => onChange(isActivityKind(next) ? next : "note")}
        />
      )}
    </Field>
  );
}
