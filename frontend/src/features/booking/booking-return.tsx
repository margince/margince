import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { useBookingTitle } from "./booking-common";

// Where the link an erased subject asked for lands. Pressing the button, not
// opening the page, stores them again: a mail scanner that opens the link
// stores nothing.
export function BookingReturnScreen({
  hostSlug,
  token,
}: Readonly<{ hostSlug: string; token: string }>) {
  const t = useT();
  useBookingTitle(t("book.returnTitle"));
  const confirm = useMutation({
    mutationFn: async () => {
      const { error } = await api.POST(
        "/public/booking/{host_slug}/return/confirm",
        { params: { path: { host_slug: hostSlug } }, body: { token } },
      );
      if (error) throwProblem(error);
    },
  });
  return (
    <div className="book-guest-page">
      <div className="book-guest-column">
        <Panel title={t("book.returnTitle")}>
          <PanelBody>
            {confirm.isSuccess ? (
              <Callout
                tone="success"
                kind="outcome"
                title={t("book.returnDone")}
              />
            ) : (
              <>
                <p>{t("book.returnBody")}</p>
                {confirm.isError && (
                  <Callout
                    tone="warning"
                    kind="outcome"
                    title={t("book.returnSpent")}
                  />
                )}
                <Button
                  variant="primary"
                  disabled={confirm.isPending || confirm.isError}
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
