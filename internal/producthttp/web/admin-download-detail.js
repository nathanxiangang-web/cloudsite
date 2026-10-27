'use strict';
const AdminDownloadDetail={
 labels:{PENDING:'待提交',ACTIVE:'下载处理中',AWAITING_VISIBILITY:'等待索引观察',AWAITING_CANONICAL:'等待规范确认',READY:'已收录',FAILED:'失败',CANCELED:'已取消',RECOVERY_REQUIRED:'需人工处理'},
 uuid:value=>typeof value==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(value),
 time:value=>typeof value==='string'&&/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)&&!value.startsWith('0000-')&&!value.startsWith('0001-01-01')&&Number.isFinite(Date.parse(value))&&new Date(value.slice(0,10)+'T00:00:00Z').toISOString().slice(0,10)===value.slice(0,10),
 valid(row){
  const u=AdminDownloadDetail.uuid,t=AdminDownloadDetail.time;
  if(!row||![row.manifest_id,row.owner_user_id,row.binding_id].every(u)||!t(row.created_at)||!t(row.updated_at)||Date.parse(row.updated_at)<Date.parse(row.created_at)||![row.claim_attempts,row.failure_count].every(n=>Number.isSafeInteger(n)&&n>=0))return false;
  if(row.manifest_state==='PENDING'||row.manifest_state==='CANCELED'&&row.job_id===null)return row.job_id===null&&row.job_state===null&&row.result_copy_id===null&&row.claim_attempts===0&&row.failure_count===0&&(row.max_attempts===undefined||row.max_attempts===0)&&(row.copy===undefined||row.copy===null);
  const pairs={ACTIVE:['QUEUED','RUNNING','RETRY_WAIT'],AWAITING_VISIBILITY:['RUNNING','RETRY_WAIT'],AWAITING_CANONICAL:['RUNNING','RETRY_WAIT'],READY:['SUCCEEDED'],FAILED:['FAILED'],CANCELED:['CANCELED'],RECOVERY_REQUIRED:['RECOVERY_REQUIRED']};
  return u(row.job_id)&&Boolean(pairs[row.manifest_state]?.includes(row.job_state))&&(row.manifest_state==='READY'?u(row.result_copy_id):row.result_copy_id===null);
 },
 create({el,read,generation,principal,isActive,dateText,bindingAllowed,onChange}){
  let seq=0,controller,focus,id='';
  function close(restore=false){onChange?.(null);seq++;controller?.abort();controller=null;id='';el('admin-download-detail').hidden=true;el('admin-download-facts').replaceChildren();el('admin-download-detail-status').textContent='';el('admin-download-detail-refresh').disabled=false;if(restore)focus?.focus();}
  function facts(rows){const list=el('admin-download-facts');list.replaceChildren();for(const [label,value]of rows){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=String(value);row.append(dt,dd);list.append(row);}}
  function validate(d,target){
   const t=AdminDownloadDetail.time,u=AdminDownloadDetail.uuid;
   if(!AdminDownloadDetail.valid(d)||!bindingAllowed(d.binding_id)||d.manifest_id!==target||d.source!=='CLOUDSITE_POSTGRES_MANIFEST_JOB_COPY'||!t(d.read_at)||!Number.isSafeInteger(d.max_attempts)||d.max_attempts<0||typeof d.error_recorded!=='boolean'||!['NONE','RECORDED_REDACTED','POLL_TRANSPORT_TIMEOUT','POLL_TRANSPORT_OTHER','POLL_HTTP_STATUS','POLL_DECODE','POLL_PROVIDER_TEMPORARY','POLL_PERMANENT_OR_UNKNOWN'].includes(d.error_code)||d.error_recorded!==(d.error_code!=='NONE'))throw Error('Invalid detail');
   if(d.job_id===null){if(d.max_attempts!==0||d.error_recorded||[d.job_created_at,d.job_updated_at,d.started_at,d.finished_at,d.next_attempt_at].some(v=>v!==null))throw Error('Unexpected job');}
   else {if(d.max_attempts<1||d.failure_count>d.max_attempts||!t(d.job_created_at)||!t(d.job_updated_at)||Date.parse(d.job_updated_at)<Date.parse(d.job_created_at))throw Error('Invalid job');for(const stamp of [d.started_at,d.finished_at,d.next_attempt_at])if(stamp!==null&&!t(stamp))throw Error('Invalid time');for(const stamp of [d.started_at,d.finished_at])if(stamp!==null&&(Date.parse(stamp)<Date.parse(d.job_created_at)||Date.parse(stamp)>Date.parse(d.job_updated_at)))throw Error('Invalid order');if(d.job_state==='RETRY_WAIT'&&d.next_attempt_at===null)throw Error('Missing retry time');}
   if(d.result_copy_id===null){if(d.copy!==null)throw Error('Unexpected copy');}else if(!d.copy||!u(d.copy.copy_id)||d.copy.copy_id!==d.result_copy_id||d.copy.binding_id!==d.binding_id||!['PRESENT','REMOVED'].includes(d.copy.availability)||!t(d.copy.updated_at))throw Error('Invalid copy');
  }
  async function load(target,refresh=false){
   if(principal()?.role!=='ADMIN'||!isActive())return;
   if(!AdminDownloadDetail.uuid(target)){el('admin-download-lookup-status').textContent='请输入完整的 Manifest UUID。';return;}
   onChange?.(null);if(!refresh)focus=document.activeElement;seq++;controller?.abort();controller=new AbortController();id=target;
   const request=seq,gen=generation(),actor=principal().user_id,signal=controller.signal;
   const current=()=>request===seq&&gen===generation()&&!signal.aborted&&isActive()&&principal()?.role==='ADMIN'&&principal()?.user_id===actor;
   el('admin-download-lookup-status').textContent='';el('admin-download-detail').hidden=false;el('admin-download-facts').replaceChildren();el('admin-download-detail-status').textContent='LOADING · 正在读取任务详情';el('admin-download-detail-refresh').disabled=true;el('admin-download-detail').focus();el('admin-download-detail').scrollIntoView({block:'nearest'});
   try{const d=await read('/api/v1/admin/downloads/'+target,gen,signal);if(!current())return;validate(d,target);
    const explanation={PENDING:'尚未激活，没有执行 Job。',ACTIVE:'Provider 阶段；本页面不查询网盘进度。',AWAITING_VISIBILITY:'等待可信 Hint / 索引观察；不是下载失败。',AWAITING_CANONICAL:'等待规范资源与投影 Copy 的精确确认。',READY:'历史获取已成功；当前文件可用性应另看 Copy 证据。',FAILED:'已记录失败终态；此页不重试。',CANCELED:'已记录取消终态；不表示网盘文件被删除。',RECOVERY_REQUIRED:'结果或恢复条件需人工核实；不会自动重试或再次发起下载。'};
    facts([['Manifest',d.manifest_id],['用户',d.owner_user_id],['Binding',d.binding_id],['请求状态',AdminDownloadDetail.labels[d.manifest_state]+' · '+d.manifest_state],['状态说明',explanation[d.manifest_state]],['Job',d.job_id??'未创建 Job'],['执行状态',d.job_state??'未创建 Job'],['领取代数 / 已消耗失败预算',d.claim_attempts+' / '+d.failure_count],['失败预算上限',d.job_id?d.max_attempts:'无 Job'],['已记录错误',d.error_recorded?(d.error_code==='RECORDED_REDACTED'?'存在历史错误，原文已隐藏，具体原因未提供':d.error_code+' · '+({POLL_TRANSPORT_TIMEOUT:'已记录轮询请求超时',POLL_TRANSPORT_OTHER:'已记录轮询传输错误',POLL_HTTP_STATUS:'已记录轮询 HTTP 状态异常',POLL_DECODE:'已记录轮询响应解析失败',POLL_PROVIDER_TEMPORARY:'已记录上游暂时性错误',POLL_PERMANENT_OR_UNKNOWN:'已记录永久或未知错误，具体原因未确认'}[d.error_code])):'无已记录错误（非健康证明）'],['Manifest 创建',dateText(d.created_at)],['Manifest 更新',dateText(d.updated_at)],['Job 创建',dateText(d.job_created_at)],['Job 更新（非最近失败时间）',dateText(d.job_updated_at)],['首次执行开始（非最近尝试）',dateText(d.started_at)],['执行结束记录',dateText(d.finished_at)],['已安排下次执行',dateText(d.next_attempt_at)],['结果 Copy',d.result_copy_id??'尚无结果 Copy'],['Copy 当前投影状态',d.copy?d.copy.availability+'（不是即时网盘检测）':'尚无 Copy 证据'],['Copy 投影更新',dateText(d.copy?.updated_at)],['数据库读取',dateText(d.read_at)]]);
    onChange?.(d);el('admin-download-detail-status').textContent=d.manifest_state==='RECOVERY_REQUIRED'?'RECOVERY_REQUIRED · 需人工处理，禁止盲目重试':'已读取持久状态 · 不表示网盘在线';return d;
   }catch(error){if(current()){el('admin-download-facts').replaceChildren();el('admin-download-detail-status').textContent=error.code==='DOWNLOAD_NOT_FOUND'?'NOT_FOUND · 任务不存在或不在当前管理范围':error.name==='AbortError'?'查询已取消':'UNAVAILABLE · 详情读取失败，未确认任何成功状态';}}
   finally{if(current())el('admin-download-detail-refresh').disabled=false;}
  }
  return {load,close,reset(){close();el('admin-download-id').value='';el('admin-download-lookup-status').textContent='';},invalidate:close,refresh:()=>id?load(id,true):undefined};
 }
};
