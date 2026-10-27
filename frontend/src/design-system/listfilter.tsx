import {
  ChevronDown,
  ChevronRight,
  Filter,
  MoreVertical,
  Plus,
} from "lucide-react";
import { Fragment, useEffect, useId, useRef, useState } from "react";
import { useT } from "../i18n";
import { Button, Radio } from "./atoms";
import { ChipValueList } from "./listfiltervalues";
import { Menu } from "./listmenu";
import type { FilterRowSegment, ListChip } from "./listsurface.dials";

/**
 * The one Filter trigger: a searchable list of the attributes not yet
 * applied, then (once one is picked) that attribute's value list. It reads as
 * the "Filter" button (its icon and label) until a filter exists. Once the
 * first applied row is standing, it reads as a bare "+". What the button
 * offers has not changed, only what it is called once there is something
 * beside it to add to. It is its own component because the two steps, plus
 * the search text narrowing the first one, were most of the toolbar's
 * cognitive complexity.
 */
export function FilterMenu({
  chips,
  chosen,
  onChipChange,
  onRemember,
  open,
  onToggle,
  hasApplied,
}: Readonly<{
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  /** Reports the label a value was picked under, for a searched chip. */
  onRemember: (key: string, value: string, label: string) => void;
  open: boolean;
  onToggle: () => void;
  hasApplied: boolean;
}>) {
  const t = useT();
  const [attributeKey, setAttributeKey] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const wrap = useRef<HTMLSpanElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  // Set only by a step or a pick. Opening the menu with a pointer then does
  // not pull focus anywhere the reader did not ask for it.
  const stepped = useRef(false);
  const picked = useRef(false);

  // A closed menu always reopens at the attribute search, never wherever the
  // reader last left it. The button says "Filter", and names no attribute.
  useEffect(() => {
    if (!open) {
      setAttributeKey(null);
      setQuery("");
    }
  }, [open]);

  // A step between levels takes the control the reader is on with it, and
  // focus then falls to the top of the document. Focus is handed to whatever
  // replaced that control. Going in, that is the head that leads back out.
  // Coming back, it is the search the attribute was chosen from. The back head
  // belongs to `Menu`, so it is reached through the wrap. A ref would have to
  // be threaded into a component that does not otherwise need one.
  useEffect(() => {
    if (!stepped.current) {
      return;
    }
    stepped.current = false;
    if (attributeKey === null) {
      searchRef.current?.focus();
      return;
    }
    wrap.current?.querySelector<HTMLButtonElement>(".lt-mback")?.focus();
  }, [attributeKey]);

  // The same invariant on the way out: a pick closes the menu, which inerts
  // the fieldset the reader is standing in. Focus goes back to the trigger, as
  // it does on Escape. This runs after the close, not during it. Applying the
  // first filter swaps the labelled trigger for the bare "+". So the node to
  // land on is the one that exists once the press has been rendered.
  useEffect(() => {
    if (open || !picked.current) {
      return;
    }
    picked.current = false;
    triggerRef.current?.focus();
  }, [open]);

  // Only the attributes with no filter row of their own yet. Once a filter is
  // applied it stands as its own row (FilterRow below). Offering it again here
  // would be a second way to reach the same value. That route goes through a
  // menu that no longer names the attribute it belongs to.
  const addable = chips.filter((chip) => !chosen[chip.key]);
  const attribute = addable.find((chip) => chip.key === attributeKey);
  const narrowed = addable.filter((chip) =>
    chip.label.toLowerCase().includes(query.toLowerCase()),
  );

  return (
    <span className="lt-menu-wrap" ref={wrap}>
      {/* The "+" carries no caret: the word it dropped is what the caret
          qualifies, and a glyph-only trigger with one reads as two marks. */}
      {hasApplied ? (
        <Button
          ref={triggerRef}
          iconOnly
          aria-expanded={open}
          aria-label={t("table.addFilter")}
          onClick={onToggle}
        >
          <Plus strokeWidth={1.8} aria-hidden="true" />
        </Button>
      ) : (
        <Button ref={triggerRef} aria-expanded={open} onClick={onToggle}>
          <Filter strokeWidth={1.6} aria-hidden="true" />
          {t("table.filter")}
          <ChevronDown className="lt-caret" aria-hidden="true" />
        </Button>
      )}
      <Menu
        open={open}
        head={attribute ? attribute.label : t("table.filter")}
        back={
          attribute && {
            label: t("table.filterBack"),
            onBack: () => {
              stepped.current = true;
              setAttributeKey(null);
            },
          }
        }
      >
        {attribute ? (
          <ChipValueList
            chip={attribute}
            value={chosen[attribute.key] ?? ""}
            onPick={(value, label) => {
              picked.current = true;
              onRemember(attribute.key, value, label);
              onChipChange?.(attribute.key, value);
              onToggle();
            }}
          />
        ) : (
          <>
            <label className="lt-fsearch">
              <span className="sr-only">{t("table.filterSearch")}</span>
              <input
                ref={searchRef}
                className="lt-fsearch-input"
                value={query}
                placeholder={t("table.filterSearch")}
                onChange={(event) => setQuery(event.target.value)}
              />
            </label>
            {narrowed.map((chip) => (
              <button
                type="button"
                key={chip.key}
                className="lt-mi"
                onClick={() => {
                  stepped.current = true;
                  setAttributeKey(chip.key);
                }}
              >
                {chip.label}
                {/* This row opens the attribute's own values rather than
                    picking one, and the glyph is all that says so. */}
                <ChevronRight
                  strokeWidth={1.8}
                  aria-hidden="true"
                  className="lt-caret lt-mi-more"
                />
              </button>
            ))}
          </>
        )}
      </Menu>
    </span>
  );
}

