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
 const api=Object.freeze({validScope,within,partition});
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.FileSurface=api;
})(typeof window!=='undefined'?window:globalThis);
