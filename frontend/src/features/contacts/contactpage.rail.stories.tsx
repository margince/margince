// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { meFixture } from "../../testing/mefixture";
import { ContactRail } from "./contactrail";
import "./contact360.css";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  channelReached,
  page,
  populated,
  type View,
} from "./contactpage.fixtures";

// The contact page's left column, from the page gallery
// (contactpage.stories.tsx). Its records come from contactpage.fixtures.ts.

// The column with the guard already answered, as the page hands it over.
function LeftColumn({
  view,
  guard,
}: Readonly<{
  view: View;
  guard: components["schemas"]["ContactConsentGuard"] | undefined;
}>) {
  return (
    <ContactRail
      view={view}
      consent={{
        guard,
        loading: false,
        failed: false,
        onRetry: () => undefined,
      }}
      writeRefusalId="write-refused"
    />
  );
}

const meta: Meta = {
  title: "Records/Contact 360/Page rail",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// --- Rail --------------------------------------------------------------------

export const Rail: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn
            view={populated}
            guard={{
              contact_id: "p-1",
              entries: [
                {
                  purpose_key: "business_correspondence",
                  purpose_class: "business_correspondence",
                  channel: "email",
                  verdict: "allowed",
                  reason: "She wrote to you on 1 Aug 2026.",
                },
                {
                  purpose_key: "phone_outreach",
                  purpose_class: "phone_outreach",
                  channel: "phone",
                  verdict: "unknown",
                  reason: "No consent recorded.",
                },
              ],
            }}
          />
        </div>
      </StoryProviders>
    );
  },
};

// The Companies section with two employment edges (one current, one former),
// so the current/former distinction and the add/mark-ended/remove verbs
// are all on screen at once.
const twoEmployers: View = {
  ...populated,
  employments: {
    data: [
      {
        relationship_id: "rel-1",
        company_id: "o-1",
        company_name: "Brandt Automotive GmbH",
        role: "Head of Fleet",
        is_current_primary: true,
        started_at: "2022-03-01T00:00:00Z",
        ended_at: null,
      },
      {
        relationship_id: "rel-2",
        company_id: "o-2",
        company_name: "Voss Logistics",
        role: "Fleet Coordinator",
        is_current_primary: false,
        started_at: "2018-01-01T00:00:00Z",
        ended_at: "2022-02-01T00:00:00Z",
      },
    ],
    page,
  },
};

export const RailEmployments: Story = {
  render: () => {
    // The rail's editable rows read the grant map, so the fixture is the /me
    // shape, not a hand-written stand-in. An object absent from `allow` is
    // absent the way the server omits it. A client that treats "missing" as
    // "permitted" gets that case wrong.
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn view={twoEmployers} guard={undefined} />
        </div>
      </StoryProviders>
    );
  },
};

// The rail's consent slot when a purpose is refused rather than merely
// unrecorded: verdictClass (contactrail.tsx) reads this as the refused/warning
// treatment. The readings above carry no consent slot (the header's Write
// verb states the refusal), so this is the one surface that draws it.
export const RailConsentBlocked: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn
            view={populated}
            guard={{
              contact_id: "p-1",
              entries: [
                {
                  purpose_key: "business_correspondence",
                  purpose_class: "business_correspondence",
                  channel: "email",
                  verdict: "blocked",
                  reason:
                    "She opted out of business correspondence on 3 Aug 2026.",
                },
                {
                  purpose_key: "phone_outreach",
                  purpose_class: "phone_outreach",
                  channel: "phone",
                  verdict: "unknown",
                  reason: "No consent recorded.",
                },
              ],
            }}
          />
        </div>
      </StoryProviders>
    );
  },
};

export const RailChannelReached: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
      // A transport is named by the directory, so the story has to serve it.
      // Without this the rows fall back to the raw provider ids. That is the
      // resolver's correct behaviour, and not what this story is about.
      "GET /channel-providers": () =>
        jsonResponse({
          data: [
            {
              provider: "zalo_oa",
              label: "Zalo OA",
              credential_model: "workspace_bot",
              supplies_transport: true,
            },
            {
              provider: "dispact",
              label: "Dispact",
              credential_model: "per_member",
              supplies_transport: true,
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn
            view={channelReached}
            guard={{
              contact_id: "p-1",
              entries: [
                {
                  purpose_key: "business_correspondence",
                  purpose_class: "business_correspondence",
                  channel: "email",
                  verdict: "allowed",
                  reason: "She wrote to you on 1 Aug 2026.",
                },
                {
                  purpose_key: "phone_outreach",
                  purpose_class: "phone_outreach",
                  channel: "phone",
                  verdict: "unknown",
                  reason: "No consent recorded.",
                },
              ],
            }}
          />
        </div>
      </StoryProviders>
    );
  },
};

// A record with an open deal, an empty committee, no meeting booked, and a
// last inbound reply well past the 14-day threshold. It fires three of
// derivedSignals' four warnings at once (contactrail.tsx), plus the pulse's
// "cooling" trend and "at risk" overall word. No other fixture here carries a deal.
const dealAtRisk: View = {
  ...populated,
  last_inbound_at: "2026-07-10T09:00:00Z",
  last_outbound_at: "2026-07-25T09:00:00Z",
  commercial: {
    role: "champion",
    deal: {
      deal_id: "d-1",
      title: "Fleet retrofit, 40 vehicles",
      stage: "negotiation",
      amount_minor: 9_500_000,
      currency: "EUR",
      close_date: "2026-09-30",
    },
    committee: [],
  },
  next_meeting: undefined,
};

export const RailAtRisk: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn view={dealAtRisk} guard={undefined} />
        </div>
      </StoryProviders>
    );
  },
};

// The thin end of every rail section at once. No reply ever captured, so the
// pulse's "thin" overall word and the signals' empty state both render.
// One direction only (the one-sided reading, since twoWay needs both). No
// employer, nobody who knows her, and nothing in the recent-activity list.
// A record can look like this the day after it is captured.
const thinContact: View = {
  ...populated,
  last_inbound_at: undefined,
  last_outbound_at: "2026-08-10T09:00:00Z",
  network: { colleagues: [] },
  employments: { data: [], page },
  activities: { data: [], page },
  commercial: { role: null, committee: [] },
  next_meeting: undefined,
};

export const RailThin: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn view={thinContact} guard={undefined} />
        </div>
      </StoryProviders>
    );
  },
};

// Every profile field DetailsGrid can hold, unset at once: title, linkedin,
// city, email and phone all blank. Email and phone have no inline editor in the
// rail (the record's Edit action edits them). They read `field.unset` whether
// or not the reader can edit. Title, linkedin and city are editable
// under this fixture's granted /me, so they read as the "Add …" placeholder.
// The two empty-field states sit side by side.
const unsetFields: View = {
  ...populated,
  contact: {
    ...populated.contact,
    title: null,
    social: {},
    address: undefined,
    emails: [],
    primary_email: null,
    phones: [],
  },
};

export const RailUnsetFields: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { contact: ["update"] } })),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <LeftColumn view={unsetFields} guard={undefined} />
        </div>
      </StoryProviders>
    );
  },
};
