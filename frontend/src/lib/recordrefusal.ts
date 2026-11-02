// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  useRecordWriteRefusalCode,
  type WritableRecord,
  type WriteRefusal,
} from "./capability";

/** The record kinds whose reads carry `write_refusal`. */
export type RefusingKind = "contact" | "company" | "lead" | "deal" | "project";

// The one table from refusal code to sentence. The archived and not-yours
// sentences name the record's kind, so each kind has its own; the seat, role
// and owner sentences hold for every kind.
const KIND_SENTENCES = {
  contact: {
    archived: "contact.rail.archivedReadOnly",
    not_yours: "contact.notYoursToChange",
  },
  company: {
    archived: "record.archivedReadOnly",
    not_yours: "record.notYoursToChange",
  },
  lead: {
    archived: "lead.terminalReadOnly",
    not_yours: "lead.notYoursToChange",
  },
  deal: {
    archived: "deal.archivedReadOnly",
    not_yours: "deal.notYoursToChange",
  },
  project: {
    archived: "project.archivedReadOnly",
    not_yours: "project.notYoursToChange",
  },
} as const satisfies Record<
  RefusingKind,
  Record<"archived" | "not_yours", MessageKey>
>;

const SHARED_SENTENCES = {
  seat_read_only: "writeRefusal.seatReadOnly",
  role: "writeRefusal.role",
  unowned: "writeRefusal.unowned",
} as const satisfies Record<
  Exclude<WriteRefusal, "archived" | "not_yours">,
  MessageKey
>;

export type RecordRefusal = Readonly<{ code: WriteRefusal; text: string }>;

/** Why this caller may not change the record, as code and sentence. */
export function useRecordRefusal(
  kind: RefusingKind,
  record: WritableRecord | undefined,
): RecordRefusal | undefined {
  const t = useT();
  const code = useRecordWriteRefusalCode(kind, record);
  if (!code) {
    return undefined;
  }
  const key =
    code === "archived" || code === "not_yours"
      ? KIND_SENTENCES[kind][code]
      : SHARED_SENTENCES[code];
  return { code, text: t(key) };
}

/** The sentence alone, for the controls that only say why they are locked. */
export function useRecordWriteRefusal(
  kind: RefusingKind,
  record: WritableRecord | undefined,
): string | undefined {
  return useRecordRefusal(kind, record)?.text;
}
