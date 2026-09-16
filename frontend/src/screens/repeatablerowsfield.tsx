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

  // Every press that changes the row list decides where focus goes next, or
  // it falls to <body>: a reorder must follow the row that MOVED rather than
  // the fixed DOM position the index key reuses (otherwise the next press acts
  // on whatever row slid into that slot), removing the last row unmounts the
  // very button that was pressed, and Add would strand focus behind the row it
  // created. Each handler leaves a finder for the element that should hold
  // focus once the re-render has produced it.
  const containerRef = useRef<HTMLDivElement>(null);
  const pendingFocus = useRef<
    ((container: HTMLElement) => HTMLElement | null) | null
  >(null);
  useEffect(() => {
    const findTarget = pendingFocus.current;
    if (!findTarget || !containerRef.current) {
      return;
    }
    pendingFocus.current = null;
    findTarget(containerRef.current)?.focus();
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
    // When the moved row reaches an end its own direction button disables, so
    // focus lands on the opposite one rather than falling to <body>.
    const dirEnabledAtTarget =
      direction === "up" ? target > 0 : target < rows.length - 1;
    const dir = dirEnabledAtTarget
      ? direction
      : direction === "up"
        ? "down"
        : "up";
    pendingFocus.current = (container) =>
      container.querySelector(`[data-move="${dir}"][data-row="${target}"]`);
    setRows(withRowMoved(rows, index, direction));
    setMoveNotice(t("field.rowMoved", { n: ordinalNumber(target + 1) }));
  }

  function removeRow(index: number) {
    // The remove control now at this position — the row that slid in, or the
    // new last row when the removed one was last — or Add once no row is left.
    const survivor = Math.min(index, rows.length - 2);
    pendingFocus.current = (container) =>
      container.querySelector(
        survivor < 0 ? "[data-add-row]" : `[data-remove="${survivor}"]`,
      );
    setRows(rows.filter((_, rowIndex) => rowIndex !== index));
  }

  function addRow() {
    // Focus follows the row the press created rather than staying on Add, so
    // a keyboard user types into the new row without tabbing backwards.
    const created = rows.length;
    pendingFocus.current = (container) =>
      container
        .querySelectorAll(":scope > .card")
        .item(created)
        ?.querySelector("input, textarea, select, button") ?? null;
    setRows([...rows, typeKey ? { [typeKey]: typeDefault } : {}]);
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
          {/* field.removeRow and field.removeRowLabel are one control's two
              names — the short visible text and the row-numbered accessible
              name — and an edit to either wording has to carry the other. */}
          <Button
            type="button"
            data-remove={index}
            aria-label={t("field.removeRowLabel", {
              n: ordinalNumber(index + 1),
            })}
            onClick={() => removeRow(index)}
          >
            {t("field.removeRow")}
          </Button>
        </Card>
      ))}
      <Button type="button" data-add-row onClick={addRow}>
        {field.addLabel ? t(field.addLabel) : fieldLabel(field, t)}
      </Button>
      <p className="sr-only" role="status" aria-live="polite">
        {moveNotice}
      </p>
    </div>
  );
}
