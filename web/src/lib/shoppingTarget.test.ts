import { describe, expect, it } from "vitest";
import { noTarget, targetChoice, type MqttStatus } from "./shoppingTarget";

const home = { id: "todo.bring_zuhause", name: "Zuhause" };
const flat = { id: "todo.bring_wg", name: "WG" };

function status(
  targets: MqttStatus["targets"],
  target: MqttStatus["target"],
): MqttStatus {
  return { status: "connected", targets, target };
}

describe("targetChoice", () => {
  it("lists Keine and the offered lists by name and selects the chosen one", () => {
    expect(targetChoice(status([home, flat], flat))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause" },
        { value: "todo.bring_wg", label: "WG" },
      ],
      selected: "todo.bring_wg",
    });
  });

  it("shows the name of the offer for a chosen list renamed in Home Assistant", () => {
    const stored = { id: "todo.bring_zuhause", name: "Daheim" };
    expect(targetChoice(status([home], stored))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause" },
      ],
      selected: "todo.bring_zuhause",
    });
  });

  it("keeps a chosen list that is not offered as not available and selected", () => {
    expect(targetChoice(status([flat], home))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_wg", label: "WG" },
        { value: "todo.bring_zuhause", label: "Zuhause (nicht verfügbar)" },
      ],
      selected: "todo.bring_zuhause",
    });
  });

  it("selects Keine without a chosen list", () => {
    expect(targetChoice(status([home, flat], null))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause" },
        { value: "todo.bring_wg", label: "WG" },
      ],
      selected: noTarget,
    });
  });

  it("offers only Keine for an empty offer without a chosen list", () => {
    expect(targetChoice(status([], null))).toEqual({
      options: [{ value: noTarget, label: "Keine" }],
      selected: noTarget,
    });
  });

  it("keeps the chosen list as not available for an empty offer", () => {
    expect(targetChoice(status([], home))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause (nicht verfügbar)" },
      ],
      selected: "todo.bring_zuhause",
    });
  });

  it("adds the id to offered lists that share a name", () => {
    const otherHome = { id: "todo.bring_zuhause_2", name: "Zuhause" };
    expect(targetChoice(status([home, flat, otherHome], otherHome))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause (todo.bring_zuhause)" },
        { value: "todo.bring_wg", label: "WG" },
        { value: "todo.bring_zuhause_2", label: "Zuhause (todo.bring_zuhause_2)" },
      ],
      selected: "todo.bring_zuhause_2",
    });
  });

  it("keeps a not offered chosen list apart from an offered list of the same name", () => {
    const oldHome = { id: "todo.alt", name: "Zuhause" };
    expect(targetChoice(status([home], oldHome))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause" },
        { value: "todo.alt", label: "Zuhause (nicht verfügbar)" },
      ],
      selected: "todo.alt",
    });
  });

  it("counts only the first of offered lists with the same id", () => {
    const sameId = { id: "todo.bring_zuhause", name: "Daheim" };
    expect(targetChoice(status([home, sameId], null))).toEqual({
      options: [
        { value: noTarget, label: "Keine" },
        { value: "todo.bring_zuhause", label: "Zuhause" },
      ],
      selected: noTarget,
    });
  });
});
