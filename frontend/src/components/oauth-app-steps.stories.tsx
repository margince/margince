import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field, TextInput } from "../design-system/atoms";
import { CopyAction } from "../design-system/clipboardcopy";
import { StoryProviders } from "../testing/story-utils";
import { OAuthAppSteps } from "./oauth-app-steps";

const meta = {
  title: "Settings/Data/Integrations/App registration steps",
  component: OAuthAppSteps,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof OAuthAppSteps>;
export default meta;
type Story = StoryObj<typeof meta>;
const fields = (
  <>
    <Field label="Client ID">
      {(control) => (
        <TextInput {...control} value="synthetic-client-id" readOnly />
      )}
    </Field>
    <Field label="Client secret">
      {(control) => (
        <TextInput
          {...control}
          type="password"
          value="synthetic-secret"
          readOnly
        />
      )}
    </Field>
  </>
);
const redirects = (
  <CopyAction
    text="https://app.example.test/v1/messenger/slack/callback"
    labels={{
      copy: "Copy callback",
      copied: "Copied",
      remedy: "Copy the displayed callback address.",
    }}
  />
);
export const Slack: Story = {
  args: { provider: "slack", redirects, children: fields },
};
export const Microsoft: Story = {
  args: { provider: "microsoft", redirects: null, children: fields },
};
export const Google: Story = {
  args: { provider: "google", redirects: null, children: fields },
};
