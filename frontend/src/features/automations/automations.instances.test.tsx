// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en } from "../../i18n/en";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { AutomationsAdmin, type AutomationsView } from "./automations";
import { TEMPLATE_WORDS } from "./automations.catalogwords";
import {
  AUTOMATION_CATALOG,
  configuredAutomations,
} from "./automations.fixtures";

const OPERATOR: GrantSpec = {
  automation: ["create", "read", "update", "delete"],
};
const RECORDED_AT = Date.parse("2026-09-01T12:00:00Z");

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
});

function mount(
  allow: GrantSpec = OPERATOR,
  routes: RouteMap = {},
  view: AutomationsView = "mine",
  locale: "en" | "de" = "en",
) {
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /automations/catalog": () =>
      jsonResponse({ data: AUTOMATION_CATALOG }),
    "GET /automations": () =>
      jsonResponse({
        data: configuredAutomations(RECORDED_AT),
        page: { next_cursor: null },
      }),
    ...routes,
  });
  render(
    <StoryProviders locale={locale}>
      <AutomationsAdmin view={view} />
    </StoryProviders>,
  );
}

const row = (id: string) => screen.findByTestId(`automation-${id}`);

const templateWords = (key: string) => {
  const words = TEMPLATE_WORDS[key];
  if (!words) throw new Error(`No words for the template ${key}`);
  return { name: en[words.name], description: en[words.description] };
};

