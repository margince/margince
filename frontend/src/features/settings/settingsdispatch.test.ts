import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SETTINGS_PAGES, type SettingsPageId } from "../../lib/settingscatalog";
import { drawsComponent } from "./drawn-testing";
import { SETTINGS_PAGE_VIEWS } from "./settings";

// Every catalog page renders something, and nothing renders that is not a page.
//
// The catalog decides which pages EXIST and `SETTINGS_PAGE_VIEWS` decides what
// each one draws, and the two are separate declarations. A page declared in the
// catalog with no view appears in navigation, is reachable by address, and
// renders nothing. A reader gets a blank column and no explanation.

const here = dirname(fileURLToPath(import.meta.url));

function dispatchedIds(): Set<string> {
  return new Set(Object.keys(SETTINGS_PAGE_VIEWS));
}

/** Whether a page draws `card` on `tab`, read from the element tree it returns. */
async function tabDraws(id: SettingsPageId, tab: string, card: string) {
  return drawsComponent(await SETTINGS_PAGE_VIEWS[id].drawn({ tab }), card);
}

/** The tab ids a catalog page carries, or the bare page. */
function tabsOf(id: SettingsPageId): string[] {
  const page = SETTINGS_PAGES.find((each) => each.id === id);
  return page && "tabs" in page ? page.tabs.map((tab) => tab.id) : [""];
}

describe("the moved cards", () => {
  // The settingsdispatch census rule proves every card is still reached, but
  // not where one sits. So the moved cards are asserted by page, against the dispatch.
  it("dispatches the moved cards to the pages they moved to", async () => {
    /** One component's body, for a page whose entry draws a tab component. */
    function bodyOf(fn: string, file: string): string {
      const text = readFileSync(join(here, file), "utf8");
      const start = text.indexOf(`function ${fn}(`);
      if (start < 0) {
        throw new Error(`${file} has no ${fn}`);
      }
      return text.slice(start, text.indexOf("\n}\n", start));
    }
    // The mail-sharing switch writes the WORKSPACE's posture. It belongs with
    // the rest of the capture rules, not among the reader's own mailboxes.
    expect(await tabDraws("capture", "sharing", "MailSharingCard")).toBe(true);
    for (const tab of tabsOf("connections")) {
      expect(await tabDraws("connections", tab, "MailSharingCard")).toBe(false);
    }
    // And the personal autonomy choice belongs on the page about agents
    // deciding without you, under the tier reference it is written in terms of.
    // `agents` draws a tab COMPONENT that holds the cards. An assertion on
    // the page alone would pass with the card nowhere at all.
    for (const tab of tabsOf("account")) {
      expect(await tabDraws("account", tab, "AutonomySettingsCard")).toBe(
        false,
      );
    }
    expect(await tabDraws("agents", "", "AiAccessTab")).toBe(true);
    const aiAccess = [
      bodyOf("AiAccessTab", "settings.aiaccess.tsx"),
      bodyOf("DeveloperTabs", "settings.aiaccess.tsx"),
    ].join("\n");
    expect(aiAccess).toContain("<AutonomySettingsCard />");
    expect(aiAccess).toContain("<ConnectAiToolCard />");
    expect(aiAccess).toContain("<PassportCard />");
    // The mailbox list stays the first thing Connections draws.
    expect(tabsOf("connections")[0]).toBe("mail");
    expect(await tabDraws("connections", "mail", "ConnectorsCard")).toBe(true);
  });
});

describe("the dispatch covers the catalog", () => {
  it("draws every page the catalog declares", () => {
    const dispatched = dispatchedIds();
    const missing = SETTINGS_PAGES.map((page) => page.id).filter(
      (id) => !dispatched.has(id),
    );
    expect(missing).toEqual([]);
  });

  it("draws nothing the catalog does not declare", () => {
    // The other direction: an entry left behind after a page was renamed or
    // dropped.
    const declared = new Set<string>(SETTINGS_PAGES.map((page) => page.id));
    const extra = [...dispatchedIds()].filter((id) => !declared.has(id));
    expect(extra).toEqual([]);
  });

  it("finds a realistic number of arms, so a broken scan cannot pass", () => {
    // Two empty sets compare equal, so an empty read would report perfect
    // agreement between nothing and nothing.
    expect(dispatchedIds().size).toBe(SETTINGS_PAGES.length);
    expect(SETTINGS_PAGES.length).toBeGreaterThan(20);
  });
});
