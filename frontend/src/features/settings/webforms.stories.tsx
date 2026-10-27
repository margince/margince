// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WebForm } from "./webformkinds";
import { WebFormsCard } from "./webforms";
import { WebFormSetupModal } from "./webformsetup";

const OPERATOR: GrantSpec = {
  integrations: ["read", "create", "update", "delete"],
};

const contactForm: WebForm = {
  id: "wf-contact",
  name: "Contact page",
  form_key: "Q2hhbmdlTWVJbkFTdG9yeUtleTEy",
  sender: "browser",
  allowed_origins: ["https://www.acme.test"],
  lead_origin: "website",
  owner_id: null,
  field_map: {},
  consent_required: true,
  consent_policy_version: "2026-05",
  consent_wording: "I agree that Acme may contact me about my enquiry.",
  redirect_url: null,
  success_message: "Thanks, we will be in touch.",
  state: "active",
  secret_configured: false,
  version: 1,
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  archived_at: null,
};

const serverForm: WebForm = {
  ...contactForm,
  id: "wf-server",
  name: "Demo request",
  form_key: "U2VydmVyRm9ybUtleUZvclN0b3J5",
  sender: "server",
  allowed_origins: [],
  consent_required: false,
  consent_wording: null,
  secret_configured: true,
};

const typeformForm: WebForm = {
  ...serverForm,
  id: "wf-typeform",
  name: "Event sign-up",
  form_key: "VHlwZWZvcm1Gb3JtS2V5U3Rvcnkx",
  sender: "typeform",
  state: "paused",
};

function cardStory(forms: WebForm[], allow: GrantSpec = OPERATOR) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /web-forms": () =>
        jsonResponse({ data: forms, secrets_available: true }),
    });
    return (
      <StoryProviders>
        <WebFormsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof WebFormsCard> = {
  title: "Settings/Data/Integrations/Website forms",
  component: WebFormsCard,
};
export default meta;
type Story = StoryObj<typeof WebFormsCard>;

export const Forms: Story = {
  render: cardStory([contactForm, serverForm, typeformForm]),
};

export const FormsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: cardStory([contactForm, serverForm]),
};

export const Empty: Story = {
  render: cardStory([]),
};

export const ReadOnly: Story = {
  render: cardStory([contactForm], {}),
};

export const CreateDrawer: Story = {
  render: cardStory([contactForm]),
  play: async () => {
    await userEvent.click(await screen.findByTestId("new-web-form"));
  },
};

export const BrowserSetup: Story = {
  render: () => (
    <StoryProviders>
      <WebFormSetupModal
        form={contactForm}
        origin="https://crm.acme.test"
        onClose={() => {}}
      />
    </StoryProviders>
  ),
};

export const ServerSetupWithSecret: Story = {
  render: () => (
    <StoryProviders>
      <WebFormSetupModal
        form={serverForm}
        secret="mwf_c3Rvcnktb25seS1ub3QtYS1yZWFsLXNlY3JldA"
        origin="https://crm.acme.test"
        onClose={() => {}}
      />
    </StoryProviders>
  ),
};
