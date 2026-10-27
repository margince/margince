/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProvenanceTag } from "./provenance";
import { provenanceOf } from "./provenanceof";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Restored here rather than at the end of the case that installed a spy.
  // A case that fails its assertion never reaches its own teardown. A leaked
  // console spy then silences the rest of the file, which looks like a
  // suite that passes.
  vi.restoreAllMocks();
});

describe("provenanceOf", () => {
  it("maps captured_by to a kind without doubling the prefix", () => {
    // An agent id keeps the bare name, never "agent: agent:<id>".
    expect(provenanceOf("agent:capture")).toEqual({
      kind: "agent",
      job: "capture",
    });
    // A connector reads as a connector, not an agent.
    expect(provenanceOf("connector:gmail")).toEqual({
      kind: "connector",
      connector: "gmail",
    });
    // A background job reads as the system, not as an agent. The four kinds the
    // contract lists are four different answers to "who do I ask". Routing
    // the unrecognised ones into the agent arm makes a sweep announce itself as an AI.
    expect(provenanceOf("system:contact_auto_enrich")).toEqual({
      kind: "system",
      job: "contact_auto_enrich",
    });
    // A kind this app cannot read names no actor rather than the wrong one.
    expect(provenanceOf("capture")).toEqual({ kind: "unknown" });
  });

  it("says a job ran without naming one when the wire names none", () => {
    // The privacy-retention sweep stamps a bare `system`, so the kind is all
    // there is. The tag still has to say it was the system and not a contact.
    expect(provenanceOf("system")).toEqual({ kind: "system", job: undefined });
  });

  it("never carries a passport uuid into an agent tag", () => {
    // A passport call stamps `agent:<passport_id>`, and there is no name behind
    // it on this side: the design system holds no record lookups. So the kind
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
    // A named tool is not opaque and keeps its job name for the tag to look
    // up. The rule is about ids that name nothing, and still attributes agents.
    expect(provenanceOf("agent:document-extractor")).toEqual({
      kind: "agent",
      job: "document-extractor",
    });
  });

  it("names the connector, and never the member's uuid, whatever the id's shape", () => {
    // `connector:<system>:<user-uuid>`: the system is the connector, the uuid
    // is the member whose grant it ran under and belongs in no tag.
    expect(
      provenanceOf("connector:gmail:11111111-1111-4111-8111-111111111111"),
    ).toEqual({ kind: "connector", connector: "gmail" });
    // `connector:ext:<unit>[:<user-uuid>]`: the unit is the connector. A
    // 2-limit split reads the literal word "ext" for every unit, so
    // dispact-connector and zalo-oa would look the same.
    expect(
      provenanceOf(
        "connector:ext:dispact-connector:11111111-1111-4111-8111-111111111111",
      ),
    ).toEqual({ kind: "connector", connector: "dispact-connector" });
    expect(provenanceOf("connector:ext:zalo-oa")).toEqual({
      kind: "connector",
      connector: "zalo-oa",
    });
    // Nothing after the kind at all: the kind is still the true label.
    expect(provenanceOf("connector")).toEqual({
      kind: "connector",
      connector: "connector",
    });
    // The extension marker with no unit behind it names nothing that ran. So it
    // falls back to the kind rather than labelling the tag with a segment of the
    // grammar. Nothing mints this shape; it costs one fallback to not print it.
    expect(provenanceOf("connector:ext")).toEqual({
      kind: "connector",
      connector: "connector",
    });
  });

  it("keeps the whole remainder for a human id, and for a named agent", () => {
    // Dropping an extra segment of a human id would make a partial match
    // against the reader's own id, and that comparison must be exact.
    // It is safe to keep whole because it is compared and never printed.
    expect(provenanceOf("human:abc:stale-tail", "abc")).toEqual({
      kind: "human",
      self: false,
      userId: "abc:stale-tail",
    });
    // An agent id that is a name all the way through is kept whole, however
    // many segments it has.
    expect(provenanceOf("agent:ext:notes:summarise")).toEqual({
      kind: "agent",
      job: "ext:notes:summarise",
    });
  });

  it("only calls a human 'you' when that human is the reader", () => {
    // "Typed by you" over a colleague's entry is a false statement about who
    // to ask. So the human branch carries the id and whether it is the
    // reader's. The tag decides the wording from that, not from the kind.
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
    // A buyer's own write stamps `buyer:<participant uuid>`, since the principal
    // is the participant. Without an arm for it the string reaches the fallback
    // below, which says nobody recorded a source. The source is recorded here,
    // and it is a contact. The two are one branch apart, so both are asserted together.
    expect(provenanceOf("buyer:0192abcd-2222-4222-8222-222222222222")).toEqual({
      kind: "buyer",
    });
    expect(provenanceOf("someone")).toEqual({ kind: "unknown" });
  });

  it("never carries a participant uuid into a buyer tag", () => {
    // A participant uuid resolves to no name on this side. A tag that
    // printed it would attribute the change to a string. Asserted on the whole
    // value, so a later field carrying the id fails here.
    const uuid = "0192abcd-3333-4333-8333-333333333333";
    expect(JSON.stringify(provenanceOf(`buyer:${uuid}`))).not.toContain(uuid);
  });

  it("reports an unrecorded source as unknown rather than as the reader's own typing", () => {
    // Reading an unattributed row as "entered by you" would be an attribution
    // nobody can check.
    expect(provenanceOf(undefined)).toEqual({ kind: "unknown" });
    expect(provenanceOf("")).toEqual({ kind: "unknown" });
  });
});

