/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { noteHistoryEntry } from "../lib/historydepth";
import type { Departure } from "../lib/navigatefrom";
import { throwProblem } from "../lib/problem";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { jsonResponse } from "../testing/story-utils";
import { ArchiveAction } from "./archive";

// The shared archive confirm. A refused archive leaves the dialog exactly as it
// was apart from one line of red text, so that line is the only thing that
// distinguishes "the server said no" from "it is still working".

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

function answerMe(allow: GrantSpec) {
  globalThis.fetch = (async () =>
    jsonResponse(meFixture({ allow }))) as typeof fetch;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject is what an archive SAYS mounts it the same. */}
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("ArchiveAction", () => {
  it("is disabled with the reason for a seat without the delete grant", async () => {
    answerMe({ contact: ["read", "update"] });
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="There is no undo control."
        archive={() => Promise.resolve({ id: "c-1" })}
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="Contact archived."
        onArchived={() => undefined}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await screen.findByText(en["record.actionRefused"]);
    expect(screen.getByTestId("archive-record").hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("takes the row's own answer over the grant when the record carries one", async () => {
    answerMe({ deal: ["read", "update"] });
    const archiveDeal = (archivable?: boolean) => (
      <ArchiveAction
        label="Archive deal"
        confirmText="It leaves the open deals."
        archive={() => Promise.resolve({ id: "d-1" })}
        invalidate="deals"
        recordKey="deal"
        archivedMessage="Deal archived."
        onArchived={() => undefined}
        requires={{ object: "deal", action: "delete" }}
        archivable={archivable}
      />
    );
    render(
      <>
        {archiveDeal(true)}
        {archiveDeal()}
      </>,
    );
    // The grant-only twin's refusal proves /me has answered.
    await screen.findByText(en["record.actionRefused"]);
    const [owned, granted] = screen.getAllByTestId("archive-record");
    expect(owned?.hasAttribute("disabled")).toBe(false);
    expect(granted?.hasAttribute("disabled")).toBe(true);
  });

  it("refuses a row the server says is not archivable, with the caller's reason", async () => {
    answerMe({ deal: ["read", "update", "delete"] });
    render(
      <ArchiveAction
        label="Archive deal"
        confirmText="It leaves the open deals."
        archive={() => Promise.resolve({ id: "d-1" })}
        invalidate="deals"
        recordKey="deal"
        archivedMessage="Deal archived."
        onArchived={() => undefined}
        requires={{ object: "deal", action: "delete" }}
        archivable={false}
        refusedReason="Only the owner archives this."
      />,
    );
    await screen.findByText("Only the owner archives this.");
    expect(screen.getByTestId("archive-record").hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("shows a permission refusal that carries no detail in the dialog", async () => {
    const user = userEvent.setup();
    answerMe({ contact: ["read", "delete"] });
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="There is no undo control."
        archive={() => throwProblem({ status: 403, code: "permission_denied" })}
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="Contact archived."
        onArchived={() => undefined}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await waitFor(() =>
      expect(
        screen.getByTestId("archive-record").hasAttribute("disabled"),
      ).toBe(false),
    );
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );
    expect(
      (await screen.findByRole("alert")).textContent?.length,
    ).toBeGreaterThan(0);
    expect(screen.getByRole("dialog")).toBeTruthy();
  });

  it("announces a refused archive with the server's own reason", async () => {
    const user = userEvent.setup();
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="There is no undo control."
        archive={() =>
          throwProblem({ detail: "the record is under retention" })
        }
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="Contact archived."
        onArchived={() => {
          throw new Error("a refused archive must not report success");
        }}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );

    const announced = await screen.findByRole("alert");
    expect(announced.textContent).toBe("the record is under retention");
    // Still open, so the reader can read the reason and decide — a dialog that
    // closed on failure would take the only explanation with it.
    expect(screen.getByRole("dialog")).toBeTruthy();
    // And nothing claims it worked. The confirmation and the refusal come from
    // the same choreography, so the one that must never fire on this path is
    // worth an assertion rather than an assumption.
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("says what went, once it has gone", async () => {
    const user = userEvent.setup();
    // A dialog closing is dismissal, not confirmation. It names the record
    // because "Archived." beside a list of twenty is a sentence about nothing.
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="There is no undo control."
        archive={() => Promise.resolve({ id: "p-1" })}
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="“Jana Brandt” archived"
        onArchived={() => {}}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      "“Jana Brandt” archived",
    );
  });

  describe("leaving the archived record", () => {
    // What a record page passes: close the record onto its list.
    const toList = (from: Departure) => from.closeOnto({ screen: "contacts" });

    function archiveContact(archive: () => Promise<{ id: string }>) {
      render(
        <ArchiveAction
          label="Archive contact"
          confirmText="There is no undo control."
          archive={archive}
          invalidate="contacts"
          recordKey="contact"
          archivedMessage="Contact archived."
          onArchived={toList}
          requires={{ object: "contact", action: "delete" }}
        />,
      );
    }

    async function confirm() {
      const user = userEvent.setup();
      await user.click(screen.getByTestId("archive-record"));
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: /^Archive/,
        }),
      );
    }

    it("puts the list in the place of a record opened from elsewhere", async () => {
      globalThis.history.replaceState(null, "", "#/contacts/c-1");
      const entries = globalThis.history.length;
      archiveContact(() => Promise.resolve({ id: "c-1" }));
      await confirm();

      expect(await screen.findByRole("status")).toHaveTextContent(
        "Contact archived.",
      );
      expect(globalThis.location.hash).toBe("#/contacts");
      expect(globalThis.history.length).toBe(entries);
    });

    it("leaves a reader who moved on before it finished where they are", async () => {
      globalThis.history.replaceState(null, "", "#/contacts/c-1");
      let finish: (archived: { id: string }) => void = () => undefined;
      archiveContact(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      );
      await confirm();
      globalThis.history.replaceState(null, "", "#/deals/d-1");
      finish({ id: "c-1" });

      expect(await screen.findByRole("status")).toHaveTextContent(
        "Contact archived.",
      );
      expect(globalThis.location.hash).toBe("#/deals/d-1");
    });

    it("goes back to the list it was opened from, filter and all", async () => {
      globalThis.history.replaceState(null, "", "#/contacts?owner=me");
      noteHistoryEntry("start");
      globalThis.history.pushState(null, "", "#/contacts/c-1");
      noteHistoryEntry("moved");
      const go = vi
        .spyOn(globalThis.history, "go")
        .mockImplementation(() => undefined);
      archiveContact(() => Promise.resolve({ id: "c-1" }));
      await confirm();

      expect(await screen.findByRole("status")).toHaveTextContent(
        "Contact archived.",
      );
      expect(go.mock.calls).toEqual([[-1]]);
      go.mockRestore();
    });
  });

  it("takes the archive back from its confirmation, when the screen can", async () => {
    const user = userEvent.setup();
    const undone: { id: string }[] = [];
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="You can bring it back from its history."
        archive={() => Promise.resolve({ id: "p-1" })}
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="“Jana Brandt” archived"
        onArchived={() => undefined}
        undo={async (archived) => {
          undone.push(archived);
        }}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );

    expect(undone).toEqual([{ id: "p-1" }]);
  });

  it("says why an undo from the confirmation was refused", async () => {
    const user = userEvent.setup();
    render(
      <ArchiveAction
        label="Archive contact"
        confirmText="You can bring it back from its history."
        archive={() => Promise.resolve({ id: "p-1" })}
        invalidate="contacts"
        recordKey="contact"
        archivedMessage="“Jana Brandt” archived"
        onArchived={() => undefined}
        undo={async () => throwProblem({ detail: "the record changed since" })}
        requires={{ object: "contact", action: "delete" }}
      />,
    );
    await user.click(screen.getByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["common.undo"] }),
    );

    expect(await screen.findByText("the record changed since")).toBeTruthy();
  });
});
