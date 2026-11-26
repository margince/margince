// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen, userEvent } from "storybook/test";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import {
  INQUIRY_AUTHORIZER,
  INQUIRY_AUTOMATION,
  INQUIRY_COORDINATOR,
  INQUIRY_DEADLINE_PREVIEW,
  INQUIRY_FORM,
  INQUIRY_GRANTS,
  INQUIRY_MEMBERS,
  INQUIRY_POLICY,
  INQUIRY_REQUEST_FIELD,
} from "./websiteinquiry.fixtures";
import { WebsiteInquiryDialog } from "./websiteinquirydialog";
import { WebsiteInquiryFields } from "./websiteinquiryfields";

type Automation = components["schemas"]["Automation"];
function setup(policy: Automation | null, failure?: number) {
  return () => {
    const form = {
      ...INQUIRY_FORM,
      handling_automation_id: policy?.id ?? null,
    };
    installFetchStub({
      "GET /me": meRoute(INQUIRY_GRANTS),
      [`POST /web-forms/${INQUIRY_FORM.id}/handling/preview`]: () =>
        jsonResponse(INQUIRY_DEADLINE_PREVIEW),
      "GET /custom-fields": () =>
        jsonResponse({ data: [INQUIRY_REQUEST_FIELD] }),
      "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
      "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
      [`GET /automations/${INQUIRY_AUTOMATION.id}`]: () =>
        failure
          ? jsonResponse(
              {
                code: failure === 403 ? "permission_denied" : "internal",
                detail:
                  failure === 403
                    ? "You cannot read this handling policy."
                    : "The handling policy could not be loaded.",
                status: failure,
              },
              failure,
            )
          : jsonResponse(policy),
    });
    return (
      <StoryProviders>
        <WebsiteInquiryDialog form={form} onClose={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof WebsiteInquiryDialog> = {
  title: "Settings/Data/Website forms/Inquiry handling",
  component: WebsiteInquiryDialog,
};
export default meta;
type Story = StoryObj<typeof WebsiteInquiryDialog>;

export const NeedsSetup: Story = { render: setup(null) };
export const Configured: Story = {
  render: setup({ ...INQUIRY_AUTOMATION, status: "enabled" }),
};
export const Paused: Story = { render: setup(INQUIRY_AUTOMATION) };
export const Rotation: Story = {
  render: setup({
    ...INQUIRY_AUTOMATION,
    inquiry_policy: {
      ...INQUIRY_POLICY,
      mode: "rotate",
      routing: { owners: [INQUIRY_AUTHORIZER, INQUIRY_COORDINATOR] },
    },
  }),
};
export const Restricted: Story = { render: setup(INQUIRY_AUTOMATION, 403) };
export const Failed: Story = { render: setup(INQUIRY_AUTOMATION, 500) };
export const Review: Story = {
  render: setup(INQUIRY_AUTOMATION),
  play: async () => {
    await screen.findByRole("textbox", { name: "Task title" });
    await userEvent.click(
      screen.getByRole("button", { name: "Review handling" }),
    );
    await screen.findByText(/^A question received at /);
  },
};

function FieldsDemo() {
  const [policy, setPolicy] = useState(INQUIRY_POLICY);
  return (
    <div className="form-stack">
      <WebsiteInquiryFields
        form={INQUIRY_FORM}
        value={policy}
        onChange={setPolicy}
      />
    </div>
  );
}

export const Fields: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute(INQUIRY_GRANTS),
      [`POST /web-forms/${INQUIRY_FORM.id}/handling/preview`]: () =>
        jsonResponse(INQUIRY_DEADLINE_PREVIEW),
      "GET /custom-fields": () =>
        jsonResponse({ data: [INQUIRY_REQUEST_FIELD] }),
      "GET /users": () => jsonResponse({ data: INQUIRY_MEMBERS }),
      "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
    });
    return (
      <StoryProviders>
        <FieldsDemo />
      </StoryProviders>
    );
  },
};
