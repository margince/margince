// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { jsonResponse } from "../../testing/company.fixtures";
import { meFixture } from "../../testing/mefixture";
import { project, project360 } from "../../testing/projects.fixtures";
import { RecordShell } from "../../testing/recordshell.testkit";
import { ProjectScreen } from "./project360";
import { projectsBackend } from "./projects.testing";

// The project page: every section drawn from one Project360, a withheld
// section saying so rather than reading as empty, and the close dialog that
// holds its Confirm until a reason is given and then posts exactly
// `{to_phase: "closed", reason}` under the project's version.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function lastOf(nodes: HTMLElement[]): HTMLElement {
  const node = nodes.at(-1);
  if (!node) {
    throw new Error("no such control on the page");
  }
  return node;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("ProjectScreen", () => {
  it("renders the head, how it stands, the figures and the sections", async () => {
    projectsBackend({
      view: project360({
        deals: {
          data: [
            {
              id: "d-1",
              name: "Phase one licence",
              pipeline_id: "pl",
              stage_id: "s1",
              status: "won",
              amount_minor: 450_000,
              currency: "EUR",
              source: "manual",
              captured_by: "u-me",
              created_at: "2026-06-02T09:00:00Z",
              updated_at: "2026-06-02T09:00:00Z",
            },
          ],
          page: { next_cursor: null, has_more: false },
        },
      }),
    });
    render(<ProjectScreen id="pr-1" />);
    expect(
      await screen.findByRole("heading", { name: "CRM rollout" }),
    ).toBeTruthy();
    expect(screen.getByText("ACME-CRM")).toBeTruthy();
    // The account, once in the head's lines and once under About.
    expect(await screen.findAllByText("Brandt Automotive")).toHaveLength(2);

    // Nobody has judged the project, so it reads as not assessed, never as on
    // track; nothing is due, so the next bar says so.
    expect(screen.getByText(en["standing.notAssessed"])).toBeTruthy();
    expect(screen.getByText(en["standing.next.none"])).toBeTruthy();

    // The ladder: the current phase is a marker, the others are moves.
    const stepper = screen.getByRole("group", { name: "Phase" });
    expect(
      stepper.querySelector('[aria-current="step"]')?.textContent,
    ).toContain("Initiative");
    expect(screen.getByTestId("project-step-closed").tagName).toBe("BUTTON");

    // The figures: the phase's days, the open tasks, no file attached.
    const figures = screen.getByTestId("project-figures");
    expect(within(figures).getByText("31 days")).toBeTruthy();
    expect(within(figures).getByText("4 open")).toBeTruthy();
    expect(
      within(figures).getByText(en["project.standing.noDocuments"]),
    ).toBeTruthy();
    expect(screen.queryByTestId("project-coverage")).toBeNull();

    // The four cards: a linked deal row, the phase history with its visit,
    // and empty states for contracts and tasks.
    expect(
      screen.getByRole("button", { name: "Phase one licence" }),
    ).toBeTruthy();
    expect(screen.getByText("Since 1 Jun · Me opened it")).toBeTruthy();
    expect(
      screen.getByText(/No contract is filed under this project/),
    ).toBeTruthy();
    expect(
      screen.getByText(/No open task is filed under this project/),
    ).toBeTruthy();

    // Nothing else: the stakeholders, the files, the health log and the
    // chronology open from the menus, not from the Overview.
    expect(screen.queryByText("Anna Weber")).toBeNull();
    expect(
      screen.queryByText(/No file is attached to this project/),
    ).toBeNull();
    expect(screen.queryByText("Delivery health")).toBeNull();
    expect(
      screen.queryByRole("region", { name: en["record.timeline"] }),
    ).toBeNull();
  });

  it("names an agent move, its reason and an unrecorded move on the phase history", async () => {
    projectsBackend({
      view: project360({
        phase_history: {
          data: [
            {
              id: "ph-1",
              from_phase: null,
              to_phase: "initiative",
              reason: null,
              changed_at: "2026-06-01T09:00:00Z",
            },
            {
              id: "ph-2",
              from_phase: "initiative",
              to_phase: "delivering",
              reason: "Contract signed",
              changed_at: "2026-06-10T09:00:00Z",
              changed_by: {
                actor_type: "agent",
                actor_id: "agent:p-1",
                actor_name: null,
                on_behalf_of: "u-1",
                on_behalf_of_name: "Anna Weber",
                agent_client: "Claude",
                passport_id: "p-1",
              },
            },
          ],
          phase_durations: [
            { phase: "initiative", seconds: 777_600, current: false },
            { phase: "delivering", seconds: 1_900_800, current: true },
          ],
        },
      }),
    });
    render(<ProjectScreen id="p-1" />);
    expect(
      await screen.findByText(
        "Since 10 Jun · Anna Weber, via Claude, moved it · Contract signed",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("1 Jun → 10 Jun · Who made this move was not recorded"),
    ).toBeTruthy();
    expect(screen.queryByText(/System/)).toBeNull();
  });

  it("opens the stakeholders and the companies from the More menu", async () => {
    const user = userEvent.setup();
    projectsBackend({ view: project360() });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(
      screen.getByRole("button", { name: en["project.more.contacts"] }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: en["project.more.contacts"],
    });
    expect(within(drawer).getByText("Anna Weber")).toBeTruthy();
    expect(within(drawer).getByText("Project lead")).toBeTruthy();
  });

  it("opens the files from the Contracts card", async () => {
    const user = userEvent.setup();
    projectsBackend({ view: project360() });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(
      screen.getByRole("button", { name: en["project.documents.title"] }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: en["project.documents.title"],
    });
    expect(
      within(drawer).getByText(/No file is attached to this project/),
    ).toBeTruthy();
  });

  // The update itself is read in the full history, which the More menu opens.
  it("opens the full history from the More menu", async () => {
    projectsBackend({ view: project360() });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    const user = userEvent.setup();
    await user.click(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["record.fullHistory"] }),
    );

    const drawer = await screen.findByRole("dialog", {
      name: en["record.fullHistory"],
    });
    expect(
      within(drawer).getByRole("region", { name: en["record.timeline"] }),
    ).toBeTruthy();
    expect(
      within(drawer).getByText(/Nothing is filed under this project yet/),
    ).toBeTruthy();
  });

  it("offers Relink and Reply on a timeline row", async () => {
    const user = userEvent.setup();
    projectsBackend({
      view: project360({
        activities: {
          data: [
            {
              offers_email: false,
              id: "act-1",
              kind: "email",
              writable: true,
              subject: "Kickoff agenda",
              occurred_at: "2026-07-01T09:00:00Z",
              is_done: false,
              source: "manual",
              captured_by: "human:u-1",
              created_at: "2026-07-01T09:00:00Z",
              updated_at: "2026-07-01T09:00:00Z",
            },
          ],
          page: { next_cursor: null, has_more: false },
        },
      }),
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(
      screen.getByRole("button", { name: en["record.fullHistory"] }),
    );
    const drawer = await screen.findByRole("dialog", {
      name: en["record.fullHistory"],
    });
    await within(drawer).findByText("Kickoff agenda");

    expect(within(drawer).getByRole("button", { name: "Reply" })).toBeTruthy();
    await user.click(within(drawer).getByRole("button", { name: "Relink" }));
    await waitFor(() =>
      expect(screen.getAllByRole("dialog").length).toBeGreaterThan(1),
    );
  });

  it("says a withheld section is hidden rather than drawing it empty", async () => {
    projectsBackend({
      view: project360({
        sections_omitted: [
          "contracts",
          "rollups",
          "coverage",
          "company",
          "activities",
        ],
        contracts: undefined,
        rollups: undefined,
        coverage: undefined,
        company: undefined,
        activities: undefined,
      }),
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    const withheld = screen.getAllByText("Hidden for your role");
    // The contracts, company references and both financial readings stay hidden.
    expect(withheld).toHaveLength(5);
    expect(
      screen.queryByText(/No contract is filed under this project/),
    ).toBeNull();
    expect(screen.queryByTestId("project-coverage")).toBeNull();
    expect(screen.queryByTestId("project-coverage-withheld")).toBeNull();
    expect(screen.getAllByTestId("project-company-withheld")).toHaveLength(2);
    expect(screen.queryByText("Brandt Automotive")).toBeNull();
  });

  it("closing asks for a reason, holds Confirm until it is given, and posts the move", async () => {
    const user = userEvent.setup();
    let posted: { body: unknown; ifMatch: string | null } | null = null;
    projectsBackend({
      respond: async (url, method, request) => {
        if (method === "POST" && url.endsWith("/projects/pr-1/advance")) {
          posted = {
            body: JSON.parse(await request.text()),
            ifMatch: request.headers.get("If-Match"),
          };
          return jsonResponse(project({ phase: "closed" }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    await user.click(screen.getByTestId("project-step-closed"));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Move to Closed")).toBeTruthy();
    const confirm = within(dialog).getByRole("button", {
      name: "Close project",
    });
    expect(confirm.hasAttribute("disabled")).toBe(true);

    await user.type(
      within(dialog).getByLabelText("Reason *"),
      "Delivered and signed off",
    );
    expect(confirm.hasAttribute("disabled")).toBe(false);
    await user.click(confirm);

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toEqual({
      body: { to_phase: "closed", reason: "Delivered and signed off" },
      ifMatch: "3",
    });
  });

  it("a move that is not a close needs no reason", async () => {
    const user = userEvent.setup();
    let posted: unknown = null;
    projectsBackend({
      respond: async (url, method, request) => {
        if (method === "POST" && url.endsWith("/advance")) {
          posted = JSON.parse(await request.text());
          return jsonResponse(project({ phase: "pursuing" }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    await user.click(screen.getByTestId("project-step-pursuing"));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Move" }));
    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toEqual({ to_phase: "pursuing", reason: null });
  });

  // `writable` is what the server's write gate would answer on a mutation. A
  // reader who may open a project but not change it was offered Edit, Archive,
  // Assign and the phase stepper, and learned otherwise from a 403 after
  // filling the form in.
  it("withholds the write controls on a project this caller may not change", async () => {
    const user = userEvent.setup();
    let posted = false;
    projectsBackend({
      view: project360({
        project: project({ owner_id: "u-someone-else", writable: false }),
      }),
      respond: async (url, method) => {
        if (method === "POST" && url.endsWith("/advance")) {
          posted = true;
          return jsonResponse(project({ phase: "pursuing" }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    // The page says why once, rather than each control failing on its own.
    expect(screen.getByText(/You cannot edit this project/)).toBeTruthy();

    // Every verb, not a sample of them: asserting one control would leave the
    // others free to regress one at a time. New Deal is here because binding a
    // deal to a project needs write authority OVER the project.
    for (const label of ["New deal"]) {
      expect(screen.queryByRole("button", { name: label })).toBeNull();
    }
    for (const label of [en["log.title"], en["log.addTask"]]) {
      expect(
        screen.getByRole("button", { name: label }).hasAttribute("disabled"),
      ).toBe(true);
    }
    // Inside the overflow menu the verbs are refused rather than dropped, so a
    // reader learns the verb exists and why it is shut. What must NOT happen is
    // a pressable one: clicking it opens the flow and the save 403s. Edit is
    // in here with them, and the id each describes itself by must resolve to
    // the band's sentence — which is why that sentence is minted OUTSIDE the
    // panel: one minted inside would name no element until the menu opened.
    await user.click(screen.getByRole("button", { name: "More actions" }));
    // Barred is the NATIVE disabled attribute — the design system reserves
    // aria-disabled for "busy" — and each carries aria-describedby pointing at
    // the one sentence in the band, so a reader is told why rather than left
    // with a dead control.
    for (const label of [
      /^Edit project/,
      /^Archive/,
      /^Assign/,
      /^Share/,
      /^Record reading/,
    ]) {
      const verbs = screen.queryAllByRole("button", { name: label });
      expect(verbs.length).toBeGreaterThan(0);
      for (const verb of verbs) {
        expect(
          `${verb.textContent} disabled=${verb.hasAttribute("disabled")}`,
        ).toBe(`${verb.textContent} disabled=true`);
        expect(
          document.getElementById(verb.getAttribute("aria-describedby") ?? "")
            ?.textContent,
        ).toMatch(/You cannot edit this project/);
      }
    }

    // The stepper is the one write control on the page that takes a single
    // click, so it is the one that would have posted before any dialog.
    await user.click(screen.getByTestId("project-step-pursuing"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(posted).toBe(false);
  });

  // A company keeps its controls on an ownerless record because the claim door
  // is what makes it writable. A project has no such door — ClaimRecord refuses
  // the type — and the server treats an ownerless row as nobody's to change. So
  // `writable` is the whole answer here, and an ownerless project the server
  // says is unwritable is drawn read-only like any other.
  it("refuses an unowned project the server says this caller cannot write", async () => {
    projectsBackend({
      view: project360({
        project: project({ owner_id: null, writable: false }),
      }),
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    expect(screen.getByText(/You cannot edit this project/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });

  // Absent is not "unknown", it is "no": a response from a server too old to
  // send the field must fail closed, or the fix is only as good as the oldest
  // server a client talks to.
  it("treats a project with no writable field as one it may not change", async () => {
    const withoutWritable = project({ owner_id: "u-someone-else" });
    delete (withoutWritable as { writable?: boolean }).writable;
    projectsBackend({
      view: project360({ project: withoutWritable }),
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });

    expect(screen.getByText(/You cannot edit this project/)).toBeTruthy();
  });

  // The owner list is portalled beside the menu that holds its trigger, so a
  // press inside it must not read to the menu as a press away from it.
  it("keeps the overflow menu open while the owner list is worked, and lands focus on a live control after the pick", async () => {
    const user = userEvent.setup();
    const patches: string[] = [];
    projectsBackend({
      view: project360(),
      respond: async (url, method, request) => {
        if (new URL(url).pathname === "/v1/users") {
          return jsonResponse({
            data: [{ id: "u-42", display_name: "Jane Doe", email: "j@x.test" }],
            page: { has_more: false },
          });
        }
        if (method === "PATCH") {
          patches.push(await request.text());
          return jsonResponse(project({ owner_id: "u-42", version: 4 }));
        }
        return null;
      },
    });
    render(<ProjectScreen id="pr-1" />);
    await screen.findByRole("heading", { name: "CRM rollout" });
    const more = screen.getByRole("button", { name: "More actions" });
    await user.click(more);
    const assign = screen.getByRole("button", {
      name: en["project.assignOwner"],
    });
    await user.click(assign);
    const list = await screen.findByRole("dialog", {
      name: en["project.assignOwner"],
    });

    await user.click(within(list).getByRole("combobox"));
    expect(more.getAttribute("aria-expanded")).toBe("true");
    expect(assign.closest("[hidden]")).toBeNull();

    // Escape answers the layer the reader is in, not the menu under it too.
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(more.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement).toBe(assign);
    await user.click(assign);
    const reopened = await screen.findByRole("dialog", {
      name: en["project.assignOwner"],
    });

    await user.click(
      await within(reopened).findByRole("option", { name: "Jane Doe" }),
    );
    await waitFor(() => expect(patches).toHaveLength(1));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const landed = document.activeElement;
    expect(landed).not.toBe(document.body);
    expect(landed?.isConnected).toBe(true);
    expect(landed?.closest("[hidden]")).toBeNull();
  });
});

// A task's verbs answer to the TASK's permission, not the project's.
//
// They are different questions with different answers. The modal's verbs PATCH
// /activities/{id}, which asks activity:update and the activity's own row
// authority; the project asks whether this record takes changes. Deriving one
// from the other offers verbs the server refuses to a reader who may write the
// project and not its activities — and hides valid ones from a task's own
// author whenever the project is read-only.
describe("a commitment's task detail", () => {
  const commitment = {
    activity_id: "a-1",
    subject: "Confirm the depot slot",
    due_at: null,
    assignee_id: null,
    assignee_name: null,
    overdue: false,
  };

  const TASK = {
    id: commitment.activity_id,
    kind: "task" as const,
    subject: commitment.subject,
    is_done: false,
    version: 1,
    source: "manual" as const,
    captured_by: "u-me",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    occurred_at: "2026-09-01T09:00:00Z",
  };

  function showCommitments(activityActions: ("update" | "delete")[]) {
    projectsBackend({
      view: project360({
        commitments: { data: [commitment], page: { has_more: false } },
      }),
      respond: async (url) => {
        if (url.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({
              allow: {
                // Writable PROJECT throughout, so the only thing that moves
                // between the two cases is the activity grant.
                project: ["read", "create", "update", "delete"],
                activity: ["read", ...activityActions],
              },
            }),
          );
        }
        return url.includes(`/v1/activities/${commitment.activity_id}`)
          ? jsonResponse(TASK)
          : null;
      },
    });
  }

  it("offers no task verbs to a reader who may write the project but not its activities", async () => {
    showCommitments([]);
    render(<ProjectScreen id={project().id} />);

    // The subject opens the task from the next bar and from its card row.
    await userEvent.click(
      lastOf(
        await screen.findAllByRole("button", { name: commitment.subject }),
      ),
    );

    // The task opened — the refusal below is about the VERBS, not about the
    // reader being unable to look. The subject appears twice by design: the
    // row it was opened from, and the modal's own heading.
    expect(
      await screen.findByRole("heading", { name: commitment.subject }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["tasks.complete"] }),
    ).toBeNull();
  });

  // The admit case. Without it the refusal above passes against a modal that
  // shows no verbs to anyone — including the reader who may use them.
  it("offers them to a reader who may write activities", async () => {
    showCommitments(["update"]);
    render(<ProjectScreen id={project().id} />);

    // The subject opens the task from the next bar and from its card row.
    await userEvent.click(
      lastOf(
        await screen.findAllByRole("button", { name: commitment.subject }),
      ),
    );

    // The next bar carries its own Done; the one this opens is in the dialog.
    expect(
      within(await screen.findByRole("dialog")).getByRole("button", {
        name: en["tasks.complete"],
      }),
    ).toBeTruthy();
  });
});
