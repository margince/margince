// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { TagsPanel } from "./tagspanel";

// Taking a tag off a record runs at once, with the toast's Undo as the way
// back. The panel's read states are in companyrailtags.stories.tsx.

const meta: Meta = {
  title: "Records/Record 360/Tag removal",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const COMPANY = "01a06151-0000-7000-8000-000000000001";

const KEY_ACCOUNT = {
  tag_id: "t-1",
  name: "Key Account",
  color: "amber",
  archived: false,
  assigned_at: "2026-03-03T10:00:00Z",
  assigned_by: { display_name: "Lena Fischer", kind: "human" },
};

const CHURN_RISK = {
  tag_id: "t-2",
  name: "Churn Risk",
  color: "rose",
  archived: false,
  assigned_at: "2026-02-01T10:00:00Z",
  assigned_by: { display_name: "Lars Becker", kind: "human" },
};

const REMOVE = `DELETE /tags/${KEY_ACCOUNT.tag_id}/apply`;
const RESTORE = `POST /tags/${KEY_ACCOUNT.tag_id}/apply/restore`;

function Served({ restore }: Readonly<{ restore?: () => Response }>) {
  let carried = [KEY_ACCOUNT, CHURN_RISK];
  installFetchStub({
    [`GET /records/company/${COMPANY}/tags`]: () =>
      jsonResponse({ data: carried, withheld: false }),
    [REMOVE]: () => {
      carried = [CHURN_RISK];
      return jsonResponse({ audit_id: "0199a000-0000-7000-8000-0000000000a1" });
    },
    [RESTORE]: () => {
      if (restore) {
        return restore();
      }
      carried = [KEY_ACCOUNT, CHURN_RISK];
      return jsonResponse({});
    },
  });
  return (
    <StoryProviders>
      <ToastProvider>
        <TagsPanel entityType="company" entityID={COMPANY} canEdit />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

const pressRemove: Story["play"] = async ({ canvasElement }) => {
  const user = userEvent.setup();
  await user.click(
    await within(canvasElement).findByRole("button", {
      name: "Remove Key Account",
    }),
  );
};

async function undoInToast() {
  const undo = await within(document.body).findByRole("button", {
    name: "Undo",
  });
  await waitFor(() => expect(undo).toBeVisible());
  return undo;
}

/** The cross takes the tag off at once: no dialog, and the toast carries Undo. */
export const RemovedWithUndo: Story = {
  render: () => <Served />,
  play: async (context) => {
    await pressRemove(context);
    await undoInToast();
    await expect(
      within(document.body).queryByRole("dialog"),
    ).not.toBeInTheDocument();
  },
};

/** An Undo the server refuses stays on screen as a danger toast. */
export const UndoRefused: Story = {
  render: () => (
    <Served
      restore={() =>
        jsonResponse(
          { detail: "Key Account was applied to this record again since." },
          409,
        )
      }
    />
  ),
  play: async (context) => {
    await pressRemove(context);
    const user = userEvent.setup();
    await user.click(await undoInToast());
    const close = await within(document.body).findByRole("button", {
      name: "Close",
    });
    await waitFor(() => expect(close).toBeVisible());
    await waitFor(() =>
      expect(
        within(document.body).getByText(
          "Key Account was applied to this record again since.",
        ),
      ).toBeVisible(),
    );
  },
};

/** Who applied the tag and when, on the tag itself, raised by focus. */
export const Provenance: Story = {
  render: () => <Served />,
  play: async ({ canvasElement }) => {
    const link = await within(canvasElement).findByRole("link", {
      name: /Key Account/,
    });
    link.focus();
    await expect(
      await within(document.body).findByRole("tooltip"),
    ).toHaveTextContent(/Added by Lena Fischer/);
  },
};

/** On a phone the cross stays drawn, since there is no hover to grow it. */
export const Phone: Story = {
  ...RemovedWithUndo,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
