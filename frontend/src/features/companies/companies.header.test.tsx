/** @vitest-environment happy-dom */
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
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { company, jsonResponse } from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { meFixture } from "../../testing/mefixture";
import { RecordShell } from "../../testing/recordshell.testkit";
import { CompanyScreen } from "./companies";

// The account header's `archivedReasonId` wiring specifically: split out of
// companies.test.tsx (already past the 1000-line ceiling frontend/AGENTS.md
// sets for a test file) rather than grown there.
//
// CompanyHeaderActions and CompanyActionBadges are unit-tested directly in
// companyheaderactions.test.tsx and companyheader.test.tsx; the bug this file
// pins lived one level up, in how CompanyScreen composes them, so a
// leaf-component test with a hand-supplied prop would have exercised the fix,
// not the defect.

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
    expect(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    ).toBeTruthy(),
  );
  await user.click(
    screen.getByRole("button", { name: en["record.moreActions"] }),
  );
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
  it.each([
    ["a writing role", ["read", "update"], en["record.notYoursToChange"]],
    ["a role that only reads", ["read"], en["record.roleReadsOnly"]],
  ] as const)(
    "names why edit/merge/archive/share are refused to %s, from an id the page actually renders",
    async (_role, actions, sentence) => {
      const user = userEvent.setup();
      const notMine = {
        ...company,
        owner_id: "u-someone-else",
        writable: false,
      };
      stubFetch(async (url) => {
        if (url.includes("/me")) {
          return jsonResponse(meFixture({ allow: { company: actions } }));
        }
        if (url.includes("/activities")) {
          return jsonResponse({ data: [] });
        }
        return jsonResponse(notMine);
      });
      render(<CompanyScreen id="o-1" />);

      const refused = [
        await openRecordMenu(user, "archive-record"),
        await screen.findByTestId("share-record"),
      ];
      for (const control of refused) {
        expect(control.hasAttribute("disabled")).toBe(true);
        const describedBy = control.getAttribute("aria-describedby");
        expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
          sentence,
        );
      }
    },
  );
});
