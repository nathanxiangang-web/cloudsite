'use strict';
const test=require('node:test'),assert=require('node:assert/strict');
const {workspace,principal,descriptor,tick}=require('./admin-dom.cjs');
const user={user_id:'98000000-0000-4000-8000-000000000001',login:'other-user',role:'USER',enabled:false,created_at:'2026-10-20T00:00:00Z',updated_at:'2026-10-21T00:00:00Z'};
const task={manifest_id:'98000000-0000-4000-8000-000000000002',owner_user_id:user.user_id,binding_id:descriptor.bindings[0].binding_id,job_id:null,manifest_state:'PENDING',job_state:null,result_copy_id:null,claim_attempts:0,failure_count:0,created_at:user.created_at,updated_at:user.updated_at};
async function ready(){const app=workspace();await app.drain();return app;}
async function reach(app,kind){app.click('nav-'+(kind==='users'?'users':'downloads'));app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();return app.pending(new RegExp('^/api/v1/admin/'+kind+'\\?'));}
test('UI2A real user/download pages use admin endpoints, allowlisted columns and client timestamps',async()=>{
 const app=await ready();let r=await reach(app,'users');assert.match(r.url,/limit=20/);app.respond(r,{items:[{...user,password_hash:'SECRET',session:'SECRET'}],next_cursor:user.user_id});await tick();
 assert.equal(app.elements.get('users-panel').hidden,false);assert.match(app.text('users-rows'),/other-user/);assert.match(app.text('users-rows'),/停用/);assert.doesNotMatch(app.text('users-rows'),/SECRET/);assert.match(app.text('users-footer'),/客户端读取于.*非数据库观察时间/);assert.match(app.text('users-footer'),/不代表全站总数/);
 app.click('more-users');await app.drain();assert.match(app.requests.filter(r=>r.url.startsWith('/api/v1/admin/users?')).at(-1).url,/cursor=98000000/);
 r=await reach(app,'downloads');app.respond(r,{items:[{...task,source_ref:'SECRET',provider_reference:'SECRET',last_error:'SECRET'}],next_cursor:''});await tick();
 assert.match(app.text('admin-downloads-rows'),/PENDING/);assert.match(app.text('admin-downloads-rows'),/未创建 Job/);assert.doesNotMatch(app.text('admin-downloads-rows'),/SECRET/);assert.match(app.text('admin-downloads-footer'),/配置 Binding 内全部已归属任务/);
 app.click('nav-tasks');await app.drain();assert.equal(app.elements.get('global-tasks-placeholder').hidden,true);assert.match(app.requests.at(-1).url,/^\/api\/v1\/tasks\?/);
});
test('user task links and explicit UUID filtering never impersonate or accept arbitrary owner input',async()=>{
 const app=await ready();const r=await reach(app,'users');app.respond(r,{items:[user],next_cursor:''});await tick();
 const row=app.elements.get('users-rows').children[0];row.children.at(-1).children[0].listeners.click[0]();await app.drain();
 assert.match(app.requests.filter(r=>r.url.startsWith('/api/v1/admin/downloads?')).at(-1).url,new RegExp('user_id='+user.user_id));assert.equal(app.run('state.principal.user_id'),principal.user_id);
 const before=app.requests.length;app.elements.get('admin-user-filter').value='other-user';app.elements.get('admin-filter-form').listeners.submit[0]({preventDefault(){}});await tick();assert.equal(app.requests.length,before);assert.match(app.text('workspace-error'),/输入不符合/);
 app.click('admin-filter-clear');await app.drain();assert.doesNotMatch(app.requests.filter(r=>r.url.startsWith('/api/v1/admin/downloads?')).at(-1).url,/user_id=/);
});
test('admin pages distinguish empty, failed, stale, new cursor and changed owner context',async()=>{
 for(const [kind,prefix,item] of [['users','users',user],['downloads','admin-downloads',task]]){
  const app=await ready();let r=await reach(app,kind);app.respond(r,{items:[],next_cursor:''});await tick();assert.equal(app.elements.get(prefix+'-empty').hidden,false);
  const first=app.run('adminLists.'+kind+'.refresh();');await app.drain({['/api/v1/admin/'+kind]:{items:[item],next_cursor:item.user_id||item.manifest_id}});await first;
  const refresh=app.run('adminLists.'+kind+'.load();');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();app.respond(app.pending(new RegExp('^/api/v1/admin/'+kind+'\\?')),{code:'READ_UNAVAILABLE'},503);await refresh;
  assert.equal(app.elements.get(prefix+'-rows').children.length,1);assert.equal(app.elements.get(prefix+'-empty').hidden,true);assert.match(app.text(prefix+'-footer'),/STALE/);
  const next=app.run('adminLists.'+kind+'.more();');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();app.respond(app.pending(new RegExp('^/api/v1/admin/'+kind+'\\?')),{code:'READ_UNAVAILABLE'},503);await next;
  assert.equal(app.elements.get(prefix+'-rows').children.length,0);assert.doesNotMatch(app.text(prefix+'-footer'),/STALE/);
 }
});
test('admin read rejection immediately clears every cached list and old owner filter',async()=>{
 for(const status of [401,403]){
  const app=await ready();let r=await reach(app,'users');app.respond(r,{items:[user],next_cursor:user.user_id});await tick();r=await reach(app,'downloads');app.respond(r,{items:[task],next_cursor:task.manifest_id});await tick();app.elements.get('admin-user-filter').value=user.user_id;
  const done=app.run('adminLists.users.load();');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();app.respond(app.pending(/^\/api\/v1\/admin\/users\?/),{code:status===403?'ADMIN_REQUIRED':'AUTH_REQUIRED'},status);await done;
  assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('users-rows'),'');assert.equal(app.text('admin-downloads-rows'),'');assert.equal(app.elements.get('admin-user-filter').value,'');assert.equal(app.run('adminLists.users.next'),'');assert.equal(app.run('adminLists.downloads.next'),'');
 }
});
test('late admin replies cannot repaint after tab change, logout, newer owner filter or new login',async()=>{
 for(const transition of ["selectTab('storage');","signedOut();","signedOut();state.principal={user_id:'new-session'};"]){const app=await ready();const r=await reach(app,'users');app.run(transition);app.respond(r,{items:[user],next_cursor:user.user_id});await tick();assert.equal(app.text('users-rows'),'');}
 const app=await ready();const old=await reach(app,'downloads');const next=app.run('adminLists.downloads.filter("'+user.user_id+'");');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();const newest=app.requests.find(r=>!r.done&&r!==old&&r.url.startsWith('/api/v1/admin/downloads?'));app.respond(newest,{items:[{...task,manifest_id:'newest'}],next_cursor:''});await next;app.respond(old,{items:[{...task,manifest_id:'oldest'}],next_cursor:''});await tick();assert.match(app.text('admin-downloads-rows'),/newest/);assert.doesNotMatch(app.text('admin-downloads-rows'),/oldest/);
});
