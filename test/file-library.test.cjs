'use strict';
const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const path=require('node:path');
const web=path.join(__dirname,'../internal/producthttp/web');
const catalogCode=fs.readFileSync(path.join(web,'file-catalog.js'),'utf8');
const {Catalog}=vm.runInNewContext(catalogCode+'\nFileCatalog;',{});
const file=(id,extra={})=>({binding_id:'one',resource_id:id,name:id+'.pdf',directory:false,presence:'PRESENT',size_bytes:123,...extra});
test('all public HTML surfaces use cloudsite and the exact release version',()=>{
 const version=fs.readFileSync(path.join(__dirname,'..','VERSION'),'utf8').trim();assert.equal(version,'1.0.0');
 for(const name of ['library.html','index.html','share.html','downloads.html']){const html=fs.readFileSync(path.join(web,name),'utf8');assert.ok(html.includes(version),name);assert.ok(html.includes('cloudsite'),name);assert.ok(!/panta/i.test(html),name);}
});
const page=(items=[],next_cursor='')=>({items,next_cursor});

test('six-character password reaches registration; five characters do not',async()=>{
 const app=workspace();app.run('authMode(true)');app.elements.get('account-name').value='six.user';
 const submit=app.elements.get('login-form').listeners.submit[0];
 for(const password of ['abcde','中文密码短']){app.elements.get('account-password').value=password;app.elements.get('account-confirm').value=password;await submit({preventDefault(){}});assert.equal(app.requests.filter(r=>r.url==='/api/v1/auth/register').length,0);assert.ok(app.elements.get('login-error').textContent.includes('6'));}
 app.elements.get('account-password').value='abcdef';app.elements.get('account-confirm').value='abcdef';const pending=submit({preventDefault(){}});const req=app.requests.find(r=>r.url==='/api/v1/auth/register');assert.ok(req);assert.equal(JSON.parse(req.options.body).password,'abcdef');req.resolve({ok:true,status:201,json:async()=>({role:'USER'})});await pending;
 for(const file of ['library.html','index.html']){const html=fs.readFileSync(path.join(web,file),'utf8');assert.ok(!html.includes('minlength="12"'));assert.ok(html.includes('minlength="6"'));}
});

test('registration mode validates confirmation, clears secrets and never requests roles',async()=>{
 const app=workspace();app.run('authMode(true)');assert.equal(app.elements.get('confirm-field').hidden,false);
 app.elements.get('account-name').value='new.user';app.elements.get('account-password').value='test-only-password-123';app.elements.get('account-confirm').value='different-password';
 const submit=app.elements.get('login-form').listeners.submit[0];await submit({preventDefault(){}});assert.ok(app.elements.get('login-error').textContent.includes('不一致'));assert.equal(app.requests.filter(r=>r.url.includes('/register')).length,0);
 app.elements.get('account-confirm').value='test-only-password-123';const pending=submit({preventDefault(){}});const req=app.requests.find(r=>r.url==='/api/v1/auth/register');assert.ok(req);assert.deepEqual(Object.keys(JSON.parse(req.options.body)).sort(),['login','password']);
 req.resolve({ok:true,status:201,json:async()=>({login:'new.user',role:'USER'})});await pending;assert.equal(app.run('registering'),false);assert.ok(app.elements.get('auth-notice').textContent.includes('已创建'));assert.equal(app.elements.get('account-password').value,'');assert.equal(app.elements.get('account-confirm').value,'');assert.equal(app.requests.filter(r=>r.url==='/api/v1/auth/login').length,0);
});

test('registration link is controlled by the public server capability',async()=>{
 for(const enabled of [false,true]){const app=workspace();const req=app.requests.find(r=>r.url==='/api/v1/auth/capabilities');req.resolve({ok:true,status:200,json:async()=>({registration_enabled:enabled})});await new Promise(resolve=>setImmediate(resolve));assert.equal(app.elements.get('auth-toggle').hidden,!enabled);}
});

