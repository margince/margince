/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { KnowledgeCard } from "./knowledge";
import {
  ADMIN,
  backendFor,
  corpus,
  render,
  shippedProblem,
} from "./knowledge.testkit";

// Settings → Knowledge → Document sets: archiving, editing and restoring a set,
// and the set Margince ships, which is edited but never archived.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("KnowledgeCard set verbs", () => {
  it("asks before archiving a set, and sends nothing until the reader agrees", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    expect(
      within(screen.getByRole("dialog")).getByText("Archive How-to?"),
    ).toBeTruthy();
    expect(backend.deletes).toHaveLength(0);

    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(backend.deletes.length).toBeGreaterThan(0));
  });

  // The archived row takes its menu with it, so focus cannot go back there.
  it("hands focus to the card's title once a set is archived", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(screen.queryByText("How-to")).toBeNull());
    expect(globalThis.document.activeElement?.textContent).toBe(
      "Document sets",
    );
  });

  it("renames a set through Edit, sending only the field that changed", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: "Edit set" }));
    const dialog = screen.getByRole("dialog");
    const name = within(dialog).getByRole("textbox", { name: /^name/i });
    await user.clear(name);
    await user.type(name, "Runbook");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(backend.patches).toEqual([{ name: "Runbook" }]));
    expect(await screen.findByText("Runbook")).toBeTruthy();
  });

  it("says a cleared name is required on Save, and sends nothing", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: "Edit set" }));
    const dialog = screen.getByRole("dialog");
    await user.clear(within(dialog).getByRole("textbox", { name: /^name/i }));
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByText(/Name is required/)).toBeTruthy();
    expect(backend.patches).toEqual([]);
  });

  it("shows an archived set under Show archived sets and restores it", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(screen.queryByText("How-to")).toBeNull());

    await user.click(
      screen.getByRole("switch", { name: /show archived sets/i }),
    );
    expect(await screen.findByText("Archived")).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "Actions for How-to" }),
    );
    expect(screen.queryByRole("button", { name: "Edit set" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Restore set" }));
    await waitFor(() =>
      expect(backend.posts.some((url) => url.endsWith("/restore"))).toBe(true),
    );
    await waitFor(() => expect(screen.queryByText("Archived")).toBeNull());
    expect(screen.getByText("How-to")).toBeTruthy();
  });

  it("returns focus to the menu when the archive is called off", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    const menu = await screen.findByRole("button", {
      name: "Actions for How-to",
    });
    await user.click(menu);
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Cancel",
      }),
    );
    await waitFor(() => expect(globalThis.document.activeElement).toBe(menu));
  });
});

describe("KnowledgeCard, a set Margince ships", () => {
  // Its words can still be edited, and an update keeps those edits.
  it("offers an edit but no archive and no upload on it", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, { sets: [corpus({ shipped: true })] }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await screen.findByRole("table", { name: "Documents in How-to" });
    await user.click(
      screen.getByRole("button", { name: "Actions for How-to" }),
    );
    expect(screen.getByRole("button", { name: "Edit set" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /archive set/i })).toBeNull();
    expect(screen.queryByLabelText(/add document/i)).toBeNull();
  });

  it("says why an archive was refused when the set turned out to ship", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        refuse: { archive: () => shippedProblem("corpus_shipped") },
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    const dialog = within(screen.getByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: /^archive set$/i }));
    expect(
      await dialog.findByText(
        "This set ships with Margince, so it cannot be archived.",
      ),
    ).toBeInTheDocument();
  });

  it("says why an upload was refused when the set turned out to ship", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        refuse: { upload: () => shippedProblem("corpus_shipped") },
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await user.upload(
      await screen.findByLabelText(/add document/i),
      new File(["notes"], "notes.md", { type: "text/markdown" }),
    );
    await user.click(screen.getByRole("button", { name: "Add document" }));
    expect(
      await screen.findByText(
        "notes.md: This set ships with Margince, so only Margince adds documents to it.",
      ),
    ).toBeInTheDocument();
  });
});
