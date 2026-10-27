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

test("policy draft inputs freeze while a workspace action refreshes their record", async () => {
 const save=vi.fn();
 const templates=[{id:"test-contribution-v1",license_ids:["CC0-1.0"],terms_url:"https://example.org/test-terms"}];
 const initial={...policy(),policy:{...policy().policy,template_ids:["test-contribution-v1"]}};
 const {rerender}=render(<OwnerPolicy record={initial} templates={templates} busy={true} save={save}/>);
 const consent=screen.getByRole("checkbox",{name:"Permit coordinator succession approval and acceptance"});
 const template=screen.getByRole("checkbox",{name:/test-contribution-v1/});
 const license=screen.getByRole("checkbox",{name:"CC0-1.0"});
 const category=screen.getByRole("checkbox",{name:"Permit JSON dataset tasks"});
 const maximum=screen.getByLabelText("Maximum active tasks");
 const expiry=screen.getByLabelText("Policy expires at (UTC)");
 for(const control of [consent,template,license,category,maximum,expiry]) expect(control.matches(":disabled")).toBe(true);
 await userEvent.click(consent);
 await userEvent.type(maximum,"8");
 expect((consent as HTMLInputElement).checked).toBe(false);
 expect((maximum as HTMLInputElement).value).toBe("0");
 expect(save).not.toHaveBeenCalled();
 rerender(<OwnerPolicy record={{...initial}} templates={templates} busy={false} save={save}/>);
 for(const control of [consent,template,license,category,maximum,expiry]) expect(control.matches(":disabled")).toBe(false);
 await userEvent.click(consent);
 await userEvent.clear(maximum);
 await userEvent.type(maximum,"8");
 await userEvent.click(screen.getByRole("button",{name:"Save policy"}));
 expect(save).toHaveBeenCalledWith(expect.objectContaining({allow_coordinator_succession:true,max_active_tasks:8}));
});
