import { type ReactNode, lazy as reactLazy } from "react";
import { CompleteLocale } from "./index";

export function lazy<Props extends object>(
  load: () => Promise<{ default: (props: Props) => ReactNode }>,
) {
  return reactLazy(async () => {
    const { default: View } = await load();
    return {
      default: (props: Props) => (
        <CompleteLocale>
          <View {...props} />
        </CompleteLocale>
      ),
    };
  });
}
