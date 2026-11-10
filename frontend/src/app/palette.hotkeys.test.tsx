// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { holdExits } from "../design-system/presence-testing";
import { LocaleProvider } from "../i18n";
import type { Screen } from "../lib/router";
import { useShortcutDispatcher } from "../lib/shortcuts";
import { CREATE_ID, CREATE_SCREENS } from "./nav";
import {
  type Command,
  CommandPalette,
  isCreateCommand,
  useBuiltinCommands,
} from "./palette";
import { type PaletteMode, usePaletteShortcuts } from "./palettehotkey";

// The palette's keys as the shell wires them: the one dispatcher, and the
// handlers App mounts for search, `/` and `C`.

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return {
    ...view,
    rerender: (next: ReactNode) =>
      view.rerender(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">{next}</LocaleProvider>
        </QueryClientProvider>,
      ),
  };
};

const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    type: "screen",
    route: { screen: "deals" },
  },
  {
    id: "action:new-deal",
    label: "New deal",
    type: "action",
    route: { screen: "deals", id: CREATE_ID },
  },
];

// Every row button except "Ask your documents", which is not a command.
function destinationRows(): HTMLElement[] {
  return screen
    .getAllByRole("button")
    .filter((button) => !button.classList.contains("palette-ask"));
}

// The shell's wiring, with a dialog the page behind may be holding up.
function ShellWithDialog({
  dialogOpen,
  onDialogClose = () => {},
  screen = "home",
}: Readonly<{
  dialogOpen: boolean;
  onDialogClose?: () => void;
  screen?: Screen;
}>) {
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [mode, setMode] = useState<PaletteMode>("all");
  usePaletteShortcuts(
    paletteOpen,
    (next) => {
      setMode(next);
      setPaletteOpen(true);
    },
    () => setPaletteOpen(false),
    screen,
  );
  useShortcutDispatcher();
  return (
    <>
      <CommandPalette
        open={paletteOpen}
        mode={mode}
        onClose={() => setPaletteOpen(false)}
        commands={commands}
      />
      <Modal
        open={dialogOpen}
        onClose={onDialogClose}
        labelledBy="edit-deal"
        intent="form"
      >
        <Heading size="large" id="edit-deal">
          Edit deal
        </Heading>
        <button type="button">Save</button>
      </Modal>
    </>
  );
}

// The palette is the app, and an open dialog makes the app unreachable.
describe("the palette hotkey", () => {
  it("does nothing while a dialog is up, and Escape still closes the dialog", async () => {
    const onDialogClose = vi.fn();
    render(<ShellWithDialog dialogOpen onDialogClose={onDialogClose} />);
    const user = userEvent.setup();

    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.queryByRole("searchbox")).toBeNull();
    // The browser's own ⌘K is still withheld: the chord belongs to the product.
    expect(fireEvent.keyDown(document.body, { key: "k", metaKey: true })).toBe(
      false,
    );

    await user.keyboard("{Escape}");
    expect(onDialogClose).toHaveBeenCalledOnce();
  });

  it("opens the palette and closes it again on the same chord", async () => {
    render(<ShellWithDialog dialogOpen={false} />);
    const user = userEvent.setup();

    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.getByRole("searchbox")).toBeTruthy();
    await user.keyboard("{Meta>}k{/Meta}");
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("opens the palette again while its own exit is still playing", async () => {
    const exits = holdExits();
    try {
      render(<ShellWithDialog dialogOpen={false} />);
      const user = userEvent.setup();
      await user.keyboard("{Meta>}k{/Meta}");
      await user.keyboard("{Escape}");
      expect(document.querySelector(".palette-overlay[inert]")).not.toBeNull();

      await user.keyboard("{Meta>}k{/Meta}");
      const input = screen.getByRole("searchbox");
      expect(input.closest("[inert]")).toBeNull();
    } finally {
      exits.mockRestore();
    }
  });

  it("opens over a dialog that is already on its way out", async () => {
    const exits = holdExits();
    try {
      const view = render(<ShellWithDialog dialogOpen />);
      view.rerender(<ShellWithDialog dialogOpen={false} />);
      expect(view.baseElement.querySelector("#edit-deal")).not.toBeNull();
      const user = userEvent.setup();

      await user.keyboard("{Meta>}k{/Meta}");
      expect(screen.getByRole("searchbox")).toBeTruthy();
    } finally {
      exits.mockRestore();
    }
  });
});

describe("the palette's single keys", () => {
  it("opens on / without typing it into the box", async () => {
    render(<ShellWithDialog dialogOpen={false} />);
    const user = userEvent.setup();

    await user.keyboard("/");
    expect(screen.getByRole("searchbox")).toHaveProperty("value", "");
  });

  it.each([...CREATE_SCREENS])(
    "opens the %s list's own create form on C",
    async (listScreen) => {
      window.location.hash = `#/${listScreen}`;
      render(<ShellWithDialog dialogOpen={false} screen={listScreen} />);
      const user = userEvent.setup();

      await user.keyboard("c");
      expect(window.location.hash).toBe(`#/${listScreen}/${CREATE_ID}`);
      expect(screen.queryByRole("searchbox")).toBeNull();
    },
  );

  it("lists only what creates on C where the screen has no form", async () => {
    render(<ShellWithDialog dialogOpen={false} screen="analytics" />);
    const user = userEvent.setup();

    await user.keyboard("c");
    expect(destinationRows().map((row) => row.textContent)).toEqual([
      expect.stringContaining("New deal"),
    ]);
  });

  it("keeps every command once ⌘K opens it again", async () => {
    render(<ShellWithDialog dialogOpen={false} screen="analytics" />);
    const user = userEvent.setup();

    await user.keyboard("c{Escape}{Meta>}k{/Meta}");
    expect(destinationRows()).toHaveLength(commands.length);
  });
});

describe("isCreateCommand", () => {
  it("names the commands whose id says they make a record", () => {
    expect(
      commands.filter(isCreateCommand).map((command) => command.id),
    ).toEqual(["action:new-deal"]);
  });
});

describe("the palette C opens", () => {
  it("lists only what creates, and nothing beside it, even with a word typed", async () => {
    render(
      <CommandPalette
        open
        mode="create"
        onClose={() => {}}
        commands={commands}
      />,
    );
    const user = userEvent.setup();
    expect(screen.queryByText("Ask your documents")).toBeNull();

    await user.type(screen.getByRole("searchbox"), "deal");
    expect(destinationRows().map((row) => row.textContent)).toEqual([
      expect.stringContaining("New deal"),
    ]);
    expect(screen.queryByText("Ask your documents")).toBeNull();
  });

  it("lists every create command the product has", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(meFixture({}))),
    );
    function Builtin() {
      return (
        <CommandPalette
          open
          mode="create"
          onClose={() => {}}
          commands={useBuiltinCommands()}
        />
      );
    }
    render(<Builtin />);
    expect(destinationRows().map((row) => row.textContent)).toEqual(
      ["New contact", "New company", "New lead", "New deal", "New project"].map(
        (label) => expect.stringContaining(label),
      ),
    );
  });
});
