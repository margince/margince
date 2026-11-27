/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { ProjectHealthModal } from "./projecthealthmodal";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("says the note is required for an at-risk reading on Save, and sends nothing", async () => {
  const fetched = vi.fn();
  vi.stubGlobal("fetch", fetched);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ProjectHealthModal open onClose={() => {}} projectId="p1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByText("At risk"));
  await user.click(
    screen.getByRole("button", {
      name: englishCopy["projectHealth.saveReading"],
    }),
  );
  expect(screen.getByText("Note is required.")).toBeTruthy();
  expect(document.activeElement).toBe(
    screen.getByRole("textbox", { name: /^Note/ }),
  );
  expect(fetched).not.toHaveBeenCalled();
});
