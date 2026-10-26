'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {workspace,principal,descriptor,tick}=require('./admin-dom.cjs');
const fixture=require('./fixtures/admin-download-detail.json');
const dto=()=>structuredClone(fixture);
async function ready(){const app=workspace();await app.drain();app.click('nav-downloads');await app.drain();return app;}
async function auth(app){app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();}
async function open(app,value=dto()){const done=app.run('adminDownloadDetail.load("'+fixture.manifest_id+'");');await auth(app);app.respond(app.pending('/api/v1/admin/downloads/'+fixture.manifest_id),value);await done;}
test('exact ADMIN detail uses GET, renders safe linkage/error and never invokes owner-only task or provider ports',async()=>{
 const app=await ready();await open(app,{...dto(),source_ref:'SECRET',last_error:'SECRET',provider_ref:'SECRET'});
 const text=app.text('admin-download-facts');assert.match(text,/POLL_DECODE/);assert.match(text,/RECOVERY_REQUIRED/);assert.match(text,/不.*自动重试/);assert.match(text,/9 \/ 2/);assert.doesNotMatch(text,/SECRET/);assert.equal(app.elements.get('admin-download-detail').hidden,false);assert.ok(app.requests.every(r=>r.options.method==='GET'));assert.equal(app.requests.some(r=>r.url.startsWith('/api/v1/tasks/')),false);
});
test('exact lookup validates UUID; row button opens the same detail and Escape returns focus',async()=>{
 const app=await ready();app.elements.get('admin-download-id').value='bad';const before=app.requests.length;await app.elements.get('admin-download-lookup').listeners.submit[0]({preventDefault(){}});assert.equal(app.requests.length,before);
 const loaded=app.run('adminLists.downloads.load();');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),{items:[dto()],next_cursor:''});await loaded;
 const button=app.elements.get('admin-downloads-rows').children[0].children[0].children[0];button.focus();const done=button.listeners.click[0]();await auth(app);app.respond(app.pending('/api/v1/admin/downloads/'+fixture.manifest_id),dto());await tick();await done;
 app.events.keydown[0]({key:'Escape'});assert.equal(app.elements.get('admin-download-detail').hidden,true);assert.equal(app.document.activeElement,button);
});
test('server-side status and owner filters retain opaque cursor and clear on filter changes',async()=>{
 const app=await ready();const done=app.run('adminLists.downloads.filter("'+fixture.owner_user_id+'","RECOVERY_REQUIRED");');await auth(app);const r=app.pending(/\/admin\/downloads\?/);assert.match(r.url,/status=RECOVERY_REQUIRED/);assert.match(r.url,/user_id=/);app.respond(r,{items:[dto()],next_cursor:'eyJjb250aW51ZSI6MX0'});await done;
 const next=app.click('more-admin-downloads');await auth(app);const page=app.pending(/\/admin\/downloads\?/);assert.match(page.url,/cursor=eyJjb250aW51ZSI6MX0/);assert.match(page.url,/status=RECOVERY_REQUIRED/);app.respond(page,{items:[],next_cursor:''});await next;
 const cleared=app.click('admin-filter-clear');await app.drain();await cleared;assert.equal(app.run('adminLists.downloads.cursor'),'');assert.equal(app.run('adminLists.downloads.status'),'ALL');
});
test('PENDING has no fabricated Job, READY may legitimately reference a REMOVED Copy',async()=>{
 const app=await ready(),p=dto();Object.assign(p,{manifest_state:'PENDING',job_id:null,job_state:null,result_copy_id:null,claim_attempts:0,failure_count:0,max_attempts:0,job_created_at:null,job_updated_at:null,started_at:null,finished_at:null,next_attempt_at:null,error_recorded:false,error_code:'NONE'});await open(app,p);assert.match(app.text('admin-download-facts'),/未创建 Job/);
 const r=dto();Object.assign(r,{manifest_state:'READY',job_state:'SUCCEEDED',result_copy_id:'97000000-0000-4000-8000-000000000007',error_recorded:false,error_code:'NONE',copy:{copy_id:'97000000-0000-4000-8000-000000000007',binding_id:r.binding_id,availability:'REMOVED',updated_at:r.updated_at}});await open(app,r);assert.match(app.text('admin-download-facts'),/REMOVED/);assert.match(app.text('admin-download-facts'),/历史获取已成功/);
});
test('malformed detail never commits partial facts or guessed success',async()=>{
 const app=await ready();for(const patch of [{manifest_id:'97000000-0000-4000-8000-000000000008'},{source:'OTHER'},{read_at:'bad'},{manifest_state:'UNKNOWN'},{job_state:'SUCCEEDED'},{claim_attempts:-1},{failure_count:6},{max_attempts:1},{job_updated_at:null},{error_recorded:'true'},{error_code:'Cookie=SECRET'},{copy:{}},{job_created_at:'bad'},{started_at:'2030-01-01T00:00:00Z'}]){await open(app,{...dto(),...patch});assert.equal(app.text('admin-download-facts'),'');assert.match(app.text('admin-download-detail-status'),/UNAVAILABLE/);}
});
test('404 vs 503/network distinguish not-found from unavailable, clearing prior facts',async()=>{
 for(const kind of [404,503,'network']){const app=await ready();await open(app);const done=app.click('admin-download-detail-refresh');assert.equal(app.text('admin-download-facts'),'');await auth(app);const r=app.pending('/api/v1/admin/downloads/'+fixture.manifest_id);if(kind==='network'){r.done=true;r.reject(Error('SECRET'));}else app.respond(r,{code:kind===404?'DOWNLOAD_NOT_FOUND':'READ_UNAVAILABLE'},kind);await done;assert.match(app.text('admin-download-detail-status'),kind===404?/NOT_FOUND/:/UNAVAILABLE/);assert.equal(app.text('admin-download-facts'),'');}
});
test('late detail is fenced by tab, owner/status filter, logout, principal change and newer detail',async()=>{
 for(const transition of ["selectTab('users');","adminLists.downloads.filter('','FAILED');","signedOut();","signedOut();state.principal={user_id:'other',role:'ADMIN'};","adminDownloadDetail.load('97000000-0000-4000-8000-000000000008');"]){const app=await ready(),done=app.run('adminDownloadDetail.load("'+fixture.manifest_id+'");');await auth(app);const r=app.pending('/api/v1/admin/downloads/'+fixture.manifest_id);app.run(transition);app.respond(r,dto());await done;assert.equal(app.text('admin-download-facts'),'');}
});
test('list rejects malformed pairs, duplicated IDs, mismatched filters and preserves honest STALE on refresh failure',async()=>{
 const app=await ready();for(const value of [{...dto(),manifest_state:'UNKNOWN'},{...dto(),job_id:null},{...dto(),result_copy_id:'bad'},{...dto(),failure_count:-1}]){const done=app.run('adminLists.downloads.load();');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),{items:[value],next_cursor:''});await done;assert.equal(app.text('admin-downloads-rows'),'');}
 const load=app.run('adminLists.downloads.load();');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),{items:[dto()],next_cursor:''});await load;const refresh=app.click('refresh-admin-downloads');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),{code:'READ_UNAVAILABLE'},503);await refresh;assert.match(app.text('admin-downloads-footer'),/STALE/);assert.match(app.text('admin-downloads-rows'),/RECOVERY_REQUIRED/);
});
test('authorization loss clears open detail; no read without current ADMIN',async()=>{
 for(const status of [401,403]){const app=await ready();await open(app);const done=app.click('admin-download-detail-refresh');await auth(app);app.respond(app.pending('/api/v1/admin/downloads/'+fixture.manifest_id),{code:status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED'},status);await done;assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('admin-download-facts'),'');}
 for(const value of [null,{user_id:'other',role:'USER'}]){const app=await ready();app.ctx.p=value;const before=app.requests.length;app.run('state.principal=p;adminDownloadDetail.load("'+fixture.manifest_id+'");');await tick();assert.equal(app.requests.length,before);}
});
test('newest same-ID refresh wins over a late older result',async()=>{
 const app=await ready(),older=app.run('adminDownloadDetail.load("'+fixture.manifest_id+'");');await auth(app);const old=app.pending('/api/v1/admin/downloads/'+fixture.manifest_id);
 const newer=app.click('admin-download-detail-refresh');await auth(app);const next=app.requests.find(r=>!r.done&&r!==old&&r.url==='/api/v1/admin/downloads/'+fixture.manifest_id);app.respond(next,{...dto(),error_code:'POLL_HTTP_STATUS'});await newer;app.respond(old,dto());await older;assert.match(app.text('admin-download-facts'),/POLL_HTTP_STATUS/);assert.doesNotMatch(app.text('admin-download-facts'),/POLL_DECODE/);
});
test('duplicate list rows and owner/status mismatch fail before painting any rows',async()=>{
 const app=await ready();for(const page of [{items:[dto(),dto()],next_cursor:''},{items:[{...dto(),created_at:'2026-02-30T00:00:00Z'}],next_cursor:''},{items:[dto()],next_cursor:'bad:cursor'}]){const done=app.run('adminLists.downloads.load();');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),page);await done;assert.equal(app.text('admin-downloads-rows'),'');}
 const done=app.run('adminLists.downloads.filter("97000000-0000-4000-8000-000000000008","READY");');await auth(app);app.respond(app.pending(/\/admin\/downloads\?/),{items:[dto()],next_cursor:''});await done;assert.equal(app.text('admin-downloads-rows'),'');
});
