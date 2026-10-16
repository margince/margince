// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { ImportCard } from "./import";
import { formatFixture, recentRunsFixture } from "./import.fixtures";

// Bringing a customer's data in. The settings page is the hub: the drop zone,
// the template with its import order, and the recent runs. The wizard that performs
// an import is the page behind it, at #/settings/import/run.
//
// What is catalogued here is the hub with and without runs, the withheld
// answer and the run page's first step. The later steps have their own files.
function story(
  allow: Parameters<typeof meRoute>[0],
  subpage?: string,
  runs: readonly unknown[] | "refused" = recentRunsFixture,
) {
  return () => {
    // The run page reads its run from the address, and the capture harness
    // drives every story through one page, so each story sets it.
    globalThis.history.replaceState(null, "", "#/settings/import");
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /imports/format": () => jsonResponse(formatFixture),
      "GET /imports": () =>
        runs === "refused"
          ? jsonResponse(
              { title: "Server error", status: 500, type: "about:blank" },
              500,
            )
          : jsonResponse({ data: runs, page: { has_more: false } }),
      "GET /users/names": () =>
        jsonResponse({ data: [{ id: "u-ada", display_name: "Ada Lovelace" }] }),
    });
    return (
      <StoryProviders>
        <ImportCard subpage={subpage} />
      </StoryProviders>
    );
  };
}

const RUN = "run";

const OPERATOR = { import_run: ["create", "read", "update"] } as const;

const meta: Meta<typeof ImportCard> = {
  title: "Settings/Data/Data import/Import",
  component: ImportCard,
};
export default meta;
type Story = StoryObj<typeof ImportCard>;

// The hub with two runs behind it: one complete that may still be undone, and
// one already undone.
export const Hub: Story = { render: story(OPERATOR) };

// The same hub in dark: the status badges and the drop zone read off derived
// tokens.
export const HubDark: Story = {
  globals: { theme: "dark" },
  render: story(OPERATOR),
};

// No import has run yet: the hub stands alone, with no history panel.
export const HubEmpty: Story = { render: story(OPERATOR, undefined, []) };

// At a phone width the import order wraps with each arrow beside the name
// before it, and the history table folds.
export const HubPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(OPERATOR),
};

// The history did not load: the panel stays and says so inside its body.
export const HistoryUnreadable: Story = {
  render: story(OPERATOR, undefined, "refused"),
  play: async ({ canvasElement }) => {
    const panel = await within(canvasElement).findByRole("region", {
      name: "Recent imports",
    });
    await expect(await within(panel).findByRole("alert")).toBeVisible();
  },
};

// Maintenance opens on the admin role OR an embedding-reindex read, so a seat
// holding only the latter reaches this page. It is told the import exists and is
// not theirs to run — an absent card would say the installation cannot import.
export const Withheld: Story = {
  render: story({ embedding_reindex: ["read"] }),
};

// The run page's first step, reached from the hub's drop zone or by address.
export const RunPage: Story = { render: story(OPERATOR, RUN) };