test('whole-root Q6 inventory continues beyond32 pages without directory traversal',async()=>{
 const c=new Catalog([{binding_id:'one'}],true);let n=0;
 while(!c.complete){await c.round(async scope=>{assert.equal(scope.parent,null);n++;return page([file('folder-'+n,{directory:true}),file('file-'+n)],n<45?'cursor-'+n:'');});}
 assert.equal(c.files.length,45);assert.equal(c.pages,45);assert.equal(c.limited,false);assert.equal(c.scopes.size,1);
});
test('nested folders are internal Q4 reads; only explicit files are collected',async()=>{
 const c=new Catalog([{binding_id:'one'},{binding_id:'two'}]);const calls=[];
 await c.round(async s=>{calls.push([s.binding,s.parent,s.cursor]);if(s.binding==='two')return page([file('same',{binding_id:'two',name:'same.pdf'})]);if(!s.parent)return page([file('same'),file('folder',{directory:true}),file('unknown',{directory:null})]);return page([file('nested')]);});
 assert.equal(c.files.length,3);assert.equal(c.unknown,1);assert.equal(c.complete,true);assert.equal(calls.length,3);assert.equal(calls[2][1],'folder');assert.equal(c.files.filter(f=>f.directory!==false).length,0);
});
test('four pages per action; explicit further rounds follow cursors without looping',async()=>{
 const c=new Catalog([{binding_id:'one'}]);let n=0;
 const read=async()=>{n++;return page([file(String(n))],n<6?'cursor-'+n:'');};
 await c.round(read);assert.equal(n,4);assert.equal(c.files.length,4);assert.equal(c.complete,false);
 await c.round(read);assert.equal(n,6);assert.equal(c.complete,true);
});
test('empty source is complete; absent directory flag is not guessed',async()=>{
 const c=new Catalog([{binding_id:'one'}]);await c.round(async()=>page());assert.equal(c.complete,true);assert.equal(c.files.length,0);
 const x=new Catalog([{binding_id:'one'}]);await x.round(async()=>page([file('fake.exe',{directory:undefined})]));assert.equal(x.files.length,0);assert.equal(x.unknown,1);
});
test('upstream errors preserve incomplete state, not empty success',async()=>{
 const c=new Catalog([{binding_id:'one'}]);await assert.rejects(c.round(async()=>{throw new Error('upstream');}));assert.equal(c.complete,false);assert.equal(c.pages,0);
});
test('malformed/mismatched/duplicated records fail before committing a page',async()=>{
 for(const bad of [null,page([file('x',{binding_id:'outside'})]),page([file('x'),file('x')]),page([file('x',{presence:'REMOVED'})]),{items:[],next_cursor:3},page(Array.from({length:51},(_,n)=>file(String(n))))]){
  const c=new Catalog([{binding_id:'one'}]);await assert.rejects(c.round(async()=>bad));assert.equal(c.pages,0);assert.equal(c.files.length,0);assert.equal(c.complete,false);
 }
});
test('cursor and directory cycles stop without silently accepting duplicates',async()=>{
 const c=new Catalog([{binding_id:'one'}]);let n=0;
 await assert.rejects(c.round(async()=>{n++;return page([file(String(n))],n===1?'a':n===2?'b':'a');}));assert.equal(n,3);assert.equal(c.pages,2);
 const d=new Catalog([{binding_id:'one'}]);await assert.rejects(d.round(async()=>page([file('dir',{directory:true})])));assert.equal(d.pages,1);
});
test('session cancellation drops late results',async()=>{
 const c=new Catalog([{binding_id:'one'}]);let active=true;
 await c.round(async()=>{active=false;return page([file('private')]);},()=>active);assert.equal(c.files.length,0);assert.equal(c.pages,0);
});
test('session page and queue caps disclose incomplete results',async()=>{
 const c=new Catalog([{binding_id:'one'}]);let n=0;
 for(let round=0;round<8;round++)await c.round(async()=>page([file(String(++n))],'next-'+n));
 assert.equal(n,32);assert.equal(c.limited,true);assert.equal(c.complete,false);
 await c.round(async()=>{throw Error('must not read after cap');});
 const d=new Catalog([{binding_id:'one'}]);let id=0;
 for(let round=0;round<2;round++)await d.round(async()=>page(Array.from({length:50},()=>file('dir-'+(++id),{directory:true}))));
 assert.equal(d.limited,true);assert.equal(d.complete,false);assert.ok(d.scopes.size<=256);
});
function workspace(surface='library'){

 const html=fs.readFileSync(path.join(web,surface==='downloads'?'downloads.html':'library.html'),'utf8').replace('<!--FILE_BROWSER-->',fs.readFileSync(path.join(web,'file-browser.html'),'utf8'));const ids=new Set(Array.from(html.matchAll(/id="([^"]+)"/g),m=>m[1]));
 class Element{
  constructor(){this.children=[];this.attrs={};this.listeners={};this.value='';this.hidden=false;this.disabled=false;this.isConnected=true;this._text='';}
  set textContent(value){this._text=String(value);this.children=[];}get textContent(){return this._text;}
  append(...nodes){this.children.push(...nodes);}replaceChildren(...nodes){this._text='';this.children=nodes;}
  addEventListener(type,fn){(this.listeners[type]||=[]).push(fn);}setAttribute(k,v){this.attrs[k]=v;}removeAttribute(k){delete this.attrs[k];delete this[k];}scrollIntoView(){}focus(){this.focused=true;}async play(){this.paused=false;this.played=true;}pause(){this.paused=true;}load(){this.loaded=true;}click(){this.clicked=true;}select(){this.selected=true;}
 }
 const elements=new Map(Array.from(ids,id=>[id,new Element()]));const requests=[],created=[],copied=[],timers=[];
 const ctx=vm.createContext({AbortController,URLSearchParams,TextEncoder,setTimeout,setInterval(callback,ms){timers.push({callback,ms});return timers.length;},crypto:require('node:crypto').webcrypto,navigator:{clipboard:{async writeText(value){copied.push(value);}}},document:{body:{dataset:{fileSurface:surface}},getElementById(id){assert.ok(elements.has(id),'Unknown id '+id);return elements.get(id);},createElement(){const e=new Element();created.push(e);return e;}},fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));}});
 for(const script of ['resource-display.js','file-catalog.js','file-surface.js','library.js','library-sharing.js'])vm.runInContext(fs.readFileSync(path.join(web,script),'utf8'),ctx);
 return {ctx,elements,requests,created,copied,timers,run:s=>vm.runInContext(s,ctx)};
}
test('consumer DOM has literal file cards and no operational topology controls',()=>{
 const app=workspace();app.ctx.item=file('evil',{name:'<img src=x onerror=alert(1)>.pdf',path:'/PRIVATE/path'});
 app.run("library.catalog=new FileCatalog.Catalog([{binding_id:'one'}]);library.catalog.files=[item];library.catalog.pending=[];drawFiles();fileDetail(item,null);");
 assert.equal(app.elements.get('file-grid').children.length,1);assert.equal(app.elements.get('detail-title').textContent,app.ctx.item.name);
 assert.equal(app.elements.get('count-document').textContent,'1');assert.equal(app.elements.get('file-detail').hidden,false);
 const html=fs.readFileSync(path.join(web,'library.html'),'utf8');
 for(const forbidden of ['binding-select','directory-back','resolve-form','Copy 投影','Schema','parent_resource_id','删除文件'])assert.ok(!html.includes(forbidden),forbidden);
 app.run("library.category='video';drawFiles();");assert.equal(app.elements.get('files-empty').hidden,false);
 app.elements.get('file-search').value='PRIVATE';app.run("library.category='all';drawFiles();");assert.equal(app.elements.get('file-grid').children.length,0,'search must not expose hidden paths');
});
test('logout clears metadata and invalidates outstanding reads',async()=>{
 const app=workspace();app.run("library.catalog=new FileCatalog.Catalog([{binding_id:'one'}]);");
 const pending=app.run('loadRound()');const read=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));assert.ok(read);
 app.elements.get('file-search').value='secret';app.run('signOut();');read.resolve({ok:true,status:200,json:async()=>page([file('private')])});await pending;
 assert.equal(app.run('library.catalog'),null);assert.equal(app.elements.get('file-grid').children.length,0);assert.equal(app.elements.get('file-search').value,'');assert.equal(app.elements.get('detail-title').textContent,'');assert.equal(app.elements.get('load-more').hidden,true);
});
test('failure is announced without claiming an empty or complete library',async()=>{
 const app=workspace();app.run("library.catalog=new FileCatalog.Catalog([{binding_id:'one'}]);");const pending=app.run('loadRound()');
 const read=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));read.resolve({ok:false,status:503,json:async()=>({code:'READ_UNAVAILABLE'})});await pending;
 assert.equal(app.elements.get('library-error').hidden,false);assert.equal(app.elements.get('files-empty').hidden,true);assert.ok(app.elements.get('load-status').textContent.includes('不完整'));
});
test('detail close restores focus; consumer navigation omits admin entry for every role',async()=>{
 const app=workspace();app.ctx.item=file('name');app.run("fileDetail(item,node('refresh'));closeDetail(true);");assert.equal(app.elements.get('refresh').focused,true);
 for(const role of ['USER','ADMIN']){
 const current=workspace();current.ctx.principal={login:'test',role};const pending=current.run('signIn(principal)');const read=current.requests.find(r=>r.url==='/api/v1/workspace');read.resolve({ok:true,status:200,json:async()=>({bindings:[{binding_id:'one'}]})});
 await new Promise(resolve=>setImmediate(resolve));const files=current.requests.find(r=>r.url.startsWith('/api/v1/resources?'));files.resolve({ok:true,status:200,json:async()=>page()});await pending;assert.equal(current.elements.has('admin-link'),false);current.run('signOut()');
 }
 for(const surface of ['library.html','downloads.html'])assert.ok(!fs.readFileSync(path.join(web,surface),'utf8').includes('href="/admin/"'));
});

