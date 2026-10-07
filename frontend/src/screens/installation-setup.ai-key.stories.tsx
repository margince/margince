// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { AiKeyFields } from "./installation-setup.ai-key";
import { SETUP_PROVIDERS, type SetupProviderId } from "./setup-providers";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The model step's credential: a pasted key for most vendors, the key file and
// a location for Gemini on Vertex. Before the key is saved Google cannot be
// asked for locations, so the location field offers eu and says why.
function story(choice: SetupProviderId, attempted = false) {
  return function Render() {
    installFetchStub({
      "GET /ai/provider-locations/gemini_vertex": () =>
        jsonResponse({
          provider: "gemini_vertex",
          locations: [],
          unavailable: "no_key",
        }),
    });
    const [secret, setSecret] = useState("");
    const [location, setLocation] = useState("eu");
    return (
      <StoryProviders>
        <div style={{ maxWidth: "560px" }}>
          <AiKeyFields
            preset={SETUP_PROVIDERS[choice]}
            secret={secret}
            refusal={undefined}
            attempted={attempted}
            location={location}
            disabled={false}
            onSecret={setSecret}
            onLocation={setLocation}
          />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiKeyFields> = {
  title: "Onboarding/First run/Model credential",
  component: AiKeyFields,
};
export default meta;
type Story = StoryObj<typeof AiKeyFields>;

export const ApiKey: Story = { render: story("gemini") };

export const VertexKeyFile: Story = { render: story("gemini_vertex") };

// Continue pressed with nothing pasted: the field names what is missing.
export const VertexKeyFileNeeded: Story = {
  render: story("gemini_vertex", true),
};

export const VertexKeyFileDark: Story = {
  globals: { theme: "dark" },
  render: story("gemini_vertex"),
};
