// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button } from "../design-system/atoms";
import { DiscardDraftQuestion } from "../design-system/modal";
import { useT } from "../i18n";
import { type CreateField, fieldLabel } from "./create.fields";
import "./edit.rows.css";

// Text rather than a read-only input, which reads as a box to type in.
export function ReadOnlyFieldRow({
  field,
  readOnly,
}: Readonly<{
  field: CreateField;
  readOnly: NonNullable<CreateField["readOnly"]>;
}>) {
  const t = useT();
  const reasonId = useId();
  return (
    <fieldset className="field form-flow" aria-describedby={reasonId}>
      <legend className="t-label">{fieldLabel(field, t)}</legend>
      {readOnly.reading && (
        <span className="form-flow-reading">{readOnly.reading}</span>
      )}
      <p className="t-caption" id={reasonId}>
        {readOnly.reason}
      </p>
    </fieldset>
  );
}

export function FlowFieldRow({
  field,
  flow,
  disabled,
  onFollow,
}: Readonly<{
  field: CreateField;
  flow: NonNullable<CreateField["flow"]>;
  disabled: boolean;
  onFollow: (open: () => void) => void;
}>) {
  const t = useT();
  const label = fieldLabel(field, t);
  return (
    <fieldset className="field form-flow">
      <legend className="t-label">{label}</legend>
      <div className="form-flow-line">
        <span className="form-flow-reading">
          {flow.reading ?? t("field.unset")}
        </span>
        <Button
          variant="link"
          disabled={disabled}
          onClick={() => onFollow(flow.open)}
        >
          {t("inlineChoice.change", { field: label })}
        </Button>
      </div>
    </fieldset>
  );
}

/**
 * The rows an edit draws for its read-only and flow fields. A flow leaves the
 * form, so a changed answer asks before it is dropped.
 */
export function useAnswerlessRows({
  open,
  dirty,
  pending,
  onClose,
}: Readonly<{
  open: boolean;
  dirty: boolean;
  pending: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const [following, setFollowing] = useState<(() => void) | null>(null);
  if (!open && following !== null) setFollowing(null);
  const follow = (start: () => void) => {
    if (dirty) {
      setFollowing(() => start);
      return;
    }
    onClose();
    start();
  };
  const row = (field: CreateField) =>
    field.flow ? (
      <FlowFieldRow
        field={field}
        flow={field.flow}
        disabled={pending}
        onFollow={follow}
      />
    ) : (
      field.readOnly && (
        <ReadOnlyFieldRow field={field} readOnly={field.readOnly} />
      )
    );
  const question = (
    <DiscardDraftQuestion
      open={following !== null}
      body={t("unsaved.dialogBody")}
      onKeep={() => setFollowing(null)}
      onDiscard={() => {
        setFollowing(null);
        onClose();
        following?.();
      }}
    />
  );
  return { row, question };
}
