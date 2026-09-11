// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown, ChevronUp } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Button, Card, Field, Radio } from "../design-system/atoms";
import { ordinalNumber } from "../format/format";
import { useT } from "../i18n";
import {
  type CreateField,
  type FormRow,
  fieldControl,
  fieldLabel,
} from "./create";
import {
  kindOf,
  withPrimaryMarked,
  withRowMoved,
  withRowUpdated,
} from "./createrows";

// A repeatable-row field (emails/phones/domains): each existing row renders
// its subfields via the same fieldControl every scalar field uses, plus an
// optional "primary" radio (selecting one clears it on every other row),
// move-up/move-down and a remove button; an "Add" button appends a blank row.
// Rows live in the second `rows` channel — never merged into `values` — so
// scalar-only screens stay untouched.
export function RepeatableRowsField({
  field,
  formId,
  rows,
  setRows,
}: Readonly<{
  field: CreateField;
  formId: string;
  rows: FormRow[];
  setRows: (next: FormRow[]) => void;
}>) {
  const t = useT();
  const rowFields = field.rowFields ?? [];
  const primaryKey = field.primaryKey;
  const typeKey = field.typeKey;
  const typeDefault = field.typeDefault ?? "";
  // Reorder moves nothing on screen a sighted reader cannot see, but a screen
  // reader hears only the button it pressed — so the new position is announced
  // through a polite live region rather than left silent.
  const [moveNotice, setMoveNotice] = useState("");

  // After a reorder, focus follows the row that MOVED rather than staying on the
  // fixed DOM position the index key reuses — otherwise the next press acts on
  // whatever row slid into that slot. When the moved row reaches an end its own
  // direction button disables, so focus lands on the opposite one rather than
  // falling to <body>.
  const containerRef = useRef<HTMLDivElement>(null);
  const pendingFocus = useRef<{ pos: number; dir: "up" | "down" } | null>(null);
  useEffect(() => {
    const req = pendingFocus.current;
    if (!req) {
      return;
    }
    pendingFocus.current = null;
    containerRef.current
      ?.querySelector<HTMLButtonElement>(
        `[data-move="${req.dir}"][data-row="${req.pos}"]`,
      )
      ?.focus();
  });

  function updateRow(index: number, key: string, value: string) {
    setRows(withRowUpdated(rows, index, key, value, primaryKey, typeKey));
  }

  function markPrimary(index: number) {
    if (!primaryKey) {
      return;
    }
    setRows(withPrimaryMarked(rows, index, primaryKey, typeKey, typeDefault));
  }

  function moveRow(index: number, direction: "up" | "down") {
    const target = direction === "up" ? index - 1 : index + 1;
    if (target < 0 || target >= rows.length) {
      return;
    }
    const dirEnabledAtTarget =
      direction === "up" ? target > 0 : target < rows.length - 1;
    pendingFocus.current = {
      pos: target,
      dir: dirEnabledAtTarget ? direction : direction === "up" ? "down" : "up",
    };
    setRows(withRowMoved(rows, index, direction));
    setMoveNotice(t("field.rowMoved", { n: ordinalNumber(target + 1) }));
  }

  function removeRow(index: number) {
    setRows(rows.filter((_, rowIndex) => rowIndex !== index));
  }

  return (
    <div className="field-repeatable" ref={containerRef}>
      <span className="t-label">
        {fieldLabel(field, t)}
        {field.required ? " *" : ""}
      </span>
      {rows.map((row, index) => (
        // Rows have no stable identity until saved, so index is the only key.
        // Reorder swaps two entries, but every cell — each subfield input and
        // the primary radio — is controlled from rows[index], so React re-renders
        // each position with the swapped row's values rather than carrying stale
        // local state; the index key stays correct under a move.
        <Card
          as="div"
          // biome-ignore lint/suspicious/noArrayIndexKey: every cell is controlled from rows[index], so a swap re-renders in place
          key={index}
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "var(--space-2)",
            alignItems: "center",
          }}
        >
          {rowFields.map((subField) => (
            <Field
              key={subField.key}
              label={t(subField.label)}
              required={subField.required}
            >
              {(control) =>
                fieldControl(
                  subField,
                  control,
                  row[subField.key] ?? "",
                  (next) => updateRow(index, subField.key, next),
                  t,
                )
              }
            </Field>
          ))}
          {primaryKey && (
            <Radio
              className="t-label"
              // Scoped by kind so the native radio group itself cannot enforce exclusivity across kinds.
              name={`${formId}-${field.key}-${kindOf(row, typeKey, typeDefault)}-primary`}
              checked={row[primaryKey] === "true"}
              onChange={() => markPrimary(index)}
              label={t("field.primary")}
            />
          )}
          <Button
            small
            type="button"
            variant="ghost"
            data-move="up"
            data-row={index}
            disabled={index === 0}
            aria-label={t("field.moveRowUp", { n: ordinalNumber(index + 1) })}
            onClick={() => moveRow(index, "up")}
          >
            <ChevronUp aria-hidden size={16} />
          </Button>
          <Button
            small
            type="button"
            variant="ghost"
            data-move="down"
            data-row={index}
            disabled={index === rows.length - 1}
            aria-label={t("field.moveRowDown", { n: ordinalNumber(index + 1) })}
            onClick={() => moveRow(index, "down")}
          >
            <ChevronDown aria-hidden size={16} />
          </Button>
          <Button small type="button" onClick={() => removeRow(index)}>
            {t("field.removeRow")}
          </Button>
        </Card>
      ))}
      <Button small type="button" onClick={() => setRows([...rows, {}])}>
        {field.addLabel ? t(field.addLabel) : fieldLabel(field, t)}
      </Button>
      <p className="sr-only" role="status" aria-live="polite">
        {moveNotice}
      </p>
    </div>
  );
}
