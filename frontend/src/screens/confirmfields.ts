// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// The fields a confirm link lets a contact correct, in the order the page shows
// them. `company` stays out: correcting it would create or merge a company record.
export const CORRECTABLE = ["full_name", "title", "email", "phone"] as const;
export type CorrectableField = (typeof CORRECTABLE)[number];

export const FIELD_LABELS: Readonly<Record<CorrectableField, MessageKey>> = {
  full_name: "confirm.field.fullName",
  title: "confirm.field.title",
  email: "confirm.field.email",
  phone: "confirm.field.phone",
};

export function correctableFieldLabel(field: string): MessageKey | undefined {
  const known = CORRECTABLE.find((candidate) => candidate === field);
  return known && FIELD_LABELS[known];
}
