/** @vitest-environment happy-dom */
import { cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "../i18n";
import { ConnectivityError } from "./connectivity";
import {
  isConsentNotGranted,
  logUnexpectedError,
  ProblemError,
  problemExistingId,
  problemFieldErrors,
  problemFieldErrorsOf,
  problemMessage,
  problemMessageOf,
  throwProblem,
  unwrap,
} from "./problem";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Unconditionally, not at the end of the case that installed a spy. A case
  // that fails its assertion never reaches its own teardown. A leaked console
  // spy then silences the rest of the file, which looks like a suite that passes.
  vi.restoreAllMocks();
});

describe("problemExistingId", () => {
  it("extracts existing_id + code from a duplicate problem", () => {
    expect(
      problemExistingId({
        code: "duplicate_email",
        details: { existing_id: "01ABC" },
      }),
    ).toEqual({ id: "01ABC", code: "duplicate_email" });
  });

  it("returns null when there is no existing_id", () => {
    expect(
      problemExistingId({ code: "duplicate_email", details: {} }),
    ).toBeNull();
    expect(problemExistingId({ title: "nope" })).toBeNull();
    expect(problemExistingId(null)).toBeNull();
  });
});

describe("problemMessage", () => {
  // httperr.Duplicate sends one fixed API sentence for every key it guards,
  // which names neither the field nor the record that holds it.
  it("names what a duplicate refusal collided with, not the API's sentence", () => {
    const message = problemMessage(
      {
        status: 409,
        code: "duplicate_email",
        detail: "a live record with this key already exists",
        details: { existing_id: "01ABC" },
      },
      t,
    );
    expect(message).toBe(
      "Another contact or lead already has this email address, so it was not saved here.",
    );
  });

  // An object-RBAC denial and a read-share denial both arrive with the sentinel
  // string. It names neither the authority the reader holds nor what would
  // widen it. Catalog copy replaces it.
  it("translates a permission_denied refusal instead of echoing the sentinel", () => {
    const message = problemMessage(
      { code: "permission_denied", detail: "permission denied" },
      t,
    );
    expect(message).toBe(t("common.permissionDenied"));
    expect(message).not.toBe("permission denied");
  });

  // One code, two authorities, and no wire field separating them: whatever the
  // server put in `detail`, the reader gets the copy that is true for both. A
  // branch that fell through to the detail for one of them would print a
  // sentence written for the other refusal.
  it("answers both denials with the same copy, whatever the server detailed", () => {
    const objectDenial = problemMessage(
      { code: "permission_denied", detail: "permission denied" },
      t,
    );
    const rowDenial = problemMessage(
      { code: "permission_denied", detail: "contact:update denied" },
      t,
    );
    expect(rowDenial).toBe(objectDenial);
  });

  // The gate's own wrapping, which is the other real producer of this code. Its
  // detail names the admission spec and the resolver state. Those are the
  // authority model's internals, not a sentence for a reader. So the copy
  // stands in for this one too, and none of it reaches the screen.
  it("stands in for the admission gate's detail, and shows none of it", () => {
    const message = problemMessage(
      {
        code: "permission_denied",
        detail:
          "gate: deal.advance: no authority resolver composed: permission denied",
      },
      t,
    );
    expect(message).toBe(t("common.permissionDenied"));
    expect(message).not.toContain("resolver");
  });

  // The wrapped form of the same sentinel, which `auth.Require` sends for every
  // object-RBAC denial. The prefix is the RBAC object and verb, internals no
  // client may be shown. They are no more use to a reader than the sentinel
  // after them, so this is the same nothing, spelled longer.
  it("stands in for a wrapped sentinel too, and never shows the object and verb", () => {
    const message = problemMessage(
      {
        code: "permission_denied",
        detail: "contact.update: permission denied",
      },
      t,
    );
    expect(message).toBe(t("common.permissionDenied"));
    expect(message).not.toContain("contact.update");
  });

  it("keeps the server detail when no translator is given", () => {
    expect(
      problemMessage({
        code: "version_skew",
        detail: "record changed elsewhere",
      }),
    ).toBe("record changed elsewhere");
    expect(
      problemMessage({
        code: "permission_denied",
        detail: "permission denied",
      }),
    ).toBe("permission denied");
  });

  it("keeps the server detail for an unrelated code even with a translator", () => {
    expect(
      problemMessage({ code: "conflict", detail: "name already taken" }, t),
    ).toBe("name already taken");
  });

  it("says a lost version race in the reader's language", () => {
    const german = (key: Parameters<typeof translate>[1]) =>
      translate("de", key);
    expect(
      problemMessage(
        {
          code: "version_skew",
          detail:
            "This record changed after you opened it. Reload it to see the change, then make yours again.",
        },
        german,
      ),
    ).toBe(german("common.versionSkew"));
  });
});

