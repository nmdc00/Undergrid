import fs from 'node:fs';
import Ajv2020 from 'ajv/dist/2020.js';
import addFormats from 'ajv-formats';
const document = JSON.parse(fs.readFileSync(new URL('../../internal/network/api/openapi.json', import.meta.url)));
const ajv = new Ajv2020({ allErrors: true, strict: false });
addFormats(ajv);
const root = 'urn:undergrid:network-contract';
ajv.addSchema({ $id: root, components: document.components });
const validateSchema = schema => ajv.compile(JSON.parse(JSON.stringify(schema).replaceAll('"#/components/', '"'+root+'#/components/')));
for (const [name, schema] of Object.entries(document.components.schemas)) {
 if (!ajv.validateSchema(schema)) throw Error(name+': '+ajv.errorsText());
 validateSchema(schema);
}
if (process.argv.includes('--schemas')) {
 console.log(`Compiled ${Object.keys(document.components.schemas).length} JSON Schema 2020-12 components`);
 process.exit(0);
}
const cases = JSON.parse(fs.readFileSync(0,'utf8'));
for (const c of cases) {
 const path = new URL(c.path,'https://contract.test').pathname;
 const candidates = Object.entries(document.paths).filter(([template]) => new RegExp('^'+template.replace(/\{[^}]+\}/g,'[^/]+')+'$').test(path));
 const entry = candidates.find(([,item]) => item[c.method.toLowerCase()]);
 if (!entry) throw Error(`Unadvertised actual operation ${c.method} ${path}`);
 const op = entry[1][c.method.toLowerCase()];
 if (c.status < 400 && !op.responses[c.status]) throw Error(`Undocumented success ${c.status}: ${c.method} ${path}`);
 const response = op.responses[c.status] ?? op.responses.default;
 const schema = response.content?.['application/json']?.schema;
 if (schema) {
  const validate = validateSchema(schema);
  if (!validate(c.response)) throw Error(`${c.method} ${path} ${c.status} response: ${ajv.errorsText(validate.errors)}\n${JSON.stringify(c.response)}`);
 }
 if (c.status < 400 && c.request !== null && op.requestBody?.content?.['application/json']) {
  const validate=validateSchema(op.requestBody.content['application/json'].schema);
  if (!validate(c.request)) throw Error(`${c.method} ${path} request: ${ajv.errorsText(validate.errors)}\n${JSON.stringify(c.request)}`);
 }
 if (c.status < 400 && c.response?.etag && c.etag !== c.response.etag) throw Error("Response ETag differs from body");
 if (c.status >= 400 && c.response.error.request_id !== c.request_id) throw Error('Error request ID differs from header');
}
// Independent negative oracles prove validators reject types and forbidden data,
// instead of merely accepting whatever the current server returned.
const publicIntent=validateSchema({$ref:'#/components/schemas/PublicIntent'});
const baseline={id:'a8391ea6-5003-401e-85e1-47c116fe3da0',version:1,etag:'"1"',state:'active',public_summary:'Public summary',output:{kind:'json_dataset',schema_family:'source_catalog_v1'},hard_constraints:{required_fields:[],license_ids:['CC0-1.0']}};
if (!publicIntent(baseline)) throw Error('Independent public fixture invalid');
if (publicIntent({...baseline,budget:{amount_minor:100,currency:'EUR',kind:'stated_budget'}})) throw Error('Public budget leak permitted');
if (publicIntent({...baseline,version:'1'})) throw Error('String version permitted');
if (process.argv.includes('--compatible-groups')) {
 const actual=cases.find(c=>c.path.endsWith('/compatible-groups')&&c.status===200&&c.response.items?.length);
 if (!actual) throw Error('Missing actual compatible-group response');
 const validate=validateSchema({$ref:'#/components/schemas/CompatibleGroups'});
 for (const mutate of [
  x=>x.items[0].members[0].private_objective='forbidden',
  x=>x.items[0].identity_assurance='verified_independent_owners',
  x=>x.items[0].common_license_ids=[],
  x=>x.items[0].comparison_mode='verified_semantic_group'
 ]) {
  const invalid=structuredClone(actual.response);mutate(invalid);
  if (validate(invalid)) throw Error('Group privacy/evidence negative oracle accepted');
 }
 console.log(`Validated ${cases.length} actual group HTTP cases and privacy/evidence denials`);
 process.exit(0);
}
if (process.argv.includes('--typed-demand-constraints')) {
 const proof=cases.flatMap(c=>Array.isArray(c.response?.items)?c.response.items:[]).map(x=>x.evidence).find(x=>x?.combined_constraints);
 if (!proof) throw Error('Missing actual combined typed demand lifecycle');
 const validate=validateSchema({$ref:'#/components/schemas/Compatibility'});
 if (validate({...proof,combined_constraints:{...proof.combined_constraints,freshness:{max_age_seconds:0}}})) throw Error('Invalid freshness evidence permitted');
 if (validate({...proof,constraint_evidence:[{dimension:'coverage',status:'verified',reason:'unsupported assurance'}]})) throw Error('Declared coverage mislabeled verified');
 console.log(`Validated ${cases.length} actual typed demand HTTP cases and independent range/assurance denials`);
 process.exit(0);
}
if (process.argv.includes('--private-semantic-query')) {
 const policy=cases.find(c=>c.path==='/v1/matching-policy'&&c.status===200)?.response;
 const request=cases.find(c=>c.method==='POST'&&c.request?.private_semantic_query)?.request;
 if (!policy?.profile_hash||!request) throw Error('Missing actual pinned private query lifecycle');
 const validate=validateSchema({$ref:'#/components/schemas/PrivateSemanticQuery'});
 for (const grant of [{...request.private_semantic_query,acknowledge_external_processing:false},{projection:request.private_semantic_query.projection,profile_hash:policy.profile_hash},{...request.private_semantic_query,profile_hash:'unbound'}]) {
  if (validate(grant)) throw Error('Invalid private processing acknowledgment permitted');
 }
 const metadata=validateSchema({$ref:'#/components/schemas/SemanticMatchingPolicy'});
 if (metadata({...policy,api_key:'secret'})) throw Error('Provider credential disclosure permitted');
 console.log(`Validated ${cases.length} private query HTTP cases and independent consent/privacy denials`);
 process.exit(0);
}
if (process.argv.includes('--semantic-discovery')) {
 const proof=cases.flatMap(c=>Array.isArray(c.response?.items)?c.response.items:[]).map(x=>x.evidence).find(x=>x?.algorithm==='hard-constraints-hybrid-cosine-v1');
 if (!proof) throw Error('Missing actual semantic opportunity lifecycle');
 const compatibility=validateSchema({$ref:'#/components/schemas/Compatibility'});
 for (const leaked of ['candidate_revision_hash','source_revision_hash','confidence']) {
  if (compatibility({...proof,[leaked]:leaked==='confidence'?0.99:'a'.repeat(64)})) throw Error('Semantic private fingerprint/confidence leak permitted: '+leaked);
 }
 for (const minimum_similarity of [-0.01, 1.01]) {
  if (compatibility({...proof,minimum_similarity})) throw Error('Out-of-range semantic threshold evidence permitted');
 }
 const intentInput=validateSchema({$ref:'#/components/schemas/IntentInput'});
 const request=cases.find(c=>c.method==='POST'&&c.path==='/v1/intents'&&c.request?.semantic_minimum_similarity!==undefined)?.request;
 if (!request) throw Error('Missing actual user-selected threshold request');
 for (const semantic_minimum_similarity of [-0.01,1.01]) {
  if (intentInput({...request,semantic_minimum_similarity})) throw Error('Out-of-range intention threshold permitted');
 }
 const {semantic_profile_hash,...missingPin}=proof;
 if (compatibility(missingPin)) throw Error('Semantic result without profile pin permitted');
 console.log(`Validated ${cases.length} actual semantic HTTP cases and independent projection/pinning denials`);
 process.exit(0);
}
if (process.argv.includes('--owner-swarm-creation')) {
 const created=cases.find(c=>c.method==='POST'&&c.path.endsWith('/swarms')&&c.status===201);
 if (!created) throw Error('Missing actual owner swarm creation lifecycle');
 const request=validateSchema({$ref:'#/components/schemas/OwnerSwarmCreateInput'});
 for (const mutate of [x=>delete x.history_policy,x=>x.history_policy='from_join',x=>x.visibility='public',x=>x.principal_id=baseline.id,x=>x.public_transcript_consent=true]) {
  const invalid=structuredClone(created.request);mutate(invalid);
  if (request(invalid)) throw Error('Owner room request authority/disclosure bypass');
 }
 const response=validateSchema({$ref:'#/components/schemas/OwnerSwarmCreated'});
 if (response({...created.response,visibility:'public'})||response({...created.response,version:'1'})||response({...created.response,private_transcript:'forbidden'})) throw Error('Owner creation response privacy/type bypass');
 console.log(`Validated ${cases.length} actual owner creation cases and independent disclosure/privacy denials`);
 process.exit(0);
}
if (process.argv.includes('--owner-room-messages')) {
 const created=cases.find(c=>c.method==='POST'&&c.status===201&&c.path.endsWith('/messages'));
 if(!created) throw Error('Missing actual owner message creation');
 const validate=validateSchema({$ref:'#/components/schemas/OwnerRoomMessageInput'});
 for(const invalid of [{body:''},{...created.request,replaces_id:'00000000-0000-0000-0000-000000000001'},{...created.request,principal_id:'00000000-0000-0000-0000-000000000001'},{...created.request,kind:'executable'}])
  if(validate(invalid)) throw Error('Owner message closed input bypass');
 console.log(`Validated ${cases.length} actual owner message cases and closed-input denials`);
 process.exit(0);
}
if (process.argv.includes('--owner-member-invitations')) {
 const offered=cases.find(c=>c.method==='POST'&&c.status===201&&c.path.endsWith('/invitations'));
 if (!offered) throw Error('Missing actual owner offered membership lifecycle');
 const validate=validateSchema({$ref:'#/components/schemas/OwnerMemberInvitationInput'});
 for (const invalid of [{...offered.request,history_policy:'none'},{...offered.request,admitted:true},{...offered.request,application_id:'00000000-0000-0000-0000-000000000001'}])
  if (validate(invalid)) throw Error('Owner invitation acknowledgment/closed-input bypass');
 const missing={...offered.request};delete missing.history_policy;
 if (validate(missing)) throw Error('Missing full history acknowledgment permitted');
 console.log(`Validated ${cases.length} actual owner invitation HTTP cases and independent negative oracles`);
 process.exit(0);
}
if (process.argv.includes('--owner-participation')) {
 const page=cases.find(c=>c.path.endsWith('/participation')&&c.method==='GET'&&c.status===200);
 const invitation=cases.find(c=>c.path.includes('/invitations/')&&c.status===200);
 const decision=cases.find(c=>c.path.endsWith('/membership-response')&&c.method==='POST'&&c.status===200);
 if (!page||!invitation||!decision) throw Error('Missing actual owner participation lifecycle');
 const validate=validateSchema({$ref:'#/components/schemas/OwnerParticipation'});
 if (validate({...page.response,private_transcript:'forbidden'})) throw Error('Participation transcript leak permitted');
 if (validate({...page.response,next_after:42})) throw Error('Participation cursor type bypass');
 const packet=validateSchema({$ref:'#/components/schemas/Invitation'});
 if (packet({...invitation.response,messages:[]})) throw Error('Invitation history leak permitted');
 console.log(`Validated ${cases.length} actual owner HTTP cases and independent privacy/type denials`);
 process.exit(0);
}
const published=cases.find(c=>c.path.startsWith('/v1/artifacts/')&&c.method==='GET'&&c.response?.state==='published');
if (!published) throw Error('Missing real public artifact projection case');
const artifact=validateSchema({$ref:'#/components/schemas/Artifact'});
if (artifact({...published.response,swarm_id:baseline.id})) throw Error('Published room metadata leak permitted');
console.log(`Validated ${cases.length} real HTTP request/response cases and privacy/type negative oracles`);
