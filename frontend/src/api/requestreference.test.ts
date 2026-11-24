/** @vitest-environment happy-dom */
import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { ProblemError, problemMessageOf } from "../lib/problem";
import { withRequestReference } from "./requestreference";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

function problemAnswer(headers: Record<string, string>, body: string) {
  return new Response(body, {
    status: 409,
    headers: { "Content-Type": "application/problem+json", ...headers },
  });
}

describe("withRequestReference", () => {
  it("copies the X-Request-Id of a failed answer into its problem body", async () => {
    const answer = await withRequestReference(
      problemAnswer({ "X-Request-Id": "01JC-abc" }, '{"detail":"email taken"}'),
    );
    expect(await answer.json()).toEqual({
      detail: "email taken",
      request_id: "01JC-abc",
    });
    expect(answer.status).toBe(409);
  });

  it("leaves an answer with no id, a success and a non-problem body alone", async () => {
    const noId = problemAnswer({}, '{"detail":"x"}');
    expect(await withRequestReference(noId)).toBe(noId);
    const ok = new Response("{}", {
      status: 200,
      headers: { "X-Request-Id": "01JC" },
    });
    expect(await withRequestReference(ok)).toBe(ok);
    const broken = problemAnswer({ "X-Request-Id": "01JC" }, "<html>");
    expect(await withRequestReference(broken)).toBe(broken);
  });
});

describe("problemMessageOf with a request id", () => {
  it("ends the message with the reference a reader can report", () => {
    const error = new ProblemError({
      detail: "email taken",
      request_id: "01JC-abc",
    });
    expect(problemMessageOf(error, t)).toBe("email taken Reference: 01JC-abc");
  });

  it("adds no reference to a problem that has none", () => {
    expect(
      problemMessageOf(new ProblemError({ detail: "email taken" }), t),
    ).toBe("email taken");
  });
});
