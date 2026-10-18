import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { countUnseen, NEW_WINDOW_MS } from "../lib/announcementsSeen";
import { excerpt, RichText } from "./RichText";

describe("RichText", () => {
  it("renders paragraphs, lists, bold and only http(s) links, without HTML injection", () => {
    const { container } = render(
      <RichText text={"Olá **mundo**\nlinha 2\n\n- um\n- [docs](https://d.example.com)\n\n<img src=x onerror=alert(1)> [x](javascript:alert(1)) https://a.example.com/p."} />,
    );
    expect(container.querySelectorAll("p")).toHaveLength(2);
    expect(container.querySelector("strong")?.textContent).toBe("mundo");
    expect(container.querySelectorAll("li")).toHaveLength(2);
    const links = [...container.querySelectorAll("a")].map((a) => a.getAttribute("href"));
    expect(links).toEqual(["https://d.example.com", "https://a.example.com/p"]);
    expect(container.querySelector("img")).toBeNull();
    expect(container.textContent).toContain("<img src=x onerror=alert(1)>");
  });

  it("builds plain excerpts", () => {
    expect(excerpt("**Oi** [site](https://x.example.com)\n- a", 100)).toBe("Oi site • a");
    expect(excerpt("a".repeat(20), 10)).toHaveLength(10);
  });
});

describe("countUnseen", () => {
  const now = Date.parse("2026-03-10T12:00:00Z");
  it("counts publications after the last seen one", () => {
    expect(countUnseen(["2026-03-10T11:00:00Z", "2026-03-09T00:00:00Z"], "2026-03-09T00:00:00Z", now)).toBe(1);
  });
  it("without a record, counts only recent publications", () => {
    const old = new Date(now - NEW_WINDOW_MS - 1000).toISOString();
    expect(countUnseen(["2026-03-10T11:00:00Z", old], null, now)).toBe(1);
  });
});
