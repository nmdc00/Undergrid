import { useState } from "react";
import type { components } from "./network.generated";
import { object } from "./api";

export type IntentionConstraints = components["schemas"]["Constraints"];
function labels(value: unknown): string[] | null {
  if (value === null) return null;
  if (
    !Array.isArray(value) ||
    !value.every((part): part is string => typeof part === "string")
  )
    throw new Error("Reload the intention before editing its requirements");
  return value;
}
// Preserve every supported typed dimension. This checks the response shape;
// the domain command remains responsible for transition and constraint policy.
export function readIntentionConstraints(value: unknown): IntentionConstraints {
  const raw = object(value);
  const result: IntentionConstraints = {
    required_fields: labels(raw.required_fields),
    license_ids: labels(raw.license_ids),
  };
  if (raw.coverage !== undefined) {
    if (raw.coverage === null) result.coverage = null;
    else {
      const coverage = object(raw.coverage);
      const required = labels(coverage.required),
        excluded = labels(coverage.excluded);
      if (required === null || excluded === null)
        throw new Error("Invalid coverage; reload the intention");
      result.coverage = { required, excluded };
    }
  }
  if (raw.freshness !== undefined) {
    if (raw.freshness === null) result.freshness = null;
    else {
      const freshness = object(raw.freshness);
      if (
        typeof freshness.max_age_seconds !== "number" ||
        !Number.isFinite(freshness.max_age_seconds)
      )
        throw new Error("Invalid freshness; reload the intention");
      result.freshness = { max_age_seconds: freshness.max_age_seconds };
    }
  }
  if (raw.delivery !== undefined) {
    if (raw.delivery === null) result.delivery = null;
    else {
      const delivery = object(raw.delivery);
      const resultDelivery: { not_before?: string; deliver_by?: string } = {};
      for (const key of ["not_before", "deliver_by"] as const) {
        const bound = delivery[key];
        if (bound !== undefined) {
          if (typeof bound !== "string" || !Number.isFinite(Date.parse(bound)))
            throw new Error("Invalid delivery; reload the intention");
          resultDelivery[key] = bound;
        }
      }
      result.delivery = resultDelivery;
    }
  }
  if (raw.input_schema_families !== undefined)
    result.input_schema_families = labels(raw.input_schema_families);
  return result;
}
const splitLabels = (value: string) =>
  value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
function LabelsInput({
  label,
  value,
  limit,
  onChange,
}: {
  label: string;
  value: string[] | null;
  limit: number;
  onChange: (value: string[]) => void;
}) {
  const [draft, setDraft] = useState((value ?? []).join(", "));
  return (
    <label>
      {label}
      <input
        maxLength={limit * 130}
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value);
          onChange(splitLabels(event.target.value));
        }}
      />
    </label>
  );
}
export function IntentionConstraintsEditor({
  value,
  onChange,
  disabled = false,
}: {
  value: IntentionConstraints;
  onChange: (value: IntentionConstraints) => void;
  disabled?: boolean;
}) {
  return (
    <fieldset disabled={disabled}>
      <legend>Output and hard requirements</legend>
      <p>
        JSON dataset · source_catalog_v1. Enter declared labels separated by
        commas; these are requirements, not verified capabilities.
      </p>
      <LabelsInput
        label="Required field IDs"
        value={value.required_fields}
        limit={50}
        onChange={(required_fields) => onChange({ ...value, required_fields })}
      />
      <LabelsInput
        label="Permitted license IDs"
        value={value.license_ids}
        limit={20}
        onChange={(license_ids) => onChange({ ...value, license_ids })}
      />
      <label>
        <input
          type="checkbox"
          checked={value.coverage != null}
          onChange={(event) => {
            if (event.target.checked)
              onChange({ ...value, coverage: { required: [], excluded: [] } });
            else {
              const { coverage: _coverage, ...rest } = value;
              onChange(rest);
            }
          }}
        />
        Declare coverage
      </label>
      {value.coverage != null && (
        <>
          <LabelsInput
            label="Required coverage labels"
            value={value.coverage.required}
            limit={50}
            onChange={(required) =>
              onChange({
                ...value,
                coverage: {
                  required,
                  excluded: value.coverage?.excluded ?? [],
                },
              })
            }
          />
          <LabelsInput
            label="Excluded coverage labels"
            value={value.coverage.excluded}
            limit={50}
            onChange={(excluded) =>
              onChange({
                ...value,
                coverage: {
                  excluded,
                  required: value.coverage?.required ?? [],
                },
              })
            }
          />
        </>
      )}
    </fieldset>
  );
}
