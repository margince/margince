/** @vitest-environment happy-dom */
import {
  cleanup,
  render as renderLibrary,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { translate } from "../../i18n";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import {
  renderWorklist,
  stub,
  day as worklistDay,
} from "../../testing/worklist.testkit";
import { BriefQueue } from "../brief/brief.queue";
import { runContactMomentAction } from "../contacts/contactpage";
import {
  keptWorklistReturn,
  WorklistReturnScope,
  withWorklistReturn,
  worklistReturnHref,
} from "./worklist.return";

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
    await screen.findByRole("button", { name: "Call Weber" });
    await userEvent
      .setup()
      .click(
        screen.getByRole("button", { name: "More actions for Call Weber" }),
      );
    await screen.findByRole("link", { name: "Open" });

    const dials = linkTo("Open");
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
    await screen.findByRole("button", { name: "Call Weber" });
    await userEvent
      .setup()
      .click(
        screen.getByRole("button", { name: "More actions for Call Weber" }),
      );
    await screen.findByRole("link", { name: "Open" });

    expect(linkTo("Open").has("from")).toBe(false);
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

  it("marks a shared component's record link drawn inside the drawer", () => {
    const drawer = new Map([["queue_scope", "team"]]);
    renderLibrary(
      <>
        <WorklistReturnScope dials={drawer}>
          <AppLink href="#/contacts/c-1">Open contact</AppLink>
          <AppLink href="#/settings/privacy">Privacy</AppLink>
        </WorklistReturnScope>
        <AppLink href="#/contacts/c-2">Elsewhere</AppLink>
      </>,
    );

    expect(linkTo("Open contact").get("from")).toBe("worklist");
    expect(linkTo("Open contact").get("queue_scope")).toBe("team");
    expect(linkTo("Privacy").has("from")).toBe(false);
    expect(linkTo("Elsewhere").has("from")).toBe(false);
  });
});

describe("the way back to the Worklist from a record", () => {
  it("returns to Home with the drawer open on the carried dials", () => {
    expect(
      worklistReturnHref(
        "#/contacts/c-1?filter=tasks&from=worklist&owner=u-7&queue_scope=team",
      ),
    ).toBe("#/home?filter=tasks&owner=u-7&queue=1&queue_scope=team");
  });

  it("returns to the view the drawer was on", () => {
    expect(
      worklistReturnHref(
        "#/contacts/c-1?from=worklist&queue_scope=team&queue_view=decisions",
      ),
    ).toBe("#/home?queue=1&queue_scope=team&queue_view=decisions");
  });

  it("is absent on a record opened any other way", () => {
    expect(worklistReturnHref("#/contacts/c-1?filter=tasks")).toBeUndefined();
  });

  it("is absent on a page that is not a returning record", () => {
    expect(
      worklistReturnHref("#/settings/privacy?from=worklist"),
    ).toBeUndefined();
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

  it("opens the composer drafting when the moment names the composer", () => {
    const openComposer = vi.fn();
    runContactMomentAction(
      {
        kind: "draft_reply",
        label: "Draft a follow-up",
        state: "available",
        destination: {
          surface: "composer",
          entity_type: "activity",
          entity_id: "a-1",
          prefill: { intent: "follow_up" },
        },
      },
      t,
      { ...handlers, openComposer },
    );

    expect(openComposer).toHaveBeenCalledWith(
      translate("en", "contact.composer.intentFollowUp"),
      true,
      "a-1",
    );
  });

  it("opens the deal bare when the contact came from anywhere else", () => {
    window.location.hash = "#/contacts/c-1";
    runContactMomentAction(openDeal, t, handlers);

    expect(window.location.hash).toBe("#/deals/d-1");
  });
});
