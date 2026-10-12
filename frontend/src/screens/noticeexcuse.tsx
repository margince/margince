// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field, Textarea } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { useT } from "../i18n";

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
  const [state, setState] = useState<ExcuseState>("provided_elsewhere");
  const [note, setNote] = useState("");

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
        onClose();
      }}
      intent="form"
      title={t("notice.excuseTitle")}
      confirmLabel={t("notice.excuseConfirm")}
      // Disabled until there is a ground, because the server refuses without
      // one and a button that fails is worse than one that waits.
      confirmDisabled={note.trim() === ""}
      onConfirm={() => onConfirm(state, note.trim())}
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
      <Field label={t("notice.excuseGround")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            // The server holds 500 characters too. Bounded here as well so a
            // reader learns the limit while typing rather than on submit.
            maxLength={500}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
