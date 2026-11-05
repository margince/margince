// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { PORTRAIT } from "../../testing/seatphotos";
import { accountStory } from "./account-settings.stories";
import { ProfilePictureRow } from "./profilepicture";

// The profile card with the seat's picture in each of its states.
const meta: Meta<typeof ProfilePictureRow> = {
  title: "Settings/You/Account/Profile picture",
  component: ProfilePictureRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProfilePictureRow>;

// The profile picture row writes on pick, so these frames drive the real input.
function pickPicture(file: File) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const page = within(canvasElement);
    await userEvent.upload(
      await page.findByLabelText("Profile picture"),
      file,
      { applyAccept: false },
    );
  };
}

const PICTURE = new File(["png"], "me.png", { type: "image/png" });

function problem(status: number, code: string) {
  return () =>
    new Response(JSON.stringify({ status, code }), {
      status,
      headers: { "content-type": "application/problem+json" },
    });
}

export const PictureUploaded: Story = {
  render: accountStory(["rep"], {}, { avatar_url: PORTRAIT }),
};

export const PictureUploadedPhone: Story = {
  ...PictureUploaded,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const PictureUploadedDark: Story = {
  ...PictureUploaded,
  globals: { theme: "dark" },
};

// The upload is still out: the tile takes nothing and its verb is pending.
export const PictureUploading: Story = {
  render: accountStory(["rep"], {
    "POST /me/avatar": () => new Promise<Response>(() => undefined),
  }),
  play: async (context) => {
    await pickPicture(PICTURE)(context);
    await expect(
      await within(context.canvasElement).findByRole("button", {
        name: "Upload picture",
      }),
    ).toHaveAttribute("aria-busy", "true");
  },
};

// Refused before it is sent: the size is checked in the browser first.
export const PictureTooLarge: Story = {
  render: accountStory(["rep"]),
  play: async (context) => {
    await pickPicture(
      new File([new Uint8Array(5 * 1024 * 1024 + 1)], "big.png", {
        type: "image/png",
      }),
    )(context);
    await within(context.canvasElement).findByText(
      "Picture not uploaded. Use a file up to 5 MB.",
    );
  },
};

export const PictureWrongType: Story = {
  render: accountStory(["rep"]),
  play: async (context) => {
    await pickPicture(new File(["gif"], "me.gif", { type: "image/gif" }))(
      context,
    );
    await within(context.canvasElement).findByText(
      "Picture not uploaded. Use a PNG, JPEG or WebP file.",
    );
  },
};

export const PictureWrongTypePhone: Story = {
  ...PictureWrongType,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// An installation with no object store answers 501, and says so in words.
export const PictureNoStorage: Story = {
  render: accountStory(["rep"], {
    "POST /me/avatar": problem(501, "not_implemented"),
  }),
  play: async (context) => {
    await pickPicture(PICTURE)(context);
    await within(context.canvasElement).findByText(
      "This installation has no file storage for pictures. Contact your system administrator.",
    );
  },
};

export const PictureLongName: Story = {
  render: accountStory(
    ["rep"],
    {},
    {
      display_name:
        "Maximiliane Alexandra von Hohenstein-Wittelsbach-Schwarzenberg",
      avatar_url: PORTRAIT,
    },
  ),
};

export const PictureLongNamePhone: Story = {
  ...PictureLongName,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
