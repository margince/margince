/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { quietRun } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture, READS_RECORDS } from "../../testing/mefixture";
import { BriefScreen } from "./brief";
import { firstBriefWait } from "./brief.firstday";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const declined = {
  data: ["morning_brief", "overnight_at_risk_sweep"].map((spec) => ({
    spec,
    state: "declined",
    credential_usable: false,
    credential_funds_agent: false,
  })),
};

function firstDayPanel(): Promise<HTMLElement> {
  return screen.findByRole("region", { name: en["brief.firstDay.title"] });
}

describe("Home before the first Morning brief", () => {
  it("says what Home is for and where to turn the overnight brief on", async () => {
    stubApi({ "GET /me/agent-grants": () => jsonResponse(declined) });
    render(<BriefScreen />);
    const panel = await firstDayPanel();
    expect(within(panel).getByText(en["brief.firstDay.what"])).toBeTruthy();
    await within(panel).findByText(en["brief.firstDay.briefOff"], {
      exact: false,
    });
    const link = within(panel).getByRole("link", {
      name: en["brief.firstDay.turnOn"],
    });
    expect(link.getAttribute("href")).toContain("settings/account");
    expect(screen.queryByText(en["magic.heading.brief"])).toBeNull();
  });

  // A granted answer whose credential lapsed prepares nothing tonight.
  it("sends a reader whose overnight authority lapsed to renew it", async () => {
    const lapsed = {
      data: declined.data.map((grant) => ({ ...grant, state: "granted" })),
    };
    stubApi({ "GET /me/agent-grants": () => jsonResponse(lapsed) });
    render(<BriefScreen />);
    const panel = await firstDayPanel();
    expect(
      await within(panel).findByRole("link", {
        name: en["brief.firstDay.turnOn"],
      }),
    ).toBeTruthy();
    expect(
      within(panel).queryByText(en["brief.firstDay.briefTonight"]),
    ).toBeNull();
  });

  // A read seat writes nothing, so the overnight switch is not theirs to turn.
  it("offers a read-only member no link to a switch the seat cannot turn", async () => {
    stubApi({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: READS_RECORDS, seat: "read" })),
      "GET /me/agent-grants": () => jsonResponse(declined),
    });
    render(<BriefScreen />);
    const panel = await firstDayPanel();
    expect(
      within(panel).getByText(en["brief.firstDay.briefLater"]),
    ).toBeTruthy();
    expect(
      within(panel).queryByRole("link", { name: en["brief.firstDay.turnOn"] }),
    ).toBeNull();
  });

  it("is gone once a brief has been prepared", async () => {
    stubApi({ "GET /brief": () => jsonResponse(quietRun) });
    render(<BriefScreen />);
    await screen.findByText(/^Briefed by Margince at/);
    expect(
      screen.queryByRole("region", { name: en["brief.firstDay.title"] }),
    ).toBeNull();
  });
});

describe("firstBriefWait", () => {
  it.each([
    [true, true, "prepared"],
    [true, false, "switchedOff"],
    [true, undefined, "notTheirs"],
    [false, false, "notTheirs"],
    [false, true, "notTheirs"],
  ] as const)("can write %s, granted %s: %s", (canMutate, granted, wait) => {
    expect(firstBriefWait(canMutate, granted)).toBe(wait);
  });
});
