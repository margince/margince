import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { replySubject } from "./replysubject";

it("matches the server's subjects, including sent-mail follow-ups and repeated prefixes", () => {
  const corpus = readFileSync(
    new URL(
      "../../../backend/internal/modules/activities/testdata/replysubjects.txt",
      import.meta.url,
    ),
    "utf8",
  );
  for (const line of corpus.trimEnd().split("\n")) {
    const [input, expected] = line.split("|");
    expect(input).toBeDefined();
    expect(replySubject(input ?? "")).toBe(expected);
  }
});
