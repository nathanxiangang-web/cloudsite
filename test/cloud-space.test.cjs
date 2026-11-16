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