// The 422 shape httperr.Validation emits: the top-level code is always
// "validation_error", so the rule a caller keys on lives only here.
describe("problemFieldErrors", () => {
  it("reads the field, code, and message the server asserted", () => {
    expect(
      problemFieldErrors({
        code: "validation_error",
        detail: "reconnect your mailbox",
        details: {
          errors: [
            {
              field: "from",
              code: "mailbox_not_send_capable",
              message: "reconnect your mailbox",
            },
          ],
        },
      }),
    ).toEqual([
      {
        field: "from",
        code: "mailbox_not_send_capable",
        message: "reconnect your mailbox",
      },
    ]);
  });

  it("reads nothing out of a body that carries no field errors", () => {
    expect(problemFieldErrors({ code: "consent_not_granted" })).toEqual([]);
    expect(
      problemFieldErrors({
        code: "validation_error",
        details: { errors: "not a list" },
      }),
    ).toEqual([]);
    expect(problemFieldErrors(null)).toEqual([]);
    expect(problemFieldErrors("nope")).toEqual([]);
  });

  it("reads nothing out of a problem that is not a validation error", () => {
    // `details` is a free-form extension any problem may carry. A gateway or
    // dependency failure may spell an `errors` array without the server
    // asserting a rule about a submitted field. Reading it as one would turn an
    // unrelated fault into an actionable send refusal.
    expect(
      problemFieldErrors({
        code: "internal",
        details: {
          errors: [
            {
              field: "from",
              code: "mailbox_not_send_capable",
              message: "reconnect your mailbox",
            },
          ],
        },
      }),
    ).toEqual([]);
  });

  it("drops an entry that does not name a field, a code, and a message", () => {
    // A half-formed entry cannot be matched on, and filling its holes with
    // empty strings would let a caller key on a rule nobody asserted.
    expect(
      problemFieldErrors({
        code: "validation_error",
        details: {
          errors: [
            { field: "from", code: "mailbox_not_send_capable" },
            { code: "shared_unsubscribe_token", message: "one at a time" },
            null,
          ],
        },
      }),
    ).toEqual([]);
  });

  it("claims field errors only off a failure that carries a server problem", () => {
    const problem = {
      code: "validation_error",
      details: {
        errors: [{ field: "from", code: "not_send_capable", message: "fix" }],
      },
    };
    let thrown: unknown;
    try {
      throwProblem(problem);
    } catch (error) {
      thrown = error;
    }
    expect(problemFieldErrorsOf(thrown)).toEqual([
      { field: "from", code: "not_send_capable", message: "fix" },
    ]);
    expect(problemFieldErrorsOf(new Error("network down"))).toEqual([]);
    expect(problemFieldErrorsOf(problem)).toEqual([]);
  });
});

describe("unwrap", () => {
  const response = new Response();

  it("answers the data of a call that succeeded", () => {
    expect(unwrap({ data: { id: "c1" }, response })).toEqual({ id: "c1" });
  });

  it("throws the same problem throwProblem does", () => {
    const problem = { code: "not_found", detail: "contact c1 not found" };
    const thrown = (run: () => unknown): unknown => {
      try {
        run();
      } catch (error) {
        return error;
      }
      return undefined;
    };
    const fromUnwrap = thrown(() => unwrap({ error: problem, response }));
    const fromThrowProblem = thrown(() => throwProblem(problem));
    expect(fromUnwrap).toBeInstanceOf(ProblemError);
    expect(fromUnwrap).toEqual(fromThrowProblem);
    expect(fromUnwrap instanceof ProblemError && fromUnwrap.problem).toBe(
      problem,
    );
  });
});

describe("isConsentNotGranted", () => {
  it("detects the consent gate 409 code", () => {
    expect(isConsentNotGranted({ code: "consent_not_granted" })).toBe(true);
    expect(isConsentNotGranted({ code: "version_skew" })).toBe(false);
    expect(isConsentNotGranted(null)).toBe(false);
    expect(isConsentNotGranted("nope")).toBe(false);
  });
});

