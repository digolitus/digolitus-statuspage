import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";

/**
 * Discreet "i" that holds explanatory text, so visible copy stays short.
 * Opens on hover, keyboard focus or tap; closes with Escape, a second tap
 * or a tap elsewhere. The bubble flips to the left when it would overflow
 * the viewport.
 *
 * `label` names what the tip explains ("Sobre Usuários"); it becomes the
 * button's accessible name.
 */
export function InfoTip({ label, text, className }: { label: string; text: ReactNode; className?: string }) {
  const [pinned, setPinned] = useState(false);
  const [hover, setHover] = useState(false);
  const [alignEnd, setAlignEnd] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);
  const bubble = useRef<HTMLSpanElement>(null);
  const tipId = useId();
  const open = pinned || hover;

  useEffect(() => {
    if (!open) return;
    const close = () => {
      setPinned(false);
      setHover(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    const onDown = (e: MouseEvent | TouchEvent) => ref.current && !ref.current.contains(e.target as Node) && close();
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("touchstart", onDown);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("touchstart", onDown);
    };
  }, [open]);

  // Measure from the anchor, not the bubble, so the result doesn't depend on
  // the alignment chosen the last time the tip was open.
  useLayoutEffect(() => {
    if (!open || !ref.current || !bubble.current) return;
    const overflows = ref.current.getBoundingClientRect().left + bubble.current.offsetWidth > document.documentElement.clientWidth - 16;
    setAlignEnd(overflows);
  }, [open]);

  return (
    <span
      className={`info-tip${className ? ` ${className}` : ""}`}
      ref={ref}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
    >
      <button
        type="button"
        className="info-tip-button"
        aria-label={label}
        aria-expanded={open}
        aria-describedby={open ? tipId : undefined}
        onClick={() => setPinned((p) => !p)}
        onFocus={() => setHover(true)}
        onBlur={() => setHover(false)}
      >
        <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true">
          <circle cx="8" cy="8" r="7" fill="none" stroke="currentColor" strokeWidth="1.4" />
          <circle cx="8" cy="4.9" r="1" fill="currentColor" />
          <path d="M8 7.3v4.4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
        </svg>
      </button>
      {open && (
        <span id={tipId} ref={bubble} role="tooltip" className={`info-tip-bubble${alignEnd ? " info-tip-bubble-end" : ""}`}>
          {text}
        </span>
      )}
    </span>
  );
}

/**
 * Content followed by an InfoTip on the same line. Wrap a whole heading
 * with `block` (the tip stays outside the heading, so its accessible name
 * is just the title); wrap inline text such as a legend without it.
 */
export function WithInfo({ children, label, info, block }: { children: ReactNode; label: string; info?: ReactNode; block?: boolean }) {
  if (!info) return <>{children}</>;
  const Tag = block ? "div" : "span";
  return (
    <Tag className="with-info">
      {children}
      <InfoTip label={label} text={info} />
    </Tag>
  );
}
