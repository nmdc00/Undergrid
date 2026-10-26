import type { components } from "./network.generated";
import { object, type ObjectValue } from "./api";

export type DeclaredProfile = Required<
  Omit<components["schemas"]["AgentProfile"], "capabilities">
> & { capabilities: string[] };
export const privateAgentProfile: DeclaredProfile = {
  public: false,
  accept_invitations: false,
  recruitment: false,
  capabilities: [],
};
export function savedAgentProfile(value: unknown): DeclaredProfile {
  const profile = object(value);
  if (
    typeof profile.public !== "boolean" ||
    typeof profile.accept_invitations !== "boolean" ||
    typeof profile.recruitment !== "boolean" ||
    !Array.isArray(profile.capabilities) ||
    !profile.capabilities.every((c) => typeof c === "string")
  )
    throw new Error(
      "Invalid saved agent profile; refresh before relying on its declarations",
    );
  return {
    public: profile.public,
    accept_invitations: profile.accept_invitations,
    recruitment: profile.recruitment,
    capabilities: profile.capabilities,
  };
}
export function ProfileReview({
  profile,
  onChange,
  reviewed,
  onReview,
  busy,
}: {
  profile: DeclaredProfile;
  onChange: (profile: DeclaredProfile) => void;
  reviewed: boolean;
  onReview: (checked: boolean) => void;
  busy: boolean;
}) {
  return (
    <fieldset disabled={busy}>
      <legend>Declared builder profile</legend>
      <p>
        Capabilities are your declarations, not verified expertise. A public
        declaration enables recruitment eligibility; it does not create a public
        directory page. Recruitment and invitations grant no membership, rights
        or spending authority.
      </p>
      {(
        [
          ["public", "Make declared profile public for recruitment"],
          ["accept_invitations", "Accept foreign invitations"],
          ["recruitment", "Receive recruitment opportunities"],
        ] as const
      ).map(([key, label]) => (
        <label className="check" key={key}>
          <input
            type="checkbox"
            checked={profile[key]}
            onChange={(event) =>
              onChange({ ...profile, [key]: event.target.checked })
            }
          />
          {label}
        </label>
      ))}
      <label className="check">
        <input
          type="checkbox"
          checked={profile.capabilities.includes("source_catalog_v1")}
          onChange={(event) =>
            onChange({
              ...profile,
              capabilities: event.target.checked ? ["source_catalog_v1"] : [],
            })
          }
        />
        Declare source_catalog_v1 capability
      </label>
      <section aria-label="Declared profile review">
        <pre data-testid="profile-json">{JSON.stringify(profile, null, 2)}</pre>
      </section>
      <label className="check">
        <input
          type="checkbox"
          checked={reviewed}
          onChange={(event) => onReview(event.target.checked)}
        />
        I reviewed this declared profile
      </label>
    </fieldset>
  );
}
export function SavedProfile({ agent }: { agent: ObjectValue }) {
  if (agent.profile === undefined)
    return (
      <p>Saved profile unavailable. Refresh to review its declarations.</p>
    );
  try {
    const profile = savedAgentProfile(agent.profile);
    return (
      <section aria-label={`Saved profile for ${String(agent.name)}`}>
        <h4>Saved declared profile</h4>
        <pre>{JSON.stringify(profile, null, 2)}</pre>
        <p>
          Profile editing is not yet available. These declarations do not verify
          expertise or independent ownership.
        </p>
      </section>
    );
  } catch (error) {
    return (
      <p role="alert">
        {error instanceof Error ? error.message : "Invalid saved agent profile"}
      </p>
    );
  }
}
