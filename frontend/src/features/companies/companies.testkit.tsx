import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { expect } from "vitest";
import { LocaleProvider } from "../../i18n";
import { RecordShell } from "../../testing/recordshell.testkit";
import { WriteToHost } from "../settings/writetohost";

// The page's context column belongs to the shell, and the record fills it
// through a portal. So these renders carry the real region, not a stand-in.
// A test with its own column would prove nothing about the one the product
// draws, and the record's context cards would have nowhere to land.
export function render(ui: ReactNode): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The composer host is the shell's in the running app (`App.tsx`);
            the contacts cards' addresses are buttons into it. */}
        <WriteToHost>
          <RecordShell>{ui}</RecordShell>
        </WriteToHost>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The rare verbs live behind the header's overflow menu. Its items stay mounted
// so their dialogs survive the closing click; the shut state is asserted apart.
export async function openRecordMenu(
  testId: string,
  user: UserEvent | typeof userEvent = userEvent,
): Promise<HTMLElement> {
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "More actions" })).toBeTruthy(),
  );
  await user.click(screen.getByRole("button", { name: "More actions" }));
  await waitFor(() => expect(screen.getByTestId(testId)).toBeTruthy());
  return screen.getByTestId(testId);
}

// openProfile switches to the tab with the account's own reference material.
// That is the dossier, its filed fields, its facts and its connections. It also
// holds the one-off tools: enrichment, the deep read, the hierarchy roll-up.
// None of it renders under any other tab.
export async function openProfile() {
  await userEvent.click(await screen.findByRole("button", { name: "Profile" }));
}
