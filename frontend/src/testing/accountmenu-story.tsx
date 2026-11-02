// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { AccountMenu } from "../app/account";
import { meFixture } from "./mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
// This frame builds a `.topbar` by hand and imports nothing that loads the
// strip's sheet, so without it every frame draws an unstyled strip.
import "../app/topbar.css";

// The account menu's story frame, shared by the stories beside account.tsx and
// the modules it is split into, so each renders the menu where the product does.

type SessionUser = components["schemas"]["MeResponse"]["user"];

/** `GET /me` is routed explicitly: an unrouted probe reads as a malformed
 *  session and the block draws its unresolved branch. */
export function stubSession(
  user: Partial<SessionUser> = {},
  routes: RouteMap = {},
) {
  const me = meFixture();
  installFetchStub({
    "GET /me": () => jsonResponse({ ...me, user: { ...me.user, ...user } }),
    ...routes,
  });
}

/** The block in the real `.app` frame and top bar: `--topbarH` and
 *  `--pageGutter` are declared on `.app`, and the menu's drop offset reads
 *  `--topbarH`, so a bare bar draws the wrong height, gutter and offset. */
export function TopBarTrail({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <div className="app railexpanded">
      <div className="rail expanded" />
      <main className="main">
        <header className="topbar">
          <div className="topbar-lead" />
          <div
            className="topbar-searchslot"
            style={{ minInlineSize: "var(--controlHeight)" }}
          />
          <div className="topbar-trail">{children}</div>
        </header>
        <div className="scroll" />
      </main>
    </div>
  );
}

/** Open the menu, the way the reader does. The panel's state is component-local
 *  (there is no `startOpen` prop), so the open frames drive the real trigger. */
export async function openMenu({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) {
  const canvas = within(canvasElement);
  await userEvent.click(canvas.getByRole("button", { name: /Account$/ }));
}

export async function openThemeFlyout(context: { canvasElement: HTMLElement }) {
  await openMenu(context);
  const canvas = within(context.canvasElement);
  await userEvent.click(canvas.getByRole("menuitem", { name: "Theme" }));
}

/** Open the menu and then Help and feedback. */
export async function openHelpFlyout(context: { canvasElement: HTMLElement }) {
  await openMenu(context);
  const canvas = within(context.canvasElement);
  await userEvent.click(
    canvas.getByRole("menuitem", { name: "Help and feedback" }),
  );
}

/** The account block in the top bar's trail, over the session `stubSession` set. */
export function AccountMenuFrame() {
  return (
    <StoryProviders>
      <TopBarTrail>
        <AccountMenu />
      </TopBarTrail>
    </StoryProviders>
  );
}