test('whole-root library scope does not hardcode a physical folder or promise the entire drive',async()=>{
 const app=workspace();const pending=app.run("signIn({login:'test',role:'USER'});");
 const read=app.requests.find(r=>r.url==='/api/v1/workspace');read.resolve({ok:true,status:200,json:async()=>({bindings:[{binding_id:'one'},{binding_id:'two'}],whole_root_library:true})});
 await new Promise(resolve=>setImmediate(resolve));const files=app.requests.filter(r=>r.url.startsWith('/api/v1/resources?'));for(const request of files)request.resolve({ok:true,status:200,json:async()=>page()});
 await new Promise(resolve=>setImmediate(resolve));for(const request of app.requests.filter(r=>r.url.startsWith('/api/v1/resources?')&&!files.includes(r)))request.resolve({ok:true,status:200,json:async()=>page()});await pending;
 assert.equal(app.elements.get('library-scope').textContent,'展示已接入目录中完成索引的文件，不代表整个网盘。');assert.equal(app.elements.get('library-scope').textContent.includes('CloudSite'),false);
});
test('real content actions use only same-origin identity URLs and stop media on close',()=>{
 const app=workspace();app.ctx.item=file('video',{name:'real.mp4',path:'/videos/real.mp4'});
 app.run('library.content=true;fileDetail(item,null);');
 assert.ok(app.elements.get('file-download').href.startsWith('/api/v1/files/content?'));
 assert.ok(app.elements.get('file-download').href.includes('disposition=attachment'));
 const player=app.run('library.player');assert.ok(player.src.startsWith('/api/v1/files/content?'));assert.equal(player.controls,true);
 app.run('closeDetail();');assert.equal(player.paused,true);assert.equal(player.loaded,true);assert.equal(player.src,undefined);assert.equal(app.elements.get('file-preview').children.length,0);
});
test('late text preview never crosses detail change or logout; text stays literal',async()=>{
 const app=workspace();app.ctx.item=file('text',{name:'real.md',path:'/docs/real.md'});
 app.run('library.content=true;fileDetail(item,null);');const read=app.requests.find(r=>r.url.startsWith('/api/v1/files/preview?'));assert.ok(read);
 app.run('closeDetail();');read.resolve({ok:true,status:200,json:async()=>({text:'SECRET <script>alert(1)</script>'})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('file-preview').children.length,0);
 app.run('fileDetail(item,null);');const next=app.requests.filter(r=>r.url.startsWith('/api/v1/files/preview?'))[1];next.resolve({ok:true,status:200,json:async()=>({text:'<script>alert(1)</script>'})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('file-preview').children[0].textContent,'<script>alert(1)</script>');
 app.run('signOut();');assert.equal(app.elements.get('file-preview').children.length,0);assert.equal(app.elements.get('file-download').hidden,true);
});

test('PDF uses server PNG pages, bounded navigation and discards late pages',async()=>{
 const app=workspace();app.ctx.item=file('pdf',{name:'real.pdf',path:'/docs/real.pdf'});
 app.run('library.content=true;fileDetail(item,null);');
 const first=app.requests.find(r=>r.url.includes('/files/preview?'));
 assert.ok(first.url.endsWith('&page=1'));
 first.resolve({ok:true,status:200,json:async()=>({format:'pdf',image_base64:'iVBORw0KGgoAAA',pages:2,page:1})});
 await new Promise(resolve=>setImmediate(resolve));
 const [tools,image]=app.elements.get('file-preview').children;
 assert.equal(image.src,'data:image/png;base64,iVBORw0KGgoAAA');
 assert.equal(tools.children[0].disabled,true);assert.equal(tools.children[2].disabled,false);
 tools.children[2].listeners.click[0]();
 const next=app.requests.filter(r=>r.url.includes('/files/preview?'))[1];assert.ok(next.url.endsWith('&page=2'));
 app.run('closeDetail();');next.resolve({ok:true,status:200,json:async()=>({format:'pdf',image_base64:'iVBORw0KGgoAAA',pages:2,page:2})});
 await new Promise(resolve=>setImmediate(resolve));assert.equal(app.elements.get('file-preview').children.length,0);
});

test('share link is one-file only and cleared on logout; late create is ignored',async()=>{
 const app=workspace();app.ctx.item=file('doc',{name:'file.md',path:'/docs/file.md'});app.run('library.content=true;library.sharing=true;fileDetail(item,null);');
 app.elements.get('share-duration').value='1h';app.elements.get('share-code').value='aB39';
 const pending=app.elements.get('share-form').listeners.submit[0]({preventDefault(){}});const read=app.requests.find(r=>r.url==='/api/v1/shares');assert.equal(JSON.parse(read.options.body).resource_id,'doc');assert.equal(app.elements.get('share-create').disabled,true);
 read.resolve({ok:true,status:201,json:async()=>({url:'http://127.0.0.1:8180/s/#opaque',expires_at:'2026-10-16T00:00:00Z'})});await pending;assert.ok(app.elements.get('share-url').value.endsWith('#opaque'));
 app.run('signOut();');assert.equal(app.elements.get('share-url').value,'');assert.equal(app.elements.get('share-code').value,'');assert.equal(app.elements.get('my-shares-panel').hidden,true);
 app.run('library.content=true;library.sharing=true;fileDetail(item,null);');app.elements.get('share-duration').value='1h';
 const late=app.elements.get('share-form').listeners.submit[0]({preventDefault(){}});const last=app.requests.filter(r=>r.url==='/api/v1/shares').at(-1);app.run('closeDetail();');last.resolve({ok:true,status:201,json:async()=>({url:'SECRET',expires_at:'2026-10-16T00:00:00Z'})});await late;assert.equal(app.elements.get('share-url').value,'');
});

test('download keeps real native attachment click without asynchronous HEAD or synthetic anchor',()=>{
 const app=workspace();app.ctx.item=file('doc',{name:'file.md',path:'/docs/file.md'});app.run('library.content=true;fileDetail(item,null);');
 let prevented=false;app.elements.get('file-download').listeners.click[0]({preventDefault(){prevented=true;}});assert.equal(prevented,false);
 const link=app.elements.get('file-download');assert.equal(link.target,'_blank');assert.equal(link.rel,'noopener');assert.ok(link.href.includes('disposition=attachment'));
 assert.ok(!app.requests.some(r=>r.options.method==='HEAD'));assert.ok(!app.created.some(e=>e.clicked));assert.ok(app.elements.get('download-status').textContent.includes('已请求'));
 assert.ok(!app.elements.get('download-status').textContent.includes('已完成'));app.run('closeDetail();');app.elements.get('file-download').listeners.click[0]({preventDefault(){prevented=true;}});assert.equal(prevented,true);
});

test('native download retains library in its own tab and owner revoke uses protected POST',async()=>{
 const app=workspace();app.ctx.item=file('doc',{name:'file.md',path:'/docs/file.md'});app.run('library.content=true;fileDetail(item,null);');
 assert.equal(app.elements.get('file-download').target,'_blank');assert.equal(app.elements.get('file-download').rel,'noopener');
 const listing=app.run('loadShares()');const request=app.requests.find(r=>r.url==='/api/v1/shares');request.resolve({ok:true,status:200,json:async()=>({items:[{id:'one',name:'<script>secret</script>',active:true,expires_at:'2026-10-16T00:00:00Z'}]})});await listing;
 const row=app.elements.get('owned-shares').children[0];assert.ok(row.children[0].textContent.includes('<script>'));
 const revoke=row.children[1].listeners.click[0]();const write=app.requests.find(r=>r.url==='/api/v1/shares/one/revoke');assert.equal(write.options.method,'POST');write.resolve({ok:true,status:204});await new Promise(resolve=>setImmediate(resolve));const refresh=app.requests.filter(r=>r.url==='/api/v1/shares').at(-1);refresh.resolve({ok:true,status:200,json:async()=>({items:[]})});await revoke;assert.equal(app.elements.get('owned-shares').children.length,0);
});

test('guest share page uses fragment token and verified single-file route, never markup',async()=>{
 const ids=Array.from(fs.readFileSync(path.join(web,'share.html'),'utf8').matchAll(/id="([^"]+)"/g),m=>m[1]);
 const elements=new Map(ids.map(id=>[id,{value:'',listeners:{},textContent:'',addEventListener(k,v){this.listeners[k]=v;},removeAttribute(k){delete this[k];}}]));const requests=[];
 const ctx=vm.createContext({location:{hash:'#'+'A'.repeat(43)},document:{getElementById:id=>elements.get(id)},fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));}});vm.runInContext(fs.readFileSync(path.join(web,'share.js'),'utf8'),ctx);
 const pending=elements.get('redeem-form').listeners.submit({preventDefault(){}});assert.equal(requests[0].url,'/api/v1/shares/redeem');assert.equal(JSON.parse(requests[0].options.body).token,'A'.repeat(43));
 requests[0].resolve({ok:true,json:async()=>({name:'<script>file</script>',size_bytes:4,expires_at:'2026-10-16T00:00:00Z',download_url:'/api/v1/shared/95000000-0000-4000-8000-000000000001/content'})});await pending;
 assert.equal(elements.get('shared-name').textContent,'<script>file</script>');assert.equal(elements.get('shared-file').hidden,false);
 let prevented=false;elements.get('shared-download').listeners.click({preventDefault(){prevented=true;}});assert.equal(prevented,false);assert.equal(requests.length,1);assert.equal(elements.get('shared-download').target,'_blank');assert.equal(elements.get('shared-download').rel,'noopener');
});

