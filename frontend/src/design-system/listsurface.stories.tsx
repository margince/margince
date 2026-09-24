import type { Meta, StoryObj } from "@storybook/react-vite";
import { Plus } from "lucide-react";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { CountLine, ListSurface, type ListView } from "./listsurface";

// The surface's header when its sentence is longer than the row: German runs
// the count a third longer than English, and a list with six saved views and
// two verbs leaves it no room. The count is the one item that gives way — it
// moves down with the verbs, and on a line too short for it, ellipsizes — so
// every tab and every verb stays inside the card at any width.

const meta: Meta = {
  title: "Design System/ListSurface",
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
