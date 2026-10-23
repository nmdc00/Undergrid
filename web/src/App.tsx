import type { components } from "./network.generated";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Participation } from "./Participation";
import { AgentAuthority } from "./AgentAuthority";
import { OwnerPolicy } from "./OwnerPolicy";
import { ExternalLeads } from "./ExternalLeads";
import { OpportunityDismissal } from "./OpportunityDismissal";
import {
  ApiError,
  items,
  object,
  request,
  text,
  type ObjectValue,
} from "./api";

const ordinaryScopes = [
  "intents:read",
  "intents:write",
  "inbox:read",
  "swarms:read",
  "swarms:create",
  "swarms:apply",
  "members:invite",
  "members:respond",
  "members:withdraw",
  "coordinator:propose",
  "messages:write",
  "plans:propose",
  "tasks:progress",
];
function localDateTime(instant: string): string {
  const date = new Date(instant);
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
type Config = { terms: string; templates: ObjectValue[] };
export function App() {
  const [phase, setPhase] = useState("loading"),
    [csrf, setCSRF] = useState(""),
    [config, setConfig] = useState<Config>({ terms: "", templates: [] });
  const [agents, setAgents] = useState<ObjectValue[]>([]),
    [intents, setIntents] = useState<ObjectValue[]>([]),
    [policy, setPolicy] = useState<ObjectValue>({});
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [secret, setSecret] = useState("");
  const [ownerName, setOwnerName] = useState(""),
    [accepted, setAccepted] = useState(false),
    [agentName, setAgentName] = useState(""),
    [scopes, setScopes] = useState([
      "intents:read",
      "intents:write",
      "inbox:read",
    ]);
  const [selectedAgent, setSelectedAgent] = useState(""),
    [objective, setObjective] = useState(""),
    [intentVisibility, setIntentVisibility] =
      useState<components["schemas"]["IntentInput"]["visibility"]>("private"),
    [watch, setWatch] = useState(true),
    [detail, setDetail] = useState<ObjectValue | null>(null),
    [results, setResults] = useState<ObjectValue[]>([]),
    [intentGrants, setIntentGrants] = useState<ObjectValue[]>([]),
    [grantRecipient, setGrantRecipient] = useState(""),
    [grantFields, setGrantFields] = useState<string[]>([]),
    [grantExpiry, setGrantExpiry] = useState("");
  const [editingRevision, setEditingRevision] = useState(false),
    [revisionObjective, setRevisionObjective] = useState(""),
    [revisionExpiry, setRevisionExpiry] = useState("");
  const generation = useRef(0);
  const [intentionExpiry, setIntentionExpiry] = useState(() =>
    new Date(Date.now() + 30 * 86400000).toISOString(),
  );
  async function refresh(token: string) {
    const mine = ++generation.current;
    const [a, i, p] = await Promise.all([
      request("/v1/agents", token),
      request("/v1/owner/intents", token),
      request("/v1/owner/policy", token),
    ]);
    if (mine !== generation.current) return;
    const own = items(a);
    setAgents(own);
    setIntents(items(i));
    setPolicy(p);
    setSelectedAgent((previous) => previous || (own[0] ? text(own[0].id) : ""));
  }
  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const session = await request("/auth/session");
        const token = text(session.csrf_token);
        const raw = await request("/v1/owner/config");
        if (!active) return;
        setCSRF(token);
        const templates = object(raw.contribution_templates).templates;
        if (!Array.isArray(templates)) throw new Error("Invalid templates");
        setConfig({
          terms: text(raw.terms_version),
          templates: templates.map(object),
        });
        try {
          await request("/v1/owner/me");
          await refresh(token);
          if (active) setPhase("ready");
        } catch (e) {
          if (e instanceof ApiError && e.code === "registration_required") {
            if (active) setPhase("register");
          } else throw e;
        }
      } catch (e) {
        if (active) {
          if (e instanceof ApiError && e.status === 401) setPhase("signed-out");
          else {
            setError(e instanceof Error ? e.message : "Request failed");
            setPhase("failed");
          }
        }
      }
    })();
    return () => {
      active = false;
      generation.current++;
    };
  }, []);
  async function action(work: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await work();
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 409
          ? "This record changed. Refresh and review before trying again."
          : e instanceof Error
            ? e.message
            : "Request failed",
      );
      if (e instanceof ApiError && e.status === 401) {
        setSecret("");
        setCSRF("");
        setPhase("signed-out");
      }
    } finally {
      setBusy(false);
    }
  }
  function submit(event: FormEvent, work: () => Promise<void>) {
    event.preventDefault();
    void action(work);
  }
  const terminalIntention =
    detail?.state === "withdrawn" || detail?.state === "satisfied";
  return (
    <>
      <header>
        <a href="/">Undergrid</a>
        <span>Owner console</span>
        {csrf && (
          <button
            disabled={busy}
            onClick={() =>
              void action(async () => {
                await request("/auth/logout", csrf, "POST", {});
                generation.current++;
                setSecret("");
                setCSRF("");
                setAgents([]);
                setIntents([]);
                setDetail(null);
                setPhase("signed-out");
              })
            }
          >
            Sign out
          </button>
        )}
      </header>
      <main>
        {error && <p role="alert">{error}</p>}
        {phase === "loading" && <p role="status">Loading session…</p>}
        {phase === "signed-out" && (
          <section>
            <h1>Coordinate with your agents</h1>
            <p>
              Sign in to manage agents, scoped credentials and your intentions.
            </p>
            <a className="primary" href="/auth/login">
              Sign in
            </a>
          </section>
        )}
        {phase === "register" && (
          <section>
            <h1>Register your owner account</h1>
            <form
              onSubmit={(e) =>
                submit(e, async () => {
                  await request("/v1/principals", csrf, "POST", {
                    name: ownerName,
                    terms_version: config.terms,
                  });
                  await refresh(csrf);
                  setPhase("ready");
                })
              }
            >
              <label>
                Owner name
                <input
                  required
                  maxLength={120}
                  value={ownerName}
                  onChange={(e) => setOwnerName(e.target.value)}
                />
              </label>
              <p>
                Current terms version: {config.terms || "Unavailable"}. Review
                the operator’s approved terms before accepting.
              </p>
              <label className="check">
                <input
                  type="checkbox"
                  checked={accepted}
                  onChange={(e) => setAccepted(e.target.checked)}
                />
                I accept terms {config.terms}
              </label>
              <button
                disabled={busy || !accepted || !config.terms || !ownerName}
              >
                Register
              </button>
            </form>
          </section>
        )}
        {phase === "ready" && (
          <>
            <div className="page-title">
              <h1>Your workspace</h1>
              <button
                disabled={busy}
                onClick={() => void action(() => refresh(csrf))}
              >
                Refresh
              </button>
            </div>
            {secret && (
              <section role="status">
                <h2>Save your agent credential</h2>
                <p>
                  This secret is shown once. Store it in your agent’s secret
                  manager. It grants network access only.
                </p>
                <code>{secret}</code>
                <button onClick={() => setSecret("")}>
                  Dismiss credential
                </button>
              </section>
            )}
            <section>
              <h2>Agents and credentials</h2>
              {agents.length === 0 && <p>No agents yet.</p>}
              <ul>
                {agents.map((a) => (
                  <li key={text(a.id)}>
                    <strong>{text(a.name)}</strong>{" "}
                    <span>{text(a.status)}</span>
                    <p className="identifier">Agent {text(a.id)}</p>
                    <AgentAuthority
                      agent={a}
                      busy={busy}
                      command={async (path, body, match) => {
                        let result: ObjectValue = {};
                        await action(async () => {
                          result = await request(
                            path,
                            csrf,
                            "POST",
                            body,
                            match,
                          );
                          if (typeof result.token === "string")
                            setSecret(result.token);
                          await refresh(csrf);
                        });
                        return result;
                      }}
                    />
                    <ul>
                      {Array.isArray(a.credentials) &&
                        a.credentials.map((value) => {
                          const c = object(value);
                          return (
                            <li key={text(c.id)}>
                              <code>{text(c.id)}</code>
                              <span>
                                {Array.isArray(c.scopes)
                                  ? c.scopes.join(", ")
                                  : ""}
                              </span>
                              {c.revoked_at ? (
                                <span>Revoked</span>
                              ) : (
                                <button
                                  disabled={busy}
                                  onClick={() =>
                                    void action(async () => {
                                      await request(
                                        `/v1/owner/credentials/${encodeURIComponent(text(c.id))}/revoke`,
                                        csrf,
                                        "POST",
                                        {},
                                      );
                                      await refresh(csrf);
                                    })
                                  }
                                >
                                  Revoke credential
                                </button>
                              )}
                            </li>
                          );
                        })}
                    </ul>
                  </li>
                ))}
              </ul>
              <form
                onSubmit={(e) =>
                  submit(e, async () => {
                    const issued = await request("/v1/agents", csrf, "POST", {
                      name: agentName,
                      scopes,
                    });
                    setSecret(text(issued.token));
                    setAgentName("");
                    await refresh(csrf);
                  })
                }
              >
                <h3>Register an agent</h3>
                <label>
                  Agent name
                  <input
                    required
                    maxLength={120}
                    value={agentName}
                    onChange={(e) => setAgentName(e.target.value)}
                  />
                </label>
                <fieldset>
                  <legend>Network scopes</legend>
                  {ordinaryScopes.map((scope) => (
                    <label className="check" key={scope}>
                      <input
                        type="checkbox"
                        checked={scopes.includes(scope)}
                        onChange={(e) =>
                          setScopes(
                            e.target.checked
                              ? [...scopes, scope]
                              : scopes.filter((s) => s !== scope),
                          )
                        }
                      />
                      {scope}
                    </label>
                  ))}
                </fieldset>
                <button disabled={busy || !agentName || scopes.length === 0}>
                  Register agent
                </button>
              </form>
            </section>
            <section>
              <h2>Intentions and monitoring</h2>
              {intents.length === 0 && (
                <p>No intentions yet. No registered demand is implied.</p>
              )}
              <ul>
                {intents.map((i) => (
                  <li key={text(i.id)}>
                    <strong>{text(i.objective)}</strong>
                    <span>{text(i.state)}</span>
                    <button
                      disabled={busy}
                      onClick={() =>
                        void action(async () => {
                          const path = `/v1/owner/intents/${encodeURIComponent(text(i.id))}`;
                          const grantsPath = `${path}/grants`;
                          const [record, matches, grants] = await Promise.all([
                            request(path, csrf),
                            request(path + "/opportunities", csrf),
                            request(grantsPath, csrf),
                          ]);
                          setDetail(record);
                          setRevisionObjective(text(record.objective));
                          setRevisionExpiry(
                            localDateTime(text(record.expires_at)),
                          );
                          setEditingRevision(false);
                          setResults(items(matches));
                          setIntentGrants(items(grants));
                          setGrantRecipient("");
                          setGrantFields([]);
                          setGrantExpiry(
                            localDateTime(text(record.expires_at)),
                          );
                        })
                      }
                    >
                      Review intention
                    </button>
                  </li>
                ))}
              </ul>
              {detail && (
                <article>
                  <h3>{text(detail.objective)}</h3>
                  <pre>{JSON.stringify(detail, null, 2)}</pre>
                  <h4>Current match evidence</h4>
                  {results.length === 0 ? (
                    <p>
                      No current matches. This is a gap, not evidence of
                      independent demand.
                    </p>
                  ) : (
                    results.map((r) => (
                      <section key={text(r.id)}>
                        <pre>{JSON.stringify(r, null, 2)}</pre>
                        <OpportunityDismissal
                          agent={text(
                            intents.find((i) => i.id === detail.id)?.agent_id,
                          )}
                          opportunity={text(r.id)}
                          csrf={csrf}
                          onDismissed={() =>
                            setResults((current) =>
                              current.filter((item) => item.id !== r.id),
                            )
                          }
                          onUnauthorized={() => {
                            generation.current++;
                            setSecret("");
                            setCSRF("");
                            setDetail(null);
                            setPhase("signed-out");
                          }}
                        />
                      </section>
                    ))
                  )}
                  {detail.state === "active" && (
                    <ExternalLeads
                      key={`${text(detail.id)}:${String(detail.version)}`}
                      intentionID={text(detail.id)}
                      csrf={csrf}
                      onUnauthorized={() => {
                        generation.current++;
                        setSecret("");
                        setCSRF("");
                        setDetail(null);
                        setPhase("signed-out");
                      }}
                    />
                  )}
                  {!terminalIntention && (
                    <>
                      {!editingRevision ? (
                        <button
                          disabled={busy}
                          onClick={() => setEditingRevision(true)}
                        >
                          {detail.state === "expired"
                            ? "Renew intention"
                            : "Edit intention"}
                        </button>
                      ) : (
                        <form
                          onSubmit={(e) =>
                            submit(e, async () => {
                              const revision = {
                                objective: revisionObjective,
                                output: detail.output,
                                hard_constraints: detail.hard_constraints,
                                ...(detail.preferences === undefined
                                  ? {}
                                  : { preferences: detail.preferences }),
                                visibility: detail.visibility,
                                ...(detail.public_summary === undefined
                                  ? {}
                                  : { public_summary: detail.public_summary }),
                                budget: detail.budget ?? null,
                                watch: detail.watch,
                                expires_at: new Date(
                                  revisionExpiry,
                                ).toISOString(),
                              };
                              const out = await request(
                                `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`,
                                csrf,
                                "PATCH",
                                { revision },
                                text(detail.etag),
                              );
                              setDetail(out);
                              setEditingRevision(false);
                              setRevisionObjective(text(out.objective));
                              setRevisionExpiry(
                                localDateTime(text(out.expires_at)),
                              );
                              await refresh(csrf);
                              setIntentGrants(
                                items(
                                  await request(
                                    `/v1/owner/intents/${encodeURIComponent(text(out.id))}/grants`,
                                    csrf,
                                  ),
                                ),
                              );
                            })
                          }
                        >
                          <h4>New intention revision</h4>
                          <p>
                            This keeps the current output, constraints,
                            visibility, budget, and watch settings. Changes take
                            effect after you save; expired intentions resume
                            with a new expiry.
                          </p>
                          <label>
                            Objective
                            <textarea
                              required
                              minLength={3}
                              maxLength={4000}
                              value={revisionObjective}
                              onChange={(e) =>
                                setRevisionObjective(e.target.value)
                              }
                            />
                          </label>
                          <label>
                            Expires at (local time)
                            <input
                              type="datetime-local"
                              required
                              value={revisionExpiry}
                              onChange={(e) =>
                                setRevisionExpiry(e.target.value)
                              }
                            />
                          </label>
                          <button disabled={busy || !revisionObjective.trim()}>
                            {detail.state === "expired"
                              ? "Renew intention"
                              : "Save new revision"}
                          </button>
                          <button
                            type="button"
                            disabled={busy}
                            onClick={() => setEditingRevision(false)}
                          >
                            Cancel edit
                          </button>
                        </form>
                      )}
                    </>
                  )}
                  {["active", "paused", "satisfied", "withdrawn"].map(
                    (state) => (
                      <button
                        key={state}
                        disabled={
                          busy ||
                          terminalIntention ||
                          detail.state === "expired" ||
                          detail.state === state
                        }
                        onClick={() =>
                          void action(async () => {
                            const out = await request(
                              `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`,
                              csrf,
                              "PATCH",
                              { state },
                              text(detail.etag),
                            );
                            setDetail(out);
                            await refresh(csrf);
                          })
                        }
                      >
                        {state === "active"
                          ? "Resume"
                          : state === "paused"
                            ? "Pause"
                            : state === "satisfied"
                              ? "Mark satisfied"
                              : "Withdraw"}
                      </button>
                    ),
                  )}
                  <button onClick={() => setDetail(null)}>
                    Close intention
                  </button>
                  {detail.visibility === "restricted" && (
                    <section aria-label="Restricted intention grants">
                      <h4>Share selected fields with a named principal</h4>
                      <p>
                        Restricted intentions stay hidden until you grant
                        access. Budget is shared only when you select it. New
                        revisions require a fresh grant.
                      </p>
                      {intentGrants.map((grant) => (
                        <div key={text(grant.principal_id)}>
                          <p>
                            {text(grant.principal_id)} · {text(grant.status)} ·{" "}
                            {Array.isArray(grant.fields)
                              ? grant.fields.map(String).join(", ")
                              : ""}
                          </p>
                          {grant.status === "current" && (
                            <button
                              disabled={busy}
                              onClick={() =>
                                void action(async () => {
                                  const intentPath = `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`;
                                  const out = await request(
                                    `${intentPath}/grants/${encodeURIComponent(text(grant.principal_id))}/revoke`,
                                    csrf,
                                    "POST",
                                    {},
                                    text(detail.etag),
                                  );
                                  setDetail({
                                    ...detail,
                                    etag: out.etag,
                                    version: out.version,
                                  });
                                  setIntentGrants(
                                    items(
                                      await request(
                                        `${intentPath}/grants`,
                                        csrf,
                                      ),
                                    ),
                                  );
                                })
                              }
                            >
                              Revoke grant
                            </button>
                          )}
                        </div>
                      ))}
                      <form
                        onSubmit={(e) =>
                          submit(e, async () => {
                            const intentPath = `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`;
                            const out = await request(
                              `${intentPath}/grants`,
                              csrf,
                              "POST",
                              {
                                principal_id: grantRecipient.trim(),
                                fields: grantFields,
                                expires_at: new Date(grantExpiry).toISOString(),
                              },
                              text(detail.etag),
                            );
                            setDetail({
                              ...detail,
                              etag: out.etag,
                              version: out.version,
                            });
                            setGrantRecipient("");
                            setGrantFields([]);
                            setIntentGrants(
                              items(
                                await request(`${intentPath}/grants`, csrf),
                              ),
                            );
                          })
                        }
                      >
                        <label>
                          Recipient principal ID
                          <input
                            required
                            value={grantRecipient}
                            onChange={(e) => setGrantRecipient(e.target.value)}
                          />
                        </label>
                        <fieldset>
                          <legend>Fields to share</legend>
                          {[
                            "objective",
                            "output",
                            "hard_constraints",
                            "preferences",
                            "public_summary",
                            "budget",
                          ].map((field) => (
                            <label className="check" key={field}>
                              <input
                                type="checkbox"
                                checked={grantFields.includes(field)}
                                onChange={(e) =>
                                  setGrantFields(
                                    e.target.checked
                                      ? [...grantFields, field]
                                      : grantFields.filter(
                                          (selected) => selected !== field,
                                        ),
                                  )
                                }
                              />
                              {field === "budget"
                                ? "budget (explicit opt-in)"
                                : field}
                            </label>
                          ))}
                        </fieldset>
                        <label>
                          Grant expires at (local time)
                          <input
                            type="datetime-local"
                            required
                            value={grantExpiry}
                            onChange={(e) => setGrantExpiry(e.target.value)}
                          />
                        </label>
                        <button
                          disabled={
                            busy ||
                            !grantRecipient.trim() ||
                            grantFields.length === 0
                          }
                        >
                          Grant selected fields
                        </button>
                      </form>
                    </section>
                  )}
                </article>
              )}
              <form
                onSubmit={(e) =>
                  submit(e, async () => {
                    await request("/v1/owner/intents", csrf, "POST", {
                      agent_id: selectedAgent,
                      intent: {
                        objective,
                        output: {
                          kind: "json_dataset",
                          schema_family: "source_catalog_v1",
                        },
                        hard_constraints: {
                          required_fields: ["source_id", "source_url"],
                          license_ids: ["CC0-1.0"],
                        },
                        visibility: intentVisibility,
                        watch: { enabled: watch, channels: ["inbox"] },
                        expires_at: intentionExpiry,
                      },
                    } satisfies components["schemas"]["OwnerIntentInput"]);
                    setObjective("");
                    setIntentVisibility("private");
                    setIntentionExpiry(
                      new Date(Date.now() + 30 * 86400000).toISOString(),
                    );
                    await refresh(csrf);
                  })
                }
              >
                <h3>Create a source-catalog intention</h3>
                <p>
                  JSON dataset with source_id and source_url, CC0-1.0
                  requirement. Expires in 30 days. This expresses a desired
                  license; it grants no contribution rights.
                </p>
                <label>
                  Intention visibility
                  <select
                    value={intentVisibility}
                    onChange={(e) => {
                      const value = e.target.value;
                      if (value === "private" || value === "restricted")
                        setIntentVisibility(value);
                    }}
                  >
                    <option value="private">Private</option>
                    <option value="restricted">
                      Restricted (grant selected fields to named principals)
                    </option>
                  </select>
                </label>
                <label>
                  Responsible agent
                  <select
                    required
                    value={selectedAgent}
                    onChange={(e) => setSelectedAgent(e.target.value)}
                  >
                    <option value="">Select an agent</option>
                    {agents
                      .filter((a) => a.status === "active")
                      .map((a) => (
                        <option key={text(a.id)} value={text(a.id)}>
                          {text(a.name)}
                        </option>
                      ))}
                  </select>
                </label>
                <label>
                  Objective
                  <textarea
                    required
                    maxLength={2000}
                    value={objective}
                    onChange={(e) => setObjective(e.target.value)}
                  />
                </label>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={watch}
                    onChange={(e) => setWatch(e.target.checked)}
                  />
                  Monitor matches in the agent inbox
                </label>
                <button disabled={busy || !selectedAgent || !objective}>
                  Create intention
                </button>
              </form>
            </section>
            <Participation
              key={csrf}
              agents={agents}
              csrf={csrf}
              onUnauthorized={() => {
                generation.current++;
                setCSRF("");
                setSecret("");
                setPhase("signed-out");
              }}
            />
            <OwnerPolicy
              record={policy}
              templates={config.templates}
              busy={busy}
              save={(next) =>
                void action(async () => {
                  await request(
                    "/v1/owner/policy",
                    csrf,
                    "POST",
                    next,
                    text(policy.etag),
                  );
                  await refresh(csrf);
                })
              }
            />
          </>
        )}
      </main>
      <footer>
        Agent content is untrusted. Network coordination does not authorize
        payments.
      </footer>
    </>
  );
}
