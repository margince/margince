// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { SettingsScreen, settingsAddress } from "./settings";
import { PipelinesCard } from "./settings.pipelines";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The settings entries and the cards each one carries. Every story installs the
// fetch stub those cards read through, so the render is deterministic and
// network-free — the same fixture shapes the settings.test.tsx cases use.
//
// A company entry is only reachable when the principal holds what its cards
// ask for, and SettingsScreen shows the access boundary for anything else. So a
// story about such an entry has to name its grants: `me({...})` builds the /me
// body that opens the entry the story is capturing.

const me =
  (allow: GrantSpec = {}) =>
  () =>
    jsonResponse({
      ...meFixture({ roles: ["admin"], allow }),
      // The BARE user id, the way /me reports it. The audit fixture spells the
      // same actor the way the wire does ("human:u-mor"), so the trail reads
      // "You" for the viewer's own entry — ActorTag owns that difference.
      user: { ...meFixture().user, id: "u-mor", email: "ada@acme.test" },
    });

const passports = () =>
  jsonResponse({
    data: [
      {
        id: "pp-1",
        label: "Scout",
        scopes: ["read", "draft"],
        created_at: "2026-07-01T08:00:00Z",
        expires_at: "2026-10-01T08:00:00Z",
        revoked_at: null,
      },
    ],
    api_base_url: "https://crm.example.com/v1",
    page: { next_cursor: null, has_more: false },
  });

// IT-1 governed tool console: two tools of differing tier/egress, plus a
// read-only passport so the play() below can show the send_email row struck
// (its "send" scope isn't in the selected passport's grant). Both live on the
// personal "Your agents" entry, which no grant gates.
//
// `title` and `description` are REQUIRED on AgentTool and the row draws both, so
// a fixture without them captured a console the product cannot serve: one tool
// name per row and nothing else, which is precisely the half that made the
// layout look settled while the real rows carry three lines of prose. The
// governance clause is part of the served description — the server appends it —
// so it stays here verbatim.
const tools = () =>
  jsonResponse({
    data: [
      {
        name: "search_records",
        title: "Search records",
        description:
          'Find contacts, companies, deals, leads and projects by name. (Governance: runs immediately; requires passport scope "read".)',
        required_scope: "read",
        tier: "auto_execute",
        egress: false,
      },
      {
        name: "send_email",
        title: "Send email",
        description:
          'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
        required_scope: "send",
        tier: "confirmation_required",
        egress: true,
      },
    ],
  });

// The MCP connector's own discovery document (RFC 9728), which is what the
// connect guide builds its four commands from. Unrouted, the stub's list-shaped
// fallback answers with no `resource` and the guide renders its error state — so
// every story on the agents tab that shows the guide has to say which of the two
// worlds it is in.
const connectorOn = () =>
  jsonResponse({ resource: "https://crm.acme.test/mcp" });

const connectorOff = () =>
  jsonResponse({ title: "no MCP connector on this installation" }, 404);

// The Account tab's bookability card indexes its own answer, so a tab story
// that leaves this endpoint unrouted takes the WHOLE SCREEN down: the fallback
// empty page carries no `working_hours`, the card reads `start_time` off
// undefined and throws mid-render. Routed here rather than per story for the
// reason settings.testkit.tsx gives about its own fakes — an endpoint added to
// one of them alone leaves the others failing in exactly that way, nowhere near
// the cause. The shape is restated rather than shared with that testkit because
// the testkit is built on `vi`, which no Storybook build has.
//
// The zone comes from the identity fixture rather than being typed again here.
// A story that named one would be a second author of the reader's clock, and
// `format/timezone.ts` is the module that owns naming zones.
const WORKING_HOURS: RouteMap = {
  "GET /me/working-hours": () =>
    jsonResponse({
      chosen: false,
      working_hours: {
        start_time: "09:00",
        end_time: "17:00",
        days: [1, 2, 3, 4, 5],
        timezone: meFixture().user.timezone,
      },
    }),
};

