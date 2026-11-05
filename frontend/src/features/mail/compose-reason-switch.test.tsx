// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import {
  allowedPreview,
  refusedPreview,
} from "../../testing/sendpermission.testkit";
import { ComposeModal } from "./compose";
import { jsonResponse, pickWhy, render, stubRoutes } from "./compose.testkit";
import { alternativeReasonOf } from "./composereason";

// A refused reason answered with the one the record already carries.
//
// The engine names, per recipient, the other reasons that would carry the
// message. The composer offers one press that switches the reason and asks
// again. It records nothing and sends nothing.

const ANNA = "anna@acme.test";
const SWITCH = "Use “Follow-up they requested” instead";

type Sent = ReturnType<typeof stubRoutes>;

function claimOf(body: unknown): unknown {
  return (body as { communication_context?: unknown } | null)
    ?.communication_context;
}

// The deal claim is refused with the follow-up named; the follow-up passes.
function openComposer(alternatives: string[]) {
  let sent: Sent = [];
  sent = stubRoutes({
    "POST /emails:preview": () =>
      jsonResponse(
        claimOf(sent.at(-1)?.body) === "requested_followup"
          ? allowedPreview([ANNA])
          : refusedPreview(ANNA, {
              verdict: "review",
              reason_code: "no_compatible_evidence",
              decided_by: "machine",
              can_be_overruled: true,
              resolved_category: "requested_followup",
              requested_category: "active_deal_followup",
              supported_alternatives: alternatives as never,
            }),
      ),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={() => {}}
    />,
  );
  return () => sent;
}

async function writeAndClaimTheDeal() {
  await userEvent.type(screen.getByLabelText("To"), ANNA);
  await userEvent.tab();
  await userEvent.type(screen.getByLabelText("Subject"), "Your question");
  writeMessage("Body", "Here is the answer.");
  await pickWhy("Active deal");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("switching to the reason the record carries", () => {
  it("switches the reason and asks again, without sending", async () => {
    const sent = openComposer(["requested_followup"]);
    await writeAndClaimTheDeal();

    await userEvent.click(await screen.findByRole("button", { name: SWITCH }));

    await waitFor(() =>
      expect(
        claimOf(
          sent()
            .filter((s) => s.key === "POST /emails:preview")
            .at(-1)?.body,
        ),
      ).toBe("requested_followup"),
    );
    expect(screen.queryByRole("button", { name: SWITCH })).toBeNull();
    expect(sent().some((s) => s.key === "POST /emails")).toBe(false);
    expect(sent().some((s) => s.key.includes("overrule"))).toBe(false);
  });

  it("offers no switch when the record carries no other reason", async () => {
    openComposer([]);
    await writeAndClaimTheDeal();

    expect(
      await screen.findByText("Choose the deal for this follow-up."),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: SWITCH })).toBeNull();
  });
});

describe("alternativeReasonOf", () => {
  const refused = (address: string, supported?: string[]) => ({
    address,
    verdict: "review" as const,
    reason_code: "no_compatible_evidence",
    decided_by: "machine" as const,
    would_refuse: true,
    can_be_overruled: true,
    supported_alternatives: supported as never,
  });
  const ask = (recipients: ReturnType<typeof refused>[]) => ({
    preview: { allowed: false, recipients },
    asking: false,
    unanswered: false,
  });

  it("offers only a reason every refused recipient shares", () => {
    expect(
      alternativeReasonOf(
        ask([
          refused("a@x.test", ["requested_followup", "precontract_quote"]),
          refused("b@x.test", ["precontract_quote"]),
        ]),
      ),
    ).toBe("precontract_quote");
    expect(
      alternativeReasonOf(
        ask([refused("a@x.test", ["requested_followup"]), refused("b@x.test")]),
      ),
    ).toBeUndefined();
  });

  it("offers nothing when another recipient is already allowed", () => {
    const allowed = {
      ...refused("b@x.test", []),
      verdict: "allow" as const,
      reason_code: "allowed",
      would_refuse: false,
    };
    expect(
      alternativeReasonOf(
        ask([refused("a@x.test", ["requested_followup"]), allowed]),
      ),
    ).toBeUndefined();
  });

  it("offers nothing while the answer is out or failed", () => {
    const answer = ask([refused("a@x.test", ["requested_followup"])]);
    expect(alternativeReasonOf({ ...answer, asking: true })).toBeUndefined();
    expect(
      alternativeReasonOf({ ...answer, unanswered: true }),
    ).toBeUndefined();
  });
});