test('explicit media play pause and errors follow actual events; old callbacks cannot alter new detail',async()=>{
 const app=workspace();app.ctx.item=file('movie',{name:'movie.mp4',path:'/movie.mp4'});app.run('library.content=true;fileDetail(item,null);');
 const [toggle,player]=app.elements.get('file-preview').children;player.paused=true;assert.equal(toggle.textContent,'播放视频');
 await toggle.listeners.click[0]();assert.equal(player.played,true);player.listeners.playing[0]();assert.equal(toggle.textContent,'暂停播放');assert.equal(app.elements.get('preview-status').textContent,'正在播放');
 await toggle.listeners.click[0]();assert.equal(player.paused,true);player.listeners.pause[0]();assert.equal(toggle.textContent,'播放视频');
 player.play=async()=>{const e=new Error();e.name='NotAllowedError';throw e;};await toggle.listeners.click[0]();assert.ok(app.elements.get('preview-status').textContent.includes('未允许'));
 player.error={code:3};player.listeners.error[0]();assert.ok(app.elements.get('preview-status').textContent.includes('解码'));
 app.run('closeDetail();');player.listeners.playing[0]();player.listeners.error[0]();assert.equal(app.elements.get('preview-status').textContent,'');
});

test('four-character code is random on open, clearing stays empty and copying preserves created code',async()=>{
 const app=workspace();app.ctx.item=file('doc',{name:'file.md',path:'/docs/file.md'});app.run('library.content=true;library.sharing=true;fileDetail(item,null);');
 const generated=new Set();for(let n=0;n<100;n++){const code=app.run('randomShareCode()');assert.match(code,/^[A-Za-z0-9]{4}$/);generated.add(code);}assert.ok(generated.size>1);
 app.elements.get('file-share').listeners.click[0]();assert.match(app.elements.get('share-code').value,/^[A-Za-z0-9]{4}$/);
 app.elements.get('share-code').value='';app.elements.get('share-duration').value='1h';
 const pending=app.elements.get('share-form').listeners.submit[0]({preventDefault(){}});const request=app.requests.find(r=>r.url==='/api/v1/shares');assert.equal(JSON.parse(request.options.body).code,'');
 request.resolve({ok:true,status:201,json:async()=>({url:'http://127.0.0.1:8180/s/#no-code',expires_at:'2026-10-16T00:00:00Z'})});await pending;assert.equal(app.elements.get('share-code-result').textContent,'无需提取码');
 await app.elements.get('share-copy').listeners.click[0]();assert.equal(app.copied[0],'http://127.0.0.1:8180/s/#no-code\n无需提取码');
 app.run("library.createdShareCode='aB39';");app.elements.get('share-code').value='xxxx';await app.elements.get('share-copy').listeners.click[0]();assert.ok(app.copied[1].endsWith('提取码：aB39'));
});

