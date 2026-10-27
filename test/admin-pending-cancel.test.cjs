'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {workspace,principal,descriptor,tick}=require('./admin-dom.cjs');
const fixture=require('./fixtures/admin-download-detail.json'),overview=require('./fixtures/admin-overview.json');
const id=fixture.manifest_id,path='/api/v1/admin/downloads/'+id;
const capability={...descriptor,schema_version:19,admin_pending_cancellation_enabled:true};
const pending=()=>({...structuredClone(fixture),manifest_state:'PENDING',job_id:null,job_state:null,result_copy_id:null,claim_attempts:0,failure_count:0,max_attempts:0,job_created_at:null,job_updated_at:null,started_at:null,finished_at:null,next_attempt_at:null,error_recorded:false,error_code:'NONE',copy:null});
const canceled=()=>({...pending(),manifest_state:'CANCELED'});
const overrides=d=>({'/api/v1/admin/workspace':capability,[path]:d,'/api/v1/admin/downloads':{items:[d],next_cursor:''}});
const posts=app=>app.requests.filter(r=>r.options.method==='POST');
async function auth(app,cap=capability){await tick();app.respond(app.pending('/api/v1/admin/workspace'),cap);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();}
async function ready(cap=capability){const app=workspace();await app.drain({'/api/v1/admin/workspace':cap});app.click('nav-downloads');await app.drain({'/api/v1/admin/workspace':cap});return app;}
async function detail(app,d=pending(),cap=capability){const done=app.run('adminDownloadDetail.load("'+id+'");');await auth(app,cap);app.respond(app.pending(path),d);await done;}
async function confirm(app,d=pending()){const done=app.click('pending-cancel-open');await auth(app);app.respond(app.pending(path),d);await done;assert.equal(app.elements.get('pending-cancel-dialog').hidden,false);}
function result(app,status='SUCCEEDED',code='PENDING_CANCELED'){
 const request=posts(app).at(-1),body=JSON.parse(request.options.body);
 return {request_id:body.request_id,target_id:id,action:'CANCEL_PENDING_MANIFEST',status,code,recorded_at:fixture.read_at,...(status==='SUCCEEDED'?{manifest:{state:'CANCELED',job_id:null,updated_at:fixture.updated_at}}:{})};
}
test('fresh exact GET, controlled modal, one cryptographic request and server-only refresh after success',async()=>{
 const app=await ready();await detail(app);assert.equal(app.elements.get('pending-cancel-open').hidden,false);
 await confirm(app);assert.equal(app.document.activeElement,app.elements.get('pending-cancel-dismiss'));assert.equal(posts(app).length,0);
 const done=app.click('pending-cancel-confirm');app.click('pending-cancel-confirm');await auth(app);
 assert.equal(posts(app).length,1);const request=posts(app)[0],body=JSON.parse(request.options.body);
 assert.equal(request.url,path+'/cancel-pending');assert.deepEqual(Object.keys(body).sort(),['expected_binding_id','expected_owner_user_id','expected_updated_at','request_id']);
 assert.equal(body.expected_updated_at,fixture.updated_at);assert.equal(body.expected_owner_user_id,fixture.owner_user_id);assert.equal(body.expected_binding_id,fixture.binding_id);assert.match(body.request_id,/^[0-9a-f-]{36}$/);
 app.respond(request,result(app));await app.drain(overrides(canceled()));await done;
 assert.match(app.text('admin-download-facts'),/CANCELED/);assert.match(app.text('pending-cancel-status'),/PENDING_CANCELED/);assert.equal(app.elements.get('pending-cancel-open').hidden,true);assert.equal(posts(app).length,1);
 assert.ok(app.requests.some(r=>r.url==='/api/v1/admin/overview'&&r.options.method==='GET'));
});
test('UNKNOWN, 503 and network failure retain the exact request and only an explicit GET resolves it',async()=>{
 for(const kind of ['unknown','503','network']){
  const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);const request=posts(app)[0];
  if(kind==='unknown')app.respond(request,result(app,'UNKNOWN','RESERVED_NO_CONFIRMED_RESULT'),202);
  else if(kind==='503')app.respond(request,{code:'ACTION_UNCONFIRMED'},503);
  else{request.done=true;request.reject(Error('SECRET'));}
  await done;assert.match(app.text('pending-cancel-status'),/未确认/);const requestID=JSON.parse(request.options.body).request_id;
  assert.equal(app.text('pending-cancel-request'),requestID);assert.equal(app.elements.get('pending-cancel-lookup').disabled,false);
  assert.equal(app.requests.some(r=>r.url==='/api/v1/admin/actions/'+requestID),false);
  await app.click('pending-cancel-open');assert.equal(posts(app).length,1);
  const lookup=app.click('pending-cancel-lookup');await auth(app);const get=app.pending('/api/v1/admin/actions/'+requestID);assert.equal(get.options.method,'GET');app.respond(get,result(app));await app.drain(overrides(canceled()));await lookup;assert.equal(posts(app).length,1);
 }
});
test('deadline before preflight auth completes cannot dispatch a late POST',async()=>{
 const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');
 const timer=[...app.timers.values()].find(t=>t.ms===15000);assert.ok(timer);timer.callback();await done;
 await auth(app);await tick();assert.equal(posts(app).length,0);assert.match(app.text('pending-cancel-status'),/未确认/);
});
test('modal Escape and focus trap preserve confirmation boundary; stale detail expires without POST',async()=>{
 const app=await ready();await detail(app);await confirm(app);
 app.events.keydown[0]({key:'Tab',shiftKey:true,preventDefault(){}});assert.equal(app.document.activeElement,app.elements.get('pending-cancel-confirm'));
 app.events.keydown[0]({key:'Tab',preventDefault(){}});assert.equal(app.document.activeElement,app.elements.get('pending-cancel-dismiss'));
 app.events.keydown[0]({key:'Escape',preventDefault(){}});assert.equal(app.elements.get('pending-cancel-dialog').hidden,true);assert.equal(app.elements.get('admin-download-detail').hidden,false);assert.equal(app.document.activeElement,app.elements.get('pending-cancel-open'));
 await confirm(app);const timer=[...app.timers.values()].find(t=>t.ms===30000);assert.ok(timer);timer.callback();await app.click('pending-cancel-confirm');assert.equal(posts(app).length,0);
});
test('capability false, absent, schema16 and non-PENDING/foreign/malformed shapes never expose cancellation',async()=>{
 for(const cap of [{...capability,schema_version:16},{...capability,admin_pending_cancellation_enabled:false},descriptor]){
  const app=await ready(cap);await detail(app,pending(),cap);assert.equal(app.elements.get('pending-cancel-open').hidden,true);await app.click('pending-cancel-open');assert.equal(posts(app).length,0);
 }
 for(const d of [fixture,canceled(),{...pending(),manifest_state:'ACTIVE'},{...pending(),manifest_state:'FAILED'},{...pending(),manifest_state:'READY'},{...pending(),manifest_state:'RECOVERY_REQUIRED'},{...pending(),binding_id:'97000000-0000-4000-8000-000000000099'},{...pending(),max_attempts:1},{...pending(),updated_at:'2026-10-26T00:00:00.123456789Z'}]){
  const app=await ready();await detail(app,d);assert.equal(app.elements.get('pending-cancel-open').hidden,true);
 }
});
test('409 rechecks current state and 404 hides the target without another command',async()=>{
 for(const status of [409,404]){
  const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);
  app.respond(posts(app)[0],result(app,'REJECTED',status===409?'PENDING_CHANGED':'MANIFEST_NOT_FOUND'),status);
  await app.drain(overrides(status===409?fixture:canceled()));await done;
  assert.equal(posts(app).length,1);assert.match(app.text('pending-cancel-status'),status===409?/PENDING_CHANGED/:/MANIFEST_NOT_FOUND/);
  if(status===404)assert.equal(app.elements.get('admin-download-detail').hidden,true);
 }
});
test('late responses after tab, target, principal, logout or Binding transitions do not carry action intent',async()=>{
 for(const change of ["selectTab('users');","adminDownloadDetail.load('97000000-0000-4000-8000-000000000099');","signedOut();","signedOut();state.principal={user_id:'other',role:'ADMIN'};","state.workspace={...state.workspace,bindings:[]};adminPendingCancel.invalidate();"]){
  const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);const request=posts(app)[0];
  app.run(change);app.respond(request,result(app));await done;assert.equal(posts(app).length,1);assert.doesNotMatch(app.text('pending-cancel-status'),/PENDING_CANCELED/);
 }
});
test('401/403 close the session; malformed or foreign receipt remains UNKNOWN without rendering payload',async()=>{
 for(const status of [401,403]){
  const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);app.respond(posts(app)[0],{code:status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED'},status);await done;assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('pending-cancel-request'),'—');
 }
 for(const patch of [{target_id:'97000000-0000-4000-8000-000000000099'},{action:'REVOKE_SHARE'},{status:'SUCCEEDED',code:'SECRET'},{manifest:{state:'ACTIVE',job_id:null,updated_at:fixture.updated_at}}]){
  const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);app.respond(posts(app)[0],{...result(app),...patch});await done;assert.match(app.text('pending-cancel-status'),/未确认/);assert.doesNotMatch(app.text('pending-cancel-status'),/SECRET/);assert.equal(posts(app).length,1);
 }
});
test('overview accepts omitted/zero/nonzero no-Job cancellations and rejects contradictory counts',async()=>{
 for(const n of [undefined,0,1]){
  const app=await ready(),o=structuredClone(overview);if(n!==undefined)o.manifests.canceled_without_job=n;
  if(n){o.jobs.total--;o.jobs.canceled--;}
  app.ctx.dto=o;const done=app.run('adminOverview.load(true)');await app.drain({'/api/v1/admin/workspace':capability,'/api/v1/admin/overview':o});await done;
  assert.equal(app.text('overview-manifests-canceled_without_job'),String(n||0));assert.doesNotMatch(app.text('overview-status'),/UNAVAILABLE/);
 }
 for(const n of [-1,'0',null,Number.MAX_SAFE_INTEGER+1,999]){
  const app=await ready(),o=structuredClone(overview);o.manifests.canceled_without_job=n;const done=app.run('adminOverview.load(true)');await app.drain({'/api/v1/admin/workspace':capability,'/api/v1/admin/overview':o});await done;assert.match(app.text('overview-status'),/UNAVAILABLE/);
 }
});
test('CANCELED without Job is valid only with zero attempts/no Copy; legacy CANCELED+Job remains valid',async()=>{
 const app=await ready();for(const row of [canceled(),{...fixture,manifest_state:'CANCELED',job_state:'CANCELED'}]){app.ctx.row=row;assert.equal(app.run('AdminDownloadDetail.valid(row)'),true);}
 for(const patch of [{claim_attempts:1},{failure_count:1},{max_attempts:1},{job_state:'CANCELED'},{result_copy_id:fixture.manifest_id},{copy:{}}]){app.ctx.row={...canceled(),...patch};assert.equal(app.run('AdminDownloadDetail.valid(row)'),false);}
 await detail(app,canceled());assert.match(app.text('admin-download-facts'),/未创建 Job/);assert.equal(app.elements.get('pending-cancel-open').hidden,true);
});
test('confirmation freezes the immediately preceding exact-read microsecond tuple and capability loss blocks dispatch',async()=>{
 const app=await ready();await detail(app);const latest={...pending(),updated_at:'2026-10-27T00:00:00.123456Z'};await confirm(app,latest);
 const done=app.click('pending-cancel-confirm');await auth(app);const request=posts(app)[0];assert.equal(JSON.parse(request.options.body).expected_updated_at,latest.updated_at);
 app.respond(request,result(app,'UNKNOWN','RESERVED_NO_CONFIRMED_RESULT'),202);await done;
 const next=await ready();await detail(next);await confirm(next);const blocked=next.click('pending-cancel-confirm');await auth(next,{...capability,admin_pending_cancellation_enabled:false});await blocked;assert.equal(posts(next).length,0);
});
test('an obsolete POST deadline cannot abort a new receipt lookup after a tab round-trip',async()=>{
 const app=await ready();await detail(app);await confirm(app);const done=app.click('pending-cancel-confirm');await auth(app);
 const old=posts(app)[0],oldDeadline=[...app.timers.values()].find(t=>t.ms===15000);old.done=true;
 app.run("selectTab('users');");await app.drain({'/api/v1/admin/workspace':capability});app.click('nav-downloads');await app.drain({'/api/v1/admin/workspace':capability});
 const lookup=app.click('pending-cancel-lookup');await auth(app);const get=app.pending('/api/v1/admin/actions/'+JSON.parse(old.options.body).request_id);assert.ok(get);
 oldDeadline.callback();await done;assert.equal(get.options.signal.aborted,false);
 app.respond(get,result(app,'UNKNOWN','RESERVED_NO_CONFIRMED_RESULT'),202);await lookup;assert.match(app.text('pending-cancel-status'),/未确认/);assert.equal(posts(app).length,1);
});
