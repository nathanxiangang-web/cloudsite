'use strict';
function closeShare(){node('share-form').hidden=true;node('share-result').hidden=true;node('share-code').value='';node('share-url').value='';node('share-status').textContent='';node('share-create').disabled=false;}
function clearSharing(){closeShare();node('owned-shares').replaceChildren();node('my-shares-panel').hidden=true;node('shares-status').textContent='';}
function shareErrorText(error){return ({SHARE_UNAVAILABLE:'分享已失效或不属于当前账号。',SHARE_LIMIT:'有效分享或访问次数达到上限，请撤销旧分享后重试。',SHARE_SERVICE_UNAVAILABLE:'分享服务暂时不可用。',INVALID_REQUEST:'请检查有效期和提取码（6–32 位非空格英文字符）。'})[error.code]||failureText(error);}
node('file-share').addEventListener('click',()=>{closeShare();node('share-form').hidden=false;node('share-duration').focus();});
node('share-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!library.selected||node('share-create').disabled)return;
 const item=library.selected,generation=library.detail,session=library.session;node('share-create').disabled=true;node('share-status').textContent='正在创建分享…';
 try{const result=await request('/api/v1/shares','POST',{binding_id:item.binding_id,resource_id:item.resource_id,path:item.path,duration:node('share-duration').value,code:node('share-code').value});
  if(generation!==library.detail||session!==library.session)return;
  node('share-url').value=result.url;node('share-result').hidden=false;node('share-status').textContent='分享已创建，有效期至 '+new Date(result.expires_at).toLocaleString()+'。请复制保存链接；当前入口仅在能访问本站的设备使用。';node('share-code').value='';
 }catch(error){if(generation===library.detail&&session===library.session)node('share-status').textContent=shareErrorText(error);}
 finally{if(generation===library.detail&&session===library.session)node('share-create').disabled=false;}
});
node('share-copy').addEventListener('click',async()=>{const value=node('share-url').value;if(!value)return;try{await navigator.clipboard.writeText(value);node('share-status').textContent='链接已复制。若设置了提取码，请单独告知接收者。';}catch{node('share-url').focus();node('share-url').select();node('share-status').textContent='请手动复制已选中的分享链接。';}});
async function loadShares(){
 const session=library.session;node('my-shares-panel').hidden=false;node('shares-status').textContent='正在加载…';
 try{const result=await request('/api/v1/shares');if(session!==library.session)return;node('owned-shares').replaceChildren();
  for(const item of result.items){const row=document.createElement('div');row.className='owned-share';const label=document.createElement('span');label.textContent=item.name+' · '+(item.active?'有效至 '+new Date(item.expires_at).toLocaleString():'已撤销或过期')+(item.protected?' · 有提取码':'');row.append(label);
   if(item.active){const revoke=document.createElement('button');revoke.type='button';revoke.className='secondary';revoke.textContent='撤销分享';revoke.addEventListener('click',async()=>{revoke.disabled=true;try{await request('/api/v1/shares/'+encodeURIComponent(item.id)+'/revoke','POST',{});if(session===library.session)await loadShares();}catch(error){if(session===library.session){node('shares-status').textContent=shareErrorText(error);revoke.disabled=false;}}});row.append(revoke);}node('owned-shares').append(row);
  }node('shares-status').textContent=result.items.length?'最近 '+result.items.length+' 条分享；撤销不会删除网盘文件。':'你还没有创建分享。';
 }catch(error){if(session===library.session)node('shares-status').textContent=shareErrorText(error);}
}
node('my-shares').addEventListener('click',loadShares);node('shares-close').addEventListener('click',()=>{node('my-shares-panel').hidden=true;});
node('file-download').addEventListener('click',async event=>{
 if(event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;event.preventDefault();if(library.downloading||!library.selected)return;
 const generation=library.detail,session=library.session,href=node('file-download').href,name=ResourceDisplay.name(library.selected);library.downloading=true;node('download-status').textContent='正在检查文件下载…';
 try{const result=await fetch(href,{method:'HEAD',credentials:'same-origin',cache:'no-store',signal:library.controller.signal});if(generation!==library.detail||session!==library.session)return;
  if(!result.ok){if(result.status===401){signOut();return;}throw new Error('Download refused');}
  const link=document.createElement('a');link.href=href;link.download=name;link.click();node('download-status').textContent='已交给浏览器下载；保存位置与进度请查看浏览器下载列表。';
 }catch(error){if(generation===library.detail&&session===library.session)node('download-status').textContent='暂时无法下载，请稍后重试。';}
 finally{if(generation===library.detail&&session===library.session)library.downloading=false;}
});
