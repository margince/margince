// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ExtensionAccessCard } from "./extension-access";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The role × CRUD matrix: what each composed unit brought into the installation
// and which roles may reach it. Until somebody grants one, an enabled unit
// renders "you do not hold access" for every seat — which is why this surface
// exists and why its withheld state is worth looking at.
//
// Each registered object is one stacked `SettingRow` — a toggle matrix is the
// subject of its row, never an answer that fits beside the question — and what
// the unit BROUGHT (its objects, routes and jobs) reads last, behind a closed
// disclosure: it is reference an operator opens to check which object gates the
// route they care about, not a decision.
const YOGI = {
  name: "yogi",
  version: "0.4.1",
  rbac_objects: ["ext_yogi_briefing"],
  routes: [{ path: "/ext/yogi/brief", method: "GET" }],
  jobs: ["yogi_nightly_brief"],
};

const DE = {
  name: "de",
  version: "1.2.0",
  rbac_objects: [],
  routes: [],
  jobs: [],
};

const ROLES = [
  { key: "admin", name: "Admin", is_system: true, version: 3 },
  { key: "rep", name: "Rep", is_system: true, version: 3 },
];

const NONE = { create: false, read: false, update: false, delete: false };
const READ = { ...NONE, read: true };

function story(
  extensions: Record<string, unknown>[],
  roles: string[],
  objects: Record<string, unknown> = {},
  seat: "full" | "read" = "full",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(
        {
          extension_access: ["read"],
          role_admin: roles.includes("admin") ? ["read", "update"] : ["read"],
        },
        { roles, seat },
      ),
      "GET /extensions": () => jsonResponse({ extensions }),
      // `roles`, which is what RoleDirectory names — not the `data` envelope the
      // paginated collections use. Keyed wrong, the read narrowed to an empty
      // list and every story in this file drew a matrix with no ROLE ROWS, over
      // the "nobody holds read" warning that an empty list makes vacuously true:
      // UnitsWithGrants and NothingGrantedYet were the same picture, and neither
      // was the matrix.
      "GET /roles": () =>
        jsonResponse({
          roles: ROLES.map((role) => ({ ...role, objects })),
        }),
    });
    return (
      <StoryProviders>
        <ExtensionAccessCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ExtensionAccessCard> = {
  title: "Settings/Governance/Extensions/Extensions and access",
  component: ExtensionAccessCard,
};
export default meta;
type Story = StoryObj<typeof ExtensionAccessCard>;

export const UnitsWithGrants: Story = {
  render: story([YOGI, DE], ["admin"], { ext_yogi_briefing: READ }),
};

// The state a fresh installation is actually in: the unit is enabled, its object
// is registered, and no role has been pointed at it yet.
export const NothingGrantedYet: Story = {
  render: story([YOGI, DE], ["admin"], { ext_yogi_briefing: NONE }),
};

export const NoUnitsComposed: Story = { render: story([], ["admin"]) };

// A rep reads the inventory and cannot change who reaches it. The matrix stays
// on screen — an absent one would say this installation composes nothing.
export const NotAnAdmin: Story = {
  render: story([YOGI], ["rep"], { ext_yogi_briefing: READ }),
};

// An admin on a read seat: every tick is legible and none of them is pressable,
// with the seat ceiling said once above the rows and attached to each switch as
// its own `reason`. Worth a story of its own because it is the state that is
// easiest to draw as an absent card, and an absent one would read as "this
// installation composes nothing".
export const ReadSeat: Story = {
  render: story([YOGI], ["admin"], { ext_yogi_briefing: READ }, "read"),
};

// The inventory and the matrix in dark. Three things here are drawn from tokens
// that mean "one step off the card ground", and dark is where a step that small
// either survives or collapses: a soft `Badge` fills an RBAC object and a route
// with --bgChip inside a card, the matrix separates every role row with a single
// --borderSubtle hairline, and the `SettingList` now rules between one object's
// grid and the next with the same hairline — two rules of the same weight, one
// inside a grid and one between two of them, which either read as a hierarchy or
// as a wall. The Switch tracks in the cells are the fourth — an off track and an
// on track have to stay two different things when the whole palette darkens
// under them.
export const UnitsWithGrantsDark: Story = {
  globals: { theme: "dark" },
  render: story([YOGI, DE], ["admin"], { ext_yogi_briefing: READ }),
};

// At 390px a role column plus four CRUD columns is wider than the phone. The
// matrix scrolls inside its `TableScroll` while the card keeps its width.
// The unit's page link renders from the SPA's generated screen registry. That
// registry is empty in every story, so no story can show the link truncate.
export const UnitsWithGrantsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story([YOGI, DE], ["admin"], { ext_yogi_briefing: READ }),
  play: async ({ canvasElement }) => {
    const matrix = await within(canvasElement).findByRole("table");
    const card = matrix.closest<HTMLElement>(".panel");
    if (!card) throw new Error("the matrix rendered outside its card");
    await expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
    // A hidden label placed against a box outside the scroller widens the page
    // once the grid is wide enough to reach the card's edge.
    const scroller = matrix.closest(".table-scroll");
    if (!scroller) throw new Error("the matrix rendered outside its scroller");
    for (const hidden of matrix.querySelectorAll<HTMLElement>(".sr-only")) {
      await expect(scroller.contains(hidden.offsetParent)).toBe(true);
    }
  },
};
