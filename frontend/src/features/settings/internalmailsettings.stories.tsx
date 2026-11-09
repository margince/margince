// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { InternalMailCaptureCard } from "./internalmailsettings";

type Settings = components["schemas"]["InternalMailSettings"];
const source: components["schemas"]["InternalMailSourceOption"] = {
  connection_id: "11111111-1111-4111-8111-111111111111",
  owner_user_id: "22222222-2222-4222-8222-222222222222",
  provider: "graph",
  generation: 3,
  account_label: "operations@demo.test",
  owner_name: "Ana Fischer",
};
const enrolled = {
  ...source,
  containers: ["graph:AbCd"],
  enrolled_at: "2026-01-01T00:00:00Z",
  enrolled_by: "human:admin",
};
function story(settings?: Settings) {
  return () => {
    installFetchStub({
      "GET /capture/settings": () =>
        jsonResponse(settings ? { internal_mail: settings } : {}),
      "GET /connectors/graph/containers": () =>
        jsonResponse({
          containers: [{ id: "AbCd", name: "Customer Operations" }],
          truncated: false,
        }),
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <SavePageProvider>
            <InternalMailCaptureCard />
            <PageSaveBar />
          </SavePageProvider>
        </div>
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Settings/Data/Mail capture/Internal operational mail",
  component: InternalMailCaptureCard,
  parameters: { layout: "padded" },
} satisfies Meta<typeof InternalMailCaptureCard>;
export default meta;
type Story = StoryObj<typeof meta>;
export const DefaultOff: Story = {
  render: story({ enabled: false, sources: [], available_sources: [source] }),
};
export const ScopedSource: Story = {
  render: story({
    enabled: true,
    sources: [enrolled],
    available_sources: [source],
  }),
};
export const HistoryApproved: Story = {
  render: story({
    enabled: true,
    sources: [{ ...enrolled, history_since: "2026-01-01" }],
    available_sources: [source],
  }),
};
export const AccountChanged: Story = {
  render: story({
    enabled: true,
    sources: [enrolled],
    available_sources: [{ ...source, generation: 4 }],
  }),
};
export const NonAdministrator: Story = { render: story() };
