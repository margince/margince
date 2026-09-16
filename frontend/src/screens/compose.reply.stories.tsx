import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ChannelReplyAction } from "./compose.reply";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The reply affordance for one captured conversation. Three facts about it and
// no more, which is the whole component:
//
//  - on a MAIL row it is always offered, and its word is "Reply";
//  - on a CHANNEL row it is offered only where the contact is reachable on the
//    transport that carried the conversation, and withheld where they are not;
//  - where the row's content is withheld it keeps working but changes its WORD
//    to "Write email", because writing to a contact is not answering a message
//    nobody may read.
//
// Every story keeps the row behind the verb, for two reasons. The verb belongs
// to a row and reads as a loose button without one — and the last story's whole
// subject is that NOTHING is drawn, which is unreadable on an empty canvas and
// which fe-uat cannot tell from a render that failed.
//
// The drawer the verb opens is `ComposeModal`, whose own gallery is
// compose.stories.tsx. Opening it here would be a second, thinner set of
// frames for a surface that already has its own.

const meta: Meta<typeof ChannelReplyAction> = {
  title: "Records/Channel reply action",
  component: ChannelReplyAction,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ChannelReplyAction>;

// Whether this contact can be reached on one transport. The shape is the
// record read's own, because that read is what the gate consults — see
// useChannelReachable, which matches the row's provider against this list.
function contactReachable(provider: string, reachable: boolean) {
  return () =>
    jsonResponse({
      id: "p-1",
      full_name: "Jane Doe",
      reachability: [{ provider, reachable, since: "2026-07-01T00:00:00Z" }],
      version: 1,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    });
}

// The timeline row the verb sits at the end of, drawn flat rather than through
// the real row: this gallery is about the verb, and a row that fetched its own
// activity would put a second surface's states in these frames.
function Row({
  when,
  said,
  children,
}: Readonly<{ when: string; said: string; children: ReactNode }>) {
  return (
    <div className="wrap">
      <p className="t-caption">{when}</p>
      <p className="t-body">{said}</p>
      {children}
    </div>
  );
}

/** A mail row: the reply is always offered, because reaching a contact by mail
 *  is not a fact this gate has an opinion about. */
export const MailReply: Story = {
  render: () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <Row
          when="24 August, 10:00"
          said="Can we align the payment schedule with our fiscal quarters?"
        >
          <ChannelReplyAction
            activityId="a-1"
            kind="email"
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
          />
        </Row>
      </StoryProviders>
    );
  },
};

/** A channel row whose contact IS reachable on the transport that carried it.
 *  The word is the same as mail's: this is an answer to the message above. */
export const ChannelReply: Story = {
  render: () => {
    installFetchStub({
      "GET /contacts/p-1": contactReachable("telegram", true),
    });
    return (
      <StoryProviders>
        <Row when="24 August, 10:04" said="Sent you the revised annex on here.">
          <ChannelReplyAction
            activityId="a-2"
            kind="message"
            channelProvider="telegram"
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
          />
        </Row>
      </StoryProviders>
    );
  },
};

/** The row's content is not this reader's to see. The verb still works — writing
 *  to the contact is not reading their mail — so it changes its word rather than
 *  going away, and what it opens is an account-started email, not a reply. */
export const WithheldWritesEmail: Story = {
  render: () => {
    installFetchStub({
      "GET /contacts/p-1": contactReachable("telegram", true),
    });
    return (
      <StoryProviders>
        <Row
          when="24 August, 10:04"
          said="This message is not shared with you."
        >
          <ChannelReplyAction
            activityId="a-3"
            kind="message"
            channelProvider="telegram"
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
            contentWithheld
          />
        </Row>
      </StoryProviders>
    );
  },
};

/** Not reachable on the transport that carried the conversation, so NOTHING is
 *  offered: a reply this rep could not send is worse than no verb at all, and
 *  the same gate answers on every surface that draws this row. The row stays so
 *  the absence is something a reader can see. */
export const UnreachableOffersNothing: Story = {
  render: () => {
    installFetchStub({
      "GET /contacts/p-1": contactReachable("telegram", false),
    });
    return (
      <StoryProviders>
        <Row when="24 August, 10:04" said="Sent you the revised annex on here.">
          <ChannelReplyAction
            activityId="a-4"
            kind="message"
            channelProvider="telegram"
            entityType="contact"
            entityId="p-1"
            contactId="p-1"
          />
        </Row>
      </StoryProviders>
    );
  },
};
