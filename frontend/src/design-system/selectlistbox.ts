// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import {
  type PopupFrame,
  useActiveOptionVisible,
  useAnchoredPopup,
  useDismissOnOutsidePress,
} from "./anchoredpopup";

/**
 * The open/active state machine under `Select` and `MultiSelect` — everything
 * a trigger-plus-portalled-listbox control decides that is not markup: whether
 * the list is open, which option is active, where the popup sits, and what a
 * keypress does about any of that. One machine, because one value and many
 * differ ONLY in what a pick does (`onPick`); a second copy would be a second
 * keyboard contract to keep true.
 *
 * Generic over the option: the machine reads `label` (typeahead) and
 * `disabled` (skipping), and hands the whole option back on a pick.
 */
export type ListboxOption = Readonly<{ label: string; disabled?: boolean }>;

// How long a typeahead buffer survives between keystrokes. Measured from the
// previous keystroke rather than reset by a timer: there is no timeout to cancel
// when the control unmounts mid-word, and nothing to fake in a test.
const TYPEAHEAD_RESET_MS = 500;

// The next option at or after `from` that a keyboard may land on, walking in
// `step`'s direction. Deliberately does not wrap: a list that jumps from its
// last entry back to its first hides from the reader that they reached the end.
function stepEnabled<T extends ListboxOption>(
  options: readonly T[],
  from: number,
  step: 1 | -1,
): number {
  for (let index = from; index >= 0 && index < options.length; index += step) {
    if (!options[index]?.disabled) {
      return index;
    }
  }
  return -1;
}

/**
 * What a keypress means to a combobox, as data and with no React in it.
 *
 * The whole keyboard contract lives here so it can be read in one screen and
 * argued with: the same key means different things open and closed, which is the
 * part every hand-rolled dropdown gets partly right.
 *
 * `null` is "not ours" — the press keeps its default, which is what lets Tab,
 * the browser's own shortcuts and a screen reader's keys through.
 */
type KeyIntent =
  | Readonly<{ act: "open"; step: 1 | -1 }>
  | Readonly<{ act: "move"; step: 1 | -1 }>
  | Readonly<{ act: "edge"; step: 1 | -1 }>
  | Readonly<{ act: "commit" }>
  | Readonly<{ act: "cancel" }>
  | Readonly<{ act: "leave" }>
  | Readonly<{ act: "search"; char: string }>
  | null;

function intentFor(key: string, open: boolean): KeyIntent {
  if (!open) {
    if (key === "ArrowUp") {
      return { act: "open", step: -1 };
    }
    // Typeahead on a CLOSED control is deliberately absent: a native select
    // changes its value when someone types "w" while tabbing past it, and a
    // stage that moved on a stray keystroke is a defect, not a shortcut.
    const opens = key === "ArrowDown" || key === "Enter" || key === " ";
    return opens ? { act: "open", step: 1 } : null;
  }
  switch (key) {
    case "ArrowDown":
      return { act: "move", step: 1 };
    case "ArrowUp":
      return { act: "move", step: -1 };
    case "Home":
      return { act: "edge", step: 1 };
    case "End":
      return { act: "edge", step: -1 };
    case "Enter":
    case " ":
      return { act: "commit" };
    case "Escape":
      return { act: "cancel" };
    case "Tab":
      return { act: "leave" };
    default:
      return key.length === 1 ? { act: "search", char: key } : null;
  }
}

// What each intent does. Named callbacks rather than a bag of setters, so the
// table in the hook below reads as the behaviour it is.
type IntentActions = Readonly<{
  openFrom: (step: 1 | -1) => void;
  moveBy: (step: 1 | -1) => void;
  toEdge: (step: 1 | -1) => void;
  commitActive: () => void;
  cancel: () => void;
  leave: () => void;
  search: (char: string) => void;
}>;

