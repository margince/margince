// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronLeft, ChevronRight, PanelRight } from "lucide-react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { IconAction } from "../design-system/iconaction";
import { useT } from "../i18n";

/**
 * The page's own right-hand column: what stands AROUND the thing being read.
 *
 * It is a column of the WINDOW, not of the page's content — a sibling of the
 * nav rail and the work column, flush to the viewport edge and running the
 * full height past the page's own header. That placement is the whole point:
 * the context on an account is true of the account, not of whichever part of
 * it the reader has open, so it does not start below the header and it does
 * not move when a tab changes.
 *
 * A screen fills it by rendering `<PageAside>`; the shell owns the column, the
 * collapse and the toggle, because those belong to the window. A screen that
 * renders none leaves no column and no toggle behind — the region is absent
 * rather than empty, which is the same rule PageZones holds for its own rails.
 *
 * The collapse is remembered across routes and reloads. A reader who folds the
 * column away has said something about how they want to work, not about the
 * account they happened to be on.
 */

type PageAsideState = {
  // Whether a screen currently supplies content. The shell draws neither the
  // column nor its toggle without one.
  filled: boolean;
  setFilled: (filled: boolean) => void;
  collapsed: boolean;
  toggle: () => void;
  // Whether the window is too narrow to hold both columns at once. Below that
  // width the two are ONE region a reader switches, so the switch says "close"
  // rather than "hide" and the panel is shut on arrival.
  narrow: boolean;
  // The element a screen's content is portalled into. Null until the shell has
  // mounted, which is why `PageAside` renders nothing on its first pass.
  host: HTMLElement | null;
  setHost: (host: HTMLElement | null) => void;
};

const PageAsideContext = createContext<PageAsideState | null>(null);

const COLLAPSE_KEY = "margince.pageAside.collapsed";

function readCollapsed(): boolean {
  // A private window, cleared site data, or a browser refusing storage all
  // throw here rather than returning null. None of them is a reason to fail to
  // render a column, so the answer is the default and the reader simply does
  // not get their remembered choice.
  try {
    return window.localStorage.getItem(COLLAPSE_KEY) === "1";
  } catch {
    return false;
  }
}

/*
 * Below this width the window has room for one column, not two.
 *
 * The same figure the shell's own stylesheet folds the column at, and it is
 * duplicated here for one reason: the fold below the fold is not a width but a
 * BEHAVIOUR — one region at a time, shut on arrival — and behaviour is the
 * component's. `frontend/e2e/recordchrome.spec.ts` measures both sides at 900px
 * and 390px, so a figure changed on one side and not the other fails there
 * rather than drifting.
 */
const NARROW = "(max-width: 1100px)";

function useNarrowShell(): boolean {
  const [narrow, setNarrow] = useState(() => matches(NARROW));
  useEffect(() => {
    // Absent in some embedded contexts, and a missing media query is not a
    // reason to fail to render a shell — the same guard app/theme.ts takes for
    // `prefers-color-scheme`.
    if (typeof window.matchMedia !== "function") {
      return;
    }
    const media = window.matchMedia(NARROW);
    const read = () => setNarrow(media.matches);
    read();
    media.addEventListener("change", read);
    return () => media.removeEventListener("change", read);
  }, []);
  return narrow;
}

function matches(query: string): boolean {
  return (
    typeof window.matchMedia === "function" && window.matchMedia(query).matches
  );
}

export function PageAsideProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [filled, setFilled] = useState(false);
  const [host, setHost] = useState<HTMLElement | null>(null);
  const narrow = useNarrowShell();
  const [collapsed, setCollapsed] = useState(readCollapsed);
  // The narrow window's own answer, and it is deliberately NOT the remembered
  // one. Below the fold the panel takes the whole screen in place of the record,
  // so a reader who arrives with it open never sees what they opened; and a
  // phone-sized choice is not a statement about how they want to work at a desk,
  // which is what the stored preference means. Shut on arrival, per visit.
  const [narrowOpen, setNarrowOpen] = useState(false);
  const toggleWide = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      try {
        window.localStorage.setItem(COLLAPSE_KEY, next ? "1" : "0");
      } catch {
        // Storage refused the write. The toggle still works for this session;
        // only the memory of it is lost, and a column that would not fold
        // because a preference could not be saved is the worse failure.
      }
      return next;
    });
  }, []);
  const toggleNarrow = useCallback(() => {
    setNarrowOpen((current) => !current);
  }, []);
  return (
    <PageAsideContext.Provider
      value={{
        filled,
        setFilled,
        collapsed: narrow ? !narrowOpen : collapsed,
        toggle: narrow ? toggleNarrow : toggleWide,
        narrow,
        host,
        setHost,
      }}
    >
      {children}
    </PageAsideContext.Provider>
  );
}

