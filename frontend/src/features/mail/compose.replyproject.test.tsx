/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  jsonResponse,
  pickWhy,
  render,
  stubRoutes,
  WHY_LABEL,
} from "./compose.testkit";

// The project a reply files under, and the key it carries in the subject.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the project a reply files under", () => {
  // The composer's project control, on a reply. A reply inherits its thread's
  // project on its own (capture's stickiness rung). The composer suggests that
  // project so the rep does not retype it. It is a suggestion in a picker they
  // can set to None, not a fact stated at them.
  const replyBackend = (
    links: { entity_type: string; entity_id: string }[],
    projects: { project_id: string; name: string; key: string }[],
    routes: Record<string, () => Response | Promise<Response>> = {},
  ) =>
    stubRoutes({
      "GET /activities/act-1": () =>
        jsonResponse({
          id: "act-1",
          kind: "email",
          occurred_at: "2026-08-09T09:00:00Z",
          source: "gmail",
          created_at: "2026-08-09T09:00:00Z",
          updated_at: "2026-08-09T09:00:00Z",
          version: 1,
          links,
        }),
      "GET /deals/d-1": () =>
        jsonResponse({
          id: "d-1",
          name: "netcare",
          company_id: "o-1",
          project_id: "pr-9",
        }),
      "GET /companies/o-1/360": () =>
        jsonResponse({
          as_of: "2026-08-09T09:00:00Z",
          company: {
            id: "o-1",
            display_name: "netcare",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          contacts: { data: [], page: { next_cursor: null } },
          deals: { data: [], page: { next_cursor: null } },
          projects: projects.map((one) => ({
            ...one,
            phase: "delivering",
          })),
        }),
      "GET /projects/pr-9": () =>
        jsonResponse({ id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }),
      "GET /projects/pr-1": () =>
        jsonResponse({ id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" }),
      ...routes,
    });

  const openReply = async () => {
    render(
      <ComposeModal
        activityId="act-1"
        entityType="deal"
        entityId="d-1"
        open
        onClose={vi.fn()}
      />,
    );
    return () => screen.getByLabelText("Subject") as HTMLInputElement;
  };

  it("offers the company's projects and defaults to the thread's own", async () => {
    replyBackend(
      [
        { entity_type: "deal", entity_id: "d-1" },
        { entity_type: "project", entity_id: "pr-1" },
      ],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();

    const picker = await screen.findByLabelText("Project");
    // The thread's own project is the default, not the deal's: a conversation
    // already filed settled this.
    await waitFor(() =>
      expect(picker.textContent).toContain("ERP rollout Acme"),
    );
    // Both of the company's projects are on offer, plus None.
    await userEvent.setup().click(picker);
    expect(
      (await screen.findAllByRole("option")).map((o) => o.textContent),
    ).toEqual([
      "No project",
      "ERP-27 · ERP rollout Acme",
      "N2P-1 · Netcare 2 project",
    ]);
    await userEvent.setup().keyboard("{Escape}");
    // And the tag is already in the subject. Nothing explains it, because the
    // field shows it.
    await waitFor(() => expect(subject().value).toBe("[ERP-27]"));
  });

  it("defaults to the deal's project when the thread names none", async () => {
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();

    const picker = await screen.findByLabelText("Project");
    await waitFor(() =>
      expect(picker.textContent).toContain("Netcare 2 project"),
    );
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
  });

  it("takes the tag out when the rep picks None, and puts another one in", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();
    const picker = await screen.findByLabelText("Project");
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
    // `[` opens a key descriptor in user-event's parser, so the tag is typed
    // through fireEvent rather than escaped into unreadability.
    await user.clear(subject());
    fireEvent.change(subject(), {
      target: { value: "[N2P-1] Re: Angebot" },
    });

    await pickOption(user, picker, "No project");
    await waitFor(() => expect(subject().value).toBe("Re: Angebot"));

    // Choosing a different project stamps that one instead.
    await pickOption(user, picker, "ERP-27 · ERP rollout Acme");
    await waitFor(() => expect(subject().value).toBe("[ERP-27] Re: Angebot"));
  });

  it("offers nothing when the company reaches no live project", async () => {
    replyBackend([{ entity_type: "deal", entity_id: "d-1" }], []);
    const subject = await openReply();

    await draftButton();
    // A list whose only entry is None asks a question with one answer.
    expect(screen.getByLabelText("Project").hasAttribute("disabled")).toBe(
      true,
    );
    expect(subject().value).toBe("");
  });

  it("keeps the tag when the rep types a subject over it", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    // The ordinary thing a rep does next is type the subject. The field is
    // replaced wholesale, and the tag has to survive that. Writing it only when
    // the picker moves would lose it.
    await user.clear(subject());
    await user.type(subject(), "Re: Kurzer Austausch?");

    await waitFor(() =>
      expect(subject().value).toBe("[N2P-1] Re: Kurzer Austausch?"),
    );
  });

  it("puts the tag back when the subject loses it, while a project is chosen", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    // Deleting the tag out of the text does not unset a project. The picker
    // does, and it still names one. Leaving the text without it would put
    // the field and the picker in two different states, and the send would
    // honour the picker.
    await user.clear(subject());
    await user.type(subject(), "Re: ohne Tag");

    await waitFor(() => expect(subject().value).toBe("[N2P-1] Re: ohne Tag"));

    // The way to have no tag is to choose No project.
    await pickOption(user, screen.getByLabelText("Project"), "No project");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));
  });

  // The key reaches the client's inbox, so whether it goes is a switch the rep
  // sees beside the subject. It is on by default, because it files the answer.
  const KEY_SWITCH = "Add project key to subject (helps filing)";
  const savedReply = (subject: string) => ({
    id: "md-1",
    anchor_type: "activity",
    anchor_id: "act-1",
    to: ["kunde@netcare.test"],
    cc: [],
    bcc: [],
    subject,
    body: "Hallo",
    html_body: "<p>Hallo</p>",
    version: 1,
    agent_drafted: false,
    created_at: "2026-08-09T09:00:00Z",
    updated_at: "2026-08-09T09:05:00Z",
  });

  it("shows the key switch on while a project is chosen, and none without one", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();

    const key = await screen.findByRole("switch", { name: KEY_SWITCH });
    expect(key.getAttribute("aria-checked")).toBe("true");
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    await pickOption(user, screen.getByLabelText("Project"), "No project");
    await waitFor(() =>
      expect(screen.queryByRole("switch", { name: KEY_SWITCH })).toBeNull(),
    );
  });

  it("keeps the tag out once turned off, through a draft and a retype", async () => {
    const user = userEvent.setup();
    const sent = replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
      {
        "POST /activities/act-1/draft-email": () =>
          jsonResponse({ subject: "[N2P-1] Re: Angebot", body: "Hallo" }),
        "PUT /mail-drafts": () => jsonResponse(savedReply("Re: ohne Tag"), 200),
        "POST /activities/act-1/send-email": () => jsonResponse({}, 202),
      },
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    await user.click(await screen.findByRole("switch", { name: KEY_SWITCH }));
    await waitFor(() => expect(subject().value).toBe(""));

    await user.click(draftButton());
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-1/draft-email"),
      ).toBe(true),
    );
    await waitFor(() => expect(subject().value).toBe("Re: Angebot"));

    await user.clear(subject());
    await user.type(subject(), "Re: ohne Tag");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));
    expect(
      screen
        .getByRole("switch", { name: KEY_SWITCH })
        .getAttribute("aria-checked"),
    ).toBe("false");

    await user.type(screen.getByLabelText("To"), "kunde@netcare.test");
    await user.tab();
    await pickWhy(WHY_LABEL.requestedFollowup);
    await user.click(screen.getByRole("button", { name: /^Send$/ }));
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "POST /activities/act-1/send-email")?.body,
      ).toMatchObject({ subject: "Re: ohne Tag" }),
    );
  });

  // A restored subject is read against this project's own key. Another
  // project's key is the rep's text, so it stays and the switch is off.
  it.each([
    ["Re: Angebot", "false"],
    ["[N2P-1] Re: Angebot", "true"],
    ["[RP-3] Re: Angebot", "false"],
  ])(
    "takes the switch from a restored draft's subject %s",
    async (saved, on) => {
      replyBackend(
        [{ entity_type: "deal", entity_id: "d-1" }],
        [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
        { "GET /mail-drafts": () => jsonResponse(savedReply(saved)) },
      );
      const subject = await openReply();
      await screen.findByText("Saved draft restored");

      await waitFor(() =>
        expect(
          screen
            .getByRole("switch", { name: KEY_SWITCH })
            .getAttribute("aria-checked"),
        ).toBe(on),
      );
      expect(subject().value).toBe(saved);
    },
  );

  // A pick after a restore keeps what the restore read. A key that was on
  // follows the new project or leaves with None; one that was off stays off.
  it.each([
    ["[N2P-1] Re: Angebot", "No project", "Re: Angebot", null],
    [
      "[N2P-1] Re: Angebot",
      "ERP-27 · ERP rollout Acme",
      "[ERP-27] Re: Angebot",
      "true",
    ],
    ["Re: Angebot", "ERP-27 · ERP rollout Acme", "Re: Angebot", "false"],
  ])(
    "restored %s, then %s picked, leaves %s",
    async (saved, pick, after, on) => {
      const user = userEvent.setup();
      replyBackend(
        [{ entity_type: "deal", entity_id: "d-1" }],
        [
          { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
          { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
        ],
        { "GET /mail-drafts": () => jsonResponse(savedReply(saved)) },
      );
      const subject = await openReply();
      await screen.findByText("Saved draft restored");
      await screen.findByRole("switch", { name: KEY_SWITCH });

      await pickOption(user, screen.getByLabelText("Project"), pick);

      await waitFor(() => expect(subject().value).toBe(after));
      expect(
        screen
          .queryByRole("switch", { name: KEY_SWITCH })
          ?.getAttribute("aria-checked") ?? null,
      ).toBe(on);
    },
  );

  // A pick before the restored project's record arrives is read against that
  // project's key once it does, and the subject waits for it.
  it.each([
    ["[N2P-1] Re: Angebot", "No project", "Re: Angebot", null],
    [
      "[N2P-1] Re: Angebot",
      "ERP-27 · ERP rollout Acme",
      "[ERP-27] Re: Angebot",
      "true",
    ],
    ["Re: Angebot", "ERP-27 · ERP rollout Acme", "Re: Angebot", "false"],
  ])(
    "restored %s, then %s picked before its project loads, leaves %s",
    async (saved, pick, after, on) => {
      const user = userEvent.setup();
      let release: (() => void) | undefined;
      const held = new Promise<void>((resolve) => {
        release = resolve;
      });
      replyBackend(
        [{ entity_type: "deal", entity_id: "d-1" }],
        [
          { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
          { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
        ],
        {
          "GET /mail-drafts": () => jsonResponse(savedReply(saved)),
          "GET /projects/pr-9": async () => {
            await held;
            return jsonResponse({
              id: "pr-9",
              name: "Netcare 2 project",
              key: "N2P-1",
            });
          },
        },
      );
      const subject = await openReply();
      await screen.findByText("Saved draft restored");
      await pickOption(user, screen.getByLabelText("Project"), pick);
      expect(subject().value).toBe(saved);

      release?.();
      await waitFor(() => expect(subject().value).toBe(after));
      await waitFor(() =>
        expect(
          screen
            .queryByRole("switch", { name: KEY_SWITCH })
            ?.getAttribute("aria-checked") ?? null,
        ).toBe(on),
      );
    },
  );

  // Each step strips the key last applied, so no pick after a restore leaves an
  // earlier project's key behind.
  const ERP = "ERP-27 · ERP rollout Acme";
  const RP = "RP-3 · Reporting";
  const NONE = "No project";
  const N2P = "N2P-1 · Netcare 2 project";
  const SWITCH_STEP = "turn the switch";
  it.each([
    {
      path: "ERP-27, None",
      saved: "[N2P-1] Re: Angebot",
      steps: [
        [ERP, "[ERP-27] Re: Angebot"],
        [NONE, "Re: Angebot"],
      ],
      on: null,
    },
    {
      path: "ERP-27, RP-3, None",
      saved: "[N2P-1] Re: Angebot",
      steps: [
        [ERP, "[ERP-27] Re: Angebot"],
        [RP, "[RP-3] Re: Angebot"],
        [NONE, "Re: Angebot"],
      ],
      on: null,
    },
    {
      path: "ERP-27, None",
      saved: "Re: Angebot",
      steps: [
        [ERP, "Re: Angebot"],
        [NONE, "Re: Angebot"],
      ],
      on: null,
    },
    {
      path: "None, N2P-1",
      saved: "[N2P-1] Re: Angebot",
      steps: [
        [NONE, "Re: Angebot"],
        [N2P, "[N2P-1] Re: Angebot"],
      ],
      on: "true",
    },
    {
      path: "switch off, ERP-27",
      saved: "[N2P-1] Re: Angebot",
      steps: [
        [SWITCH_STEP, "Re: Angebot"],
        [ERP, "Re: Angebot"],
      ],
      on: "false",
    },
  ])("restored $saved, then $path", async ({ saved, steps, on }) => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-5", name: "Reporting", key: "RP-3" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
      {
        "GET /mail-drafts": () => jsonResponse(savedReply(saved)),
        "GET /projects/pr-5": () =>
          jsonResponse({ id: "pr-5", name: "Reporting", key: "RP-3" }),
      },
    );
    const subject = await openReply();
    await screen.findByText("Saved draft restored");
    await screen.findByRole("switch", { name: KEY_SWITCH });

    for (const [step, after] of steps) {
      if (step === SWITCH_STEP) {
        await user.click(screen.getByRole("switch", { name: KEY_SWITCH }));
      } else {
        await pickOption(user, screen.getByLabelText("Project"), step);
      }
      await waitFor(() => expect(subject().value).toBe(after));
    }
    await waitFor(() =>
      expect(
        screen
          .queryByRole("switch", { name: KEY_SWITCH })
          ?.getAttribute("aria-checked") ?? null,
      ).toBe(on),
    );
  });

  it("waits for the project before reading a restored subject", async () => {
    let release: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
      {
        "GET /mail-drafts": () =>
          jsonResponse(savedReply("[N2P-1] Re: Angebot")),
        "GET /projects/pr-9": async () => {
          await held;
          return jsonResponse({
            id: "pr-9",
            name: "Netcare 2 project",
            key: "N2P-1",
          });
        },
      },
    );
    const subject = await openReply();
    await screen.findByText("Saved draft restored");
    expect(subject().value).toBe("[N2P-1] Re: Angebot");

    release?.();
    await waitFor(() =>
      expect(
        screen
          .getByRole("switch", { name: KEY_SWITCH })
          .getAttribute("aria-checked"),
      ).toBe("true"),
    );
    expect(subject().value).toBe("[N2P-1] Re: Angebot");
  });

  it("puts the tag back when the switch is turned on again", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
    await user.click(await screen.findByRole("switch", { name: KEY_SWITCH }));
    await user.type(subject(), "Re: ohne Tag");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));

    // Turned on again, the tag comes back in front of what the rep wrote.
    await user.click(screen.getByRole("switch", { name: KEY_SWITCH }));
    await waitFor(() => expect(subject().value).toBe("[N2P-1] Re: ohne Tag"));
  });
});
