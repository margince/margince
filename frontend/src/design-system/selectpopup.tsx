// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check } from "lucide-react";
import { type KeyboardEvent as ReactKeyboardEvent, useCallback } from "react";
import { holdsEveryWord, searchWords } from "../format/collate";
import { useT } from "../i18n";
import { contentSizedPopupBox, type PopupFrame } from "./anchoredpopup";
import type { SelectOption } from "./select";
import type { Listbox } from "./selectlistbox";

// A list this long scrolls, so the open list gets a box that narrows it. A
// first-letter jump finds one name; the box finds a part of any word.
export const FILTER_FROM = 12;

export function filtered(
  options: readonly SelectOption[],
  query: string,
): readonly SelectOption[] {
  const words = searchWords(query);
  return words.length === 0
    ? options
    : options.filter((option) =>
        holdsEveryWord(words, [option.label, ...(option.keywords ?? [])]),
      );
}

export type ListFilter = Readonly<{
  query: string;
  onQuery: (next: string) => void;
}>;

export function SelectPopup({
  options,
  selected,
  multiselectable,
  frame,
  listbox,
  animate,
  filter,
}: Readonly<{
  options: readonly SelectOption[];
  // Membership, not a value. The single select asks "is this the value" and
  // the multi asks "is this in the set"; the popup draws both the same way.
  selected: (option: SelectOption) => boolean;
  multiselectable?: boolean;
  frame: PopupFrame;
  listbox: Listbox;
  animate: boolean;
  filter?: ListFilter;
}>) {
  if (filter) {
    return (
      <FilteredPopup
        popupRef={listbox.popup}
        options={options}
        selected={selected}
        frame={frame}
        listbox={listbox}
        animate={animate}
        filter={filter}
      />
    );
  }
  return (
    // The scroller is the listbox, so it takes no tab stop. Divs, as ul/li
    // would announce twice; no name, as the combobox is named.
    <div
      ref={listbox.popup}
      className="select-popup"
      id={listbox.listboxId}
      role="listbox"
      aria-multiselectable={multiselectable}
      // Reduced motion resolves in the hook, not a second media query in the
      // stylesheet. So the suite can assert the decision, not only a browser.
      data-motion={animate ? "in" : "none"}
      data-above={frame.above ? "true" : undefined}
      // The trigger's width is the list's floor, not its width. See
      // `contentSizedPopupBox` and the cap in select.css.
      style={contentSizedPopupBox(frame)}
    >
      <div className="select-list">
        <OptionRows options={options} selected={selected} listbox={listbox} />
      </div>
    </div>
  );
}

// The option rows every popup draws, whichever box holds them.
function OptionRows({
  options,
  selected,
  listbox,
}: Readonly<{
  options: readonly SelectOption[];
  selected: (option: SelectOption) => boolean;
  listbox: Listbox;
}>) {
  return (
    <>
      {options.map((option, index) => (
        // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the combobox trigger's own keydown handling
        // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays on the combobox, which is what makes typeahead and Escape work
        <div // NOSONAR: keyboard path is the combobox's own keydown; an activedescendant option must not be focusable
          key={option.value}
          id={listbox.optionDomId(index)}
          role="option"
          aria-selected={selected(option)}
          aria-disabled={option.disabled}
          className={[
            "select-option",
            index === listbox.active ? "is-active" : "",
            option.disabled ? "is-disabled" : "",
          ]
            .filter(Boolean)
            .join(" ")}
          onClick={option.disabled ? undefined : () => listbox.pick(index)}
          // A press on an option must not steal focus from the combobox. The
          // multi list stays open after a toggle, so Escape and the arrows must
          // still reach the trigger's handler. On the rows, not the container,
          // so the list's scrollbar still drags.
          onMouseDown={(event) => event.preventDefault()}
          onMouseEnter={
            option.disabled ? undefined : () => listbox.hover(index)
          }
        >
          {option.adornment && (
            <span className="select-option-adornment" aria-hidden="true">
              {option.adornment}
            </span>
          )}
          <span className="select-option-label" lang={option.lang}>
            {option.label}
          </span>
          {selected(option) && (
            <Check className="select-option-check" size={14} aria-hidden />
          )}
        </div>
      ))}
    </>
  );
}

// The keys the filter box hands to the list. Letters, Space and the caret
// keys stay the box's, so a reader can type and correct a name.
const LIST_KEYS = new Set(["ArrowDown", "ArrowUp", "Enter", "Escape", "Tab"]);

// A long list: a box on top narrows it while focus sits in the box. The box
// drives the list through aria-activedescendant, as the trigger does.
function FilteredPopup({
  popupRef,
  options,
  selected,
  frame,
  listbox,
  animate,
  filter,
}: Readonly<{
  popupRef: Listbox["popup"];
  options: readonly SelectOption[];
  selected: (option: SelectOption) => boolean;
  frame: PopupFrame;
  listbox: Listbox;
  animate: boolean;
  filter: ListFilter;
}>) {
  const t = useT();
  const focusOnOpen = useCallback((box: HTMLInputElement | null) => {
    box?.focus({ preventScroll: true });
  }, []);
  const onKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (!LIST_KEYS.has(event.key)) {
      return;
    }
    // Tab moves on from the trigger, not from a box portalled to the page end.
    if (event.key === "Tab") {
      listbox.trigger.current?.focus();
    }
    listbox.onKeyDown(event);
  };
  const { active } = listbox;
  return (
    <div
      ref={popupRef}
      className="select-popup"
      data-motion={animate ? "in" : "none"}
      data-above={frame.above ? "true" : undefined}
      style={contentSizedPopupBox(frame)}
    >
      <input
        ref={focusOnOpen}
        type="text"
        className="input select-filter"
        role="combobox"
        aria-label={t("select.filter")}
        placeholder={t("select.filter")}
        aria-expanded="true"
        aria-autocomplete="list"
        aria-controls={listbox.listboxId}
        aria-activedescendant={
          active === -1 ? undefined : listbox.optionDomId(active)
        }
        autoComplete="off"
        value={filter.query}
        onChange={(event) => filter.onQuery(event.target.value)}
        onKeyDown={onKeyDown}
      />
      <div className="select-list">
        <div id={listbox.listboxId} role="listbox">
          <OptionRows options={options} selected={selected} listbox={listbox} />
        </div>
      </div>
      {options.length === 0 && (
        <p className="select-empty" role="status">
          {t("picker.noMatch")}
        </p>
      )}
    </div>
  );
}