test('all files remain reachable with120-card pages; non-core extensions are Other',()=>{
 const app=workspace();app.ctx.items=Array.from({length:241},(_,n)=>file(String(n)));
 app.run("library.catalog=new FileCatalog.Catalog([{binding_id:'one'}],true);library.catalog.files=items;library.catalog.pending=[];drawFiles();");
 assert.equal(app.elements.get('file-grid').children.length,120);assert.equal(app.elements.get('count-all').textContent,'241');
 app.elements.get('file-page-next').listeners.click[0]();app.elements.get('file-page-next').listeners.click[0]();assert.equal(app.elements.get('file-grid').children.length,1);assert.ok(app.elements.get('file-page-status').textContent.includes('3 / 3'));
 for(const name of ['image.png','archive.zip','music.mp3','no-extension','file.xyz']){app.ctx.item=file('other',{name});assert.equal(app.run('ResourceDisplay.category(item)'),'other');app.run('fileDetail(item,null);');assert.equal(app.elements.get('detail-type').textContent,'其他');}
});

test('both real controllers partition before search/count/pagination and retain all file actions',()=>{
 for(const surface of ['library','downloads']){
  const app=workspace(surface);app.ctx.items=[file('cloud',{path:'/云下载/nested/a.pdf'}),file('main',{path:'/教程/b.pdf'}),file('sibling',{path:'/云下载备份/c.pdf'})];
  app.run("library.cloudScope={binding_id:'one',path:'/云下载'};library.catalog=new FileCatalog.Catalog([{binding_id:'one'}],true);library.catalog.files=items;library.catalog.pending=[];library.content=true;library.sharing=true;drawFiles();");
  assert.equal(app.elements.get('count-all').textContent,surface==='downloads'?'1':'2');
  const card=app.elements.get('file-grid').children[0];card.listeners.click[0]();
  assert.equal(app.elements.get('file-download').hidden,false);assert.equal(app.elements.get('file-share').hidden,false);
  assert.ok(app.elements.get('file-download').href.includes('disposition=attachment'));
  assert.equal(app.elements.get('file-download').target,'_blank');
  app.elements.get('file-search').value=surface==='downloads'?'main':'cloud';app.run('drawFiles();');assert.equal(app.elements.get('file-grid').children.length,0);
 }
});
test('download file mount has one canonical GET loader; clear drops late results and previews',async()=>{
 const app=workspace('downloads');assert.equal(app.requests.length,0,'file component must not start a second auth or Worker flow');
 const pending=app.run("CloudFileSurface.mount({user_id:'test'}, {cloud_download_files:{binding_id:'one',path:'/云下载'},content_access_enabled:true,sharing_enabled:true},()=>{});");
 const read=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));assert.ok(read);assert.equal(read.options.method,'GET');assert.ok(read.url.includes('whole_root=true'));
 app.run('CloudFileSurface.clear();');read.resolve({ok:true,status:200,json:async()=>page([file('late',{path:'/云下载/a.pdf'})])});await pending;
 assert.equal(app.elements.get('file-grid').children.length,0);assert.equal(app.elements.get('file-preview').children.length,0);assert.equal(app.elements.get('download-files').hidden,true);
 assert.equal(app.requests.some(r=>r.options.method==='POST'),false);
});
test('download file canonical read failure stays incomplete and never becomes empty success',async()=>{
 const app=workspace('downloads');const pending=app.run("CloudFileSurface.mount({user_id:'test'}, {cloud_download_files:{binding_id:'one',path:'/云下载'}},()=>{});");
 app.requests[0].resolve({ok:false,status:503,json:async()=>({code:'READ_UNAVAILABLE'})});await pending;
 assert.equal(app.elements.get('library-error').hidden,false);assert.equal(app.elements.get('files-empty').hidden,true);assert.match(app.elements.get('load-status').textContent,/不完整/);
});

