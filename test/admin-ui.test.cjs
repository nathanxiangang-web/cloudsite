'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {workspace,principal,descriptor,tick,web}=require('./admin-dom.cjs');
const file={binding_id:descriptor.bindings[0].binding_id,resource_id:'stable-resource',name:'真实文档.md',path:'/文档/真实文档.md',directory:false,size_bytes:321,presence:'PRESENT',copy_id:'stable-copy',copy_availability:'PRESENT'};
const task={manifest_id:'97000000-0000-4000-8000-000000000005',job_id:'stable-job',manifest_state:'AWAITING_CANONICAL',job_state:'RETRY_WAIT',result_copy_id:null,claim_attempts:3,failure_count:0,created_at:'2026-10-20T00:00:00Z',updated_at:'2026-10-20T00:01:00Z'};
async function ready(){const app=workspace();await app.drain();return app;}
test('overview is descriptor-only; nine destinations keep real and unavailable scopes distinct',async()=>{
 const app=await ready();assert.equal(app.elements.get('overview-panel').hidden,false);assert.equal(app.elements.get('overview-schema').textContent,'v15');assert.equal(app.elements.get('overview-bindings').textContent,'1');
 assert.equal(app.requests.some(r=>r.url.startsWith('/api/v1/resources')||r.url.startsWith('/api/v1/tasks')),false);
 for(const id of ['overview','users','resources','downloads','storage','indexcore','shares','audit','settings']){app.click('nav-'+id);await app.drain();assert.equal(app.run('state.tab'),id);assert.equal(app.elements.get('workspace-view').hidden,false);}
 for(const id of ['users','storage','indexcore','shares','audit','settings']){app.click('nav-'+id);assert.equal(app.elements.get('planned-panel').hidden,false);assert.match(app.text('planned-title'),/尚未接入/);assert.equal(app.elements.get('planned-action').disabled,true);await app.drain();}
 app.click('nav-downloads');assert.equal(app.elements.get('global-tasks-placeholder').hidden,false);assert.equal(app.elements.get('tasks-panel').hidden,true);await app.drain();
 app.click('nav-tasks');await app.drain();assert.equal(app.elements.get('tasks-panel').hidden,false);assert.match(app.text('tasks-footer'),/仅当前登录者/);
 assert.ok(app.requests.every(r=>['/api/v1/auth/me','/api/v1/admin/workspace','/api/v1/resources','/api/v1/tasks'].includes(r.url.split('?')[0])));
 assert.ok(app.requests.every(r=>r.options.method==='GET'&&r.options.credentials==='same-origin'&&r.options.cache==='no-store'));
});
test('anonymous, USER, non-JSON 401 and ADMIN_REQUIRED never expose admin facts',async()=>{
 for(const [status,code] of [[401,'AUTH_REQUIRED'],[403,'ADMIN_REQUIRED']]){
  const app=workspace();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();const r=app.pending('/api/v1/admin/workspace');
  r.done=true;r.resolve({ok:false,status,json:async()=>{throw Error('proxy HTML');}});await tick();
  assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('identity-facts'),'');assert.equal(app.text('overview-schema'),'—');assert.match(app.text('login-error'),status===401?/登录已失效/:/仅供管理员/);
 }
 const app=workspace();app.respond(app.pending('/api/v1/auth/me'),{...principal,role:'USER'});await app.drain();assert.equal(app.elements.get('workspace-view').hidden,true);
});
test('login keeps six-character password contract and no token storage; logout clears immediately',async()=>{
 const app=workspace();app.respond(app.pending('/api/v1/auth/me'),{code:'AUTH_REQUIRED'},401);await tick();
 app.elements.get('login-name').value='admin';app.elements.get('login-password').value='abcdef';
 const login=app.elements.get('login-form').listeners.submit[0]({preventDefault(){}});const request=app.pending('/api/v1/auth/login');
 assert.deepEqual(JSON.parse(request.options.body),{login:'admin',password:'abcdef'});app.respond(request,principal);await app.drain();await login;
 app.ctx.page={items:[file],next_cursor:'cursor'};app.run('renderResources(page);resourceDetail(page.items[0]);');app.click('logout');
 assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('file-title'),'');assert.equal(app.text('identity-facts'),'');assert.equal(app.run('state.resourceNext'),'');assert.ok(app.pending('/api/v1/auth/logout'));
 assert.match(app.html,/minlength="6"/);const js=fs.readFileSync(path.join(web,'app.js'),'utf8');
 for(const forbidden of ['localStorage','sessionStorage','innerHTML','document.cookie','StartDownload','/api/v1/admin/users'])assert.equal(js.includes(forbidden),false,forbidden);
});
test('late initial auth, login, failed requests and previous-session replies cannot revive or clear a new session',async()=>{
 const app=workspace(),initial=app.pending('/api/v1/auth/me');app.run('signedOut();');
 app.respond(initial,principal);await tick();assert.equal(app.pending('/api/v1/admin/workspace'),undefined);
 app.elements.get('login-name').value='a';app.elements.get('login-password').value='abcdef';const login=app.elements.get('login-form').listeners.submit[0]({preventDefault(){}});
 const late=app.pending('/api/v1/auth/login');app.run('signedOut();');app.respond(late,principal);await login;assert.equal(app.elements.get('workspace-view').hidden,true);
});
test('role downgrade and principal switch clear all private caches before a read is issued',async()=>{
 for(const identity of [{...principal,role:'USER'},{...principal,user_id:'other-user'}]){
  const app=await ready();app.ctx.page={items:[file],next_cursor:'next'};app.run('renderResources(page);resourceDetail(page.items[0]);loadResources();');
  app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),identity);await tick();
  assert.equal(app.elements.get('workspace-view').hidden,true);assert.equal(app.text('file-id'),'');assert.equal(app.run('state.resources.length'),0);assert.equal(app.pending(/^\/api\/v1\/resources\?/),undefined);
 }
});
test('resource browse preserves binding, parent, opaque cursor and page-local classification',async()=>{
 const app=await ready();app.click('nav-resources');await app.drain({'/api/v1/resources':{items:[file],next_cursor:'next-page'}});
 assert.equal(app.text('resource-count'),'1');assert.equal(app.text('count-document'),'1');assert.match(app.text('resources-footer'),/不含其他分页/);
 app.click('more-resources');await app.drain();const last=app.requests.filter(r=>r.url.startsWith('/api/v1/resources?')).at(-1);assert.match(last.url,/binding_id=97000000/);assert.match(last.url,/cursor=next-page/);assert.doesNotMatch(last.url,/whole_root/);
 app.ctx.folder={...file,resource_id:'folder-id',directory:true,path:'/文档'};app.run('openResource(folder);');await app.drain();assert.match(app.requests.filter(r=>r.url.startsWith('/api/v1/resources?')).at(-1).url,/parent_resource_id=folder-id/);assert.equal(app.text('directory-label'),'/文档');
 app.click('directory-back');await app.drain();assert.equal(app.text('directory-label'),'根层级');
 const before=app.requests.length;app.elements.get('binding-select').value='forged';app.elements.get('binding-select').listeners.change[0]();assert.equal(app.requests.length,before);
 app.ctx.item={...file,name:'<img onerror=x>.md',asset_id:'asset',release_id:'release',variant_id:'variant'};app.run('resourceDetail(item);');
 assert.equal(app.text('file-title'),'<img onerror=x>.md');assert.match(app.text('resource-identities'),/asset.*release.*variant/s);assert.match(app.text('resource-identities'),/DTO 未提供/);assert.equal(app.text('file-id'),'stable-resource');
});
test('loading, true empty, source failure and stale same-page results remain distinct',async()=>{
 const app=await ready();app.click('nav-resources');assert.match(app.text('resources-footer'),/LOADING/);
 await app.drain();assert.equal(app.elements.get('resources-empty').hidden,false);
 app.ctx.page={items:[file],next_cursor:'next'};app.run('renderResources(page);');app.click('refresh-resources');
 app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();
 app.respond(app.pending(/^\/api\/v1\/resources\?/),{code:'READ_UNAVAILABLE'},503);await tick();assert.match(app.text('resources-footer'),/STALE/);assert.equal(app.elements.get('resource-rows').children.length,1);assert.equal(app.elements.get('resources-empty').hidden,true);
 app.ctx.two={...descriptor,bindings:[...descriptor.bindings,{binding_id:'second',label:'Second'}]};app.run('renderWorkspace(two,state.principal);');app.elements.get('binding-select').value='second';app.elements.get('binding-select').listeners.change[0]();
 app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();app.respond(app.pending(/^\/api\/v1\/resources\?/),{code:'READ_UNAVAILABLE'},503);await tick();
 assert.equal(app.elements.get('resource-rows').children.length,0);assert.doesNotMatch(app.text('resources-footer'),/STALE/);
});
test('late resource and exact-path replies are fenced by navigation, binding and logout generations',async()=>{
 for(const transition of ["selectTab('users');","signedOut();","clearResources();"]){
  const app=await ready();app.elements.get('resolve-path').value='/test.md';
  const done=app.elements.get('resolve-form').listeners.submit[0]({preventDefault(){}});
  app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();
  const request=app.pending(/^\/api\/v1\/resources\/resolve\?/);app.run(transition);app.respond(request,file);await done;assert.equal(app.elements.get('file-detail').hidden,true);assert.equal(app.text('file-id'),'');
 }
});
test('task details are allowlisted, null remains unknown, newest request wins, close/Escape restores focus',async()=>{
 const app=await ready();app.click('nav-tasks');await app.drain({'/api/v1/tasks':{items:[task],next_cursor:'next-task'}});
 assert.match(app.text('task-rows'),/等待规范确认/);assert.equal(app.run('state.taskNext'),'next-task');
 app.elements.get('refresh-tasks').focus();const first=app.run("taskDetail('one');");await app.drain({'/api/v1/tasks/one':{...task,manifest_id:'one',source:'PRIVATE-SOURCE',provider_reference:'PRIVATE-REF'}});
 await first;assert.match(app.text('task-facts'),/one/);assert.match(app.text('task-facts'),/未提供/);assert.doesNotMatch(app.text('task-facts'),/PRIVATE/);
 app.events.keydown[0]({key:'Escape'});assert.equal(app.elements.get('task-detail').hidden,true);assert.equal(app.document.activeElement.id,'refresh-tasks');
 const old=app.run("taskDetail('old');");app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();const late=app.pending('/api/v1/tasks/old');
 const newer=app.run("taskDetail('new');");app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();
 app.respond(app.pending('/api/v1/tasks/new'),{...task,manifest_id:'new'});await newer;
 app.respond(late,{...task,manifest_id:'old'});await old;assert.match(app.text('task-facts'),/new/);assert.doesNotMatch(app.text('task-facts'),/old/);
});
test('drawer has modal keyboard containment, Escape/backdrop/focus return and resize cleanup',async()=>{
 const app=await ready();app.click('menu-open');assert.equal(app.run('state.drawer'),true);assert.equal(app.elements.get('main-shell').inert,true);assert.equal(app.document.activeElement.id,'menu-close');
 app.elements.get('logout').focus();let prevented=false;app.events.keydown[0]({key:'Tab',shiftKey:false,preventDefault(){prevented=true;}});assert.ok(prevented);assert.equal(app.document.activeElement.id,'menu-close');
 app.events.keydown[0]({key:'Escape'});assert.equal(app.run('state.drawer'),false);assert.equal(app.document.activeElement.id,'menu-open');assert.equal(app.elements.get('main-shell').inert,false);
 app.click('menu-open');app.click('menu-backdrop');assert.equal(app.run('state.drawer'),false);
 app.click('menu-open');app.windowEvents.resize[0]();assert.equal(app.run('state.drawer'),false);
 const css=fs.readFileSync(path.join(web,'app.css'),'utf8');assert.match(css,/@media\(max-width:767px\)/);assert.match(css,/232px/);assert.match(css,/overflow:auto/);assert.match(css,/\[hidden\]/);
});
test('empty configured bindings degrade honestly and focus/visibility revalidate current server authorization',async()=>{
 const app=workspace();await app.drain({'/api/v1/admin/workspace':{...descriptor,bindings:[]}});assert.equal(app.text('overview-bindings'),'0');app.click('nav-resources');assert.match(app.text('resources-footer'),/未配置/);assert.equal(app.elements.get('resolve-submit').disabled,true);
 app.windowEvents.focus[0]();const request=app.pending('/api/v1/admin/workspace');app.respond(request,{code:'ADMIN_REQUIRED'},403);await tick();assert.equal(app.elements.get('workspace-view').hidden,true);
 app.document.visibilityState='visible';app.events.visibilitychange[0]();assert.equal(app.requests.filter(r=>!r.done).length,0);
});
test('late unauthorized old-session response cannot sign out a newly established identity',async()=>{
 const app=await ready(),request=app.run("api('/api/v1/tasks?limit=20').catch(()=>null);"),late=app.pending('/api/v1/tasks?limit=20');
 app.run('signedOut();');const generation=app.run('state.session');
 app.ctx.p=principal;const login=app.run('signedIn(p);');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await login;
 app.respond(late,{code:'AUTH_REQUIRED'},401);await request;
 assert.equal(app.run('state.session'),generation);assert.equal(app.elements.get('workspace-view').hidden,false);assert.match(app.text('identity-facts'),/test-admin/);
});
test('resource browse requests and task detail cannot land after navigation or logout',async()=>{
 const app=await ready();app.click('nav-resources');app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();
 const late=app.pending(/^\/api\/v1\/resources\?/);app.click('nav-users');app.respond(late,{items:[file],next_cursor:'stale'});await tick();assert.equal(app.elements.get('resource-rows').children.length,0);
 await app.drain();const detail=app.run("taskDetail('hidden');");app.respond(app.pending('/api/v1/admin/workspace'),descriptor);await tick();app.respond(app.pending('/api/v1/auth/me'),principal);await tick();
 const taskRead=app.pending('/api/v1/tasks/hidden');app.run('signedOut();');assert.equal(taskRead.options.signal.aborted,true);app.respond(taskRead,task);await detail;
 assert.equal(app.text('task-facts'),'');assert.equal(app.elements.get('task-detail').hidden,true);
});
test('descriptor refresh failure retains values as stale and never invents zero metrics',async()=>{
 const app=await ready();const refresh=app.run('refreshWorkspace();');app.respond(app.pending('/api/v1/admin/workspace'),{code:'READ_UNAVAILABLE'},503);await refresh;
 assert.equal(app.text('overview-bindings'),'1');assert.equal(app.text('overview-schema'),'v15');assert.match(app.text('workspace-status'),/STALE/);assert.equal(app.elements.get('workspace-error').hidden,false);
});
