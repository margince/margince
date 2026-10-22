/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { LocaleProvider } from "../i18n";
import { navigate, useHash } from "../lib/router";
import { UnsavedGuard, useGuardedLeave, useUnsavedGuard } from "./unsaved";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

// A page with two addresses and a draft on the first one, which is the shape the
// settings screen has: the content is chosen by an address the sidebar changes,
// and the draft lives several components below whatever holds the answer.
function Page({
  onKeep,
  inDialog = false,
}: Readonly<{ onKeep?: (address: string) => void; inDialog?: boolean }>) {
  const [address, setAddress] = useState("first");
  return (
    <LocaleProvider>
      <button type="button" onClick={() => setAddress("second")}>
        go second
      </button>
      <UnsavedGuard
        address={address}
        onKeep={(kept) => {
          setAddress(kept);
          onKeep?.(kept);
        }}
      >
        {(shown) => {
          if (shown !== "first") return <p>Second page</p>;
          return inDialog ? <DraftDialog /> : <DraftCard />;
        }}
      </UnsavedGuard>
    </LocaleProvider>
  );
}

function DraftCard() {
  const [typed, setTyped] = useState("");
  useUnsavedGuard(typed !== "");
  return (
    <label>
      Signature
      <input value={typed} onChange={(event) => setTyped(event.target.value)} />
    </label>
  );
}

function DraftDialog() {
  const [open, setOpen] = useState(true);
  return (
    <Modal
      open={open}
      onClose={() => setOpen(false)}
      labelledBy="d"
      intent="form"
    >
      <Heading size="large" id="d">
        Signature
      </Heading>
      <DraftCard />
    </Modal>
  );
}

const dialog = () => screen.queryByRole("dialog");

describe("UnsavedGuard", () => {
  it("lets the reader move on when nothing is unsaved", async () => {
    const user = userEvent.setup();
    render(<Page />);
    await user.click(screen.getByRole("button", { name: "go second" }));
    expect(screen.getByText("Second page")).toBeInTheDocument();
    expect(dialog()).toBeNull();
  });

  it("holds the page it is on and asks once something is typed", async () => {
    const user = userEvent.setup();
    render(<Page />);
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));

    expect(dialog()).not.toBeNull();
    // The draft is STILL ON SCREEN, which is the whole point: a guard that
    // shows the new page and asks afterwards has already taken the work away.
    expect(screen.getByRole("textbox", { name: /Signature/ })).toHaveValue(
      "Jane",
    );
    expect(screen.queryByText("Second page")).toBeNull();
  });

  it("holds the page for a draft inside a dialog too", async () => {
    const user = userEvent.setup();
    render(<Page inDialog />);
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    fireEvent.click(
      screen.getByRole("button", { name: "go second", hidden: true }),
    );

    expect(screen.getByText(/Leaving this page discards/)).toBeInTheDocument();
    expect(screen.queryByText("Second page")).toBeNull();
  });

  it("lets the page go once the dialog's draft is discarded", async () => {
    const user = userEvent.setup();
    render(<Page inDialog />);
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Discard changes" }));
    await user.click(screen.getByRole("button", { name: "go second" }));

    expect(screen.getByText("Second page")).toBeInTheDocument();
  });

  it("puts the address back when the reader keeps the edit", async () => {
    const user = userEvent.setup();
    const onKeep = vi.fn();
    render(<Page onKeep={onKeep} />);
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));
    // Escape is the safe answer: dismissing the question must not be the way to
    // lose the work.
    await user.keyboard("{Escape}");

    expect(onKeep).toHaveBeenCalledWith("first");
    expect(dialog()).toBeNull();
    expect(screen.getByRole("textbox", { name: /Signature/ })).toHaveValue(
      "Jane",
    );
  });

  it("moves on when the reader discards", async () => {
    const user = userEvent.setup();
    render(<Page />);
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));
    await user.click(screen.getByRole("button", { name: "Discard changes" }));

    expect(screen.getByText("Second page")).toBeInTheDocument();
    expect(dialog()).toBeNull();
  });

  it("stops guarding once the draft is cleared again", async () => {
    // The claim follows the FLAG, not the mounting: a reader who undoes their own
    // typing has nothing unsaved, and a guard that still asks has become a
    // dialog they cannot get rid of.
    const user = userEvent.setup();
    render(<Page />);
    const field = screen.getByRole("textbox", { name: /Signature/ });
    await user.type(field, "Jane");
    await user.clear(field);
    await user.click(screen.getByRole("button", { name: "go second" }));

    expect(screen.getByText("Second page")).toBeInTheDocument();
    expect(dialog()).toBeNull();
  });

  it("asks the window before a reload, and only while something is unsaved", async () => {
    const user = userEvent.setup();
    render(<Page />);

    // A listener that is merely present costs the page its back/forward cache,
    // so its absence while clean is part of the contract rather than an
    // accident of implementation.
    const clean = new Event("beforeunload", { cancelable: true });
    globalThis.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);

    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "Jane");
    const dirty = new Event("beforeunload", { cancelable: true });
    globalThis.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);
  });
});

/** A draft whose page can save it, and optionally a second one it cannot. */
function SaveablePage({
  save,
  lone = false,
  onKeep,
}: Readonly<{
  save: () => Promise<boolean>;
  lone?: boolean;
  onKeep?: (address: string) => void;
}>) {
  const [address, setAddress] = useState("first");
  return (
    <LocaleProvider>
      <button type="button" onClick={() => setAddress("second")}>
        go second
      </button>
      <UnsavedGuard
        address={address}
        onKeep={(kept) => {
          setAddress(kept);
          onKeep?.(kept);
        }}
      >
        {(shown) =>
          shown === "first" ? (
            <>
              <SaveableCard save={save} />
              {lone && <DraftCard />}
            </>
          ) : (
            <p>Second page</p>
          )
        }
      </UnsavedGuard>
    </LocaleProvider>
  );
}

