'use strict';
const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const vm=require('node:vm');const path=require('node:path');
const code=fs.readFileSync(path.join(__dirname,'../internal/producthttp/web/resource-display.js'),'utf8');
const display=vm.runInNewContext(code+'\nResourceDisplay;',{});
const file=(name,extra={})=>({name,path:'/'+name,directory:false,resource_id:'test-id',...extra});
test('explicit files classify without asserting MIME or playback',()=>{
 for(const [name,kind] of [['app.MSI','software'],['Android.apk','software'],['video.MKV','video'],['视频.mp4','video'],['文档.md','document'],['表格.xlsx','document'],['photo.heic','image'],['音频.flac','audio'],['package.zip','archive'],['bundle.tar.gz','archive'],['README','other'],['.mp4','other'],['film.mp4.exe','software'],['file.exe.zip','archive']]){
  assert.equal(display.classify(file(name)).key,kind,name);
 }
 assert.equal(display.classify(file('folder.mp4',{directory:true})).key,'folder');
 assert.equal(display.classify(file('unknown.mp4',{directory:null})).key,'unknown');
 assert.equal(display.classify({directory:false,path:'/docs/Report.PDF'}).key,'document');
});
test('page counts and search do not mutate the real metadata',()=>{
 const items=[file('软件.EXE'),file('video.mkv'),file('Guide.MD'),file('unknown.zip'),file('目录.pdf',{directory:true})];
 const before=JSON.stringify(items);const c=display.counts(items);
 assert.equal(c.all,5);assert.equal(c.software,1);assert.equal(c.video,1);assert.equal(c.document,1);assert.equal(c.other,2);
 assert.equal(display.filter(items,'document',' GUIDE ').length,1);
 assert.equal(display.filter(items,'all','/软件').length,1);
 assert.equal(display.filter(items,'video','missing').length,0);
 assert.equal(display.filter(items,'invalid','').length,0);
 assert.equal(JSON.stringify(items),before);
 assert.equal(display.counts([]).all,0);
});
test('untrusted names stay literal; no content or access promises',()=>{
 const x=file('<img src=x onerror=alert(1)>.mp4');
 assert.equal(display.name(x),x.name);assert.equal(display.classify(x).key,'video');
 assert.ok(display.accessNote(x).includes('尚未接入'));assert.ok(display.accessNote(x).includes('未投影 Copy'));
 assert.ok(display.accessNote(file('app.zip')).includes('不会被自动当作软件'));
 assert.ok(display.accessNote(file('a.mp4',{copy_id:'copy'})).includes('不等于文件访问授权'));
 assert.equal(display.size(1955),'1.9 KB');assert.equal(display.size(0),'0 B');assert.equal(display.size(2147483648),'2.0 GB');assert.equal(display.size(null),'未提供');assert.equal(display.size(-1),'未提供');
});

function workspace(){
 const html=fs.readFileSync(path.join(__dirname,'../internal/producthttp/web/index.html'),'utf8');
 const ids=new Set(Array.from(html.matchAll(/id="([^"]+)"/g),m=>m[1]));
 class Element {
  constructor(){this.children=[];this.hidden=false;this.disabled=false;this.value='';this.attrs={};this.listeners={};this._text='';this.classList={toggle(){}};}
  set textContent(value){this._text=String(value);this.children=[];}get textContent(){return this._text;}
  append(...children){this.children.push(...children);}replaceChildren(...children){this._text='';this.children=children;}
  addEventListener(type,fn){(this.listeners[type] ||= []).push(fn);}setAttribute(key,value){this.attrs[key]=value;}scrollIntoView(){}
 }
 const elements=new Map(Array.from(ids,id=>[id,new Element()]));
 const requests=[];
 const ctx=vm.createContext({AbortController,URLSearchParams,document:{getElementById(id){assert.ok(elements.has(id),'unknown DOM id '+id);return elements.get(id);},createElement(){return new Element();}},window:{scrollTo(){}},fetch(url,options){return new Promise(resolve=>requests.push({url,options,resolve}));}});
 vm.runInContext(code,ctx);for(const script of ['admin-lists.js','admin-observe.js','admin-operations.js','admin-shares.js','admin-overview.js','app.js'])vm.runInContext(fs.readFileSync(path.join(__dirname,'../internal/producthttp/web',script),'utf8'),ctx);
 return {ctx,elements,requests,run:(source)=>vm.runInContext(source,ctx)};
}
test('DOM presentation stays literal and clears private DTO/detail on logout',()=>{
 const app=workspace();app.ctx.page={items:[file('<img src=x>.md',{size_bytes:1955,presence:'PRESENT'})],next_cursor:'cursor'};
 app.run('renderResources(page);resourceDetail(page.items[0]);');
 assert.equal(app.elements.get('count-document').textContent,'1');assert.equal(app.elements.get('resource-rows').children.length,1);
 assert.equal(app.elements.get('file-title').textContent,'<img src=x>.md');assert.equal(app.elements.get('file-detail').hidden,false);
 app.run("state.category='video';drawResources();");assert.equal(app.elements.get('resources-empty').hidden,false);assert.equal(app.elements.get('resource-rows').children.length,0);
 app.run("state.category='all';state.layout='cards';drawResources();");assert.equal(app.elements.get('resource-table').hidden,true);assert.equal(app.elements.get('resource-cards').hidden,false);assert.equal(app.elements.get('resource-cards').children.length,1);
 app.elements.get('resource-search').value='SECRET';app.elements.get('directory-label').textContent='/PRIVATE';app.run('signedOut();');
 assert.equal(app.run('state.resources.length'),0);assert.equal(app.run('state.resourceNext'),'');assert.equal(app.elements.get('file-title').textContent,'');assert.equal(app.elements.get('file-detail').hidden,true);assert.equal(app.elements.get('resource-cards').children.length,0);assert.equal(app.elements.get('count-document').textContent,'—');assert.equal(app.elements.get('resource-search').value,'');assert.equal(app.elements.get('directory-label').textContent,'根层级');
});
test('late Q5 detail cannot overwrite a changed scope or ended session',async()=>{
 for(const transition of ['clearResources();', 'signedOut();']){
  const app=workspace();app.run("state.binding='binding-one';");app.elements.get('resolve-path').value='/secret.md';
  const pending=app.elements.get('resolve-form').listeners.submit[0]({preventDefault(){}});
  // The admin shell now revalidates server-side RoleAdmin and current identity
  // before using ordinary product read APIs. Preserve the late-Q5 assertion.
  app.requests.find(r=>r.url==='/api/v1/admin/workspace').resolve({ok:true,status:200,json:async()=>({bindings:[]})});
  await new Promise(resolve=>setImmediate(resolve));
  app.requests.filter(r=>r.url==='/api/v1/auth/me').at(-1).resolve({ok:true,status:200,json:async()=>({user_id:'admin',login:'admin',role:'ADMIN'})});
  await new Promise(resolve=>setImmediate(resolve));
  const request=app.requests.find(r=>r.url.startsWith('/api/v1/resources/resolve?'));assert.ok(request);
  app.run(transition);request.resolve({ok:true,status:200,json:async()=>file('PRIVATE.md',{presence:'PRESENT'})});await pending;
  assert.equal(app.elements.get('file-detail').hidden,true);assert.equal(app.elements.get('file-title').textContent,'');
 }
});
