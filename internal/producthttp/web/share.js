'use strict';
const guestNode=id=>document.getElementById(id);const guestToken=location.hash.slice(1);
if(!/^[A-Za-z0-9_-]{43}$/.test(guestToken)){guestNode('redeem-submit').disabled=true;guestNode('share-message').textContent='分享链接不完整，请向分享者索取新链接。';}
guestNode('redeem-form').addEventListener('submit',async event=>{
 event.preventDefault();if(guestNode('redeem-submit').disabled)return;guestNode('redeem-submit').disabled=true;guestNode('shared-file').hidden=true;guestNode('shared-download').removeAttribute('href');guestNode('share-message').textContent='正在验证分享…';
 try{const response=await fetch('/api/v1/shares/redeem',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:guestToken,code:guestNode('redeem-code').value})});const result=await response.json();
  if(!response.ok){const error=new Error('Share unavailable');error.code=result.code;throw error;}
  if(!/^\/api\/v1\/shared\/[0-9a-f-]{36}\/content$/.test(result.download_url))throw new Error('Invalid target');
  guestNode('shared-name').textContent=result.name;guestNode('shared-info').textContent=result.size_bytes.toLocaleString()+' 字节 · 有效至 '+new Date(result.expires_at).toLocaleString();guestNode('shared-download').href=result.download_url;guestNode('shared-file').hidden=false;guestNode('share-message').textContent='验证成功。点击下载原文件；不会修改网盘内容。';
 }catch(error){guestNode('share-message').textContent=error.code==='SHARE_BUSY'?'访问频繁，请一分钟后再试。':'链接或提取码无效、已过期或被撤销。';}
 finally{guestNode('redeem-code').value='';guestNode('redeem-submit').disabled=false;}
});
guestNode('shared-download').addEventListener('click',async event=>{
 if(event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;event.preventDefault();const href=guestNode('shared-download').href;if(!href)return;
 guestNode('share-message').textContent='正在检查下载权限…';
 try{const response=await fetch(href,{method:'HEAD',credentials:'same-origin',cache:'no-store'});if(!response.ok)throw new Error('Share unavailable');const link=document.createElement('a');link.href=href;link.download=guestNode('shared-name').textContent;link.click();guestNode('share-message').textContent='已交给浏览器下载，请查看浏览器下载列表。';}
 catch{guestNode('share-message').textContent='当前分享已不可用或连接失败，请联系分享者。';}
});
