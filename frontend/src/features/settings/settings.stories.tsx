// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { pickOption } from "../../design-system/select-testing";
import type { SettingsPageId } from "../../lib/settingscatalog";
import { readingsDay } from "../../testing/brief.fixtures";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { SettingsScreen, settingsAddress } from "./settings";
import { PipelinesCard } from "./settings.pipelines";
import {
  COMPANY_CONTEXT_OFF,
  EVERY_GRANT,
  NOTIFICATION_ROUTES,
  UNUSED_INSTALLATION_ROUTES,
} from "./settingspages.fixtures";

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
// personal "AI access" entry, which no grant gates.
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
          "Find contacts, companies, deals, leads and projects by name.",
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
// The shell around every tab reads the worklist, and its queue, summary and
// caveat lists are all required by the contract. An unrouted read falls to the
// generic empty page, which carries none of them, so the page fails in a way
// no server could produce.
const SHELL: RouteMap = {
  "GET /worklist": () => jsonResponse(readingsDay({}, [])),
};

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
    installFetchStub({ ...SHELL, ...WORKING_HOURS, ...routes });
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

// Profile, security, and language and appearance: three panels, each answer at
// the same x down the page.
export const AccountTab: Story = {
  render: tab("account", { "GET /me": me() }),
};

// And in dark, because the hairline between two decisions and the avatar's tint
// are derived values that move between themes.
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

// The notices by kind, then the two email summaries, as one page.
export const NotificationsTab: Story = {
  render: tab("notifications", { "GET /me": me(), ...NOTIFICATION_ROUTES }),
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

// The reader's AI access: the connect picker first, then the tools already
// connected, with every developer card behind "For developers".
const aiAccessRoutes = {
  "GET /me": me(),
  "GET /passports": passports,
  "GET /agent-tools": tools,
  "GET /.well-known/oauth-protected-resource": connectorOn,
};

async function openDevelopers(canvasElement: HTMLElement, tab?: string) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByText("For developers"));
  if (tab !== undefined) {
    await userEvent.click(await canvas.findByRole("button", { name: tab }));
  }
}

export const AiAccess: Story = {
  render: tab("agents", aiAccessRoutes),
};

export const AiAccessDark: Story = {
  globals: { theme: "dark" },
  render: tab("agents", aiAccessRoutes),
};

// At 390px the two questions wrap and the steps stay one column.
export const AiAccessPhone: Story = {
  name: "AI access — phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: tab("agents", aiAccessRoutes),
};

// For developers open on its first tab, API keys.
export const AiAccessDevelopersOpen: Story = {
  name: "AI access — for developers open",
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) => openDevelopers(canvasElement),
};

export const AiAccessCodingAgents: Story = {
  name: "AI access — coding agents",
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) =>
    openDevelopers(canvasElement, "Coding agents"),
};

export const AiAccessAgentRules: Story = {
  name: "AI access — agent rules",
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) =>
    openDevelopers(canvasElement, "Agent rules"),
};

// The connector switched off: the picker and the coding-agents card both say so
// and say what still works. It is the state a default install is in.
export const AiAccessConnectorOff: Story = {
  name: "AI access — connector off",
  render: tab("agents", {
    ...aiAccessRoutes,
    "GET /.well-known/oauth-protected-resource": connectorOff,
  }),
  play: async ({ canvasElement }) =>
    openDevelopers(canvasElement, "Coding agents"),
};

// The row's menu opens the revoke confirm, left open here so the guarded
// state is what the render gate captures.
export const PassportRevokeConfirm: Story = {
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) => {
    await openDevelopers(canvasElement);
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Scout" }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Revoke Scout" }),
    );
  },
};

// Minting: a name and five scope ticks in a confirm-width dialog, where the
// token is then shown once.
export const PassportMintDialog: Story = {
  name: "Mint a passport",
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) => {
    await openDevelopers(canvasElement);
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New passport" }),
    );
  },
};

// After the mint: the passport once, with its own copy control, and the ways to
// use it under it. The example keeps the variable, never the new value.
const mintedRoutes = {
  ...aiAccessRoutes,
  "POST /passports": () =>
    jsonResponse({
      id: "pp-new",
      label: "Claude Code on my laptop",
      scopes: ["read", "draft"],
      created_at: "2026-10-01T08:00:00Z",
      expires_at: "2026-10-31T08:00:00Z",
      revoked_at: null,
      token: "mgp_example_passport",
    }),
};

const mintPassport = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  await openDevelopers(canvasElement);
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
  render: tab("agents", aiAccessRoutes),
  play: async ({ canvasElement }) => {
    await openDevelopers(canvasElement);
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New passport" }),
    );
  },
};

// The tool groups unfiltered, then scoped to the read-only "Scout" passport: the
// send_email row is struck, because "send" is not in that grant.
const toolConsoleRoutes = aiAccessRoutes;

