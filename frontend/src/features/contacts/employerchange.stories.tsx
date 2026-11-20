// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useContactAboutSections } from "./contactdetails";
import "./contact360.css";

// The employer changes from its row in the About section's dialog, which steps
// aside for the employer's own dialog.

type Contact360 = components["schemas"]["Contact360"];

const employed: Contact360 = {
  as_of: "2026-09-01T00:00:00Z",
  sections_omitted: [],
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    title: "Head of Fleet",
    writable: true,
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-06-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  },
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
    ],
    page: { has_more: false },
  },
};

function About() {
  const about = useContactAboutSections(employed);
  return (
    <>
      <RecordDetails sections={about.sections} />
      {about.element}
    </>
  );
}

const meta: Meta = {
  title: "Records/Contact 360/Employer change",
  parameters: { layout: "padded" },
  render: () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: ["read", "update"],
        relationship: ["read", "create", "update"],
        company: ["read"],
      }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 320 }}>
          <About />
        </div>
      </StoryProviders>
    );
  },
};
export default meta;

type Story = StoryObj;

export const FromAboutDialog: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", { name: "Edit About" }),
    );
    const about = await screen.findByRole("dialog", { name: "About" });
    await user.click(
      within(about).getByRole("button", { name: "Change Employer" }),
    );
    const change = await screen.findByRole("dialog", {
      name: en["contact.employer.changeTitle"],
    });
    await waitFor(() => expect(change).toBeVisible());
    await expect(screen.queryByRole("dialog", { name: "About" })).toBeNull();
  },
};

export const FromAboutDialogDark: Story = {
  ...FromAboutDialog,
  globals: { theme: "dark" },
};

export const FromAboutDialogPhone: Story = {
  ...FromAboutDialog,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
