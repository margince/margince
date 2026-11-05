// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  Check,
  ChevronDown,
  ChevronRight,
  SlidersHorizontal,
} from "lucide-react";
import {
  type ReactNode,
  type RefObject,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
} from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { OptionCount } from "./atoms";
import { coveredByDialog } from "./dialogfocus";
import { MenuHead } from "./listmenu";
import { PopoverSheet } from "./listpopover";

export type ViewRow = Readonly<{
  key: string;
  label: string;
  /** What the dial holds now, read on the row before it is opened. */
  value: string;
  /** The step the row opens; `back` returns to the panel's rows. */
  step: (back: () => void) => ReactNode;
  /** The step draws its own `ViewStep`, because it has a level of its own. */
  ownHead?: boolean;
  /** The dial is off its default, which the row marks with a dot. */
  changed?: boolean;
}>;

/** A dial with two states, drawn as a pressed row in the panel's end group. */
export type ViewToggle = Readonly<{
  key: string;
  label: string;
  checked: boolean;
  onChange: (next: boolean) => void;
}>;

/** Every dial of a list behind one View button. A row opens its dial's step in
 * place of the rows, so the panel never holds a second panel. */
export function ListViewMenu({
  open,
  onOpenChange,
  filterCount,
  changed,
  layout,
  rows,
  toggles = [],
  footer,
}: Readonly<{
  open: boolean;
  onOpenChange: (open: boolean) => void;
  filterCount: number;
  /** The list departs from every view it offers. */
  changed: boolean;
  /** How the body is drawn (Table or Board), above the rows. */
  layout?: ReactNode;
  rows: readonly ViewRow[];
  toggles?: readonly ViewToggle[];
  footer?: ReactNode;
}>) {
  const t = useT();
  const [stepKey, setStepKey] = useState<string | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const anchor = useRef<HTMLSpanElement>(null);
  useCloseOnFocusOut(open, anchor, body, () => onOpenChange(false));
  // A closed panel reopens on its rows, never on the step it was left at.
  if (!open && stepKey !== null) {
    setStepKey(null);
  }
  const arrive = useStepFocus(body, stepKey, "data-view-row", open);

  const row = rows.find((candidate) => candidate.key === stepKey);
  const back = () => setStepKey(null);
  const enter = (key: string) => {
    arrive(key);
    setStepKey(key);
  };

  return (
    <span className="lt-menu-wrap" ref={anchor}>
      <PopoverSheet
        label={<ViewLabel filterCount={filterCount} changed={changed} />}
        title={t("list.viewMenu")}
        variant="ghost"
        className="lt-vtrigger"
        align="end"
        panelClassName="lt-vpopover"
        open={open}
        onOpenChange={onOpenChange}
      >
        <div className="lt-vpanel" ref={body}>
          {row ? (
            row.ownHead ? (
              row.step(back)
            ) : (
              <ViewStep
                head={row.label}
                back={{ label: t("list.viewMenuBack"), onBack: back }}
              >
                {row.step(back)}
              </ViewStep>
            )
          ) : (
            <>
              {layout && <div className="lt-vlayout">{layout}</div>}
              <div className="lt-vrows">
                {rows.map((dial) => (
                  <DialRow key={dial.key} dial={dial} onEnter={enter} />
                ))}
              </div>
              {toggles.length > 0 && (
                <div className="lt-vgroup">
                  {toggles.map((toggle) => (
                    <ToggleRow key={toggle.key} toggle={toggle} />
                  ))}
                </div>
              )}
              {footer && <div className="lt-vfoot">{footer}</div>}
            </>
          )}
        </div>
      </PopoverSheet>
    </span>
  );
}

function DialRow({
  dial,
  onEnter,
}: Readonly<{ dial: ViewRow; onEnter: (key: string) => void }>) {
  return (
    <ValueRow
      label={dial.label}
      value={dial.value}
      changed={dial.changed}
      data-view-row={dial.key}
      onClick={() => onEnter(dial.key)}
    />
  );
}

/** A row that opens a step: its name, what it holds now, and a dot while that
 * is off its default. */
export function ValueRow({
  label,
  value,
  changed = false,
  onClick,
  ...marker
}: Readonly<{
  label: string;
  value?: string;
  changed?: boolean;
  onClick: () => void;
  [data: `data-${string}`]: string;
}>) {
  const t = useT();
  const name = [label, value, changed ? t("list.viewMenuChanged") : undefined]
    .filter((part) => part !== undefined && part !== "")
    .join(", ");
  return (
    <button
      type="button"
      className="lt-mi lt-vrow"
      {...marker}
      // The visible words in order, with commas between the name and value.
      aria-label={name}
      onClick={onClick}
    >
      <span className="lt-vrow-label">
        {label}
        {changed && <span className="lt-vdot" />}
      </span>
      {value && <span className="lt-vrow-value">{value}</span>}
      <ChevronRight
        strokeWidth={1.8}
        aria-hidden="true"
        className="lt-caret lt-mi-more"
      />
    </button>
  );
}

