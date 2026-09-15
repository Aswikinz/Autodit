import { describe, it, expect } from "vitest";
import { label, message, dateTime, result } from "./client";
describe("audit presentation contracts", () => {
  it("keeps state vocabulary readable", () => {
    expect(label("resolved_in_source")).toBe("Resolved In Source");
    expect(dateTime(null)).toBe("Not yet");
  });
  it("surfaces server failures instead of a successful empty queue", async () => {
    await expect(
      result(
        Promise.resolve({
          response: new Response("", { status: 409 }),
          error: { error: "state changed; refresh and retry" },
        }),
      ),
    ).rejects.toThrow("state changed");
    expect(message(new Error("failed"))).toBe("failed");
  });
  it("returns decimal strings intact", async () => {
    const r = await result<{ amount: string }>(
      Promise.resolve({
        response: new Response("", { status: 200 }),
        data: { amount: "9999999999999999.9999" },
      }),
    );
    expect(r.amount).toBe("9999999999999999.9999");
  });
});
