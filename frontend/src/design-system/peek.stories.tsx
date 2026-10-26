// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { type PeekContent, usePeek } from "./peek";

// A peek is the card a count opens: which deals "4 open" counts, who "3 in
// touch" is. It is hung off an element the caller owns. These frames show the
// three shapes of that element. One is a reading with no door of its own.
// One is a tile whose door keeps its press. One is a card whose rows are a page
// of a longer list.

const meta: Meta = {
  title: "Components/Overlays and layering/Peek",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const deals: PeekContent = {
  head: "Open deals on this account",
  items: [
    {
      key: "a",
      title: "Expansion Phase 2",
      sub: "Proposal · closes 30 Nov",
      right: "€95k",
      href: "#/deals/a",
    },
    {
      key: "b",
      title: "Facade retrofit programme",
      sub: "Proposal · stalled · closes 15 Feb",
      right: "€42k",
      href: "#/deals/b",
    },
    {
      key: "c",
      title: "Depot phase two services",
      sub: "Discovery",
      right: "Not priced",
      href: "#/deals/c",
    },
  ],
};

function Reading({ content }: Readonly<{ content: PeekContent }>) {
  const peek = usePeek<HTMLButtonElement>({ content, pressOpens: true });
  return (
    <>
      <button ref={peek.ref} type="button" {...peek.props}>
        3 open deals
      </button>
      {peek.panel}
    </>
  );
}

/** A reading with no door of its own: a press opens the card and takes focus to
 *  the first row; hover and keyboard focus open it without moving focus. */
export const OpensOnPress: Story = {
  render: () => <Reading content={deals} />,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "3 open deals" }),
    );
    await expect(
      within(document.body).getByRole("region", {
        name: "Open deals on this account",
      }),
    ).toBeVisible();
  },
};

/** A card whose rows are a page of a longer list says so in its foot, so a count
 *  of eleven never sits over a card of three that passes for all of them. */
export const APageOfALongerList: Story = {
  render: () => <Reading content={{ ...deals, foot: "Showing 3 of 11." }} />,
};

/** German is longer: names and sub-lines wrap inside the card rather than
 *  widening it. */
export const LongGermanRows: Story = {
  render: () => (
    <Reading
      content={{
        head: "Offene Deals dieses Unternehmens",
        items: [
          {
            key: "a",
            title: "Erweiterung Phase 2 für die Standorte Nord und Süd",
            sub: "Angebot · Abschluss am 30. November · ins Stocken geraten",
            right: "95.000 €",
            href: "#/deals/a",
          },
        ],
      }}
    />
  ),
};
