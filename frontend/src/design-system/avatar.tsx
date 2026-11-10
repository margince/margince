import { useState } from "react";
import { meshOf, meshStyle, monogramOf } from "./avatarmesh";
import "./avatar.css";

export function Avatar({
  name,
  identity,
  src,
  photo,
  size = "sm",
}: Readonly<{
  name: string;
  /**
   * The record's own id (a company's, a contact's, a seat's user id). Two
   * surfaces keying one record on different strings draw it in two colours.
   * A site with no id passes the name and says why. An empty key (a payload
   * missing its id) falls back to the name, not to one colour for all.
   */
  identity: string;
  // A resolved logo to render instead of the monogram. The monogram is the
  // floor, not the fallback of last resort. It shows while the image loads,
  // if it fails to load, and whenever no logo resolved. So a company is never
  // a broken image or an empty slot.
  src?: string | null;
  // A seat's own picture, in place of `src`: it covers the whole
  // circle inside a hairline, so a white picture still has an edge.
  photo?: string | null;
  /**
   * The four sizes: `sm` every list row, `md` a record header, `lg` a wide
   * one, `xl` a record page's own mark. They replace four numbers in four
   * stylesheets for a prop that admitted two. `sm` is the floor: a dense table
   * brings its chips down from its own sheet, density being its decision.
   */
  size?: "sm" | "md" | "lg" | "xl";
}>) {
  const picture = photo || src;
  // An image that fails to load falls back to the monogram for the rest of
  // this mount. Keyed by src so a record whose logo changes gets a fresh try
  // rather than inheriting the previous one's failure.
  const [brokenSrc, setBrokenSrc] = useState<string | null>(null);
  const broken = Boolean(picture) && brokenSrc === picture;
  const setBroken = () => setBrokenSrc(picture ?? null);
  // The monogram is the floor under the mark. It has to stop being drawn the
  // moment the mark is on screen, or a logo with transparency shows the
  // initials through it. Tracked by src for the same reason as the failure
  // above.
  const [paintedSrc, setPaintedSrc] = useState<string | null>(null);
  // A mark that painted once and then failed on a later load is no longer on
  // screen, so it stops holding the monogram down. Without the `!broken`, the
  // image is removed while the fallback stays suppressed. The avatar is then
  // empty.
  const painted = Boolean(picture) && paintedSrc === picture && !broken;
  const initials = monogramOf(name);
  // A chip with a live logo draws no mesh at any moment. The initials wait on
  // a neutral ground, and only a logo that failed falls back to the mesh.
  const logo = Boolean(picture) && !broken;
  // A seat waits for its picture on the mesh a contact without one shows.
  const meshed = !logo || (Boolean(photo) && !painted);
  const classes = [
    "avatar",
    logo ? "avatar-has-logo" : "",
    meshed ? "avatar-mesh" : "",
    logo && photo ? "avatar-photo" : "",
    `avatar-${size}`,
  ].filter(Boolean);
  return (
    <span
      className={classes.join(" ")}
      style={meshed ? meshStyle(meshOf(identity || name)) : undefined}
    >
      {logo ? (
        // The monogram stays underneath: it is what the chip shows until the
        // image paints, and what is left if the image never does.
        <img
          className="avatar-img"
          src={picture ?? undefined}
          alt=""
          loading="lazy"
          onError={setBroken}
          onLoad={() => setPaintedSrc(picture ?? null)}
        />
      ) : null}
      {!painted && initials}
    </span>
  );
}
