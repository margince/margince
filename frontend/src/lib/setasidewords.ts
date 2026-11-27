// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { type RbacObject, useCanWriteRecord } from "./capability";
import { useMe } from "./session";

/**
 * Whether the signed-in reader may not change this record. False until `/me`
 * answers, so the bar does not flash the view-only words at its owner.
 */
export function useReadsOnly(
  kind: RbacObject,
  record: { readonly writable?: boolean } | undefined,
): boolean {
  const canWrite = useCanWriteRecord(kind, record);
  const known = useMe().data?.authorization !== undefined;
  return known && !canWrite;
}

/**
 * What the Next bar calls its set-aside. For a reader who may not change the
 * record, it is the one live verb among locked ones. So it says it hides the
 * card for them alone.
 */
export function useSetAsideWords(
  viewOnly: boolean,
): Readonly<{ label: string; hint?: string }> {
  const t = useT();
  return viewOnly
    ? {
        label: t("record.setAside.forMe"),
        hint: t("record.setAside.forMeHint"),
      }
    : { label: t("record.setAside.verb") };
}
