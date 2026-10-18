import { useEffect, useState, type AnchorHTMLAttributes, type MouseEvent } from "react";

// A minimal client-side router on the History API. The app has a handful of
// flat routes, so a routing library would add more than it saves. Nginx and
// the Vite dev server fall back to index.html for unknown paths.

const NAVIGATE_EVENT = "app:navigate";

export function navigate(to: string, replace = false): void {
  if (replace) {
    window.history.replaceState(null, "", to);
  } else {
    window.history.pushState(null, "", to);
  }
  window.dispatchEvent(new Event(NAVIGATE_EVENT));
  window.scrollTo?.(0, 0);
}

export interface Location {
  pathname: string;
  search: URLSearchParams;
}

function current(): Location {
  return { pathname: window.location.pathname, search: new URLSearchParams(window.location.search) };
}

export function useLocation(): Location {
  const [location, setLocation] = useState(current);
  useEffect(() => {
    const update = () => setLocation(current());
    window.addEventListener("popstate", update);
    window.addEventListener(NAVIGATE_EVENT, update);
    return () => {
      window.removeEventListener("popstate", update);
      window.removeEventListener(NAVIGATE_EVENT, update);
    };
  }, []);
  return location;
}

/** Matches "/admin/incidents/:id" style patterns; returns params or null. */
export function match(pattern: string, pathname: string): Record<string, string> | null {
  const p = pattern.split("/").filter(Boolean);
  const s = pathname.replace(/\/+$/, "").split("/").filter(Boolean);
  if (p.length !== s.length) {
    return null;
  }
  const params: Record<string, string> = {};
  for (let i = 0; i < p.length; i++) {
    if (p[i].startsWith(":")) {
      params[p[i].slice(1)] = decodeURIComponent(s[i]);
    } else if (p[i] !== s[i]) {
      return null;
    }
  }
  return params;
}

type LinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & { to: string };

/** An anchor that navigates without reloading; modifier clicks behave natively. */
export function Link({ to, onClick, ...rest }: LinkProps) {
  const handle = (event: MouseEvent<HTMLAnchorElement>) => {
    onClick?.(event);
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    navigate(to);
  };
  return <a href={to} onClick={handle} {...rest} />;
}
