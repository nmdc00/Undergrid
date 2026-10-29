import { CapabilityDiscovery } from "./CapabilityDiscovery";
import { ConstraintClarifications } from "./ConstraintClarifications";
import { CompatibleGroups } from "./CompatibleGroups";
import { type RoomOrigin } from "./CreateSwarm";
import {
  IntentionConstraintsEditor,
  readIntentionConstraints,
  type IntentionConstraints,
} from "./IntentionConstraints";
import {
  ProfileReview,
  SavedProfile,
  privateAgentProfile,
} from "./AgentProfile";
import { OwnerInbox } from "./OwnerInbox";
import type { components } from "./network.generated";
import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  PrivateSemanticQuery,
  type PrivateQuery,
} from "./PrivateSemanticQuery";
import { Participation } from "./Participation";
import { AgentAuthority } from "./AgentAuthority";
import { OwnerPolicy } from "./OwnerPolicy";
import { ExternalLeads } from "./ExternalLeads";
import { OpportunityDismissal } from "./OpportunityDismissal";
import {
  IntentionPublication,
  intentionVisibility,
  publicationReady,
  validateSavedIntention,
  type IntentionVisibility,
} from "./IntentionPublication";
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
const sourceCatalogOutput = {
  kind: "json_dataset",
  schema_family: "source_catalog_v1",
} as const;
const sourceCatalogConstraints = {
  required_fields: ["source_id", "source_url"],
  license_ids: ["CC0-1.0"],
};
function localDateTime(instant: string): string {
  const date = new Date(instant);
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}
type Config = { terms: string; templates: ObjectValue[] };
function FulfillmentSummary({ record }: { record: ObjectValue }) {
  try {
    const audit = object(record.partial_satisfaction),
      fields = object(record.hard_constraints).required_fields;
    if (
      !Array.isArray(fields) ||
      !Number.isInteger(audit.result_revision) ||
      !Number.isInteger(record.current_revision)
    )
      throw new Error("Invalid fulfillment summary");
    return (
      <section aria-label="Current unmet requirements">
        <h4>Unmet required fields</h4>
        <ul>
          {fields.map(text).map((field) => (
            <li key={field}>{field}</li>
          ))}
        </ul>
        <p>
          Last owner-declared fulfillment: revision{" "}
          {String(audit.result_revision)}. Current revision:{" "}
          {String(record.current_revision)}.{" "}
          {audit.result_revision !== record.current_revision
            ? "The recorded fulfillment is historical; current requirements may have changed."
            : "Other constraints remain required; this is not provider or rights verification."}
        </p>
      </section>
    );
  } catch {
    return (
      <p role="alert">
        Fulfillment details could not be read. Reload the intention before
        making another decision.
      </p>
    );
  }
}
export function App() {
  const [roomOrigin, setRoomOrigin] = useState<RoomOrigin | null>(null);
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
    [semanticThreshold, setSemanticThreshold] = useState(""),
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
    [revisionExpiry, setRevisionExpiry] = useState(""),
    [revisionVisibility, setRevisionVisibility] =
      useState<IntentionVisibility>("private"),
    [publicSummary, setPublicSummary] = useState(""),
    [reviewedCreation, setReviewedCreation] = useState(""),
    [revisionSummary, setRevisionSummary] = useState(""),
    [reviewedRevision, setReviewedRevision] = useState("");
  const [revisionSemanticThreshold, setRevisionSemanticThreshold] =
    useState("");
  const [revisionPrivateQuery, setRevisionPrivateQuery] =
    useState<PrivateQuery>();
  const [privateQuery, setPrivateQuery] = useState<PrivateQuery>();
  const [queryGeneration, setQueryGeneration] = useState(0);
  const [creationConstraints, setCreationConstraints] =
    useState<IntentionConstraints>(sourceCatalogConstraints);
  const [revisionConstraints, setRevisionConstraints] =
    useState<IntentionConstraints>(sourceCatalogConstraints);
  const [agentProfile, setAgentProfile] = useState(privateAgentProfile),
    [reviewedAgentProfile, setReviewedAgentProfile] = useState("");
  const agentProfileReview = JSON.stringify([csrf, agentProfile]);
  const generation = useRef(0);
  useEffect(() => {
    setRoomOrigin(null);
  }, [csrf]);
  const [intentionExpiry, setIntentionExpiry] = useState(() =>
    new Date(Date.now() + 30 * 86400000).toISOString(),
  );
  async function refresh(token: string, expectedGeneration?: number) {
    if (
      expectedGeneration !== undefined &&
      expectedGeneration !== generation.current
    )
      return;
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
    return mine;
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
  const creationPublication = {
    identity: JSON.stringify([csrf, selectedAgent]),
    visibility: intentVisibility,
    summary: publicSummary,
    output: sourceCatalogOutput,
    hardConstraints: creationConstraints,
  };
  const revisionPublication = {
    identity: JSON.stringify([csrf, detail?.id, detail?.etag]),
    visibility: revisionVisibility,
    summary: revisionSummary,
    output: detail?.output,
    hardConstraints: revisionConstraints,
  };
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
                onClick={() =>
                  void action(async () => {
                    await refresh(csrf);
                  })
                }
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
                    <SavedProfile agent={a} />
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
                    if (reviewedAgentProfile !== agentProfileReview) return;
                    const mine = generation.current;
                    const issued = await request(
                      "/v1/agents",
                      csrf,
                      "POST",
                      {
                        name: agentName,
                        scopes,
                        profile: agentProfile,
                      },
                      undefined,
                      (result) => {
                        if (
                          typeof result.credential_id !== "string" ||
                          !result.credential_id
                        )
                          throw new Error("Invalid server response");
                      },
                    );
                    if (mine !== generation.current) return;
                    setSecret(text(issued.token));
                    setAgentName("");
                    setAgentProfile(privateAgentProfile);
                    setReviewedAgentProfile("");
                    await refresh(csrf, mine);
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
                    disabled={busy}
                    onChange={(e) => setAgentName(e.target.value)}
                  />
                </label>
                <fieldset disabled={busy}>
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
                <ProfileReview
                  profile={agentProfile}
                  onChange={(profile) => {
                    setAgentProfile(profile);
                    setReviewedAgentProfile("");
                  }}
                  reviewed={reviewedAgentProfile === agentProfileReview}
                  onReview={(checked) =>
                    setReviewedAgentProfile(checked ? agentProfileReview : "")
                  }
                  busy={busy}
                />
                <button
                  disabled={
                    busy ||
                    !agentName ||
                    scopes.length === 0 ||
                    reviewedAgentProfile !== agentProfileReview
                  }
                >
                  Register agent
                </button>
              </form>
            </section>
            <section id="intentions-monitoring">
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
                          setRevisionSemanticThreshold(
                            typeof record.semantic_minimum_similarity ===
                              "number"
                              ? String(record.semantic_minimum_similarity)
                              : "",
                          );
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
                  {detail.private_semantic_query_status ===
                    "review_required" && (
                    <p>
                      Retained private query requires explicit review for this
                      revision before external processing.
                    </p>
                  )}
                  <pre>{JSON.stringify(detail, null, 2)}</pre>
                  {detail.partial_satisfaction !== undefined && (
                    <FulfillmentSummary record={detail} />
                  )}
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
                        <button
                          type="button"
                          disabled={
                            busy ||
                            detail.state !== "active" ||
                            !agents.some(
                              (a) =>
                                a.id ===
                                  intents.find((i) => i.id === detail.id)
                                    ?.agent_id && a.status === "active",
                            )
                          }
                          onClick={() =>
                            setRoomOrigin({
                              agent: text(
                                intents.find((i) => i.id === detail.id)
                                  ?.agent_id,
                              ),
                              context: JSON.stringify([
                                detail.id,
                                detail.version,
                                r,
                              ]),
                              opportunity: r,
                            })
                          }
                        >
                          Create swarm from this match
                        </button>
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
                    <ConstraintClarifications
                      key={`requirements:${text(detail.id)}:${String(detail.version)}`}
                      intentionID={text(detail.id)}
                      sourceVersion={Number(detail.version)}
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
                  {detail.state === "active" && (
                    <CapabilityDiscovery
                      key={`capabilities:${text(detail.id)}:${String(detail.version)}`}
                      intentionID={text(detail.id)}
                      sourceVersion={Number(detail.version)}
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
                  {detail.state === "active" &&
                    object(detail.watch).enabled === true && (
                      <CompatibleGroups
                        key={`groups:${text(detail.id)}:${String(detail.version)}`}
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
                  {detail.state === "active" && (
                    <ExternalLeads
                      key={`${text(detail.id)}:${String(detail.version)}`}
                      intentionID={text(detail.id)}
                      onIntentionChanged={(result) => {
                        setDetail(result);
                        setResults([]);
                        setRevisionObjective(text(result.objective));
                        setRevisionExpiry(
                          localDateTime(text(result.expires_at)),
                        );
                        setEditingRevision(false);
                      }}
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
                          onClick={() => {
                            try {
                              setRevisionPrivateQuery(undefined);
                              setRevisionConstraints(
                                readIntentionConstraints(
                                  detail.hard_constraints,
                                ),
                              );
                              setRevisionVisibility(
                                intentionVisibility(detail.visibility),
                              );
                              setRevisionSummary(
                                typeof detail.public_summary === "string"
                                  ? detail.public_summary
                                  : "",
                              );
                              setReviewedRevision("");
                              setEditingRevision(true);
                            } catch (failure) {
                              setError(
                                failure instanceof Error
                                  ? failure.message
                                  : "Reload the intention before editing",
                              );
                            }
                          }}
                        >
                          {detail.state === "expired"
                            ? "Renew intention"
                            : "Edit intention"}
                        </button>
                      ) : (
                        <form
                          onSubmit={(e) =>
                            submit(e, async () => {
                              if (
                                !publicationReady(
                                  revisionPublication,
                                  reviewedRevision,
                                )
                              )
                                return;
                              const mine = generation.current;
                              const previous = detail;
                              const revision = {
                                objective: revisionObjective,
                                ...(revisionPrivateQuery
                                  ? {
                                      private_semantic_query:
                                        revisionPrivateQuery,
                                    }
                                  : {}),
                                ...(revisionSemanticThreshold === ""
                                  ? {}
                                  : {
                                      semantic_minimum_similarity: Number(
                                        revisionSemanticThreshold,
                                      ),
                                    }),
                                output: detail.output,
                                hard_constraints: revisionConstraints,
                                ...(detail.preferences === undefined
                                  ? {}
                                  : { preferences: detail.preferences }),
                                visibility: detail.visibility,
                                ...(detail.visibility === "redacted_public"
                                  ? { public_summary: revisionSummary }
                                  : {}),
                                budget: detail.budget ?? null,
                                watch: detail.watch,
                                expires_at:
                                  revisionExpiry ===
                                  localDateTime(text(detail.expires_at))
                                    ? text(detail.expires_at)
                                    : new Date(revisionExpiry).toISOString(),
                              };
                              let out: ObjectValue;
                              try {
                                out = await request(
                                  `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`,
                                  csrf,
                                  "PATCH",
                                  { revision },
                                  text(detail.etag),
                                  detail.visibility === "redacted_public" ||
                                    revisionVisibility === "redacted_public"
                                    ? (result) =>
                                        validateSavedIntention(
                                          result,
                                          revision,
                                          previous,
                                        )
                                    : undefined,
                                );
                              } catch (failure) {
                                if (mine !== generation.current) return;
                                throw failure;
                              }
                              if (mine !== generation.current) return;
                              setDetail(out);
                              setEditingRevision(false);
                              setRevisionObjective(text(out.objective));
                              setRevisionSemanticThreshold(
                                typeof out.semantic_minimum_similarity ===
                                  "number"
                                  ? String(out.semantic_minimum_similarity)
                                  : "",
                              );
                              setRevisionExpiry(
                                localDateTime(text(out.expires_at)),
                              );
                              const refreshed = await refresh(csrf, mine);
                              if (
                                refreshed === undefined ||
                                refreshed !== generation.current
                              )
                                return;
                              const grants = await request(
                                `/v1/owner/intents/${encodeURIComponent(text(out.id))}/grants`,
                                csrf,
                              );
                              if (refreshed === generation.current)
                                setIntentGrants(items(grants));
                            })
                          }
                        >
                          <fieldset disabled={busy}>
                            <legend>Intention revision</legend>
                            <h4>New intention revision</h4>
                            <p>
                              Output, budget, audience and watch preferences
                              stay unchanged. Review your revised objective,
                              requirements and any public projection. Changes
                              take effect after saving; expired intentions
                              resume with a new expiry.
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
                            <IntentionConstraintsEditor
                              key={`revision-${String(detail.id)}-${String(detail.etag)}`}
                              value={revisionConstraints}
                              onChange={setRevisionConstraints}
                            />
                            <IntentionPublication
                              fixedAudience
                              review={revisionPublication}
                              label="Revision visibility"
                              reviewedKey={reviewedRevision}
                              onVisibility={setRevisionVisibility}
                              onSummary={setRevisionSummary}
                              onReview={setReviewedRevision}
                            />
                            <label>
                              Minimum semantic similarity for this intention
                              (0–1)
                              <input
                                type="number"
                                min="0"
                                max="1"
                                step="any"
                                value={revisionSemanticThreshold}
                                aria-describedby="revision-semantic-threshold-help"
                                onChange={(e) =>
                                  setRevisionSemanticThreshold(e.target.value)
                                }
                              />
                            </label>
                            {detail.private_semantic_query_status ===
                              "review_required" && (
                              <p>
                                Retained private query requires explicit review
                                for this revision before external processing.
                              </p>
                            )}
                            <PrivateSemanticQuery
                              csrf={csrf}
                              onChange={setRevisionPrivateQuery}
                              initialProjection={
                                detail.private_semantic_query == null
                                  ? ""
                                  : text(
                                      object(detail.private_semantic_query)
                                        .projection,
                                    )
                              }
                            />
                            <p id="revision-semantic-threshold-help">
                              Optional. Leave blank to use the service default.
                              Applies when semantic matching is enabled; the
                              score is not a probability.
                            </p>
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
                            <button
                              disabled={
                                busy ||
                                !revisionObjective.trim() ||
                                !publicationReady(
                                  revisionPublication,
                                  reviewedRevision,
                                )
                              }
                            >
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
                          </fieldset>
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
                            const mine = generation.current;
                            let out: ObjectValue;
                            try {
                              out = await request(
                                `/v1/owner/intents/${encodeURIComponent(text(detail.id))}`,
                                csrf,
                                "PATCH",
                                { state },
                                text(detail.etag),
                                detail.visibility === "redacted_public" &&
                                  state === "withdrawn"
                                  ? (result) =>
                                      validateSavedIntention(
                                        result,
                                        {
                                          ...detail,
                                          watch: {
                                            ...object(detail.watch),
                                            enabled: false,
                                          },
                                        },
                                        detail,
                                        "withdrawn",
                                      )
                                  : undefined,
                              );
                            } catch (failure) {
                              if (mine !== generation.current) return;
                              throw failure;
                            }
                            if (mine !== generation.current) return;
                            setDetail(out);
                            await refresh(csrf, mine);
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
                  <button
                    onClick={() => {
                      generation.current++;
                      setDetail(null);
                      setEditingRevision(false);
                    }}
                  >
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
                    if (
                      !publicationReady(creationPublication, reviewedCreation)
                    )
                      return;
                    const mine = generation.current;
                    const intent = {
                      objective,
                      ...(privateQuery
                        ? { private_semantic_query: privateQuery }
                        : {}),
                      ...(semanticThreshold === ""
                        ? {}
                        : {
                            semantic_minimum_similarity:
                              Number(semanticThreshold),
                          }),
                      output: sourceCatalogOutput,
                      hard_constraints: creationConstraints,
                      visibility: intentVisibility,
                      ...(intentVisibility === "redacted_public"
                        ? { public_summary: publicSummary }
                        : {}),
                      watch: { enabled: watch, channels: ["inbox"] },
                      expires_at: intentionExpiry,
                    } satisfies components["schemas"]["IntentInput"];
                    try {
                      await request(
                        "/v1/owner/intents",
                        csrf,
                        "POST",
                        {
                          agent_id: selectedAgent,
                          intent,
                        } satisfies components["schemas"]["OwnerIntentInput"],
                        undefined,
                        intentVisibility === "redacted_public"
                          ? (result) => validateSavedIntention(result, intent)
                          : undefined,
                      );
                    } catch (failure) {
                      if (mine !== generation.current) return;
                      throw failure;
                    }
                    if (mine !== generation.current) return;
                    setObjective("");
                    setPrivateQuery(undefined);
                    setCreationConstraints(sourceCatalogConstraints);
                    setQueryGeneration((n) => n + 1);
                    setSemanticThreshold("");
                    setIntentVisibility("private");
                    setPublicSummary("");
                    setReviewedCreation("");
                    setIntentionExpiry(
                      new Date(Date.now() + 30 * 86400000).toISOString(),
                    );
                    await refresh(csrf, mine);
                  })
                }
              >
                <fieldset disabled={busy}>
                  <legend>New intention details</legend>
                  <h3>Create a source-catalog intention</h3>
                  <p>
                    JSON source-catalog dataset. Review the requirements and
                    expiry below. A desired license grants no contribution
                    rights.
                  </p>
                  <IntentionConstraintsEditor
                    key={`creation-${queryGeneration}`}
                    value={creationConstraints}
                    onChange={setCreationConstraints}
                  />
                  <IntentionPublication
                    review={creationPublication}
                    label="Intention visibility"
                    reviewedKey={reviewedCreation}
                    onVisibility={setIntentVisibility}
                    onSummary={setPublicSummary}
                    onReview={setReviewedCreation}
                  />
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
                  <label>
                    Minimum semantic similarity (0–1)
                    <input
                      type="number"
                      min="0"
                      max="1"
                      step="any"
                      value={semanticThreshold}
                      aria-describedby="create-semantic-threshold-help"
                      onChange={(e) => setSemanticThreshold(e.target.value)}
                    />
                  </label>
                  <PrivateSemanticQuery
                    key={queryGeneration}
                    csrf={csrf}
                    onChange={setPrivateQuery}
                  />
                  <p id="create-semantic-threshold-help">
                    Optional. Leave blank to use the service default. Applies
                    when semantic matching is enabled; the score is not a
                    probability.
                  </p>
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={watch}
                      onChange={(e) => setWatch(e.target.checked)}
                    />
                    Monitor matches in the agent inbox
                  </label>
                  <button
                    disabled={
                      busy ||
                      !selectedAgent ||
                      !objective ||
                      !publicationReady(creationPublication, reviewedCreation)
                    }
                  >
                    Create intention
                  </button>
                </fieldset>
              </form>
            </section>
            <OwnerInbox
              key={`inbox:${csrf}`}
              agents={agents}
              csrf={csrf}
              onUnauthorized={() => {
                generation.current++;
                setCSRF("");
                setSecret("");
                setPhase("signed-out");
              }}
            />
            <Participation
              roomOrigin={
                roomOrigin &&
                detail &&
                results.some(
                  (r) =>
                    r.id === roomOrigin.opportunity.id &&
                    roomOrigin.context ===
                      JSON.stringify([detail.id, detail.version, r]),
                )
                  ? roomOrigin
                  : null
              }
              onClearRoomOrigin={() => setRoomOrigin(null)}
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
