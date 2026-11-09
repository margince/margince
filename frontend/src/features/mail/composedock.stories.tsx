// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useContext, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../../design-system/atoms";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../../testing/story-utils";
import { ComposeDockProvider } from "./composedock";
import { ComposeDockContext } from "./composedockcontext";

function DockedMail() {
  const dock = useContext(ComposeDockContext);
  const [page, setPage] = useState("Contacts");
  return (
    <>
      <p>{page}</p>
      <Button
        onClick={() =>
          dock?.show({
            id: "mail-1",
            props: {
              open: true,
              entityType: "contact",
              entityId: "p-1",
              contactId: "p-1",
              onClose: () => {},
            },
          })
        }
      >
        Compose a message
      </Button>
      <Button onClick={() => setPage("Deals")}>Go to deals</Button>
    </>
  );
}
const meta: Meta = { title: "Shell/Composer dock" };
export default meta;
type Story = StoryObj;
export const SurvivesNavigation: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <ComposeDockProvider>
          <DockedMail />
        </ComposeDockProvider>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Compose a message" }),
    );
    await visibleDialog();
    await userEvent.click(canvas.getByRole("button", { name: "Go to deals" }));
    await expect(canvas.getByText("Deals")).toBeVisible();
    await expect(within(document.body).getByRole("dialog")).toBeVisible();
  },
};
export const SurvivesNavigationDark: Story = {
  ...SurvivesNavigation,
  globals: { theme: "dark" },
};
