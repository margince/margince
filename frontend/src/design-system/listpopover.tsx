// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
  useId,
  useState,
} from "react";
import { usePhoneViewport } from "../app/viewport";
import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";
import { useActiveOptionVisible } from "./anchoredpopup";
import { Button, type ButtonVariant, SearchField } from "./atoms";
import { ErrorLine } from "./errorline";
import { Heading } from "./heading";
import { Modal } from "./modal";
import { Popover } from "./popover";
import { type CandidateSearch, useCandidateSearch } from "./recordpicker";
import { type ListKey, SuggestOption, walkedTo } from "./suggestlist";
import "./listpopover.css";

export type ListPopoverOption = Readonly<{
  id: string;
  /** What the row reads as, what a typed term matches, and what is announced. */
  name: string;
  /** Drawn in place of the name where the name has a mark of its own. */
  face?: ReactNode;
  hint?: string;
  disabled?: boolean;
}>;

/** The rows come in hand, filtered here as the reader types (`undefined`
 *  while they load), or from the server, asked once per settled term. */
type ListSource =
  | Readonly<{
      options: readonly ListPopoverOption[] | undefined;
      search?: undefined;
    }>
  | Readonly<{
      search: (term: string) => Promise<readonly ListPopoverOption[]>;
      options?: undefined;
    }>;

type ListPopoverProps = Readonly<{
  /** The trigger's face. */
  label: ReactNode;
  /** The list's name, and the sheet's heading on a phone. */
  title: string;
  searchLabel: string;
  variant?: ButtonVariant;
  className?: string;
  reasonId?: string;
  /** The id of the option the record holds now, marked selected. */
  selected?: string;
  /**
   * Called with the picked option and `done`, which closes the panel. Call it
   * at once for a pick that writes nothing, or once the write lands, so a
   * refused write keeps the panel open over its `error`.
   */
  onPick: (option: ListPopoverOption, done: () => void) => void;
  /** The caller's write is in flight: every row refuses a second pick. */
  pending?: boolean;
  error?: ReactNode;
  /** What an empty list says, where "No match" is not the whole answer. */
  empty?: ReactNode;
  footer?: ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}> &
  ListSource;

/**
 * A trigger that opens a search over a list, anchored to it, where a pick is
 * the whole act: the panel is a non-modal dialog, so the page stays usable.
 * On a phone the same search and list fill a `Modal` sheet instead, because a
 * panel anchored to a thumb-sized trigger leaves no room for the keyboard.
 */
export function ListPopover({
  label,
  title,
  searchLabel,
  variant = "ghost",
  className,
  reasonId,
  open: openProp,
  onOpenChange,
  ...list
}: ListPopoverProps) {
  const phone = usePhoneViewport();
  const titleId = useId();
  const [ownOpen, setOwnOpen] = useState(false);
  const open = openProp ?? ownOpen;
  const setOpen = (next: boolean) => {
    setOwnOpen(next);
    onOpenChange?.(next);
  };
  const search = (
    <ListSearch
      title={title}
      searchLabel={searchLabel}
      list={list}
      done={() => setOpen(false)}
    />
  );

  if (!phone) {
    return (
      <Popover
        label={label}
        variant={variant}
        className={className}
        reasonId={reasonId}
        open={open}
        onOpenChange={setOpen}
        dialog
        panelClassName="listpopover-panel"
      >
        {search}
      </Popover>
    );
  }
  return (
    <>
      <Button
        variant={variant}
        className={className}
        reasonId={reasonId}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        {label}
      </Button>
      <Modal
        intent="drawer"
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
      >
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {title}
        </Heading>
        {search}
      </Modal>
    </>
  );
}

