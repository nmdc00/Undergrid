import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { test, expect, vi } from "vitest";
import { AgentAuthority } from "./AgentAuthority";

test("owner rotates exact reviewed credential without browser persistence", async () => {
  const call = vi.fn(async () => ({ token: "ug_rotated_one_time" }));
  const saved = vi.spyOn(Storage.prototype, "setItem");
  render(
    <AgentAuthority
      agent={{
        id: "agent",
        etag: '"1"',
        key_control_verified: false,
        credentials: [
          {
            id: "credential",
            etag: '"1"',
            scopes: ["intents:read"],
            revoked_at: null,
          },
        ],
      }}
      busy={false}
      command={call}
    />,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Rotate credential" }),
  );
  await waitFor(() =>
    expect(call).toHaveBeenCalledWith(
      "/v1/owner/credentials/credential/rotate",
      { scopes: ["intents:read"] },
      '"1"',
    ),
  );
  expect(saved).not.toHaveBeenCalled();
  saved.mockRestore();
});
test("key challenge and signature refer to exact server challenge and never imply endpoint ownership", async () => {
  const call = vi.fn(async (path: string) =>
    path.endsWith("/control-challenge")
      ? { challenge: "exact bounded challenge", etag: '"2"' }
      : { key_control_verified: true },
  );
  render(
    <AgentAuthority
      agent={{
        id: "agent",
        etag: '"1"',
        key_control_verified: false,
        credentials: [],
      }}
      busy={false}
      command={call}
    />,
  );
  await userEvent.click(
    screen.getByText("Signing proof and credential recovery"),
  );
  await userEvent.type(
    screen.getByLabelText("Ed25519 public key (base64url)"),
    "configured-public-key",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Create signing challenge" }),
  );
  await screen.findByText("exact bounded challenge");
  await userEvent.type(
    screen.getByLabelText("Ed25519 signature (base64url)"),
    "signed-challenge",
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Verify signing key" }),
  );
  await waitFor(() =>
    expect(call).toHaveBeenCalledWith(
      "/v1/owner/agents/agent/control-proof",
      { challenge: "exact bounded challenge", signature: "signed-challenge" },
      '"2"',
    ),
  );
  expect(
    screen.getByText(
      "Endpoint control is unverified. Beneficial ownership is unknown.",
    ),
  ).toBeTruthy();
});
