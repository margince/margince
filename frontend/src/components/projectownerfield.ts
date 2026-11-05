// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import type { CreateField } from "./create";
import type { CreateFieldOption } from "./create.fields";

/** Who a project's owner may be picked from, and who is picking. */
export type ProjectOwnerPicker = Readonly<{
  // useRecordOwners: the roster less agent seats, plus the current owner.
  owners: readonly CreateFieldOption[];
  me: string;
  // Set when the roster failed or stopped short.
  hint?: string;
}>;

// On create a blank means the creator.
// On edit a blank is sent as null, which clears the owner.
export function projectOwnerField(
  t: (key: MessageKey) => string,
  picker: ProjectOwnerPicker,
  opts: Readonly<{
    key: string;
    // A form that already has an Owner of its own names this one apart.
    label?: MessageKey;
    mode: "create" | "edit";
  }>,
): CreateField[] {
  const mine = picker.owners.find((option) => option.value === picker.me);
  const others = picker.owners.filter((option) => option.value !== picker.me);
  const options =
    opts.mode === "create"
      ? [{ value: "", label: mine?.label ?? t("project.ownerMe") }, ...others]
      : [
          ...(mine ? [mine] : []),
          ...others,
          { value: "", label: t("list.unowned") },
        ];
  return [
    {
      key: opts.key,
      label: opts.label ?? "project.owner",
      type: "select",
      options,
      hint: picker.hint,
    },
  ];
}
