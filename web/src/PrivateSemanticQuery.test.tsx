import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { PrivateSemanticQuery } from "./PrivateSemanticQuery";
const hash = "a".repeat(64);
const policy = {
  enabled: true,
  allow_private_projections: true,
  profile_hash: hash,
  provider_id: "selected-provider",
  model: "selected-model",
  model_revision: "r1",
  privacy_policy_sha256: "b".repeat(64),
  permission_evidence: "operator approval",
};
test("selected query requires explicit exact-profile consent, which edits revoke", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(policy))),
  );
  const changed = vi.fn();
  render(<PrivateSemanticQuery csrf="csrf" onChange={changed} />);
  expect(screen.queryByLabelText("Selected private matching query")).toBeNull();
  await userEvent.click(
    screen.getByRole("button", { name: "Review private matching policy" }),
  );
  await screen.findByText("selected-provider / selected-model / r1");
  expect(screen.getByText(hash)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Selected private matching query"), {
    target: { value: "Explicit selected query" },
  });
  expect(changed).toHaveBeenLastCalledWith(undefined);
  await userEvent.click(
    screen.getByLabelText(
      "Allow this selected query to be processed by this provider profile",
    ),
  );
  expect(changed).toHaveBeenLastCalledWith({
    projection: "Explicit selected query",
    profile_hash: hash,
    acknowledge_external_processing: true,
  });
  fireEvent.change(screen.getByLabelText("Selected private matching query"), {
    target: { value: "Changed query" },
  });
  expect(changed).toHaveBeenLastCalledWith(undefined);
  expect(
    (
      screen.getByLabelText(
        "Allow this selected query to be processed by this provider profile",
      ) as HTMLInputElement
    ).checked,
  ).toBe(false);
  await userEvent.click(
    screen.getByRole("button", { name: "Remove private matching query" }),
  );
  expect(changed).toHaveBeenLastCalledWith(undefined);
  expect(screen.queryByLabelText("Selected private matching query")).toBeNull();
});
test.each([
  { enabled: false },
  { ...policy, allow_private_projections: false },
  { ...policy, profile_hash: "invalid" },
])("unavailable policy cannot grant disclosure", async (raw) => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify(raw))),
  );
  const changed = vi.fn();
  render(<PrivateSemanticQuery csrf="csrf" onChange={changed} />);
  await userEvent.click(
    screen.getByRole("button", { name: "Review private matching policy" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("status").textContent).toMatch(/unavailable/),
  );
  expect(screen.queryByLabelText("Selected private matching query")).toBeNull();
  expect(changed).toHaveBeenLastCalledWith(undefined);
});

test("failed policy review cannot silently discard an existing revision query", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ enabled: false }))),
  );
  const submit = vi.fn();
  const changed = vi.fn();
  render(
    <form
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <PrivateSemanticQuery
        csrf="csrf"
        initialProjection="Existing selected query"
        onChange={changed}
      />
      <button>Save revision</button>
    </form>,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Review private matching policy" }),
  );
  await screen.findByRole("status");
  await userEvent.click(screen.getByRole("button", { name: "Save revision" }));
  expect(submit).not.toHaveBeenCalled();
  await userEvent.click(
    screen.getByRole("button", { name: "Remove private matching query" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Save revision" }));
  expect(submit).toHaveBeenCalledOnce();
  expect(changed).toHaveBeenLastCalledWith(undefined);
});
test.each(["   ", "€".repeat(400)])(
  "invalid selected query cannot silently submit without processing",
  async (projection) => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify(policy))),
    );
    const submit = vi.fn();
    render(
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        <PrivateSemanticQuery csrf="csrf" onChange={() => {}} />
        <button>Save revision</button>
      </form>,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Review private matching policy" }),
    );
    await screen.findByLabelText("Selected private matching query");
    fireEvent.change(screen.getByLabelText("Selected private matching query"), {
      target: { value: projection },
    });
    await userEvent.click(
      screen.getByRole("button", { name: "Save revision" }),
    );
    expect(submit).not.toHaveBeenCalled();
  },
);
