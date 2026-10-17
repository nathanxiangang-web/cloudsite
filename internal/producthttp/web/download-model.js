(function(root){
 'use strict';
 const validKey=value=>typeof value==='string'&&/^[A-Za-z0-9._-]{1,64}$/.test(value);
 const validID=value=>typeof value==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value);
 const userKey=user=>{if(typeof user!=='string'||!/^[0-9a-f-]{36}$/.test(user))throw new Error('INVALID_OWNER');return 'cloudsite-pending-download:'+user;};
 function storage(value){return value||root.sessionStorage;}
 function pending(user,store){try{const key=storage(store).getItem(userKey(user));return validKey(key)?key:null;}catch(error){return null;}}
 function savePending(user,key,store){if(!validKey(key))throw new Error('INVALID_KEY');storage(store).setItem(userKey(user),key);}
 function clearPending(user,store){try{storage(store).removeItem(userKey(user));}catch(error){}}
 class Intent {
  constructor(key=null){if(key!==null&&!validKey(key))throw new Error('INVALID_KEY');this.key=key;this.source=null;this.expected=null;this.busy=false;this.uncertain=key!==null;}
  begin(source,expected,createKey){
   if(this.busy)throw new Error('BUSY');
   if(this.source!==null&&(source!==this.source||expected!==this.expected))throw new Error('FROZEN_INTENT');
   if(!this.key){this.key=createKey();if(!validKey(this.key))throw new Error('INVALID_KEY');}
   this.source=source;this.expected=expected;this.busy=true;this.uncertain=true;
   const body={operation_key:this.key,source};if(expected!=='')body.expected_name=expected;return body;
  }
  failed(){this.busy=false;this.uncertain=true;}
  accepted(result){if(!result||!['CREATED','REPLAY','PENDING_ACTIVATION'].includes(result.status)||!validID(result.manifest_id)||!validID(result.job_id))throw new Error('INVALID_RESULT');this.busy=false;this.uncertain=result.status==='PENDING_ACTIVATION';if(!this.uncertain){this.key=null;this.source=null;this.expected=null;}return !this.uncertain;}
  clear(){this.key=null;this.source=null;this.expected=null;this.busy=false;this.uncertain=false;}
 }
 function taskLabel(task){
  if(task.manifest_state==='READY')return task.job_state==='SUCCEEDED'&&typeof task.result_copy_id==='string'&&task.result_copy_id?'已收录，可在本页查看':'正在核实结果';
  return ({PENDING:'提交已保存，等待激活',AWAITING_VISIBILITY:'下载完成，等待收录',AWAITING_CANONICAL:'正在确认文件',RECOVERY_REQUIRED:'需要处理，请勿重复提交',FAILED:'下载失败',CANCELED:'已取消'})[task.manifest_state]||(task.manifest_state==='ACTIVE'?(task.job_state==='RUNNING'?'网盘下载中':task.job_state==='RETRY_WAIT'?'等待下一次查询':'任务已排队'):'正在核实状态');
 }
 const api={Intent,validKey,pending,savePending,clearPending,taskLabel};
 if(typeof module!=='undefined'&&module.exports)module.exports=api;else root.CloudDownloadModel=api;
})(typeof window!=='undefined'?window:globalThis);
