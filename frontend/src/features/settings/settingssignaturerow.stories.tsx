// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SignatureSettingRow } from "./settingssignaturerow";

function SignatureRow() {
  const toast = useToast();
  return (
    <Panel>
      <SettingList bleed="settings">
        <SignatureSettingRow toast={toast} />
      </SettingList>
    </Panel>
  );
}
function render() {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /me/email-signature": () =>
      jsonResponse({
        body: "Mira Voss",
        title: "Head of Sales",
        phone: "+49 711 555 0100",
        template_active: true,
      }),
    "POST /emails:sign-off": () =>
      jsonResponse({
        text: "Mira Voss",
        html: "<p><b>Mira Voss</b><br>Head of Sales<br>+49 711 555 0100</p>",
        kind: "template",
      }),
  });
  return (
    <StoryProviders>
      <SignatureRow />
    </StoryProviders>
  );
}
const meta = {
  title: "Settings/You/Account/Signature row",
  component: SignatureRow,
  parameters: { layout: "padded" },
} satisfies Meta<typeof SignatureRow>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Saved: Story = { render };
export const TemplateEditor: Story = {
  render,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Edit signature",
      }),
    );
  },
};
