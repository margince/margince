// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { VertexLocationField } from "./vertex-location";

// Where a Gemini-on-Vertex lane is processed. Open the list: it reads EU, US,
// Other, Global, each with its residency badge, and under eu_hosted the
// non-resident rows are greyed and say why.
const LOCATIONS = {
  provider: "gemini_vertex",
  locations: [
    {
      id: "eu",
      display_name: "EU (multi-region)",
      jurisdiction: "eu",
      resident: true,
    },
    {
      id: "europe-west4",
      display_name: "Netherlands",
      jurisdiction: "eu",
      resident: true,
    },
    {
      id: "us",
      display_name: "US (multi-region)",
      jurisdiction: "us",
      resident: false,
    },
    {
      id: "europe-west2",
      display_name: "London",
      jurisdiction: "other",
      resident: false,
    },
    {
      id: "global",
      display_name: "Global",
      jurisdiction: "global",
      resident: false,
    },
  ],
};

function story(body: unknown = LOCATIONS, initial = "eu") {
  return function Render() {
    installFetchStub({
      "GET /ai/provider-locations/gemini_vertex": () => jsonResponse(body),
    });
    const [value, setValue] = useState(initial);
    return (
      <StoryProviders>
        <div style={{ maxWidth: "480px" }}>
          <VertexLocationField
            value={value}
            disabled={false}
            onChange={setValue}
          />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof VertexLocationField> = {
  title: "Settings/AI/AI models/Vertex location",
  component: VertexLocationField,
};
export default meta;
type Story = StoryObj<typeof VertexLocationField>;

export const Default: Story = { render: story() };

// A stored location the list does not name still shows, so the field never
// holds a value it has no option for.
export const StoredLocationOutsideTheEU: Story = {
  render: story(LOCATIONS, "europe-west2"),
};

export const NoKey: Story = {
  render: story({
    provider: "gemini_vertex",
    locations: [],
    unavailable: "no_key",
  }),
};

export const DefaultDark: Story = {
  globals: { theme: "dark" },
  render: story(),
};