// A tool's identifier is not its name.
//
// The wire spells a job the way the code does. Printed as is, a reader sees
// "Automatisiert durch signal-scan": the plumbing, in a sentence in their own
// language.
describe("what a tool is called on screen", () => {
  const tagText = (capturedBy: string, locale: "en" | "de") => {
    render(
      <LocaleProvider initial={locale}>
        <ProvenanceTag provenance={provenanceOf(capturedBy)} />
      </LocaleProvider>,
    );
    return document.body.textContent;
  };

  it("names an agent in the reader's language, never by its id", () => {
    expect(tagText("agent:signal-scan", "de")).toBe(
      "Automatisiert durch E-Mail-Analyse",
    );
    cleanup();
    expect(tagText("agent:capture_counterparty_verdict", "en")).toBe(
      "Automated by Mail sender check",
    );
  });

  it("says only the kind for an agent the catalogue cannot name", () => {
    expect(tagText("agent:ext:notes:summarise", "de")).toBe(
      "Automatisiert durch einen Agenten",
    );
    cleanup();
    expect(tagText("agent:01a05500-0000-7000-8000-000000000001", "en")).toBe(
      "Automated by an agent",
    );
    cleanup();
    expect(tagText("agent:signal_scanner_v2", "en")).toBe(
      "Automated by an agent",
    );
  });

  it("names a data update without printing the task identifier", () => {
    expect(tagText("system:datatask:core/100_mail", "en")).toContain(
      "Data update",
    );
    expect(tagText("system:datatask:core/100_mail", "en")).not.toContain(
      "core/100_mail",
    );
  });

  // The core catalogue cannot know a unit's jobs, so an extension dispatcher
  // keeps its unit and job as words rather than losing its name.
  it("names an extension dispatcher from its kind", () => {
    expect(tagText("agent:ext_openchannel_drain", "de")).toBe(
      "Automatisiert durch openchannel drain",
    );
  });
});

// An import runs as one administrator, so captured_by names that seat on every
// row it wrote. The reader who ran it would be told they typed the file.
describe("a row the CSV importer wrote", () => {
  const VIEWER = "u-admin";
  const imported = (stamp: string) =>
    provenanceOf(`human:${VIEWER}`, VIEWER, undefined, stamp);

  it("reads as imported, not typed by the administrator who ran it", () => {
    expect(imported("mirror:csv:pipedrive-export:company:co-12")).toEqual({
      kind: "import",
      uploadName: "pipedrive-export",
    });
  });

  it("reads the wire's imported_from when the row carries no stamp", () => {
    const fromWire = (importedFrom: string | null) =>
      provenanceOf(`human:${VIEWER}`, VIEWER, undefined, "", importedFrom);

    expect(fromWire("crm")).toEqual({ kind: "import", uploadName: "crm" });
    expect(fromWire("")).toEqual({ kind: "import", uploadName: undefined });
    expect(fromWire(null)).toMatchObject({ kind: "human", self: true });
  });

  it("names a connector import by the product's own spelling", () => {
    const tag = (importedFrom: string) => {
      render(
        <LocaleProvider initial="vi">
          <ProvenanceTag
            provenance={provenanceOf(
              `human:${VIEWER}`,
              VIEWER,
              undefined,
              "",
              importedFrom,
            )}
          />
        </LocaleProvider>,
      );
      const said = document.body.textContent;
      cleanup();
      return said;
    };

    expect(tag("hubspot")).toBe("Đã nhập từ HubSpot");
    // An upload the administrator named keeps the name they gave it.
    expect(tag("Q3 leads")).toBe("Đã nhập từ Q3 leads");
  });

  it("names the upload of an imported activity", () => {
    expect(imported("mirror:csv:pipedrive-export:activity:a-9")).toEqual({
      kind: "import",
      uploadName: "pipedrive-export",
    });
  });

  it("names the upload and never the stamp that carries it", () => {
    const provenance = imported("mirror:csv:Q3 leads.v2:contact:a@b.example");

    expect(provenance).toEqual({ kind: "import", uploadName: "Q3 leads.v2" });
    expect(JSON.stringify(provenance)).not.toContain("mirror:");
  });

  it("says only that it was imported when the upload has no name", () => {
    for (const stamp of [
      "mirror:csv:company:7",
      "mirror:csv:lead:l-3",
      "mirror:csv:company:contact",
      "mirror:csv:company:lead",
      "mirror:csv:contact:company",
      "mirror:csv",
      "mirror:",
    ]) {
      expect(imported(stamp), stamp).toEqual({
        kind: "import",
        uploadName: undefined,
      });
    }
  });

  it("keeps an upload whose name is an object word", () => {
    expect(imported("mirror:csv:contact:contact:7")).toEqual({
      kind: "import",
      uploadName: "contact",
    });
  });

  it("never reads an object it does not know as the upload's name", () => {
    expect(imported("mirror:csv:deal:7")).toEqual({
      kind: "import",
      uploadName: undefined,
    });
  });

  it("lets the source system's author speak first", () => {
    const author = { display_name: "Mutaz Suleiman", via: "HubSpot" };

    expect(
      provenanceOf(
        `human:${VIEWER}`,
        VIEWER,
        author,
        "mirror:csv:pipedrive-export:company:1",
      ),
    ).toEqual({ kind: "human", self: true, userId: VIEWER, author });
  });

  it("leaves a typed row typed, whatever free text its source holds", () => {
    for (const stamp of ["manual", "my-mirror:csv:x"]) {
      expect(imported(stamp), stamp).toEqual({
        kind: "human",
        self: true,
        userId: VIEWER,
      });
    }
  });
});
