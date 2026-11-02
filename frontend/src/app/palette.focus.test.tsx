/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as renderRTL, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type Command, CommandPalette } from "./palette";

afterEach(cleanup);

// One screen command is enough: this file asks where focus goes when the
// palette closes, not what it lists.
const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["pipeline"],
    type: "screen",
    route: { screen: "deals" },
  },
];

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderRTL(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

it("hands focus out of the overlay before making it inert", () => {
  const opener = document.createElement("button");
  opener.textContent = "open the palette";
  document.body.append(opener);
  try {
    opener.focus();
    const { rerender } = render(
      <CommandPalette open onClose={() => {}} commands={commands} />,
    );
    expect(document.activeElement).toBe(screen.getByRole("searchbox"));

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <CommandPalette open={false} onClose={() => {}} commands={commands} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    // The overlay stays mounted to play its exit and is marked inert. Focus
    // left inside an inert subtree is focus no reader can use, and the next
    // Tab restarts at the top of the document.
    const inertOverlay = document.querySelector("[inert]");
    if (inertOverlay) {
      expect(inertOverlay.contains(document.activeElement)).toBe(false);
    }
    expect(document.activeElement).toBe(opener);
  } finally {
    opener.remove();
  }
});
