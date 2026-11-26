// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// The six address columns of a record and the words that name each one. The
// Address dialog and History both read them here, so the two say one thing.
export const ADDRESS_COLUMN_LABELS: ReadonlyArray<
  readonly [column: string, label: MessageKey]
> = [
  ["address_line1", "create.addressLine1"],
  ["address_line2", "create.addressLine2"],
  ["address_postal_code", "create.postalCode"],
  ["address_city", "create.city"],
  ["address_region", "create.region"],
  ["address_country", "create.country"],
];

export const ADDRESS_KEYS: readonly string[] = ADDRESS_COLUMN_LABELS.map(
  ([column]) => column,
);
