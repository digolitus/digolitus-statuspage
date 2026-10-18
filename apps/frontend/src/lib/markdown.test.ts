import { describe, expect, it } from "vitest";
import { fromDom, parse, plainText, toHtml } from "./markdown";

const roundTrip = (md: string) => {
  const el = document.createElement("div");
  el.innerHTML = toHtml(md);
  return fromDom(el);
};

describe("markdown dialect", () => {
  it("parses blocks and inline formatting", () => {
    const blocks = parse("## Título\n\ntexto **b** *i* ++u++ ~~s~~ ==m== `c`\n\n1. um\n2. dois\n\n> cit\n\n-# nota\n\n---");
    expect(blocks.map((b) => b.t)).toEqual(["h2", "p", "ol", "quote", "small", "hr"]);
    expect(toHtml("texto **b** *i* ++u++ ~~s~~ ==m== `c`")).toBe(
      "<p>texto <strong>b</strong> <em>i</em> <u>u</u> <s>s</s> <mark>m</mark> <code>c</code></p>",
    );
  });

  it("never produces markup from text and links only http(s)", () => {
    const html = toHtml('<img src=x onerror="alert(1)"> [x](javascript:alert(1)) [ok](https://a.example.com)');
    expect(html).not.toContain("<img");
    expect(html).toContain("&lt;img");
    expect(html).not.toContain('href="javascript');
    expect(html).toContain('<a href="https://a.example.com">ok</a>');
  });

  it("does not treat snake_case or arithmetic as italics", () => {
    expect(toHtml("snake_case_name e 2 * 3 * 4")).toBe("<p>snake_case_name e 2 * 3 * 4</p>");
  });

  it("round-trips through the visual editor DOM", () => {
    const md = "## Título\n\nlinha **forte** e [link](https://x.example.com)\n\n### Sub\n\n- a\n- b\n\n1. um\n2. dois\n\n> citação\n\n-# pequeno\n\n---\n\nfim";
    expect(roundTrip(md)).toBe(md);
  });

  it("converts browser markup and keeps literal markers literal", () => {
    const el = document.createElement("div");
    el.innerHTML = "<div><b>forte</b> <i>it</i> <u>sub</u> <strike>x</strike> 2*3</div><div>- não é lista</div><div>1. não numerada</div>";
    const md = fromDom(el);
    expect(md).toBe("**forte** *it* ++sub++ ~~x~~ 2\\*3\n\n\\- não é lista\n\n1\\. não numerada");
    expect(toHtml(md)).toBe("<p><strong>forte</strong> <em>it</em> <u>sub</u> <s>x</s> 2*3</p><p>- não é lista</p><p>1. não numerada</p>");
  });

  it("builds plain text without markers", () => {
    expect(plainText("## Oi\n\n**negrito** [site](https://x.example.com)\n- a")).toBe("Oi negrito site • a");
  });
});
