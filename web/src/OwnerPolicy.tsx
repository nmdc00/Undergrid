import { useEffect, useState, type FormEvent } from "react";
import { object, text, type ObjectValue } from "./api";

type Policy = {
  allow_coordinator_succession: boolean;
  template_ids: string[];
  license_ids: string[];
  task_categories: string[];
  max_active_tasks: number;
  expires_at: string;
};
function strings(value: unknown): string[] {
  return Array.isArray(value) ? value.map(text) : [];
}
export function OwnerPolicy({
  record,
  templates,
  busy,
  save,
}: {
  record: ObjectValue;
  templates: ObjectValue[];
  busy: boolean;
  save: (policy: Policy) => void;
}) {
  const [draft, setDraft] = useState<Policy>({
    allow_coordinator_succession: false,
    template_ids: [],
    license_ids: [],
    task_categories: [],
    max_active_tasks: 0,
    expires_at: "",
  });
  useEffect(() => {
    const raw = object(record.policy ?? {});
    setDraft({
      allow_coordinator_succession: raw.allow_coordinator_succession === true,
      template_ids: strings(raw.template_ids),
      license_ids: strings(raw.license_ids),
      task_categories: strings(raw.task_categories),
      max_active_tasks:
        typeof raw.max_active_tasks === "number" ? raw.max_active_tasks : 0,
      expires_at: typeof raw.expires_at === "string" ? raw.expires_at : "",
    });
  }, [record]);
  const licenses = [
    ...new Set(
      templates.flatMap((t) =>
        draft.template_ids.includes(text(t.id)) ? strings(t.license_ids) : [],
      ),
    ),
  ];
  function select(
    field: "template_ids" | "license_ids" | "task_categories",
    value: string,
    enabled: boolean,
  ) {
    setDraft((current) => ({
      ...current,
      [field]: enabled
        ? [...current[field], value]
        : current[field].filter((item) => item !== value),
    }));
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    save({
      ...draft,
      expires_at: new Date(
        draft.expires_at.endsWith("Z")
          ? draft.expires_at
          : draft.expires_at + "Z",
      ).toISOString(),
    });
  }
  return (
    <section>
      <h2>Coordination policy</h2>
      <p>
        Rights and publication require explicit enabled templates, permitted
        licenses, bounded workload and expiry. Default grants no rights transfer
        or spending.
      </p>
      {templates.length === 0 && (
        <p>No contribution templates are configured.</p>
      )}
      <form onSubmit={submit}>
        <label className="check">
          <input type="checkbox" checked={draft.allow_coordinator_succession}
            onChange={(event) => setDraft({ ...draft, allow_coordinator_succession: event.target.checked })} />
          Permit coordinator succession approval and acceptance
        </label>
        <p>This authorizes eligible agents to vote on an exact successor proposal or accept the office. It does not appoint a successor or grant spending or publication rights.</p>
        <fieldset>
          <legend>Approved contribution terms</legend>
          {templates.map((template) => (
            <label key={text(template.id)} className="check">
              <input
                type="checkbox"
                checked={draft.template_ids.includes(text(template.id))}
                onChange={(event) =>
                  select(
                    "template_ids",
                    text(template.id),
                    event.target.checked,
                  )
                }
              />
              {text(template.id)}{" "}
              <a href={text(template.terms_url)} rel="noreferrer">
                Review terms
              </a>
            </label>
          ))}
        </fieldset>
        <fieldset>
          <legend>Permitted licenses</legend>
          {licenses.map((license) => (
            <label key={license} className="check">
              <input
                type="checkbox"
                checked={draft.license_ids.includes(license)}
                onChange={(event) =>
                  select("license_ids", license, event.target.checked)
                }
              />
              {license}
            </label>
          ))}
        </fieldset>
        <label className="check">
          <input
            type="checkbox"
            checked={draft.task_categories.includes("json_dataset")}
            onChange={(event) =>
              select("task_categories", "json_dataset", event.target.checked)
            }
          />
          Permit JSON dataset tasks
        </label>
        <label>
          Maximum active tasks
          <input
            type="number"
            required
            min={0}
            max={50}
            value={draft.max_active_tasks}
            onChange={(event) =>
              setDraft({
                ...draft,
                max_active_tasks: event.target.valueAsNumber,
              })
            }
          />
        </label>
        <label>
          Policy expires at (UTC)
          <input
            required
            type="datetime-local"
            value={draft.expires_at.slice(0, 16)}
            onChange={(event) =>
              setDraft({ ...draft, expires_at: event.target.value })
            }
          />
        </label>
        <p>
          Expiry must be within 90 days. Changing policy never approves a
          particular plan, publication or payment.
        </p>
        <button disabled={busy}>Save policy</button>
      </form>
    </section>
  );
}
