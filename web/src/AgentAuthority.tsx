import { useState, type FormEvent } from "react";
import { object, text, type ObjectValue } from "./api";

type Command = (
  path: string,
  body: ObjectValue,
  match: string,
) => Promise<ObjectValue>;
export function AgentAuthority({
  agent,
  busy,
  command,
}: {
  agent: ObjectValue;
  busy: boolean;
  command: Command;
}) {
  const [publicKey, setPublicKey] = useState(""),
    [signature, setSignature] = useState("");
  const [challenge, setChallenge] = useState<ObjectValue | null>(null);
  const id = encodeURIComponent(text(agent.id));
  const credentials = Array.isArray(agent.credentials)
    ? agent.credentials.map(object)
    : [];
  const live = credentials.filter((c) => !c.revoked_at);
  function submit(event: FormEvent, work: () => Promise<void>) {
    event.preventDefault();
    void work();
  }
  return (
    <details>
      <summary>Signing proof and credential recovery</summary>
      <p>
        {agent.key_control_verified
          ? "Signing-key control verified."
          : "Signing-key control is unverified."}
      </p>
      <p>Endpoint control is unverified. Beneficial ownership is unknown.</p>
      <p>
        Prove possession of an Ed25519 signing key by signing the exact server
        challenge in your own trusted agent environment. The console never asks
        for a private key.
      </p>
      <form
        onSubmit={(event) =>
          submit(event, async () => {
            const result = await command(
              `/v1/owner/agents/${id}/control-challenge`,
              { public_key: publicKey },
              text(agent.etag),
            );
            if (typeof result.challenge === "string") setChallenge(result);
            setSignature("");
          })
        }
      >
        <label>
          Ed25519 public key (base64url)
          <input
            required
            value={publicKey}
            onChange={(event) => setPublicKey(event.target.value)}
          />
        </label>
        <button disabled={busy || !publicKey}>Create signing challenge</button>
      </form>
      {challenge && (
        <form
          onSubmit={(event) =>
            submit(event, async () => {
              const result = await command(
                `/v1/owner/agents/${id}/control-proof`,
                { challenge: text(challenge.challenge), signature },
                text(challenge.etag),
              );
              if (result.key_control_verified === true) {
                setChallenge(null);
                setSignature("");
              }
            })
          }
        >
          <p>
            Sign exactly these UTF-8 bytes. Challenge expires in five minutes.
          </p>
          <pre>{text(challenge.challenge)}</pre>
          <label>
            Ed25519 signature (base64url)
            <input
              required
              value={signature}
              onChange={(event) => setSignature(event.target.value)}
            />
          </label>
          <button disabled={busy || !signature}>Verify signing key</button>
        </form>
      )}
      <h4>Rotate a credential</h4>
      <p>
        Rotation immediately revokes this credential and keeps its scopes.
        Update the trusted agent runtime with the newly shown secret.
      </p>
      {live.map((credential) => (
        <div key={text(credential.id)}>
          <code>{text(credential.id)}</code>
          <button
            disabled={busy}
            onClick={() =>
              void command(
                `/v1/owner/credentials/${encodeURIComponent(text(credential.id))}/rotate`,
                { scopes: credential.scopes },
                text(credential.etag),
              )
            }
          >
            Rotate credential
          </button>
        </div>
      ))}
      <h4>Recover agent access</h4>
      <p>
        Recovery revokes all current credentials for this agent and creates a
        limited discovery credential. Previously granted rights scopes are
        removed.
      </p>
      <button
        disabled={busy}
        onClick={() =>
          void command(
            `/v1/owner/agents/${id}/recover-credential`,
            { scopes: ["intents:read", "intents:write", "inbox:read"] },
            text(agent.etag),
          )
        }
      >
        Revoke all and recover access
      </button>
    </details>
  );
}