function tab(tabId: string, routes: RouteMap) {
  return () => {
    // The story's own routes last, so a story about this card can still say
    // something different from the default.
    installFetchStub({ ...WORKING_HOURS, ...routes });
    return (
      <StoryProviders>
        <SettingsScreen route={settingsAddress(tabId)} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof SettingsScreen> = {
  title: "Settings/Settings screen",
  component: SettingsScreen,
};
export default meta;

type Story = StoryObj<typeof SettingsScreen>;

// The whole tab is ONE card now: the identity block as its subject, and the
// password, the sign-off and the language as three rows under it, each answer at
// the same x. It used to be four panels with four header bands.
export const AccountTab: Story = {
  render: tab("account", { "GET /me": me() }),
};

// And in dark, because the card is a plate (the identity block) sitting above a
// ruled list, and the hairline between two decisions plus the avatar's tint are
// three derived values that move between themes.
export const AccountTabDark: Story = {
  globals: { theme: "dark" },
  render: tab("account", { "GET /me": me() }),
};

// Language belongs to the contact, not to the sidebar. The play() opens the
// listbox so the capture carries the options rather than only the control's
// closed face.
export const AccountPreferences: Story = {
  render: tab("account", { "GET /me": me() }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("combobox", { name: "Language" }),
    );
  },
};

// The sign-off's editor. A textarea committed with a Save button is the settings
// page's modal case, not its row case, so the row states what the signature
// currently says and the verb opens the form — which is the state this captures.
export const AccountSignatureDialog: Story = {
  name: "Account — edit signature",
  render: tab("account", {
    "GET /me": me(),
    "GET /me/email-signature": () =>
      jsonResponse({ body: "Marek Janetzke\nGradion · +49 40 123456" }),
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit signature" }),
    );
  },
};

// The contact's own agent authority, and the one page the founder reads for
// whether the four cards on it space alike: the passports minted, the clients
// holding one (with the connect guide open, because nothing is connected), the
// governed tools those credentials reach, and the autonomy tiers they run under.
const agentsTabRoutes = {
  "GET /me": me(),
  "GET /passports": passports,
  "GET /agent-tools": tools,
  "GET /.well-known/oauth-protected-resource": connectorOn,
};

export const AgentsTab: Story = {
  render: tab("agents", agentsTabRoutes),
};

// Dark, because the whole page is now hairlines between rows and chips against a
// card ground — both derived values that move with the theme, and the interval
// between two cards is only legible if the rule between two rows is.
export const AgentsTabDark: Story = {
  globals: { theme: "dark" },
  render: tab("agents", agentsTabRoutes),
};

// At 390px. The code example wraps inside the card instead of widening it.
export const AgentsTabPhone: Story = {
  name: "Your agents — phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: tab("agents", agentsTabRoutes),
};

// The connector switched off: the guide has no commands to print, so its one row
// says so and says what still works. It is the state a default install is in, and
// it used to render as a bold line and a paragraph flush against the disclosure.
export const AgentsConnectorOff: Story = {
  name: "Your agents — MCP connector off",
  render: tab("agents", {
    ...agentsTabRoutes,
    "GET /.well-known/oauth-protected-resource": connectorOff,
  }),
};

// AS-2 kill-switch: PassportCard revoke is a hard DELETE behind a ConfirmModal.
// Mirrors share.stories' revoke play() — render the card with a live
// (non-revoked) passport, click Revoke, leave the confirm modal open so the
// guarded state is what the render gate captures.
export const PassportRevokeConfirm: Story = {
  render: tab("agents", agentsTabRoutes),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const revokeButton = await canvas.findByRole("button", { name: "Revoke" });
    await userEvent.click(revokeButton);
  },
};

// Minting: a name and five scope ticks in a confirm-width dialog, where the
// token is then shown once.
export const PassportMintDialog: Story = {
  name: "Mint a passport",
  render: tab("agents", agentsTabRoutes),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New passport" }),
    );
  },
};

// After the mint: the passport once, with its own copy control, and the ways to
// use it under it. The example keeps the variable, never the new value.
const mintedRoutes = {
  ...agentsTabRoutes,
  "POST /passports": () =>
    jsonResponse({
      id: "pp-new",
      label: "Claude Code on my laptop",
      scopes: ["read", "draft"],
      created_at: "2026-10-01T08:00:00Z",
      expires_at: "2026-10-31T08:00:00Z",
      revoked_at: null,
      token: "mgp_7Hq2vXkP9rLw4Tn8sYc1Zb6Ud3Fe0Ga5Jm",
    }),
};

const mintPassport = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "New passport" }),
  );
  // The Modal portals into the document body, outside the canvas.
  const body = within(canvasElement.ownerDocument.body);
  const dialog = within(await body.findByRole("dialog"));
  await userEvent.click(dialog.getByRole("button", { name: "Mint passport" }));
  await dialog.findByText("Next, use it");
};

export const PassportMinted: Story = {
  name: "Mint a passport — minted",
  render: tab("agents", mintedRoutes),
  play: mintPassport,
};

export const PassportMintedDark: Story = {
  name: "Mint a passport — minted, dark",
  globals: { theme: "dark" },
  render: tab("agents", mintedRoutes),
  play: mintPassport,
};

