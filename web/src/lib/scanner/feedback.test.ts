import { expect, it } from "vitest";
import { TONES } from "./feedback";

it("defines the tone patterns from the plan", () => {
  expect(TONES).toStrictEqual({
    add: [{ hz: 880, ms: 100 }],
    consume: [{ hz: 660, ms: 100 }],
    warn: [
      { hz: 440, ms: 80 },
      { hz: 440, ms: 80 },
    ],
    error: [{ hz: 220, ms: 300 }],
  });
});
