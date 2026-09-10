// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { IntroAsksCard } from "./introasks";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The asks in flight about a contact. The pane says different things to each
// side of an ask, so both sides are stories: the colleague being asked is
// offered the answer, and the rep who asked is offered the withdrawal. A
// catalog that showed only one of them would show half the component.

type IntroRequest = components["schemas"]["IntroRequest"];

const PERSON = "018f3a1b-0000-7000-8000-000000000010";
const REQUESTER = "018f3a1b-0000-7000-8000-000000000031";
// The reader themselves. Which verbs the rows offer is decided by this id
// matching one of the two on the ask, so it is read from the session fixture
// rather than typed a second time here.
const READER = meFixture({}).user.id;

function ask(over: Partial<IntroRequest>): IntroRequest {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000a1",
    person_id: PERSON,
    requester_user_id: REQUESTER,
    introducer_user_id: READER,
    route_type: "direct",
    internal_reason: "she ran the migration we are pitching",
    status: "requested",
    name_drop_allowed: false,
    fallback_policy: "none",
    note_generated_by: "human",
    note_ai_generated: false,
    requested_at: "2026-09-01T09:00:00Z",
    due_at: "2026-09-08T09:00:00Z",
    version: 1,
    ...over,
  };
}

function asks(rows: IntroRequest[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        person: ["read"],
        introduction: ["read", "update"],
      }),
      [`GET /people/${PERSON}/intro-requests`]: () =>
        jsonResponse({ data: rows }),
    });
    return (
      <StoryProviders>
        <IntroAsksCard personId={PERSON} personName="Dana Buyer" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof IntroAsksCard> = {
  title: "Records/Person network/Introductions",
  component: IntroAsksCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof IntroAsksCard>;

/** The colleague being asked: the one row still open, with the answer on it. */
export const BeingAsked: Story = { render: asks([ask({})]) };

/**
 * The rep who asked, reading the same payload from the other side: no answer
 * to give, a withdrawal to make, and a settled row that offers neither.
 */
export const Requester: Story = {
  render: asks([
    ask({ requester_user_id: READER, introducer_user_id: REQUESTER }),
    ask({
      id: "018f3a1b-0000-7000-8000-0000000000a2",
      requester_user_id: READER,
      introducer_user_id: REQUESTER,
      status: "introduced",
      internal_reason: "he owns the account we are being routed through",
    }),
  ]),
};

/**
 * Dark, because the pane's ground and the quiet status badge on each row are
 * two elevations a darker palette compresses toward each other.
 */
export const BeingAskedDark: Story = {
  name: "Being asked — dark",
  globals: { theme: "dark" },
  render: asks([ask({})]),
};
