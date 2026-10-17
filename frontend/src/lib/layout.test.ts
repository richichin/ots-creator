import { describe, expect, it } from "vitest";
import { CENTER_MIN, LIST_MIN, PROPS_MIN, dragList, dragProps, maxListWidth, minLayoutWidth } from "./layout";

describe("panel widths", () => {
  it("grows and shrinks with the drag", () => {
    expect(dragList(1400, 300, 280, 100)).toBe(400);
    expect(dragList(1400, 300, 280, -100)).toBe(LIST_MIN);
    expect(dragProps(1400, 300, 280, 100)).toBe(220);
    expect(dragProps(1400, 300, 280, -100)).toBe(380);
  });

  it("never takes the preview below its minimum", () => {
    const room = 1200;
    const widest = maxListWidth(room, 280);
    expect(widest).toBe(room - 280 - CENTER_MIN - 24);
    expect(dragList(room, 300, 280, 2000)).toBe(widest);
    expect(dragProps(room, 300, 280, -2000)).toBe(room - 300 - CENTER_MIN - 24);
  });

  it("keeps a minimum even in a tiny window", () => {
    expect(dragList(400, 300, 280, -100)).toBe(LIST_MIN);
    expect(dragProps(400, 300, 280, 100)).toBe(PROPS_MIN);
  });

  it("sums to the minimum layout width", () => {
    expect(minLayoutWidth(300, 280)).toBe(300 + 280 + CENTER_MIN + 24);
  });
});