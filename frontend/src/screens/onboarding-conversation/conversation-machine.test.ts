import { describe, expect, it } from "vitest";
import {
  type ConversationEvent,
  type ConversationQuestion,
  type ConversationState,
  conversationReducer,
  initialConversationState,
  THREAD_CAP,
  type ThreadEntry,
} from "./conversation-machine";
import { entityQuestion, run, speakerQuestion } from "./test-fixtures";

// The reducer is the whole conversation: this suite walks the transition
// table end to end and pins the welcome gate, restore routing, member path,
// the compile-time XOR contracts, and the thread cap. Run-correlation and
// build-retry semantics live in conversation-correlation.test.ts.

describe("conversationReducer happy path", () => {
  it("walks the creator journey across all seven acts", () => {
    let state = run([{ type: "START", memberPath: false }]);
    expect(state).toMatchObject({ act: "company", phase: "co.intro" });

    state = run(
      [
        { type: "URL_SUBMITTED", url: "https://acme.example" },
        { type: "READ_STARTED", readId: "r1" },
        {
          type: "NARRATION",
          readId: "r1",
          entry: {
            kind: "narration",
            id: "pages:3",
            i18nKey: "ob.conv.read.pages",
            params: { pages: "3" },
          },
        },
      ],
      state,
    );
    expect(state.phase).toBe("co.reading");
    expect(state.activeReadId).toBe("r1");
    expect(state.thread.map((entry) => entry.kind)).toEqual([
      "user",
      "narration",
    ]);

    state = run(
      [{ type: "CLARIFY", readId: "r1", question: entityQuestion }],
      state,
    );
    expect(state.phase).toBe("co.clarify");
    expect(state.pendingQuestion?.id).toBe("clarify-entity");

    state = run(
      [
        {
          type: "QUESTION_ANSWERED",
          questionId: "clarify-entity",
          value: "acme-gmbh",
        },
      ],
      state,
    );
    expect(state.phase).toBe("co.reading");
    expect(state.pendingQuestion).toBeNull();
    expect(state.thread.at(-1)).toMatchObject({
      kind: "user",
      text: "Acme GmbH",
    });

    state = run(
      [
        {
          type: "READ_TERMINAL",
          readId: "r1",
          status: "ready",
        },
        { type: "REVIEW_READY" },
        { type: "COMPANY_CONFIRMED" },
      ],
      state,
    );
    // The installation's reporting basis comes right after the company,
    // before anything about the person answering.
    expect(state).toMatchObject({ act: "basis", phase: "bs.ask" });
    // The read terminal appends nothing: it is silent success, and the
    // outcome right after COMPANY_CONFIRMED proves no bubble sits between
    // them.
    expect(state.thread.at(-1)).toMatchObject({
      kind: "outcome",
      i18nKey: "ob.conv.company.confirmed",
      tone: "success",
    });

    // Only then is a creator asked whether they will work in Margince
    // themselves, before either personal act opens.
    state = run([{ type: "BASIS_DONE" }], state);
    expect(state).toMatchObject({ act: "invite", phase: "in.ask" });

    state = run([{ type: "INVITE_ACCEPTED" }], state);
    expect(state).toMatchObject({ act: "voice", phase: "vo.collecting" });

    state = run(
      [
        { type: "UPLOAD_ADDED", id: "u1", name: "call.vtt" },
        { type: "SPEAKER_NEEDED", question: speakerQuestion },
        {
          type: "QUESTION_ANSWERED",
          questionId: "speaker",
          value: "Speaker 1",
        },
        { type: "BUILD_STARTED", buildId: "b1" },
        { type: "BUILD_STAGE", buildId: "b1", stage: "snapshot" },
        { type: "BUILD_STAGE", buildId: "b1", stage: "extract" },
        { type: "BUILD_STAGE", buildId: "b1", stage: "evaluate" },
        { type: "BUILD_STAGE", buildId: "b1", stage: "activate" },
        { type: "BUILD_TERMINAL", buildId: "b1", status: "succeeded" },
      ],
      state,
    );
    expect(state).toMatchObject({ act: "voice", phase: "vo.result" });
    expect(state.thread.at(-1)).toMatchObject({
      kind: "outcome",
      i18nKey: "ob.conv.build.succeeded",
      tone: "success",
    });

    // Leaving the voice act opens straight into the connect screen: mail and
    // LinkedIn sit on it together, so there is no separate network-ask act.
    state = run([{ type: "VOICE_DONE" }], state);
    expect(state).toMatchObject({ act: "connect", phase: "cn.consent" });

    state = run(
      [
        {
          type: "LINKEDIN_SAVED",
          profile: "https://www.linkedin.com/in/x",
        },
      ],
      state,
    );
    // Resolving LinkedIn never moves the act — only mail's own consent gates
    // CONNECT_DONE.
    expect(state).toMatchObject({
      act: "connect",
      phase: "cn.consent",
      linkedinStatus: "saved",
    });

    // Leaving connect IS the end: it is the last stop, and its own move is
    // the terminal.
    state = run([{ type: "CONNECT_DONE" }], state);
    expect(state).toMatchObject({ act: "done", phase: "done" });
    expect(state.thread.at(-1)).toMatchObject({
      kind: "outcome",
      i18nKey: "ob.conv.done",
      tone: "success",
    });
  });

  it("records a failed read as a failure outcome and allows the manual path out", () => {
    let state = run([
      { type: "START", memberPath: false },
      { type: "READ_STARTED", readId: "r1" },
      { type: "READ_TERMINAL", readId: "r1", status: "failed" },
    ]);
    expect(state.phase).toBe("co.reading");
    expect(state.thread.at(-1)).toMatchObject({
      kind: "outcome",
      i18nKey: "ob.conv.read.failed",
      tone: "failure",
    });

    state = run(
      [{ type: "MANUAL_CHOSEN" }, { type: "COMPANY_CONFIRMED" }],
      state,
    );
    expect(state).toMatchObject({ act: "basis", phase: "bs.ask" });
  });

  it("declining the invite opens the team act, and leaving it ends the journey", () => {
    let state = run([
      { type: "START", memberPath: false },
      { type: "READ_STARTED", readId: "r1" },
      { type: "READ_TERMINAL", readId: "r1", status: "ready" },
      { type: "REVIEW_READY" },
      { type: "COMPANY_CONFIRMED" },
      { type: "BASIS_DONE" },
      { type: "INVITE_DECLINED" },
    ]);
    expect(state).toMatchObject({ act: "team", phase: "tm.ask" });
    expect(state.thread.at(-1)).toMatchObject({
      kind: "user",
      i18nKey: "ob.conv.invite.declined",
    });
    // Neither personal act is reachable from here: the answer was about them.
    expect(conversationReducer(state, { type: "VOICE_SKIPPED" })).toBe(state);
    expect(conversationReducer(state, { type: "CONNECT_DONE" })).toBe(state);

    state = run([{ type: "TEAM_DONE" }], state);
    expect(state).toMatchObject({ act: "done", phase: "done" });
    expect(state.thread.at(-1)).toMatchObject({
      kind: "outcome",
      i18nKey: "ob.conv.team.done",
      tone: "success",
    });
  });

  it("sends a succeeded build back to collecting on revise, corpus kept, and refuses it for any other outcome", () => {
    const built = run([
      { type: "START", memberPath: false },
      { type: "READ_STARTED", readId: "r1" },
      { type: "READ_TERMINAL", readId: "r1", status: "ready" },
      { type: "REVIEW_READY" },
      { type: "COMPANY_CONFIRMED" },
      { type: "BASIS_DONE" },
      { type: "INVITE_ACCEPTED" },
      { type: "BUILD_STARTED", buildId: "b1" },
      { type: "BUILD_TERMINAL", buildId: "b1", status: "succeeded" },
    ]);
    const revised = conversationReducer(built, { type: "VOICE_REVISE" });
    expect(revised).toMatchObject({ act: "voice", phase: "vo.collecting" });
    expect(revised.thread.at(-1)).toMatchObject({
      kind: "user",
      i18nKey: "ob.conv.voice.revise",
    });
    // A failed build retries and a deferred one resumes; neither is revised.
    const failed = run(
      [
        { type: "BUILD_STARTED", buildId: "b2" },
        { type: "BUILD_TERMINAL", buildId: "b2", status: "failed" },
      ],
      revised,
    );
    expect(conversationReducer(failed, { type: "VOICE_REVISE" })).toBe(failed);
  });

  it("lets the voice act be skipped and still reach connect", () => {
    const state = run([
      { type: "START", memberPath: false },
      { type: "READ_STARTED", readId: "r1" },
      { type: "READ_TERMINAL", readId: "r1", status: "ready" },
      { type: "REVIEW_READY" },
      { type: "COMPANY_CONFIRMED" },
      { type: "BASIS_DONE" },
      { type: "INVITE_ACCEPTED" },
      { type: "VOICE_SKIPPED" },
      { type: "VOICE_DONE" },
    ]);
    expect(state).toMatchObject({ act: "connect", phase: "cn.consent" });
    expect(
      state.thread.some(
        (entry) =>
          entry.kind === "user" && entry.i18nKey === "ob.conv.voice.skipped",
      ),
    ).toBe(true);
  });
});

