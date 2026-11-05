import { ChevronRight, MoreVertical } from "lucide-react";
import {
  type ComponentProps,
  Fragment,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { holdsEveryWord, searchWords } from "../format/collate";
import { useT } from "../i18n";
import { Radio } from "./atoms";
import { ChipValueList } from "./listfiltervalues";
import { Menu } from "./listmenu";
import type { FilterRowSegment, ListChip } from "./listsurface.dials";
import { ViewStep } from "./listviewmenu";
import "./listtable.css";

/** The attributes not yet applied, then the picked one's values; a pick applies
 * the value and returns to the panel's rows. */
export function FilterStep({
  chips,
  chosen,
  onChipChange,
  onRemember,
  back,
}: Readonly<{
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  /** Reports the label a value was picked under, for a searched chip. */
  onRemember: (key: string, value: string, label: string) => void;
  back: () => void;
}>) {
  const t = useT();
  const [attributeKey, setAttributeKey] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const wrap = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  // Set only by a step between the two levels, so arriving from the panel's
  // rows leaves focus where the panel put it.
  const stepped = useRef(false);

  // Going in, focus moves to the way back. Coming back, it returns to the
  // search the attribute was chosen from.
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

  // An applied filter is its own row under the search (FilterRow) and changes
  // there. So this step offers only the attributes not yet applied.
  const addable = chips.filter((chip) => !chosen[chip.key]);
  const attribute = addable.find((chip) => chip.key === attributeKey);
  const words = searchWords(query);
  const narrowed = addable.filter((chip) =>
    holdsEveryWord(words, [chip.label]),
  );

  return (
    <div ref={wrap}>
      {attribute ? (
        <ViewStep
          head={attribute.label}
          back={{
            label: t("table.filterBack"),
            onBack: () => {
              stepped.current = true;
              setAttributeKey(null);
            },
          }}
        >
          <ChipValueList
            chip={attribute}
            value={chosen[attribute.key] ?? ""}
            onPick={(value, label) => {
              onRemember(attribute.key, value, label);
              onChipChange?.(attribute.key, value);
              back();
            }}
          />
        </ViewStep>
      ) : (
        <ViewStep
          head={t("table.filters")}
          back={{ label: t("list.viewMenuBack"), onBack: back }}
        >
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
        </ViewStep>
      )}
    </div>
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
  menuHost,
}: Readonly<{
  open: boolean;
  onToggle: () => void;
  menuHost: HTMLElement | null;
}>) {
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
      <HostedMenu host={menuHost} open={open} head={t("table.filterCondition")}>
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
      </HostedMenu>
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
  menuHost = null,
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
  /**
   * Where the row's menus are drawn instead of under their segments: a row in
   * a sideways scroller would clip them.
   */
  menuHost?: HTMLElement | null;
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
        menuHost={menuHost}
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
        <HostedMenu
          host={menuHost}
          open={openSegment === "value"}
          head={chip.label}
        >
          <ChipValueList chip={chip} value={value} onPick={onPick} />
        </HostedMenu>
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
        <HostedMenu
          host={menuHost}
          open={openSegment === "menu"}
          head={chip.label}
          align="right"
        >
          <button type="button" className="lt-mi" onClick={onDelete}>
            {t("table.deleteFilter")}
          </button>
        </HostedMenu>
      </span>
    </fieldset>
  );
}

function HostedMenu({
  host,
  ...menu
}: Readonly<{ host: HTMLElement | null } & ComponentProps<typeof Menu>>) {
  return host ? createPortal(<Menu {...menu} />, host) : <Menu {...menu} />;
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