// The display side of the same rule: what a caught failure is allowed to say.
// A server problem states a cause the reader can act on. A rejected fetch or a
// bug in a handler states our internals in wording nobody wrote for a user. At
// the point of rendering the two look alike unless the type is checked here.
describe("problemMessageOf", () => {
  it("shows the server's own detail when the failure carries a problem", () => {
    expect(
      problemMessageOf(new ProblemError({ detail: "email taken" }), t),
    ).toBe("email taken");
  });

  it("never repeats the words of a bare Error", () => {
    const bug = new TypeError("Cannot read properties of undefined");
    expect(problemMessageOf(bug, t)).toBe(t("common.errorNoCause"));
    expect(problemMessageOf(bug, t)).not.toContain(bug.message);
  });

  it("never repeats the words of a thrown non-Error either", () => {
    for (const thrown of ["boom", { detail: "not a ProblemError" }, null]) {
      expect(problemMessageOf(thrown, t)).toBe(t("common.errorNoCause"));
    }
  });

  it("answers a problem body carrying no reader text with the shared line", () => {
    // A proxy 502, or a refusal the server sent no body with, reaches the
    // client as a ProblemError all the same. "request failed" is the message
    // that error carries into a stack trace. It is a developer's words, in one
    // language, and not a sentence anybody wrote for a reader.
    for (const body of [undefined, {}, "<html>502 Bad Gateway</html>", 42]) {
      expect(problemMessageOf(new ProblemError(body), t)).toBe(
        t("common.errorNoCause"),
      );
    }
  });

  it("keeps the server's words whenever the body carries any", () => {
    // The line above must not become a way to lose a real detail. A body with
    // a detail, or with only a title, still speaks for itself. A blank detail
    // is not words and would render an empty error state, so the reader gets
    // the title behind it.
    expect(
      problemMessageOf(new ProblemError({ title: "Seat expired" }), t),
    ).toBe("Seat expired");
    expect(
      problemMessageOf(
        new ProblemError({ detail: "  ", title: "Seat expired" }),
        t,
      ),
    ).toBe("Seat expired");
  });

  it("says in the reader's language that the host's calendar could not be read", () => {
    const refusal = new ProblemError({
      code: "calendar_unavailable",
      status: 502,
      detail: "The server's English sentence",
    });
    for (const locale of ["en", "de", "vi"] as const) {
      const line = problemMessageOf(refusal, (key) => translate(locale, key));
      expect(line).toBe(translate(locale, "scheduling.calendarUnavailable"));
    }
    expect(problemMessageOf(refusal, t)).not.toBe("Internal Server Error");
  });

  // The wire shape httperr.Validation sends: the top-level code is
  // `validation_error` and the module's own code rides only on the field fault.
  it("says in the reader's language that a win needs its reason, from the field fault's code", () => {
    const refusal = new ProblemError({
      code: "validation_error",
      status: 422,
      detail: "validation failed",
      details: {
        errors: [
          {
            field: "won_without_contract_reason",
            code: "win_evidence_required",
            message:
              "a won deal needs a signed contract with its paper attached, or a reason why there is none (imported, purchase_order)",
          },
        ],
      },
    });
    for (const locale of ["en", "de", "vi"] as const) {
      const line = problemMessageOf(refusal, (key) => translate(locale, key));
      expect(line).toBe(translate(locale, "deals.winEvidenceRequired"));
      expect(line).not.toContain("purchase_order");
    }
  });

  it("prefers the surface's own copy for a failure the server never described", () => {
    expect(
      problemMessageOf(
        new Error("Failed to fetch"),
        t,
        t("settings.saveFailed"),
      ),
    ).toBe(t("settings.saveFailed"));
    // A server problem still speaks for itself. The fallback is for the case
    // where there is nothing to say, not a way to overwrite the server.
    expect(
      problemMessageOf(
        new ProblemError({ detail: "budget exhausted" }),
        t,
        t("settings.saveFailed"),
      ),
    ).toBe("budget exhausted");
  });
});

describe("problemMessageOf, for a request that never reached Margince", () => {
  const refused = (
    outage: "offline" | "unreachable",
    method: string,
    unsent = false,
  ) =>
    new ConnectivityError(
      outage,
      new Request("https://test.local/v1/notes", { method }),
      new TypeError("Failed to fetch"),
      unsent,
    );

  it("says a write the device never sent was not saved", () => {
    expect(problemMessageOf(refused("offline", "POST", true), t)).toBe(
      t("connectivity.unsaved.offline"),
    );
  });

  // A connection cut after the request left may still have landed it, and a
  // blind retry would then write it twice.
  it("says a write cut off in flight may have been saved, and to check first", () => {
    expect(problemMessageOf(refused("offline", "POST"), t)).toBe(
      t("connectivity.uncertain.offline"),
    );
    expect(problemMessageOf(refused("unreachable", "PATCH"), t)).toBe(
      t("connectivity.uncertain.unreachable"),
    );
  });

  it("names the outage over a surface's own copy, which cannot know it", () => {
    expect(
      problemMessageOf(
        refused("unreachable", "DELETE"),
        t,
        t("settings.saveFailed"),
      ),
    ).toBe(t("connectivity.uncertain.unreachable"));
  });

  it("leaves a read on the ordinary line: it saved nothing and runs again", () => {
    expect(problemMessageOf(refused("offline", "GET", true), t)).toBe(
      t("common.errorNoCause"),
    );
    expect(
      problemMessageOf(refused("offline", "GET"), t, t("settings.saveFailed")),
    ).toBe(t("settings.saveFailed"));
  });
});

// The other half of that rule. Hiding a failure from the reader is only sound
// if the failure still exists somewhere an operator can read.
describe("logUnexpectedError", () => {
  it("keeps a failure the reader is only shown generic copy for", () => {
    const logged = vi.spyOn(console, "error").mockImplementation(() => {});
    const bug = new TypeError("Cannot read properties of undefined");

    logUnexpectedError(bug);

    // The value itself, once: a message string would drop the stack that is
    // the only reason anyone opens the console for this.
    expect(logged).toHaveBeenCalledTimes(1);
    expect(logged).toHaveBeenCalledWith(bug);
  });

  it("stays silent for a failure the reader can already read", () => {
    const logged = vi.spyOn(console, "error").mockImplementation(() => {});

    logUnexpectedError(new ProblemError({ detail: "no seat" }));

    expect(logged).not.toHaveBeenCalled();
  });
});
