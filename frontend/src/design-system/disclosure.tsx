// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import {
  createContext,
  type MouseEvent,
  type ReactNode,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
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
  /** Forces it open for a state the reader must not miss; undefined leaves it to the reader. */
  open?: boolean;
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
          expanded.set(true);
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
          if (ownPress(event)) {
            event.preventDefault();
            expanded.set(!expanded.value);
          }
        }}
      >
        <span className="disclosure-label">{summary}</span>
        <ExpanderChevron />
      </summary>
      <div className="disclosure-body" id={bodyId}>
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

type Group = Readonly<{ open: string | null; claim: (id: string) => void }>;

const GroupContext = createContext<Group | null>(null);

/**
 * The `Disclosure`s inside open one at a time: opening one closes the open one.
 * They may sit in any component under it. A disclosure's body is outside the
 * group, so a section nested in an open one never closes it.
 */
export function DisclosureGroup({
  children,
}: Readonly<{ children: ReactNode }>) {
  const [open, claim] = useState<string | null>(null);
  return (
    <GroupContext.Provider value={{ open, claim }}>
      {children}
    </GroupContext.Provider>
  );
}

/** The one turning chevron: down while shut, up once the `aria-expanded` control around it opens. */
export function ExpanderChevron() {
  return <ChevronDown aria-hidden="true" className="expander-chevron" />;
}

// Open from the reader, the caller's `open`, or the group, in that order of
// arrival. A change of `open` wins over the group: a forced section claims it.
function useExpanded(
  id: string,
  open: boolean | undefined,
  onToggle: ((open: boolean) => void) | undefined,
) {
  const group = useContext(GroupContext);
  const groupOpen = group?.open ?? null;
  const [value, setValue] = useState(open ?? false);
  const [given, setGiven] = useState(open);
  const [heard, setHeard] = useState(groupOpen);
  let next = value;
  if (heard !== groupOpen) {
    setHeard(groupOpen);
    if (groupOpen !== null && groupOpen !== id) {
      next = false;
    }
  }
  if (given !== open) {
    setGiven(open);
    next = open ?? false;
  }
  if (next !== value) {
    setValue(next);
  }
  const claim = group?.claim;
  useLayoutEffect(() => {
    if (value) {
      claim?.(id);
    }
  }, [claim, id, value]);
  // Every change reaches `onToggle`, the caller's and the group's as well as
  // the reader's, as the native `toggle` event does.
  const told = useRef(false);
  useEffect(() => {
    if (told.current !== value) {
      told.current = value;
      onToggle?.(value);
    }
  }, [onToggle, value]);
  return { value, set: setValue };
}

// A control inside the summary keeps its own press.
function ownPress(event: MouseEvent<HTMLElement>): boolean {
  const target = event.target;
  if (!(target instanceof Element)) {
    return true;
  }
  const control = target.closest("a, button, input, select, textarea, label");
  return control === null || !event.currentTarget.contains(control);
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
