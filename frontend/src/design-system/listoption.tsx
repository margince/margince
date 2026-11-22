// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode, Ref } from "react";

// The two halves of a listbox a text box drives while it keeps focus. One is
// what the box says about the list, the other is one option of it.

/** What a text box that drives a listbox spreads onto itself. */
export type ComboboxAria = Readonly<{
  role: "combobox";
  "aria-expanded": boolean;
  "aria-haspopup": "listbox";
  // Never "both": inline completion rewrites what a reader is halfway through
  // typing, and the one thing these controls promise is that they do not.
  "aria-autocomplete": "list";
  "aria-controls": string | undefined;
  "aria-activedescendant": string | undefined;
}>;

/**
 * The combobox wiring of a text box over the listbox `listboxId`. A closed
 * list is controlled by nothing and has no active row, so both point nowhere.
 */
export function comboboxAria(
  listboxId: string,
  expanded: boolean,
  activeOptionId: string | undefined,
): ComboboxAria {
  return {
    role: "combobox",
    "aria-expanded": expanded,
    "aria-haspopup": "listbox",
    "aria-autocomplete": "list",
    "aria-controls": expanded ? listboxId : undefined,
    "aria-activedescendant": expanded ? activeOptionId : undefined,
  };
}

export type ListOptionProps = Readonly<{
  id: string;
  selected: boolean;
  disabled?: boolean;
  busy?: boolean;
  className: string;
  onPick: () => void;
  onHover?: () => void;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}>;

/**
 * An option of a listbox whose text box keeps focus and names the active row
 * with `aria-activedescendant`. The face is the caller's: `SuggestOption` is
 * the suggestion row, and the `⌘K` palette draws its own.
 */
export function ListOption({
  id,
  selected,
  disabled,
  busy,
  className,
  onPick,
  onHover,
  ref,
  children,
}: ListOptionProps) {
  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the driving text box's own keydown handling
    // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays in the text box, which is what keeps typing working
    <div // NOSONAR: keyboard path is the text box's own keydown; an activedescendant option must not be focusable
      ref={ref}
      id={id}
      role="option"
      aria-selected={selected}
      aria-disabled={disabled === true || undefined}
      aria-busy={busy === true || undefined}
      className={className}
      onMouseDown={(event) => {
        // The press must not take focus off the text box before the click
        // lands. Blur closes the list, and the click would then land nowhere.
        event.preventDefault();
      }}
      onClick={disabled ? undefined : onPick}
      onMouseEnter={disabled ? undefined : onHover}
    >
      {children}
    </div>
  );
}
