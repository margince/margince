// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronLeft, PanelRight } from "lucide-react";
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
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
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
 * whether the pane is open. The screen hands `RecordView` its content only
 * then, so a closed pane leaves no empty landmark behind.
 * `PageAsideToggle` is the switch: on the pane itself (`pane`), or in a tab
 * row for a page whose pane is not a record's.
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

function readCollapsed(): boolean {
  // Open until folded: the pane holds the record's own facts, and a reader
  // who has never chosen came for the whole record.
  return readStored(STORAGE_KEYS.pageAsideCollapsed) === "1";
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
      writeStored(STORAGE_KEYS.pageAsideCollapsed, next ? "1" : "0");
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

/** The field a reader was just sent to in the details pane, if any. */
export function useDetailsFocusField(): string | null {
  return useContext(PageAsideContext)?.focusField ?? null;
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
 * The control that folds the pane away and brings it back. A record passes it
 * to `RecordView` as `asideToggle` with `pane`, so it sits on the pane it
 * folds; elsewhere it ends a tab row. Renders nothing when no screen supplies
 * a pane.
 */
export function PageAsideToggle({
  labels,
  quiet = false,
  prominent = false,
  pane = false,
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
  // Drawn on the pane itself (RecordView's `asideToggle`): a quiet "hide" in
  // its head while open, and a narrow strip naming the pane while folded.
  pane?: boolean;
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
  if (pane) {
    return (
      <PaneToggle
        open={open}
        words={words}
        dirty={dirty}
        onToggle={controlled?.onToggle ?? toggle}
      />
    );
  }
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
        <OptionCount count={controlled.count} />
      )}
    </Button>
  );
}

/** The two things the switch can say: the verb that opens the pane and the
 *  verb that folds it, each naming what the pane holds. */
export type PaneWords = Readonly<{ show: string; hide: string }>;

// The switch as the pane draws it. Open, it is a quiet "hide" in the head;
// folded, it is a strip with the pane's name set on its side.
function PaneToggle({
  open,
  words,
  dirty,
  onToggle,
}: Readonly<{
  open: boolean;
  words: PaneWords;
  dirty: boolean;
  onToggle: () => void;
}>) {
  const t = useT();
  if (open) {
    return (
      <Button
        className="record-details-toggle"
        variant="link"
        reason={dirty ? t("record.finishFieldEdit") : undefined}
        aria-pressed
        aria-label={words.hide}
        onClick={onToggle}
      >
        {t("record.panel.hide")}
      </Button>
    );
  }
  return (
    <Button
      className="record-details-fold"
      variant="ghost"
      aria-pressed={false}
      aria-label={words.show}
      onClick={onToggle}
    >
      <ChevronLeft aria-hidden="true" />
      <span className="record-details-fold-label">
        {t("record.panel.fold")}
      </span>
    </Button>
  );
}
