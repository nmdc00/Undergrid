import { expect } from "@playwright/test";
import { type Actor } from "./owner-plan-journey";

export async function admitPeerAndExchangeMessage(
  actors: Actor[],
  room: string,
) {
  const creator = actors[0].page,
    peer = actors[1].page;
  // B explicitly published invitation consent at normal registration. Its agent
  // ID is a normal participant reference, not an administrator-selected identity.
  await creator
    .getByLabel("Target agent ID", { exact: true })
    .fill(actors[1].agent);
  await creator
    .getByLabel(/I understand accepting grants full room history/)
    .check();
  const offer = creator.waitForResponse(
    (r) =>
      new URL(r.url()).pathname ===
        `/v1/owner/agents/${actors[0].agent}/swarms/${room}/invitations` &&
      r.request().method() === "POST",
  );
  await creator
    .getByRole("button", { name: "Send invitation", exact: true })
    .click();
  expect((await offer).status()).toBe(201);
  await peer
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await peer
    .getByRole("button", { name: "Review invitation", exact: true })
    .click();
  const admission = peer.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith(
        `/swarms/${room}/membership-response`,
      ) && r.request().method() === "POST",
  );
  await peer
    .getByRole("button", { name: "Accept invitation", exact: true })
    .click();
  expect((await admission).status()).toBe(200);
  const message =
    "Reviewed catalogue collaboration; this message grants no plan or work authority.";
  await creator.getByLabel("Message text", { exact: true }).fill(message);
  const posting = creator.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith(`/swarms/${room}/messages`) &&
      r.request().method() === "POST",
  );
  await creator
    .getByRole("button", { name: "Post message", exact: true })
    .click();
  expect((await posting).status()).toBe(201);
  await peer.reload();
  await peer
    .getByRole("button", { name: "Load participation", exact: true })
    .click();
  await peer.getByRole("button", { name: "Open room", exact: true }).click();
  await expect(peer.getByText(message, { exact: true })).toBeVisible();
}