describe("the configured automations table", () => {
  it("names the authorizer for a reader without editing rights", async () => {
    const rules = configuredAutomations(RECORDED_AT).map((rule, index) =>
      index === 0 ? { ...rule, owner_id: "u-2" } : rule,
    );
    const members = () =>
      jsonResponse({
        data: [
          {
            id: "u-2",
            display_name: "Morgan",
            status: "active",
            is_agent: false,
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    mount(
      { automation: ["read"] },
      {
        "GET /automations": () =>
          jsonResponse({ data: rules, page: { next_cursor: null } }),
        "GET /users": members,
        "GET /users/names": members,
      },
    );
    const configured = within(await row("au-1"));
    expect(await configured.findByText("Morgan")).toBeVisible();
    expect(configured.queryByText(en["auto.ownerNone"])).toBeNull();
    expect(
      configured.queryByRole("combobox", { name: /authorized by/i }),
    ).toBeNull();
  });

  it("reads each rule's last run as an outcome word, and Never when it has none", async () => {
    mount();
    expect(await row("au-1")).toHaveTextContent(en["auto.runs.outcomeFired"]);
    const failed = within(await row("au-2")).getByText(
      en["auto.runs.outcomeFailed"],
    );
    expect(failed.closest(".badge")?.className).toContain("badge-danger");
    const blocked = within(await row("au-3")).getByText(
      en["auto.runs.outcomeBlocked"],
    );
    expect(blocked.closest(".badge")?.className).toContain("badge-warning");
    expect(await row("au-4")).toHaveTextContent(en["auto.runs.outcomeQueued"]);
    expect(
      within(await row("au-5")).getByText(en["auto.lastRunNever"]),
    ).toBeInTheDocument();
    const at = (await row("au-1")).querySelector("time");
    expect(at?.getAttribute("dateTime")).toBe(
      configuredAutomations(RECORDED_AT)[0].last_run_at,
    );
  });

  it("puts the 30-day run count in an end-aligned column, grouped for the reader", async () => {
    mount();
    const heading = await screen.findByRole("columnheader", {
      name: en["auto.colRuns30"],
    });
    expect(heading.className).toContain("datatable-end");
    const figure = within(await row("au-4")).getByText("1,240");
    expect(figure.closest("td")?.className).toContain("datatable-end");
    expect(within(await row("au-6")).getByText("0")).toBeInTheDocument();
    // A rule that never ran says so once: no count beside "Never".
    expect(within(await row("au-5")).queryByText("0")).toBeNull();
  });

  it("captions a rule the system paused with why, and leaves one paused by hand bare", async () => {
    mount();
    const paused = await row("au-6");
    expect(
      within(paused).getByRole("button", {
        name: en["auto.pausedShort.listArchived"],
      }),
    ).toBeInTheDocument();
    expect(await row("au-5")).not.toHaveTextContent(/Paused ·/);
  });

  it("marks a rule that waits for approval, and tells a screen reader one runs by itself", async () => {
    mount();
    const approval = within(await row("au-4")).getByText(
      en["auto.tier.approval"],
    );
    expect(approval.closest(".badge")?.className).toContain("badge-warning");
    expect(
      within(await row("au-4")).queryByText(en["auto.modeAuto"]),
    ).toBeNull();
    expect(
      within(await row("au-5")).getByText(en["auto.tier.approval"]),
    ).toBeVisible();
    const auto = within(await row("au-1")).getByText(en["auto.modeAuto"]);
    expect(auto).toHaveClass("sr-only");
    expect(
      within(await row("au-1")).queryByText(en["auto.tier.approval"]),
    ).toBeNull();
  });

  it("says what a rule does in words, never in its key or raw parameters", async () => {
    mount();
    const first = await row("au-1");
    expect(first).toHaveTextContent("Quiet accounts follow-up");
    expect(first).toHaveTextContent(
      `${en["auto.trigger.noActivity"]}: ${en["auto.action.createTask"]}`,
    );
    for (const raw of ["no_activity_reminder", "days=", "create_task"]) {
      expect(first).not.toHaveTextContent(raw);
    }
    expect(await row("au-6")).not.toHaveTextContent(/list_id|list\.evaluated/);
  });
});

/** Records the If-Match of every PATCH the page sends. */
function recordIfMatch(): (string | null)[] {
  const stubbed = globalThis.fetch;
  const sent: (string | null)[] = [];
  globalThis.fetch = async (input, init) => {
    const request = new Request(input, init);
    if (request.method === "PATCH") {
      sent.push(request.headers.get("If-Match"));
    }
    return stubbed(input, init);
  };
  return sent;
}

const flipRecap = async (user: ReturnType<typeof userEvent.setup>) =>
  user.click(
    within(await row("au-5")).getByRole("switch", {
      name: "Post-meeting recap draft is enabled",
    }),
  );

describe("a rule's switch", () => {
  it("pins the flip to the version the row was read at", async () => {
    const user = userEvent.setup();
    mount();
    const sent = recordIfMatch();
    await flipRecap(user);
    await waitFor(() => expect(sent).toEqual(["2"]));
  });

  it("reads the list again after a version_skew refusal, so the next flip carries the new version", async () => {
    const user = userEvent.setup();
    let version = 2;
    const rules = () =>
      configuredAutomations(RECORDED_AT).map((rule) =>
        rule.id === "au-5" ? { ...rule, version } : rule,
      );
    mount(OPERATOR, {
      "GET /automations": () =>
        jsonResponse({ data: rules(), page: { next_cursor: null } }),
      "PATCH /automations/au-5": () => {
        if (version === 2) {
          version = 3;
          return jsonResponse(
            { title: "Conflict", status: 409, code: "version_skew" },
            409,
          );
        }
        return jsonResponse({ ...rules()[4], status: "enabled" });
      },
    });
    const sent = recordIfMatch();
    await flipRecap(user);
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    await waitFor(() =>
      expect(
        within(screen.getByTestId("automation-au-5"))
          .getByRole("switch")
          .getAttribute("aria-busy"),
      ).not.toBe("true"),
    );
    await flipRecap(user);
    await waitFor(() => expect(sent).toEqual(["2", "3"]));
  });
});

describe("a rule's owner", () => {
  it("hands the rule to the member chosen, pinned to the version the row was read at", async () => {
    const user = userEvent.setup();
    const bodies: unknown[] = [];
    mount(OPERATOR, {
      "GET /users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Dana Rep", is_agent: false },
            { id: "u-2", display_name: "Lee Closer", is_agent: false },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "PATCH /automations/au-5": () =>
        jsonResponse(configuredAutomations(RECORDED_AT)[4]),
    });
    const sent = recordIfMatch();
    const stubbed = globalThis.fetch;
    globalThis.fetch = async (input, init) => {
      const request = new Request(input, init);
      if (request.method === "PATCH") {
        bodies.push(await request.clone().json());
      }
      return stubbed(input, init);
    };
    const picker = await within(await row("au-5")).findByRole("combobox", {
      name: "Person authorizing Post-meeting recap draft",
    });
    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: "Lee Closer" }));
    await waitFor(() => expect(bodies).toEqual([{ owner_id: "u-2" }]));
    expect(sent).toEqual(["2"]);
  });
});

describe("a rule's run history", () => {
  it("opens as a detail row under its rule, from the keyboard, and closes again", async () => {
    const user = userEvent.setup();
    mount();
    const renewal = await row("au-2");
    const toggle = within(renewal).getByRole("button", {
      name: "Run history of Renewal reminder",
    });
    toggle.focus();
    await user.keyboard("{Enter}");
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    const detail = document.getElementById(
      toggle.getAttribute("aria-controls") ?? "",
    );
    if (!detail) {
      throw new Error("the toggle names no detail");
    }
    expect(detail.closest("tr")?.previousElementSibling).toBe(renewal);
    // A plain body under the row, not a card: no heading and no dial while
    // the history is empty.
    expect(
      await within(detail).findByText(en["auto.runs.empty"]),
    ).toBeInTheDocument();
    expect(within(detail).queryByRole("heading")).toBeNull();
    expect(within(detail).queryByRole("combobox")).toBeNull();
    await user.keyboard("{Enter}");
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });
});

