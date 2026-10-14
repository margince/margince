// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import {
  createContext,
  type MouseEvent,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { type FoldEnds, useFold } from "./fold";
import "./atoms.css";

/** A section the reader opens when they want it: a one-off tool, configuration or a deep read, kept to one line until asked for. */
export function Disclosure({
  summary,
  action,
  open,
  onToggle,
  className,
  children,
}: Readonly<{
  /** A row, so it may carry a count or a status chip beside the name. */
  summary: ReactNode;
  /** A section-level action stays outside summary to avoid nested controls and
   * remains visible while the section is closed. */
  action?: ReactNode;
  /** With `onToggle`, the section shows `open` and a press asks `onToggle`. Alone,
   * a change of `open` opens or shuts it, and the reader may still toggle it. */
  open?: boolean;
  /** Every change after mount; with `open`, the request for one. */
  onToggle?: (open: boolean) => void;
  className?: string;
  children: ReactNode;
}>) {
  const bodyId = useId();
  const root = useRef<HTMLDetailsElement>(null);
  const expanded = useExpanded(bodyId, open, onToggle);
  const shown = useFold(root, expanded.value, disclosureEnds);
  // Not uncontrolled `<details>`: it hides the body the instant it closes, so
  // the close could not move. It stays `open` until the fold has ended.
  const details = (
    <details
      ref={root}
      className={className ? `disclosure ${className}` : "disclosure"}
      open={shown}
      onToggle={(event) => {
        // Find-in-page opens a shut section itself; the state follows it.
        if (event.currentTarget.open && !shown) {
          expanded.ask(true);
        }
      }}
    >
      {/* biome-ignore lint/a11y/noStaticElementInteractions: a summary is the details' own toggle, which the rule does not count as interactive */}
      {/* biome-ignore lint/a11y/useAriaPropsSupportedByRole: `open` stays set through the close, so the summary says the state the reader asked for */}
      <summary
        className="disclosure-summary"
        aria-expanded={expanded.value}
        aria-controls={bodyId}
        onClick={(event) => {
          const press = pressOn(event);
          if (press === "control") {
            return;
          }
          event.preventDefault();
          if (press === "summary") {
            expanded.ask(!expanded.value);
          }
        }}
      >
        <span className="disclosure-label">{summary}</span>
        <ExpanderChevron />
      </summary>
      {/* Inert through the close, so Tab and a screen reader agree with the summary. */}
      <div
        className="disclosure-body"
        id={bodyId}
        inert={shown && !expanded.value}
      >
        <GroupContext.Provider value={null}>{children}</GroupContext.Provider>
      </div>
    </details>
  );
  if (!action) {
    return details;
  }
  return (
    <div className="disclosure-wrap">
      {details}
      <span className="disclosure-action">{action}</span>
    </div>
  );
}

type Group = Readonly<{
  open: string | null;
  /** False when `hint` is set and another section already holds the group. */
  claim: (id: string, shut: () => void, hint: boolean) => boolean;
  release: (id: string) => void;
}>;

const GroupContext = createContext<Group | null>(null);

/**
 * The `Disclosure`s inside open one at a time: opening one closes the open one.
 * They may sit in any component under it. A disclosure's body is outside the
 * group, so a section nested in an open one never closes it.
 */
export function DisclosureGroup({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [open, setOpen] = useState<string | null>(null);
  const holder = useRef<Readonly<{ id: string; shut: () => void }> | null>(
    null,
  );
  const claim = useCallback((id: string, shut: () => void, hint: boolean) => {
    const held = holder.current;
    if (held !== null && held.id !== id) {
      if (hint) {
        return false;
      }
      held.shut();
    }
    holder.current = { id, shut };
    setOpen(id);
    return true;
  }, []);
  const release = useCallback((id: string) => {
    if (holder.current?.id === id) {
      holder.current = null;
      setOpen(null);
    }
  }, []);
  const group = useMemo(
    () => ({ open, claim, release }),
    [open, claim, release],
  );
  return (
    <GroupContext.Provider value={group}>{children}</GroupContext.Provider>
  );
}

/** The one turning chevron: down while shut, up once the `aria-expanded` control around it opens. */
export function ExpanderChevron() {
  return <ChevronDown aria-hidden="true" className="expander-chevron" />;
}

function useExpanded(
  id: string,
  open: boolean | undefined,
  onToggle: ((open: boolean) => void) | undefined,
) {
  const group = useContext(GroupContext);
  const controlled = open !== undefined && onToggle !== undefined;
  // In a group, `open` at mount is a hint: a section already open keeps it.
  const [own, setOwn] = useState(
    () => (open ?? false) && (group?.open ?? null) === null,
  );
  const [given, setGiven] = useState(open);
  if (given !== open) {
    setGiven(open);
    setOwn(open ?? false);
  }
  const value = open !== undefined && onToggle !== undefined ? open : own;
  const ask = useRef((wanted: boolean) => setOwn(wanted));
  useLayoutEffect(() => {
    ask.current = open !== undefined && onToggle ? onToggle : setOwn;
  }, [open, onToggle]);
  const [initial] = useState(value);
  const moved = useRef(false);
  useLayoutEffect(() => {
    moved.current ||= value !== initial;
    if (group === null || !value) {
      return;
    }
    const shut = () => ask.current(false);
    if (!group.claim(id, shut, !moved.current)) {
      shut();
    }
    return () => group.release(id);
  }, [group, id, initial, value]);
  const told = useRef(value);
  useEffect(() => {
    if (told.current === value) {
      return;
    }
    told.current = value;
    if (!controlled) {
      onToggle?.(value);
    }
  }, [controlled, onToggle, value]);
  return { value, ask: (wanted: boolean) => ask.current(wanted) };
}

// A control with its own activation keeps the press. A field takes none, as a
// summary toggling under a caret would.
const ACTIVATES =
  'a[href], button, label, input[type="checkbox"], input[type="radio"], input[type="button"], input[type="submit"], [role="button"]';
const FIELD = "input, select, textarea";

function pressOn(
  event: MouseEvent<HTMLElement>,
): "summary" | "control" | "field" {
  const target = event.target;
  if (!(target instanceof Element)) {
    return "summary";
  }
  const summary = event.currentTarget;
  const within = (found: Element | null) =>
    found !== null && found !== summary && summary.contains(found);
  if (within(target.closest(ACTIVATES))) {
    return "control";
  }
  return within(target.closest(FIELD)) ? "field" : "summary";
}

const disclosureEnds: FoldEnds = (box) => {
  const top = box.getBoundingClientRect().top;
  const style = getComputedStyle(box);
  const tail =
    Number.parseFloat(style.paddingBottom) +
    Number.parseFloat(style.borderBottomWidth);
  const summary = box.querySelector(":scope > .disclosure-summary");
  const body = box.querySelector(":scope > .disclosure-body");
  const shut = (summary?.getBoundingClientRect().bottom ?? top) - top + tail;
  if (body === null) {
    return { shut, open: shut };
  }
  const margin = Number.parseFloat(getComputedStyle(body).marginBottom);
  return {
    shut,
    open: body.getBoundingClientRect().bottom + margin - top + tail,
  };
};
