// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, TextInput } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { LossReason } from "../lib/lossreasons.queries";

/**
 * Why a deal was lost: a reason from the administered list, which a report
 * counts, and an optional note that explains this one deal. Every dialog that
 * closes a deal as lost draws this one picker. A dialog that already asks for
 * a note of its own leaves out `onNote`, and the picker comes alone.
 */
export function LostReasonFields({
  reasons,
  code,
  note,
  onCode,
  onNote,
}: Readonly<{
  reasons: readonly LossReason[];
  code: string;
  note?: string;
  onCode: (value: string) => void;
  onNote?: (value: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("deals.lostReasonCode")}>
        {(control) => (
          <Select
            {...control}
            placeholder={t("deals.lostReasonPick")}
            value={code}
            onChange={onCode}
            options={reasons.map((reason) => ({
              value: reason.key,
              label: reason.label,
            }))}
          />
        )}
      </Field>
      {onNote && (
        <Field label={t("deals.lostReasonNote")}>
          {(control) => (
            <TextInput
              {...control}
              value={note ?? ""}
              onChange={(event) => onNote(event.target.value)}
            />
          )}
        </Field>
      )}
    </>
  );
}
