(function(root){
 'use strict';
 const state={available:false,active:false,status:'UNAVAILABLE',mounted:false};
 function element(id){return document.getElementById(id);}
 function endpoint(){return state.active?'/api/v2/cloud-downloads':'/api/v1/cloud-downloads';}
 function render(){
  const panel=element('cloud-space-panel');if(!panel)return;
  panel.hidden=!state.available;
  element('cloud-space-status').textContent=state.active?'私有空间已启用。新下载使用您的独立目录；相同内容经核验后复制。':state.available?'可选择创建私有空间；未启用前仍使用原下载方式。':'暂时无法确认私有空间状态，原下载方式不受影响。';
  element('cloud-space-provision').hidden=state.active||!state.available;
 }
 function reset(){state.available=false;state.active=false;state.status='UNAVAILABLE';state.mounted=false;render();}
 async function init(){
  try{const response=await fetch('/api/v2/cloud-downloads/space',{credentials:'same-origin',cache:'no-store'});if(response.status===404){state.available=false;state.active=false;render();return;}const data=await response.json();if(!response.ok)throw new Error('SPACE_UNAVAILABLE');state.available=true;state.active=data.status==='ACTIVE';state.status=data.status;render();}
  catch(error){state.available=false;state.active=false;render();}
 }
 async function provision(){
  const button=element('cloud-space-provision');button.disabled=true;
  try{
   const body=JSON.stringify({operation_key:'space-'+crypto.randomUUID()});
   const response=await fetch('/api/v2/cloud-downloads/space/provision',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:body});
   const data=await response.json();
   if(!response.ok)throw new Error('PROVISION_FAILED');
   state.available=true;state.active=data.status==='ACTIVE';state.status=data.status;render();
  }
  catch(error){state.available=true;state.active=false;state.status='UNAVAILABLE';render();}
  finally{button.disabled=false;}
 }
 function mount(){if(state.mounted||!element('cloud-space-panel'))return Promise.resolve();state.mounted=true;element('cloud-space-provision').addEventListener('click',provision);return init();}
 const api={mount,init,provision,reset,endpoint,state};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.CloudSpace=api;
})(typeof window!=='undefined'?window:globalThis);