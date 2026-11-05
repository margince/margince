// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyReachCard } from "./company-reach";

// CompanyReachCard stories for the fe-uat render gate. Three cases read
// differently and the card has to keep them apart. Colleagues who can open the
// door, nobody who can, and a read that failed. A refused read drawn as
// "nobody" reads as a cold door.

const COMPANY = "018f3a1b-0000-7000-8000-0000000000c1";

function reachStory(body: unknown, status = 200) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture()),
      [`GET /companies/${COMPANY}/linkedin-reach`]: () =>
        jsonResponse(body, status),
    });
    return (
      <StoryProviders>
        <CompanyReachCard companyId={COMPANY} companyName="Acme GmbH" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CompanyReachCard> = {
  title: "Records/Company 360/Who can get you in",
  component: CompanyReachCard,
};
export default meta;
type Story = StoryObj<typeof CompanyReachCard>;

/** Two colleagues can open this door, the better route first. */
export const Reached: Story = {
  render: reachStory({
    colleagues: [
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u1",
        display_name: "Dana Okonjo",
        connections: 3,
        functions: [{ function: "it", connections: 2 }],
      },
      {
        user_id: "018f3a1b-0000-7000-8000-0000000000u2",
        display_name: "Minh Tran",
        connections: 1,
        functions: [],
      },
    ],
  }),
};

/** No reach is shown for this company, said in words rather than as an
 *  empty table. A colleague who switched their reach off reads the same way,
 *  deliberately: telling the two apart would disclose their choice. */
export const NoReachShown: Story = {
  render: reachStory({ colleagues: [] }),
};

/** The read was refused. Not drawn as "nobody": this reader was not given the
 *  list, which is a different fact about a door that may be warm. */
export const ReadRefused: Story = {
  render: reachStory({ title: "Forbidden", detail: "no" }, 403),
};
