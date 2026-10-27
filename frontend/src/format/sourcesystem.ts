// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A product's own spelling of its name, keyed by the lowercase id an import
// stores as the system a record came from.
const SOURCE_SYSTEM_NAME: Readonly<Record<string, string>> = {
  hubspot: "HubSpot",
  salesforce: "Salesforce",
};

/** The system a record came from, as its maker spells it. An id this build
 *  does not know reads as stored. */
export function sourceSystemName(id: string): string {
  const known = id.trim().toLowerCase();
  return Object.hasOwn(SOURCE_SYSTEM_NAME, known)
    ? (SOURCE_SYSTEM_NAME[known] ?? id)
    : id;
}
