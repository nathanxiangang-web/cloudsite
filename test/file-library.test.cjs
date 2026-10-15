'use strict';
const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const path=require('node:path');
const web=path.join(__dirname,'../internal/producthttp/web');
const catalogCode=fs.readFileSync(path.join(web,'file-catalog.js'),'utf8');
const {Catalog}=vm.runInNewContext(catalogCode+'\nFileCatalog;',{});
const file=(id,extra={})=>({binding_id:'one',resource_id:id,name:id+'.pdf',directory:false,presence:'PRESENT',size_bytes:123,...extra});
const page=(items=[],next_cursor='')=>({items,next_cursor});
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
function workspace(){
 const html=fs.readFileSync(path.join(web,'library.html'),'utf8');const ids=new Set(Array.from(html.matchAll(/id="([^"]+)"/g),m=>m[1]));
 class Element{
  constructor(){this.children=[];this.attrs={};this.listeners={};this.value='';this.hidden=false;this.disabled=false;this.isConnected=true;this._text='';}
  set textContent(value){this._text=String(value);this.children=[];}get textContent(){return this._text;}
  append(...nodes){this.children.push(...nodes);}replaceChildren(...nodes){this._text='';this.children=nodes;}
  addEventListener(type,fn){(this.listeners[type]||=[]).push(fn);}setAttribute(k,v){this.attrs[k]=v;}removeAttribute(k){delete this.attrs[k];delete this[k];}scrollIntoView(){}focus(){this.focused=true;}pause(){this.paused=true;}load(){this.loaded=true;}
 }
 const elements=new Map(Array.from(ids,id=>[id,new Element()]));const requests=[];
 const ctx=vm.createContext({AbortController,URLSearchParams,document:{getElementById(id){assert.ok(elements.has(id),'Unknown id '+id);return elements.get(id);},createElement(){return new Element();}},fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));}});
 for(const script of ['resource-display.js','file-catalog.js','library.js'])vm.runInContext(fs.readFileSync(path.join(web,script),'utf8'),ctx);
 return {ctx,elements,requests,run:s=>vm.runInContext(s,ctx)};
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
test('detail close restores focus; admin link follows authenticated role only',async()=>{
 const app=workspace();app.ctx.item=file('name');app.run("fileDetail(item,node('refresh'));closeDetail(true);");assert.equal(app.elements.get('refresh').focused,true);
 const pending=app.run("signIn({login:'test',role:'USER'});");const read=app.requests.find(r=>r.url==='/api/v1/workspace');read.resolve({ok:true,status:200,json:async()=>({bindings:[{binding_id:'one'}]})});
 await new Promise(resolve=>setImmediate(resolve));const files=app.requests.find(r=>r.url.startsWith('/api/v1/resources?'));files.resolve({ok:true,status:200,json:async()=>page()});await pending;assert.equal(app.elements.get('admin-link').hidden,true);
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
