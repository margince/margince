// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useBookingTitle } from "./booking-common";
import { throwBookingProblem } from "./booking-errors";

type ReturnRequest = Readonly<{ name: string; email: string }>;

// Where support sends an erased contact who wants to be stored again. No page
// links here. The answer is the same for every address, so it reveals nothing.
export function BookingReturnRequestScreen({
  hostSlug,
}: Readonly<{ hostSlug: string }>) {
  const t = useT();
  useBookingTitle(t("book.returnTitle"));
  const [details, setDetails] = useState<ReturnRequest>({
    name: "",
    email: "",
  });
  const request = useMutation({
    mutationFn: async (body: ReturnRequest) => {
      const { error, response } = await api.POST(
        "/public/booking/{host_slug}/return",
        { params: { path: { host_slug: hostSlug } }, body },
      );
      // This address asked already today. That is an answer to show calmly,
      // not a failure.
      if (response.status === 429) return "recent";
      if (error) throwBookingProblem(error, t);
      return "sent";
    },
  });
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <Panel title={t("book.returnTitle")}>
          <PanelBody>
            {request.data === "sent" ? (
              <Callout
                tone="success"
                kind="outcome"
                title={t("book.returnRequestSent")}
              />
            ) : (
              <form
                className="book-form"
                onSubmit={(event: FormEvent) => {
                  event.preventDefault();
                  request.mutate(details);
                }}
              >
                <p>{t("book.returnRequestBody")}</p>
                <Field label={t("book.name")}>
                  {(control) => (
                    <TextInput
                      {...control}
                      required
                      maxLength={200}
                      autoComplete="name"
                      value={details.name}
                      onChange={(e) =>
                        setDetails({ ...details, name: e.target.value })
                      }
                    />
                  )}
                </Field>
                <Field label={t("book.email")}>
                  {(control) => (
                    <TextInput
                      {...control}
                      required
                      type="email"
                      autoComplete="email"
                      value={details.email}
                      onChange={(e) =>
                        setDetails({ ...details, email: e.target.value })
                      }
                    />
                  )}
                </Field>
                <div className="book-actions">
                  <Button
                    type="submit"
                    variant="primary"
                    pending={request.isPending}
                  >
                    {t("book.returnRequestAction")}
                  </Button>
                </div>
                {request.data === "recent" && (
                  <Callout
                    tone="info"
                    kind="outcome"
                    title={t("book.returnRequestRecent")}
                  />
                )}
                <ErrorLine error={request.isError && request.error} />
              </form>
            )}
          </PanelBody>
        </Panel>
      </div>
    </div>
  );
}