// The same dialog in dark, because the fieldset's legend, the checkbox rows and
// the recessed token plate are three surfaces whose separation is carried by
// tokens that move between themes.
export const PassportMintDialogDark: Story = {
  name: "Mint a passport — dark",
  globals: { theme: "dark" },
  render: tab("agents", agentsTabRoutes),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New passport" }),
    );
  },
};

// The governed tool console renders the inventory unfiltered by default,
// then strikes the send_email row once the read-only "Scout" passport (whose
// only granted scope is "read") is selected — its required "send" scope
// is absent from that grant.
const toolConsoleRoutes = agentsTabRoutes;

// Selects the read-only passport, so the send_email row is dimmed. Shared with
// the dark variant below, which is about that dimming and nothing else.
const selectScoutPassport = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  const canvas = within(canvasElement);
  await canvas.findByText("search_records");
  // The listbox is portalled to the body, outside this story's canvas, so the
  // pick goes through the shared helper rather than a canvas-scoped query.
  await pickOption(
    userEvent.setup(),
    canvas.getByRole("combobox", { name: "All passports" }),
    "Reachable by Scout",
  );
};

export const AgentToolConsole: Story = {
  render: tab("agents", toolConsoleRoutes),
  play: selectScoutPassport,
};

// The unreachable row is dimmed, and dimming is the one signal that does not
// survive a theme swap by construction: on a light ground it reads as "faded
// toward the paper", on a dark one the same reduction moves the text toward the
// background it is meant to stay legible against. This is the story that says
// whether the dim row is still readable text or has become a grey smear — and
// whether the tier/egress badges beside it still separate from each other.
export const AgentToolConsoleDark: Story = {
  globals: { theme: "dark" },
  render: tab("agents", toolConsoleRoutes),
  play: selectScoutPassport,
};

// The shape a record takes, on one page: the field editor, the pipeline
// designer, the product list and the offer templates. The four surfaces used to
// be three separate screens behind door-cards, and the doors are gone.
//
// The custom_field READ is what opens the entry — opening a page is reading it,
// and `meFixture` grants only the verbs named here. A write-only fixture reaches
// no entry at all and the story silently captures the access boundary instead,
// which is exactly what it did: nothing asserts on a story, so the gates stayed
// green while the picture was of the wrong page. The writes stay so the builder
// and the row actions render.
export const DataModelTab: Story = {
  render: tab("data-model", {
    "GET /me": me({ custom_field: ["read", "create", "update"] }),
  }),
};

// The consent registry, the retention ladder and the subject-request queue. The
// audit trail that proves them honoured is its own page.
export const PrivacyTab: Story = {
  // `contact` AND `consent_config` open this entry: the registry reads through
  // `contact` server-side (consent/store.go), and every seat holds that alone.
  // Without the pair `SettingsScreen` answers the address with the access
  // boundary, and the story captures the refusal under the name of a page it
  // never rendered — the failure the comment two stories up describes.
  render: tab("privacy", {
    "GET /me": me({ contact: ["read"], consent_config: ["read"] }),
  }),
};

const privacyRoutes = {
  "GET /me": me({ contact: ["read"], consent_config: ["read"] }),
};

// A whole settings PAGE at 390px, which is the thing only this file can show —
// every other story in the settings tree renders one card with nothing above or
// below it. `layout: "fullscreen"` because SettingsScreen brings `.wrap`, which
// carries production's own gutter; the canvas frame would add a second one and
// make this a 326px phone instead of a 390px one.
//
// What it watches is the seam BETWEEN cards rather than any one card's insides:
// three panels stack here, each with a title and an action button on the same
// header line, and one of them is a withheld body whose whole content is a
// sentence explaining a denial. A page is where the panel header's title/action
// split and the stack's own rhythm have to hold at once — and where the DSR facet
// control runs out of width first.
export const PrivacyTabPhone: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: tab("privacy", privacyRoutes),
};

// The Maintenance entry's danger zone: the ONE control on this screen that
// destroys an installation's data, so it is gated twice — the literal admin role
// AND the switch a deployment arms — and it is one ROW now, with the verb in the
// right column beside what it does. The whole card is still absent on either
// gate: an action-only surface holds no fact a reader could misread as "zero".
export const MaintenanceDangerZone: Story = {
  name: "Maintenance — danger zone",
  render: tab("maintenance", {
    "GET /me": () =>
      jsonResponse({
        ...meFixture({
          roles: ["admin"],
          allow: { embedding_reindex: ["read", "update"] },
        }),
        workspace_name: "Acme Inc",
        data_reset_available: true,
      }),
    "GET /admin/job-health": () =>
      jsonResponse({
        generated_at: "2026-08-13T09:30:00Z",
        kinds: [],
        recent_failures: [],
      }),
  }),
};

