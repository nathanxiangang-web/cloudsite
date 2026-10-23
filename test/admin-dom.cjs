'use strict';
// Small dependency-free DOM boundary, not a layout engine. Browser QA is separate.
const fs=require('node:fs'),path=require('node:path'),vm=require('node:vm'),assert=require('node:assert/strict');
const web=path.join(__dirname,'../internal/producthttp/web');
const principal={user_id:'97000000-0000-4000-8000-000000000009',login:'test-admin',role:'ADMIN'};
const descriptor={schema_version:15,mode:'ACQUISITION',bindings:[{binding_id:'97000000-0000-4000-8000-000000000001',label:'Test files'}],cloud_download_enabled:true,cloud_download_preview_enabled:true,content_access_enabled:true,sharing_enabled:true};
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function workspace({hash='',storage}={}){
 const html=fs.readFileSync(path.join(web,'index.html'),'utf8'),requests=[],events={},windowEvents={};
 let document;
 class Element{
  constructor(id='',attrs=''){this.id=id;this.children=[];this.hidden=/\bhidden\b/.test(attrs);this.disabled=/\bdisabled\b/.test(attrs);this.value=attrs.match(/value="([^"]*)"/)?.[1]||'';this.attrs={};this.listeners={};this._text='';this.classes=new Set();this.classList={toggle:(key,force)=>{if(force===undefined)force=!this.classes.has(key);if(force)this.classes.add(key);else this.classes.delete(key);},contains:key=>this.classes.has(key)};}
  set textContent(value){this._text=String(value);this.children=[];}get textContent(){return this._text;}
  append(...children){this.children.push(...children);}replaceChildren(...children){this._text='';this.children=children;}
  addEventListener(type,fn){(this.listeners[type]||=[]).push(fn);}setAttribute(key,value){this.attrs[key]=String(value);}
  focus(){document.activeElement=this;}scrollIntoView(){}get isConnected(){return true;}
  querySelectorAll(){return [elements.get('menu-close'),...Array.from(elements).filter(([id])=>id.startsWith('nav-')&&id!=='nav-tasks').map(([,n])=>n),elements.get('logout')];}
 }
 const elements=new Map(Array.from(html.matchAll(/<[^>]*\bid="([^"]+)"[^>]*>/g),m=>[m[1],new Element(m[1],m[0])]));
 document={activeElement:null,visibilityState:'visible',body:new Element('body'),getElementById(id){assert.ok(elements.has(id),'unknown DOM id '+id);return elements.get(id);},createElement(){return new Element();},addEventListener(type,fn){(events[type]||=[]).push(fn);}};
 const timers=new Map();let timerID=0;
 const window={crypto:require('node:crypto').webcrypto,location:{hash,pathname:'/admin/',search:''},history:{replaceState(_state,_unused,url){window.location.hash=url.includes('#')?'#'+url.split('#')[1]:'';}},scrollTo(){},addEventListener(type,fn){(windowEvents[type]||=[]).push(fn);},matchMedia(){return{matches:true};}};
 const ctx=vm.createContext({AbortController,URLSearchParams,document,window,setTimeout(callback,ms){const id=++timerID;timers.set(id,{callback,ms});return id;},clearTimeout(id){timers.delete(id);},fetch(url,options){return new Promise((resolve,reject)=>requests.push({url,options,resolve,reject,done:false}));}});
 if(storage)window.sessionStorage={getItem:key=>storage.get(key)??null,setItem:(key,value)=>storage.set(key,value),removeItem:key=>storage.delete(key)};
 vm.runInContext(fs.readFileSync(path.join(web,'resource-display.js'),'utf8'),ctx);
 vm.runInContext(fs.readFileSync(path.join(web,'admin-lists.js'),'utf8'),ctx);
 vm.runInContext(fs.readFileSync(path.join(web,'admin-observe.js'),'utf8'),ctx);
 vm.runInContext(fs.readFileSync(path.join(web,'admin-operations.js'),'utf8'),ctx);
 vm.runInContext(fs.readFileSync(path.join(web,'admin-shares.js'),'utf8'),ctx);
 vm.runInContext(fs.readFileSync(path.join(web,'app.js'),'utf8'),ctx);
 const run=source=>vm.runInContext(source,ctx);
 function respond(request,body,status=200){assert.ok(request,'expected pending request');request.done=true;request.resolve({ok:status>=200&&status<300,status,json:async()=>body});}
 function pending(url){return requests.find(r=>!r.done&&(typeof url==='string'?r.url===url:url.test(r.url)));}
 async function drain(overrides={}){
  for(let i=0;i<15;i++){await tick();const list=requests.filter(r=>!r.done);if(!list.length)return;
   for(const request of list){const route=request.url.split('?')[0],value=overrides[route]??(route==='/api/v1/auth/me'?principal:route==='/api/v1/admin/workspace'?descriptor:route==='/api/v1/admin/users'?{items:[],next_cursor:'',total:0,read_at:'2026-10-22T00:00:00Z',scope:'FILTERED_PRODUCT_USERS'}:route==='/api/v1/admin/shares'?{items:[],next_cursor:'',total:0,read_at:'2026-10-23T00:00:00Z',scope:'FILTERED_PRODUCT_SHARES'}:{items:[],next_cursor:''});respond(request,value);}
  }throw Error('request loop');
 }
 const click=id=>elements.get(id).listeners.click?.[0]({});
 const text=id=>{function all(n){return n.textContent+n.children.map(all).join(' ');}return all(elements.get(id));};
 return {ctx,run,html,elements,requests,events,windowEvents,document,window,timers,respond,pending,drain,click,text,tick};
}
module.exports={workspace,principal,descriptor,tick,web};
