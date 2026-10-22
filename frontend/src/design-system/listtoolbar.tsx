import { Search } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useT } from "../i18n";
import { Checkbox } from "./atoms";
import { FilterMenu, FilterRow } from "./listfilter";
import { SortMenu } from "./listsort";
import {
  type ListChip,
  openRowSegment,
  type SortControl,
  type SortOption,
} from "./listsurface.dials";

/**
 * The controls row, read as two halves. What narrows the list is on the left:
 * search, chips, the body's own filter pickers. What changes how the list is
 * displayed is on the right. A reader looking to cut the set down never has to
 * scan the whole row to find out which controls do that.
 */
export function Toolbar({
  search,
  sort,
  sortOptions,
  chips,
  chosen,
  onChipChange,
  archived,
  tools,
  displayMenu,
  saveView,
  openMenu,
  setOpenMenu,
}: Readonly<{
  search?: { value: string; onChange: (next: string) => void };
  sort?: SortControl;
  sortOptions: readonly SortOption[];
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  archived?: { checked: boolean; onChange: (next: boolean) => void };
  tools?: ReactNode;
  displayMenu?: (
    menu: Readonly<{ open: boolean; onToggle: () => void }>,
  ) => ReactNode;
  saveView?: ReactNode;
  openMenu: string | null;
  setOpenMenu: (next: string | null) => void;
}>) {
  const t = useT();
  const applied = chips.filter((chip) => chosen[chip.key]);
  // The label a searched value was picked under, kept only while that value is
  // still the one applied. A saved view, Clear filters, or the reader deleting
  // the row can also change the filter. A label held past that would name the
  // wrong record.
  const [picked, setPicked] = useState<Record<string, [string, string]>>({});
  const labelFor = (key: string, value: string) => {
    const remembered = picked[key];
    return remembered && remembered[0] === value ? remembered[1] : undefined;
  };
  const remember = (key: string, value: string, label: string) =>
    setPicked((prev) => ({ ...prev, [key]: [value, label] }));
  // Every dial on the row asks the same state which one is open, so opening one
  // is what closes the last. Spelled once: three copies of this arithmetic is
  // how the Display menu came to keep an `open` of its own.
  const menu = (key: string) => ({
    open: openMenu === key,
    onToggle: () => setOpenMenu(openMenu === key ? null : key),
  });
  return (
    <div className="lt-tools">
      {search && (
        <label className="lt-search">
          <Search size={13} strokeWidth={1.6} aria-hidden="true" />
          <span className="sr-only">{t("list.search")}</span>
          <input
            // A search field, not a plain text box: it is what the control is,
            // and it is how a reader's assistive technology announces it.
            type="search"
            className="lt-search-input"
            value={search.value}
            placeholder={t("list.search")}
            onChange={(event) => search.onChange(event.target.value)}
          />
        </label>
      )}

      {applied.map((chip) => (
        <FilterRow
          key={chip.key}
          chip={chip}
          value={chosen[chip.key] ?? ""}
          pickedLabel={labelFor(chip.key, chosen[chip.key] ?? "")}
          openSegment={openRowSegment(openMenu, chip.key)}
          onToggleSegment={(next) => {
            const key = `row:${chip.key}:${next}`;
            setOpenMenu(openMenu === key ? null : key);
          }}
          onPick={(value, label) => {
            remember(chip.key, value, label);
            onChipChange?.(chip.key, value);
            setOpenMenu(null);
          }}
          onDelete={() => {
            onChipChange?.(chip.key, "");
            setOpenMenu(null);
          }}
        />
      ))}

      {chips.length > 0 && (
        <FilterMenu
          chips={chips}
          chosen={chosen}
          onChipChange={onChipChange}
          onRemember={remember}
          {...menu("filter")}
          hasApplied={applied.length > 0}
        />
      )}

      {archived && <ArchivedSwitch archived={archived} />}

      <span className="lt-spacer" />

      {sort && sortOptions.length > 0 && (
        <SortMenu sort={sort} options={sortOptions} {...menu("sort")} />
      )}

      {displayMenu?.(menu("display"))}

      {tools}

      {saveView}
    </div>
  );
}

export function ArchivedSwitch({
  archived,
}: Readonly<{
  archived: { checked: boolean; onChange: (next: boolean) => void };
}>) {
  const t = useT();
  return (
    <Checkbox
      checked={archived.checked}
      label={t("list.showArchived")}
      onChange={(event) => archived.onChange(event.target.checked)}
    />
  );
}
