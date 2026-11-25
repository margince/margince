// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { IdentityLine } from "../design-system/identityline";
import { RecordDetails } from "../design-system/recorddetails";
import { VisibilityBadge } from "../design-system/visibility";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { useViewOnlyAccess, ViewOnly } from "./viewonly";

type Contact = components["schemas"]["Contact"];

// A record this reader may view but not change: the head's View only pill
// beside who can see it, the panel behind it, and the Access row it adds to
// Ownership. Every record type draws these two the same way.
const meta: Meta = {
  title: "Records/Record 360/View only",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const VIEWER = meFixture().user.id;
const OWNER = "01a05500-0000-7000-8000-0000000000e2";

const colleagues: Contact = {
  id: "01a05500-0000-7000-8000-0000000000d1",
  full_name: "Dana Buyer",
  source: "gmail:seed",
  captured_by: "connector:gmail",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  version: 7,
  visibility: "owner",
  owner_id: OWNER,
  writable: false,
};

const MAY_WRITE: GrantSpec = { contact: ["read", "update"] };

function serve({
  asked = false,
  seat = "full",
}: Readonly<{ asked?: boolean; seat?: "full" | "read" }> = {}) {
  installFetchStub({
    "GET /me": meRoute(MAY_WRITE, { seat }),
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: OWNER, display_name: "Lena Fischer" }] }),
    [`GET /records/contact/${colleagues.id}/edit-access-request`]: () =>
      jsonResponse(
        asked
          ? {
              approval_id: "a-1",
              record_type: "contact",
              record_id: colleagues.id,
              owner_id: OWNER,
              status: "pending",
              expires_at: "2026-12-09T09:00:00Z",
            }
          : null,
      ),
    [`GET /records/contact/${colleagues.id}/ownership-request`]: () =>
      jsonResponse(null),
  });
}

function Head({ contact }: Readonly<{ contact: Contact }>) {
  return (
    <IdentityLine separator="space">
      <Badge>Champion</Badge>
      <VisibilityBadge state="private" opens />
      <ViewOnly
        kind="contact"
        record={contact}
        name={contact.full_name}
        reasonId="view-only-reason"
      />
    </IdentityLine>
  );
}

function Ownership({ contact }: Readonly<{ contact: Contact }>) {
  const access = useViewOnlyAccess("contact", contact);
  return (
    <div style={{ maxInlineSize: "340px" }}>
      <RecordDetails
        sections={[
          {
            key: "ownership",
            label: "Ownership",
            rows: [{ key: "owner", term: "Owner", value: "Lena Fischer" }],
          },
        ]}
        access={access}
      />
    </div>
  );
}

async function openPill() {
  await userEvent.click(
    await screen.findByRole("button", { name: /Your access/ }),
  );
  await screen.findByText("Lena Fischer owns Dana Buyer");
}

/** Closed: "View only" beside "Private", one pill among the marks. */
export const Closed: Story = {
  render: () => {
    serve();
    return (
      <StoryProviders>
        <Head contact={colleagues} />
      </StoryProviders>
    );
  },
};

/** Open: who owns it, why, and the one filled ask; the handover ask under it. */
export const Open: Story = {
  render: () => {
    serve();
    return (
      <StoryProviders>
        <Head contact={colleagues} />
      </StoryProviders>
    );
  },
  play: async () => {
    await openPill();
    await expect(
      await screen.findByRole("button", {
        name: "Ask Lena Fischer for edit access",
      }),
    ).toBeVisible();
  },
};

/** Already asked: the panel says until when, and offers to withdraw. */
export const OpenAlreadyAsked: Story = {
  render: () => {
    serve({ asked: true });
    return (
      <StoryProviders>
        <Head contact={colleagues} />
      </StoryProviders>
    );
  },
  play: openPill,
};

/** The Ownership group's Access row with the ask as a ghost under a hairline. */
export const OwnershipRow: Story = {
  render: () => {
    serve();
    return (
      <StoryProviders>
        <Ownership contact={colleagues} />
      </StoryProviders>
    );
  },
};

/** A read seat on its own contact: View only, with no owner to ask. */
export const ReadSeat: Story = {
  render: () => {
    serve({ seat: "read" });
    const own = { ...colleagues, owner_id: VIEWER, writable: true };
    return (
      <StoryProviders>
        <div style={{ display: "grid", gap: "var(--space-4)" }}>
          <Head contact={own} />
          <Ownership contact={own} />
        </div>
      </StoryProviders>
    );
  },
};

/** Refused controls next to a live one: locked, unfilled, a lock in place of
 *  the ✦, the reason as a tip on hover. */
export const LockedButtons: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ display: "flex", gap: "var(--space-2)" }}>
        <Button
          variant="ai"
          reason="Only Lena Fischer can act on Dana. Ask Lena for edit access."
        >
          Draft a follow-up
        </Button>
        <Button variant="primary" reasonId="view-only-reason">
          Log a call
        </Button>
        <Button>Not now</Button>
      </div>
    </StoryProviders>
  ),
};