describe("the welcome act", () => {
  it("rejects every event except START", () => {
    const notStart: ConversationEvent[] = [
      { type: "URL_SUBMITTED", url: "https://acme.example" },
      { type: "READ_STARTED", readId: "r1" },
      { type: "MANUAL_CHOSEN" },
      { type: "COMPANY_CONFIRMED" },
      { type: "RESUME" },
      { type: "CONNECT_DONE" },
    ];
    for (const event of notStart) {
      expect(conversationReducer(initialConversationState, event)).toBe(
        initialConversationState,
      );
    }
  });
});

describe("restore normalization out of co.confirmed", () => {
  const restored = (memberPath: boolean): ConversationState => ({
    ...initialConversationState,
    act: "company",
    phase: "co.confirmed",
    memberPath,
  });

  it("routes a restored creator to the basis act, as the live confirmation does", () => {
    expect(
      conversationReducer(restored(false), { type: "RESUME" }),
    ).toMatchObject({ act: "basis", phase: "bs.ask" });
  });

  it("routes a restored member to the voice act, where their journey begins", () => {
    expect(
      conversationReducer(restored(true), { type: "RESUME" }),
    ).toMatchObject({ act: "voice", phase: "vo.collecting" });
  });

  it("fast-forwards a creator to the stable point the target names", () => {
    const targets = [
      { target: "bs.ask", act: "basis" },
      { target: "vo.collecting", act: "voice" },
      { target: "vo.skipped", act: "voice" },
      { target: "in.ask", act: "invite" },
      { target: "tm.ask", act: "team" },
      { target: "cn.consent", act: "connect" },
    ] as const;
    for (const { target, act } of targets) {
      expect(
        conversationReducer(restored(false), { type: "RESUME", target }),
      ).toMatchObject({ act, phase: target });
    }
  });

  it("fast-forwards a member to their own stable points, and never into a creator act", () => {
    const own = [
      { target: "vo.collecting", act: "voice" },
      { target: "vo.skipped", act: "voice" },
      { target: "cn.consent", act: "connect" },
    ] as const;
    for (const { target, act } of own) {
      expect(
        conversationReducer(restored(true), { type: "RESUME", target }),
      ).toMatchObject({ act, phase: target });
    }
    // A member landed in the invite would stand in an act whose every event
    // is illegal for them — a screen with no way out.
    for (const target of ["bs.ask", "in.ask", "tm.ask"] as const) {
      expect(
        conversationReducer(restored(true), { type: "RESUME", target }),
      ).toMatchObject({ act: "voice", phase: "vo.collecting" });
    }
  });

  it("ignores RESUME outside co.confirmed", () => {
    const collecting: ConversationState = {
      ...initialConversationState,
      act: "voice",
      phase: "vo.collecting",
    };
    expect(
      conversationReducer(collecting, { type: "RESUME", target: "cn.consent" }),
    ).toBe(collecting);
  });
});

