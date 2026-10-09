// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  expect,
  fireEvent,
  screen,
  userEvent,
  waitFor,
  within,
} from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import { PrivacyInboxCard } from "./privacy";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The DSR inbox: the open form, a request worked in its drawer, and fulfilling
// an erasure. A legal-hold 409 never carries a retention date; nor does a story.

const DSRS = {
  data: [
    {
      id: "d1",
      kind: "erasure",
      subject_ref: "8f3a-contact-uuid",
      status: "open",
      due_at: "2026-08-01T00:00:00Z",
      created_at: "2026-07-01T00:00:00Z",
    },
    {
      id: "d2",
      kind: "access",
      subject_ref: "anna@acme.test",
      status: "fulfilled",
      resolution: "sent by post",
      due_at: "2026-07-12T00:00:00Z",
      created_at: "2026-06-01T00:00:00Z",
    },
    {
      id: "d3",
      kind: "rectify",
      subject_ref: "00000000-0000-4000-8000-0000000000c3",
      subject_label: "Lena Hoffmann",
      status: "in_progress",
      assignee_id: "u-1",
      due_at: "2099-11-30T00:00:00Z",
      created_at: "2026-06-01T00:00:00Z",
    },
    {
      id: "d4",
      kind: "access",
      subject_ref: "00000000-0000-4000-8000-0000000000c4",
      subject_label: null,
      status: "rejected",
      resolution: "No record of this address",
      due_at: "2026-05-01T00:00:00Z",
      created_at: "2026-04-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// The longest a subject runs: a partner's own reference typed in by hand, and a
// contact whose name fills a phone's width on its own.
const LONG = {
  data: [
    {
      ...DSRS.data[0],
      id: "d5",
      subject_ref:
        "partner-escalation-reference-2026-0001234567@subsidiary.example-holdings.test",
    },
    {
      ...DSRS.data[2],
      id: "d6",
      subject_label:
        "Maximiliane Theodora von Hohenzollern-Sigmaringen-Wittelsbach",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// The card asks three separate object grants, and a session holding none of
// them draws "seeing subject requests needs permission" under every name here —
// rows, form and confirm alike. A role name does not stand in for them:
// `meFixture` seats an admin by default, so a session naming only a role reads
// as fully authorised while every `useCan` on it answers false.
//
//   privacy_request:read   — the queue itself (consent/dsr.go), and the query
//                            is disabled without it, so no row ever arrives
//   privacy_request:update — the transition verbs and the erasure fulfil
//   contact:update          — OPENING a request, which writes the contact named
//
// Every story here is an officer working the queue, so every one holds all
// three; the refusals each grant governs are privacy.test.tsx's subject.
const WORKS_SUBJECT_REQUESTS: GrantSpec = {
  privacy_request: ["read", "update"],
  contact: ["update"],
};

// The one assignee the fixtures name, and the roster the drawer's picker offers.
const SEATS: RouteMap = {
  "GET /users/names": () =>
    jsonResponse({ data: [{ id: "u-1", display_name: "Anna Weber" }] }),
  "GET /users": () =>
    jsonResponse({
      data: [
        {
          id: "u-1",
          email: "anna@margince.test",
          display_name: "Anna Weber",
          status: "active",
          is_agent: false,
        },
      ],
      page: { next_cursor: null, has_more: false },
    }),
};

function inbox(routes: RouteMap) {
  return () => {
    stubWithSession({ ...SEATS, ...routes }, WORKS_SUBJECT_REQUESTS);
    return (
      <StoryProviders>
        <PrivacyInboxCard />
      </StoryProviders>
    );
  };
}

async function expandRow(canvasElement: HTMLElement, subjectRef: string) {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: new RegExp(subjectRef, "i") }),
  );
}

// The facet bar's "Fulfilled" substring-matches /fulfil/i too, so a request's
// verbs are looked up in its drawer, which portals to the body.
function findRow(subjectRef: string): Promise<HTMLElement> {
  return screen.findByRole("dialog", { name: subjectRef });
}

const meta: Meta<typeof PrivacyInboxCard> = {
  title: "Settings/Governance/Privacy and retention/Subject requests",
  component: PrivacyInboxCard,
};
export default meta;

type Story = StoryObj<typeof PrivacyInboxCard>;

// An overdue erasure, a fulfilled access request, a named correction in
// progress and a rejected request whose contact this reader cannot see.
export const Inbox: Story = {
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
};

// A still-open request in its drawer: subject, assignee, and only the
// transitions the server's status machine would accept.
export const RequestOpen: Story = {
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
  play: async ({ canvasElement }) => {
    await expandRow(canvasElement, "8f3a-contact-uuid");
  },
};

// The folded table at 390px: subject and status on line one, the rest below.
export const InboxPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
};

// The drawer is a full-screen sheet on a phone.
export const RequestOpenPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
  play: async ({ canvasElement }) => {
    await expandRow(canvasElement, "8f3a-contact-uuid");
  },
};

export const LongSubjects: Story = {
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(LONG) }),
};

export const LongSubjectsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(LONG) }),
};

// A refused transition says so in the drawer; the badges stay where they were.
export const RefusedWrite: Story = {
  render: inbox({
    "GET /data-subject-requests": () => jsonResponse(DSRS),
    "PATCH /data-subject-requests/d3": () =>
      jsonResponse(
        { title: "Forbidden", status: 403, code: "permission_denied" },
        403,
      ),
  }),
  play: async ({ canvasElement }) => {
    await expandRow(canvasElement, "Lena Hoffmann");
    const drawer = await findRow("Lena Hoffmann");
    await userEvent.type(within(drawer).getByLabelText(/resolution/i), "done");
    await userEvent.click(
      within(drawer).getByRole("button", { name: /reject/i }),
    );
    await within(drawer).findByRole("alert");
  },
};