test('quiet cloud refresh discovers delayed files without closing playback or flashing existing cards',async()=>{
 const app=workspace('downloads');app.ctx.first=file('first',{path:'/云下载/first.pdf'});app.ctx.second=file('second',{path:'/云下载/nested/second.pdf'});
 app.run("library.userID='user';library.wholeRoot=true;library.cloudScope={binding_id:'one',path:'/云下载'};library.bindings=[{binding_id:'one'}];library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[first];library.catalog.pending=[];drawFiles();fileDetail(first,null);");
 const oldCard=app.elements.get('file-grid').children[0];
 const unchanged=app.run('CloudFileSurface.refresh()');
 assert.equal(app.elements.get('file-grid').children[0],oldCard);
 assert.equal(app.elements.get('file-detail').hidden,false);
 app.requests[0].resolve({ok:true,status:200,json:async()=>page([app.ctx.first])});await unchanged;
 assert.equal(app.elements.get('file-grid').children[0],oldCard,'unchanged inventory must not repaint');
 const changed=app.run('CloudFileSurface.refresh()');
 app.requests[1].resolve({ok:true,status:200,json:async()=>page([app.ctx.first,app.ctx.second])});await changed;
 assert.equal(app.elements.get('count-all').textContent,'2');assert.equal(app.elements.get('file-detail').hidden,false);
 assert.equal(app.run('library.selected.resource_id'),'first');
 const late=app.run('CloudFileSurface.refresh()');app.run('CloudFileSurface.clear();');
 app.requests[2].resolve({ok:true,status:200,json:async()=>page([app.ctx.first])});await late;
 assert.equal(app.elements.get('file-grid').children.length,0);assert.equal(app.elements.get('download-files').hidden,true);
 assert.equal(app.requests.some(r=>r.options.method!=='GET'),false);
});

