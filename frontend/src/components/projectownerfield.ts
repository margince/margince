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

// On create the reader is the blank first value: the server makes a create
// that names nobody its creator's. So the default survives a hidden field.
//
// On edit a blank sends owner_id null, which the update reads as "not
// supplied". So the field is required once the project has an owner.
export function projectOwnerField(
  t: (key: MessageKey) => string,
  picker: ProjectOwnerPicker,
  opts: Readonly<{
    key: string;
    // A form that already has an Owner of its own names this one apart.
    label?: MessageKey;
    currentOwner: string | null;
    mode: "create" | "edit";
  }>,
): CreateField[] {
  const mine = picker.owners.find((option) => option.value === picker.me);
  const others = picker.owners.filter((option) => option.value !== picker.me);
  const options =
    opts.mode === "create"
      ? [{ value: "", label: mine?.label ?? t("project.ownerMe") }, ...others]
      : [...(mine ? [mine] : []), ...others];
  return [
    {
      key: opts.key,
      label: opts.label ?? "project.owner",
      type: "select",
      required: opts.mode === "edit" && opts.currentOwner !== null,
      options,
      hint: picker.hint,
    },
  ];
}