describe("restore seeding through START", () => {
  it("seeds recap turns and opens in co.confirmed when the company is saved", () => {
    const state = run([
      {
        type: "START",
        memberPath: false,
        companyConfirmed: true,
        recap: [
          {
            kind: "narration",
            id: "recap:back",
            i18nKey: "ob.conv.recap.back",
          },
          {
            kind: "narration",
            id: "recap:company",
            i18nKey: "ob.conv.recap.company",
            params: { name: "Gradion" },
          },
        ],
      },
    ]);
    expect(state).toMatchObject({ act: "company", phase: "co.confirmed" });
    expect(state.thread.map((entry) => entry.kind)).toEqual([
      "narration",
      "narration",
    ]);
    expect(state.thread[1]).toMatchObject({ params: { name: "Gradion" } });
  });

  it("a fresh START still opens the company intro with an empty thread", () => {
    const state = run([{ type: "START", memberPath: false }]);
    expect(state).toMatchObject({ act: "company", phase: "co.intro" });
    expect(state.thread).toEqual([]);
  });
});

describe("member path", () => {
  // A member's company was confirmed before they arrived: START opens in
  // co.confirmed and RESUME lands them on the first act that is theirs.
  const member = () =>
    run([
      { type: "START", memberPath: true, companyConfirmed: true },
      { type: "RESUME" },
    ]);

  it("walks voice, connect and preferences — the personal acts, nothing else", () => {
    let state = member();
    expect(state).toMatchObject({ act: "voice", phase: "vo.collecting" });
    state = run(
      [
        { type: "BUILD_STARTED", buildId: "b1" },
        { type: "BUILD_TERMINAL", buildId: "b1", status: "succeeded" },
        { type: "VOICE_DONE" },
      ],
      state,
    );
    expect(state).toMatchObject({ act: "connect", phase: "cn.consent" });
    state = run(
      [{ type: "LINKEDIN_SKIPPED" }, { type: "CONNECT_DONE" }],
      state,
    );
    expect(state).toMatchObject({ act: "done", phase: "done" });
  });

  it("ignores every creator-only event", () => {
    const state = member();
    // The installation's questions: its basis, whether its creator works in
    // it, and who to invite if not. None of them is a member's to answer.
    const creatorOnly: ConversationEvent[] = [
      { type: "BASIS_DONE" },
      { type: "INVITE_ACCEPTED" },
      { type: "INVITE_DECLINED" },
      { type: "TEAM_DONE" },
    ];
    for (const event of creatorOnly) {
      expect(conversationReducer(state, event)).toBe(state);
    }
    // The voice act is theirs, with every event in it.
    const skipped = run(
      [{ type: "VOICE_SKIPPED" }, { type: "VOICE_DONE" }],
      state,
    );
    expect(skipped).toMatchObject({ act: "connect", phase: "cn.consent" });
  });
});

