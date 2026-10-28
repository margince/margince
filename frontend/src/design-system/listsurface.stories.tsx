import type { Meta, StoryObj } from "@storybook/react-vite";
import { Phone, Plus } from "lucide-react";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import {
  expectAppliedFiltersFit,
  expectAppliedFiltersOneLine,
  LONG_FILTER_CHIPS,
  LONG_FILTER_VALUES,
  WIDE,
} from "../testing/appliedfilterchecks";
import { Button } from "./atoms";
import { IconAction } from "./iconaction";
import { Menu, useCloseOnEscape } from "./listmenu";
import { CountLine, ListSurface, type ListView } from "./listsurface";

// The surface's header when its sentence is longer than the row: German runs
// the count a third longer than English, and a list with six saved views and
// two verbs leaves it no room. The count is the one item that gives way — it
// moves down with the verbs, and on a line too short for it, ellipsizes — so
// every tab and every verb stays inside the card at any width.

const meta: Meta = {
  title: "Components/Text and data display/List surface",
  component: ListSurface,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="de">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const VIEWS: readonly ListView[] = [
  { label: "Alle" },
  { label: "Meine" },
  { label: "Kunden" },
  { label: "Interessenten" },
  { label: "Partner" },
  { label: "Lieferanten" },
];

function GermanSurface() {
  const [active, setActive] = useState(0);
  const [query, setQuery] = useState("");
  return (
    <ListSurface
      title="Unternehmen"
      views={VIEWS}
      activeView={active}
      onViewChange={setActive}
      count={
        <CountLine
          unit="Unternehmen"
          first={1}
          last={25}
          total={1312}
          more
          sortedBy="Zuletzt aktualisiert"
        />
      }
      action={
        <>
          <Button>Partner</Button>
          <Button>
            <Plus aria-hidden="true" />
            Neues Unternehmen
          </Button>
        </>
      }
      search={{ value: query, onChange: setQuery }}
    >
      <p>Zeilen</p>
    </ListSurface>
  );
}

// The verbs and the tab rail lie inside the card: neither is clipped or pushed
// past its edge to keep the sentence on the first line. Below 1100px the verbs
// are one overflow menu and a rail with more tabs than room scrolls sideways.
async function nothingPushedOut(canvasElement: HTMLElement) {
  await within(canvasElement).findByRole("button", { name: "Alle" });
  const card = canvasElement.querySelector(".lt");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the list surface did not render");
  }
  const edge = card.getBoundingClientRect();
  const held = card.querySelectorAll(".lt-views, .lt-actions > *");
  await expect(held.length).toBeGreaterThan(1);
  for (const item of held) {
    const box = item.getBoundingClientRect();
    await expect(box.left).toBeGreaterThanOrEqual(edge.left);
    await expect(box.right).toBeLessThanOrEqual(edge.right);
  }
}

// A desktop window with a narrow column: the sentence cannot share the tabs'
// line, so it moves down beside the verbs.
export const GermanCountNarrow: Story = {
  name: "German count, narrow column",
  render: () => (
    <div style={{ maxWidth: "40rem" }}>
      <GermanSurface />
    </div>
  ),
  play: async ({ canvasElement }) => nothingPushedOut(canvasElement),
};

// Narrower than the sentence itself: it ellipsizes on its own line, and the
// truncation tip on hover or focus carries the whole of it.
export const GermanCountTruncated: Story = {
  name: "German count, truncated",
  render: () => (
    <div style={{ maxWidth: "26rem" }}>
      <GermanSurface />
    </div>
  ),
  play: async ({ canvasElement }) => nothingPushedOut(canvasElement),
};

// A phone: the count takes a line of its own under the tabs and wraps there.
export const GermanCountPhone: Story = {
  name: "German count, phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <GermanSurface />,
  play: async ({ canvasElement }) => nothingPushedOut(canvasElement),
};

function SortMenuBesideAVerb() {
  const [open, setOpen] = useState<string | null>("sort");
  useCloseOnEscape(open, () => setOpen(null));
  return (
    <div style={{ display: "flex", gap: "var(--space-3)" }}>
      <span className="lt-menu-wrap">
        <Button
          aria-expanded={open !== null}
          onClick={() => setOpen(open === null ? "sort" : null)}
        >
          Sort
        </Button>
        <Menu open={open !== null} head="Sort by">
          <Button variant="ghost">Name</Button>
          <Button variant="ghost">Last updated</Button>
        </Menu>
      </span>
      <IconAction label="Call" icon={<Phone aria-hidden="true" />} />
    </div>
  );
}

export const MenuUnderATip: Story = {
  name: "Menu under a tip",
  render: () => <SortMenuBesideAVerb />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const menu = canvas.getByRole("group", { name: "Sort by" });
    const call = canvas.getByRole("button", { name: "Call" });
    await userEvent.hover(call);
    const body = within(document.body);
    await waitFor(() => expect(body.getByRole("tooltip")).toBeVisible());

    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(body.queryByRole("tooltip")).toBeNull());
    await expect(menu).toHaveClass("open");

    await userEvent.keyboard("{Escape}");
    await expect(menu).not.toHaveClass("open");
  },
};

// An applied filter longer than a phone's line: its value wraps at its spaces,
// whole, and the "⋮" that removes the filter stays inside the toolbar.
export const LongAppliedFilterPhone: Story = {
  name: "Long applied filter, phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <LongFilters />,
  play: async ({ canvasElement }) => expectAppliedFiltersFit(canvasElement),
};

// The same filters on a desktop line: each stays one line at the control height.
export const LongAppliedFilterWide: Story = {
  name: "Long applied filter, wide",
  parameters: WIDE,
  globals: { viewport: { value: "wide" } },
  tags: ["uat-wide"],
  render: () => <LongFilters />,
  play: async ({ canvasElement }) => expectAppliedFiltersOneLine(canvasElement),
};

function LongFilters() {
  return (
    <ListSurface
      title="Leads"
      chips={LONG_FILTER_CHIPS}
      chosen={LONG_FILTER_VALUES}
      onChipChange={() => undefined}
    >
      <p>Zeilen</p>
    </ListSurface>
  );
}
