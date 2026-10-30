/** @vitest-environment happy-dom */
import { act, cleanup, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  beginModelCall,
  endModelCall,
  modelCallsInFlight,
} from "../api/model-inflight";
import { en } from "../i18n/en";
import {
  block,
  jsonResponse,
  openPanel,
  panel,
  ROUTE,
  render,
  settlesOnLine,
  stubAgentRailApi,
} from "./agentrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  // The in-flight count is module state. It is reset here the way
  // clearAgentEdge resets its sibling signal. Otherwise a case failing between
  // begin and end would start every later case in this file at "working".
  while (modelCallsInFlight() > 0) {
    endModelCall();
  }
  // The frame case spies on Element.prototype.getBoundingClientRect, which every
  // later case in this file shares. Restored here rather than there: a spy left
  // on a prototype is the kind of leak whose failure names a case that is fine.
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("AgentRail", () => {
  // Work this tab did not start. The rail's other cases move the Core because
  // a query in this browser fetched. These cases cover the overnight runner,
  // reported through /me/ai-activity. They also cover two invariants that hold
  // alongside it. Operator vocabulary stays out of the reader's line. The
  // surface never invents a sentence for a kind it has no copy for.

  /** One run as the wire spells it: a JSON body, not a typed literal. Some of
   *  these cases send a kind that is off the contract. */
  const RUN = (over: Readonly<Record<string, unknown>> = {}) => ({
    id: "019f7e65-fbf7-7114-b114-40af4af63a01",
    kind: "morning_brief",
    state: "running",
    // Relative: the resting line ages a settled run out (agentrail-resting.ts).
    started_at: new Date(Date.now() - 60_000).toISOString(),
    ...over,
  });

  const withRuns = (...running: readonly unknown[]) =>
    stubAgentRailApi({
      agentActivity: () => jsonResponse({ running, recent: [], faults: [] }),
    });

  /** A run that has settled. The read puts these in `recent` and never in
   *  `running` (compose/agentactivity `recentSQL` vs `runningSQL`).
   *  `degrade_reason` and `summary` are written alongside the terminal status,
   *  so this is the only shape those two fields ever arrive in. */
  // Faults are derived from the settled rows here as the server derives them.
  // A run that failed a minute ago is in both arms, because both statements
  // about it are true. A stub that carried a failed run in `recent` alone
  // would be describing a server that does not exist.
  const faultsAmong = (rows: readonly unknown[]) =>
    rows.filter((row) => {
      const state = (row as { state?: string }).state;
      return state === "failed" || state === "degraded";
    });

  const withSettled = (...recent: readonly unknown[]) =>
    stubAgentRailApi({
      agentActivity: () =>
        jsonResponse({ running: [], recent, faults: faultsAmong(recent) }),
    });

  // The case this arm exists for: a fault that has already been pushed out of
  // `recent` by ten later successes, and is still unacknowledged.
  const withEvictedFault = (fault: unknown, settled: readonly unknown[]) =>
    stubAgentRailApi({
      agentActivity: () =>
        jsonResponse({ running: [], recent: settled, faults: [fault] }),
    });

  const BRIEF_RUNNING = "Preparing Morning brief…";
  const BRIEF_FAILED = "Morning brief failed";

  const runLines = () =>
    [...panel().querySelectorAll(".arrunline")].map((el) => el.textContent);

  // The work a contact asks for. The rail was silent on it until the router
  // could say `running`. Asserted at render and not against the map. The claim
  // is that a live summarize reaches the reader's own line, in their own
  // words, through the same feed the overnight run uses.
  //
  // Named, because that is the line that answers the reader. The server sends
  // the record the summary is about as `subject_label`. The rail says "about
  // Acme" rather than "about this company". That generic phrase was wrong for
  // a contact or a meeting, and told a rep nothing for a company.
  it("names the record a summary is about while it is still being written", async () => {
    withRuns(RUN({ kind: "summarize", subject_label: "Acme" }));
    const { container } = render(ROUTE);
    await settlesOnLine(container, "Summarizing Acme…");
  });

  // The name is the way to the record. A company's name goes to the company,
  // a contact's to the contact. It uses the same address every other link in
  // the app uses for that record. Following it leaves for the record and does
  // not open the panel.
  it.each([
    ["company", "Acme", "#/companies/"],
    ["contact", "Ana Roth", "#/contacts/"],
  ])(
    "links the %s a summary is about to its record",
    async (subjectType, name, page) => {
      const id = "019f7e65-fbf7-7114-b114-40af4af63a77";
      withRuns(
        RUN({
          kind: "summarize",
          subject_label: name,
          subject_type: subjectType,
          subject_id: id,
        }),
      );
      const user = userEvent.setup();
      const { container } = render(ROUTE);
      await settlesOnLine(container, `Summarizing ${name}…`);
      const link = container.querySelector(".arline a");
      expect(link?.textContent).toBe(name);
      expect(link?.getAttribute("href")).toBe(
        `/r/${page.slice(2)}${id}${page}${id}`,
      );
      if (!link) throw new Error("no record link in the line");
      await user.click(link);
      expect(document.querySelector(".arloose")).toBeNull();
    },
  );

  // A record with no page of its own keeps its name as a word. A meeting is
  // an activity, and the timeline has no screen for one. So does a kind this
  // build has never heard of, which an older tab gets from a newer server.
  // The sentence is still true, and it points nowhere rather than somewhere
  // wrong.
  it.each([
    ["a meeting", "activity"],
    ["a kind this build does not know", "constellation"],
  ])("names %s as text rather than a link", async (_what, subjectType) => {
    withRuns(
      RUN({
        kind: "summarize",
        subject_label: "the cutover review",
        subject_type: subjectType,
        subject_id: "019f7e65-fbf7-7114-b114-40af4af63a77",
      }),
    );
    const { container } = render(ROUTE);
    await settlesOnLine(container, "Summarizing the cutover review…");
    expect(container.querySelector(".arline a")).toBeNull();
  });

  // The panel restates the line and lists the live runs. The name is the way
  // to the record there too. A reader who opened the panel to see which
  // account is ready should not have to go back to the rail to reach it.
  it("links the record in the panel's head and in its list of live runs", async () => {
    const id = "019f7e65-fbf7-7114-b114-40af4af63a77";
    withRuns(
      RUN({
        kind: "account_scan",
        subject_label: "Brandt Automotive",
        subject_type: "company",
        subject_id: id,
      }),
    );
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await settlesOnLine(
      container,
      "Analyzing the history of Brandt Automotive…",
    );
    await openPanel(user, container);
    const links = [...panel().querySelectorAll("a")].filter(
      (a) => a.getAttribute("href") === `/r/companies/${id}#/companies/${id}`,
    );
    expect(links.map((a) => a.textContent)).toEqual([
      "Brandt Automotive",
      "Brandt Automotive",
    ]);
  });

  // An occurrence can arrive without a name, from an older server or a record
  // with no name to give. It falls back to a line that names no record,
  // rather than guessing at one.
  it("names no record for a summary that carried no name", async () => {
    withRuns(RUN({ kind: "summarize" }));
    const { container } = render(ROUTE);
    await settlesOnLine(container, "Writing summary…");
  });

  it("moves the Core to working when a server run is live and this tab is idle", async () => {
    withRuns(RUN());
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );
  });

  it("names the run in the rail line", async () => {
    withRuns(RUN());
    const { container } = render(ROUTE);
    await settlesOnLine(container, BRIEF_RUNNING);
  });

  // The window the feed cannot cover. The projection is written from an event
  // the router publishes. A gap opens between the press of "Draft with AI" and
  // the first read that carries the occurrence. In it, a live model call runs
  // under an orb at rest. For a task that runs in seconds, that was most of the run.
  // At the idle cadence it was the whole of it.
  it("moves the Core to working while this tab holds a model call open", async () => {
    withRuns();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );

    act(() => {
      beginModelCall();
    });
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );

    act(() => {
      endModelCall();
    });
  });

  // The feed is the only thing that may name the work. So the moment it
  // carries the occurrence, it outranks the bare fact that a request is open.
  // Both are true here at once, and the sentence has to be the run's own.
  it("lets the feed name the work as soon as it carries the run", async () => {
    withRuns(RUN());
    const { container } = render(ROUTE);
    act(() => {
      beginModelCall();
    });

    await settlesOnLine(container, BRIEF_RUNNING);

    act(() => {
      endModelCall();
    });
  });

  // One line under the orb. While this tab is fetching something it can name,
  // that sentence is the orb's line. The agent's own sentence has the slot
  // back once the read settles. Both are true at the same moment here: an
  // overnight brief is running, and this tab is fetching its own sources.
  // The reader sees one status at a time, never two stacked.
  it("gives the tool's named read the orb's one line, then hands it back", async () => {
    vi.useFakeTimers();
    stubAgentRailApi({
      agentActivity: () =>
        jsonResponse({ running: [RUN()], recent: [], faults: [] }),
    });
    const { container } = render(ROUTE);
    // Inside the ticker's `LINGER_MS`, so the mount-time reads it named are
    // still standing while the agent's own line has answered. Which of them is
    // newest is a race between stubs and no part of the claim. The claim: the
    // tool's sentence holds the one slot, and nothing sits under it.
    await act(() => vi.advanceTimersByTimeAsync(300));
    const inFlight = container.querySelector(".arline")?.textContent;
    expect(inFlight).toBeTruthy();
    expect(inFlight).not.toBe(BRIEF_RUNNING);
    expect(container.querySelectorAll(".arwords > span").length).toBe(1);
    // Past every linger the mount reads can hold, the agent's line is back.
    await act(() => vi.advanceTimersByTimeAsync(2500));
    expect(container.querySelector(".arline")?.textContent).toBe(BRIEF_RUNNING);
  });

  // A fault keeps the line while reads are in flight. The colour and the
  // caption are about the same thing. An amber orb captioned with a read would
  // tell a reader the read is the fault. Proven by the caption standing still
  // across the window the case above shows the tool taking.
  it("never lets a read caption a fault", async () => {
    vi.useFakeTimers();
    withRuns(RUN({ state: "stalled" }));
    const { container } = render(ROUTE);
    await act(() => vi.advanceTimersByTimeAsync(300));
    expect(block(container).getAttribute("data-core-state")).toBe("warning");
    const inFlight = container.querySelector(".arline")?.textContent;
    await act(() => vi.advanceTimersByTimeAsync(2500));
    expect(container.querySelector(".arline")?.textContent).toBe(inFlight);
  });

  // The colour and the sentence are always about the same thing. A run that
  // failed unread and a second one still in flight are true at once. The
  // failure outranks the live run, so the live run's sentence must not
  // caption it. A red orb captioned "I'm writing your morning brief" tells a
  // reader the brief is the fault.
  it("never captions a state with a run that did not cause it", async () => {
    window.localStorage.removeItem("margince.agent.faults-seen");
    const failed = RUN({
      id: "019f7e65-fbf7-7114-b114-40af4af63c01",
      state: "failed",
    });
    stubAgentRailApi({
      agentActivity: () =>
        jsonResponse({ running: [RUN()], recent: [failed], faults: [failed] }),
    });
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    expect(container.querySelector(".arline")?.textContent).toBe(BRIEF_FAILED);
  });

  // Opening the panel acknowledges every fault the read reported, at once.
  // Clearing only the fault the orb happened to show would leave the colour up
  // after the reader had already turned to the report.
  it("clears every fault the panel delivered, not one per open", async () => {
    window.localStorage.removeItem("margince.agent.faults-seen");
    withSettled(
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63c01", state: "failed" }),
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63c02", state: "degraded" }),
    );
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await openPanel(user, container);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  // The other half of the live vocabulary. It had no producer at all until the
  // lane came off the kind of work. Evidence arriving is `ingest`. Reasoning
  // over evidence already held is `working` (ai-activity-orb.ts).
  it("moves the Core to ingest when the live run is evidence arriving", async () => {
    withRuns(RUN({ kind: "document_extract" }));
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("ingest"),
    );
  });

  // The crawl a contact starts from a company page. The dossier row itself
  // reports it, not the settled model calls it makes. That lets the orb hold
  // `ingest` for the whole read instead of resting between calls. The line
  // names the company, because the source sent its name.
  it("moves the Core to ingest while a company website is being read, and names the company", async () => {
    withRuns(RUN({ kind: "site_read", subject_label: "Acme" }));
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("ingest"),
    );
    await settlesOnLine(container, "Reading the Acme website…");
  });

  // A run past the lease its own source declared. The server derives it, so a
  // worker that died without saying so cannot go on being displayed as busy.
  // Amber is right for it: the work may yet land, and there is nothing for
  // the reader to do but know.
  it("moves the Core to warning for a run past its own lease", async () => {
    withRuns(RUN({ state: "stalled" }));
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("warning"),
    );
  });

  // The overnight case is the whole reason a fault is acknowledged rather than
  // decayed. A run fails at four in the morning and the contact it ran for is
  // asleep. Whatever the orb does at 04:12 is seen by nobody. It holds
  // until the panel has been opened.
  it("holds a failed run on the Core until the panel has been opened", async () => {
    // Acknowledgement is remembered per browser, so the case owns its own id
    // and its own storage. Sharing either would make this test's verdict
    // depend on which of its neighbours ran first.
    window.localStorage.removeItem("margince.agent.faults-seen");
    withSettled(
      RUN({ id: "019f7e65-fbf7-7114-b114-40af4af63b02", state: "failed" }),
    );
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    await openPanel(user, container);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  // The defect this arm exists for. The overnight run fails at four. By the
  // time its owner looks, ten later runs have settled. So it is gone from
  // `recent`, which keeps the newest ten of any outcome. Read from there, the
  // orb released a fault nobody had ever seen. The product knew its overnight
  // work failed and stopped saying so.
  it("holds a failed run the day's later successes have pushed out of recent", async () => {
    window.localStorage.removeItem("margince.agent.faults-seen");
    const overnight = RUN({
      id: "019f7e65-fbf7-7114-b114-40af4af63b03",
      state: "failed",
    });
    // Ten settled runs, none of them the fault: the shape `recent` has by
    // mid-morning on any working day.
    const later = Array.from({ length: 10 }, (_, i) =>
      RUN({ id: `019f7e65-fbf7-7114-b114-40af4af640${10 + i}`, state: "done" }),
    );
    withEvictedFault(overnight, later);
    const user = userEvent.setup();
    const { container } = render(ROUTE);

    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("error"),
    );
    // And it still clears the ordinary way, so the arm changed where the fault
    // is read from and nothing about how it is answered.
    await openPanel(user, container);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("idle"),
    );
  });

  // Above the counts, because a run happening right now outranks a queue that
  // has been waiting since yesterday.
  it("lists running work in the panel under its own heading", async () => {
    withRuns(RUN());
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await openPanel(user, container);
    await waitFor(() => expect(runLines()).toEqual([BRIEF_RUNNING]));
    const headings = [...panel().querySelectorAll(".arsect h3")].map(
      (el) => el.textContent,
    );
    expect(headings[0]).toBe("Running now");
    expect(headings).toContain(en["agent.panel.needsYou"]);
  });

  // `degrade_reason` is server-authored operator vocabulary and untranslated.
  // The reader's surfaces carry the localized line and nothing else, so the
  // raw token reaches no surface at all.
  it("keeps the degrade reason out of the line and out of the panel", async () => {
    const reason = "brief_partial: crm_read_timeout";
    const stopped = "Morning brief stopped partway";
    withSettled(RUN({ state: "degraded", degrade_reason: reason }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await settlesOnLine(container, stopped);
    expect(container.querySelector(".arline")?.textContent).not.toContain(
      reason,
    );
    await openPanel(user, container);
    expect(panel().textContent).not.toContain(reason);
  });

  // A kind the copy map has never heard of produces nothing: no fallback
  // sentence, no raw message key, no de-underscored token. The run is still
  // real, so the orb still moves. Only the words are missing.
  it("renders no line for a kind it has no copy for", async () => {
    withRuns(RUN({ kind: "telepathic_prospecting" }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await waitFor(() =>
      expect(block(container).getAttribute("data-core-state")).toBe("working"),
    );
    await openPanel(user, container);
    expect(runLines()).toEqual([]);
    expect(panel().textContent).not.toContain("Running now");
    expect(panel().textContent).not.toContain("agent.activity");
    expect(panel().textContent).not.toContain("telepathic_prospecting");
  });

  // A settled run reaches the reader twice: the resting rotation says what the
  // agent has just got done, the recap lists the day's. It reaches the running
  // section in neither case. The two bounds differ. agentrail.resting.test.tsx
  // holds the rotation's.
  it("reads a run that finished today in the resting line and in the recap, never as live work", async () => {
    withSettled(RUN({ state: "done" }));
    const user = userEvent.setup();
    const { container } = render(ROUTE);
    await settlesOnLine(container, "Morning brief ready");
    expect(block(container).getAttribute("data-core-state")).toBe("idle");
    await openPanel(user, container);
    expect(runLines()).toEqual([]);
    expect(panel().textContent).not.toContain("Running now");
    expect(
      panel().querySelector(".aritem:not(.arempty)")?.textContent,
    ).toContain("Morning brief ready");
  });
});
