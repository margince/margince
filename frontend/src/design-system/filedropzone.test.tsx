// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { FileDropzone } from "./filedropzone";

// The two ways in, and the one way out. A drop zone that only works with a
// mouse silently excludes every keyboard and screen-reader user, so the input
// is the control and the zone is chrome around it — which is also why these
// tests drive both paths.

afterEach(cleanup);

function show(onPick: (file: File) => void, file?: File) {
  return render(
    <LocaleProvider initial="en">
      <FileDropzone
        label="Document"
        hint="Up to 25 MB."
        emptyLabel="Drop the file here, or click to choose one"
        file={file}
        onPick={onPick}
      />
    </LocaleProvider>,
  );
}

function showMany(onPick: (file: File) => void, files: readonly File[] = []) {
  return render(
    <LocaleProvider initial="en">
      <FileDropzone
        label="Document"
        emptyLabel="Drop the files here, or click to choose some"
        multiple
        files={files}
        onPick={onPick}
      />
    </LocaleProvider>,
  );
}

const ORDER_FORM = () =>
  new File(["EUR 148,500.00"], "order_form.txt", { type: "text/plain" });

/** The box that carries the highlight. */
function zone() {
  const found = document.querySelector(".fdz");
  if (!found) {
    throw new Error("the dropzone did not render");
  }
  return found;
}

/** Where a drag actually lands: the input is stretched over the whole zone, and
 * it is the only element here with handlers on it. */
function target() {
  return screen.getByLabelText("Document");
}

describe("choosing a file", () => {
  it("takes a file from the picker", async () => {
    const user = userEvent.setup();
    const picked = vi.fn();
    show(picked);

    await user.upload(screen.getByLabelText(/Document/), ORDER_FORM());

    expect(picked).toHaveBeenCalledTimes(1);
    expect(picked.mock.calls[0][0].name).toBe("order_form.txt");
  });

  it("takes a file that was dropped on the zone", () => {
    const picked = vi.fn();
    show(picked);

    fireEvent.drop(target(), { dataTransfer: { files: [ORDER_FORM()] } });

    expect(picked).toHaveBeenCalledTimes(1);
    expect(picked.mock.calls[0][0].name).toBe("order_form.txt");
  });

  it("admits it will accept the file while one is held over it", () => {
    show(vi.fn());

    expect(zone().className).not.toContain("dragover");
    fireEvent.dragOver(target(), { dataTransfer: { files: [] } });
    // The CLASS is all this can prove: jsdom loads no stylesheet, so whether
    // anything is drawn for it is the story's job, not this one's. The old
    // screen-local copy toggled the same class with no rule behind it anywhere,
    // and a test at this level could not have told the difference.
    expect(zone().className).toContain("dragover");

    fireEvent.dragLeave(target());
    expect(zone().className).not.toContain("dragover");
  });

  it("leaves the current choice alone when a drop carries no file", () => {
    const picked = vi.fn();
    show(picked, ORDER_FORM());

    fireEvent.drop(target(), { dataTransfer: { files: [] } });

    // Cancelling a picker is not the act of clearing a field. Treating it as
    // one would discard a file the reader had already chosen.
    expect(picked).not.toHaveBeenCalled();
    expect(screen.getByText("order_form.txt")).toBeTruthy();
  });

  it("takes the SAME file again after the caller cleared the field", async () => {
    const user = userEvent.setup();
    const picked = vi.fn();
    show(picked);
    const input = screen.getByLabelText(/Document/);

    await user.upload(input, ORDER_FORM());
    await user.upload(input, ORDER_FORM());

    // A browser fires no change event when the same path is chosen twice in a
    // row, so the input's value is cleared after every pick. Without that, the
    // second choice is silently inert — and choosing the same file again is
    // exactly what a caller invites when it clears the field after a half-
    // failed upload.
    expect(picked).toHaveBeenCalledTimes(2);
  });

  it("names the control after its field, and never after the file in it", async () => {
    const user = userEvent.setup();
    show(vi.fn(), ORDER_FORM());
    const input = screen.getByLabelText("Document");

    // A <label> wrapping the zone would name the input a SECOND time and fold
    // the state text into it, so the control would announce as "Document
    // order_form.txt" — the value baked into the name, changing on every pick.
    expect(input.getAttribute("type")).toBe("file");
    expect(screen.queryByLabelText(/order_form/)).toBeNull();
    await user.upload(input, ORDER_FORM());
    expect(screen.getByLabelText("Document")).toBe(input);
  });

  it("shows the chosen file's name instead of the invitation", () => {
    show(vi.fn(), ORDER_FORM());

    expect(screen.getByText("order_form.txt")).toBeTruthy();
    expect(
      screen.queryByText("Drop the file here, or click to choose one"),
    ).toBeNull();
  });

  // Dropping a folder's worth of files at once is the point of asking for
  // several. Taking only the first would look like it worked and quietly file
  // one of ten.
  it("takes EVERY file when the caller asked for several", () => {
    const picked = vi.fn();
    showMany(picked);

    fireEvent.drop(target(), {
      dataTransfer: {
        files: [
          new File(["a"], "capture.md", { type: "text/markdown" }),
          new File(["b"], "records.md", { type: "text/markdown" }),
          new File(["c"], "settings.md", { type: "text/markdown" }),
        ],
      },
    });

    expect(picked).toHaveBeenCalledTimes(3);
    expect(picked.mock.calls.map((call) => call[0].name)).toEqual([
      "capture.md",
      "records.md",
      "settings.md",
    ]);
  });

  // The single-file contract is unchanged by the new option: a browser hands a
  // FileList to every drop whatever the input says, so a caller with one slot
  // must still be given exactly one.
  it("still takes only the first when the caller did not ask for several", () => {
    const picked = vi.fn();
    show(picked);

    fireEvent.drop(target(), {
      dataTransfer: {
        files: [
          ORDER_FORM(),
          new File(["b"], "second.txt", { type: "text/plain" }),
        ],
      },
    });

    expect(picked).toHaveBeenCalledTimes(1);
    expect(picked.mock.calls[0][0].name).toBe("order_form.txt");
  });

  it("names every held file, so a short drop is visible before it is sent", () => {
    showMany(vi.fn(), [
      new File(["a"], "capture.md", { type: "text/markdown" }),
      new File(["b"], "records.md", { type: "text/markdown" }),
    ]);

    expect(screen.getByText("capture.md, records.md")).toBeTruthy();
  });

  // The zone is used for several kinds of file now, and a caller that narrows
  // the picker has no other way to tell whether it worked: a wrong or missing
  // `accept` still opens a dialog, just the wrong one.
  it("narrows the picker to what the caller asked for", () => {
    render(
      <LocaleProvider initial="en">
        <FileDropzone
          label="Document"
          emptyLabel="Choose a .vcf file"
          accept=".vcf,text/vcard"
          onPick={vi.fn()}
        />
      </LocaleProvider>,
    );

    expect(target().getAttribute("accept")).toBe(".vcf,text/vcard");
  });

  it("offers every file when the caller narrowed nothing", () => {
    show(vi.fn());

    expect(target().hasAttribute("accept")).toBe(false);
  });
});
