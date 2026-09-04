// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PanelRight } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { IconAction } from "../design-system/iconaction";
import { useT } from "../i18n";

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
};

const PageAsideContext = createContext<PageAsideState | null>(null);

const COLLAPSE_KEY = "margince.pageAside.collapsed";

function readCollapsed(): boolean {
  // Closed until asked: the details pane is where a reader goes for the
  // attributes and the short lists, not what they open a record to see, so a
  // reader who has never chosen finds it folded. A private window, cleared
  // site data, or a browser refusing storage all throw here rather than
  // returning null. None of them is a reason to fail to render a record, so
  // the answer is the default and the reader simply does not get their
  // remembered choice.
  try {
    return window.localStorage.getItem(COLLAPSE_KEY) !== "0";
  } catch {
    return true;
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
    <PageAsideContext.Provider value={{ filled, setFilled, collapsed, toggle }}>
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
};

function usePageAsideState(): PageAsideState {
  return useContext(PageAsideContext) ?? NO_SHELL;
}

/**
 * Claims the details pane for the screen calling it, and says whether the
 * pane is open — the one answer a screen needs to decide whether to hand
 * `RecordView` its aside.
 *
 * `available` is whether the screen has a pane to offer right now: a record
 * whose composer has taken the column's place passes false, and the switch
 * goes with the pane rather than standing beside a drawer it cannot open.
 */
export function usePageAside(available = true): { open: boolean } {
  const { filled, setFilled, collapsed } = usePageAsideState();
  useEffect(() => {
    setFilled(available);
    return () => setFilled(false);
  }, [available, setFilled]);
  return { open: filled && available && !collapsed };
}

/**
 * The control that folds the pane away and brings it back, for the tab row
 * to carry.
 *
 * It chooses what the page shows beside the work, so it stands with the
 * controls that choose what the work column shows, and never in the head
 * among the record's verbs. Renders nothing when no screen supplies a pane.
 */
export function PageAsideToggle() {
  const t = useT();
  const { filled, collapsed, toggle } = usePageAsideState();
  if (!filled) {
    return null;
  }
  const label = collapsed ? t("record.panel.show") : t("record.panel.hide");
  // The glyph carries it, through the catalog's own square: a panel icon says
  // which region this governs without the words, and the words were competing
  // with the tabs for the row's width. `IconAction` is what keeps the sentence
  // reachable for both readers at once, one string spoken as the name and
  // shown as the tip.
  return (
    <IconAction
      label={label}
      icon={<PanelRight aria-hidden="true" />}
      // The switch says which way it is set: folded away and standing open look
      // identical otherwise, and a reader cannot tell a control they have used
      // from one they have not.
      pressed={!collapsed}
      onClick={toggle}
    />
  );
}
