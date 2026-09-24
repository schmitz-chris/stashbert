import type { components } from "./api/schema";

/**
 * The connection to the MQTT broker, the lists offered by Home Assistant
 * and the chosen list (architecture.md, 11.7).
 */
export type MqttStatus = components["schemas"]["MqttStatus"];

/** An option of the select "Liste in Home Assistant". */
export interface TargetOption {
  /** The id of the list, or noTarget for "Keine". */
  value: string;
  label: string;
}

/** The options of the select "Liste in Home Assistant" and its value. */
export interface TargetChoice {
  options: TargetOption[];
  selected: string;
}

/**
 * The value of the option "Keine": no list chosen. The id of a list is
 * never empty (architecture.md, 11.7).
 */
export const noTarget = "";

/**
 * Returns the options of the select "Liste in Home Assistant" for status
 * and the selected value. The options are "Keine", then the offered lists
 * in their order, by name. Offered lists that share a name get their id in
 * parentheses, like "Zuhause (todo.bring_zuhause)", so they can be told
 * apart. Of offered lists with the same id only the first counts, like on
 * the server. A chosen list that is not offered right now comes last as
 * "<Name> (nicht verfügbar)". selected is the id of the chosen list, or
 * noTarget if none is chosen.
 */
export function targetChoice(status: MqttStatus): TargetChoice {
  const offered = status.targets.filter(
    (target, index) =>
      status.targets.findIndex((other) => other.id === target.id) === index,
  );
  const nameCount = new Map<string, number>();
  for (const { name } of offered) {
    nameCount.set(name, (nameCount.get(name) ?? 0) + 1);
  }

  const options: TargetOption[] = [
    { value: noTarget, label: "Keine" },
    ...offered.map(({ id, name }) => ({
      value: id,
      label: (nameCount.get(name) ?? 0) > 1 ? `${name} (${id})` : name,
    })),
  ];
  const { target } = status;
  if (target !== null && !offered.some(({ id }) => id === target.id)) {
    options.push({ value: target.id, label: `${target.name} (nicht verfügbar)` });
  }
  return { options, selected: target?.id ?? noTarget };
}
