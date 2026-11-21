// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { listFormat } from "../../format/formatters";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { EntityKind } from "../../lib/entity";

// The plural noun each record kind takes in running text: "contacts, deals".
const KIND_NOUN = {
  contact: "unit.contacts",
  company: "unit.companies",
  lead: "unit.leads",
  deal: "unit.deals",
  project: "unit.projects",
} as const satisfies Record<EntityKind, MessageKey>;

/** The kinds a seat reads, as one list in the reader's language. */
export function useKindList(): (kinds: readonly EntityKind[]) => string {
  const t = useT();
  const { locale } = useLocale();
  return (kinds) =>
    listFormat(locale, {
      style: "long",
      type: "conjunction",
    }).format(kinds.map((kind) => t(KIND_NOUN[kind])));
}
