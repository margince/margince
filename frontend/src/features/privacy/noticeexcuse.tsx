// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field, Textarea } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useLocale, useT } from "../../i18n";
import { blankRequired } from "../../lib/problem";

// The two ways to end a disclosure duty without sending anything. Shared by the
// privacy settings queue and the Focus card, so both offer the same grounds.
export const EXCUSE_STATES = [
  "provided_elsewhere",
  "exempt_with_reason",
] as const;
export type ExcuseState = (typeof EXCUSE_STATES)[number];

// Ending a duty without sending anything, on a ground the officer states.
//
// The ground is required by the server and by this form, which is the whole
// reason these two states exist beside `not_required` — that one records the
// same conclusion with nothing to defend it.
export function ExcuseModal({
  open,
  onClose,
  onConfirm,
  pending,
  error,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  onConfirm: (state: ExcuseState, note: string) => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [state, setState] = useState<ExcuseState>("provided_elsewhere");
  const [note, setNote] = useState("");
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const groundLabel = t("notice.excuseGround");

  const stateOptions = EXCUSE_STATES.map((value) => ({
    value,
    label:
      value === "provided_elsewhere"
        ? t("notice.excuseProvided")
        : t("notice.excuseExempt"),
  }));

  return (
    <ConfirmModal
      open={open}
      onClose={() => {
        setNote("");
        faults.show(new Map());
        onClose();
      }}
      intent="form"
      title={t("notice.excuseTitle")}
      confirmLabel={t("notice.excuseConfirm")}
      // The server refuses without a ground, so the press says so first.
      bodyRef={faultsRoot}
      onConfirm={() => {
        const blank = blankRequired(locale, { note: [groundLabel, note] });
        if (!faults.show(blank)) onConfirm(state, note.trim());
      }}
      pending={pending}
      error={error}
    >
      <Field label={t("notice.excuseWhich")}>
        {() => (
          <Select
            options={stateOptions}
            value={state}
            onChange={(value) => setState(value as ExcuseState)}
            name="notice-excuse-state"
          />
        )}
      </Field>
      <Field label={groundLabel} required error={faults.of("note")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            // The server holds 500 characters too. Bounded here as well so a
            // reader learns the limit while typing rather than on submit.
            maxLength={500}
            onChange={(event) => {
              setNote(event.target.value);
              faults.edited("note");
            }}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
