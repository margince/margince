/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { jsonResponse, org, stubFetch } from "./company.fixtures";
import { CompanyScreen } from "./organizations";

// The account header's `archivedReasonId` wiring specifically: split out of
// organizations.test.tsx (already past the 1000-line ceiling frontend/AGENTS.md
// sets for a test file) rather than grown there.
//
// CompanyPrimaryActions and CompanyActionBadges are unit-tested directly in
// companyheader.test.tsx; the bug this file pins lived one level up, in how
// CompanyScreen composes them — a leaf-component test with a hand-supplied
// prop would have exercised the fix, not the defect.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function openRecordMenu(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
): Promise<HTMLElement> {
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "More actions" })).toBeTruthy(),
  );
  await user.click(screen.getByRole("button", { name: "More actions" }));
  await waitFor(() => expect(screen.getByTestId(testId)).toBeTruthy());
  return screen.getByTestId(testId);
}

// `archivedReasonId` is a `useId()` value, so passing it to
// CompanyPrimaryActions/CompanyActionBadges unconditionally is always
// truthy — even on a live record — which told CompanyActionBadges a
// sentence was already drawn elsewhere and left its OWN refusal (a record
// that is somebody else's) pointing `aria-describedby` at an id nothing on
// the page carries. Distinct from the archived case: this is the record's
// OTHER refusal cause, on a record that is not archived at all.
describe("CompanyScreen — a live record that is not the viewer's to change", () => {
  it("names why edit/merge/archive/share are refused, from an id the page actually renders", async () => {
    const user = userEvent.setup();
    const notMine = { ...org, owner_id: "u-someone-else", writable: false };
    stubFetch(async (url) => {
      if (url.includes("/me")) {
        return jsonResponse(meFixture({ allow: { organization: ["read"] } }));
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(notMine);
    });
    render(<CompanyScreen id="o-1" />);

    // Each control WAITED for, not read in the tick the first one arrived in.
    // openRecordMenu waits for `edit-record`; the two below were then fetched
    // with a non-retrying getByTestId in the same breath, so a menu whose
    // items mount a beat apart threw here — the failure this file sees under a
    // loaded full run and never in isolation.
    const refused = [
      await openRecordMenu(user, "edit-record"),
      await screen.findByTestId("archive-record"),
      await screen.findByTestId("share-record"),
    ];
    for (const control of refused) {
      expect(control.hasAttribute("disabled")).toBe(true);
      const describedBy = control.getAttribute("aria-describedby");
      expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
        "This company belongs to someone else. Ask its owner to share it with you if you need to make changes.",
      );
    }
  });
});