const selectScoutPassport = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  await openDevelopers(canvasElement, "Agent rules");
  const canvas = within(canvasElement);
  // The listbox is portalled to the body, outside this story's canvas, so the
  // pick goes through the shared helper rather than a canvas-scoped query.
  const passport = await canvas.findByRole("combobox", {
    name: "All passports",
  });
  const disclosure = passport.closest("details");
  if (!disclosure)
    throw new Error("The passport filter needs its developer section.");
  await Promise.all(
    disclosure.getAnimations().map((motion) => motion.finished),
  );
  passport.scrollIntoView({ block: "center", behavior: "instant" });
  await pickOption(userEvent.setup(), passport, "Reachable by Scout");
  await userEvent.click(await canvas.findByText("Send messages"));
};

export const AgentToolConsole: Story = {
  render: tab("agents", toolConsoleRoutes),
  play: selectScoutPassport,
};

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

// The reset page's danger zone: the one control that destroys an
// installation's data, gated on the grant and on the switch a deployment arms.
export const ResetDangerZone: Story = {
  name: "Reset data — danger zone",
  render: tab("reset", {
    "GET /me": () =>
      jsonResponse({
        ...meFixture({
          roles: ["admin"],
          allow: { system_reset: ["delete"] },
        }),
        workspace_name: "Acme Inc",
        data_reset_available: true,
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
    version: 1,
    ...(semantic === "open"
      ? { exit_criteria: { required: n % 2, optional: n > 2 ? 1 : 0 } }
      : {}),
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

// The ladder at phone width. A page-level phone story would picture only empty
// states, since the other cards answer from the stub's empty page.
export const PipelinesAdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: pipelinesCard({ pipeline: ["read", "create", "update"] }),
};

// The dark render of the same page, its densest real content. The compact
// strips, the meters and the Default badge are each tinted on a tinted ground.
export const PipelinesAdminDark: Story = {
  globals: { theme: "dark" },
  render: pipelinesCard({ pipeline: ["read", "create", "update"] }),
};

// Every catalog page whole, for an admin holding every grant, over an unused
// installation. `settingssections.test.tsx` pins search's sections to these.
const everyGrantMe = () =>
  jsonResponse({
    ...meFixture({ roles: ["admin"], allow: EVERY_GRANT }),
    data_reset_available: true,
  });

function catalogPage(page: SettingsPageId): Story {
  return {
    parameters: { settingsPage: page },
    render: tab(page, {
      "GET /me": everyGrantMe,
      ...UNUSED_INSTALLATION_ROUTES,
    }),
  };
}

export const AccountPage: Story = catalogPage("account");
export const MeetingsPage: Story = catalogPage("meetings");
export const VoicePage: Story = catalogPage("voice");
export const AgentsPage: Story = catalogPage("agents");
export const NotificationsPage: Story = catalogPage("notifications");
export const ConnectionsPage: Story = catalogPage("connections");
export const CaptureActivityPage: Story = catalogPage("capture-activity");
export const CompanyPage: Story = catalogPage("company");
export const AuthenticationPage: Story = catalogPage("authentication");
export const MembersPage: Story = catalogPage("members");
export const TeamsPage: Story = catalogPage("teams");
export const RolesPage: Story = catalogPage("roles");
export const SeatsPage: Story = catalogPage("seats");
export const PipelinesPage: Story = catalogPage("pipelines");
export const StageautomationPage: Story = catalogPage("stageautomation");
export const LeadsPage: Story = catalogPage("leads");
export const AcquisitionPage: Story = catalogPage("acquisition");
export const LossreasonsPage: Story = catalogPage("lossreasons");
export const ReviewtemplatesPage: Story = catalogPage("reviewtemplates");
export const RecordrolesPage: Story = catalogPage("recordroles");
export const FieldsPage: Story = catalogPage("fields");
export const TagsPage: Story = catalogPage("tags");
export const ProductsPage: Story = catalogPage("products");
export const CapturePage: Story = catalogPage("capture");
export const IntegrationsPage: Story = catalogPage("integrations");
export const WebFormsPage: Story = catalogPage("web-forms");
export const KnowledgePage: Story = catalogPage("knowledge");
export const ImportPage: Story = catalogPage("import");
export const UsagePage: Story = catalogPage("usage");
export const ModelsPage: Story = catalogPage("models");
export const McpPage: Story = catalogPage("mcp");
export const ModelCallsPage: Story = catalogPage("model-calls");
export const AutomationsPage: Story = catalogPage("automations");
export const PrivacyPage: Story = catalogPage("privacy");
export const JurisdictionPage: Story = catalogPage("jurisdiction");
export const AuditPage: Story = catalogPage("audit");
export const SystemHealthPage: Story = catalogPage("system-health");
export const ConfigurationPage: Story = catalogPage("configuration");
export const ExtensionsPage: Story = catalogPage("extensions");
export const ResetPage: Story = catalogPage("reset");

// Company profile while the rollout keeps the company context unread: the card
// draws nothing, and search offers none of its names.
export const CompanyPageContextOff: Story = {
  parameters: { companyContext: "off" },
  render: tab("company", {
    "GET /me": everyGrantMe,
    ...UNUSED_INSTALLATION_ROUTES,
    "GET /company/context/capabilities": () =>
      jsonResponse(COMPANY_CONTEXT_OFF),
  }),
};
