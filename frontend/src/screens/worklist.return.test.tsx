/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { translate } from "../i18n";
import { en } from "../i18n/en";
import { readingsDay, taskRow } from "./brief.fixtures";
import { BriefQueue } from "./brief.queue";
import { jsonResponse, render, stubApi } from "./brief.testkit";
import { runContactMomentAction } from "./contactpage";
import {
  keptWorklistReturn,
  WorklistReturnLink,
  withWorklistReturn,
} from "./worklist.return";
import { renderWorklist, stub, day as worklistDay } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const aboutWeber: components["schemas"]["WorklistItem"] = {
  ...taskRow("t-1", "Call Weber"),
  subject: { type: "contact", id: "c-1", label: "Weber" },
};

function linkTo(name: string): URLSearchParams {
  const href = screen.getByRole("link", { name }).getAttribute("href") ?? "";
  return new URLSearchParams(href.split("?")[1] ?? "");
}

describe("a row opened from the Worklist drawer", () => {
  it("carries the drawer's filter, scope and owner onto the record link", async () => {
    window.location.hash =
      "#/home?filter=tasks&owner=u-7&queue=1&queue_scope=team";
    stubApi({
      "GET /worklist": () => jsonResponse(readingsDay({}, [aboutWeber])),
    });
    render(<BriefQueue />);
    await screen.findByRole("link", { name: "Call Weber" });

    const dials = linkTo("Call Weber");
    expect(dials.get("from")).toBe("worklist");
    expect(dials.get("filter")).toBe("tasks");
    expect(dials.get("queue_scope")).toBe("team");
    expect(dials.get("owner")).toBe("u-7");
    expect(dials.has("queue"), "the record would open the drawer over it").toBe(
      false,
    );
  });

  it("links the same row bare on the Worklist page itself", async () => {
    window.location.hash = "#/worklist?filter=tasks";
    stub(worklistDay({ queue: [aboutWeber] }));
    renderWorklist();
    await screen.findByRole("link", { name: "Call Weber" });

    expect(linkTo("Call Weber").has("from")).toBe(false);
  });

  it("leaves a link to anything but a record page untouched", () => {
    const drawer = new Map([["filter", "tasks"]]);
    expect(withWorklistReturn("#/settings/privacy?case=x", drawer)).toBe(
      "#/settings/privacy?case=x",
    );
    expect(withWorklistReturn("#/contacts/c-1/network", drawer)).toBe(
      "#/contacts/c-1/network?filter=tasks&from=worklist",
    );
  });
});

describe("Back to Worklist on the record", () => {
  it("returns to Home with the drawer open on the carried dials", () => {
    window.location.hash =
      "#/contacts/c-1?filter=tasks&from=worklist&owner=u-7&queue_scope=team";
    render(<WorklistReturnLink />);

    const back = screen.getByRole("link", { name: en["brief.queue.back"] });
    expect(back.getAttribute("href")).toBe(
      "#/home?filter=tasks&owner=u-7&queue=1&queue_scope=team",
    );
  });

  it("is absent on a record opened any other way", () => {
    window.location.hash = "#/contacts/c-1?filter=tasks";
    render(<WorklistReturnLink />);

    expect(
      screen.queryByRole("link", { name: en["brief.queue.back"] }),
    ).toBeNull();
  });

  it("travels with a tab switch inside the record", () => {
    window.location.hash = "#/contacts/c-1?filter=tasks&from=worklist&prep=1";
    expect(Object.fromEntries(keptWorklistReturn() ?? [])).toEqual({
      from: "worklist",
      filter: "tasks",
    });

    window.location.hash = "#/contacts/c-1?prep=1";
    expect(keptWorklistReturn()).toBeUndefined();
  });
});

describe("a related record opened from a record that came from the Worklist", () => {
  const openDeal: components["schemas"]["ContactMomentAction"] = {
    kind: "open_record",
    label: "Open the deal",
    state: "available",
    destination: { surface: "record", entity_type: "deal", entity_id: "d-1" },
  };
  const handlers = {
    contactId: "c-1",
    openComposer: vi.fn(),
    setDrawer: vi.fn(),
    openBrief: vi.fn(),
    nextMeetingId: null,
  };
  const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

  it("carries the way back onto the deal a contact moment opens", () => {
    window.location.hash = "#/contacts/c-1?filter=tasks&from=worklist";
    runContactMomentAction(openDeal, t, handlers);

    expect(window.location.hash).toBe("#/deals/d-1?filter=tasks&from=worklist");
  });

  it("opens the deal bare when the contact came from anywhere else", () => {
    window.location.hash = "#/contacts/c-1";
    runContactMomentAction(openDeal, t, handlers);

    expect(window.location.hash).toBe("#/deals/d-1");
  });
});