describe("the starter library", () => {
  it("puts each grouped template under its catalog section", async () => {
    mount(OPERATOR, {}, "library");
    const meetings = await screen.findByRole("table", {
      name: en["auto.group.meetings"],
    });
    expect(
      within(meetings).getByTestId("template-post_meeting_recap"),
    ).toBeInTheDocument();
    expect(
      within(meetings).queryByTestId("template-renewal_reminder"),
    ).toBeNull();
    const customers = screen.getByRole("table", {
      name: en["auto.group.customers"],
    });
    expect(
      within(customers).getByTestId("template-renewal_reminder"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("template-close_approval")).toBeInTheDocument();
  });

  it("opens a template's create dialog from a press anywhere on its row", async () => {
    const user = userEvent.setup();
    mount(OPERATOR, {}, "library");
    const template = await screen.findByTestId("template-renewal_reminder");
    expect(template).not.toHaveTextContent(/clock:|create_task|->/);
    expect(template).toHaveTextContent(en["auto.trigger.renewal"]);
    const renewal = templateWords("renewal_reminder");
    await user.click(within(template).getByText(renewal.name));
    const dialog = await screen.findByRole("dialog");
    expect(
      within(dialog).getByRole("heading", { name: renewal.name }),
    ).toBeInTheDocument();
  });

  it("reaches the same verb from the keyboard, and Escape puts it away", async () => {
    const user = userEvent.setup();
    mount(OPERATOR, {}, "library");
    const template = await screen.findByTestId("template-post_meeting_recap");
    within(template)
      .getByRole("button", {
        name: "Use template: Draft a follow-up after a meeting",
      })
      .focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  // The row says what the template does; the dialog it opens reads the rest,
  // so no description is ever cut off mid-sentence.
  it("reads a template's whole description in the dialog its row opens", async () => {
    const user = userEvent.setup();
    mount(OPERATOR, {}, "library");
    const template = await screen.findByTestId("template-stage_change_notify");
    expect(template).not.toHaveTextContent(/deal history|…/);
    await user.click(
      within(template).getByRole("button", { name: /^Use template/ }),
    );
    expect(
      within(await screen.findByRole("dialog")).getByText(
        templateWords("stage_change_notify").description,
      ),
    ).toBeInTheDocument();
  });

  it("stays readable without the create grant, but no row opens", async () => {
    const user = userEvent.setup();
    mount({ automation: ["read"] }, {}, "library");
    const template = await screen.findByTestId("template-renewal_reminder");
    expect(template.className).not.toContain("rowlink");
    expect(within(template).queryByRole("button")).toBeNull();
    await user.click(
      within(template).getByText(templateWords("renewal_reminder").name),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

describe("a rule's Actions menu", () => {
  it.each([
    [en["auto.runs.open"], en["auto.runs.title"]],
    [en["auto.preview.open"], en["auto.preview.title"]],
  ])(
    "closes after %s and puts focus on the panel it opened",
    async (item, panel) => {
      const user = userEvent.setup();
      mount();
      const trigger = within(await row("au-1")).getByRole("button", {
        name: "Actions for Quiet accounts follow-up",
      });
      await user.click(trigger);
      await user.click(screen.getByRole("button", { name: item }));
      expect(trigger).toHaveAttribute("aria-expanded", "false");
      expect(screen.queryByRole("button", { name: item })).toBeNull();
      await waitFor(() =>
        expect(document.activeElement).toBe(
          screen.getByRole("region", { name: panel }),
        ),
      );
    },
  );
});

describe("the catalog's words", () => {
  it("reads the starter library in the reader's language", async () => {
    mount(OPERATOR, {}, "library", "de");
    const template = await screen.findByTestId("template-route_lead");
    expect(template).toHaveTextContent(de["auto.template.route_lead.name"]);
    expect(template).not.toHaveTextContent(/Route new lead|Creates/);
  });

  it("names a rule that keeps its template's name in the reader's language", async () => {
    const seeded = configuredAutomations(RECORDED_AT).map((rule) =>
      rule.id === "au-2" ? { ...rule, name: "Prepare for a renewal" } : rule,
    );
    mount(
      OPERATOR,
      {
        "GET /automations": () =>
          jsonResponse({ data: seeded, page: { next_cursor: null } }),
      },
      "mine",
      "de",
    );
    expect(await row("au-2")).toHaveTextContent(
      de["auto.template.renewal_reminder.name"],
    );
    expect(await row("au-1")).toHaveTextContent("Quiet accounts follow-up");
  });

  it("says a task goes to the record owner without a capital mid-sentence", async () => {
    mount();
    expect(await row("au-2")).toHaveTextContent(
      en["auto.recipient.tasksToOwner"],
    );
  });
});
