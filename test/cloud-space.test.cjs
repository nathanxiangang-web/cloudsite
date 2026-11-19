const test=require('node:test');
const assert=require('node:assert/strict');
const space=require('../internal/producthttp/web/cloud-space.js');

function setup(){
 const listeners={};
 const elements=new Map();
 for(const id of ['cloud-space-panel','cloud-space-status','cloud-space-provision'])elements.set(id,{hidden:false,disabled:false,textContent:'',addEventListener(type,fn){listeners[type]=fn;}});
 global.document={getElementById:id=>elements.get(id)};
 global.crypto=require('node:crypto').webcrypto;
 return{elements,listeners};
}

test('space GET performs no provision and preserves legacy endpoint',async()=>{
 const{elements}=setup();const calls=[];
 global.fetch=async(url,options)=>{
  calls.push({url,options});
  return {status:200,ok:true,json:async()=>({status:'NOT_PROVISIONED'})};
 };
 await space.init();
 assert.equal(calls.length,1);assert.equal(calls[0].options.method,undefined);
 assert.equal(space.endpoint(),'/api/v1/cloud-downloads');
 assert.equal(elements.get('cloud-space-provision').hidden,false);
 space.reset();
});

test('ACTIVE space uses v2 and provision is explicit',async()=>{
 const{listeners}=setup();const calls=[];
 global.fetch=async(url,options)=>{
  calls.push({url,options});
  return {status:200,ok:true,json:async()=>({status:'ACTIVE'})};
 };
 await space.mount();
 assert.equal(space.endpoint(),'/api/v2/cloud-downloads');
 await listeners.click();
 assert.equal(calls.length,2);assert.equal(calls[1].options.method,'POST');
 const body=JSON.parse(calls[1].options.body);assert.match(body.operation_key,/^space-/);
 assert.equal(space.endpoint(),'/api/v2/cloud-downloads');
 space.reset();assert.equal(space.endpoint(),'/api/v1/cloud-downloads');
});

test('missing private API falls back without creating directories',async()=>{
 setup();const calls=[];
 global.fetch=async(url,options)=>{
  calls.push({url,options});
  return {status:404,ok:false,json:async()=>({code:'NOT_FOUND'})};
 };
 await space.init();
 assert.equal(calls.length,1);assert.equal(space.endpoint(),'/api/v1/cloud-downloads');
});
test('private-only space has no submit endpoint until active',async()=>{
 const{listeners}=setup();
 global.fetch=async()=>({status:200,ok:true,json:async()=>({status:'NOT_PROVISIONED'})});
 await space.mount(null,true);
 assert.equal(space.endpoint(),null);
 global.fetch=async()=>({status:200,ok:true,json:async()=>({status:'ACTIVE'})});
 await listeners.click();
 assert.equal(space.endpoint(),'/api/v2/cloud-downloads');
 space.reset();
});

test('private files use the dedicated cursor API and sanitize presentation fields',async()=>{
 const surface=require('../internal/producthttp/web/file-surface.js');
 const files=new surface.PrivateFiles(),calls=[];
 const item=(id,name)=>({binding_id:'binding',resource_id:id,path:'/private/'+name,name:null,directory:false,size_bytes:12,presence:'PRESENT',donor:'SECRET'});
 const read=async url=>{
  calls.push(url);
  return calls.length===1?{items:[item('one','a.txt')],next_cursor:'next page'}:{items:[item('two','b.pdf')],next_cursor:''};
 };
 let result=await files.read(read);
 assert.equal(result.items[0].name,'a.txt');assert.equal('donor'in result.items[0],false);assert.equal(result.cursor,'next page');
 result=await files.read(read,true);
 assert.deepEqual(calls,['/api/v2/cloud-downloads/files?limit=50','/api/v2/cloud-downloads/files?limit=50&cursor=next%20page']);
 assert.deepEqual(result.items.map(item=>item.resource_id),['one','two']);assert.equal(result.cursor,'');
});

test('private file cursors fail closed and clear fences late pages',async()=>{
 const surface=require('../internal/producthttp/web/file-surface.js');
 const item=id=>({binding_id:'binding',resource_id:id,path:'/private/'+id+'.txt',name:id,directory:false,size_bytes:null,presence:'PRESENT'});
 const files=new surface.PrivateFiles();
 await files.read(async()=>({items:[item('one')],next_cursor:'same'}));
 await assert.rejects(files.read(async()=>({items:[item('two')],next_cursor:'same'}),true),/INVALID_PRIVATE_FILES/);
 assert.deepEqual(files.items.map(item=>item.resource_id),['one']);
 let resolve;const pending=files.read(()=>new Promise(r=>{resolve=r;}));files.clear();resolve({items:[item('late')],next_cursor:''});
 assert.equal(await pending,null);assert.deepEqual(files.items,[]);assert.equal(files.cursor,'');
});
