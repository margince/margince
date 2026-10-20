/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  company,
  renderFiling,
  renderRail,
  stub,
  view,
} from "./companyrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CompanyRail", () => {
  it("offers the add-tag verb once the section has answered, on a writable record", async () => {
    stub();
    renderRail();
    // The section answered `empty` (view()'s tags defaults to []), so the
    // verb renders beside the tags it acts on.
    expect(
      await screen.findByRole("button", { name: /add tag/i }),
    ).toBeInTheDocument();
  });

  it("withholds the add-tag verb on an archived record", async () => {
    stub();
    renderRail({
      view: view({
        company: { ...company, archived_at: "2026-06-02T00:00:00Z" },
      }),
    });
    // The values themselves still draw (this suite's own empty-badges test
    // covers that); only the verb stands down.
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /add tag/i }),
      ).not.toBeInTheDocument();
    });
  });

  // sectionState (design-system/surfacestate.tsx) reads an undefined `view`
  // as "loading" only when told the composite read is still running.
  // Otherwise it reads the same undefined `view` as "unavailable", the words a
  // real outage shows. Before `loading` was threaded through, every ordinary page open
  // flashed "could not be loaded" on Health/Contacts/Tags for as long as the
  // read was in flight. Pinned here as two DIFFERENT renders rather than one
  // happy-path check, because a fix that only makes the loading case not
  // crash is not the fix — it has to render DIFFERENTLY from the failed case.
  it("reads an in-flight composite read as loading, not unavailable", () => {
    stub();
    const { container } = renderFiling(true);
    // The skeleton placeholder, not the "could not be loaded" sentence.
    expect(container.querySelector(".skeleton")).toBeTruthy();
    expect(screen.queryByText(en["state.unavailable"])).not.toBeInTheDocument();
  });

  it("reads a failed composite read (view undefined, not loading) as unavailable", () => {
    stub();
    renderFiling(false);
    // The SAME undefined `view` as the loading test above, but with
    // `loading={false}` — the honest "could not be loaded" sentence, not a
    // skeleton pretending a read is still running.
    expect(screen.getAllByText(en["state.unavailable"]).length).toBeGreaterThan(
      0,
    );
  });
});
