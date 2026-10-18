import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";

// jsdom does not implement scrolling; navigation calls it.
window.scrollTo = () => undefined;

afterEach(() => {
  vi.restoreAllMocks();
  window.history.pushState(null, "", "/");
  try {
    window.localStorage.clear();
  } catch {
    // Storage unavailable: nothing to reset.
  }
});
