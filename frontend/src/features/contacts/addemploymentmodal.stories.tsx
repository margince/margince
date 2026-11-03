// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { Employers } from "./contactemployers";
import "./contact360.css";

// The dialog is opened the way a reader opens it — through the employment
// panel's own verb — so the `create` mutation it writes through and the
// already-connected list its picker excludes are the panel's real ones rather
// than a second set assembled here.
//
// The two stories differ in the ONE fact the modal reads off the record:
// whether this contact already holds a job nobody has ended. That decides
// whether "current employer" starts ticked, and a box that started on the
// other answer would show a state the save never writes.

type Contact360 = components["schemas"]["Contact360"];

const CONTACT: Contact360["contact"] = {
  id: "01930000-0000-7000-8000-0000000000c1",
  full_name: "Dana Buyer",
  writable: true,
  source: "manual",
  captured_by: "human:ada",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

function record(employments: Contact360["employments"]): Contact360 {
  return {
    as_of: "2026-09-01T00:00:00Z",
    sections_omitted: [],
    contact: CONTACT,
    employments,
  };
}

const NO_JOB = record({ data: [], page: { has_more: false } });

const HOLDS_A_JOB = record({
  data: [
    {
      relationship_id: "01930000-0000-7000-8000-0000000000r1",
      company_id: "01930000-0000-7000-8000-0000000000o1",
      company_name: "Brandt Automotive GmbH",
      role: "Head of Procurement",
      is_current_primary: true,
      employment_status: "current",
      version: 1,
    },
  ],
  page: { has_more: false },
});

// A colleague who may name a company for this contact — every verb the panel
// offers is refused without it, and the dialog is only ever reached through
// one of them.
function panel(view: Contact360) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: ["read", "update"],
        relationship: ["read", "create", "update", "delete"],
        company: ["read"],
      }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 420 }}>
          <Employers view={view} />
        </div>
      </StoryProviders>
    );
  };
}

// Pressing the panel's own "Add company" is what mounts the dialog open; the
// modal portals to the document body, so the click is all this asks of the
// canvas and the dialog itself is read from the capture.
const openTheDialog = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  const user = userEvent.setup();
  const canvas = within(canvasElement);
  await user.click(await canvas.findByRole("button", { name: "Add company" }));
};

const meta: Meta = {
  title: "Records/Contact 360/Add employment",
};
export default meta;

type Story = StoryObj;

// Nobody employs this contact yet, so the box states what the save will do
// anyway: the only current employment a contact holds is their primary one.
export const FirstEmployer: Story = {
  render: panel(NO_JOB),
  play: openTheDialog,
};

// A contact who already works somewhere. The box starts unticked, because
// ticking it would move the primary marker off the job they hold — a decision
// the reader has to make rather than one the default makes for them.
export const AlongsideACurrentJob: Story = {
  render: panel(HOLDS_A_JOB),
  play: openTheDialog,
};

// The same dialog in the dark theme: the overlay, the modal ground and the
// fields behind it are three elevations a darker palette compresses.
export const FirstEmployerDark: Story = {
  globals: { theme: "dark" },
  render: panel(NO_JOB),
  play: openTheDialog,
};

// Opened from a signature's company: the picker starts on that name.
export const FromSignatureCompany: Story = {
  render: panel({
    ...NO_JOB,
    profile_fields: [
      {
        field: "company_name",
        value: "Nordlicht Logistik GmbH",
        evidence_snippet: "Dana Buyer\nHead of Fleet\nNordlicht Logistik GmbH",
        source: "capture_enrich",
        captured_by: "agent:enrich",
        captured_at: "2026-08-28T09:12:00Z",
        claim_key: "profile_field:company_name",
      },
    ],
  }),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await user.click(await canvas.findByRole("button", { name: "Link" }));
  },
};
