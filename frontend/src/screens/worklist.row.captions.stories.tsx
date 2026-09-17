// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";
import "./worklist.css";

// What a row says about itself UNDER its title — whose row it is and which
// side wrote last, then when, worth, why and cost. Every frame goes through
// `WorklistRow`, because the captions are the row's own account of itself and
// a frame that drew them alone would be a picture of a part nobody ships.
//
// What to check in each frame:
//   · the contact is linked in the facts line only where the title does not
//     already name them;
//   · "They last wrote / We last wrote" stand on a line of their own, the dates
//     in the reading ink, "Never" where a side never wrote;
//   · a row whose moments the server withheld draws no line at all.

type WorklistItem = components["schemas"]["WorklistItem"];

const SONYA = "01a05500-0000-7000-8000-000000000009";
const DEAL = "01a05500-0000-7000-8000-0000000000da";

/** A thread filed under a deal, from a contact the server named beside it. */
function dealFiledThread(): WorklistItem {
  return {
    id: "waiting-1",
    source: "customer_waiting",
    category: "customer_waiting",
    band: "now",
    level: 1,
    consequence: "buyer_waits",
    title: "Re: pricing for the retrofit",
    because: [
      { kind: "buyer_wrote_last" },
      { kind: "waiting_days", value: { kind: "days", days: 13 } },
    ],
    subject: { type: "deal", id: DEAL, label: "Turbinenbau retrofit" },
    contact: {
      id: SONYA,
      label: "Sonya Beck",
      touch: {
        last_inbound_at: "2026-09-03T16:46:00Z",
        last_outbound_at: null,
      },
    },
    actions: ["open"],
    dispositions: ["snooze", "not_mine"],
  } as WorklistItem;
}

/** Nothing this row's frames press reaches a real API. */
function stubRow() {
  globalThis.fetch = (async (): Promise<Response> =>
    jsonResponse({ data: [] })) as typeof fetch;
}

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/Row captions",
  component: WorklistRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="Today">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistRow>;

// The sender named beside the deal the thread is filed under, and the silence
// in both directions: she wrote, nobody here has.
export const WhoseRowAndWhoWroteLast: Story = {
  args: { item: dealFiledThread(), position: 1, owner: "" },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};

// The reader may see the contact and not the activity: the name stands, and
// no date is claimed either way.
export const MomentsWithheld: Story = {
  args: {
    item: {
      ...dealFiledThread(),
      contact: { id: SONYA, label: "Sonya Beck" },
    },
    position: 1,
    owner: "",
  },
  render: (args) => {
    stubRow();
    return <WorklistRow {...args} />;
  },
};
