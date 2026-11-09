// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { MoreRows, useHandoffPages } from "./analytics.outcomes.handoffpages";

// The handoffs of a scope arrive a page at a time. "Show more" stands only
// while the server says a further page exists.

const meta: Meta = {
  title: "Records/Reports/Analytics/Handoff pages",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function FirstPage() {
  const rows = useHandoffPages({ kind: "team", id: "t-dach" }, "waiting", true);
  const count = rows.data?.pages.flatMap((page) => page.rows).length ?? 0;
  return (
    <Panel>
      <PanelBody>
        <p>{formatNumber(count, "en")} handoffs on this page</p>
      </PanelBody>
      <MoreRows rows={rows} />
    </Panel>
  );
}

/** A first page with a further one behind it, which ends the list. */
function aFurtherPage() {
  let served = 0;
  installFetchStub({
    "GET /analytics/sdr-summary/handoffs": () => {
      served += 1;
      const first = served === 1;
      return jsonResponse({
        as_of: "2026-09-21T12:00:00Z",
        rows: [
          {
            id: first ? "h-1" : "h-2",
            waiting_since: "2026-09-18T12:00:00Z",
            resubmitted: false,
            restricted: true,
          },
        ],
        next_cursor: first ? "cursor-2" : null,
      });
    },
  });
  return (
    <StoryProviders>
      <FirstPage />
    </StoryProviders>
  );
}

export const AFurtherPage: Story = { render: aFurtherPage };

export const AFurtherPageDark: Story = {
  globals: { theme: "dark" },
  render: aFurtherPage,
};
