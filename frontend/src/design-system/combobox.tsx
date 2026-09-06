// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import { type KeyboardEvent as ReactKeyboardEvent, useRef } from "react";
import { type Suggestion, SuggestPopup, useSuggestList } from "./suggestlist";
import "./combobox.css";

/**
 * A text field that also OFFERS. The value is whatever is in the box; the list
 * is help, never a constraint.
 *
 * The distinction from `Select` is the whole reason this exists, and it is not
 * about looks: a `Select` answers a question whose answers are known, and this
 * one answers a question where the known answers are a good starting set and the
 * real vocabulary belongs to somebody else. A model id is the case it was built
 * for — the price sheet knows which models this installation can cost, the
 * vendor ships a new one on a Tuesday, and a field that refused the new one
 * would make the picker worse than the plain text box it replaced.
 *
 * The list, the matching and the keyboard grammar are `suggestlist.tsx`, shared
 * with `TokenInput`: typing is never overridden (`aria-autocomplete="list"`,
 * never `both`), Escape closes the list and keeps the text, and with nothing to
 * suggest the whole popup apparatus stays out of the way and this is an ordinary
 * input. What is HERE is the half that is this control's own: one value bound to
 * the box, and the chevron that opens the list by pointer.
 */

export type ComboBoxSuggestion = Suggestion;

export type ComboBoxProps = Readonly<{
  value: string;
  onChange: (value: string) => void;
  suggestions: readonly ComboBoxSuggestion[];
  placeholder?: string;
  disabled?: boolean;
  /** The four a `Field` hands its control, spread at every call site. */
  id?: string;
  required?: boolean;
  "aria-label"?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
}>;

export function ComboBox(props: ComboBoxProps) {
  const { value, onChange, suggestions, disabled } = props;
  const inputRef = useRef<HTMLInputElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);

  const list = useSuggestList({
    anchorRef: inputRef,
    popupRef,
    suggestions,
    typed: value,
    disabled,
    // A pick IS the value here: this control binds one, so the row replaces
    // what is in the box rather than being added to a set beside it.
    onPick: (picked) => {
      onChange(picked);
      inputRef.current?.focus();
    },
  });

  // Everything the list does not claim is typing, and typing belongs to the
  // input — which is why nothing here prevents a default the list did not take.
  const onKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    list.navigate(event);
  };

  return (
    <>
      <div className="combobox">
        <input
          ref={inputRef}
          id={props.id}
          className="input combobox-input"
          type="text"
          autoComplete="off"
          {...list.fieldAria}
          aria-label={props["aria-label"]}
          aria-describedby={props["aria-describedby"]}
          aria-invalid={props["aria-invalid"]}
          aria-required={props.required}
          placeholder={props.placeholder}
          disabled={disabled}
          value={value}
          onChange={(event) => {
            onChange(event.target.value);
            list.retype();
          }}
          onFocus={list.show}
          // A list left hanging over the page under a control nobody is focused
          // on, still claiming aria-expanded, is what a keyboard reader gets
          // otherwise: the outside-press dismissal never fires for them. Safe as
          // a plain close because nothing in the popup takes focus — the options
          // are activedescendant rows and the chevron refuses the press that
          // would move it.
          onBlur={list.close}
          onKeyDown={onKeyDown}
        />
        {list.offers && (
          <button
            type="button"
            className="combobox-toggle"
            // The press must not move focus off the input: blur closes the list,
            // so a chevron that took focus would close it on the way to opening
            // it.
            onMouseDown={(event) => event.preventDefault()}
            // The list is reachable from the input's own keyboard grammar, so
            // this is a pointer affordance rather than a second tab stop — one
            // control, one place in the tab order.
            tabIndex={-1}
            aria-hidden="true"
            onClick={() => {
              if (list.open) {
                list.close();
                return;
              }
              list.show();
              inputRef.current?.focus();
            }}
          >
            <ChevronDown size={16} aria-hidden="true" />
          </button>
        )}
      </div>
      <SuggestPopup list={list} face="mono" selected={value} />
    </>
  );
}
