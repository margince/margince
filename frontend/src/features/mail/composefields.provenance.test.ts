/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { DraftProvenance } from "./composedraftcall";
import { emptyFields, useComposeFields } from "./composefields";

// The model's words and their disclosure travel together: an undo that brings
// the words back over an emptied body brings the disclosure back too.

const DRAFTED = "Hello Anna,\n\nThanks for the call. Here is where we stand.";
const MARKS: DraftProvenance = {
  ai_generated: true,
  ai_disclosure: "Written with AI assistance.",
  voice_profile_version: 2,
  voice_degraded: false,
};

function drafted() {
  const view = renderHook(() => useComposeFields({}));
  act(() => {
    view.result.current.adoptDraftedBody(DRAFTED);
    view.result.current.setServedBody(DRAFTED);
    view.result.current.setDraftRef("dr-1");
    view.result.current.setProvenance(MARKS);
  });
  return view;
}

const edit = (text: string) => ({ text, html: text ? `<p>${text}</p>` : "" });

afterEach(cleanup);

describe("the provenance of drafted words", () => {
  it("steps aside while the body is empty and returns with the drafted words", () => {
    const view = drafted();

    act(() => view.result.current.editBody(edit("")));
    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();

    act(() => view.result.current.editBody(edit(DRAFTED)));
    expect(view.result.current.provenance).toEqual(MARKS);
    expect(view.result.current.draftRef).toBe("dr-1");
  });

  it("returns with an edited version that still holds the model's words", () => {
    const view = drafted();
    act(() => view.result.current.editBody(edit("")));

    act(() =>
      view.result.current.editBody(
        edit(`${DRAFTED}\n\nI will send the contract tomorrow.`),
      ),
    );

    expect(view.result.current.provenance).toEqual(MARKS);
  });

  it("stays away from words the reader wrote in place of the draft", () => {
    const view = drafted();
    act(() => view.result.current.editBody(edit("")));

    act(() => view.result.current.editBody(edit("Dear Anna, my own words.")));

    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();
  });

  it("comes back without the reference of a rejected draft", () => {
    const view = drafted();
    act(() => view.result.current.clearRejected());
    expect(view.result.current.provenance).toBeNull();

    act(() => view.result.current.editBody(edit(DRAFTED)));

    expect(view.result.current.provenance).toEqual(MARKS);
    expect(view.result.current.draftRef).toBeNull();
  });

  it("stays away from the reader's own words that share the draft's greeting", () => {
    const view = drafted();
    act(() => view.result.current.editBody(edit("")));

    act(() =>
      view.result.current.editBody(edit("Hello Anna,\n\nMy own words.")),
    );

    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();
  });

  it("stays on the drafted words while the reader edits them", () => {
    const view = drafted();

    act(() =>
      view.result.current.editBody(
        edit(DRAFTED.replace("where we stand", "the plan")),
      ),
    );

    expect(view.result.current.provenance).toEqual(MARKS);
    expect(view.result.current.draftRef).toBe("dr-1");
  });

  it("leaves the reader's words when an undo takes a rewrite back to them", () => {
    const view = renderHook(() => useComposeFields({}));
    const own = "Anna, can we talk about the price?";
    act(() => view.result.current.editBody(edit(own)));
    act(() => {
      view.result.current.adoptDraftedBody(DRAFTED);
      view.result.current.setServedBody(DRAFTED);
      view.result.current.setDraftRef("dr-1");
      view.result.current.setProvenance(MARKS);
    });

    act(() => view.result.current.editBody(edit(own)));
    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();

    act(() => view.result.current.editBody(edit(DRAFTED)));
    expect(view.result.current.provenance).toEqual(MARKS);
    expect(view.result.current.draftRef).toBe("dr-1");
  });

  it("follows an undo two drafts deep back to the reader's own words", () => {
    const view = renderHook(() => useComposeFields({}));
    const own = "Anna, can we talk about the price?";
    const shorter = "Hello Anna,\n\nWhere we stand.";
    const draft = (text: string, ref: string) =>
      act(() => {
        view.result.current.adoptDraftedBody(text);
        view.result.current.setServedBody(text);
        view.result.current.setDraftRef(ref);
        view.result.current.setProvenance(MARKS);
      });
    act(() => view.result.current.editBody(edit(own)));
    draft(DRAFTED, "dr-1");
    draft(shorter, "dr-2");

    act(() => view.result.current.editBody(edit(DRAFTED)));
    expect(view.result.current.draftRef).toBe("dr-1");
    act(() => view.result.current.editBody(edit(own)));
    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();

    act(() => view.result.current.editBody(edit(DRAFTED)));
    expect(view.result.current.draftRef).toBe("dr-1");
    act(() => view.result.current.editBody(edit(shorter)));
    expect(view.result.current.draftRef).toBe("dr-2");
  });

  it("comes back when one undo restores an edited draft that was deleted key by key", () => {
    const view = drafted();
    const edited = DRAFTED.replace("call", "meeting");
    act(() => view.result.current.editBody(edit(edited)));
    for (let left = edited.length - 1; left >= 0; left -= 1) {
      act(() => view.result.current.editBody(edit(edited.slice(0, left))));
    }
    expect(view.result.current.provenance).toBeNull();

    act(() => view.result.current.editBody(edit(edited)));

    expect(view.result.current.provenance).toEqual(MARKS);
    expect(view.result.current.draftRef).toBe("dr-1");
  });

  it("stays away from a letter typed again after the draft was deleted key by key", () => {
    const view = drafted();
    for (let left = DRAFTED.length - 1; left >= 0; left -= 1) {
      act(() => view.result.current.editBody(edit(DRAFTED.slice(0, left))));
    }

    act(() => view.result.current.editBody(edit("H")));

    expect(view.result.current.provenance).toBeNull();
    expect(view.result.current.draftRef).toBeNull();
  });

  it("comes back with most of the drafted words pasted over an emptied body", () => {
    const view = drafted();
    act(() => view.result.current.editBody(edit("")));

    act(() =>
      view.result.current.editBody(
        edit("Thanks for the call. Here is where we stand."),
      ),
    );

    expect(view.result.current.provenance).toEqual(MARKS);
  });

  it("marks only the model's words as a body the reader may undo", () => {
    const view = renderHook(() => useComposeFields({}));
    const before = view.result.current.undoableRevision;

    act(() => view.result.current.editBody(edit("My words.")));
    act(() => view.result.current.load(emptyFields()));
    expect(view.result.current.undoableRevision).toBe(before);

    act(() => view.result.current.adoptDraftedBody(DRAFTED));
    expect(view.result.current.undoableRevision).not.toBe(before);
  });
});

describe("text a recipient line refused", () => {
  it("holds the send until a switch of conversation, a restore or a new transport", () => {
    const view = renderHook(() => useComposeFields({}));
    const refuse = () =>
      act(() => view.result.current.refuseAddresses("cc", ["nope"]));

    refuse();
    expect(view.result.current.addressRefused).toBe(true);
    act(() => view.result.current.load(emptyFields()));
    expect(view.result.current.addressRefused).toBe(false);

    refuse();
    act(() =>
      view.result.current.restoreSaved({
        to: [],
        cc: [],
        bcc: [],
        subject: "",
        body: "",
        html: "",
        files: [],
        context: "",
        sendAt: "",
      }),
    );
    expect(view.result.current.addressRefused).toBe(false);

    refuse();
    act(() => view.result.current.clearForTransport());
    expect(view.result.current.addressRefused).toBe(false);
  });
});
