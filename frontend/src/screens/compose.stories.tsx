// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { ComposeModal, RelinkModal } from "./compose";
import type { Transport } from "./persontransports";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The composer surface (draftEmail / sendEmail / relinkActivity) rendered off
// the same fixture shapes compose.test.tsx exercises — never a live call. The
// interesting states are reachable only through the form (draft, send-confirm),
// so each story that needs one drives it in `play` with the same userEvent
// steps the unit tests use, keeping the captured frame faithful to a real run.

// One consent purpose is enough to satisfy the Send precondition and populate
// the purpose dropdown; its `label` is what the story clicks and its `key`
// ("transactional") is the wire value a send carries. Mirrors compose.test.tsx's
// PURPOSES, not an invented shape.
const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

const DRAFT: components["schemas"]["EmailDraft"] = {
  subject: "Re: Q3 numbers",
  body: "Thanks for the note — following up as promised.",
  to: ["buyer@acme.test"],
  ai_generated: true,
  ai_disclosure: "AI-assisted draft (Art. 50): reviewed and sent by a human.",
  // A voice-styled draft: the profile version is the provenance the banner
  // reports, and the reference is what a send or a discard binds its outcome
  // to. Both are null on a draft no voice profile shaped.
  voice_profile_version: 3,
  draft_ref: "vd-1",
};