export function ViewStep({
  head,
  back,
  children,
}: Readonly<{
  head: string;
  back: { label: string; onBack: () => void };
  children: ReactNode;
}>) {
  return (
    <fieldset className="lt-vstep" aria-label={head}>
      <MenuHead head={head} back={back} />
      {children}
    </fieldset>
  );
}

/**
 * Focus for a level whose rows give way to a step. Entering moves it to the
 * step's Back, and leaving returns it to the row that opened the step.
 * Returns what a row's press calls before it opens its step.
 */
export function useStepFocus(
  root: RefObject<HTMLElement | null>,
  step: string | null,
  rowAttribute: `data-${string}`,
  shown = true,
): (key: string) => void {
  // Null until a row is pressed, so a level drawn fresh leaves focus alone.
  const cameFrom = useRef<string | null>(null);
  useEffect(() => {
    const key = cameFrom.current;
    if (key === null) {
      return;
    }
    const scope = root.current;
    if (shown && step !== null) {
      scope?.querySelector<HTMLElement>(".lt-mback")?.focus();
      return;
    }
    cameFrom.current = null;
    if (shown) {
      [...(scope?.querySelectorAll<HTMLElement>(`[${rowAttribute}]`) ?? [])]
        .find((row) => row.getAttribute(rowAttribute) === key)
        ?.focus();
    }
  }, [root, step, rowAttribute, shown]);
  return (key) => {
    cameFrom.current = key;
  };
}

/**
 * Tab past the panel's last control would leave the popover open over a page
 * the reader has left. So focus landing outside it closes it.
 */
function useCloseOnFocusOut(
  open: boolean,
  anchor: RefObject<HTMLElement | null>,
  body: RefObject<HTMLElement | null>,
  close: () => void,
) {
  const leave = useEffectEvent(close);
  useEffect(() => {
    if (!open) {
      return;
    }
    const onFocus = (event: FocusEvent) => {
      const target = event.target;
      if (
        !(target instanceof Node) ||
        anchor.current?.contains(target) ||
        body.current?.contains(target) ||
        // A dialog the panel raised (a name prompt, a calendar) is the panel's.
        coveredByDialog(anchor.current)
      ) {
        return;
      }
      leave();
    };
    document.addEventListener("focusin", onFocus);
    return () => document.removeEventListener("focusin", onFocus);
  }, [open, anchor, body]);
}

/** A filter over a list is a pressed button (README), in the rows' shape;
 * a list with no panel draws it alone. */
export function ToggleRow({ toggle }: Readonly<{ toggle: ViewToggle }>) {
  const t = useT();
  return (
    <button
      type="button"
      className="lt-mi lt-vrow lt-vtoggle"
      aria-pressed={toggle.checked}
      onClick={() => toggle.onChange(!toggle.checked)}
    >
      <span className="lt-vrow-label">{toggle.label}</span>
      {/* The pressed state already says this to a screen reader. */}
      <span className="lt-vrow-value" aria-hidden="true">
        {toggle.checked ? t("list.viewToggleOn") : t("list.viewToggleOff")}
      </span>
      <Check
        strokeWidth={1.8}
        aria-hidden="true"
        className={`lt-caret lt-mi-more${toggle.checked ? "" : " lt-vcheck-off"}`}
      />
    </button>
  );
}

/** The trigger's face: the count and dot are hidden from a screen reader,
 * and one hidden line names it "View, 2 filters, changed". */
function ViewLabel({
  filterCount,
  changed,
}: Readonly<{ filterCount: number; changed: boolean }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const name = [
    t("list.viewMenu"),
    filterCount > 0
      ? plural("list.viewMenuFilters", filterCount, {
          count: formatNumber(filterCount, locale),
        })
      : null,
    changed ? t("list.viewMenuChanged") : null,
  ]
    .filter((part) => part !== null)
    .join(", ");
  return (
    <>
      <SlidersHorizontal strokeWidth={1.5} aria-hidden="true" />
      <span className="lt-vface" aria-hidden="true">
        {t("list.viewMenu")}
        {filterCount > 0 && <OptionCount count={filterCount} />}
        {changed && <span className="lt-vdot" />}
      </span>
      <span className="sr-only">{name}</span>
      <ChevronDown className="lt-caret" aria-hidden="true" />
    </>
  );
}
