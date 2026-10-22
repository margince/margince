// What a refused rep may do: overrule a machine refusal with their own reason.
//
// The vouch is recorded on each refused contact for the send's category, and it
// stands for later sends of that kind until it is revoked. It does not send:
// the rep presses Send again, and the engine reads the vouch.

import { useMutation } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import type { paths } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Field } from "../../design-system/field";
import { Stack } from "../../design-system/stack";
import { useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import type { OverrulableRecipient, SendReview } from "./sendreview";

type OverruleRequest = Readonly<{
  overrulable: readonly OverrulableRecipient[];
  reason: string;
  // Called after each vouch commits. Each is its own write, so a retry after a
  // failure must skip the ones already recorded rather than record them twice.
  onRecorded: (recipient: OverrulableRecipient) => void;
}>;

// The category arrives as a server string. The door's own enum check refuses
// one it does not know, so the cast cannot widen what is accepted.
type AllowCategory =
  paths["/contacts/{id}/consent/allow"]["post"]["requestBody"]["content"]["application/json"]["category"];

// One write for every place a rep records a reason: the refusal after Send
// and the warning bar before it.
export async function recordOverrule({
  overrulable,
  reason,
  onRecorded,
}: OverruleRequest): Promise<void> {
  for (const { contactId, category } of overrulable) {
    const { error, response } = await api.POST("/contacts/{id}/consent/allow", {
      params: { path: { id: contactId } },
      body: { category: category as AllowCategory, reason },
    });
    if (!response.ok) {
      throwProblem(error || { title: "The overrule could not be recorded." });
    }
    onRecorded({ contactId, category });
  }
}

const recipientKey = (r: OverrulableRecipient) =>
  `${r.contactId}:${r.category}`;

// SendReviewActions draws the overrule when the server offers it.
//
// Nothing is drawn when there is nothing to offer. A refusal nobody may
// overrule (the contact objected, the address bounced) gets no control rather
// than a disabled one.
export function SendReviewActions({
  review,
}: Readonly<{ review: SendReview }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const recorded = useRef(new Set<string>());
  const overrule = useMutation({ mutationFn: recordOverrule });

  if (!review.actions.includes("overrule") || review.overrulable.length === 0) {
    return null;
  }
  if (overrule.isSuccess) {
    return (
      <p className="t-body" role="status">
        {t("compose.overrule.recorded")}
      </p>
    );
  }
  if (!open) {
    return (
      <Button variant="ghost" onClick={() => setOpen(true)}>
        {t("compose.overrule.open")}
      </Button>
    );
  }
  const trimmed = reason.trim();
  return (
    <Stack gap="2">
      <Field
        label={t("compose.overrule.reasonLabel")}
        hint={t("compose.overrule.hint")}
        required
      >
        {(control) => (
          <textarea
            {...control}
            className="input"
            rows={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        )}
      </Field>
      <Button
        onClick={() =>
          overrule.mutate({
            overrulable: review.overrulable.filter(
              (r) => !recorded.current.has(recipientKey(r)),
            ),
            reason: trimmed,
            onRecorded: (r) => recorded.current.add(recipientKey(r)),
          })
        }
        disabled={trimmed === ""}
        pending={overrule.isPending}
        busyLabel={t("compose.overrule.recording")}
      >
        {t("compose.overrule.confirm")}
      </Button>
      {overrule.isError && (
        <ErrorLine>{t("compose.overrule.failed")}</ErrorLine>
      )}
    </Stack>
  );
}

// ReviewReference names the work the refusal left behind.
//
// It is the fallback every other branch leans on. When the overrule fails, the
// copy tells the rep to open the review, which only works with the reference on
// screen. When the server offers an action this build cannot draw, dropping it
// is only safe because the reference survives.
export function ReviewReference({ review }: Readonly<{ review: SendReview }>) {
  const t = useT();
  return (
    <span className="t-caption">
      {t("compose.reviewReference")}: {review.reviewId}
    </span>
  );
}
