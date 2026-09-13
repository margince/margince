/** @vitest-environment happy-dom */
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
import type { CreateField } from "./create";
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
        invalidate="contacts"
        recordKey="contact"
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
        invalidate="contacts"
        recordKey="contact"
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
    { key: "title", label: "create.contactTitle" as const },
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
        invalidate="contacts"
        recordKey="contact"
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
        invalidate="contacts"
        recordKey="contact"
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
        invalidate="contacts"
        recordKey="contact"
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
        invalidate="contacts"
        recordKey="contact"
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
  // A background refetch mid-edit — another contact's save, a websocket
  // invalidation, a window refocus — advances the record the screen renders
  // from while the form's own values stay as the contact left them. If the
  // write takes its version from the LIVE record, the server's concurrency
  // check compares the other contact's version against itself and passes: the
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
            invalidate="contacts"
            recordKey="contact"
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
  // value moved under the dialog reads as this contact's edit and is sent —
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
            invalidate="contacts"
            recordKey="contact"
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
  // The custom-field catalog is a SECOND request, and every screen builds this
  // modal's record through `cf.recordSlice`, which projects the catalog's own
  // column list off the row. So before the catalog answers, the record the
  // modal is handed carries no cf_ key AT ALL — not a blank one, an absent
  // one. These tests build the record that way rather than putting the column
  // straight on it: a record that already carries the value sidesteps the
  // projection, which is the exact thing that makes the bug real.
  function recordSlice(
    row: Record<string, unknown>,
    catalog: { key: string }[],
  ): Record<string, unknown> {
    const slice: Record<string, unknown> = {};
    for (const field of catalog) {
      slice[field.key] = row[field.key];
    }
    return slice;
  }

  const TIER: CreateField = { key: "cf_tier", labelText: "Tier", type: "text" };
  // Stored in MINOR units, rendered in major — the one conversion a form does.
  const BUDGET: CreateField = {
    key: "cf_budget",
    labelText: "Budget",
    type: "number",
    toInput: (raw) =>
      raw == null || raw === "" ? "" : String(Number(raw) / 100),
  };

  // One screen shaped like the real ones: a row that always holds the stored
  // values, a catalog that starts empty, and a record built by projecting the
  // catalog over the row.
  function LateCatalogScreen({
    catalog,
    onUpdate,
    row,
  }: Readonly<{
    catalog: CreateField[];
    // Handed the form's answers AND the reading they are diffed against —
    // a test about the baseline cannot see it from the values alone.
    onUpdate: (
      values: Record<string, unknown>,
      opened?: Record<string, unknown>,
    ) => void;
    row: Record<string, unknown>;
  }>) {
    const [known, setKnown] = useState<CreateField[]>([]);
    return (
      <>
        <Button onClick={() => setKnown(catalog)}>catalog</Button>
        <EditAction<{ id: string }>
          label="Edit"
          fields={[...fields, ...known]}
          record={{
            ...record,
            ...recordSlice(row, known),
          }}
          savedMessage="saved"
          invalidate="contacts"
          recordKey="contact"
          update={async (values, _rows, opened) => {
            onUpdate(values, opened);
            return { id: "p1" };
          }}
        />
      </>
    );
  }

  it("seeds a field the catalog only named after the dialog opened", async () => {
    const seen: Record<string, unknown>[] = [];
    render(
      <LateCatalogScreen
        catalog={[TIER]}
        row={{ cf_tier: "Strategic" }}
        onUpdate={(values) => seen.push(values)}
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "catalog" }));

    // The control carries the STORED value. Seeding from the reading the form
    // opened on would put "" here, because that reading predates the catalog
    // and so has no cf_tier key to read.
    await waitFor(() =>
      expect(screen.getByLabelText("Tier")).toHaveValue("Strategic"),
    );
    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));
    expect(seen[0]?.cf_tier).toBe("Strategic");
  });

  it("converts a late field's stored units the way the first seed would", async () => {
    // Currency is stored in minor units and shown in major. A late field must
    // go through the same `toInput` as one the form had from the start, or the
    // reader is shown 10000 where the record means 100.
    render(
      <LateCatalogScreen
        catalog={[BUDGET]}
        row={{ cf_budget: 10000 }}
        onUpdate={() => {}}
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "catalog" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Budget")).toHaveValue(100),
    );
  });

  it("leaves what the reader typed alone when the catalog lands", async () => {
    render(
      <LateCatalogScreen
        catalog={[TIER]}
        row={{ cf_tier: "Strategic" }}
        onUpdate={() => {}}
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    const name = screen.getByLabelText(en["create.fullName"], { exact: false });
    await userEvent.clear(name);
    await userEvent.type(name, "Alice Cooper");
    await userEvent.click(screen.getByRole("button", { name: "catalog" }));

    await waitFor(() =>
      expect(screen.getByLabelText("Tier")).toHaveValue("Strategic"),
    );
    // The newcomer is seeded; the answer already on screen is not re-read.
    expect(name).toHaveValue("Alice Cooper");
  });

  it("carries a clear of a late-seeded field into the patch", async () => {
    // The write diffs the form against the reading it opened on. A field the
    // opening reading never carried — the catalog had not answered yet — would
    // otherwise make a genuine clear compare EQUAL to the absent baseline
    // (customFieldFormValue normalises null and "" alike) and drop out of the
    // body, while the save reports success.
    //
    // Asserts the patch CARRIES the key, not that the column ends up empty:
    // coerceWrite turns a cleared field into null and the backend refuses null
    // on a cf_ column, which is a separate, pre-existing gap.
    const seen: { values: Record<string, unknown>; opened?: unknown }[] = [];
    render(
      <LateCatalogScreen
        catalog={[TIER]}
        row={{ cf_tier: "Strategic" }}
        onUpdate={(values, opened) => seen.push({ values, opened })}
      />,
    );
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(screen.getByRole("button", { name: "catalog" }));
    await waitFor(() =>
      expect(screen.getByLabelText("Tier")).toHaveValue("Strategic"),
    );

    await userEvent.clear(screen.getByLabelText("Tier"));
    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));

    const { values, opened } = seen[0] as {
      values: { cf_tier?: string };
      opened: Record<string, unknown>;
    };
    // The form submits the cleared string, and the baseline it is diffed
    // against holds the STORED value — so the two differ and the clear is a
    // real change rather than a no-op.
    expect(values.cf_tier).toBe("");
    expect(opened.cf_tier).toBe("Strategic");
  });

  it("reopens on the new reading after a refetch while closed", async () => {
    // values are NOT cleared on close — only reseeded on open. So on a REOPEN
    // the additive pass sees the previous session's answers as present, skips
    // them, and merges them over the seed the transition just queued. The form
    // then submits a stale name against the CURRENT version, which is the
    // version the server checks: a lost update that passes concurrency.
    const seen: { values: Record<string, unknown>; opened?: unknown }[] = [];
    function Screen() {
      const [live, setLive] = useState({
        id: "p1",
        version: 1,
        full_name: "Alice",
      });
      const [known, setKnown] = useState<CreateField[]>([]);
      return (
        <>
          <Button
            onClick={() => {
              setLive({ id: "p1", version: 2, full_name: "Updated" });
              setKnown([TIER]);
            }}
          >
            refetch
          </Button>
          <EditAction<{ id: string }>
            label="Edit"
            fields={[...fields, ...known]}
            record={{
              ...live,
              ...(known.length ? { cf_tier: "Strategic" } : {}),
            }}
            savedMessage="saved"
            invalidate="contacts"
            recordKey="contact"
            update={async (values, _rows, opened) => {
              seen.push({ values, opened });
              return { id: "p1" };
            }}
          />
        </>
      );
    }

    render(<Screen />);
    // A first session, opened and closed — this is what leaves values behind.
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(
      screen.getByRole("button", { name: en["create.cancel"] }),
    );
    // The record moves on while the dialog is closed.
    await userEvent.click(screen.getByRole("button", { name: "refetch" }));
    // Reopen: the form must show the NEW reading, not the old session's.
    await userEvent.click(screen.getByTestId("edit-record"));
    await waitFor(() =>
      expect(
        screen.getByLabelText(en["create.fullName"], { exact: false }),
      ).toHaveValue("Updated"),
    );

    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));
    const { values, opened } = seen[0] as {
      values: { full_name?: string };
      opened: { version?: number };
    };
    // One reading: a v2 version must not be paired with v1's name.
    expect(values.full_name).toBe("Updated");
    expect(opened.version).toBe(2);
  });

  it("opens on one reading when the record changed while it was closed", async () => {
    // Both seeds run on the open transition. The second must not compute from
    // the values the first is replacing — React has not applied that setter
    // yet — or the form submits an old field value against the new version.
    const seen: { values: Record<string, unknown>; opened?: unknown }[] = [];
    function Screen() {
      const [live, setLive] = useState({
        id: "p1",
        version: 1,
        full_name: "Alice",
      });
      return (
        <>
          <Button
            onClick={() =>
              setLive({ id: "p1", version: 2, full_name: "Updated" })
            }
          >
            refetch
          </Button>
          <EditAction<{ id: string }>
            label="Edit"
            fields={fields}
            record={live}
            savedMessage="saved"
            invalidate="contacts"
            recordKey="contact"
            update={async (values, _rows, opened) => {
              seen.push({ values, opened });
              return { id: "p1" };
            }}
          />
        </>
      );
    }

    render(<Screen />);
    // Refetch BEFORE opening, so the transition seeds from the new reading.
    await userEvent.click(screen.getByRole("button", { name: "refetch" }));
    await userEvent.click(screen.getByTestId("edit-record"));
    await userEvent.click(
      screen.getByRole("button", { name: en["record.save"] }),
    );
    await waitFor(() => expect(seen).toHaveLength(1));

    const { values, opened } = seen[0] as {
      values: { full_name?: string };
      opened: { version?: number };
    };
    // One reading: the name and the version describe the same record.
    expect(values.full_name).toBe("Updated");
    expect(opened.version).toBe(2);
  });
});