function SaveableCard({ save }: Readonly<{ save: () => Promise<boolean> }>) {
  const [typed, setTyped] = useState("");
  useUnsavedGuard(typed !== "", undefined, save);
  return (
    <label>
      Name
      <input value={typed} onChange={(event) => setTyped(event.target.value)} />
    </label>
  );
}

describe("UnsavedGuard with a save", () => {
  it("offers Save, Discard and Stay, and moves on once Save has written", async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => true);
    render(<SaveablePage save={save} />);
    await user.type(screen.getByRole("textbox", { name: /Name/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));

    expect(screen.getByRole("button", { name: "Discard" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Stay" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(save).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Second page")).toBeInTheDocument();
  });

  it("stays on the page and says so when the save is refused", async () => {
    const user = userEvent.setup();
    render(<SaveablePage save={async () => false} />);
    await user.type(screen.getByRole("textbox", { name: /Name/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("alert")).toBeVisible();
    expect(screen.queryByText("Second page")).toBeNull();
  });

  it("keeps the edit on Stay", async () => {
    const user = userEvent.setup();
    const onKeep = vi.fn();
    render(<SaveablePage save={async () => true} onKeep={onKeep} />);
    await user.type(screen.getByRole("textbox", { name: /Name/ }), "Jane");
    await user.click(screen.getByRole("button", { name: "go second" }));
    await user.click(screen.getByRole("button", { name: "Stay" }));

    expect(onKeep).toHaveBeenCalledWith("first");
    expect(screen.getByRole("textbox", { name: /Name/ })).toHaveValue("Jane");
  });

  it("offers no Save while a draft it cannot save is also unsaved", async () => {
    const user = userEvent.setup();
    render(<SaveablePage save={async () => true} lone />);
    await user.type(screen.getByRole("textbox", { name: /Name/ }), "Jane");
    await user.type(screen.getByRole("textbox", { name: /Signature/ }), "J");
    await user.click(screen.getByRole("button", { name: "go second" }));

    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(
      screen.getByRole("button", { name: "Discard changes" }),
    ).toBeVisible();
  });
});

// The app's shape around one held draft: the guard above whatever the address
// picks, and a page whose own save is the way off it.
function Routed({ page }: Readonly<{ page: ReactNode }>) {
  const hash = useHash();
  return (
    <LocaleProvider>
      <UnsavedGuard
        address={hash}
        onKeep={(kept) => {
          window.location.hash = kept;
        }}
      >
        {(shown) =>
          shown === "#/filters/contacts" ? page : <p>{`Arrived at ${shown}`}</p>
        }
      </UnsavedGuard>
    </LocaleProvider>
  );
}

/** A draft that is always unsaved, and a button that keeps it and moves on. */
function SavedDraft({ leaving }: Readonly<{ leaving: "guarded" | "plain" }>) {
  const leave = useGuardedLeave(true);
  const go = leaving === "guarded" ? leave : navigate;
  return (
    <button type="button" onClick={() => go({ screen: "lists", id: "L1" })}>
      keep it
    </button>
  );
}

/** A kept draft beside a second one, each with its own way to settle. */
function TwoDrafts() {
  const leave = useGuardedLeave(true);
  const [other, setOther] = useState(true);
  useUnsavedGuard(other);
  return (
    <>
      <button
        type="button"
        onClick={() => leave({ screen: "lists", id: "L1" })}
      >
        keep it
      </button>
      <button type="button" onClick={() => setOther(false)}>
        save the other
      </button>
      <button
        type="button"
        onClick={() => navigate({ screen: "lists", id: "L1" })}
      >
        go plainly
      </button>
    </>
  );
}

describe("useGuardedLeave", () => {
  it("reaches the address without the guard asking", async () => {
    window.location.hash = "#/filters/contacts";
    const user = userEvent.setup();
    render(<Routed page={<SavedDraft leaving="guarded" />} />);
    await user.click(screen.getByRole("button", { name: "keep it" }));

    expect(
      await screen.findByText("Arrived at #/lists/L1"),
    ).toBeInTheDocument();
    expect(dialog()).toBeNull();
  });

  it("is the only way that does: a plain move off the same draft still asks", async () => {
    window.location.hash = "#/filters/contacts";
    const user = userEvent.setup();
    render(<Routed page={<SavedDraft leaving="plain" />} />);
    await user.click(screen.getByRole("button", { name: "keep it" }));

    expect(
      await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Arrived at #/lists/L1")).toBeNull();
  });

  it("guards its draft again when the move it made is kept back", async () => {
    window.location.hash = "#/filters/contacts";
    const user = userEvent.setup();
    render(<Routed page={<TwoDrafts />} />);
    await user.click(screen.getByRole("button", { name: "keep it" }));
    // The other draft still asks, and Escape keeps it, page and all.
    await screen.findByRole("dialog", { name: "Discard unsaved changes?" });
    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(window.location.hash).toBe("#/filters/contacts"),
    );

    await user.click(screen.getByRole("button", { name: "save the other" }));
    await user.click(screen.getByRole("button", { name: "go plainly" }));
    expect(
      await screen.findByRole("dialog", { name: "Discard unsaved changes?" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Arrived at #/lists/L1")).toBeNull();
  });

  it("moves at once where no guard is installed", async () => {
    const user = userEvent.setup();
    render(<SavedDraft leaving="guarded" />);
    await user.click(screen.getByRole("button", { name: "keep it" }));

    await waitFor(() => expect(window.location.hash).toBe("#/lists/L1"));
  });
});
