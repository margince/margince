// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { LinkedInNetworkPanel } from "./linkedin-import";

// The LinkedIn network panel for the fe-uat render gate. It shows the import as
// a connection row with its drop zone, the profile row, the reach and the
// how-to. It is a personal surface, so the stories differ only in what the
// reads and the import return. The profile editor is a Modal in the document
// body, so its story scopes `play` past `canvasElement`.

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
  extra: RouteMap = {},
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

async function dropTheExport(canvasElement: HTMLElement) {
  await userEvent.upload(
    await within(canvasElement).findByLabelText("Import Connections.csv"),
    new File(["First Name,Last Name\n"], "Connections.csv", {
      type: "text/csv",
    }),
  );
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

// What the import produced, reported under the row. Skipped rows stay in the
// picture: a file half-ignored under a success message is worse than a refusal.
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
    await dropTheExport(canvasElement);
    await within(canvasElement).findByTestId("linkedin-import-result");
  },
};

// The export is being read: the wait stands under the row in words.
export const Importing: Story = {
  render: panelStory({ profile_url: "" }, NO_REACH, {
    "POST /me/linkedin-connections": () => new Promise<Response>(() => {}),
  }),
  play: async ({ canvasElement }) => {
    await dropTheExport(canvasElement);
    await within(canvasElement).findByText("Reading export…");
  },
};

// The file was refused: the server's reason under the row, the zone still
// there to pick the right file.
export const ImportFailed: Story = {
  render: panelStory({ profile_url: "" }, NO_REACH, {
    "POST /me/linkedin-connections": () =>
      jsonResponse(
        { title: "Unprocessable", detail: "This file is not Connections.csv." },
        422,
      ),
  }),
  play: async ({ canvasElement }) => {
    await dropTheExport(canvasElement);
    await within(canvasElement).findByText(
      "The export was not imported. Retry.",
    );
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

// The reach read failed: the panel says so with a retry where the table stands,
// and the import above it still works.
export const ReachReadFailed: Story = {
  render: panelStory(IMPORTED, NO_REACH, {
    "GET /me/linkedin-reach": () =>
      jsonResponse(
        { title: "Internal Server Error", detail: "the reach index is down" },
        500,
      ),
  }),
};
