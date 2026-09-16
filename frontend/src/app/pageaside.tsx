// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PanelRight } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { Button, OptionCount } from "../design-system/atoms";
import { useT } from "../i18n";
import { useHasUnsavedChanges } from "./unsaved";

/**
 * The record's details pane: what stands AROUND the thing being read.
 *
 * It is a column of the RECORD's body, beside the work under the tab row
 * (DESIGN.md §6): one pane at 300px, drawn by `RecordView`'s aside slot, that
 * folds away to leave the work the whole width. The shell owns nothing of it
 * but the memory — whether this reader keeps it open is a statement about how
 * they want to work, not about the record they happened to be on, so it is
 * remembered across routes and reloads.
 *
 * A screen with such a pane claims it through `usePageAside`, which answers
 * whether the pane is open; the screen passes its content to `RecordView` only
 * then, so a closed pane leaves no empty column and no empty landmark behind.
 * `PageAsideToggle` is the switch, for the tab row to carry.
 */

type PageAsideState = {
  // Whether the screen on the page has a pane to show. The toggle draws only
  // then: a switch for a pane that does not exist is a control that does
  // nothing.
  filled: boolean;
  setFilled: (filled: boolean) => void;
  collapsed: boolean;
  toggle: () => void;
  focusField: string | null;
  setFocusField: (field: string | null) => void;
};

const PageAsideContext = createContext<PageAsideState | null>(null);

const COLLAPSE_KEY = "margince.pageAside.collapsed";

function readCollapsed(): boolean {
  // Open until folded: the pane holds the record's own facts, and a reader
  // who has never chosen came for the whole record. Only a remembered fold
  // closes it. A private window, cleared site data, or a browser refusing
  // storage all throw here rather than returning null. None of them is a
  // reason to fail to render a record, so the answer is the default and the
  // reader simply does not get their remembered choice.
  try {
    return window.localStorage.getItem(COLLAPSE_KEY) === "1";
  } catch {
    return false;
  }
}

export function PageAsideProvider({
  children,
  open,
}: Readonly<{
  children: ReactNode;
  // Starts the pane open whatever is remembered — for a surface with no reader
  // whose choice there is to keep: the catalog, and a record suite reading the
  // pane's cards. The chrome passes nothing and reads the memory.
  open?: boolean;
}>) {
  const [filled, setFilled] = useState(false);
  const [focusField, setFocusField] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useState(() =>
    open === undefined ? readCollapsed() : !open,
  );
  const toggle = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      try {
        window.localStorage.setItem(COLLAPSE_KEY, next ? "1" : "0");
      } catch {
        // Storage refused the write. The toggle still works for this session;
        // only the memory of it is lost, and a pane that would not fold
        // because a preference could not be saved is the worse failure.
      }
      return next;
    });
  }, []);
  return (
    <PageAsideContext.Provider
      value={{
        filled,
        setFilled,
        collapsed,
        toggle,
        focusField,
        setFocusField,
      }}
    >
      {children}
    </PageAsideContext.Provider>
  );
}

// A screen rendered with no shell around it — the catalog, a screen test, an
// embedded surface — has no memory to read the pane's state from. That is not
// an error to throw at a reader: the screen renders exactly as it would with
// the pane folded away, and the switch is simply absent. Throwing here would
// make mounting a screen anywhere but inside the chrome a crash.
const NO_SHELL: PageAsideState = {
  filled: false,
  setFilled: () => undefined,
  collapsed: true,
  toggle: () => undefined,
  focusField: null,
  setFocusField: () => undefined,
};

function usePageAsideState(): PageAsideState {
  return useContext(PageAsideContext) ?? NO_SHELL;
}

export function useShowDetails(field: string): (() => void) | undefined {
  const state = useContext(PageAsideContext);
  if (!state?.filled) return undefined;
  return () => {
    if (state.collapsed) state.toggle();
    state.setFocusField(field);
  };
}