// PipelinesCard (D-8, on the Data model entry) reads GET /me (roles →
// pipeline grant) and GET /pipelines. Rendered directly here so
// the admin write affordances vs the rep read-only state each get a story.
// Several pipelines, because telling them apart is what the catalog is for: a
// default, two more in use of different lengths, and one retired.
function fixtureStage(
  pipelineId: string,
  n: number,
  name: string,
  semantic: "open" | "won" | "lost",
  winProbability: number,
) {
  return {
    id: `${pipelineId}-s${n}`,
    pipeline_id: pipelineId,
    name,
    position: n,
    semantic,
    win_probability: winProbability,
  };
}
const pipelinesFixture = {
  data: [
    {
      id: "pl",
      name: "Sales",
      is_default: true,
      position: 1,
      version: 3,
      stages: [
        fixtureStage("pl", 1, "Qualify", "open", 20),
        fixtureStage("pl", 2, "Proposal", "open", 50),
        fixtureStage("pl", 3, "Won", "won", 100),
        fixtureStage("pl", 4, "Lost", "lost", 0),
      ],
    },
    {
      id: "pl-ent",
      name: "Enterprise",
      is_default: false,
      position: 2,
      version: 5,
      stages: [
        fixtureStage("pl-ent", 1, "Qualified", "open", 5),
        fixtureStage("pl-ent", 2, "Technical validation", "open", 30),
        fixtureStage("pl-ent", 3, "Business case", "open", 45),
        fixtureStage("pl-ent", 4, "Procurement", "open", 75),
        fixtureStage("pl-ent", 5, "Legal review", "open", 90),
        fixtureStage("pl-ent", 6, "Won", "won", 100),
        fixtureStage("pl-ent", 7, "Lost", "lost", 0),
      ],
    },
    {
      id: "pl-renew",
      name: "Renewals",
      is_default: false,
      position: 3,
      version: 2,
      stages: [
        fixtureStage("pl-renew", 1, "Upcoming", "open", 40),
        fixtureStage("pl-renew", 2, "Terms sent", "open", 80),
        fixtureStage("pl-renew", 3, "Renewed", "won", 100),
        fixtureStage("pl-renew", 4, "Churned", "lost", 0),
      ],
    },
    {
      id: "pl-old",
      name: "Events 2025",
      is_default: false,
      position: 4,
      version: 7,
      archived_at: "2026-03-01T00:00:00Z",
      stages: [
        fixtureStage("pl-old", 1, "Meeting booked", "open", 40),
        fixtureStage("pl-old", 2, "Won", "won", 100),
        fixtureStage("pl-old", 3, "Lost", "lost", 0),
      ],
    },
  ],
  page: { next_cursor: null, has_more: false },
};

const pipelineMe = (allow: GrantSpec) =>
  jsonResponse({
    ...meFixture({ allow }),
    user: { ...meFixture().user, id: "u-1", display_name: "Me" },
  });

// useMe() fails fast without a workspace slug, collapsing the admin state into
// read-only — seed the slug so /me resolves and the affordances render.
function pipelinesCard(allow: GrantSpec) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": () => pipelineMe(allow),
      "GET /pipelines": () => jsonResponse(pipelinesFixture),
    });
    return (
      <StoryProviders>
        <PipelinesCard />
      </StoryProviders>
    );
  };
}

export const PipelinesAdmin: Story = {
  render: pipelinesCard({ pipeline: ["read", "create", "update"] }),
};

export const PipelinesReadOnly: Story = {
  render: pipelinesCard({ pipeline: ["read"] }),
};

// The narrow render of the ladder's own breakpoint. A `.stage-row` carries a
// fixed step and odds track beside the verbs, so on a phone those ARE the width
// and the stage name has nothing left; under 560px the verbs take a line of
// their own instead. The ladder is what makes this the right story on this
// page: the other cards answer their list routes from the stub's empty-page
// fallback, so a page-level narrow story would picture empty states and a head.
export const PipelinesAdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: pipelinesCard({ pipeline: ["read", "create", "update"] }),
};

// And the dark render of the same page, the densest real content it has: the
// catalog's compact strips, the open ladder's shading, the Won and Lost plates
// beside the Default badge, and the `.t-num` odds. What it watches is whether the
// outcome plates stay distinguishable from each other and from the rows behind
// them once the ground goes dark: each is tinted text on a tinted surface.
export const PipelinesAdminDark: Story = {
  globals: { theme: "dark" },
  render: pipelinesCard({ pipeline: ["read", "create", "update"] }),
};
