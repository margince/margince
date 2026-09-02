/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useLayoutEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button } from "../design-system/atoms";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { throwProblem } from "./common";
import { EditAction, EditRecordModal } from "./edit";

// The shared edit-record form (the mirror of create): a record prefills the
// form, submit carries only the typed values (the screen attaches ifMatch),
// and a rejected update renders the server's own detail — while a failure that
// is not a server refusal never puts its own words on screen.

afterEach(() => {
  cleanup();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject is what a save SAYS mounts it the same way. */}
        <ToastProvider>
          {ui}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const record = { id: "p1", version: 3, full_name: "Alice" };
const fields = [
  { key: "full_name", label: "create.fullName" as const, required: true },
];

describe("edit record flow", () => {
  it("prefills the form from the record", async () => {
    render(
      <EditAction
        label="Edit"
        fields={fields}
        record={record}
        update={vi.fn(async () => record)}
        invalidate="people"
        recordKey="person"
        savedMessage="Saved."
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    expect(
      (screen.getByLabelText("Full name *") as HTMLInputElement).value,
    ).toBe("Alice");
  });

  it("submits only the typed values", async () => {
    const update = vi.fn(async (_values: Record<string, unknown>) => record);
    render(
      <EditAction
        label="Edit"
        fields={fields}
        record={record}
        update={update}
        invalidate="people"
        recordKey="person"
        savedMessage="Saved."
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    const input = screen.getByLabelText("Full name *");
    await userEvent.clear(input);
    await userEvent.type(input, "Alice M");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
    expect(update.mock.calls[0][0]).toEqual({ full_name: "Alice M" });
  });

  // The prefill's timing, not just its result: an edit that silently drops
  // what the user typed is the bug this pins.
  const twoFields = [
    { key: "full_name", label: "create.fullName" as const, required: true },
    { key: "title", label: "create.personTitle" as const },
  ];
  const twoFieldRecord = {
    id: "p1",
    version: 3,
    full_name: "Alice",
    title: "CTO",
  };

  it("is prefilled in the very commit that puts the form on screen", async () => {
    // What the Full name input holds the moment the open modal reaches the
    // DOM. A layout effect runs inside that commit — after the DOM is
    // updated, before the browser paints and before any passive effect — so
    // it sees precisely the first frame a user could see and type into.
    const firstFrame: string[] = [];
    function OpenHarness() {
      const [open, setOpen] = useState(false);
      useLayoutEffect(() => {
        const input = screen.queryByLabelText(
          "Full name *",
        ) as HTMLInputElement | null;
        if (input) {
          firstFrame.push(input.value);
        }
      });
      return (
        <>
          <Button small onClick={() => setOpen(true)}>
            Open
          </Button>
          <EditRecordModal
            open={open}
            onClose={() => setOpen(false)}
            title="Edit"
            fields={twoFields}
            record={twoFieldRecord}
            pending={false}
            error={null}
            onSubmit={vi.fn()}
          />
        </>
      );
    }
    render(<OpenHarness />);
    await userEvent.click(screen.getByRole("button", { name: "Open" }));
    // Prefilling in a passive effect puts the form on screen blank and fills
    // it a commit later; that gap is the window a keystroke lands in and gets
    // written through empty form state.
    expect(firstFrame).toEqual(["Alice"]);
  });

  it("renders the server's own detail for a rejected update", async () => {
    const update = vi.fn(async () => {
      throwProblem({ status: 422, detail: "name too long" });
    });
    render(
      <EditAction
        label="Edit"
        fields={fields}
        record={record}
        update={update}
        invalidate="people"
        recordKey="person"
        savedMessage="Saved."
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText("name too long")).toBeTruthy());
  });

  it("never shows the words of a failure the server did not send", async () => {
    const update = vi.fn(async () => {
      throw new TypeError("Cannot read properties of undefined (reading 'id')");
    });
    render(
      <EditAction
        label="Edit"
        fields={fields}
        record={record}
        update={update}
        invalidate="people"
        recordKey="person"
        savedMessage="Saved."
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(screen.getByText(en["common.errorNoCause"])).toBeTruthy(),
    );
    expect(screen.queryByText(/Cannot read properties/)).toBeNull();
  });
});

describe("what a save says", () => {
  it("names the record the SERVER returned, not the one the form opened on", async () => {
    // The defect this pins: the message was built at render from the row behind
    // the dialog, so renaming "Alice" to "Alice M" announced "Alice saved" —
    // confidently, and about a name that no longer existed.
    render(
      <EditAction<typeof record>
        label="Edit"
        fields={fields}
        record={record}
        update={async (values) => ({
          ...record,
          full_name: String(values.full_name),
        })}
        invalidate="people"
        recordKey="person"
        savedMessage={(saved) => `${saved.full_name} saved`}
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    const input = screen.getByLabelText("Full name *");
    await userEvent.clear(input);
    await userEvent.type(input, "Alice M");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByRole("status")).toHaveTextContent(
      "Alice M saved",
    );
  });

  it("says nothing when the write was refused", async () => {
    // The confirmation and the refusal come from one choreography, so the arm
    // that must never fire on this path is worth an assertion rather than an
    // assumption.
    render(
      <EditAction
        label="Edit"
        fields={fields}
        record={record}
        update={async () => throwProblem({ detail: "the record is locked" })}
        invalidate="people"
        recordKey="person"
        savedMessage="Saved."
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("the record is locked");
    expect(screen.queryByRole("status")).toBeNull();
  });
});

describe("what an edit is a reading of", () => {
  // The guard this defeats is the whole reason the version column exists.
  //
  // A background refetch mid-edit — another person's save, a websocket
  // invalidation, a window refocus — advances the record the screen renders
  // from while the form's own values stay as the person left them. If the
  // write takes its version from the LIVE record, the server's concurrency
  // check compares the other person's version against itself and passes: the
  // 409 that should have said "somebody changed this while you were editing"
  // cannot fire, and the overwrite lands silently.
  //
  // Both readings are recorded on purpose. Asserting only the frozen one would
  // pass against an implementation that reads the live record, because if the
  // refetch never landed the two are the same — the test has to SEE them
  // differ, or it is not about the race at all.
  function raceScreen(seen: { opened?: unknown; live?: unknown }[]) {
    return function Screen() {
      const [record, setRecord] = useState({
        id: "p1",
        version: 3,
        full_name: "Alice",
      });
      return (
        <>
          <Button
            onClick={() =>
              // Somebody else's save landing under the open dialog.
              setRecord({ id: "p1", version: 9, full_name: "Alice Cooper" })
            }
          >
            refetch
          </Button>
          <EditAction<{ id: string }>
            label="Edit"
            fields={fields}
            record={record}
            savedMessage="saved"
            invalidate="people"
            recordKey="person"
            update={async (_values, _rows, opened) => {
              seen.push({ opened, live: record });
              return { id: "p1" };
            }}
          />
        </>
      );
    };
  }

  async function editThroughARefetch(
    seen: { opened?: unknown; live?: unknown }[],
  ) {
    const Screen = raceScreen(seen);
    render(<Screen />);
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "refetch" }));
    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));
  }

  it("sends the version the form opened on, not the one that arrived while typing", async () => {
    const seen: { opened?: unknown; live?: unknown }[] = [];
    await editThroughARefetch(seen);

    const { opened, live } = seen[0] as {
      opened: { version?: number };
      live: { version?: number };
    };
    // The refetch really landed, so the two readings really do disagree —
    // without this the assertion below could hold for the wrong reason.
    expect(live.version).toBe(9);
    expect(opened.version).toBe(3);
  });

  // The same reading has to be the diff baseline, or an untouched field whose
  // value moved under the dialog reads as this person's edit and is sent —
  // overwriting a change nobody here made.
  // A screen can swap the record under an open dialog without remounting it.
  // The form is then showing one record's
  // values while the caller's write addresses another, and the frozen reading
  // would send the FIRST record's version against the second record's id.
  it("re-reads when the record under the dialog is a different one", async () => {
    const seen: { opened?: unknown; live?: unknown }[] = [];
    function Screen() {
      const [record, setRecord] = useState({
        id: "p1",
        version: 3,
        full_name: "Alice",
      });
      return (
        <>
          <Button
            onClick={() =>
              setRecord({ id: "p2", version: 11, full_name: "Bruno" })
            }
          >
            switch
          </Button>
          <EditAction<{ id: string }>
            label="Edit"
            fields={fields}
            record={record}
            savedMessage="saved"
            invalidate="people"
            recordKey="person"
            update={async (_values, _rows, opened) => {
              seen.push({ opened, live: record });
              return { id: "p1" };
            }}
          />
        </>
      );
    }

    render(<Screen />);
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "switch" }));
    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));

    const { opened } = seen[0] as {
      opened: { id: string; version?: number; full_name?: string };
    };
    // The dialog is now about p2, so everything the write carries must be p2's
    // — an id from one record and a version from another is the mismatch this
    // guards.
    expect(opened.id).toBe("p2");
    expect(opened.version).toBe(11);
    expect(opened.full_name).toBe("Bruno");
  });

  it("compares against the values it prefilled, not the ones that arrived after", async () => {
    const seen: { opened?: unknown; live?: unknown }[] = [];
    await editThroughARefetch(seen);

    const { opened, live } = seen[0] as {
      opened: { full_name?: string };
      live: { full_name?: string };
    };
    expect(live.full_name).toBe("Alice Cooper");
    expect(opened.full_name).toBe("Alice");
  });
});
