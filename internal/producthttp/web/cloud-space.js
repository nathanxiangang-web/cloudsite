(function(root){
 'use strict';
 const state={available:false,active:false,required:false,status:'UNAVAILABLE',mounted:false,generation:0,onActive:null};
 function element(id){return document.getElementById(id);}
 function endpoint(){if(state.active)return '/api/v2/cloud-downloads';return state.required?null:'/api/v1/cloud-downloads';}
 function render(){
  const panel=element('cloud-space-panel');if(!panel)return;
  panel.hidden=!state.available&&!state.required;
  element('cloud-space-status').textContent=state.active?'私有空间已启用。新下载使用您的独立目录。':state.available?(state.required?'开通私有空间后即可提交下载。':'可选择创建私有空间；未启用前仍使用原下载方式。'):(state.required?'暂时无法确认私有空间状态，请刷新后重试。':'暂时无法确认私有空间状态，原下载方式不受影响。');
  element('cloud-space-provision').hidden=state.active||!state.available;
 }
 function reset(){state.generation++;state.available=false;state.active=false;state.required=false;state.status='UNAVAILABLE';state.mounted=false;state.onActive=null;render();}
 async function init(){
  const generation=++state.generation;
  try{
   const response=await fetch('/api/v2/cloud-downloads/space',{credentials:'same-origin',cache:'no-store'});
   if(generation!==state.generation)return;
   if(response.status===404){state.available=false;state.active=false;render();return;}
   const data=await response.json();if(generation!==state.generation)return;
   if(!response.ok)throw new Error('SPACE_UNAVAILABLE');
   state.available=true;state.active=data.status==='ACTIVE';state.status=data.status;render();
  }
  catch(error){if(generation===state.generation){state.available=false;state.active=false;render();}}
 }
 async function provision(){
  const button=element('cloud-space-provision');button.disabled=true;
  const generation=++state.generation;let notify=false;
  try{
   const body=JSON.stringify({operation_key:'space-'+crypto.randomUUID()});
   const response=await fetch('/api/v2/cloud-downloads/space/provision',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:body});
   const data=await response.json();if(generation!==state.generation)return;
   if(!response.ok)throw new Error('PROVISION_FAILED');
   state.available=true;state.active=data.status==='ACTIVE';state.status=data.status;notify=state.active&&typeof state.onActive==='function';render();
  }
  catch(error){if(generation===state.generation){state.available=true;state.active=false;state.status='UNAVAILABLE';render();}}
  finally{button.disabled=false;}
  if(notify&&generation===state.generation)await state.onActive();
 }
 function mount(onActive,required=false){state.required=required;state.onActive=typeof onActive==='function'?onActive:null;if(state.mounted||!element('cloud-space-panel'))return Promise.resolve();state.mounted=true;element('cloud-space-provision').addEventListener('click',provision);return init();}
 const api={mount,init,provision,reset,endpoint,state};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.CloudSpace=api;
})(typeof window!=='undefined'?window:globalThis);
