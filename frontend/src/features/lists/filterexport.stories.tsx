// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ErrorLine } from "../../design-system/errorline";
import { newGroup, newLeaf } from "../../lib/segmentpredicate";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ExportFilterItems, useFilterExport } from "./filterexport";
import "./filters.css";

// Exporting what a filter selects: one button per format, beside the count of
// what it exports. A success hands the browser a file and leaves the page as
// it was. So the states to show are the buttons on offer and a refusal.
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

/** The buttons as the results head holds them, with the run's refusal. */
function ExportButtons() {
  const run = useFilterExport();
  return (
    <span className="filters-result-head">
      <ExportFilterItems run={run} resource="contact" tree={COMPLETE} />
      <ErrorLine inline error={run.error} />
    </span>
  );
}

export const Offered: Story = {
  render: () => {
    installFetchStub({});
    return <ExportButtons />;
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
    return <ExportButtons />;
  },
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await user.click(canvas.getByRole("button", { name: "Export CSV" }));
    await canvas.findByRole("alert");
  },
};
