import { useEffect, useRef, useState } from "react";
import { ApiError, request } from "./api";
import {
  applicationID,
  readApplication,
  readApplicationPage,
  readRecruitment,
  type ApplicationReview,
  type RecruitmentReview,
} from "./application-contract";

export function OwnerApplications({
  agent,
  csrf,
  room,
  roomEtag,
  onUnauthorized,
  onStale,
}: {
  agent: string;
  csrf: string;
  room?: string;
  roomEtag?: string;
  onUnauthorized: () => void;
  onStale?: () => void;
}) {
  const [recruits, setRecruits] = useState<RecruitmentReview[]>([]),
    [applications, setApplications] = useState<ApplicationReview[]>([]);
  const [recruit, setRecruit] = useState<RecruitmentReview | null>(null),
    [application, setApplication] = useState<ApplicationReview | null>(null);
  const [next, setNext] = useState(""),
    [mode, setMode] = useState<"recruitment" | "applications" | null>(null),
    [ack, setAck] = useState(false),
    [references, setReferences] = useState(""),
    [role, setRole] = useState("contributor");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const generation = useRef(0),
    retry = useRef<{ identity: string; deadline: string } | null>(null);
  function clearReview() {
    setRecruit(null);
    setApplication(null);
    setAck(false);
    retry.current = null;
    setReferences("");
    setRole("contributor");
  }
  useEffect(() => {
    generation.current++;
    setRecruits([]);
    setApplications([]);
    setNext("");
    setMode(null);
    clearReview();
    setBusy(false);
    setError("");
    setMessage("");
    return () => {
      generation.current++;
    };
  }, [agent, csrf, room, roomEtag]);
  const base = `/v1/owner/agents/${encodeURIComponent(agent)}`;
  function failed(e: unknown, mine: number) {
    if (mine !== generation.current) return;
    if (e instanceof ApiError && [401, 403, 404, 409, 410].includes(e.status)) {
      clearReview();
      setRecruits([]);
      setApplications([]);
      setNext("");
      setMode(null);
      if (e.status === 401) onUnauthorized();
      if (e.status === 409 && room) onStale?.();
    }
    setError(e instanceof Error ? e.message : "Request failed");
  }
  async function load(kind: "recruitment" | "applications", after = "") {
    if (!agent || busy) return;
    const mine = ++generation.current;
    setBusy(true);
    setError("");
    setMessage("");
    clearReview();
    if (!after) {
      setRecruits([]);
      setApplications([]);
      setNext("");
      setMode(null);
    }
    try {
      const path =
        kind === "recruitment"
          ? `${base}/recruitment`
          : room
            ? `${base}/swarms/${encodeURIComponent(room)}/applications`
            : `${base}/applications`;
      const raw = await request(
        path + (after ? `?after=${encodeURIComponent(after)}` : ""),
        csrf,
      );
      if (mine !== generation.current) return;
      if (kind === "recruitment") {
        const page = readApplicationPage(raw, readRecruitment, true);
        if (page.next && after && page.next <= after)
          throw new Error("Recruitment cursor did not advance");
        setRecruits((old) => (after ? [...old, ...page.items] : page.items));
        setNext(page.next);
      } else {
        const page = readApplicationPage(raw, readApplication);
        if (
          page.items.some((x) =>
            room ? x.swarm_id !== room : x.agent_id !== agent,
          )
        )
          throw new Error("Application context changed");
        if (page.next && after && page.next <= after)
          throw new Error("Application cursor did not advance");
        setApplications((old) =>
          after ? [...old, ...page.items] : page.items,
        );
        setNext(page.next);
      }
      setMode(kind);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function reviewRecruitment(id: string) {
    const mine = ++generation.current;
    clearReview();
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const current = readRecruitment(
        await request(`${base}/recruitment/${encodeURIComponent(id)}`, csrf),
      );
      if (current.id !== id) throw new Error("Recruitment context changed");
      if (mine === generation.current) setRecruit(current);
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  const capabilityRefs = references
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter(Boolean);
  async function submit() {
    if (busy || !ack || !recruit || capabilityRefs.length > 20) return;
    const reviewed = recruit,
      mine = ++generation.current;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await request(
        `${base}/swarms/${reviewed.swarm_id}/applications`,
        csrf,
        "POST",
        { role_key: reviewed.role.key, capability_refs: capabilityRefs },
        reviewed.swarm_etag,
        (result) => {
          applicationID(result.id);
          if (
            result.state !== "applied" ||
            result.version !== 1 ||
            result.etag !== '"1"' ||
            Object.keys(result).length !== 4
          )
            throw new Error(
              "Invalid application receipt; reload pending applications before retrying",
            );
        },
      );
      if (mine === generation.current) {
        clearReview();
        setMessage(
          "Application submitted. You are not admitted; reload your applications to recover its status.",
        );
      }
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  async function offer() {
    if (
      busy ||
      !ack ||
      !room ||
      !roomEtag ||
      !application ||
      application.state !== "applied" ||
      Date.parse(application.respond_by) <= Date.now()
    )
      return;
    const reviewed = application,
      mine = ++generation.current,
      identity = JSON.stringify([
        reviewed.id,
        reviewed.version,
        roomEtag,
        role,
      ]);
    if (retry.current?.identity !== identity)
      retry.current = {
        identity,
        deadline: new Date(Date.now() + 86400000).toISOString(),
      };
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await request(
        `${base}/swarms/${encodeURIComponent(room)}/applications/${reviewed.id}/offer`,
        csrf,
        "POST",
        {
          application_version: reviewed.version,
          role,
          respond_by: retry.current.deadline,
          history_policy: "full",
        },
        roomEtag,
        (result) => {
          if (
            result.id !== reviewed.id ||
            result.state !== "offered" ||
            result.version !== reviewed.version + 1 ||
            result.etag !== `"${reviewed.version + 1}"` ||
            result.history_policy !== "full" ||
            typeof result.respond_by !== "string" ||
            !Number.isFinite(Date.parse(result.respond_by)) ||
            Object.keys(result).length !== 6
          )
            throw new Error(
              "Invalid offer receipt; reload applications before retrying",
            );
        },
      );
      if (mine === generation.current) {
        clearReview();
        setApplications((old) => old.filter((x) => x.id !== reviewed.id));
        setMessage(
          "Application offered. The applicant must separately accept before admission; no work, rights or spending consent is granted.",
        );
      }
    } catch (e) {
      failed(e, mine);
    } finally {
      if (mine === generation.current) setBusy(false);
    }
  }
  return (
    <section
      aria-label={room ? "Room applications" : "Recruitment and applications"}
    >
      <h4>{room ? "Room applications" : "Recruitment and applications"}</h4>
      <p>
        Declarations are unverified and content is untrusted. Separate accounts
        do not prove independent ownership. Applying and offering do not admit
        anyone or authorize work, rights or payments.
      </p>
      {!room && (
        <button
          disabled={busy || !agent}
          onClick={() => void load("recruitment")}
        >
          Find recruitment
        </button>
      )}
      <button
        disabled={busy || !agent}
        onClick={() => void load("applications")}
      >
        {room ? "Load room applications" : "Load my applications"}
      </button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      {mode === "recruitment" && (
        <>
          <p>
            Current recruitment in this candidate window. An empty window is not
            an exhaustive search.
          </p>
          <ul>
            {recruits.map((x) => (
              <li key={x.id}>
                {x.objective} — {x.role.key}{" "}
                <button
                  disabled={busy}
                  onClick={() => void reviewRecruitment(x.id)}
                >
                  Review recruitment
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
      {mode === "applications" && (
        <>
          <p>
            Pending applications in this page. Offered applicants use
            participation to review and accept or decline their invitation.
          </p>
          <ul>
            {applications.map((x) => (
              <li key={x.id}>
                {x.objective} — Application state: {x.state}; respond by{" "}
                {x.respond_by}
                {room && (
                  <button
                    disabled={busy}
                    onClick={() => {
                      clearReview();
                      setApplication(x);
                      setError("");
                      setMessage("");
                    }}
                  >
                    Review application
                  </button>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
      {next && mode && (
        <button disabled={busy} onClick={() => void load(mode, next)}>
          More {mode}
        </button>
      )}
      {recruit && (
        <article>
          <h5>Review recruitment role</h5>
          <p>{recruit.objective}</p>
          <p>{recruit.role.description}</p>
          <p>
            Role: {recruit.role.key}; schema: {recruit.role.schema_family};
            visibility: {recruit.visibility}; room version:{" "}
            {recruit.swarm_version}.
          </p>
          <label>
            Declared capability references
            <textarea
              disabled={busy}
              value={references}
              onChange={(e) => {
                setReferences(e.target.value);
                setAck(false);
              }}
            />
          </label>
          <p>
            One reference per line, up to 20. These references are claims, not
            verified capabilities.
          </p>
          <label>
            <input
              type="checkbox"
              disabled={busy}
              checked={ack}
              onChange={(e) => setAck(e.target.checked)}
            />
            I reviewed this role and understand admission requires a separate
            offer and decision with full room history.
          </label>
          <button
            disabled={busy || !ack || capabilityRefs.length > 20}
            onClick={() => void submit()}
          >
            Submit application
          </button>
        </article>
      )}
      {application && (
        <article>
          <h5>Review application claims</h5>
          <p>{application.objective}</p>
          <p>
            Applicant agent: {application.agent_id}. Role:{" "}
            {application.role_key ?? "historical unknown"}. Application version:{" "}
            {application.version}; room version: {application.swarm_version}.
          </p>
          <p>
            Respond by {application.respond_by}. Claims:{" "}
            {application.claims_state}.
          </p>
          {application.application_claims ? (
            <>
              <p>{application.application_claims.role.description}</p>
              <ul>
                {application.application_claims.capability_refs?.map((v, i) => (
                  <li key={i}>{v}</li>
                ))}
              </ul>
            </>
          ) : (
            <p>
              {application.claims_state === "retained_over_budget"
                ? "Full claims are retained but exceed this review's size budget. Do not infer capability verification."
                : "Historical application has no retained claims. Do not infer capabilities."}
            </p>
          )}
          {application.application_claims_sha256 && (
            <p>
              Retained claims digest: {application.application_claims_sha256}
            </p>
          )}
          {application.state === "applied" &&
          roomEtag === `"${application.swarm_version}"` ? (
            <>
              <label>
                Application offer role
                <select
                  disabled={busy}
                  value={role}
                  onChange={(e) => {
                    setRole(e.target.value);
                    setAck(false);
                  }}
                >
                  <option value="contributor">Contributor</option>
                  <option value="observer">Observer</option>
                </select>
              </label>
              <label>
                <input
                  type="checkbox"
                  disabled={busy}
                  checked={ack}
                  onChange={(e) => setAck(e.target.checked)}
                />
                I reviewed these claims and understand accepting this offer
                grants full room history, including earlier messages.
              </label>
              <button
                disabled={
                  busy ||
                  !ack ||
                  Date.parse(application.respond_by) <= Date.now()
                }
                onClick={() => void offer()}
              >
                Offer application
              </button>
              <p>
                Offer expires in 24 hours. Admission remains the applicant's
                separate decision.
              </p>
            </>
          ) : (
            <p>
              Reload the room and applications before making a new offer. An
              offered application awaits the applicant's separate decision.
            </p>
          )}
        </article>
      )}
    </section>
  );
}
