// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
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
      await page.findByLabelText(englishCopy["account.pictureLabel"]),
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

async function openEdit({ canvasElement }: { canvasElement: HTMLElement }) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: englishCopy["account.pictureEdit"],
    }),
  );
}

// The Edit menu over a stored picture: upload another, or remove it.
export const PictureMenuOpen: Story = {
  ...PictureUploaded,
  play: async (context) => {
    await openEdit(context);
    await within(context.canvasElement.ownerDocument.body).findByRole(
      "button",
      {
        name: englishCopy["account.pictureRemove"],
      },
    );
  },
};

export const PictureMenuOpenDark: Story = {
  ...PictureMenuOpen,
  globals: { theme: "dark" },
};

export const PictureMenuOpenPhone: Story = {
  ...PictureMenuOpen,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// No picture yet: the initials, and a menu that only uploads.
export const PictureEmptyMenuOpen: Story = {
  render: accountStory(["rep"]),
  play: async (context) => {
    await openEdit(context);
    await within(context.canvasElement.ownerDocument.body).findByRole(
      "button",
      {
        name: englishCopy["account.pictureAdd"],
      },
    );
  },
};

// The upload is still out: nothing takes a file and the Edit pill waits.
export const PictureUploading: Story = {
  render: accountStory(["rep"], {
    "POST /me/avatar": () => new Promise<Response>(() => undefined),
  }),
  play: async (context) => {
    await pickPicture(PICTURE)(context);
    await expect(
      await within(context.canvasElement).findByRole("button", {
        name: englishCopy["account.pictureEdit"],
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
      englishCopy["account.pictureTooLarge"],
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
      englishCopy["account.pictureWrongType"],
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
      englishCopy["account.pictureNoStorage"],
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
