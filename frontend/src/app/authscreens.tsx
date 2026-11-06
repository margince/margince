import { type ComponentProps, Suspense } from "react";
import { lazy } from "../i18n/lazy";
import { AuthSplash } from "./bootsplash";

const SignIn = lazy(async () => ({
  default: (await import("../features/sign-in/auth")).AuthScreen,
}));
const Availability = lazy(async () => ({
  default: (await import("../features/sign-in/auth")).AvailabilityScreen,
}));
const Setup = lazy(async () => ({
  default: (await import("../features/sign-in/setupclaim")).SetupClaimScreen,
}));

export function AuthScreen(props: ComponentProps<typeof SignIn>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <SignIn {...props} />
    </Suspense>
  );
}
export function AvailabilityScreen(props: ComponentProps<typeof Availability>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <Availability {...props} />
    </Suspense>
  );
}
export function SetupClaimScreen(props: ComponentProps<typeof Setup>) {
  return (
    <Suspense fallback={<AuthSplash />}>
      <Setup {...props} />
    </Suspense>
  );
}