test('returning library never paints the login form while session restoration is pending',async()=>{
 const html=fs.readFileSync(path.join(web,'library.html'),'utf8');assert.match(html,/<main id="login-view"[^>]* hidden>/);assert.equal(html.includes('正在恢复登录'),false);assert.match(html,/<p id="session-error"[^>]* hidden>/);
 const app=workspace();app.elements.get('login-view').hidden=true;
 const caps=app.requests.find(r=>r.url==='/api/v1/auth/capabilities');caps.resolve({ok:true,status:200,json:async()=>({registration_enabled:true})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('login-view').hidden,true);
 const me=app.requests.find(r=>r.url==='/api/v1/auth/me');me.resolve({ok:true,status:200,json:async()=>({user_id:'user',login:'test',role:'USER'})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('login-view').hidden,true);
 const info=app.requests.find(r=>r.url==='/api/v1/workspace');info.resolve({ok:true,status:200,json:async()=>({bindings:[{binding_id:'one'}]})});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('login-view').hidden,true);assert.equal(app.elements.get('session-error').hidden,true);assert.equal(app.elements.get('library-view').hidden,false);
});
test('expired session reveals login; transient auth failure does not pretend the user logged out',async()=>{
 for(const status of [401,503]){
  const app=workspace();app.elements.get('login-view').hidden=true;
  app.requests[0].resolve({ok:true,status:200,json:async()=>({registration_enabled:true})});await new Promise(resolve=>setImmediate(resolve));
  app.requests.find(r=>r.url==='/api/v1/auth/me').resolve({ok:false,status,json:async()=>({code:status===401?'AUTH_REQUIRED':'READ_UNAVAILABLE'})});await new Promise(resolve=>setImmediate(resolve));
  assert.equal(app.elements.get('login-view').hidden,status!==401);
  if(status===503)assert.match(app.elements.get('session-error').textContent,/连接暂时不可用/);
 }
});

test('library silently drops removed canonical cards without resetting the player; polling is bounded',async()=>{
 const app=workspace();app.ctx.keep=file('keep',{name:'keep.mp4'});app.ctx.gone=file('gone',{name:'deleted.jpg'});
 app.run("library.userID='owner';library.wholeRoot=true;library.bindings=[{binding_id:'one'}];library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[keep,gone];library.catalog.pending=[];library.content=true;drawFiles();fileDetail(keep,null);");
 const player=app.run('library.player');assert.equal(app.timers.length,1);assert.equal(app.timers[0].ms,10000);
 app.timers[0].callback();app.timers[0].callback();
 const reads=app.requests.filter(r=>r.url.startsWith('/api/v1/resources?'));assert.equal(reads.length,1);assert.equal(app.elements.get('file-grid').children.length,2);
 reads[0].resolve({ok:true,status:200,json:async()=>page([app.ctx.keep])});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('file-grid').children.length,1);assert.equal(app.run('library.player'),player);assert.notEqual(player.paused,true);assert.equal(app.elements.get('file-detail').hidden,false);
 const count=app.requests.length;app.ctx.document.hidden=true;app.timers[0].callback();assert.equal(app.requests.length,count);
 app.ctx.document.hidden=false;app.run('signOut()');app.timers[0].callback();assert.equal(app.requests.length,count);
});

