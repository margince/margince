import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { useToast } from "../design-system/toast";
import { useTooltip } from "../design-system/tooltip";
import { VisibilityLine } from "../design-system/visibility";
import { useT } from "../i18n";
import { isVersionSkewOf, problemMessageOf, throwProblem } from "./common";
import "./contactaccess.css";

type Contact = components["schemas"]["Contact"];

/** Visibility stays beside the owner even when the details rail is closed. */
export function ContactAccess({ contact }: Readonly<{ contact: Contact }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const descriptionId = useId();
  const setVisibility = useMutation({
    mutationFn: async ({
      id,
      version,
      visibility,
    }: {
      id: string;
      version: Contact["version"];
      visibility: "workspace" | "owner";
    }) => {
      const { error } = await api.PATCH("/contacts/{id}", {
        params: {
          path: { id },
          ...ifMatch(requireVersion(version)),
        },
        body: { visibility },
      });
      if (error) {
        throwProblem(error);
      }
      return { id, visibility };
    },
    onError: async (error, { id }) => {
      if (isVersionSkewOf(error)) {
        await queryClient.invalidateQueries({ queryKey: ["contact360", id] });
      }
    },
    onSuccess: async ({ id, visibility }) => {
      toast.show(
        visibility === "workspace"
          ? t("contactAccess.published")
          : t("contactAccess.madePrivate"),
      );
      await queryClient.invalidateQueries({ queryKey: ["contact360", id] });
    },
  });

  if (!contact.visibility) {
    return null;
  }
  const isPrivate = contact.visibility === "owner";
  const description = t(
    isPrivate ? "contactAccess.privateToYou" : "contactAccess.company",
  );
  const mayChange = Boolean(contact.writable) && !contact.archived_at;
  return (
    <section className="contact-access" aria-label={t("contactAccess.title")}>
      {/* The mark explains itself on hover and on focus: what "Only you"
          means, and that the verb beside it is how to change it. A native
          title reached a pointer alone and only after a pause. */}
      <AccessTip
        text={
          isPrivate ? t("contactAccess.privateTip") : t("contactAccess.company")
        }
      >
        <VisibilityLine
          state={isPrivate ? "private" : "team"}
          action={
            mayChange && (
              <Button
                variant="link"
                className="contact-access-action"
                aria-describedby={descriptionId}
                pending={setVisibility.isPending}
                onClick={() =>
                  setVisibility.mutate({
                    id: contact.id,
                    version: contact.version,
                    visibility: isPrivate ? "workspace" : "owner",
                  })
                }
              >
                {t(
                  isPrivate
                    ? "contactAccess.share"
                    : "contactAccess.makePrivate",
                )}
              </Button>
            )
          }
        />
      </AccessTip>
      <span id={descriptionId} className="sr-only">
        {description}
      </span>
      {setVisibility.isError && (
        <span role="alert" className="form-error">
          {isVersionSkewOf(setVisibility.error)
            ? t("edit.versionSkew")
            : problemMessageOf(setVisibility.error, t)}
        </span>
      )}
    </section>
  );
}

// The tooltip's anchor: one span around the line, so the badge and the verb
// share the one explanation rather than each hanging its own.
function AccessTip({
  text,
  children,
}: Readonly<{ text: string; children: ReactNode }>) {
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(text);
  return (
    <span className="contact-access-tip" ref={ref} {...trigger}>
      {children}
      {tip}
    </span>
  );
}
