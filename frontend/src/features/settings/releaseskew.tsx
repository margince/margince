import { AuthCardTitle, AuthExperience } from "../../components/auth-core";
import { usePageTitle, Wordmark } from "../../components/auth-identity";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";

export { useSkewedApiRelease } from "../../lib/capabilities";

/**
 * ReleaseSkewScreen is what a reader sees instead of the app.
 *
 * It names both releases. Most readers cannot act on the numbers, but this
 * screen exists only for a broken deployment, and the one contact who can fix it
 * needs to know which of the two images is the odd one out — asking them to open
 * a console for it would be withholding the only fact on the page that matters.
 *
 * Reload is offered because it is genuinely the fix for the common case: a tab
 * held open across a deploy is running an old bundle, and fetching the document
 * again picks up the current one. It cannot fix a torn pull, which is why the
 * copy says what to do when it does not.
 */
export function ReleaseSkewScreen({
  app,
  server,
}: Readonly<{ app: string; server: string }>) {
  const t = useT();
  usePageTitle(t("release.skewTitle"));
  return (
    <AuthExperience phase="unavailable">
      <Wordmark alt={t("auth.title")} />
      <section className="auth-card" role="alert">
        <AuthCardTitle>{t("release.skewTitle")}</AuthCardTitle>
        <p>{t("release.skewBody")}</p>
        <p>{t("release.skewVersions", { app, server })}</p>
        <div className="auth-actions">
          <Button variant="primary" onClick={() => window.location.reload()}>
            {t("release.skewReload")}
          </Button>
        </div>
      </section>
    </AuthExperience>
  );
}
