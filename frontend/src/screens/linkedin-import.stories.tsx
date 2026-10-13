// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../testing/mefixture";
import { LinkedInNetworkPanel } from "./linkedin-import";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The LinkedIn network panel for the fe-uat render gate: the import as a
// connection row, the profile row, the reach section and the CSV how-to. It is
// a personal surface, so the stories differ only in what the reads return. The
// profile editor is a Modal in the document body, so its story scopes `play`
// past `canvasElement`.

type Account = Readonly<{
  profile_url?: string;
  connected?: boolean;
  connections?: number;
}>;

const NO_REACH = { accounts: [], accounts_total: 0, unresolved_connections: 0 };

const REACHED = {
  accounts: [
    {
      company_id: "018f3a1b-0000-7000-8000-0000000000a1",
      display_name: "Nordwind Logistik GmbH",
      connections: 14,
      contacts_on_file: 3,
    },
  ],
  accounts_total: 1,
  unresolved_connections: 1420,
};

const IMPORTED: Account = {
  profile_url: "https://www.linkedin.com/in/lars-brandt",
  connected: true,
  connections: 4108,
};

function panelStory(
  account: Account,
  reach: unknown = NO_REACH,
  extra: Record<string, () => Response> = {},
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture()),
      "GET /me/linkedin-account": () =>
        jsonResponse({ connected: false, connections: 0, ...account }),
      "GET /me/linkedin-reach": () => jsonResponse(reach),
      ...extra,
    });
    return (
      <StoryProviders>
        <LinkedInNetworkPanel />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LinkedInNetworkPanel> = {
  title: "Settings/You/Connections/LinkedIn network",
  component: LinkedInNetworkPanel,
};
export default meta;
type Story = StoryObj<typeof LinkedInNetworkPanel>;

// The approved first visit: Not imported, profile Not added with Add profile,
// the reach empty state, and the how-to closed.
export const NothingYet: Story = { render: panelStory({ profile_url: "" }) };

export const NothingYetDark: Story = {
  globals: { theme: "dark" },
  render: panelStory({ profile_url: "" }),
};

// After an import: the count in the row's badge and the reach table below.
export const Imported: Story = { render: panelStory(IMPORTED, REACHED) };

export const ImportedDark: Story = {
  globals: { theme: "dark" },
  render: panelStory(IMPORTED, REACHED),
};

export const ImportedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: panelStory(IMPORTED, REACHED),
};

// The field itself, in the dialog the Edit verb opens.
export const EditingTheProfile: Story = {
  render: panelStory(IMPORTED, REACHED),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(await body.findByRole("button", { name: "Edit" }));
    await body.findByRole("textbox", { name: /LinkedIn profile URL/ });
  },
};

// What the import PRODUCED, reported under the row. Skipped rows are in the
// picture on purpose: a file half-ignored under a success message is worse
// than a refusal.
export const ImportSummary: Story = {
  render: panelStory({ profile_url: "" }, NO_REACH, {
    "POST /me/linkedin-connections": () =>
      jsonResponse({
        rows: 4120,
        imported: 4108,
        skipped: 12,
        confirmed: 37,
        suggested: 214,
      }),
  }),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.upload(
      body.getByTestId("linkedin-import-file"),
      new File(["First Name,Last Name\n"], "Connections.csv", {
        type: "text/csv",
      }),
    );
    await body.findByTestId("linkedin-import-result");
  },
};

export const AccountReadFailed: Story = {
  render: panelStory({}, NO_REACH, {
    "GET /me/linkedin-account": () =>
      jsonResponse(
        { title: "Internal Server Error", detail: "the profile store is down" },
        500,
      ),
  }),
};