// The owner's profile behind that draft, in the middle maturity band — the
// state that adds the provisional label to the disclosure banner.
const VOICE_PROFILE = {
  data: [
    {
      id: "vp-1",
      owner_id: "u1",
      status: "ready",
      maturity: "provisional",
      quality_band: "thin",
      voice_profile_md: "Short sentences. Concrete nouns.",
      profile_version: 3,
      personality_md: "",
      auto_learning_enabled: false,
      active_source_hash: null,
      candidate_version: null,
      last_built_at: null,
      source: "manual",
      captured_by: "human:u1",
      version: 1,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
      archived_at: null,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// The 422 body httperr.Validation actually emits: the top-level `code` is the
// same for every validation problem, and the rule that fired is named per
// field under details.errors. A story that hoisted the specific code to the
// top level would frame a refusal the server cannot produce.
function validationProblem(field: string, code: string, message: string) {
  return {
    code: "validation_error",
    title: "Unprocessable Entity",
    detail: message,
    details: { errors: [{ field, code, message }] },
  };
}

// Renders the composer over a given route map, always serving the consent
// purposes the purpose selector needs on top of the story's own routes.
function composeStory(routes: RouteMap) {
  return () => {
    installFetchStub({
      // Every story here renders a control that reads the session. Without
      // this route the stub's list-shaped fallback reads as a malformed one,
      // which fails every grant closed and draws a branch no story is named
      // for.
      "GET /me": meRoute({}),
      "GET /consent-purposes": () => jsonResponse(PURPOSES),
      ...routes,
    });
    return (
      <StoryProviders>
        <ComposeModal
          activityId="act-1"
          entityType="person"
          entityId="p-1"
          personId="p-1"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

// The composer, asserted to be on screen — the first step of every composer
// story below (RelinkModal's own story asserts its dialog the same way).
//
// This surface is a Modal, portalled to document.body, so `#storybook-root`
// holds the preview decorator and nothing else however well the composer
// renders. The render gate watches that root, which leaves it unable to tell a
// story whose component mounted NOTHING from one that worked: an empty capture
// passes under the story's name. A play that names what it expects is where the
// difference becomes visible, because a rejecting play IS a failure the gate
// reports — so a story here without one proves nothing.
async function composerOnScreen() {
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByLabelText("To");
}

// Fills the four Send preconditions (To, subject, body, purpose) then confirms
// — the same sequence fillSendableForm drives in compose.test.tsx, so a story
// reaches the send outcome (409 gate / 501 unavailable) it means to capture.
// `screen` rather than the story canvas throughout: this composer IS a Modal,
// portalled to document.body, so a canvas-scoped query searches an empty div.
async function fillAndSend() {
  await composerOnScreen();
  const canvas = screen;
  await userEvent.type(canvas.getByLabelText("To"), "buyer@acme.test");
  await userEvent.tab();
  await userEvent.type(canvas.getByPlaceholderText("Subject"), "Following up");
  await userEvent.type(canvas.getByPlaceholderText("Body"), "As promised.");
  // The purpose control is a button plus a listbox the component portals to the
  // body, so the option is reached OUTSIDE the story canvas — and by the label a
  // reader clicks, never by the wire key behind it.
  await userEvent.click(canvas.getByRole("combobox"));
  await userEvent.click(
    screen.getByRole("option", { name: PURPOSES.data[0].label }),
  );
  await userEvent.click(canvas.getByRole("button", { name: "Send" }));
}

const meta: Meta = {
  title: "Patterns/Compose mail",
};
export default meta;

type Story = StoryObj;

// The composer as it opens: empty fields, the draft bar, and Send disabled
// until the four preconditions are met.
export const Empty: Story = {
  render: composeStory({}),
  play: async () => {
    await composerOnScreen();
  },
};

// "Draft with AI" fills To/Subject/Body from the returned EmailDraft and
// discloses it: the Art. 50 banner, the voice version that styled it, and the
// provisional label its profile currently carries.
export const Drafted: Story = {
  render: composeStory({
    "GET /voice-profiles": () => jsonResponse(VOICE_PROFILE),
    "POST /activities/act-1/draft-email": () => jsonResponse(DRAFT),
  }),
  play: async () => {
    await composerOnScreen();
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
  },
};

// The thread arrived in a COLLEAGUE's mailbox. The reply still goes out from
// the reader's own, under their own name, so the composer says whose
// conversation this is before they commit to answering it.
//
// A plain info callout rather than the indigo band: the band claims machine
// provenance, and this is one person's mail landing in another's screen.
export const ColleaguesMailbox: Story = {
  render: composeStory({
    "GET /users": () =>
      jsonResponse({
        data: [
          {
            id: "00000000-0000-4000-8000-0000000000c1",
            display_name: "Charlotte Weber",
            email: "charlotte@demo.test",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /activities/act-1/reply-recipient": () =>
      jsonResponse({
        full_name: "Accounts Singapore",
        first_name: "Accounts",
        address: "accounts.singapore@vendor.test",
        // Not the story viewer, who is …0001 — which is the whole point: a
        // thread the reader's own mailbox took is not somebody else's.
        mailbox_user_ids: ["00000000-0000-4000-8000-0000000000c1"],
      }),
  }),
  play: async () => {
    await composerOnScreen();
  },
};

// The default-deny consent gate (A22/ADR-0011): a filled, confirmed send comes
// back 409 consent_not_granted, so the modal stays open with the pointed
// "Review consent" copy instead of a raw server error.
export const ConsentBlocked: Story = {
  render: composeStory({
    "POST /activities/act-1/send-email": () =>
      jsonResponse(
        {
          code: "consent_not_granted",
          title: "Conflict",
          detail: "suppressed",
        },
        409,
      ),
  }),
  play: async () => {
    await fillAndSend();
  },
};

// The mailbox is connected for capture but holds no send grant (422). The
// refusal names the only fix — reconnect — and links to the connect surface,
// because the provider will not widen an existing grant in place.
export const MailboxNotSendCapable: Story = {
  render: composeStory({
    "POST /activities/act-1/send-email": () =>
      jsonResponse(
        validationProblem(
          "from",
          "mailbox_not_send_capable",
          "reconnect your mailbox to enable sending",
        ),
        422,
      ),
  }),
  play: async () => {
    await fillAndSend();
  },
};

// An unsubscribe link carries one recipient's own consent credential, so a
// send addressed to more than one recipient is refused outright (422). The
// refusal states the one-address-at-a-time rule instead of the opaque server
// wording.
export const SharedUnsubscribeToken: Story = {
  render: composeStory({
    "POST /activities/act-1/send-email": () =>
      jsonResponse(
        validationProblem(
          "recipients",
          "shared_unsubscribe_token",
          "reaches one addressee at a time",
        ),
        422,
      ),
  }),
  play: async () => {
    await fillAndSend();
  },
};

// No mailer configured: the send answers 501, surfaced as an honest inline
// "Sending is unavailable" note, never thrown into the error channel.
export const SendUnavailable: Story = {
  render: composeStory({
    "POST /activities/act-1/send-email": () =>
      jsonResponse(
        { title: "Not Implemented", detail: "mailer not wired" },
        501,
      ),
  }),
  play: async () => {
    await fillAndSend();
  },
};

// The channel reply, over the conversation it answers. A channel send carries
// only the words and the consent purpose, so the server files the reply under
// the conversation's own links — which is why this composer STATES its filing
// where the mail one above offers a picker.
function channelReplyStory(conversation: RouteMap[string]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /consent-purposes": () => jsonResponse(PURPOSES),
      "GET /activities/act-1": conversation,
      "GET /projects/proj-1": () =>
        jsonResponse({ id: "proj-1", name: "ERP rollout", key: "ERP-27" }),
    });
    return (
      <StoryProviders>
        <ComposeModal
          activityId="act-1"
          entityType="person"
          entityId="p-1"
          personId="p-1"
          kind="message"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

const CHANNEL_CONVERSATION = {
  id: "act-1",
  kind: "message",
  channel_provider: "telegram",
  subject: null,
};

// The filed conversation: the reply says where it lands, and offers no choice
// about it — nothing on the wire could carry one.
export const ChannelReplyFiled: Story = {
  render: channelReplyStory(() =>
    jsonResponse({
      ...CHANNEL_CONVERSATION,
      links: [
        { entity_type: "person", entity_id: "p-1" },
        { entity_type: "project", entity_id: "proj-1" },
      ],
    }),
  ),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText(/Will be filed under ERP-27 · ERP rollout/);
  },
};

// The unfiled conversation: no project to name, so the line stays away rather
// than announcing a filing that is not happening.
export const ChannelReplyUnfiled: Story = {
  render: channelReplyStory(() =>
    jsonResponse({ ...CHANNEL_CONVERSATION, links: [] }),
  ),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByPlaceholderText("Body");
  },
};

// The relink dialog: a cross-object /search returns a few candidates that the
// RecordPicker lists once the user types a query.
export const Default: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "deal", id: "d-9", title: "Acme renewal" },
            { type: "company", id: "o-2", title: "Acme GmbH" },
            { type: "person", id: "pp-1", title: "Jane Doe" },
          ],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RelinkModal
          activityId="act-1"
          entityType="person"
          entityId="p-1"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByRole("searchbox"), "Acme");
  },
};

// --- The head, and the two things a rep could not reach from here before -----

// A contact with a second address and paper on file, so the recipient offer and
// the attachment shelf both have something real to draw. Everything the head
// reads comes off these two — the 360 the drawer already holds, and the
// record's own library.
const CONTACT_360 = {
  as_of: "2026-08-15T09:00:00Z",
  person: {
    id: "p-1",
    full_name: "Dana Brandt",
    emails: [
      { email: "dana@brandt.example", is_primary: true },
      { email: "d.brandt@nordwand.example", is_primary: false },
    ],
  },
  sections_omitted: [],
};

const RECORD_FILES = {
  data: [
    {
      id: "att-1",
      entity_type: "person",
      entity_id: "p-1",
      filename: "Offer_Nordwand_v3.pdf",
      byte_size: 412_000,
    },
    {
      id: "att-2",
      entity_type: "person",
      entity_id: "p-1",
      filename: "Site_survey.jpg",
      byte_size: 2_100_000,
    },
    {
      id: "att-3",
      entity_type: "person",
      entity_id: "p-1",
      filename: "Retrofit_timeline.xlsx",
      byte_size: 88_000,
    },
  ],
  page: { has_more: false },
};

const HEAD_ROUTES: RouteMap = {
  "GET /me": meRoute({}),
  "GET /consent-purposes": () => jsonResponse(PURPOSES),
  "GET /people/p-1/360": () => jsonResponse(CONTACT_360),
  "GET /attachments": () => jsonResponse(RECORD_FILES),
};

// An account-started mail from a contact page: the head as a rep meets it.
function headStory(transports?: readonly Transport[], extra: RouteMap = {}) {
  return () => {
    installFetchStub({ ...HEAD_ROUTES, ...extra });
    return (
      <StoryProviders>
        <ComposeModal
          entityType="person"
          entityId="p-1"
          personId="p-1"
          recordAddress="dana@brandt.example"
          transports={transports}
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

/**
 * The head at rest: label beside field in one column a reader sweeps down, Cc
 * standing, Bcc still a button, and — under the body, where a mail client puts
 * it — the attachment shelf, drawn on every mail whether or not anything is
 * attached.
 *
 * What to look at: the value column's right edge is ONE margin with or without
 * a trailing control, and the To line's label sits on the field's own line
 * rather than drifting to the top of a box that grows.
 */
export const Head: Story = {
  render: headStory(),
  play: async () => {
    await composerOnScreen();
  },
};

/**
 * The recipient offer. A reader remembers a colleague's NAME and not their
 * address, so the person is the label and the address is the value — and the
 * one already standing in the To line is not offered back.
 */
export const RecipientOffer: Story = {
  render: headStory(),
  play: async () => {
    await composerOnScreen();
    await userEvent.click(screen.getByLabelText("To"));
  },
};

/** Bcc, once asked for: the row opens under Cc, with what "blind" means beside
 *  the field that does it. It never closes again — a field holding a value may
 *  not be hidden. */
export const BlindCopy: Story = {
  render: headStory(),
  play: async () => {
    await composerOnScreen();
    await userEvent.click(screen.getByRole("button", { name: "Bcc" }));
  },
};

/** The paperclip: the record's own paper first, an upload under it. That order
 *  on purpose — the file a rep means is usually already on the record, and a
 *  dropzone at the top would have them upload a second copy of it. */
export const Attaching: Story = {
  render: headStory(),
  play: async () => {
    await composerOnScreen();
    await userEvent.click(screen.getByRole("button", { name: /Attach/ }));
  },
};

/** Two files chosen: the shelf names each one with its size, and each carries
 *  the control that takes it back off before the send. */
export const Attached: Story = {
  render: headStory(),
  play: async () => {
    await composerOnScreen();
    await userEvent.click(screen.getByRole("button", { name: /Attach/ }));
    await userEvent.click(
      await screen.findByRole("button", { name: /^Offer_Nordwand_v3\.pdf/ }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /^Retrofit_timeline\.xlsx/ }),
    );
  },
};

/**
 * The transport dial, on a contact reachable both by mail and on a chat
 * channel. It is drawn only where there IS a choice — one transport is not a
 * decision — and it sits above the fields it governs, because changing it
 * changes what the head even is.
 */
export const TransportChoice: Story = {
  render: headStory([
    { id: "email", label: "Email" },
    { id: "dispact", label: "Dispact", anchorId: "act-chat" },
  ]),
  play: async () => {
    await composerOnScreen();
  },
};

/** The head in dark, where every value re-resolves. */
export const HeadDark: Story = {
  globals: { theme: "dark" },
  render: headStory(),
  play: async () => {
    await composerOnScreen();
  },
};
