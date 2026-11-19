(function(root){
 'use strict';
 // Exact canonical coordinates. This is presentation, not authorization.
 function validScope(scope){
  return !!scope&&typeof scope.binding_id==='string'&&scope.binding_id.length>0&&typeof scope.path==='string'&&scope.path.startsWith('/')&&scope.path!=='/'&&!scope.path.endsWith('/')&&!scope.path.includes('//')&&!scope.path.includes('\0')&&!scope.path.split('/').some(part=>part==='.'||part==='..');
 }
 function within(item,scope){return validScope(scope)&&item.binding_id===scope.binding_id&&typeof item.path==='string'&&item.path.startsWith(scope.path+'/');}
 function partition(items,scope,surface){
  if(surface!=='library'&&surface!=='downloads')throw new Error('INVALID_SURFACE');
  return items.filter(item=>surface==='downloads'?within(item,scope):!within(item,scope));
 }
 function privateParent(item){return item.path.slice(0,item.path.lastIndexOf('/'));}
 function validPrivateItem(item){
  if(!item||typeof item.binding_id!=='string'||!item.binding_id||typeof item.resource_id!=='string'||!item.resource_id||typeof item.path!=='string'||item.presence!=='PRESENT'||typeof item.directory!=='boolean')return false;
  if(item.name!==null&&typeof item.name!=='string')return false;
  if(item.size_bytes!==null&&(!Number.isSafeInteger(item.size_bytes)||item.size_bytes<0))return false;
  return validScope({binding_id:item.binding_id,path:item.path})&&validScope({binding_id:item.binding_id,path:privateParent(item)});
 }
 function presentPrivateItem(item){
  return {
   binding_id:item.binding_id,
   resource_id:item.resource_id,
   path:item.path,
   name:item.name||item.path.slice(item.path.lastIndexOf('/')+1)||item.resource_id,
   directory:item.directory,
   size_bytes:item.size_bytes,
   presence:item.presence,
   copy_id:item.copy_id??null,
   copy_availability:item.copy_availability??null,
   asset_id:item.asset_id??null,
   release_id:item.release_id??null,
   variant_id:item.variant_id??null
  };
 }
 function privateScope(items){
  if(!items.length)return null;
  const scope={binding_id:items[0].binding_id,path:privateParent(items[0])};
  for(const item of items)if(item.binding_id!==scope.binding_id||privateParent(item)!==scope.path)throw new Error('INVALID_PRIVATE_FILES');
  return scope;
 }
 class PrivateFiles{
  constructor(folder=''){this.folder=folder;this.items=[];this.cursor='';this.seen=new Set();this.cursors=new Set();this.generation=0;this.busy=false;}
  clear(){this.generation++;this.items=[];this.cursor='';this.seen=new Set();this.cursors=new Set();this.busy=false;}
  async read(fetcher,more=false){
   if(this.busy)return null;
   const append=more===true;if(append&&!this.cursor)return null;
   const cursor=append?this.cursor:'',generation=++this.generation;
   this.busy=true;
   try{
    const page=await fetcher('/api/v2/cloud-downloads/files?limit=50'+(this.folder?'&folder='+encodeURIComponent(this.folder):'')+(cursor?'&cursor='+encodeURIComponent(cursor):''));
    if(generation!==this.generation)return null;
    if(!page||!Array.isArray(page.items)||page.items.length>50||typeof page.next_cursor!=='string'||page.next_cursor.length>2048||!page.items.length&&page.next_cursor)throw new Error('INVALID_PRIVATE_FILES');
    const seen=append?new Set(this.seen):new Set(),cursors=append?new Set(this.cursors):new Set();
    if(page.next_cursor&&(page.next_cursor===cursor||cursors.has(page.next_cursor)))throw new Error('INVALID_PRIVATE_FILES');
    const entries=[];
    for(const item of page.items){
     if(!validPrivateItem(item))throw new Error('INVALID_PRIVATE_FILES');
     const identity=item.binding_id+'|'+item.resource_id;
     if(seen.has(identity))throw new Error('INVALID_PRIVATE_FILES');
     seen.add(identity);
     entries.push(presentPrivateItem(item));
    }
    const items=append?this.items.concat(entries):entries,scope=privateScope(items);
    if(this.folder&&scope&&scope.path!==this.folder)throw new Error('INVALID_PRIVATE_FILES');
    if(cursor)cursors.add(cursor);
    this.seen=seen;this.cursors=cursors;this.items=items;this.cursor=page.next_cursor;
    return {items:this.items.filter(item=>!item.directory),folders:this.items.filter(item=>item.directory),scope,cursor:this.cursor};
   }
   finally{if(generation===this.generation)this.busy=false;}
  }
 }
 const api=Object.freeze({validScope,within,partition,PrivateFiles});
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.FileSurface=api;
})(typeof window!=='undefined'?window:globalThis);
