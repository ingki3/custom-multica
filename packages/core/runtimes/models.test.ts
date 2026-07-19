import { describe, expect, it } from "vitest";
import { runtimeModelsOptions } from "./models";

describe("runtimeModelsOptions", () => {
  it("keeps model catalogs stale so settings remounts refetch the config", () => {
    const options = runtimeModelsOptions("runtime-1");

    expect(options.staleTime).toBe(0);
    expect(options.retry).toBe(false);
  });
});