/**
 * The condition segment of an applied filter row. Its menu offers one entry,
 * because the list endpoints compare for equality only. It is still a real
 * menu rather than a disabled label. A second condition can then join it
 * later without a shape change here.
 */
function FilterConditionMenu({
  open,
  onToggle,
}: Readonly<{ open: boolean; onToggle: () => void }>) {
  const t = useT();
  const group = useId();
  return (
    <span className="lt-menu-wrap">
      <button
        type="button"
        className="lt-frow-seg"
        aria-expanded={open}
        onClick={onToggle}
      >
        {t("table.filterIs")}
      </button>
      <Menu open={open} head={t("table.filterCondition")}>
        {/* The one condition, standing. `defaultChecked` and not `checked`,
            because a group of one has no second answer for a change event to
            report: the press closes the menu and nothing else, and a controlled
            radio whose `onChange` can never fire would be a handler that lies
            about what runs. */}
        <Radio
          className="lt-mi"
          name={group}
          defaultChecked
          label={t("table.filterIs")}
          onClick={onToggle}
        />
      </Menu>
    </span>
  );
}

/**
 * An applied filter, read as one row: the attribute, its condition, its
 * value, and a "⋮" menu to delete it. It is a compound pill rather than a bare
 * chip. A second condition (already possible in FilterConditionMenu) then has
 * somewhere to live without the row changing shape again.
 */
export function FilterRow({
  chip,
  value,
  pickedLabel,
  openSegment,
  onToggleSegment,
  onPick,
  onDelete,
}: Readonly<{
  chip: ListChip;
  value: string;
  /**
   * What the reader picked, when the chip searched for it rather than listing
   * it. A searched chip carries no `options` to look the value back up in, so
   * without this the row would name a company by its id.
   */
  pickedLabel?: string;
  openSegment: FilterRowSegment | null;
  onToggleSegment: (segment: FilterRowSegment) => void;
  onPick: (value: string, label: string) => void;
  onDelete: () => void;
}>) {
  const t = useT();
  const active = chip.options.find((option) => option.value === value);
  const valueLabel = active?.label ?? pickedLabel ?? chip.appliedName ?? value;
  return (
    <fieldset className="lt-frow" aria-label={`${chip.label}: ${valueLabel}`}>
      <span className="lt-frow-attr">{chip.label}</span>
      <FilterConditionMenu
        open={openSegment === "cond"}
        onToggle={() => onToggleSegment("cond")}
      />
      <span className="lt-menu-wrap">
        <button
          type="button"
          className="lt-frow-seg lt-frow-value"
          aria-expanded={openSegment === "value"}
          onClick={() => onToggleSegment("value")}
        >
          <span className="lt-frow-valuetext">
            <WholeWords text={valueLabel} />
          </span>
        </button>
        <Menu open={openSegment === "value"} head={chip.label}>
          <ChipValueList chip={chip} value={value} onPick={onPick} />
        </Menu>
      </span>
      <span className="lt-menu-wrap">
        <button
          type="button"
          className="lt-frow-more"
          aria-label={t("table.filterMore", { filter: chip.label })}
          aria-expanded={openSegment === "menu"}
          onClick={() => onToggleSegment("menu")}
        >
          <MoreVertical size={14} strokeWidth={1.8} aria-hidden="true" />
        </button>
        <Menu open={openSegment === "menu"} head={chip.label} align="right">
          <button type="button" className="lt-mi" onClick={onDelete}>
            {t("table.deleteFilter")}
          </button>
        </Menu>
      </span>
    </fieldset>
  );
}

// A short line may break the value only at its spaces: "1001–5000" would
// otherwise break at its dash and read as two figures.
function WholeWords({ text }: Readonly<{ text: string }>) {
  let offset = 0;
  return text.split(" ").map((word, index) => {
    const key = offset;
    offset += word.length + 1;
    return (
      <Fragment key={key}>
        {index > 0 && " "}
        <span className="lt-frow-word">{word}</span>
      </Fragment>
    );
  });
}
