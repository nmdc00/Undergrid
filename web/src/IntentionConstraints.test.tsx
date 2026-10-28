import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test } from "vitest";
import {
  IntentionConstraintsEditor,
  readIntentionConstraints,
  type IntentionConstraints,
} from "./IntentionConstraints";
function Harness() {
  const [value, setValue] = useState<IntentionConstraints>({
    required_fields: ["source_id"],
    license_ids: ["CC0-1.0"],
    coverage: { required: ["region:eu"], excluded: [] },
  });
  return (
    <>
      <IntentionConstraintsEditor value={value} onChange={setValue} />
      <output aria-label="Current declarations">{JSON.stringify(value)}</output>
    </>
  );
}
function saved() {
  return JSON.parse(screen.getByLabelText("Current declarations").textContent!);
}
test("owner explicitly declares freshness then removes only that dimension", async () => {
  render(<Harness />);
  await userEvent.click(screen.getByLabelText("Declare freshness requirement"));
  const age = screen.getByLabelText("Maximum data age (seconds)");
  await userEvent.clear(age);
  await userEvent.type(age, "3600");
  expect(saved()).toEqual({
    required_fields: ["source_id"],
    license_ids: ["CC0-1.0"],
    coverage: { required: ["region:eu"], excluded: [] },
    freshness: { max_age_seconds: 3600 },
  });
  await userEvent.click(screen.getByLabelText("Declare freshness requirement"));
  expect(saved().freshness).toBeUndefined();
  expect(saved().coverage).toEqual({ required: ["region:eu"], excluded: [] });
});
test("owner declares exact delivery bounds and nonempty input alternatives", async () => {
  render(<Harness />);
  await userEvent.click(screen.getByLabelText("Set delivery window"));
  await userEvent.type(
    screen.getByLabelText("Not before (RFC3339)"),
    "2027-01-02T10:00:00.123456789+01:00",
  );
  await userEvent.type(
    screen.getByLabelText("Deliver by (RFC3339)"),
    "2027-01-03T10:00:00Z",
  );
  await userEvent.click(
    screen.getByLabelText("Declare input schema alternatives"),
  );
  await userEvent.type(
    screen.getByLabelText("Permitted input schema families"),
    "source_catalog_v1, reference_v1",
  );
  expect(saved().delivery).toEqual({
    not_before: "2027-01-02T10:00:00.123456789+01:00",
    deliver_by: "2027-01-03T10:00:00Z",
  });
  expect(saved().input_schema_families).toEqual([
    "source_catalog_v1",
    "reference_v1",
  ]);
});
test.each([
  { freshness: { max_age_seconds: 0 } },
  { freshness: { max_age_seconds: 1.5 } },
  { freshness: { max_age_seconds: 31536001 } },
  { delivery: { not_before: "2027-02-30T10:00:00Z" } },
  { delivery: {} },
  { input_schema_families: [] },
  { input_schema_families: ["duplicate", "duplicate"] },
])("rejects malformed current typed response %j", (extra) => {
  expect(() =>
    readIntentionConstraints({
      required_fields: [],
      license_ids: ["CC0-1.0"],
      ...extra,
    }),
  ).toThrow();
});
test("preserves nullable fields and exact delivery evidence", () => {
  const value = {
    required_fields: null,
    license_ids: null,
    freshness: null,
    input_schema_families: null,
    delivery: { not_before: "2027-01-02T10:00:00.123456789-00:00" },
  };
  expect(readIntentionConstraints(value)).toEqual(value);
});
