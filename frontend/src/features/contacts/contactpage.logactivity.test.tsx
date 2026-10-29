/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { CallLogHost } from "../../components/calllog.host";
import {
  type Contact360,
  mount,
  quietMoment,
  recordHeader,
  view,
} from "../../testing/contactpage.testkit";
import { jsonResponse } from "../../testing/story-utils";

afterEach(() => {
  cleanup();
});

// The one thing a CRM must let a rep do on a contact is write down what
// happened. Two ways in, one form. The header verb is always there, and the
// moment card's action shows when its rung picks logging as the next step.
describe("logging an activity", () => {
  // The "nothing needed" rung, whose one action is to log an interaction.
  const quietDayMoment: Contact360["moment"] = {
    claim_key: "moment:nothing_needed",
    evidence_fingerprint: "quiet",
    rule: "nothing_needed",
    headline: "Nothing needs you today",
    why_now: "No meeting coming up, nothing owed, nobody waiting on a reply.",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log an interaction",
      state: "available",
      destination: { surface: "activity_log" },
    },
  };

  it("opens the log form from the header, whatever the moment card offers", async () => {
    const user = userEvent.setup();
    mount("overview");

    // Disabled until the caller's grants have arrived, and the header is
    // rebuilt when they do. So the button is looked up fresh each time,
    // never held from before the grant landed.
    const logButton = async () =>
      within(await recordHeader()).getByRole("button", {
        name: "Log activity",
      });
    await waitFor(async () =>
      expect(await logButton()).toHaveProperty("disabled", false),
    );
    await user.click(await logButton());

    expect(
      await screen.findByRole("dialog", { name: "Log activity" }),
    ).toBeTruthy();
  });

  it("keeps the verb on the page but refuses it without the create grant", async () => {
    // The store refuses a save without `activity.create`, so the button must
    // not open a form it cannot complete. It stays visible: a reader who cannot
    // tell "not mine to do" from "no such button" learns nothing from an absence.
    mount("overview", view, [], {}, { contact: ["read", "update"] });

    const header = await recordHeader();
    const button = within(header).getByRole("button", {
      name: /Log activity/,
    });
    expect(button).toHaveProperty("disabled", true);
    // The sentence is the left card's footnote, and the verb points at it.
    const reason = document.getElementById(
      button.getAttribute("aria-describedby") ?? "",
    );
    expect(reason?.textContent).toBe(
      "You do not have permission to log activities on this record.",
    );
    expect(reason?.closest(".record-aside")).not.toBeNull();
  });

  // "Add task" issues the same POST /activities as "Log activity", so the
  // same grant refuses it. The two share the one sentence instead of
  // printing it twice.
  it("also refuses Add task without the create grant, over the same sentence", async () => {
    mount("overview", view, [], {}, { contact: ["read", "update"] });

    const header = await recordHeader();
    const addTask = within(header).getByRole("button", { name: "Add task" });
    const logActivity = within(header).getByRole("button", {
      name: /Log activity/,
    });
    expect(addTask).toHaveProperty("disabled", true);
    expect(
      screen.getAllByText(
        "You do not have permission to log activities on this record.",
      ),
    ).toHaveLength(1);
    expect(addTask.getAttribute("aria-describedby")).toBe(
      logActivity.getAttribute("aria-describedby"),
    );
  });

  // A guard that has not answered yet refuses nothing. While /me is in
  // flight, useCanWrite reads false as it would for a caller without the
  // grant. Without this, both buttons would flash the "You do not have
  // permission" sentence on every page load, before the verdict is in.
  it("stays quiet — disabled, no claimed refusal — while the grant is still in flight", async () => {
    mount("overview", view, [], {
      "GET /me": () => new Promise<Response>(() => {}),
    });

    const header = await recordHeader();
    const addTask = within(header).getByRole("button", { name: "Add task" });
    const logActivity = within(header).getByRole("button", {
      name: /Log activity/,
    });
    expect(addTask).toHaveProperty("disabled", true);
    expect(logActivity).toHaveProperty("disabled", true);
    expect(
      within(header).queryByText(
        "You do not have permission to log activities on this record.",
      ),
    ).toBeNull();
  });

  // The button names a verb and opens the same form Log activity does,
  // started on the task kind. It files the task against this contact instead
  // of navigating to the Worklist, which has no way to add one.
  it("adds a task on this contact from the header, instead of leaving for the Worklist", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    mount("overview", view, [], {
      "POST /activities": (body) => {
        posted.push(body);
        return jsonResponse({}, 201);
      },
    });

    const header = await recordHeader();
    await user.click(within(header).getByRole("button", { name: "Add task" }));
    const dialog = await screen.findByRole("dialog", { name: "Add task" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Send the renewal quote",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add task" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "task",
      subject: "Send the renewal quote",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });

  // A quiet rung already says so in How it stands, so the moment's own
  // sentence under it would be the same news twice. Neither an agent byline nor a
  // verb a reader can press belongs on a record where nothing is asked.
  it("prints no suggestion card, byline, readiness word, or coverage sentence for a quiet record", async () => {
    mount("overview", { ...view, moment: quietDayMoment });

    await recordHeader();
    expect(screen.queryByText(quietDayMoment.why_now)).toBeNull();
    expect(screen.queryByText("Margince suggests")).toBeNull();
    expect(screen.queryByText("Ready")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Log an interaction" }),
    ).toBeNull();
  });

  // This contrast shows the rule above is not a blanket deletion: a rung
  // that asks for something draws its verb in the Next bar. The
  // ladder is a rule, so the bar wears no agent byline.
  it("puts the verb of a rung that does ask for something in the Next bar", async () => {
    mount("overview", { ...view, moment: quietMoment });

    const move = (await screen.findByText("Dana has gone quiet")).closest(
      ".next-bar-row",
    );
    if (!(move instanceof HTMLElement)) {
      throw new Error("the moment is not drawn as the Next bar");
    }
    expect(
      within(move).getByRole("button", { name: /Draft a follow-up/ }),
    ).toBeTruthy();
    expect(within(move).queryByText("Margince suggests")).toBeNull();
  });

  it("logs a meeting on a quiet contact from the header", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    mount("overview", { ...view, moment: quietDayMoment }, [], {
      "POST /activities": (body) => {
        posted.push(body);
        return jsonResponse({}, 201);
      },
    });

    await user.click(
      within(await recordHeader()).getByRole("button", {
        name: /Log activity/,
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.click(within(dialog).getByRole("combobox", { name: "Type" }));
    await user.click(await screen.findByRole("option", { name: "Meeting" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Coffee at the fair",
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "meeting",
      subject: "Coffee at the fair",
      meeting_status: "held",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });

  // The `task` surface ContactMomentDestination documents ("the task sheet")
  // must not fall through runAction's default and do nothing. That is a
  // capability the API serves and no screen offers, as with the header's Add task.
  const taskMoment: Contact360["moment"] = {
    claim_key: "moment:overdue_promise",
    evidence_fingerprint: "fp-task",
    rule: "overdue_promise",
    headline: "Something is due",
    why_now: "The next step needs a task to track it.",
    confidence: "observed_fact",
    evidence: [],
    recommended_action: {
      kind: "complete_task",
      label: "File a task",
      state: "available",
      destination: { surface: "task" },
    },
  };

  it("adds a task from the moment card's own 'task' surface", async () => {
    const user = userEvent.setup();
    const posted: unknown[] = [];
    mount("overview", { ...view, moment: taskMoment }, [], {
      "POST /activities": (body) => {
        posted.push(body);
        return jsonResponse({}, 201);
      },
    });

    await user.click(
      await screen.findByRole("button", { name: "File a task" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Add task" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Chase the signed order",
    );
    await user.click(within(dialog).getByRole("button", { name: "Add task" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "task",
      subject: "Chase the signed order",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });
});

// Every number on the page dials and opens the page's own Log activity drawer
// on Call. The shell's host is mounted too, so a second form would show.
describe("logging a call started on the page", () => {
  const NUMBER = "+49 30 1234 5678";
  const withPhone: Contact360 = {
    ...view,
    contact: {
      ...view.contact,
      phones: [
        {
          id: "ph-1",
          phone: NUMBER,
          phone_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "human:u-1",
        },
      ],
    },
  };
  const inShell = (page: ReactNode) => <CallLogHost>{page}</CallLogHost>;

  function mountWithCalls(page: Contact360 = withPhone, extra = {}) {
    const posted: unknown[] = [];
    mount(
      "overview",
      page,
      [],
      {
        "POST /activities": (body) => {
          posted.push(body);
          return jsonResponse({}, 201);
        },
        ...extra,
      },
      undefined,
      inShell,
    );
    return posted;
  }

  // Enabled once the grant has answered, which is when a press may open a form.
  async function readyToLog() {
    await waitFor(async () =>
      expect(
        within(await recordHeader()).getByRole("button", {
          name: /Log activity/,
        }),
      ).toHaveProperty("disabled", false),
    );
  }

  it("opens one drawer on Call from the number, and files the call on this contact", async () => {
    const user = userEvent.setup();
    const posted = mountWithCalls();
    await readyToLog();

    const [number] = await screen.findAllByRole("link", { name: NUMBER });
    expect(fireEvent.click(number)).toBe(true);
    const dialog = screen.getByRole("dialog", { name: "Log activity" });
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(
      within(dialog).getByRole("combobox", { name: "Type" }).textContent,
    ).toContain("Call");
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "Agreed the depot dates",
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "call",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });

  it("opens the same drawer from the Call icon beside the number", async () => {
    mountWithCalls();
    await readyToLog();

    const details = document.querySelector<HTMLElement>(".record-aside");
    if (!details) throw new Error("the record drew no details beside the feed");
    const call = within(details).getByRole("link", { name: "Call" });
    expect(fireEvent.click(call)).toBe(true);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(
      within(dialog).getByRole("combobox", { name: "Type" }).textContent,
    ).toContain("Call");
  });

  it("only dials for a reader without the create grant", async () => {
    mount(
      "overview",
      withPhone,
      [],
      {},
      { contact: ["read", "update"] },
      inShell,
    );
    await screen.findByText(
      "You do not have permission to log activities on this record.",
    );

    const [number] = await screen.findAllByRole("link", { name: NUMBER });
    expect(fireEvent.click(number)).toBe(true);
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