test('failed quiet library reads preserve existing cards instead of claiming deletion',async()=>{
 const app=workspace();app.ctx.keep=file('keep');app.run("library.userID='owner';library.wholeRoot=true;library.bindings=[{binding_id:'one'}];library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[keep];library.catalog.pending=[];drawFiles();");
 const pending=app.run('refreshCanonicalQuietly()');const req=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));
 req.resolve({ok:false,status:503,json:async()=>({code:'READ_UNAVAILABLE'})});await pending;
 assert.equal(app.elements.get('file-grid').children.length,1);assert.equal(app.run('library.catalog.files.length'),1);
});

test('confirmed deletion closes the removed file player on both file surfaces',async()=>{
 for(const surface of ['library','downloads']){
  const app=workspace(surface);app.ctx.gone=file('gone',{name:'deleted.mp4',path:surface==='downloads'?'/云下载/deleted.mp4':'/deleted.mp4'});
  app.run("library.userID='owner';library.wholeRoot=true;library.bindings=[{binding_id:'one'}];library.cloudScope={binding_id:'one',path:'/云下载'};library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[gone];library.catalog.pending=[];library.content=true;drawFiles();fileDetail(gone,null);");
  const player=app.run('library.player');const pending=app.run('refreshCanonicalQuietly()');
  assert.equal(app.elements.get('file-detail').hidden,false);
  const read=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));read.resolve({ok:true,status:200,json:async()=>page([])});await pending;
  assert.equal(app.run('library.selected'),null);assert.equal(app.elements.get('file-detail').hidden,true);assert.equal(app.elements.get('file-preview').children.length,0);assert.equal(player.paused,true);assert.equal(player.src,undefined);assert.equal(app.elements.get('file-grid').children.length,0);
  assert.equal(app.requests.some(r=>r.options.method!=='GET'),false);
 }
});

test('incomplete or failed quiet refresh never closes an existing preview',async()=>{
 const app=workspace();app.ctx.keep=file('keep',{name:'keep.mp4',path:'/keep.mp4'});
 app.run("library.userID='owner';library.wholeRoot=true;library.bindings=[{binding_id:'one'}];library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[keep];library.catalog.pending=[];library.content=true;drawFiles();fileDetail(keep,null);");
 const player=app.run('library.player');const pending=app.run('refreshCanonicalQuietly()');
 app.requests.find(r=>r.url.startsWith('/api/v1/resources?')).resolve({ok:true,status:200,json:async()=>page([],'next')});await new Promise(resolve=>setImmediate(resolve));
 assert.equal(app.elements.get('file-detail').hidden,false);assert.equal(app.run('library.player'),player);
 app.requests.filter(r=>r.url.startsWith('/api/v1/resources?')).at(-1).resolve({ok:false,status:503,json:async()=>({code:'READ_UNAVAILABLE'})});await pending;
 assert.equal(app.elements.get('file-detail').hidden,false);assert.equal(app.run('library.player'),player);assert.notEqual(player.paused,true);assert.equal(app.elements.get('file-grid').children.length,1);
});

test('same-path replacement does not retain a removed resource identity',async()=>{
 const app=workspace();app.ctx.gone=file('old',{name:'same.mp4',path:'/same.mp4'});app.ctx.replacement=file('new',{name:'same.mp4',path:'/same.mp4'});
 app.run("library.userID='owner';library.wholeRoot=true;library.bindings=[{binding_id:'one'}];library.catalog=new FileCatalog.Catalog(library.bindings,true);library.catalog.files=[gone];library.catalog.pending=[];library.content=true;drawFiles();fileDetail(gone,null);");
 const pending=app.run('refreshCanonicalQuietly()');app.requests.find(r=>r.url.startsWith('/api/v1/resources?')).resolve({ok:true,status:200,json:async()=>page([app.ctx.replacement])});await pending;
 assert.equal(app.run('library.selected'),null);assert.equal(app.elements.get('file-detail').hidden,true);assert.equal(app.elements.get('file-grid').children.length,1);
});
