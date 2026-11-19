(function(root){
 'use strict';
 // Pure conditional-page client. Each authenticated surface owns an instance;
 // no persistent browser metadata and no process-global cross-account cache.
 function valid(page,bindings,detailed=false){
  if(!page||typeof page.ready!=='boolean'||typeof page.stale!=='boolean'||!Array.isArray(page.items)||page.items.length>50||page.limit!==50||!Number.isSafeInteger(page.offset)||page.offset<0||!Number.isSafeInteger(page.filtered)||page.filtered<page.offset+page.items.length||!/^[a-f0-9]{64}$/.test(page.revision)||typeof page.next_cursor!=='string'||page.next_cursor.length>2048||page.updated_at!==null&&!Number.isFinite(new Date(page.updated_at).getTime()))return false;
  if(page.ready){
   const c=page.counts;
   const groups=['software','video','document','other'];
   if(detailed)groups.push('image','audio','archive');
   if(!c||!['all',...groups].every(k=>Number.isSafeInteger(c[k])&&c[k]>=0)||c.all!==groups.reduce((total,k)=>total+c[k],0)||page.filtered>c.all)return false;
  }
  else if(page.counts!==null||page.next_cursor||page.offset)return false;
  const seen=new Set();
  for(const item of page.items){const key=item?.binding_id+'|'+item?.resource_id;if(!item||!bindings.has(item.binding_id)||typeof item.resource_id!=='string'||!item.resource_id||item.resource_id.length>2048||item.directory!==false||item.presence!=='PRESENT'||seen.has(key)||item.path!==null&&typeof item.path!=='string'||item.name!==null&&typeof item.name!=='string')return false;seen.add(key);}
  return true;
 }
 class Reader{
  constructor(fetcher,bindings){this.fetcher=fetcher;this.bindings=new Set(bindings);this.cache=new Map();}
  clear(){this.cache.clear();}
  async read(query,signal,fresh=false){
   const params=new URLSearchParams(query);const key=params.toString();const old=this.cache.get(key);
   const fetcher=this.fetcher;
   const response=await fetcher('/api/v1/library?'+key,{method:'GET',credentials:'same-origin',cache:'no-store',signal,headers:!fresh&&old?.etag?{'If-None-Match':old.etag}:{}});
   if(response.status===304){if(!old)throw Error('Unexpected conditional response');return {page:old.page,changed:false};}
   const page=await response.json();
   if(!response.ok){const e=Error('Library unavailable');e.code=page?.code;throw e;}
   if(!valid(page,this.bindings,query.taxonomy==='2')||page.next_cursor&&page.next_cursor===query.cursor)throw Error('Invalid library page');
   const etag=response.headers?.get('ETag')||'';
   this.cache.delete(key);this.cache.set(key,{page,etag});while(this.cache.size>16)this.cache.delete(this.cache.keys().next().value);
   return {page,changed:!old||JSON.stringify(old.page)!==JSON.stringify(page)};
  }
 }
 const api=Object.freeze({Reader,valid});if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.LibraryPageModel=api;
})(typeof window!=='undefined'?window:globalThis);
