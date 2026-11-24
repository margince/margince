// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import { DealCard } from "../../design-system/dealcard";
import type { BoardDeal } from "../../design-system/pipelineboard";
import { useOwnToast } from "../../design-system/toast";
import { browserZone } from "../../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { DealCardMenu } from "./dealcardmenu";
import type { Deal, Stage } from "./deals";

// One card's "…" menu on an open, a won and an archived deal, opened on the
// card, and the close-date dialog and owner list it opens.

const stages: Stage[] = (
  [
    ["discovery", "Discovery", "open"],
    ["proposal", "Proposal", "open"],
    ["won", "Won", "won"],
    ["lost", "Lost", "lost"],
  ] as const
).map(([id, name, semantic], position) => ({
  id,
  pipeline_id: "sales",
  name,
  position,
  semantic,
  win_probability: 40,
}));

function wire(extra: Partial<Deal>): Deal {
  return {
    id: "d-1",
    name: "ERP connector, phase 2",
    pipeline_id: "sales",
    stage_id: "discovery",
    status: "open",
    source: "manual",
    captured_by: "human:u-1",
    writable: true,
    archivable: true,
    version: 1,
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    ...extra,
  };
}

function MenuOnCard({
  deal,
  routes = {},
}: Readonly<{ deal: Deal; routes?: RouteMap }>) {
  installFetchStub({
    "GET /me": meRoute({
      activity: ["read", "create"],
      deal: ["read", "update", "delete"],
    }),
    "GET /users": () => roster(COLLEAGUES),
    ...routes,
  });
  return (
    <StoryProviders>
      <CardWithMenu deal={deal} />
    </StoryProviders>
  );
}

function CardWithMenu({ deal }: Readonly<{ deal: Deal }>) {
  const copyRefusal = useOwnToast();
  const archived = Boolean(deal.archived_at);
  const card: BoardDeal = {
    id: deal.id,
    name: deal.name,
    company: "Halbach & Söhne KG",
    companyId: "o-2",
    valueMinor: 4_800_000,
    currency: "EUR",
    idleSince: "2026-09-13T12:00:00Z",
    measuredAt: Date.parse("2026-09-15T12:00:00Z"),
    probabilityPct: 75,
    archived,
    closeDate: deal.expected_close_date ?? null,
    closeDateProvisional: deal.close_date_provisional ?? false,
  };
  const verbs = {
    onSummary: () => undefined,
    onEmail: archived ? undefined : () => undefined,
    onAddTask: archived ? undefined : () => undefined,
  };
  return (
    <div style={{ width: 300, maxWidth: "100%" }}>
      <DealCard
        deal={card}
        href="#/deals/d-1"
        zone={browserZone()}
        actions={{
          ...verbs,
          menu: (
            <DealCardMenu
              deal={deal}
              stages={stages}
              verbs={verbs}
              onAdvance={() => undefined}
              copyRefusal={copyRefusal}
            />
          ),
        }}
      />
    </div>
  );
}

const meta: Meta = { title: "Records/Deals/Deal card menu" };
export default meta;
type Story = StoryObj;

function openMenuShowing(first: string) {
  return async ({
    canvasElement,
  }: Readonly<{ canvasElement: HTMLElement }>) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "More actions for ERP connector, phase 2",
      }),
    );
    await expect(
      await screen.findByRole("button", { name: first }),
    ).toBeVisible();
  };
}

