// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { type ActivityDraft, isActivityKind } from "../lib/activitybody";

type Kind = ActivityDraft["kind"];

/** What the logged entry is. A task is not offered while one is being
 *  completed, since its outcome cannot be a second task. */
export function ActivityKindField({
  value,
  offersTask,
  onChange,
}: Readonly<{
  value: Kind;
  offersTask: boolean;
  onChange: (kind: Kind) => void;
}>) {
  const t = useT();
  return (
    <Field label={t("log.kind")}>
      {(control) => (
        <Select
          {...control}
          options={[
            { value: "note", label: t("log.kindNote") },
            ...(offersTask
              ? [{ value: "task", label: t("log.kindTask") }]
              : []),
            { value: "call", label: t("log.kindCall") },
            { value: "meeting", label: t("log.kindMeeting") },
          ]}
          value={value}
          // The clamp narrows the type and validates nothing. The Select only
          // hands back the values above; anything else falls back to the
          // ordinary kind rather than a refusal.
          onChange={(picked) =>
            onChange(isActivityKind(picked) ? picked : "note")
          }
        />
      )}
    </Field>
  );
}
