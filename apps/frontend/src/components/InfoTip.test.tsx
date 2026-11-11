import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { InfoTip } from "./InfoTip";
import { Field } from "./ui";

describe("InfoTip", () => {
  it("opens on hover and focus, pins on click and closes with Escape", async () => {
    const user = userEvent.setup();
    render(<InfoTip label="Sobre Usuários" text="Papéis somam permissões." />);
    const button = screen.getByRole("button", { name: "Sobre Usuários" });
    expect(screen.queryByRole("tooltip")).toBeNull();

    await user.hover(button);
    expect(screen.getByRole("tooltip")).toHaveTextContent("Papéis somam permissões.");
    expect(button).toHaveAttribute("aria-expanded", "true");
    await user.unhover(button);
    expect(screen.queryByRole("tooltip")).toBeNull();

    await user.click(button);
    await user.unhover(button);
    expect(screen.getByRole("tooltip")).toBeInTheDocument();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("tooltip")).toBeNull();
  });

  it("keeps a field's explanation available to screen readers while closed", () => {
    render(<Field label="Grupo" info="Grupos aparecem recolhidos.">{(p) => <input {...p} />}</Field>);
    expect(screen.getByRole("textbox", { name: "Grupo" })).toHaveAccessibleDescription("Grupos aparecem recolhidos.");
    expect(screen.getByRole("button", { name: "Sobre Grupo" })).toBeInTheDocument();
  });
});
