// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import {
  type KeyboardEvent as ReactKeyboardEvent,
  useCallback,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import {
  useActiveOptionVisible,
  useAnchoredPopup,
  useDismissOnOutsidePress,
} from "./anchoredpopup";
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
 * So: typing is never overridden (`aria-autocomplete="list"`, never `both` —
 * inline completion silently rewrites what a reader is halfway through typing),
 * Escape closes the list and keeps the text, and with nothing to suggest the
 * whole popup apparatus stays out of the way and this is an ordinary input.
 */

export type ComboBoxSuggestion = Readonly<{
  value: string;
  /** Shown after the value, dimmed — what it is, when the id alone is opaque. */
  hint?: string;
}>;

export type ComboBoxProps = Readonly<{
  value: string;
  onChange: (value: string) => void;
  suggestions: readonly ComboBoxSuggestion[];
  placeholder?: string;
  id?: string;
  name?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
}>;

/**
 * The suggestions worth showing for what is in the box: a case-insensitive
 * substring match, and everything when the box is empty.
 *
 * A substring rather than a prefix because the ids are namespaced —
 * `mistralai/mistral-small-3.2-24b-instruct` is found by typing `mistral`, and
 * a prefix match would need the reader to know the vendor's own path segment
 * before it offered them anything.
 *
 * A value that EXACTLY matches a suggestion shows the whole list rather than
 * filtering to itself. Filtering there would be right if the text were always
 * mid-typing, and it is not: a field arrives holding what is already bound, and
 * a reader who opens it is asking what ELSE there is. Narrowing to the one row
 * they already have would make them clear the field to see the alternatives.
 */
export function matchingSuggestions(
  suggestions: readonly ComboBoxSuggestion[],
  typed: string,
): readonly ComboBoxSuggestion[] {
  const needle = typed.trim().toLowerCase();
  if (needle === "") {
    return suggestions;
  }
  if (suggestions.some((s) => s.value.toLowerCase() === needle)) {
    return suggestions;
  }
  return suggestions.filter((s) => s.value.toLowerCase().includes(needle));
}

export function ComboBox(props: ComboBoxProps) {
  const { value, onChange, suggestions, disabled, name } = props;
  const inputRef = useRef<HTMLInputElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const listboxId = useId();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);

  const matches = matchingSuggestions(suggestions, value);
  // Nothing to offer is not a broken list, it is a field with no help — and a
  // control that renders an empty popup or a chevron over nothing tells a reader
  // there is something to open. The whole apparatus stands down.
  const offers = matches.length > 0 && !disabled;
  const listOpen = open && offers;

  const close = useCallback(() => {
    setOpen(false);
    setActive(-1);
  }, []);

  const frame = useAnchoredPopup(inputRef, popupRef, listOpen, close);
  useDismissOnOutsidePress(listOpen, close, inputRef, popupRef);
  useActiveOptionVisible(listOpen, active, listboxId);

  const optionDomId = (index: number) => `${listboxId}-option-${index}`;

  const commit = (index: number) => {
    const picked = matches[index];
    if (picked) {
      onChange(picked.value);
    }
    close();
    inputRef.current?.focus();
  };

  // Arrow keys walk the list; Enter takes the active row; Escape closes and
  // leaves the text alone. Every other key is typing, and typing belongs to the
  // input — which is why this handler returns rather than preventing default.
  const onKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Escape") {
      if (listOpen) {
        event.preventDefault();
        close();
      }
      return;
    }
    if (event.key === "Enter" && listOpen && active !== -1) {
      event.preventDefault();
      commit(active);
      return;
    }
    if (event.key !== "ArrowDown" && event.key !== "ArrowUp") {
      return;
    }
    event.preventDefault();
    if (!offers) {
      return;
    }
    setOpen(true);
    const step = event.key === "ArrowDown" ? 1 : -1;
    // Deliberately does not wrap, for the reason Select's list does not: a jump
    // from the last row back to the first hides from the reader that they
    // reached the end.
    setActive((current) =>
      Math.min(Math.max(current + step, 0), matches.length - 1),
    );
  };

  return (
    <>
      <div className={["combobox", props.className].filter(Boolean).join(" ")}>
        <input
          ref={inputRef}
          id={props.id}
          name={name}
          className="input combobox-input"
          type="text"
          autoComplete="off"
          role="combobox"
          aria-expanded={listOpen}
          aria-haspopup="listbox"
          // Never "both": inline completion rewrites what a reader is halfway
          // through typing, and the one thing this control promises is that it
          // does not.
          aria-autocomplete="list"
          aria-controls={listOpen ? listboxId : undefined}
          aria-activedescendant={
            listOpen && active !== -1 ? optionDomId(active) : undefined
          }
          aria-label={props["aria-label"]}
          aria-labelledby={props["aria-labelledby"]}
          aria-describedby={props["aria-describedby"]}
          aria-invalid={props["aria-invalid"]}
          aria-required={props.required}
          placeholder={props.placeholder}
          disabled={disabled}
          value={value}
          onChange={(event) => {
            onChange(event.target.value);
            setOpen(true);
            // The rows moved under the cursor, so the old index points at a
            // different model than the one that was highlighted. Nothing is
            // active until the reader says so again.
            setActive(-1);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKeyDown}
        />
        {offers && (
          <button
            type="button"
            className="combobox-toggle"
            // The list is reachable from the input's own keyboard grammar, so
            // this is a pointer affordance rather than a second tab stop — one
            // control, one place in the tab order.
            tabIndex={-1}
            aria-hidden="true"
            onClick={() => {
              if (listOpen) {
                close();
                return;
              }
              setOpen(true);
              inputRef.current?.focus();
            }}
          >
            <ChevronDown size={16} aria-hidden="true" />
          </button>
        )}
      </div>
      {listOpen && frame
        ? createPortal(
            <div
              ref={popupRef}
              className="combobox-popup"
              data-above={frame.above ? "true" : undefined}
              style={{
                left: frame.left,
                top: frame.top,
                bottom: frame.bottom,
                width: frame.width,
                maxHeight: frame.maxHeight,
              }}
            >
              {/* Divs rather than ul/li, and no name on the listbox — the same
                  reasoning as Select's popup: the combobox that owns it is
                  named, and a list element claiming an interactive role is
                  announced twice over. */}
              <div className="combobox-list" id={listboxId} role="listbox">
                {matches.map((suggestion, index) => (
                  // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the input's own keydown handling
                  // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays in the text box, which is what keeps typing working
                  <div // NOSONAR: keyboard path is the input's own keydown; an activedescendant option must not be focusable
                    key={suggestion.value}
                    id={optionDomId(index)}
                    role="option"
                    aria-selected={suggestion.value === value}
                    className={[
                      "combobox-option",
                      index === active ? "is-active" : "",
                    ]
                      .filter(Boolean)
                      .join(" ")}
                    onMouseDown={(event) => {
                      // The press must not take focus off the input before the
                      // click lands: blur closes the list, and the click would
                      // then arrive at nothing.
                      event.preventDefault();
                    }}
                    onClick={() => commit(index)}
                    onMouseEnter={() => setActive(index)}
                  >
                    <span className="combobox-option-value">
                      {suggestion.value}
                    </span>
                    {suggestion.hint && (
                      <span className="combobox-option-hint">
                        {suggestion.hint}
                      </span>
                    )}
                  </div>
                ))}
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}
