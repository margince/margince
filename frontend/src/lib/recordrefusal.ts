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
// sentences name the record's kind. A project cannot be claimed, so its
// unowned sentence asks for an owner instead.
type KindCode = "archived" | "not_yours" | "unowned";

const KIND_SENTENCES = {
  contact: {
    archived: "contact.rail.archivedReadOnly",
    not_yours: "contact.notYoursToChange",
    unowned: "writeRefusal.unowned",
  },
  company: {
    archived: "record.archivedReadOnly",
    not_yours: "record.notYoursToChange",
    unowned: "writeRefusal.unowned",
  },
  lead: {
    archived: "lead.terminalReadOnly",
    not_yours: "lead.notYoursToChange",
    unowned: "writeRefusal.unowned",
  },
  deal: {
    archived: "deal.archivedReadOnly",
    not_yours: "deal.notYoursToChange",
    unowned: "writeRefusal.unowned",
  },
  project: {
    archived: "project.archivedReadOnly",
    not_yours: "project.notYoursToChange",
    unowned: "writeRefusal.unownedProject",
  },
} as const satisfies Record<
  RefusingKind,
  Record<KindCode, MessageKey>
>;

const SHARED_SENTENCES = {
  seat_read_only: "writeRefusal.seatReadOnly",
  role: "writeRefusal.role",
} as const satisfies Record<
  Exclude<WriteRefusal, KindCode>,
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
    code === "archived" || code === "not_yours" || code === "unowned"
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