const openDeal = {
  render: () => <MenuOnCard deal={wire({})} />,
  play: openMenuShowing("Move to Proposal"),
};
export const OpenDeal: Story = openDeal;
export const OpenDealDark: Story = { ...openDeal, globals: { theme: "dark" } };
export const OpenDealPhone: Story = {
  ...openDeal,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A won deal keeps its moves, refused, with the line that says where it reopens.
export const WonDeal: Story = {
  render: () => <MenuOnCard deal={wire({ status: "won", stage_id: "won" })} />,
  play: async (context) => {
    await openMenuShowing("Mark lost")(context);
    await expect(
      screen.getByRole("button", { name: "Mark lost" }),
    ).toBeDisabled();
    await expect(
      screen.getByText(
        "This deal is closed. Reopen it on its page to move it.",
      ),
    ).toBeVisible();
  },
};

export const ArchivedDeal: Story = {
  render: () => (
    <MenuOnCard deal={wire({ archived_at: "2026-09-10T09:00:00Z" })} />
  ),
  play: async (context) => {
    await openMenuShowing("Copy link")(context);
    await expect(
      screen.queryByRole("button", { name: "Move to stage" }),
    ).toBeNull();
    await expect(
      screen.queryByRole("button", { name: "Archive deal" }),
    ).toBeNull();
  },
};

const COLLEAGUES = [
  { id: "u-2", display_name: "Dana Fischer", email: "dana@example.test" },
  { id: "u-3", display_name: "Mai Nguyen", email: "mai@example.test" },
  { id: "u-4", display_name: "Omar Haddad", email: "omar@example.test" },
];

function roster(colleagues: readonly unknown[]): Response {
  return jsonResponse({ data: colleagues, page: { next_cursor: null } });
}

function ownedBy(routes?: RouteMap) {
  return () => <MenuOnCard deal={wire({ owner_id: "u-2" })} routes={routes} />;
}

async function openOwnerList(context: { canvasElement: HTMLElement }) {
  await openMenuShowing("Change owner")(context);
  await userEvent.click(screen.getByRole("button", { name: "Change owner" }));
  return within(await screen.findByRole("dialog"));
}

/** The owner list over the menu, the deal's owner marked. */
const ownerList = {
  render: ownedBy(),
  play: async (context: { canvasElement: HTMLElement }) => {
    const list = await openOwnerList(context);
    await expect(
      await list.findByRole("option", { name: "Dana Fischer" }),
    ).toHaveAttribute("aria-selected", "true");
  },
};
export const OwnerList: Story = ownerList;
export const OwnerListDark: Story = {
  ...ownerList,
  globals: { theme: "dark" },
};
export const OwnerListPhone: Story = {
  ...ownerList,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A colleague picked and the PATCH still out. */
export const OwnerPending: Story = {
  render: ownedBy({ "PATCH /deals/d-1": () => new Promise(() => undefined) }),
  play: async (context) => {
    const list = await openOwnerList(context);
    await userEvent.click(
      await list.findByRole("option", { name: "Mai Nguyen" }),
    );
    await expect(list.getByRole("listbox")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  },
};

/** The server refused the change: the list stays open over its reason. */
const ownerRefused = {
  render: ownedBy({
    "PATCH /deals/d-1": () =>
      jsonResponse(
        {
          title: "Forbidden",
          status: 403,
          detail: "Mai Nguyen may not own deals in this pipeline.",
        },
        403,
      ),
  }),
  play: async (context: { canvasElement: HTMLElement }) => {
    const list = await openOwnerList(context);
    await userEvent.click(
      await list.findByRole("option", { name: "Mai Nguyen" }),
    );
    await list.findByRole("alert");
  },
};
export const OwnerRefused: Story = ownerRefused;
export const OwnerRefusedDark: Story = {
  ...ownerRefused,
  globals: { theme: "dark" },
};
export const OwnerRefusedPhone: Story = {
  ...ownerRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The colleague list could not be read: its reason, not an empty list. */
export const OwnerRosterFailed: Story = {
  render: ownedBy({
    "GET /users": () =>
      jsonResponse(
        {
          title: "Forbidden",
          status: 403,
          detail: "You may not list the workspace’s members.",
        },
        403,
      ),
  }),
  play: async (context) => {
    const list = await openOwnerList(context);
    await list.findByText("You may not list the workspace’s members.");
  },
};

/** Nobody the deal may be handed to. */
export const OwnerEmptyRoster: Story = {
  render: ownedBy({ "GET /users": () => roster([]) }),
  play: async (context) => {
    const list = await openOwnerList(context);
    await list.findByText("No match");
  },
};

/** Colleagues whose names wrap inside the list. */
const ownerLongNames = {
  render: ownedBy({
    "GET /users": () =>
      roster([
        {
          id: "u-5",
          display_name:
            "Maximiliane Josefine von Hohenzollern-Sigmaringen-Wittelsbach",
          email: "maximiliane@example.test",
        },
        {
          id: "u-6",
          display_name: "Nguyễn Thị Phương Thảo Hoàng Bảo Ngọc Trâm Anh",
          email: "thao@example.test",
        },
      ]),
  }),
  play: async (context: { canvasElement: HTMLElement }) => {
    const list = await openOwnerList(context);
    await list.findByRole("option", { name: /^Maximiliane/ });
  },
};
export const OwnerLongNames: Story = ownerLongNames;
export const OwnerLongNamesPhone: Story = {
  ...ownerLongNames,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

function closingOn(extra: Partial<Deal>, routes?: RouteMap) {
  return () => (
    <MenuOnCard
      deal={wire({ expected_close_date: "2027-03-14", ...extra })}
      routes={routes}
    />
  );
}

async function openCloseDate(context: { canvasElement: HTMLElement }) {
  await openMenuShowing("Change close date")(context);
  await userEvent.click(
    screen.getByRole("button", { name: "Change close date" }),
  );
  return within(await screen.findByRole("dialog", { name: "Expected close" }));
}

async function typeCloseDate(
  dialog: ReturnType<typeof within>,
  day: string,
): Promise<void> {
  const box = dialog.getByRole("textbox", { name: "Expected close" });
  await userEvent.clear(box);
  await userEvent.type(box, day);
  await userEvent.click(dialog.getByRole("button", { name: "Save" }));
}

/** The one-field dialog, opened on the deal's close date. */
const closeDate = {
  render: closingOn({}),
  play: async (context: { canvasElement: HTMLElement }) => {
    const dialog = await openCloseDate(context);
    await expect(
      dialog.getByRole("textbox", { name: "Expected close" }),
    ).not.toHaveValue("");
  },
};
export const CloseDate: Story = closeDate;
export const CloseDateDark: Story = {
  ...closeDate,
  globals: { theme: "dark" },
};
export const CloseDatePhone: Story = {
  ...closeDate,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A new day typed and the PATCH still out. */
export const CloseDateSaving: Story = {
  render: closingOn(
    {},
    { "PATCH /deals/d-1": () => new Promise(() => undefined) },
  ),
  play: async (context) => {
    const dialog = await openCloseDate(context);
    await typeCloseDate(dialog, "2027-04-30");
    await expect(dialog.getByRole("button", { name: "Save" })).toHaveAttribute(
      "aria-busy",
      "true",
    );
  },
};

/** The server refused the day: the dialog stays open over its reason. */
const closeDateRefused = {
  render: closingOn(
    {},
    {
      "PATCH /deals/d-1": () =>
        jsonResponse(
          {
            title: "Unprocessable",
            status: 422,
            detail: "An open deal cannot close in the past.",
          },
          422,
        ),
    },
  ),
  play: async (context: { canvasElement: HTMLElement }) => {
    const dialog = await openCloseDate(context);
    await typeCloseDate(dialog, "2020-01-02");
    await dialog.findByRole("alert");
  },
};
export const CloseDateRefused: Story = closeDateRefused;
export const CloseDateRefusedDark: Story = {
  ...closeDateRefused,
  globals: { theme: "dark" },
};
export const CloseDateRefusedPhone: Story = {
  ...closeDateRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A day the server cannot store, refused on the field before anything is sent. */
export const CloseDateOutOfRange: Story = {
  render: closingOn({}),
  play: async (context) => {
    const dialog = await openCloseDate(context);
    await typeCloseDate(dialog, "31/12/9999");
    await dialog.findByText(
      "Expected close must be from 02/01/0001 to 30/12/9999.",
    );
  },
};

/** A won deal is offered its owner change but no close date. */
const closeDateClosedDeal = {
  render: closingOn({ status: "won", stage_id: "won" }),
  play: async (context: { canvasElement: HTMLElement }) => {
    await openMenuShowing("Change owner")(context);
    await expect(
      screen.queryByRole("button", { name: "Change close date" }),
    ).toBeNull();
  },
};
export const CloseDateClosedDeal: Story = closeDateClosedDeal;
export const CloseDateClosedDealDark: Story = {
  ...closeDateClosedDeal,
  globals: { theme: "dark" },
};

/** A passed day nobody confirmed, in the card's warning tone, beside its fix. */
const closeDateUnconfirmed = {
  render: closingOn({
    expected_close_date: "2026-09-01",
    close_date_provisional: true,
  }),
  play: openMenuShowing("Change close date"),
};
export const CloseDateUnconfirmed: Story = closeDateUnconfirmed;
export const CloseDateUnconfirmedDark: Story = {
  ...closeDateUnconfirmed,
  globals: { theme: "dark" },
};
