// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { EdgeDetail } from "./edgedetail";

// One selected colleague on the contact's map: the strength band leads, then
// the counts it was read from. A pair with no cited messages says the counts
// are all there is, rather than drawing an empty receipt list.

type ContactGraph = components["schemas"]["ContactGraph"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";

const graph: ContactGraph = {
  contact_id: CONTACT,
  nodes: [
    {
      id: `contact:${CONTACT}`,
      type: "contact",
      group: "anchor",
      label: "Dana Buyer",
      contact_id: CONTACT,
    },
    {
      id: `user:${SOFIA}`,
      type: "colleague",
      group: "direct",
      label: "Sofia Meier",
      sublabel: "Account Executive",
      user_id: SOFIA,
    },
  ],
  edges: [
    {
      from: `user:${SOFIA}`,
      to: `contact:${CONTACT}`,
      strength_bucket: "moderate",
      interactions_90d: 6,
      inbound_90d: 2,
      outbound_90d: 4,
    },
  ],
  groups_omitted: [],
};

const meta: Meta = {
  title: "Records/Contact network/Edge detail",
  parameters: { layout: "padded" },
};
export default meta;

export const ModerateCountsOnly: StoryObj = {
  render: () => (
    <StoryProviders>
      <EdgeDetail
        graph={graph}
        nodeId={`user:${SOFIA}`}
        anchorId={`contact:${CONTACT}`}
      />
    </StoryProviders>
  ),
};
