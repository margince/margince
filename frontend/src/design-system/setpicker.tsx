// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Plus } from "lucide-react";
import { type ReactNode, useId, useMemo, useState } from "react";
import { holdsEveryWord, searchWords } from "../format/collate";
import { Button, Checkbox, SearchField } from "./atoms";
import { ErrorLine } from "./errorline";
import { Popover } from "./popover";
import { type Token, TokenList } from "./tokeninput";
import "./setpicker.css";

export type SetPickerOption = Readonly<{
  value: string;
  label: string;
  /** A caption under the name in the list: what tells two teams apart. */
  detail?: string;
  disabled?: boolean;
}>;

type FrameProps = Readonly<{
  /** The chosen values, in the order they are drawn. */
  chosen: readonly Token[];
  removeLabel: (item: Token) => string;
  onRemove: (id: string) => void;
  /** What the frame says while nothing is chosen. */
  none: ReactNode;
  addLabel: ReactNode;
  disabled?: boolean;
  invalid?: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** For a panel whose rows read three lines, such as a webhook event. */
  wide?: boolean;
  /** The panel behind the add trigger. */
  children: ReactNode;
}>;

/**
 * The chosen set as removable tokens, and the whole list behind one add
 * trigger. A list of forty never stretches the form it sits in.
 */
export function SetPickerFrame({
  chosen,
  removeLabel,
  onRemove,
  none,
  addLabel,
  disabled,
  invalid,
  open,
  onOpenChange,
  wide,
  children,
}: FrameProps) {
  return (
    <div
      className={`set-picker-frame${invalid ? " set-picker-invalid" : ""}`}
      data-disabled={disabled || undefined}
    >
      {chosen.length === 0 ? (
        <span className="t-caption set-picker-none">{none}</span>
      ) : (
        <TokenList
          items={chosen}
          removeLabel={removeLabel}
          onRemove={disabled ? undefined : onRemove}
        />
      )}
      {!disabled && (
        <Popover
          dialog
          variant="ghost"
          align="end"
          className="set-picker-add"
          panelClassName={
            wide ? "set-picker-panel set-picker-panel-wide" : "set-picker-panel"
          }
          open={open}
          onOpenChange={onOpenChange}
          label={
            <>
              <Plus aria-hidden /> {addLabel}
            </>
          }
        >
          {children}
        </Popover>
      )}
    </div>
  );
}

type Create = Readonly<{
  /** The row's words for a typed name, such as `Create team “DACH”`. */
  label: (name: string) => string;
  /** Writes the new value; the caller adds it to the set once it lands. */
  onCreate: (name: string) => void;
  pending?: boolean;
  /** The refused write, drawn under the row. */
  error?: unknown;
}>;

type SetPickerProps = Readonly<{
  legend: string;
  options: readonly SetPickerOption[];
  values: readonly string[];
  onChange: (next: string[]) => void;
  removeLabel: (item: Token) => string;
  none: ReactNode;
  addLabel: string;
  searchLabel: string;
  noMatch: string;
  doneLabel: string;
  /** What the panel says when there is nothing to choose at all. */
  empty?: ReactNode;
  /** Offers the typed name as a new value when no value carries that name. */
  create?: Create;
  disabled?: boolean;
}>;

/**
 * Many of a listed set that is too long to show at rest. The chosen values
 * show as tokens, with a searchable list of ticks behind "Add".
 */
export function SetPicker(props: SetPickerProps) {
  const { legend, options, values, onChange, disabled } = props;
  const [open, setOpen] = useState(false);
  const chosen = useMemo(() => {
    const labels = new Map(options.map((option) => [option.value, option]));
    return values.flatMap((value) => {
      const option = labels.get(value);
      return option ? [{ id: value, label: option.label }] : [];
    });
  }, [options, values]);
  return (
    <fieldset className="set-picker" disabled={disabled}>
      <legend className="set-picker-legend">{legend}</legend>
      <SetPickerFrame
        chosen={chosen}
        removeLabel={props.removeLabel}
        onRemove={(id) => onChange(values.filter((value) => value !== id))}
        none={props.none}
        addLabel={props.addLabel}
        disabled={disabled}
        open={open}
        onOpenChange={setOpen}
      >
        <SetPickerPanel {...props} onDone={() => setOpen(false)} />
      </SetPickerFrame>
    </fieldset>
  );
}

function SetPickerPanel({
  options,
  values,
  onChange,
  searchLabel,
  noMatch,
  doneLabel,
  empty,
  create,
  onDone,
}: SetPickerProps & Readonly<{ onDone: () => void }>) {
  const [term, setTerm] = useState("");
  const listId = useId();
  const chosen = new Set(values);
  const shown = useMemo(() => {
    const words = searchWords(term);
    return options.filter((option) =>
      holdsEveryWord(words, [option.label, option.detail ?? ""]),
    );
  }, [options, term]);
  const name = term.trim();
  const typed = searchWords(name).join(" ");
  // A name that only differs in case or accents is the same team to a reader.
  const taken = options.some(
    (option) => searchWords(option.label).join(" ") === typed,
  );
  const offerCreate = create !== undefined && name !== "" && !taken;
  const toggle = (value: string, on: boolean) =>
    onChange(
      on ? [...values, value] : values.filter((other) => other !== value),
    );
  return (
    <div className="set-picker-body">
      <SearchField
        aria-label={searchLabel}
        aria-controls={listId}
        placeholder={searchLabel}
        value={term}
        onChange={(event) => setTerm(event.target.value)}
      />
      <ul id={listId} className="set-picker-list">
        {shown.map((option) => (
          <li key={option.value}>
            <Checkbox
              className="set-picker-row"
              checked={chosen.has(option.value)}
              disabled={option.disabled}
              onChange={(event) => toggle(option.value, event.target.checked)}
              label={
                <span className="set-picker-label">
                  <span>{option.label}</span>
                  {option.detail && (
                    <span className="t-caption">{option.detail}</span>
                  )}
                </span>
              }
            />
          </li>
        ))}
      </ul>
      {options.length === 0 && empty && !offerCreate && (
        <p className="t-caption">{empty}</p>
      )}
      {options.length > 0 && shown.length === 0 && (
        <p className="t-caption">{noMatch}</p>
      )}
      {offerCreate && (
        <Button
          variant="ghost"
          className="set-picker-create"
          pending={create.pending}
          onClick={() => create.onCreate(name)}
        >
          <Plus aria-hidden /> {create.label(name)}
        </Button>
      )}
      <ErrorLine error={create?.error} />
      <div className="set-picker-foot">
        <Button variant="primary" onClick={onDone}>
          {doneLabel}
        </Button>
      </div>
    </div>
  );
}
