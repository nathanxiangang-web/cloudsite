'use strict';
// Bounded Q4 composition. Directories are traversal details, never file cards.
const FileCatalog = (() => {
 class Catalog {
  constructor(bindings){
   if(!Array.isArray(bindings)||!bindings.length||bindings.length>64)throw new Error('Invalid library');
   this.pending=[];this.scopes=new Set();this.seen=new Set();this.files=[];this.pages=0;this.unknown=0;this.limited=false;
   for(const binding of bindings){
    if(!binding.binding_id||this.scopes.has(binding.binding_id+'|'))throw new Error('Invalid library');
    this.scopes.add(binding.binding_id+'|');this.pending.push({binding:binding.binding_id,parent:null,cursor:'',cursors:new Set()});
   }
  }
  get complete(){return !this.pending.length&&!this.limited;}
  async round(read, current=()=>true){
   if(this.limited)return;
   for(let step=0;step<4&&this.pending.length;step++){
    if(!current())return;
    if(this.pages>=32){this.limited=true;return;}
    const scope=this.pending[0];const page=await read(scope);
    if(!current())return;
    if(!page||!Array.isArray(page.items)||page.items.length>50||typeof page.next_cursor!=='string'||page.next_cursor.length>2048)throw new Error('Invalid page');
    if(page.next_cursor&&(page.next_cursor===scope.cursor||scope.cursors.has(page.next_cursor)))throw new Error('Repeated cursor');
    const ids=new Set();const children=[];const files=[];let unknown=0;
    for(const item of page.items){
     if(!item||item.binding_id!==scope.binding||typeof item.resource_id!=='string'||!item.resource_id||item.presence!=='PRESENT')throw new Error('Invalid identity');
     const identity=scope.binding+'|'+item.resource_id;
     if(ids.has(identity)||this.seen.has(identity))throw new Error('Repeated identity');
     ids.add(identity);
     if(item.directory===true){
      if(this.scopes.has(identity))throw new Error('Repeated directory');
      children.push({binding:scope.binding,parent:item.resource_id,cursor:'',cursors:new Set()});
     }else if(item.directory===false){files.push(item);}else{unknown++;}
    }
    if(this.scopes.size+children.length>256){this.limited=true;return;}
    this.pending.shift();this.pages++;this.unknown+=unknown;
    for(const id of ids)this.seen.add(id);
    this.files.push(...files);
    if(page.next_cursor){scope.cursors.add(scope.cursor);scope.cursor=page.next_cursor;this.pending.push(scope);}
    for(const child of children){this.scopes.add(child.binding+'|'+child.parent);this.pending.push(child);}
   }
   if(this.pending.length&&this.pages>=32)this.limited=true;
  }
 }
 return Object.freeze({Catalog});
})();
