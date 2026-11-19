// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ImportFormatColumn } from "./importtypes";

/** What the Map step calls a field: a custom field's label, else the core field's. */
export function fieldLabelOf(column: ImportFormatColumn): string {
  return column.label ?? column.field_label ?? column.name;
}
