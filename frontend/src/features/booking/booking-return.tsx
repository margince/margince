import { useMutation, useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { useBookingTitle } from "./booking-common";

type Preview = components["schemas"]["BookingReturnPreview"];

// Where the link an erased subject asked for lands. It shows the name and the
// consent wording the link carries. It stores them only when the button is
// pressed, so a mail scanner that opens the link stores nothing.
export function BookingReturnScreen({
  hostSlug,
  token,
}: Readonly<{ hostSlug: string; token: string }>) {
  const t = useT();
  useBookingTitle(t("book.returnTitle"));
  const link = { params: { path: { host_slug: hostSlug } }, body: { token } };
  const preview = useQuery({
    queryKey: ["booking-return", hostSlug, token],
    retry: false,
    queryFn: async (): Promise<Preview> => {
      const { data, error } = await api.POST(
        "/public/booking/{host_slug}/return/preview",
        link,
      );
      if (error) throwProblem(error);
      return data;
    },
  });
  const confirm = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST(
        "/public/booking/{host_slug}/return/confirm",
        link,
      );
      if (error) throwProblem(error);
    },
  });
  const spent = preview.isError || confirm.isError;
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <Panel title={t("book.returnTitle")}>
          <PanelBody>
            {confirm.isSuccess && (
              <Callout
                tone="success"
                kind="outcome"
                title={t("book.returnDone")}
              />
            )}
            {spent && (
              <Callout
                tone="warning"
                kind="outcome"
                title={t("book.returnSpent")}
              />
            )}
            {preview.data && !confirm.isSuccess && !spent && (
              <>
                <p>{t("book.returnBody")}</p>
                <p>{t("book.returnStoredAs", { name: preview.data.name })}</p>
                <blockquote>{preview.data.wording}</blockquote>
                <Button
                  variant="primary"
                  disabled={confirm.isPending}
                  onClick={() => confirm.mutate()}
                >
                  {t("book.returnAction")}
                </Button>
              </>
            )}
          </PanelBody>
        </Panel>
      </div>
    </div>
  );
}
