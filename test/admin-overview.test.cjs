'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {workspace,principal,descriptor,tick,web}=require('./admin-dom.cjs');
const fixture=require('./fixtures/admin-overview.json');
const dto=()=>structuredClone(fixture);
const users={items:[],next_cursor:'',total:42,read_at:fixture.read_at,scope:'FILTERED_PRODUCT_USERS'};
async function ready(){const app=workspace();await app.drain({'/api/v1/admin/overview':dto(),'/api/v1/admin/users':users});return app;}
// Resolve every verification GET, leaving aggregate requests under test control.
async function verified(app){for(let i=0;i<5;i++){await tick();for(const r of app.requests.filter(r=>!r.done&&(r.url==='/api/v1/admin/workspace'||r.url==='/api/v1/auth/me')))app.respond(r,r.url.endsWith('/me')?principal:descriptor);}}
async function refresh(app,value=dto(),userPage=users){const done=app.click('refresh-workspace');await app.drain({'/api/v1/admin/overview':value,'/api/v1/admin/users':userPage});await done;}
function zero(){const value=dto();for(const fields of [value.manifests,value.jobs])for(const key of Object.keys(fields))fields[key]=0;return value;}

test('initial homepage automatically reads verified ADMIN GET aggregates, separate denominators and provenance',async()=>{
 const app=await ready();assert.equal(app.text('overview-users-total'),'42');assert.equal(app.text('overview-manifests-total'),'15');assert.equal(app.text('overview-jobs-total'),'14');
 assert.equal(app.text('overview-manifests-incomplete'),'6');assert.equal(app.text('overview-manifests-failed'),'2');assert.equal(app.text('overview-manifests-recovery_required'),'2');assert.equal(app.text('overview-jobs-recovery_required'),'2');assert.match(app.text('overview-status'),/需要人工处理/);
 assert.match(app.text('overview-read-at'),/数据库读取于/);assert.match(app.html,/PRODUCT_ACQUISITIONS_IN_CONFIGURED_BINDINGS/);
 assert.equal(app.requests.filter(r=>r.url==='/api/v1/admin/overview').length,1);assert.ok(app.requests.every(r=>r.options.method==='GET'&&r.options.credentials==='same-origin'&&r.options.cache==='no-store'));
 assert.equal(app.requests.some(r=>/probe|indexcore|resources|tasks|replay/.test(r.url)),false);
});
test('loading and true zero are distinct, neither claims provider health',async()=>{
 const app=await ready(),done=app.click('refresh-workspace');assert.equal(app.text('overview-manifests-total'),'—');assert.match(app.text('overview-status'),/LOADING/);assert.equal(app.elements.get('overview-panel').attrs['aria-busy'],'true');
 await app.drain({'/api/v1/admin/overview':zero(),'/api/v1/admin/users':{...users,total:0}});await done;
 assert.equal(app.text('overview-manifests-total'),'0');assert.equal(app.text('overview-jobs-total'),'0');assert.equal(app.text('overview-users-total'),'0');assert.match(app.text('overview-status'),/已更新/);assert.equal(app.elements.get('overview-panel').attrs['aria-busy'],'false');
});
test('all required counts reject negative, unsafe, fractional, string, null or missing values',async()=>{
 const app=await ready();for(const section of ['manifests','jobs'])for(const key of Object.keys(fixture[section]))for(const bad of [-1,Number.MAX_SAFE_INTEGER+1,0.5,'0',null,undefined]){
  const value=dto();value[section][key]=bad;await refresh(app,value);assert.equal(app.text('overview-manifests-total'),'—',section+'.'+key+'='+bad);assert.match(app.text('overview-status'),/UNAVAILABLE/);
 }
});
test('source, scope, date and filtered owner are validated; arbitrary extra fields never rendered',async()=>{
 const app=await ready();for(const patch of [{source:'OTHER'},{scope:'OTHER'},{read_at:null},{read_at:'invalid'},{read_at:'2026-02-30T00:00:00Z'},{read_at:'0001-01-01T00:00:00Z'},{read_at:123},{owner_user_id:principal.user_id},{manifests:null},{jobs:[]},{source:undefined}]){
  await refresh(app,{...dto(),...patch});assert.equal(app.text('overview-manifests-total'),'—');assert.match(app.text('overview-status'),/UNAVAILABLE/);
 }
 await refresh(app,{...dto(),secret:'SECRET',manifests:{...dto().manifests,token:'SECRET'}});assert.equal(app.text('overview-manifests-total'),'15');assert.equal(app.elements.has('overview-manifests-token'),false);
});
test('every bucket sum, incomplete definition and paired terminal denominator is checked',async()=>{
 const app=await ready();const changes=[v=>v.manifests.total++,v=>v.jobs.total++,v=>v.manifests.incomplete++,v=>{v.jobs.succeeded--;v.jobs.running++;},v=>{v.jobs.failed--;v.jobs.running++;},v=>{v.jobs.canceled--;v.jobs.running++;},v=>{v.jobs.recovery_required--;v.jobs.running++;},v=>{v.jobs.queued=4;v.jobs.running=0;v.jobs.retry_wait=1;},v=>{for(const k of ['pending','active'])v.manifests[k]=Number.MAX_SAFE_INTEGER;}];
 for(const change of changes){const value=dto();change(value);await refresh(app,value);assert.match(app.text('overview-status'),/UNAVAILABLE/);assert.equal(app.text('overview-jobs-total'),'—');}
});
test('503, transport failure and invalid JSON clear old values and timestamp, without fabricating zero',async()=>{
 for(const kind of ['503','network','json']){const app=await ready(),done=app.click('refresh-workspace');await verified(app);const r=app.pending('/api/v1/admin/overview');
  if(kind==='503')app.respond(r,{code:'READ_UNAVAILABLE'},503);else{r.done=true;if(kind==='network')r.reject(Error('transport'));else r.resolve({ok:true,status:200,json:async()=>{throw Error('invalid JSON');}});}
  app.respond(app.pending('/api/v1/admin/users?limit=1'),users);await done;assert.match(app.text('overview-status'),/UNAVAILABLE/);assert.equal(app.text('overview-manifests-total'),'—');assert.doesNotMatch(app.text('overview-read-at'),/数据库读取于/);assert.equal(app.text('overview-users-total'),'42');
 }
});
test('authoritative DirectoryReads total rejects malformed pages, never counts samples',async()=>{
 const app=await ready();for(const patch of [{total:undefined},{total:-1},{total:1.5},{total:'0'},{total:Number.MAX_SAFE_INTEGER+1},{scope:'OTHER'},{read_at:null},{read_at:'invalid'},{items:[{},{}]},{next_cursor:'offset:2'}]){await refresh(app,dto(),{...users,...patch});assert.equal(app.text('overview-users-total'),'—');assert.match(app.text('overview-users-status'),/UNAVAILABLE/);assert.equal(app.text('overview-manifests-total'),'15');}
});
test('recovery is not incomplete, failure, provider state or retry estimate',async()=>{
 const app=await ready(),value=zero();value.manifests.total=1;value.manifests.recovery_required=1;value.jobs.total=1;value.jobs.recovery_required=1;await refresh(app,value);
 assert.equal(app.text('overview-manifests-incomplete'),'0');assert.equal(app.text('overview-manifests-failed'),'0');assert.equal(app.text('overview-manifests-recovery_required'),'1');assert.match(app.text('overview-status'),/需要人工处理/);
});
test('latest refresh wins even when both old GETs finish later',async()=>{
 const app=await ready(),oldDone=app.click('refresh-workspace');await verified(app);const oldTask=app.pending('/api/v1/admin/overview'),oldUser=app.pending('/api/v1/admin/users?limit=1');
 const newDone=app.click('refresh-workspace');await verified(app);const nextTask=app.requests.find(r=>!r.done&&r!==oldTask&&r.url==='/api/v1/admin/overview'),nextUser=app.requests.find(r=>!r.done&&r!==oldUser&&r.url==='/api/v1/admin/users?limit=1');
 app.respond(nextTask,zero());app.respond(nextUser,{...users,total:7});await newDone;app.respond(oldTask,dto());app.respond(oldUser,users);await oldDone;
 assert.equal(oldTask.options.signal.aborted,true);assert.equal(app.text('overview-manifests-total'),'0');assert.equal(app.text('overview-users-total'),'7');
});
test('tab leave/return, logout and principal switch fence late overview and user results',async()=>{
 for(const boundary of ["selectTab('resources');","selectTab('users');selectTab('overview',false);","signedOut();","signedOut();state.principal={user_id:'new-admin',role:'ADMIN'};"]){
  const app=await ready(),done=app.click('refresh-workspace');await verified(app);const old=app.pending('/api/v1/admin/overview'),oldUser=app.pending('/api/v1/admin/users?limit=1');app.run(boundary);
  app.respond(old,dto());app.respond(oldUser,users);await done;assert.notEqual(app.text('overview-manifests-total'),'15');assert.notEqual(app.text('overview-users-total'),'42');
 }
});
test('no principal or non-ADMIN never issues aggregate GET',async()=>{
 for(const p of [null,{user_id:'user',role:'USER'}]){const app=workspace();app.ctx.p=p;app.run('state.principal=p;adminOverview.load();');await tick();assert.equal(app.requests.some(r=>r.url==='/api/v1/admin/overview'||r.url==='/api/v1/admin/users?limit=1'),false);}
});
test('revoked auth or role clears all data even for non-JSON proxy errors',async()=>{
 for(const code of [401,403]){const app=await ready(),done=app.click('refresh-workspace');await verified(app);const r=app.pending('/api/v1/admin/overview');r.done=true;r.resolve({status:code,ok:false,json:async()=>{throw Error('proxy HTML');}});app.respond(app.pending('/api/v1/admin/users?limit=1'),users);await done;
  assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('overview-manifests-total'),'—');assert.equal(app.text('overview-users-total'),'—');
 }
});
test('different principal during verification clears data before issuing aggregates',async()=>{
 const app=await ready(),done=app.click('refresh-workspace');for(const r of app.requests.filter(r=>!r.done&&r.url==='/api/v1/admin/workspace'))app.respond(r,descriptor);await tick();for(const r of app.requests.filter(r=>!r.done&&r.url==='/api/v1/auth/me'))app.respond(r,{...principal,user_id:'other'});await done;
 assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.requests.filter(r=>r.url==='/api/v1/admin/overview').length,1);
});
test('quick links go only to implemented panels and never invoke writes or automatic probes',async()=>{
 const app=await ready();for(const [button,tab] of [['open-users','users'],['open-admin-downloads','downloads'],['open-shares','shares'],['open-resources','resources'],['open-my-tasks','tasks'],['open-diagnostics','indexcore'],['open-storage','storage']]){app.click(button);await app.drain();assert.equal(app.run('state.tab'),tab);}
 assert.ok(app.requests.every(r=>r.options.method==='GET'));assert.equal(app.requests.some(r=>/\/probe|\/api\/v1\/admin\/indexcore/.test(r.url)),false);
 assert.doesNotMatch(app.html,/受控管理|刷新能力描述|任务无聚合端点/);
 const ids=Array.from(app.html.matchAll(/\bid="([^"]+)"/g),m=>m[1]);assert.equal(ids.length,new Set(ids).size);
 const css=fs.readFileSync(path.join(web,'app.css'),'utf8');assert.match(css,/@media\(max-width:767px\).*operator-metrics\{grid-template-columns:1fr\}/);assert.match(app.html,/id="overview-status"[^>]*role="status"/);
});