describe("compile-time XOR contracts", () => {
  it("a question option and a user turn must each carry exactly one content source", () => {
    // @ts-expect-error an option without labelKey or label is unrepresentable
    const blank: ConversationQuestion["options"][number] = { value: "x" };
    // @ts-expect-error labelKey and label are mutually exclusive
    const both: ConversationQuestion["options"][number] = {
      value: "y",
      labelKey: "ob.conv.voice.skipped",
      label: "Yes",
    };
    // @ts-expect-error a user turn without i18nKey or text is unrepresentable
    const silent: ThreadEntry = { kind: "user", id: "u" };
    expect([blank.value, both.value, silent.kind]).toEqual(["x", "y", "user"]);
  });
});

describe("narration replace-in-place", () => {
  it("a repeated semantic id updates the existing bubble instead of stacking", () => {
    const counter = (pages: number) =>
      ({
        type: "NARRATION",
        readId: "r1",
        entry: {
          kind: "narration",
          id: "r1:pages",
          i18nKey: "ob.conv.read.pages",
          params: { pages: String(pages) },
        },
      }) satisfies ConversationEvent;
    let state = run([
      { type: "START", memberPath: false },
      { type: "READ_STARTED", readId: "r1" },
      counter(1),
      {
        type: "NARRATION",
        readId: "r1",
        entry: {
          kind: "narration",
          id: "r1:field:industry",
          i18nKey: "ob.conv.read.learnedField",
          params: { value: "Robotics" },
        },
      },
    ]);
    const before = state.thread.length;
    const stampedId = state.thread.find((entry) =>
      entry.id.endsWith(":r1:pages"),
    )?.id;

    state = conversationReducer(state, counter(7));

    // Latest params win; position and stamped id (the React key) hold, so
    // the counter never reorders below later narration.
    expect(state.thread.length).toBe(before);
    const updated = state.thread.find((entry) => entry.id === stampedId);
    expect(updated).toMatchObject({
      kind: "narration",
      params: { pages: "7" },
    });
    expect(state.thread.at(-1)?.id.endsWith(":r1:field:industry")).toBe(true);
  });
});

describe("thread cap", () => {
  it("caps the thread and drops the oldest narration before anything else", () => {
    let state = run([
      { type: "START", memberPath: false },
      { type: "URL_SUBMITTED", url: "https://acme.example" },
      { type: "READ_STARTED", readId: "r1" },
    ]);
    for (let index = 0; index < THREAD_CAP + 20; index += 1) {
      state = conversationReducer(state, {
        type: "NARRATION",
        readId: "r1",
        entry: {
          kind: "narration",
          id: `n:${index}`,
          i18nKey: "ob.conv.read.pages",
          params: { pages: String(index) },
        },
      });
    }
    expect(state.thread.length).toBe(THREAD_CAP);
    // The user's URL turn survives; the oldest narrations are gone.
    expect(state.thread[0]).toMatchObject({ kind: "user" });
    expect(
      state.thread.some(
        (entry) => entry.id.endsWith(":n:0") || entry.id.endsWith(":n:19"),
      ),
    ).toBe(false);
    expect(state.thread.at(-1)?.id.endsWith(`:n:${THREAD_CAP + 19}`)).toBe(
      true,
    );
  });
});
