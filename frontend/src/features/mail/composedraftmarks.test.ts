import { describe, expect, it } from "vitest";
import {
  holdsServedWords,
  MarksMemory,
  oneKeystroke,
  REMEMBERED_BODIES,
} from "./composedraftmarks";

const MARKS = {
  draftRef: "dr-1",
  provenance: null,
  reasoning: [],
  scope: undefined,
  language: null,
};

describe("the remembered bodies", () => {
  it("keeps the last ones by their plain text, and forgets the oldest past the bound", () => {
    const memory = new MarksMemory();
    for (let index = 0; index <= REMEMBERED_BODIES; index += 1) {
      memory.remember(`Body ${index}`, MARKS);
    }

    expect(memory.marksOf("Body 0")).toBeUndefined();
    expect(memory.marksOf(`  Body   ${REMEMBERED_BODIES} `)).toEqual(MARKS);
    expect(memory.marksOf("Body 1")).toEqual(MARKS);
  });

  it("tells the reader's own words, remembered without marks, from a new body", () => {
    const memory = new MarksMemory();
    memory.remember("My own words.", null);

    expect(memory.marksOf("My own words.")).toBeNull();
    expect(memory.marksOf("Other words.")).toBeUndefined();
  });
});

describe("the served words in a body", () => {
  const served = "Hello Anna,\n\nThanks for the call. Here is where we stand.";

  it("are held when most of them stand there", () => {
    expect(holdsServedWords(served.replace("call", "meeting"), served)).toBe(
      true,
    );
  });

  it("are not held by a shared greeting alone", () => {
    expect(holdsServedWords("Hello Anna,\n\nMy own words.", served)).toBe(
      false,
    );
  });

  it("are not held by the same words in another order", () => {
    expect(
      holdsServedWords(
        "We stand where the call is. Here, Anna, thanks for hello.",
        served,
      ),
    ).toBe(false);
  });

  it("are never matched from a draft too short to have a passage", () => {
    expect(holdsServedWords("Thanks for everything so far.", "Thanks")).toBe(
      false,
    );
  });

  it("are held in decomposed text the way they were served composed", () => {
    const vietnamese = "Xin chào chị, cảm ơn cuộc gọi hôm nay.";

    expect(holdsServedWords(vietnamese.normalize("NFD"), vietnamese)).toBe(
      true,
    );
  });
});

describe("one keystroke", () => {
  it("is one character typed or deleted anywhere in the body", () => {
    expect(oneKeystroke("Thanks", "Thank")).toBe(true);
    expect(oneKeystroke("Tanks", "Thanks")).toBe(true);
    expect(oneKeystroke("", "T")).toBe(true);
    expect(oneKeystroke("Thanks", "Thanks 👍")).toBe(false);
    expect(oneKeystroke("Thanks ", "Thanks 👍")).toBe(true);
  });

  it("is not a jump of several characters, as an undo makes", () => {
    expect(oneKeystroke("", "Thanks")).toBe(false);
    expect(oneKeystroke("Thanks", "Thinks")).toBe(false);
  });
});
