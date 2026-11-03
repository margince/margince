/** @vitest-environment happy-dom */
import { afterEach, expect, it, vi } from "vitest";
import { type Translator, translate } from "../../i18n";
import { currentParams } from "../../lib/urlstate";
import { runContactMomentAction } from "./contactpage.drawers";

afterEach(() => {
  window.location.hash = "";
});

it("opens the exact privacy duty without retaining an unrelated queue filter", () => {
  window.location.hash =
    "#/contacts/p-1?owner=other&filter=tasks&queue_view=decisions";
  const t: Translator = (key, params) => translate("en", key, params);
  runContactMomentAction(
    {
      kind: "open_record",
      label: "Review privacy duty",
      state: "available",
      destination: { surface: "privacy_notice", entity_id: "case-1" },
    },
    t,
    {
      contactId: "p-1",
      openComposer: vi.fn(),
      setDrawer: vi.fn(),
      openBrief: vi.fn(),
      nextMeetingId: null,
    },
  );
  const params = currentParams();
  expect(window.location.hash).toContain("#/settings/privacy?");
  expect(params.get("notice_case")).toBe("case-1");
  expect(params.has("queue_scope")).toBe(false);
  expect(params.has("owner")).toBe(false);
  expect(params.has("filter")).toBe(false);
  expect(params.has("queue_view")).toBe(false);
});
