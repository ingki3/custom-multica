import { describe, expect, it } from "vitest";

import { childIssuesOptions } from "./queries";

describe("childIssuesOptions", () => {
  it("heals a cached snapshot whenever issue detail mounts", () => {
    const options = childIssuesOptions("workspace-1", "parent-1");
    expect(options.refetchOnMount).toBe("always");
  });
});
