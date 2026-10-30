'use strict';
// Admin-only observation controllers; injected transport never exposes a root URL.
const AdminObserve={create({present,rowLabels,el,read,generation,principal,onError,cell,badge,dateText}){
 const storage={request:0,cursor:'',next:'',key:'',loaded:false,controller:null};
 const probe={request:0,binding:'',loaded:false,pending:false,controller:null};let scope='';
 function cancel(view){view.request++;view.controller?.abort();view.controller=null;view.pending=false;}
 function clearStorage(){storage.loaded=false;storage.next='';storage.key='';el('storage-rows').replaceChildren();el('storage-empty').hidden=true;el('more-storage').disabled=true;el('storage-footer').textContent='等待查询配置';}
 function clearRoot(){probe.loaded=false;el('root-facts').replaceChildren();el('root-status').textContent='尚未查询';el('root-status').className='badge neutral';el('root-evidence').textContent='没有 Q9 证据；不会自动探测';}
 function invalidate(){const wasPending=probe.pending;cancel(storage);cancel(probe);if(wasPending)el('root-status').textContent=(probe.loaded?'STALE':'UNKNOWN')+' · 查询已取消，没有新证据';el('refresh-storage').disabled=false;el('query-root').disabled=!probe.binding;el('more-storage').disabled=!storage.next;}
 function reset(){invalidate();storage.cursor='';probe.binding='';scope='';clearStorage();clearRoot();el('root-binding').replaceChildren();el('root-binding').value='';el('query-root').disabled=true;}
 function configure(bindings){if(!scope)clearRoot();const key=bindings.map(b=>b.binding_id+'|'+b.label).join(';');if(scope&&scope!==key){invalidate();clearStorage();clearRoot();storage.cursor='';}scope=key;el('root-binding').replaceChildren();for(const b of bindings){const o=document.createElement('option');o.value=b.binding_id;o.textContent=b.label;el('root-binding').append(o);}if(!bindings.some(b=>b.binding_id===probe.binding))probe.binding=bindings[0]?.binding_id||'';el('root-binding').value=probe.binding;if(!probe.binding){el('root-status').textContent='未配置可查询的存储空间';el('root-evidence').textContent='没有可信映射，不会发起 Q9 请求';}el('query-root').disabled=!probe.binding||probe.pending;}
 async function loadStorage(){
  if(!principal())return;cancel(storage);const request=storage.request,gen=generation(),key=storage.cursor,previous=storage.loaded&&storage.key===key;storage.controller=new AbortController();
  if(!previous)clearStorage();storage.key=key;el('storage-empty').hidden=true;el('refresh-storage').disabled=true;el('more-storage').disabled=true;el('storage-footer').textContent='LOADING · 正在读取配置，非在线检测';
  const q=new URLSearchParams({limit:'20'});if(storage.cursor)q.set('cursor',storage.cursor);
  try{const page=await read('/api/v1/admin/storage?'+q,gen,storage.controller.signal);if(gen!==generation()||request!==storage.request)return;if(!page||!Array.isArray(page.items)||page.items.length>20)throw Error('Invalid page');el('storage-rows').replaceChildren();for(const item of page.items){const row=document.createElement('tr');present(cell(row),item.display_label,item.binding_id,'空间编号');cell(row,item.provider_type==='115'?'115 网盘':item.provider_type);
    cell(row).append(badge(({ACTIVE:'启用配置',DISABLED:'停用配置'}[item.binding_config_status]||'配置未确认'),'neutral'));cell(row).append(badge(({ACTIVE:'启用配置',DISABLED:'停用配置'}[item.connection_config_status]||'配置未确认'),'neutral'));
    present(cell(row),'',[['空间配置状态',item.binding_config_status],['连接配置状态',item.connection_config_status],['Connection',item.connection_id],['Root 映射',item.indexcore_root_configured?'已配置':'未配置'],['空间配置更新',dateText(item.binding_updated_at)],['连接配置更新',dateText(item.connection_updated_at)]].map(([k,v])=>k+'：'+v).join('\n'),'连接记录');rowLabels(row,['名称','服务','空间配置','连接配置','技术详情']);el('storage-rows').append(row);}

   storage.loaded=true;storage.next=page.next_cursor||'';el('more-storage').disabled=!storage.next;el('storage-empty').hidden=page.items.length!==0;present(el('storage-footer'),'本页 '+page.items.length+' 个存储空间','CONFIGURED · GET /api/v1/admin/storage · 页面读取于 '+new Date().toLocaleString()+'（非数据库观察时间）','配置来源与时间');
  }catch(error){if(gen===generation()&&request===storage.request){el('storage-empty').hidden=true;el('storage-footer').textContent=previous?'STALE · 配置读取失败，保留上次同页数据':'UNKNOWN · 配置查询未完成，不以空结果代替失败';onError(error);}}
  finally{if(gen===generation()&&request===storage.request)el('refresh-storage').disabled=false;}
 }
 async function queryRoot(){
  if(!principal()||!probe.binding||probe.pending)return;const id=probe.binding,gen=generation(),request=++probe.request,previous=probe.loaded;probe.controller=new AbortController();probe.pending=true;el('query-root').disabled=true;el('root-status').textContent='正在读取索引状态';
  try{const result=await read('/api/v1/admin/indexcore?'+new URLSearchParams({binding_id:id}),gen,probe.controller.signal);if(gen!==generation()||request!==probe.request)return;
   if(result.binding_id!==id||!['NEW','ACTIVE','DEPRECATED','DELETED'].includes(result.root_lifecycle_state)||!Number.isSafeInteger(result.current_generation)||result.current_generation<0||result.last_applied_admission_seq!==null&&(!Number.isSafeInteger(result.last_applied_admission_seq)||result.last_applied_admission_seq<0)||result.evidence_source!=='INDEXCORE_Q9_ROOT_STATUS'||!Number.isFinite(new Date(result.observed_at).getTime()))throw Error('Invalid Q9 evidence');
   el('root-facts').replaceChildren();for(const [label,value] of [['Binding ID',result.binding_id],['Root 生命周期',result.root_lifecycle_state],['当前索引代数',result.current_generation],['Last applied admission seq',result.last_applied_admission_seq??'未提供 · null']]){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=String(value);row.append(dt,dd);el('root-facts').append(row);}
   probe.loaded=true;el('root-status').textContent='本次索引状态已读取';el('root-status').className='badge neutral';el('root-evidence').textContent=result.evidence_source+' · CloudSite 成功读取于 '+dateText(result.observed_at)+'（非上游观察时间，不证明文件新鲜度） · 客户端显示于 '+new Date().toLocaleString();
  }catch(error){if(gen===generation()&&request===probe.request){const kind={NOT_CONNECTED:'NOT_CONNECTED',ROOT_NOT_READY:'UNKNOWN / NOT_READY',ROOT_NOT_FOUND:'UNKNOWN / ROOT_NOT_FOUND',OBSERVATION_BUSY:'UNKNOWN / BUSY'}[error.code]||'UNKNOWN';el('root-status').textContent=(previous?'上次结果 · ':'')+'本次读取未确认';el('root-status').className='badge warning';if(!previous)el('root-evidence').textContent=kind+' · 未取得成功读取时间或代数，不推测在线/同步状态';onError(error);}}
  finally{if(gen===generation()&&request===probe.request){probe.pending=false;el('query-root').disabled=!probe.binding;}}
 }
 return {configure,reset,invalidate,loadStorage,queryRoot,
  refreshStorage(){storage.cursor='';return loadStorage();},moreStorage(){if(storage.next){storage.cursor=storage.next;return loadStorage();}},
  selectBinding(){cancel(probe);probe.binding=el('root-binding').value;clearRoot();el('query-root').disabled=!probe.binding;}};
}};
