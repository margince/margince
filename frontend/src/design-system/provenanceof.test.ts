/** @vitest-environment happy-dom */
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { provenanceOf } from "./provenanceof";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Unconditionally, not at the end of the case that installed a spy: a case
  // that fails its assertion never reaches its own teardown, and a leaked
  // console spy silences the rest of the file — which looks exactly like a
  // suite that passes.
  vi.restoreAllMocks();
});

describe("provenanceOf", () => {
  it("maps captured_by to a kind without doubling the prefix", () => {
    // An agent id keeps the bare name — never the old "agent: agent:<id>".
    expect(provenanceOf("agent:capture")).toEqual({
      kind: "agent",
      agent: "capture",
    });
    // A connector reads as a connector, not an agent.
    expect(provenanceOf("connector:gmail")).toEqual({
      kind: "connector",
      connector: "gmail",
    });
    // A background job reads as the system, not as an agent: the four kinds the
    // contract enumerates are four different answers to "who do I ask", and
    // routing the unrecognised ones into the agent arm made a scheduled sweep
    // announce itself as an AI.
    expect(provenanceOf("system:contact_auto_enrich")).toEqual({
      kind: "system",
      job: "contact_auto_enrich",
    });
    // A kind this app cannot read names no actor rather than the wrong one.
    expect(provenanceOf("capture")).toEqual({ kind: "unknown" });
  });

  it("says a job ran without naming one when the wire names none", () => {
    // The privacy-retention sweep stamps a bare `system`, so the kind is all
    // there is; the tag still has to say it was the system and not a contact.
    expect(provenanceOf("system")).toEqual({ kind: "system", job: undefined });
  });

  it("never carries a passport uuid into an agent tag", () => {
    // A passport call stamps `agent:<passport_id>`, and there is no name behind
    // it on this side — the design system holds no record lookups. So the kind
    // travels and the id does not: the tag renders "Automated by an agent"
    // rather than "Automated by 0192abcd-…".
    expect(provenanceOf("agent:0192abcd-1111-4111-8111-111111111111")).toEqual({
      kind: "agent",
      agent: undefined,
    });
    // Upper case is the same id. Nothing mints it that way today, and a matcher
    // that only reads one casing would let the other through as a label.
    expect(provenanceOf("agent:0192ABCD-1111-4111-8111-111111111111")).toEqual({
      kind: "agent",
      agent: undefined,
    });
    // A named tool is not opaque and keeps its name: this is a rule about ids
    // that name nothing, not a refusal to attribute agents at all.
    expect(provenanceOf("agent:document-extractor")).toEqual({
      kind: "agent",
      agent: "document-extractor",
    });
  });

  it("names the connector, and never the member's uuid, whatever the id's shape", () => {
    // `connector:<system>:<user-uuid>`: the system is the connector, the uuid
    // is the member whose grant it ran under and belongs in no tag.
    expect(
      provenanceOf("connector:gmail:11111111-1111-4111-8111-111111111111"),
    ).toEqual({ kind: "connector", connector: "gmail" });
    // `connector:ext:<unit>[:<user-uuid>]`: the UNIT is the connector. Parsed
    // with a 2-limit split this read the literal word "ext" — identically for
    // every unit, so dispact-connector and zalo-oa were indistinguishable.
    expect(
      provenanceOf(
        "connector:ext:dispact-connector:11111111-1111-4111-8111-111111111111",
      ),
    ).toEqual({ kind: "connector", connector: "dispact-connector" });
    expect(provenanceOf("connector:ext:zalo-oa")).toEqual({
      kind: "connector",
      connector: "zalo-oa",
    });
    // Nothing after the kind at all: the kind is still the honest label, which
    // is what the old parse fell back to as well.
    expect(provenanceOf("connector")).toEqual({
      kind: "connector",
      connector: "connector",
    });
    // The extension marker with no unit behind it names nothing that ran, so it
    // falls back to the kind rather than labelling the tag with a segment of the
    // grammar. Nothing mints this shape; it costs one fallback to not print it.
    expect(provenanceOf("connector:ext")).toEqual({
      kind: "connector",
      connector: "connector",
    });
  });

  it("keeps the whole remainder for a human id, and for a named agent", () => {
    // An extra segment used to be discarded silently, which for a human id is a
    // partial match against the reader's own — the one comparison that must be
    // exact. It is safe to keep whole because it is compared and never printed.
    expect(provenanceOf("human:abc:stale-tail", "abc")).toEqual({
      kind: "human",
      self: false,
      userId: "abc:stale-tail",
    });
    // An agent id that is a name all the way through is a name a reader can act
    // on, however many segments it has.
    expect(provenanceOf("agent:ext:notes:summarise")).toEqual({
      kind: "agent",
      agent: "ext:notes:summarise",
    });
  });

  it("only calls a human 'you' when that human is the reader", () => {
    // "Typed by you" over a colleague's entry is a false statement about who
    // to ask, so the human branch carries the id and whether it is the
    // reader's — the tag decides the wording from that, not from the kind.
    expect(provenanceOf("human:abc", "abc")).toEqual({
      kind: "human",
      self: true,
      userId: "abc",
    });
    expect(provenanceOf("human:abc", "someone-else")).toEqual({
      kind: "human",
      self: false,
      userId: "abc",
    });
    // No session resolved yet: a caller that cannot say who is reading cannot
    // claim the reader typed it.
    expect(provenanceOf("human:abc")).toEqual({
      kind: "human",
      self: false,
      userId: "abc",
    });
  });

  it("reads a Deal Room participant as a buyer, not as an unrecorded source", () => {
    // A buyer's own write stamps `buyer:<participant uuid>` — the principal is
    // the participant — and with no arm for it the string fell through to the
    // fallback below, which says nobody recorded a source. The source IS
    // recorded here, and it is a contact: the two are one branch apart, so both
    // are asserted together.
    expect(provenanceOf("buyer:0192abcd-2222-4222-8222-222222222222")).toEqual({
      kind: "buyer",
    });
    expect(provenanceOf("someone")).toEqual({ kind: "unknown" });
  });

  it("never carries a participant uuid into a buyer tag", () => {
    // The half that made this worth fixing rather than a cosmetic gain: a
    // participant uuid resolves to no name on this side, and a tag that
    // printed it would attribute the change to a string. Asserted on the whole
    // value, so a later field carrying the id fails here.
    const uuid = "0192abcd-3333-4333-8333-333333333333";
    expect(JSON.stringify(provenanceOf(`buyer:${uuid}`))).not.toContain(uuid);
  });

  it("reports an unrecorded source as unknown rather than as the reader's own typing", () => {
    // The old fallback made every unattributed row read as "entered by you" —
    // the one attribution nobody can check.
    expect(provenanceOf(undefined)).toEqual({ kind: "unknown" });
    expect(provenanceOf("")).toEqual({ kind: "unknown" });
  });
});

// A tool's identifier is not its name.
//
// The wire spells a job the way the code does, and the live Worklist printed
// "Automated by capture_counterparty_verdict" at a reader — the plumbing,
// offered as information.
describe("what a tool is called on screen", () => {
  it("writes a tool's name in words rather than as an identifier", () => {
    const provenance = provenanceOf("agent:capture_counterparty_verdict");

    expect(provenance).toEqual({
      kind: "agent",
      agent: "capture counterparty verdict",
    });
  });

  it("still says nothing where the id has no name in it", () => {
    const provenance = provenanceOf(
      "agent:01a05500-0000-7000-8000-000000000001",
    );

    expect(provenance).toEqual({ kind: "agent", agent: undefined });
  });
});
