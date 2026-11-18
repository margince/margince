// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CallStartProvider } from "../lib/callstart";
import { WriteToProvider } from "../lib/writeto";
import {
  callMessengersRoute,
  installFetchStub,
  StoryProviders,
} from "../testing/story-utils";
import { ContactLink, CopyEmail, EmailFace } from "./contactlink";
import { ReachCard, ReachLine } from "./reachcard";

// How to reach a record, as the card that opens its About. One line per
// address or number, its kind at the edge; a long address ends in "…".
const meta: Meta<typeof ReachCard> = {
  title: "Components/Text and data display/Reach card",
  component: ReachCard,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /installation/settings": callMessengersRoute(),
      });
      return (
        <StoryProviders>
          <WriteToProvider writeTo={() => {}}>
            <CallStartProvider start={() => {}}>
              <div style={{ maxWidth: 260 }}>
                <Story />
              </div>
            </CallStartProvider>
          </WriteToProvider>
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof ReachCard>;

const dana = { entityType: "contact", entityId: "p-1" } as const;

function Address({ value }: Readonly<{ value: string }>) {
  return (
    <span className="contactlink-copyable">
      <ContactLink kind="email" value={value} record={dana}>
        <EmailFace address={value} />
      </ContactLink>
      <CopyEmail address={value} />
    </span>
  );
}

/** One address and one number, each with its kind. */
export const OneOfEach: Story = {
  render: () => (
    <ReachCard>
      <ReachLine kind="email" text="dana@brandt.example" caption="Work">
        <Address value="dana@brandt.example" />
      </ReachLine>
      <ReachLine kind="phone" text="+493012345678" caption="Mobile">
        <ContactLink kind="phone" value="+493012345678" record={dana} />
      </ReachLine>
    </ReachCard>
  ),
};

/** Several, one too long for the card: it ends in "…", its kind stays. */
export const Several: Story = {
  render: () => (
    <ReachCard>
      <ReachLine kind="email" text="dana@brandt.example" caption="Work">
        <Address value="dana@brandt.example" />
      </ReachLine>
      <ReachLine
        kind="email"
        text="dana.buyer@brandt-automotive-group.example"
        caption="Work"
      >
        <Address value="dana.buyer@brandt-automotive-group.example" />
      </ReachLine>
      <ReachLine kind="email" text="dana.b@posteo.example" caption="Personal">
        <Address value="dana.b@posteo.example" />
      </ReachLine>
      <ReachLine kind="phone" text="+493012345678" caption="Work">
        <ContactLink kind="phone" value="+493012345678" record={dana} />
      </ReachLine>
      <ReachLine kind="phone" text="+4917612345678" caption="Mobile">
        <ContactLink kind="phone" value="+4917612345678" record={dana} />
      </ReachLine>
    </ReachCard>
  ),
};

/** Only a number: the card holds what the record has. */
export const PhoneOnly: Story = {
  render: () => (
    <ReachCard>
      <ReachLine kind="phone" text="+4917612345678">
        <ContactLink kind="phone" value="+4917612345678" record={dana} />
      </ReachLine>
    </ReachCard>
  ),
};
