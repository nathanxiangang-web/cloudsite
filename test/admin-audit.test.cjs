'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {workspace,principal,descriptor,tick}=require('./admin-dom.cjs');
const cap={...descriptor,schema_version:19,admin_audit_enabled:true},base='/api/v1/admin/audit/actions';
const source='CLOUDSITE_POSTGRES_ADMIN_ACTION_LEDGER',scope='CURRENT_ADMIN_ACTIONS_IN_CONFIGURED_BINDINGS';
const row=(n=3)=>({actor_id:principal.user_id,request_id:'99500000-0000-4000-8000-'+String(n).padStart(12,'0'),action:'CREATE_USER',status:'UNKNOWN',code:'RESERVED_NO_CONFIRMED_RESULT',created_at:'2026-10-27T00:00:00.123456Z',recorded_at:null,target_status:'NOT_RECORDED'});
const page=(items=[row()],next_cursor='')=>({items,next_cursor,source,scope,read_at:'2026-10-27T01:00:00Z'});
const evidence=(v=row())=>({...v,source,scope,read_at:'2026-10-27T01:00:00Z'});
async function auth(app,workspace=cap){await tick();app.respond(app.pending('/api/v1/admin/workspace'),workspace);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();}
async function ready(data=page(),configuration=cap){const app=workspace();await app.drain({'/api/v1/admin/workspace':configuration});app.click('nav-audit');await app.drain({'/api/v1/admin/workspace':configuration,[base]:data});return app;}
async function refresh(app,data,status=200){const done=app.click('audit-refresh');await auth(app);app.respond(app.pending(/\/admin\/audit\/actions\?/),data,status);await done;}
async function detail(app,value=evidence()){const done=app.run('adminAudit.detail("'+value.request_id+'");');await auth(app);app.respond(app.pending(base+'/'+principal.user_id+'/'+value.request_id),value);await done;}
test('real audit destination is read-only, actor scoped and shows UNKNOWN without invented terminal time',async()=>{
 const app=await ready();assert.equal(app.elements.get('audit-panel').hidden,false);assert.equal(app.elements.get('planned-panel').hidden,true);assert.match(app.text('audit-rows'),/结果未确认/);assert.match(app.text('audit-footer'),/本页 1 条.*未提供全量总数/);
 assert.match(app.requests.find(r=>r.url.startsWith(base+'?')).url,/actor_id=97000000-0000-4000-8000-000000000009/);
 await detail(app);assert.match(app.text('audit-facts'),/无已确认终态/);assert.match(app.text('audit-detail-status'),/不会执行或重放/);assert.ok(app.requests.every(r=>r.options.method==='GET'));
});
test('filters carry exact action/status/UTC range, cursor resets and stale same-query results remain honest',async()=>{
 const app=await ready(page([row()],'eyJjdXJzb3IiOjF9'));
 const next=app.click('audit-more');await auth(app);const request=app.pending(/\/admin\/audit\/actions\?/);assert.match(request.url,/cursor=eyJjdXJzb3IiOjF9/);app.respond(request,page([]));await next;
 app.elements.get('audit-action').value='CREATE_USER';app.elements.get('audit-status').value='UNKNOWN';app.elements.get('audit-from').value='2026-10-26T00:00';app.elements.get('audit-before').value='2026-10-28T00:00';
 const filter=app.elements.get('audit-filter').listeners.submit[0]({preventDefault(){}});await auth(app);const query=app.pending(/\/admin\/audit\/actions\?/);const params=new URLSearchParams(query.url.split('?')[1]);assert.equal(params.has('cursor'),false);assert.equal(params.get('from'),'2026-10-26T00:00:00.000Z');assert.equal(params.get('status'),'UNKNOWN');app.respond(query,page());await filter;
 await refresh(app,{code:'AUDIT_UNAVAILABLE'},503);assert.match(app.text('audit-footer'),/STALE/);assert.match(app.text('audit-rows'),/结果未确认/);assert.equal(app.elements.get('audit-empty').hidden,true);
});
test('unsafe/foreign or malformed receipts never partially render and do not expose payload fields',async()=>{
 for(const patch of [{actor_id:row().request_id},{status:'RECOVERY_REQUIRED'},{status:'SUCCEEDED'},{code:'Cookie=SECRET'},{password:'SECRET'},{created_at:'2026-02-30T00:00:00Z'},{recorded_at:'2026-10-27T01:00:00Z'},{target_status:'SECRET'},{manifest_state:'ACTIVE'}]){
  const app=await ready(page([{...row(),...patch}]));assert.equal(app.text('audit-rows'),'');assert.match(app.text('audit-footer'),/UNAVAILABLE/);assert.doesNotMatch(app.text('audit-rows'),/SECRET/);
 }
 const app=await ready();await detail(app,{...evidence(),outcome:{password:'SECRET'}});assert.equal(app.text('audit-facts'),'');assert.match(app.text('audit-detail-status'),/UNAVAILABLE/);
});
test('terminal actions and deleted user targets display only safe historical evidence',async()=>{
 const v={...row(),action:'DELETE_USER',status:'SUCCEEDED',code:'USER_DELETED',recorded_at:'2026-10-27T01:00:00Z',target_status:'DELETED_OR_MISSING'};
 const app=await ready(page([v]));await detail(app,evidence(v));assert.match(app.text('audit-facts'),/已记录成功/);assert.match(app.text('audit-facts'),/已删除或不存在/);assert.doesNotMatch(app.text('audit-facts'),/SECRET/);assert.ok(app.requests.every(r=>r.options.method==='GET'));
});
test('detail focus and Escape restore the row button; exact GET failures are not empty success',async()=>{
 const app=await ready();const button=app.elements.get('audit-rows').children[0].children[0].children[0];button.focus();const done=button.listeners.click[0]();await auth(app);app.respond(app.pending(base+'/'+principal.user_id+'/'+row().request_id),evidence());await done;assert.equal(app.document.activeElement,app.elements.get('audit-detail'));
 app.events.keydown[0]({key:'Escape'});assert.equal(app.elements.get('audit-detail').hidden,true);assert.equal(app.document.activeElement,button);
 for(const status of [404,503]){const load=app.run('adminAudit.detail("'+row().request_id+'");');await auth(app);app.respond(app.pending(base+'/'+principal.user_id+'/'+row().request_id),{code:status===404?'AUDIT_NOT_FOUND':'AUDIT_UNAVAILABLE'},status);await load;assert.match(app.text('audit-detail-status'),status===404?/NOT_FOUND/:/UNAVAILABLE/);assert.equal(app.text('audit-facts'),'');}
});
test('late list/detail results are fenced across tab, filter, logout, principal and Binding changes',async()=>{
 for(const change of ["selectTab('users');","signedOut();","signedOut();state.principal={user_id:'other',role:'ADMIN'};","state.workspace={...state.workspace,bindings:[]};adminAudit.invalidate();"]){
  const app=await ready(),done=app.run('adminAudit.detail("'+row().request_id+'");');await auth(app);const old=app.pending(base+'/'+principal.user_id+'/'+row().request_id);app.run(change);app.respond(old,evidence());await done;assert.equal(app.text('audit-facts'),'');
 }
 const app=await ready(),load=app.click('audit-refresh');await auth(app);const old=app.pending(/\/admin\/audit\/actions\?/);app.elements.get('audit-status').value='REJECTED';app.elements.get('audit-action').value='ALL';const next=app.elements.get('audit-filter').listeners.submit[0]({preventDefault(){}});await auth(app);const current=app.requests.find(r=>!r.done&&r!==old&&r.url.startsWith(base+'?'));app.respond(current,page([]));await next;app.respond(old,page());await load;assert.equal(app.text('audit-rows'),'');assert.equal(app.elements.get('audit-empty').hidden,false);
});
test('401/403 clear private rows and missing optional capability does not invent an audit',async()=>{
 for(const status of [401,403]){const app=await ready(),done=app.click('audit-refresh');await auth(app);app.respond(app.pending(/\/admin\/audit\/actions\?/),{code:status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED'},status);await done;assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('audit-rows'),'');assert.equal(app.text('audit-actor'),'—');}
 for(const conf of [{...cap,admin_audit_enabled:false},{...cap,schema_version:15}]){const app=await ready(page(),conf);assert.match(app.text('audit-footer'),/UNAVAILABLE/);assert.equal(app.requests.some(r=>r.url.startsWith(base)),false);}
});
test('microsecond ordering, time filtering and duplicate IDs are checked without rounding chronology',async()=>{
 const high={...row(2),created_at:'2026-10-27T00:00:00.123456Z'},low={...row(3),created_at:'2026-10-27T00:00:00.123455Z'};
 let app=await ready(page([high,low]));assert.match(app.text('audit-rows'),/000000000002/);
 for(const items of [[low,high],[row(),row()]]){app=await ready(page(items));assert.equal(app.text('audit-rows'),'');assert.match(app.text('audit-footer'),/UNAVAILABLE/);}
 app=await ready();app.elements.get('audit-from').value='2026-02-30T00:00';const before=app.requests.length;await app.elements.get('audit-filter').listeners.submit[0]({preventDefault(){}});assert.equal(app.requests.length,before);assert.match(app.text('audit-footer'),/筛选未提交/);
});
test('an exact-read deadline becomes unavailable and re-enables refresh instead of leaving LOADING',async()=>{
 const app=await ready(),done=app.run('adminAudit.detail("'+row().request_id+'");');await auth(app);
 const timer=[...app.timers.values()].find(t=>t.ms===15000);assert.ok(timer);timer.callback();await done;assert.match(app.text('audit-detail-status'),/UNAVAILABLE/);assert.equal(app.elements.get('audit-detail-refresh').disabled,false);assert.equal(app.text('audit-facts'),'');
});
test('fresh descriptor Binding changes erase old-scope rows and abort the pending audit query',async()=>{
 const app=await ready();assert.match(app.text('audit-rows'),/结果未确认/);const done=app.click('audit-refresh');await auth(app,{...cap,bindings:[{binding_id:'99500000-0000-4000-8000-000000000099',label:'Changed fixture scope'}]});await done;assert.equal(app.text('audit-rows'),'');assert.equal(app.text('audit-facts'),'');assert.equal(app.elements.get('audit-empty').hidden,true);assert.match(app.text('audit-footer'),/等待查询/);
});
