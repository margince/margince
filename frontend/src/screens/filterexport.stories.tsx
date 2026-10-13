// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { OverflowMenu } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { useT } from "../i18n";
import { ExportFilterItems, useFilterExport } from "./filterexport";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Exporting what a filter selects: one item per format, in the menu of the
// band the filter is kept from. A success hands the browser a file and leaves
// the page looking exactly as it did, so the states worth capturing are the
// items on offer and the export refused.
const meta: Meta = {
  title: "Patterns/Filter export",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const COMPLETE = newGroup("and", [newLeaf("city", "eq", "Berlin")]);

/** The items as a page holds them: the run outlives the menu that closes. */
function ExportMenu() {
  const t = useT();
  const run = useFilterExport();
  return (
    <>
      <OverflowMenu label={t("filters.footMore")}>
        <ExportFilterItems run={run} resource="contact" tree={COMPLETE} />
      </OverflowMenu>
      <ErrorLine inline error={run.error} />
    </>
  );
}

type User = ReturnType<typeof userEvent.setup>;

/** The menu's items are portalled to the body, outside the story's root. */
async function openMenu(user: User, canvasElement: HTMLElement) {
  await user.click(
    within(canvasElement).getByRole("button", {
      name: "More for this filter",
    }),
  );
  return within(canvasElement.ownerDocument.body);
}

export const Offered: Story = {
  render: () => {
    installFetchStub({});
    return <ExportMenu />;
  },
  play: async ({ canvasElement }) => {
    await openMenu(userEvent.setup(), canvasElement);
  },
};

export const Refused: Story = {
  // The server's own reason, beside the menu that asked. Not "request
  // failed": a bulk read can be refused for something a reader can act on,
  // and the refusal has to land somewhere or somebody waits for a file that
  // is never coming.
  render: () => {
    installFetchStub({
      "POST /exports": () =>
        jsonResponse(
          {
            title: "Export refused",
            status: 403,
            detail: "Bulk record read is human-only.",
          },
          403,
        ),
    });
    return <ExportMenu />;
  },
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const page = await openMenu(user, canvasElement);
    await user.click(page.getByRole("button", { name: "Export CSV" }));
    await within(canvasElement).findByRole("alert");
  },
};
