import "./auth-core.css";
import { useEffect } from "react";
import wordmarkDark from "../assets/wordmark-dark.png";
import wordmarkWhite from "../assets/wordmark-white.png";

export function Wordmark({
  alt,
  className = "auth-wordmark",
}: Readonly<{ alt: string; className?: string }>) {
  // CSS hides one image in each theme, so the container owns the name.
  return (
    <span className={className} role="img" aria-label={alt}>
      <img className="auth-wordmark-light" src={wordmarkDark} alt="" />
      <img className="auth-wordmark-dark" src={wordmarkWhite} alt="" />
    </span>
  );
}

export function usePageTitle(title: string) {
  useEffect(() => {
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}
