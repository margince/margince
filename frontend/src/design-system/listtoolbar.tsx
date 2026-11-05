import { Search } from "lucide-react";
import { type ReactNode, useState } from "react";
import { useT } from "../i18n";
import { useFoldedViewport } from "../lib/viewport";
import { Checkbox } from "./atoms";
import { FilterRow, FilterStep } from "./listfilter";
import { SortStep, sortSummary } from "./listsort";
import {
  type ListChip,
  openRowSegment,
  type SortControl,
  type SortOption,
} from "./listsurface.dials";
import { ListViewMenu, type ViewRow, type ViewToggle } from "./listviewmenu";

export const VIEW_MENU = "view";

export function Toolbar({
  search,
  sort,
  sortOptions,
  chips,
  chosen,
  onChipChange,
  archived,
  layout,
  viewRows,
  displayRows,
  displayToggles,
  changed,
  tools,
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
  layout?: ReactNode;
  viewRows: readonly ViewRow[];
  displayRows: readonly ViewRow[];
  displayToggles: readonly ViewToggle[];
  changed: boolean;
  tools?: ReactNode;
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
  const applyFilter = (key: string, value: string, label: string) => {
    setPicked((prev) => ({ ...prev, [key]: [value, label] }));
    onChipChange?.(key, value);
  };
  const deleteFilter = (key: string) => onChipChange?.(key, "");

  const rows: ViewRow[] = [...viewRows];
  if (chips.length > 0) {
    rows.push({
      key: "filters",
      label: t("table.filters"),
      value:
        applied.length > 0
          ? applied.map((chip) => chip.label).join(", ")
          : t("list.viewMenuNone"),
      changed: applied.length > 0,
      ownHead: true,
      step: (back) => (
        <FilterStep
          chips={chips}
          chosen={chosen}
          labelFor={labelFor}
          onPick={applyFilter}
          onDelete={deleteFilter}
          back={back}
        />
      ),
    });
  }
  if (sort && sortOptions.length > 0) {
    rows.push({
      key: "sort",
      label: t("table.sort"),
      value: sortSummary(sort, sortOptions, t),
      changed: sort.value !== (sort.initial ?? ""),
      step: () => <SortStep sort={sort} options={sortOptions} />,
    });
  }
  rows.push(...displayRows);
  // Which rows exist before how they are drawn, as in the rows above.
  const toggles: ViewToggle[] = [
    ...(archived
      ? [{ key: "archived", label: t("list.showArchived"), ...archived }]
      : []),
    ...displayToggles,
  ];
  const hasPanel =
    rows.length > 0 ||
    toggles.length > 0 ||
    layout !== undefined ||
    saveView !== undefined;

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
      {hasPanel && (
        <ListViewMenu
          open={openMenu === VIEW_MENU}
          onOpenChange={(next) => setOpenMenu(next ? VIEW_MENU : null)}
          filterCount={applied.length}
          changed={changed}
          layout={layout}
          rows={rows}
          toggles={toggles}
          footer={saveView}
        />
      )}
      {tools}
      {applied.length > 0 && (
        <AppliedFilters>
          {(menuHost) =>
            applied.map((chip) => (
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
                  applyFilter(chip.key, value, label);
                  setOpenMenu(null);
                }}
                onDelete={() => {
                  deleteFilter(chip.key);
                  setOpenMenu(null);
                }}
                menuHost={menuHost}
              />
            ))
          }
        </AppliedFilters>
      )}
    </div>
  );
}

// Below the fold the line scrolls sideways and would clip a menu opened inside
// it, so each row's menus are drawn under the line instead.
function AppliedFilters({
  children,
}: Readonly<{ children: (menuHost: HTMLElement | null) => ReactNode }>) {
  const folded = useFoldedViewport();
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  return (
    <div className="lt-applied">
      <div className="lt-applied-rows">{children(folded ? host : null)}</div>
      <div className="lt-applied-menus" ref={setHost} />
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
