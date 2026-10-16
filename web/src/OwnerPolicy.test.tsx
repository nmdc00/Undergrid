import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { OwnerPolicy } from "./OwnerPolicy";

function policy(allowed?: boolean) {
 return {etag:'"1"',policy:{template_ids:[],license_ids:[],task_categories:[],max_active_tasks:0,expires_at:"2030-01-01T00:00:00Z",...(allowed===undefined?{}:{allow_coordinator_succession:allowed})}};
}
test("succession authority defaults off and requires an explicit owner opt-in",async()=>{
 const save=vi.fn();render(<OwnerPolicy record={policy()} templates={[]} busy={false} save={save}/>);
 const consent=screen.getByRole("checkbox",{name:"Permit coordinator succession approval and acceptance"});
 expect((consent as HTMLInputElement).checked).toBe(false);
 await userEvent.click(consent);await userEvent.click(screen.getByRole("button",{name:"Save policy"}));
 expect(save).toHaveBeenCalledWith(expect.objectContaining({allow_coordinator_succession:true}));
});
test("saving an existing policy retains its explicit succession consent",async()=>{
 const save=vi.fn();render(<OwnerPolicy record={policy(true)} templates={[]} busy={false} save={save}/>);
 expect((screen.getByRole("checkbox",{name:"Permit coordinator succession approval and acceptance"}) as HTMLInputElement).checked).toBe(true);
 await userEvent.click(screen.getByRole("button",{name:"Save policy"}));
 expect(save).toHaveBeenCalledWith(expect.objectContaining({allow_coordinator_succession:true}));
});