function keyDownHandler(
  open: boolean,
  actions: IntentActions,
  typing: () => boolean,
) {
  return (event: ReactKeyboardEvent<HTMLButtonElement>) => {
    // A modified press belongs to the browser or the OS (Alt+Arrow is history
    // navigation, Cmd+F is find) — never to a typeahead buffer.
    if (event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }
    const intent: KeyIntent =
      open && event.key === " " && typing()
        ? { act: "search", char: " " }
        : intentFor(event.key, open);
    if (!intent) {
      return;
    }
    // Tab keeps its default so focus can leave; every other press we claim is
    // ours, and scrolling the page on Space is never what was meant.
    //
    // Claimed also means it STOPS HERE. A dropdown is usually inside something
    // else that listens for the same keys on the document — `Modal` closes on
    // Escape, a form submits on Enter — and a press meant for the open list must
    // not also reach them: abandoning a dropdown would take the whole dialog
    // with it, and choosing an option would submit the form around it.
    if (intent.act !== "leave") {
      event.preventDefault();
      event.stopPropagation();
    }
    switch (intent.act) {
      case "open":
        return actions.openFrom(intent.step);
      case "move":
        return actions.moveBy(intent.step);
      case "edge":
        return actions.toEdge(intent.step);
      case "commit":
        return actions.commitActive();
      case "cancel":
        return actions.cancel();
      case "leave":
        return actions.leave();
      case "search":
        return actions.search(intent.char);
    }
  };
}

// The typeahead match, kept out of React: a buffer, the character just typed and
// the moment it arrived produce the next buffer and the option it points at.
function typeaheadMatch<T extends ListboxOption>(
  options: readonly T[],
  buffer: Readonly<{ query: string; at: number }>,
  char: string,
  now: number,
): Readonly<{ query: string; at: number; hit: number }> {
  const carried = now - buffer.at < TYPEAHEAD_RESET_MS;
  const query = (carried ? buffer.query : "") + char.toLowerCase();
  const hit = options.findIndex(
    (option) =>
      !option.disabled && option.label.toLowerCase().startsWith(query),
  );
  return { query, at: now, hit };
}

// Everything the trigger and the popup need from the state machine below.
export type Listbox = Readonly<{
  open: boolean;
  active: number;
  frame: PopupFrame | null;
  trigger: RefObject<HTMLButtonElement | null>;
  popup: RefObject<HTMLDivElement | null>;
  listboxId: string;
  optionDomId: (index: number) => string;
  onKeyDown: (event: ReactKeyboardEvent<HTMLButtonElement>) => void;
  onTriggerClick: () => void;
  pick: (index: number) => void;
  hover: (index: number) => void;
}>;

/**
 * The open/active state machine, one level below the markup.
 *
 * It owns four things and nothing else: whether the list is open, which option
 * is active, where the popup sits, and what a keypress does about any of that.
 * The commit is the only place `onChange` is called, so there is exactly one
 * path by which a value changes.
 */
// The option a fresh open (by click, arrow key or mount) lands active on: the
// current value if it holds one, else the edge the direction points at.
// Shared by `openFrom` and the openOnMount initializer so a Select that opens
// itself highlights the same option one opened by the reader would.
function startingActive<T extends ListboxOption>(
  options: readonly T[],
  selectedIndex: number,
  step: 1 | -1,
): number {
  if (selectedIndex !== -1 && !options[selectedIndex]?.disabled) {
    return selectedIndex;
  }
  return stepEnabled(options, step === 1 ? 0 : options.length - 1, step);
}

