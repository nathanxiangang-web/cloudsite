'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {workspace,principal,descriptor,tick}=require('./admin-dom.cjs');
const id='99700000-0000-4000-8000-000000000001',binding=descriptor.bindings[0].binding_id,base='/api/v1/admin/copies/'+id+'/presentation',exact=base+'?binding_id='+binding;
const cap={...descriptor,schema_version:20,admin_copy_presentation_enabled:true};
const file={binding_id:binding,copy_id:id,copy_availability:'PRESENT',resource_id:'fixture-canonical',name:'Canonical original.pdf',path:'/Canonical original.pdf',directory:false,size_bytes:4,presence:'PRESENT'};
function detail(version=0,name=''){return {source:'PANTA_COPY_PROJECTION',visibility_effect:'PRESENTATION_ONLY_NO_ACCESS_CONTROL',empty_name_fallback:'CURRENT_CANONICAL_DISPLAY',can_edit:true,read_at:'2026-10-28T01:00:00Z',evidence:{copy_id:id,binding_id:binding,indexcore_root_id:'fixture-root',indexcore_resource_id:file.resource_id,availability:'PRESENT',projection_cursor:12,copy_updated_at:'2026-10-28T00:00:00Z'},presentation:{display_name:name,category:'AUTO',hidden:false,version,updated_at:version?'2026-10-28T00:01:00Z':null,updated_by:version?principal.user_id:null}};}
const defaults=d=>({'/api/v1/admin/workspace':cap,'/api/v1/resources':{items:[file],next_cursor:''},[base]:d});
async function ready(d=detail(),configuration=cap,storage=new Map(),item=file){const app=workspace({storage});await app.drain({'/api/v1/admin/workspace':configuration});app.click('nav-resources');await app.drain({'/api/v1/admin/workspace':configuration,'/api/v1/resources':{items:[item],next_cursor:''}});app.run('openResource(state.resources[0]);');await app.drain({...defaults(d),'/api/v1/admin/workspace':configuration});return app;}
async function prepare(app,d=detail(),name='Product alias'){app.elements.get('resource-edit-name').value=name;const done=app.elements.get('resource-edit-form').listeners.submit[0]({preventDefault(){}});await app.drain(defaults(d));await done;}
async function auth(app,configuration=cap){await tick();app.respond(app.pending('/api/v1/admin/workspace'),configuration);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();}
function result(p,status='SUCCEEDED',code='COPY_PRESENTATION_SET'){return {request_id:p.request_id,target_id:id,action:'SET_COPY_PRESENTATION',status,code,recorded_at:'2026-10-28T01:00:01Z',...(status==='SUCCEEDED'?{presentation_version:p.expected_version+1}:{})};}
test('real editor saves exactly one frozen PUT and renders only a fresh persisted GET',async()=>{
 const app=await ready();assert.equal(app.elements.get('resource-edit-form').hidden,false);await prepare(app);
 assert.equal(app.elements.get('resource-edit-dialog').hidden,false);assert.equal(app.document.activeElement.id,'resource-edit-dismiss');
 app.elements.get('resource-edit-name').value='Changed after confirmation';
 const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body);assert.equal(put.options.method,'PUT');assert.equal(p.display_name,'Product alias');assert.equal(p.expected_version,0);assert.equal(p.binding_id,binding);
 app.respond(put,result(p));await app.drain(defaults(detail(1,'Persisted server label')));await done;
 assert.equal(app.elements.get('resource-edit-recovery').hidden,false);assert.match(app.text('resource-edit-result'),/SUCCEEDED.*版本 1/);assert.match(app.text('resource-edit-facts'),/Persisted server label/);assert.doesNotMatch(app.text('resource-edit-facts'),/Changed after/);assert.match(app.text('file-title'),/Canonical original/);assert.equal(app.requests.filter(r=>r.options.method==='PUT').length,1);
});
test('UNKNOWN uses original status GET then exact GET with no repeated PUT; storage contains IDs only',async()=>{
 const storage=new Map(),app=await ready(detail(),cap,storage);await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body);app.respond(put,result(p,'UNKNOWN','RESERVED_NO_CONFIRMED_RESULT'),202);await done;
 assert.match(app.text('resource-edit-result'),/UNKNOWN/);assert.equal(app.elements.get('resource-edit-prepare').disabled,true);assert.equal(storage.size,1);assert.doesNotMatch([...storage.values()].join(''),/Product alias|display_name|hidden|category/);
 const lookup=app.click('resource-edit-lookup');await auth(app);const get=app.pending('/api/v1/admin/actions/'+p.request_id);assert.equal(get.options.method,'GET');app.respond(get,result(p));await app.drain(defaults(detail(1,'Recovered from server')));await lookup;
 assert.match(app.text('resource-edit-facts'),/Recovered from server/);assert.equal(storage.size,0);assert.equal(app.requests.filter(r=>r.options.method==='PUT').length,1);
});
test('ambiguous transport keeps original ID across tab changes, logout and same-account restore',async()=>{
 const storage=new Map(),app=await ready(detail(),cap,storage);await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body);app.respond(put,{code:'ACTION_UNCONFIRMED'},503);await done;
 app.run("selectTab('users',false);");await app.drain(defaults(detail()));app.run('signedOut();');assert.equal(app.text('resource-edit-request'),'—');assert.equal(app.text('resource-edit-name'),'');assert.equal(storage.size,1);
 const restored=await ready(detail(),cap,storage);assert.equal(restored.text('resource-edit-request'),p.request_id);assert.equal(restored.elements.get('resource-edit-prepare').disabled,true);await prepare(restored);assert.equal(restored.requests.some(r=>r.options.method==='PUT'),false);
 restored.run("state.principal={...state.principal,user_id:'99700000-0000-4000-8000-000000000099'};adminResourceEditor.reset();");assert.equal(restored.text('resource-edit-request'),'—');assert.equal(storage.size,1);
});
test('409 receipt refreshes server version and never resubmits or fabricates success',async()=>{
 const app=await ready();await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body);app.respond(put,result(p,'REJECTED','VERSION_CONFLICT'),409);await app.drain(defaults(detail(2,'Other administrator label')));await done;
 assert.match(app.text('resource-edit-facts'),/Other administrator label/);assert.equal(app.elements.get('resource-edit-name').value,'Other administrator label');assert.equal(app.requests.filter(r=>r.options.method==='PUT').length,1);
});
test('capability, schema, projection and removed Copy gates remain conservative',async()=>{
 for(const configuration of [{...cap,schema_version:16},{...cap,admin_copy_presentation_enabled:false},{...descriptor,schema_version:20}]){const app=await ready(detail(),configuration);assert.equal(app.requests.some(r=>r.url.startsWith(base)),false);assert.equal(app.elements.get('resource-edit-form').hidden,true);assert.doesNotMatch(app.text('resource-rows'),/编辑展示信息/);}
 let app=await ready(detail(),cap,new Map(),{...file,copy_id:null});assert.match(app.text('resource-edit-status'),/没有可信的 Copy/);assert.equal(app.requests.some(r=>r.url.startsWith(base)),false);
 const d=detail();d.evidence.availability='REMOVED';d.can_edit=false;app=await ready(d);assert.equal(app.elements.get('resource-edit-prepare').disabled,true);assert.match(app.text('resource-edit-facts'),/REMOVED/);
});
test('malformed exact evidence never enables editing or echoes unsafe metadata',async()=>{
 for(const patch of [d=>{d.evidence.copy_id=principal.user_id},d=>{d.evidence.binding_id=principal.user_id},d=>{d.evidence.indexcore_resource_id='foreign'},d=>{d.read_at='bad'},d=>{d.presentation.display_name='<script>SECRET</script>'},d=>{d.presentation.version=-1},d=>{d.presentation.hidden='false'},d=>{d.evidence.availability='REMOVED'},d=>{d.presentation.updated_by='bad';d.presentation.version=1}]){const d=detail();patch(d);const app=await ready(d);assert.equal(app.elements.get('resource-edit-form').hidden,true);assert.match(app.text('resource-edit-status'),/UNAVAILABLE/);assert.doesNotMatch(app.text('resource-edit-facts'),/SECRET/);}
});
test('confirmation keyboard focus and bounds reject bad names without PUT',async()=>{
 const app=await ready();for(const name of ['<b>name</b>',' leading','a\nname','x\u202ey','\ud800','a'.repeat(161)]){await prepare(app,detail(),name);assert.equal(app.elements.get('resource-edit-dialog').hidden,true);}
 await prepare(app);app.elements.get('resource-edit-confirm').focus();app.events.keydown[0]({key:'Tab',preventDefault(){}});assert.equal(app.document.activeElement.id,'resource-edit-dismiss');app.events.keydown[0]({key:'Escape',preventDefault(){}});assert.equal(app.elements.get('resource-edit-dialog').hidden,true);assert.equal(app.requests.some(r=>r.options.method==='PUT'),false);
 await prepare(app,detail(),'');assert.equal(app.elements.get('resource-edit-dialog').hidden,false);assert.match(app.text('resource-edit-summary'),/清除/);
});
test('changed exact version, expired confirmation and storage failure send no mutation',async()=>{
 let app=await ready();await prepare(app,detail(1,'Updated'));assert.equal(app.elements.get('resource-edit-dialog').hidden,true);assert.match(app.text('resource-edit-status'),/版本已变化/);
 app=await ready();await prepare(app);const expiry=[...app.timers.values()].find(t=>t.ms===30000);expiry.callback();await app.click('resource-edit-confirm');assert.equal(app.requests.some(r=>r.options.method==='PUT'),false);
 app=await ready(detail(),cap,null);await prepare(app);await app.click('resource-edit-confirm');assert.match(app.text('resource-edit-status'),/未发送修改/);assert.equal(app.requests.some(r=>r.options.method==='PUT'),false);
});
test('late exact/PUT responses are fenced after Binding, capability, tab or principal changes',async()=>{
 for(const change of ["selectTab('users',false);","signedOut();","state.workspace={...state.workspace,bindings:[]};adminResourceEditor.invalidate();","state.workspace={...state.workspace,admin_copy_presentation_enabled:false};adminResourceEditor.invalidate();"]){
  const app=await ready();await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body);app.run(change);app.respond(put,result(p));await done;assert.equal(app.text('resource-edit-facts'),'');assert.equal(app.requests.filter(r=>r.options.method==='PUT').length,1);
 }
});
test('malformed or mismatched terminal receipts retain UNKNOWN and no partial private result',async()=>{
 for(const mutate of [r=>{r.target_id=principal.user_id},r=>{r.request_id=id},r=>{r.presentation_version=99},r=>{r.user={login:'SECRET'}},r=>{r.code='USER_CREATED'}]){const app=await ready();await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base),p=JSON.parse(put.options.body),r=result(p);mutate(r);app.respond(put,r);await done;assert.match(app.text('resource-edit-result'),/UNKNOWN/);assert.equal(app.elements.get('resource-edit-prepare').disabled,true);assert.doesNotMatch(app.text('resource-edit-facts'),/SECRET/);}
});
test('401/403 during a write clear private UI but preserve the original recovery reference',async()=>{
 for(const status of [401,403]){const storage=new Map(),app=await ready(detail(),cap,storage);await prepare(app);const done=app.click('resource-edit-confirm');await auth(app);const put=app.pending(base);app.respond(put,{code:status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED'},status);await done;assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('resource-edit-facts'),'');assert.equal(app.elements.get('resource-edit-name').value,'');assert.equal(storage.size,1);}
});
test('fresh authorization removes capability before PUT and late exact reads cannot reopen the form',async()=>{
 const app=await ready();await prepare(app);const done=app.click('resource-edit-confirm');await auth(app,{...cap,admin_copy_presentation_enabled:false});await done;assert.equal(app.requests.some(r=>r.options.method==='PUT'),false);assert.equal(app.elements.get('resource-edit-form').hidden,true);
 const next=await ready();next.run('openResource(state.resources[0]);');await auth(next);const old=next.pending(exact);next.run("selectTab('users',false);");next.respond(old,detail());await tick();assert.equal(next.text('resource-edit-facts'),'');assert.equal(next.elements.get('resource-edit-form').hidden,true);
});
test('read deadline closes editing, and missing secure UUID never sends a command',async()=>{
 const app=await ready();app.run('openResource(state.resources[0]);');await auth(app);const timer=[...app.timers.values()].find(t=>t.ms===15000);assert.ok(timer);timer.callback();await tick();assert.match(app.text('resource-edit-status'),/UNAVAILABLE/);assert.equal(app.elements.get('resource-edit-form').hidden,true);
 const next=await ready();await prepare(next);next.window.crypto={randomUUID(){throw Error('Unavailable');}};await next.click('resource-edit-confirm');assert.match(next.text('resource-edit-status'),/未发送修改/);assert.equal(next.requests.some(r=>r.options.method==='PUT'),false);
});