// A screen rendered with no shell around it — the catalog, a screen test, an
// embedded surface — has no window to put a column in. That is not an error to
// throw at a reader: the column is the SHELL's affordance, so without one there
// is simply no column, and the screen renders exactly as it would with the
// panel folded away. Throwing here would make mounting a screen anywhere but
// inside the chrome a crash.
const NO_SHELL: PageAsideState = {
  filled: false,
  setFilled: () => undefined,
  collapsed: true,
  toggle: () => undefined,
  narrow: false,
  host: null,
  setHost: () => undefined,
};

function usePageAsideState(): PageAsideState {
  return useContext(PageAsideContext) ?? NO_SHELL;
}

/**
 * The column itself, rendered by the shell beside the work column.
 *
 * Collapsed it is a narrow full-height strip carrying only the control that
 * brings it back — a fold, not a disappearance, so a reader can always tell
 * that something is there and how to reach it.
 */
export function PageAsideRegion() {
  const t = useT();
  const { filled, collapsed, toggle, setHost } = usePageAsideState();
  // Folding is a width, and a width can only be tweened by an element that
  // survives the fold. Open and folded are therefore one `<aside>` wearing a
  // class rather than two elements swapped in and out, which is also what stops
  // a fold from unmounting the screen's cards and refetching them on the way
  // back.
  const fold = collapsed ? t("shell.aside.show") : t("shell.aside.hide");
  return (
    // Always mounted, because the host has to exist before a screen can portal
    // into it — `hidden` rather than absent is what keeps a screen with nothing
    // to show from leaving an empty landmark behind.
    <aside
      className={collapsed ? "pageaside collapsed" : "pageaside"}
      aria-label={t("record.context")}
      hidden={!filled}
    >
      <div className="pageaside-head">
        <span className="t-label pageaside-title">{t("record.context")}</span>
        <button
          type="button"
          className="pageaside-fold"
          onClick={toggle}
          title={fold}
          aria-label={fold}
          aria-expanded={!collapsed}
        >
          {collapsed ? (
            <ChevronLeft aria-hidden="true" />
          ) : (
            <ChevronRight aria-hidden="true" />
          )}
          <span className="pageaside-foldlabel">{t("shell.aside.hide")}</span>
        </button>
      </div>
      <div className="pageaside-body" ref={setHost} />
    </aside>
  );
}

/**
 * The control that folds the column away and brings it back, for a header to
 * carry.
 *
 * The column is a piece of the WINDOW, so the switch for it belongs with the
 * record's other verbs rather than only inside the panel it governs. A reader
 * who has folded the column away has no way back to it from the work column,
 * and hunting for a stub at the screen's edge is not a way back.
 *
 * Renders nothing when no screen supplies a column: a switch for a panel that
 * does not exist is a control that does nothing.
 */
export function PageAsideToggle() {
  const t = useT();
  const { filled, collapsed, toggle } = usePageAsideState();
  if (!filled) {
    return null;
  }
  // Its own short pair, not the column's. The stub at the screen's edge is a
  // glyph and needs a whole sentence to announce itself; this one sits among
  // the record's verbs and reads as one of them.
  const label = collapsed ? t("record.panel.show") : t("record.panel.hide");
  // The glyph carries it, through the catalog's own square: a panel icon says
  // which region this governs without the words, and the words were competing
  // with the record's own NAME for the header's width — on a tablet the name
  // truncated so that a verb could spell what its icon already showed.
  // `IconAction` is what keeps the sentence reachable for both readers at once,
  // one string spoken as the name and shown as the tip.
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

/**
 * A screen's content for the page column. Renders nothing where it stands.
 *
 * Absent children are not the same as no `PageAside`: a screen that mounts one
 * with nothing in it still claims the column, which is what a record whose
 * context is still loading needs — the column must not appear only once its
 * cards do, or the page reflows under a reader mid-read.
 */
export function PageAside({ children }: Readonly<{ children: ReactNode }>) {
  const { setFilled, host } = usePageAsideState();
  useEffect(() => {
    setFilled(true);
    return () => setFilled(false);
  }, [setFilled]);
  return host ? createPortal(children, host) : null;
}
