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

/** Every dial of a list behind one View button. A row opens its dial's step in
 * place of the rows, so the panel never holds a second panel. */
export function ListViewMenu({
  open,
  onOpenChange,
  filterCount,
  changed,
  layout,
  rows,
  archived,
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
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  footer?: ReactNode;
}>) {
  const t = useT();
  const [stepKey, setStepKey] = useState<string | null>(null);
  const cameFrom = useRef<string | null>(null);
  const body = useRef<HTMLDivElement>(null);
  const anchor = useRef<HTMLSpanElement>(null);
  useCloseOnFocusOut(open, anchor, body, () => onOpenChange(false));
  // A closed panel reopens on its rows, never on the step it was left at.
  if (!open && stepKey !== null) {
    setStepKey(null);
  }

  // A step replaces the pressed control: focus goes to its way back, and on
  // Back to the row that opened it. A close leaves focus to the trigger's owner.
  useEffect(() => {
    const key = cameFrom.current;
    if (key === null) {
      return;
    }
    const root = body.current;
    if (open && stepKey !== null) {
      root?.querySelector<HTMLElement>(".lt-mback")?.focus();
      return;
    }
    cameFrom.current = null;
    if (open) {
      [...(root?.querySelectorAll<HTMLElement>("[data-view-row]") ?? [])]
        .find((row) => row.dataset.viewRow === key)
        ?.focus();
    }
  }, [open, stepKey]);

  const row = rows.find((candidate) => candidate.key === stepKey);
  const back = () => setStepKey(null);
  const enter = (key: string) => {
    cameFrom.current = key;
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
              {archived && <ArchivedRow archived={archived} />}
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
  const t = useT();
  const name = [
    dial.label,
    dial.value,
    dial.changed ? t("list.viewMenuChanged") : null,
  ]
    .filter((part) => part !== null)
    .join(", ");
  return (
    <button
      type="button"
      className="lt-mi lt-vrow"
      data-view-row={dial.key}
      // The visible words in their order, with the pause between them that two
      // boxes side by side give the eye.
      aria-label={name}
      onClick={() => onEnter(dial.key)}
    >
      <span className="lt-vrow-label">
        {dial.label}
        {dial.changed && <span className="lt-vdot" />}
      </span>
      <span className="lt-vrow-value">{dial.value}</span>
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
 * Tab past the panel's last control would leave the popover open over a page
 * the reader has left. So focus landing outside it closes it.
 */
function useCloseOnFocusOut(
  open: boolean,
  anchor: RefObject<HTMLElement | null>,
  body: RefObject<HTMLElement | null>,
  close: () => void,
) {
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
      close();
    };
    document.addEventListener("focusin", onFocus);
    return () => document.removeEventListener("focusin", onFocus);
  }, [open, anchor, body, close]);
}

// A filter over a list is a pressed button (README), in the rows' shape.
function ArchivedRow({
  archived,
}: Readonly<{
  archived: { checked: boolean; onChange: (next: boolean) => void };
}>) {
  const t = useT();
  return (
    <div className="lt-vgroup">
      <button
        type="button"
        className="lt-mi lt-vrow lt-vtoggle"
        aria-pressed={archived.checked}
        onClick={() => archived.onChange(!archived.checked)}
      >
        <span className="lt-vrow-label">{t("list.showArchived")}</span>
        {/* The pressed state already says this to a screen reader. */}
        <span className="lt-vrow-value" aria-hidden="true">
          {archived.checked
            ? t("list.showArchivedOn")
            : t("list.showArchivedOff")}
        </span>
        <Check
          strokeWidth={1.8}
          aria-hidden="true"
          className={`lt-caret lt-mi-more${archived.checked ? "" : " lt-vcheck-off"}`}
        />
      </button>
    </div>
  );
}

/**
 * The trigger's face. The count and the dot are drawn for the eye and said in
 * one hidden line. So the name reads "View, 2 filters, changed" in one piece.
 */
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
