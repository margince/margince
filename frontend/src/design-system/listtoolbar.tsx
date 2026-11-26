import { type ReactNode, useRef, useState } from "react";
import { type Translator, useT } from "../i18n";
import { useFoldedViewport } from "../lib/viewport";
import { FilterRow, FilterStep } from "./listfilter";
import { ListSearch } from "./listsearch";
import { SortStep, sortSummary } from "./listsort";
import {
  type ListChip,
  openRowSegment,
  type SortControl,
  type SortOption,
} from "./listsurface.dials";
import {
  ListViewMenu,
  ToggleRow,
  type ViewRow,
  type ViewToggle,
} from "./listviewmenu";

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
  notice,
  openMenu,
  setOpenMenu,
}: Readonly<{
  search?: { value: string; onChange: (next: string) => void };
  sort?: SortControl;
  sortOptions: readonly SortOption[];
  chips: readonly ListChip[];
  chosen: Readonly<Record<string, string>>;
  onChipChange?: (key: string, value: string) => void;
  archived?: Archived;
  layout?: ReactNode;
  viewRows: readonly ViewRow[];
  displayRows: readonly ViewRow[];
  displayToggles: readonly ViewToggle[];
  changed: boolean;
  tools?: ReactNode;
  saveView?: ReactNode;
  notice?: ReactNode;
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
  const root = useRef<HTMLDivElement>(null);
  // A closing menu takes focus with it. So a chip's pick or delete hands focus
  // on: to the first of `rows` that is drawn, else to View.
  const focusApplied = (...rows: number[]) => {
    const drawn = root.current?.querySelectorAll<HTMLElement>(
      ".lt-applied .lt-frow-value",
    );
    const row = rows.map((index) => drawn?.[index]).find(Boolean);
    (row ?? root.current?.querySelector<HTMLElement>(".lt-vtrigger"))?.focus();
  };

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
      // "Default order" is the server's own, so it carries no dot.
      changed: sort.value !== "" && sort.value !== (sort.initial ?? ""),
      step: () => <SortStep sort={sort} options={sortOptions} />,
    });
  }
  rows.push(...displayRows);
  // Which rows exist before how they are drawn, as in the rows above.
  const toggles: ViewToggle[] = [
    ...(archived ? [archivedToggle(archived, t)] : []),
    ...displayToggles,
  ];
  const hasPanel =
    rows.length > 0 ||
    toggles.length > 0 ||
    layout !== undefined ||
    saveView !== undefined;

  return (
    <div className="lt-tools" ref={root}>
      {search && (
        <ListSearch
          search={search}
          chips={chips}
          chosen={chosen}
          onPick={onChipChange ? applyFilter : undefined}
          onFocus={() => setOpenMenu(null)}
          menuOpen={openMenu !== null}
        />
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
      <div className="lt-notice">{notice}</div>
      {applied.length > 0 && (
        <AppliedFilters>
          {(menuHost) =>
            applied.map((chip, index) => (
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
                  // The any option takes the filter, and this row, away.
                  focusApplied(...(value ? [index] : [index + 1, index - 1]));
                }}
                onDelete={() => {
                  deleteFilter(chip.key);
                  setOpenMenu(null);
                  focusApplied(index + 1, index - 1);
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

type Archived = { checked: boolean; onChange: (next: boolean) => void };

function archivedToggle(archived: Archived, t: Translator): ViewToggle {
  return { key: "archived", label: t("list.showArchived"), ...archived };
}

/** Show archived for a list with no View panel, as the panel draws it. */
export function ArchivedSwitch({ archived }: Readonly<{ archived: Archived }>) {
  const t = useT();
  return <ToggleRow toggle={archivedToggle(archived, t)} />;
}