function ListSearch({
  title,
  searchLabel,
  list,
  done,
}: Readonly<{
  title: string;
  searchLabel: string;
  list: Pick<
    ListPopoverProps,
    | "options"
    | "search"
    | "selected"
    | "onPick"
    | "pending"
    | "error"
    | "empty"
    | "footer"
  >;
  done: () => void;
}>) {
  const t = useT();
  const listboxId = useId();
  const [term, setTerm] = useState("");
  const [active, setActive] = useState(-1);
  const found = useCandidateSearch(list.search, term);
  const rows = list.search
    ? found.candidates
    : matchingByName(list.options ?? [], term);
  const optionId = (index: number) => `${listboxId}-option-${index}`;
  useActiveOptionVisible(true, active, listboxId);

  const pick = (row: ListPopoverOption) => {
    if (row.disabled !== true && list.pending !== true) {
      list.onPick(row, done);
    }
  };
  // Escape is not here: the panel and the sheet each close on it and hand
  // focus back to the trigger, and a second handler would close twice.
  const onKeyDown = (event: ReactKeyboardEvent) => {
    const key = listKeyOf(event.key);
    if (key !== null) {
      event.preventDefault();
      if (rows.length > 0) {
        setActive((current) => walkedTo(current, key, rows.length));
      }
      return;
    }
    const row = rows[active];
    if (event.key === "Enter" && row) {
      event.preventDefault();
      pick(row);
    }
  };

  const status = statusOf({
    t,
    list,
    found,
    empty: rows.length === 0,
  });
  return (
    <div className="listpopover">
      <SearchField
        role="combobox"
        aria-label={searchLabel}
        placeholder={searchLabel}
        aria-expanded={rows.length > 0}
        aria-controls={listboxId}
        aria-autocomplete="list"
        aria-activedescendant={rows[active] ? optionId(active) : undefined}
        value={term}
        onChange={(event) => {
          setTerm(event.target.value);
          setActive(-1);
        }}
        onKeyDown={onKeyDown}
      />
      {found.failure !== null && (
        <ErrorLine>{problemMessageOf(found.failure.cause, t)}</ErrorLine>
      )}
      {status !== null && (
        <p className="listpopover-status" aria-live="polite">
          {status}
        </p>
      )}
      <div className="listpopover-list">
        <div
          role="listbox"
          id={listboxId}
          aria-label={title}
          aria-busy={list.pending === true || undefined}
        >
          {rows.map((row, index) => (
            <SuggestOption
              key={row.id}
              id={optionId(index)}
              active={index === active}
              selected={row.id === list.selected}
              disabled={row.disabled === true || list.pending === true}
              hint={row.hint}
              onPick={() => pick(row)}
              onHover={() => setActive(index)}
            >
              {row.face ?? row.name}
            </SuggestOption>
          ))}
        </div>
      </div>
      {list.error && <ErrorLine>{list.error}</ErrorLine>}
      {list.footer}
    </div>
  );
}

// Not `matchingSuggestions`: it matches the value too, and a value here is an
// id, so every uuid would match the letters a to f.
function matchingByName(
  options: readonly ListPopoverOption[],
  term: string,
): readonly ListPopoverOption[] {
  const needle = term.trim().toLowerCase();
  if (needle === "") {
    return options;
  }
  return options.filter((option) => option.name.toLowerCase().includes(needle));
}

function listKeyOf(key: string): ListKey | null {
  switch (key) {
    case "ArrowDown":
    case "ArrowUp":
    case "Home":
    case "End":
      return key;
    default:
      return null;
  }
}

function statusOf({
  t,
  list,
  found,
  empty,
}: Readonly<{
  t: ReturnType<typeof useT>;
  list: Pick<ListPopoverProps, "options" | "search" | "empty">;
  found: CandidateSearch<ListPopoverOption>;
  empty: boolean;
}>): ReactNode {
  const noMatch = list.empty ?? t("picker.noMatch");
  if (list.search) {
    if (found.pending) {
      return t("search.pending");
    }
    return found.answered && empty ? noMatch : null;
  }
  if (list.options === undefined) {
    return t("common.loading");
  }
  return empty ? noMatch : null;
}
