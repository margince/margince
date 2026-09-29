// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { Badge } from "../design-system/atoms";
import { IdentityLine } from "../design-system/identityline";
import { RecordAccess } from "./recordaccess";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

type Contact = components["schemas"]["Contact"];
type Company = components["schemas"]["Company"];

// Who may read a record, as the chip in its header's row of marks. The closed
// chip is one pill among the others; most stories open it, because what
// differs between readers is the sentence and the switch behind it.
const meta: Meta = {
  title: "Records/Record access",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The session the stories sign in as: the chip compares the record's owner
// with this id to decide whose record it is.
const VIEWER = meFixture().user.id;
const COLLEAGUE = "01a05500-0000-7000-8000-0000000000e2";

const base: Contact = {
  id: "01a05500-0000-7000-8000-0000000000d1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 7,
  owner_id: VIEWER,
  writable: true,
};

const MAY_WRITE: GrantSpec = { contact: ["read", "update"] };

function Access({
  contact,
  seat = "full",
  patchStatus = 200,
}: Readonly<{
  contact: Contact;
  seat?: "full" | "read";
  patchStatus?: number;
}>) {
  installFetchStub({
    "GET /me": meRoute(MAY_WRITE, { seat }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: COLLEAGUE, display_name: "Mira Voss" }],
        page: { has_more: false, next_cursor: null },
      }),
    [`PATCH /contacts/${base.id}`]: () =>
      jsonResponse(
        { status: patchStatus, title: "The server could not save this." },
        patchStatus,
      ),
  });
  return (
    <StoryProviders>
      {/* The header's row of marks, so the chip is seen at the height and
          interval it keeps beside the standing badge. */}
      <IdentityLine separator="space">
        <Badge>Thin</Badge>
        <RecordAccess kind="contact" record={contact} />
      </IdentityLine>
    </StoryProviders>
  );
}

async function openChip() {
  // The panel portals to the body, so it is reached through `screen`.
  await userEvent.click(
    await screen.findByRole("button", { name: /Who can see this contact/ }),
  );
  await screen.findByRole("region", { name: /Who can see this contact/ });
}

/** Closed: one pill beside the others, of the same height, with a caret
 *  saying it opens. Nothing beside it appears on hover. */
export const Shared: Story = {
  render: () => <Access contact={{ ...base, visibility: "workspace" }} />,
};

/** Open on a shared contact the reader may change: the sentence, the switch
 *  on its current answer, Save waiting for a different one, and the way to the
 *  full list. */
export const SharedOpen: Story = {
  render: () => <Access contact={{ ...base, visibility: "workspace" }} />,
  play: openChip,
};

/** The owner's own private contact: "Private" on the chip, and the sentence
 *  saying who else can read it. */
export const PrivateToTheReader: Story = {
  render: () => <Access contact={{ ...base, visibility: "owner" }} />,
  play: openChip,
};

/** A private contact open to a colleague it was shared with. The chip says
 *  the same "Private", never "Only you", and the sentence names the owner and
 *  why this reader can see it. The row is not theirs to write, so the switch
 *  gives way to the page's own refusal. */
export const PrivateToAColleague: Story = {
  render: () => (
    <Access
      contact={{
        ...base,
        visibility: "owner",
        owner_id: COLLEAGUE,
        writable: false,
      }}
    />
  ),
  play: openChip,
};

/** A read seat: the row would take the write, the seat will not, and the
 *  panel says so rather than offering a switch the server refuses. */
export const ReadOnlySeat: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "workspace" }} seat="read" />
  ),
  play: openChip,
};

/** An archived contact keeps its answer and the way to the full list; the
 *  switch gives way to the sentence the page states for every other write. */
export const Archived: Story = {
  render: () => (
    <Access
      contact={{
        ...base,
        visibility: "workspace",
        archived_at: "2026-08-02T00:00:00Z",
      }}
    />
  ),
  play: openChip,
};

/** A refused write: the error stays inside the panel, beside the switch it
 *  answers, and a toast says it too, for a reader who closed the panel. The
 *  unsaved answer stays chosen so Save can try again. */
export const WriteRefused: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "workspace" }} patchStatus={500} />
  ),
  play: async () => {
    await openChip();
    await userEvent.click(
      await screen.findByRole("radio", { name: /^Only the owner/ }),
    );
    await userEvent.click(await screen.findByRole("button", { name: "Save" }));
    await screen.findByRole("alert");
  },
};

/** A colleague holding a write grant on somebody else's contact: the owner's
 *  answer says it would take the contact out of their own reach. */
export const NotTheOwner: Story = {
  render: () => (
    <Access
      contact={{ ...base, visibility: "workspace", owner_id: COLLEAGUE }}
    />
  ),
  play: openChip,
};

/** A contact capture minted with no owner: the owner's answer is refused, and
 *  its help line says what comes first. */
export const NoOwner: Story = {
  render: () => (
    <Access contact={{ ...base, visibility: "workspace", owner_id: null }} />
  ),
  play: openChip,
};

/** Under a thumb: the chip keeps its pill and grows its target to the touch
 *  floor, and the panel stays within the phone's width. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: () => <Access contact={{ ...base, visibility: "owner" }} />,
  play: openChip,
};

const account: Company = {
  id: "01a05500-0000-7000-8000-0000000000c1",
  display_name: "Weber GmbH",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 4,
  owner_id: VIEWER,
  writable: true,
};

/** The same chip on an account, in the account's own words. */
export const AccountPrivateOpen: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ company: ["read", "update"] }) });
    return (
      <StoryProviders>
        <RecordAccess
          kind="company"
          record={{ ...account, visibility: "owner" }}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /Who can see this company/ }),
    );
    await screen.findByRole("radio", { name: /^All users in the company/ });
  },
};