export function useSelectListbox<T extends ListboxOption>(
  options: readonly T[],
  // Where a fresh open lands: the single value's option, or a multi's first
  // chosen one. -1 means nothing chosen yet.
  selectedIndex: number,
  // What a pick does, and whether the list survives it: a single select
  // commits its one value and closes ("close"), a multi toggles membership
  // and stays open ("stay") so the next option is one click away. This is the
  // ONLY behavioural difference between the two controls, which is why it is
  // a parameter here rather than a second state machine.
  onPick: (option: T) => "close" | "stay",
  openOnMount: boolean,
  onCancel?: () => void,
  onLeave?: () => void,
): Listbox {
  const edge = (step: 1 | -1) =>
    stepEnabled(options, step === 1 ? 0 : options.length - 1, step);

  const [open, setOpen] = useState(openOnMount);
  const [active, setActive] = useState(() =>
    openOnMount ? startingActive(options, selectedIndex, 1) : -1,
  );
  const trigger = useRef<HTMLButtonElement | null>(null);
  const popup = useRef<HTMLDivElement | null>(null);
  const typed = useRef({ query: "", at: 0 });
  const listboxId = useId();

  // A caller mounting this already-open (InlineChoice, on the click that
  // started editing) mounts a TRIGGER THE CLICK NEVER LANDED ON — the DOM
  // node the reader actually pressed was the previous render's resting
  // button, gone by the time this one exists. Without this, Escape and the
  // arrow keys have nothing to reach: keyboard events go to whatever the
  // browser's default focus is, not to a listbox nobody told it opened.
  // biome-ignore lint/correctness/useExhaustiveDependencies: fires once, on mount — openOnMount names how this instance came to exist, not a value to keep reacting to on every later render.
  useEffect(() => {
    if (openOnMount) {
      trigger.current?.focus();
    }
  }, []);

  // The one place a close that picked nothing is told apart from one that
  // did: `commit` below never routes through this, because it closes on a
  // value that DID change.
  const abandon = useCallback(() => {
    setOpen(false);
    onCancel?.();
  }, [onCancel]);
  const frame = useAnchoredPopup(trigger, popup, open, abandon);
  useDismissOnOutsidePress(open, abandon, trigger, popup);
  useActiveOptionVisible(open, active, listboxId);

  const openFrom = (step: 1 | -1) => {
    typed.current = { query: "", at: 0 };
    setActive(startingActive(options, selectedIndex, step));
    setOpen(true);
  };

  const commit = (index: number) => {
    const option = options[index];
    if (!option || option.disabled) {
      // Nothing to commit — the list stays open on the reader's own choice
      // rather than closing as if something had been picked.
      return;
    }
    if (onPick(option) === "stay") {
      return;
    }
    setOpen(false);
    trigger.current?.focus();
  };

  const search = (char: string) => {
    const match = typeaheadMatch(options, typed.current, char, Date.now());
    typed.current = { query: match.query, at: match.at };
    if (match.hit !== -1) {
      setActive(match.hit);
    }
  };

  // What each intent actually does, as a table. It reads as the keyboard's
  // contract spelled a second way — `intentFor` says what a key means, this says
  // what happens — and keeping the two apart is what makes either readable.
  const actions: IntentActions = {
    openFrom,
    moveBy: (step) => {
      const from = active === -1 ? edge(step) : active + step;
      const next = stepEnabled(options, from, step);
      if (next !== -1) {
        setActive(next);
      }
    },
    toEdge: (step) => setActive(edge(step)),
    commitActive: () => commit(active),
    cancel: () => {
      trigger.current?.focus();
      abandon();
    },
    // Tab already moved focus forward — that is the browser's own default,
    // which the keydown handler above deliberately leaves unclaimed. Closing
    // through `abandon` would fire `onCancel`, and a caller that restores
    // focus on cancel (InlineChoice) would then yank it straight back to the
    // trigger the reader just left. `onLeave` tells that caller apart from a
    // real cancel so it knows not to.
    leave: () => {
      setOpen(false);
      onLeave?.();
    },
    search,
  };

  return {
    open,
    active,
    frame,
    trigger,
    popup,
    listboxId,
    optionDomId: (index: number) => `${listboxId}-option-${index}`,
    onKeyDown: keyDownHandler(
      open,
      actions,
      () =>
        typed.current.query !== "" &&
        Date.now() - typed.current.at < TYPEAHEAD_RESET_MS,
    ),
    // Pressing the trigger a second time closes on nothing chosen, which is
    // the same answer as Escape and as a press outside, so it leaves through
    // `abandon` like they do. Closed with `setOpen` alone it would be the one
    // dismissal a caller is never told about, and InlineChoice would sit in
    // its editing view with no list beneath it.
    onTriggerClick: () => (open ? abandon() : openFrom(1)),
    pick: commit,
    hover: setActive,
  };
}
