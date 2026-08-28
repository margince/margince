// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { installFetchStub } from "../screens/story-utils";
import {
  PageAside,
  PageAsideProvider,
  PageAsideRegion,
  PageAsideToggle,
} from "./pageaside";

// The page's context column, in the three states a reader can put it in: open,
// folded to its strip, and absent because no screen filled it.
//
// It is mounted inside the shell's own `.app` grid rather than on blank canvas,
// because the column IS a track of that grid — `.app:has(> .pageaside…)` is
// what grows the third column, and a story that floated the aside on its own
// would be reviewing a geometry the product never draws.
//
// StoryProviders is deliberately not used here: its RecordShell brings a
// PageAsideProvider and a second PageAsideRegion of its own, which is exactly
// right for a record screen and exactly wrong for the story of the column —
// two columns in the tree, and no way to say which one a query found. The fetch
// stub it installs is kept, routing nothing, because nothing on this surface
// asks the server anything and the empty stub is what says so.
//
// READ THESE AT TWO WIDTHS. The column has a second layout under 1100px: it
// stops being a track beside the work and becomes a block at the end of the
// page, and folding it there removes it rather than leaving a strip, because
// there is no column edge left to fold to. The capture gate's canvas is 1024px
// wide, so its screenshots are that layout; the 320px column, the width tween
// and the 34px strip are what `pnpm storybook` shows on any display past 1100.

const meta: Meta<typeof PageAsideRegion> = {
  title: "Shell/Page aside",
  component: PageAsideRegion,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof PageAsideRegion>;

// Two of the company rail's own cards, in their empty state: real context-column
// copy from the catalog rather than invented filler, so the column is judged
// against the width and rhythm it actually carries.
const contextCards = (
  <PageAside>
    <Panel title={en["co.rail.deals.title"]}>
      <PanelBody>{en["co.rail.deals.empty"]}</PanelBody>
    </Panel>
    <Panel title={en["co.rail.people.title"]}>
      <PanelBody>{en["co.rail.people.empty"]}</PanelBody>
    </Panel>
  </PageAside>
);

function page(aside: ReactNode) {
  return () => {
    // The fold is remembered in localStorage, so a story that read whatever the
    // story before it left there would draw a different column depending on the
    // order the catalog was opened in. Cleared rather than written: no key of
    // the column's is restated here, and every story starts from a reader who
    // has never folded anything.
    window.localStorage.clear();
    installFetchStub({});
    return (
      <LocaleProvider initial="en">
        <PageAsideProvider>
          <div className="app">
            {/* The rail's track, held open and empty: the rail reads a session
                and a route this story has neither of, and the column at the far
                edge is what is under review. */}
            <div />
            <main className="main">
              <div className="scroll">
                <div className="wrap card-actions">
                  <PageAsideToggle />
                </div>
              </div>
            </main>
            <PageAsideRegion />
            {aside}
          </div>
        </PageAsideProvider>
      </LocaleProvider>
    );
  };
}

/** A screen filling the column: the head naming what it is, the fold control,
 *  and the cards under it at the column's own width. */
export const Open: Story = { render: page(contextCards) };

/**
 * Folded away. Open and folded are ONE `<aside>` wearing a class, which is both
 * what lets the track tween and what keeps a screen's cards mounted across the
 * fold — so the play asserts that the element SURVIVES the fold rather than that
 * a second one appeared.
 *
 * Which way the switch is set on arrival is the WINDOW's answer and not this
 * story's: below 1100px the column and the record are one region a reader
 * switches, so the panel is shut on arrival and the switch reads "show". The
 * play therefore drives whichever state the canvas is in and asserts what is
 * true of the switch either way — it flips, it says so, and the element it
 * governs is the same one afterwards.
 */
export const Folded: Story = {
  render: page(contextCards),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    // Asserted before the fold as well as after it: `toBe` over two nulls is a
    // pass, so an identity check on a column that was never there would agree
    // with itself and prove nothing.
    const column = canvasElement.querySelector("aside");
    if (column === null) {
      throw new Error("the column is not in the canvas at all");
    }
    const foldedBefore = column.classList.contains("collapsed");
    // Whichever switch this width puts on screen. Above the fold both are
    // there — the record's header carries one and the column's head the other —
    // and below it only one can be: opening the column there gives it the
    // record's whole cell, so the header goes with the record and the way back
    // is the column's own head. A play that named one of them would be
    // asserting the width rather than the fold.
    const header = canvas.queryByRole("button", {
      name: foldedBefore ? en["record.panel.show"] : en["record.panel.hide"],
    });
    await user.click(
      header ?? canvas.getByRole("button", { name: /^(Hide|Show)$/ }),
    );

    // The one element, wearing the other state. Both halves matter: a second
    // <aside> appearing would take the screen's cards down with it and refetch
    // them on the way back, and a class that did not move means nothing folded.
    const after = canvasElement.querySelector("aside");
    await expect(after).toBe(column);
    await expect(after?.classList.contains("collapsed")).toBe(!foldedBefore);
  },
};

/** No screen supplies context, so the region draws nothing and the toggle is
 *  absent rather than present and inert — a switch for a panel that does not
 *  exist would be a control that does nothing. */
export const Empty: Story = { render: page(null) };