export function useDetailsFieldTarget(field: string) {
  const state = useContext(PageAsideContext);
  const target = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    if (state?.focusField !== field || !target.current) return;
    target.current.scrollIntoView?.({ block: "center" });
    const control = target.current.querySelector<HTMLElement>(
      "input,textarea,button",
    );
    (control ?? target.current).focus();
    state.setFocusField(null);
  }, [state, field]);
  return target;
}

/**
 * Claims the details pane for the screen calling it, and says whether the
 * pane is open — the one answer a screen needs to decide whether to hand
 * `RecordView` its aside.
 *
 * Claiming it is mounting: a screen has a pane to offer for as long as it is
 * on the page, and nothing else takes the pane away from it. An overlay does
 * not — a drawer is portalled over a scrim and takes none of the page's
 * width, so folding the column beneath one would animate the record behind
 * its own backdrop and leave the pane shut once it closed.
 */
export function usePageAside(): { open: boolean } {
  const { filled, setFilled, collapsed } = usePageAsideState();
  useEffect(() => {
    setFilled(true);
    return () => setFilled(false);
  }, [setFilled]);
  return { open: filled && !collapsed };
}

/**
 * The control that folds the pane away and brings it back, for the tab row
 * to carry.
 *
 * It chooses what the page shows beside the work, so it stands with the
 * controls that choose what the work column shows rather than in the head
 * among the record's verbs, where it reads as one more thing to do to the
 * record instead of a way to see more of it. Every record page passes it as
 * its tab strip's `trailing`, and a record with a single body draws the strip
 * anyway: the row is where the switch lives, and a reader who has learned that
 * finds it in the same place on every record. Renders nothing when no screen
 * supplies a pane.
 */
export function PageAsideToggle({
  labels,
  quiet = false,
  prominent = false,
  controlled,
}: Readonly<{
  // What the switch says in each state, naming what the pane holds. The
  // default is the record's details pane; a page whose pane holds more names
  // it in both verbs.
  labels?: PaneWords;
  // Drawn as a link in the row rather than as a boxed control: for a strip
  // whose other end is a row of tabs, a box there reads as one more verb.
  quiet?: boolean;
  // Drawn as the page's PRIMARY control: for the one page whose pane is the
  // whole queue behind the day, where the switch is the main way on and not a
  // detail fold beside a row of tabs.
  prominent?: boolean;
  controlled?: {
    open: boolean;
    labels: PaneWords;
    onToggle: () => void;
    // How much is behind the pane, beside the verb — the queue's own total,
    // so a reader knows what the switch opens before pressing it.
    count?: number;
  };
}> = {}) {
  const t = useT();
  const dirty = useHasUnsavedChanges("details");
  const { filled, collapsed, toggle } = usePageAsideState();
  if (!filled && !controlled) {
    return null;
  }
  // Named for what pressing it DOES, not a bare glyph: this control ends a
  // row of words, and a lone square at the end of a tab strip reads as chrome
  // rather than as the way to the record's own details. "Show details" while
  // folded and "Hide details" while open, because the two states look
  // identical from the button alone; `aria-pressed` carries the same answer
  // to a screen reader.
  const open = controlled?.open ?? !collapsed;
  const words = controlled?.labels ??
    labels ?? {
      show: t("record.panel.showDetails"),
      hide: t("record.panel.hideDetails"),
    };
  return (
    <Button
      className="record-details-toggle"
      variant={quiet ? "link" : prominent ? "primary" : undefined}
      reason={open && dirty ? t("record.finishFieldEdit") : undefined}
      aria-pressed={open}
      onClick={controlled?.onToggle ?? toggle}
    >
      <PanelRight aria-hidden="true" />
      {open ? words.hide : words.show}
      {controlled?.count !== undefined && (
        <OptionCount
          count={controlled.count}
          className="record-details-toggle-count"
        />
      )}
    </Button>
  );
}

/** The two things the switch can say: the verb that opens the pane and the
 *  verb that folds it, each naming what the pane holds. */
export type PaneWords = Readonly<{ show: string; hide: string }>;