export const Loading: Story = {
  render: inbox({ "GET /data-subject-requests": () => new Promise(() => {}) }),
};

export const ReadFailed: Story = {
  render: inbox({
    "GET /data-subject-requests": () =>
      jsonResponse(
        { title: "Internal Server Error", status: 500, code: "internal" },
        500,
      ),
  }),
};

// G-2: the inline open-request form (kind defaults to access — the
// free-text subject field, not the erasure RecordPicker).
export const NewRequestForm: Story = {
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /new request/i }),
    );
  },
};

// Enter in the contact search, once a contact is picked and the form is
// complete, opens nothing: a submitting Enter would file the request for the
// earlier contact while the officer was looking for another. A scripted key
// never submits a form, so the play asserts the browser's own Enter action is
// cancelled, then that the form was complete enough for it to have submitted.
const openedRequests: unknown[] = [];
export const ErasureSearchEnter: Story = {
  render: inbox({
    "GET /data-subject-requests": () => jsonResponse(DSRS),
    "GET /contacts": () =>
      jsonResponse({
        data: [
          { id: "c-anna", full_name: "Anna Weber" },
          { id: "c-ben", full_name: "Ben Ostrowski" },
        ],
        page: { next_cursor: null, has_more: false },
      }),
    "POST /data-subject-requests": (body) => {
      openedRequests.push(body);
      return jsonResponse({ ...DSRS.data[0], subject_ref: "c-anna" }, 201);
    },
  }),
  play: async ({ canvasElement }) => {
    openedRequests.length = 0;
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: /new request/i,
      }),
    );
    await user.click(await screen.findByRole("combobox", { name: "Kind" }));
    await user.click(await screen.findByRole("option", { name: "Erasure" }));
    const search = await screen.findByRole("searchbox", { name: "Contact" });
    await user.type(search, "anna");
    await user.click(await screen.findByRole("button", { name: "Anna Weber" }));
    fireEvent.change(screen.getByLabelText("Due"), {
      target: { value: "2026-08-01" },
    });
    await user.type(search, "ben");
    const enter = new KeyboardEvent("keydown", {
      key: "Enter",
      bubbles: true,
      cancelable: true,
    });
    await expect(search.dispatchEvent(enter)).toBe(false);
    await user.click(screen.getByRole("button", { name: "Open request" }));
    await waitFor(() => expect(openedRequests).toHaveLength(1));
  },
};

// Opens the fulfil confirm on the erasure request and types the word that arms
// it. The drawer and the confirm both portal to the body, outside the canvas.
async function armErasureConfirm(canvasElement: HTMLElement) {
  await expandRow(canvasElement, "8f3a-contact-uuid");
  const row = await findRow("8f3a-contact-uuid");
  await userEvent.type(within(row).getByLabelText(/resolution/i), "verified");
  await userEvent.click(within(row).getByRole("button", { name: /fulfil/i }));
  await userEvent.type(await screen.findByLabelText(/type erase/i), "ERASE");
}

// The typed-ERASE confirm modal for the destructive erasure fulfil —
// confirmVariant="danger" throughout, distinct from every routine transition.
export const ErasureConfirm: Story = {
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
  play: async ({ canvasElement }) => {
    await armErasureConfirm(canvasElement);
  },
};

// Art. 17(3)(b): a documented, lawful refusal — never a red toast. The wire
// shape is the real one (erasure.go's ErrConflict): {type, title, status:
// 409, code: "conflict", detail} — no retain_until, ever.
const legalHoldRoutes: RouteMap = {
  "GET /data-subject-requests": () => jsonResponse(DSRS),
  "PATCH /data-subject-requests/d1": () =>
    jsonResponse(
      {
        type: "https://errors.gradion.com/conflict",
        title: "Conflict",
        status: 409,
        code: "conflict",
        detail: "erasing a contact under legal hold: conflict",
      },
      409,
    ),
};

const driveToLegalHold = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  await armErasureConfirm(canvasElement);
  // Portalled too, and the refusal it produces is portalled with it.
  await userEvent.click(
    screen.getByRole("button", { name: /erase and suppress/i }),
  );
  await screen.findByText(/legal hold/i);
};

export const LegalHoldBlocked: Story = {
  render: inbox(legalHoldRoutes),
  play: driveToLegalHold,
};

// The lawful refusal in dark: the danger callout's tint, border and text must
// stay separable from the drawer behind it.
export const LegalHoldBlockedDark: Story = {
  globals: { theme: "dark" },
  render: inbox(legalHoldRoutes),
  play: driveToLegalHold,
};

export const Forbidden: Story = {
  render: inbox({
    "GET /data-subject-requests": () =>
      jsonResponse(
        { title: "permission denied", status: 403, code: "permission_denied" },
        403,
      ),
  }),
};

export const InboxDark: Story = {
  globals: { theme: "dark" },
  render: inbox({ "GET /data-subject-requests": () => jsonResponse(DSRS) }),
};

export const Empty: Story = {
  render: inbox({
    "GET /data-subject-requests": () =>
      jsonResponse({ data: [], page: { next_cursor: null, has_more: false } }),
  }),
};
