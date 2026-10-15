'use strict';
function closeShare(){node('share-form').hidden=true;node('share-result').hidden=true;node('share-code').value='';node('share-url').value='';node('share-code-result').textContent='';library.createdShareCode='';node('share-status').textContent='';node('share-create').disabled=false;}
function randomShareCode(){const alphabet='ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';let value='';while(value.length<4){const bytes=crypto.getRandomValues(new Uint8Array(16));for(const byte of bytes){if(byte<248)value+=alphabet[byte%62];if(value.length===4)break;}}return value;}
function clearSharing(){closeShare();node('owned-shares').replaceChildren();node('my-shares-panel').hidden=true;node('shares-status').textContent='';}
function shareErrorText(error){return ({SHARE_UNAVAILABLE:'分享已失效或不属于当前账号。',SHARE_LIMIT:'有效分享或访问次数达到上限，请撤销旧分享后重试。',SHARE_SERVICE_UNAVAILABLE:'分享服务暂时不可用。',INVALID_REQUEST:'提取码应为 4 位大小写字母或数字，清空则无需验证。'})[error.code]||failureText(error);}
node('file-share').addEventListener('click',()=>{closeShare();node('share-code').value=randomShareCode();node('share-form').hidden=false;node('share-form').scrollIntoView({block:'nearest'});node('share-code').focus();});
node('share-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!library.selected||node('share-create').disabled)return;
 const item=library.selected,generation=library.detail,session=library.session,code=node('share-code').value;node('share-create').disabled=true;node('share-status').textContent='正在创建分享…';
 try{const result=await request('/api/v1/shares','POST',{binding_id:item.binding_id,resource_id:item.resource_id,path:item.path,duration:node('share-duration').value,code});
  if(generation!==library.detail||session!==library.session)return;
  library.createdShareCode=code;node('share-url').value=result.url;node('share-code-result').textContent=code?'提取码：'+code:'无需提取码';node('share-result').hidden=false;node('share-status').textContent='分享已创建，有效期至 '+new Date(result.expires_at).toLocaleString()+'。请复制保存分享信息；当前入口仅在能访问本站的设备使用。';
 }catch(error){if(generation===library.detail&&session===library.session)node('share-status').textContent=shareErrorText(error);}
 finally{if(generation===library.detail&&session===library.session)node('share-create').disabled=false;}
});
node('share-copy').addEventListener('click',async()=>{const value=node('share-url').value;if(!value)return;const info=value+(library.createdShareCode?'\n提取码：'+library.createdShareCode:'\n无需提取码');try{await navigator.clipboard.writeText(info);node('share-status').textContent='分享链接'+(library.createdShareCode?'和提取码':'')+'已复制。';}catch{node('share-url').focus();node('share-url').select();node('share-status').textContent='请手动复制已选中的链接'+(library.createdShareCode?'及下方提取码。':'。');}});
async function loadShares(){
 const session=library.session;node('my-shares-panel').hidden=false;node('shares-status').textContent='正在加载…';
 try{const result=await request('/api/v1/shares');if(session!==library.session)return;node('owned-shares').replaceChildren();
  for(const item of result.items){const row=document.createElement('div');row.className='owned-share';const label=document.createElement('span');label.textContent=item.name+' · '+(item.active?'有效至 '+new Date(item.expires_at).toLocaleString():'已撤销或过期')+(item.protected?' · 有提取码':'');row.append(label);
   if(item.active){const revoke=document.createElement('button');revoke.type='button';revoke.className='secondary';revoke.textContent='撤销分享';revoke.addEventListener('click',async()=>{revoke.disabled=true;try{await request('/api/v1/shares/'+encodeURIComponent(item.id)+'/revoke','POST',{});if(session===library.session)await loadShares();}catch(error){if(session===library.session){node('shares-status').textContent=shareErrorText(error);revoke.disabled=false;}}});row.append(revoke);}node('owned-shares').append(row);
  }node('shares-status').textContent=result.items.length?'最近 '+result.items.length+' 条分享；撤销不会删除网盘文件。':'你还没有创建分享。';
 }catch(error){if(session===library.session)node('shares-status').textContent=shareErrorText(error);}
}
node('my-shares').addEventListener('click',loadShares);node('shares-close').addEventListener('click',()=>{node('my-shares-panel').hidden=true;});
node('file-download').addEventListener('click',event=>{
 if(!library.selected||!library.content){event.preventDefault();return;}
 // Keep the real trusted click and native attachment navigation. Do not wait
 // for HEAD or manufacture a second detached-anchor click after it completes.
 node('download-status').textContent='已请求下载；请在浏览器下载列表查看进度。若出现错误页，请检查本站连接后重试。';
});
