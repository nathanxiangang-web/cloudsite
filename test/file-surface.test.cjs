'use strict';
const test=require('node:test');const assert=require('node:assert/strict');const fs=require('node:fs');const path=require('node:path');
const policy=require('../internal/producthttp/web/file-surface.js');
const scope={binding_id:'a',path:'/云下载'};
const rows=[{binding_id:'a',path:'/云下载/a.iso'},{binding_id:'a',path:'/云下载/nested/b.mp4'},{binding_id:'a',path:'/云下载备份/c.pdf'},{binding_id:'b',path:'/云下载/d.pdf'},{binding_id:'a',path:'/cloud/e.pdf'},{binding_id:'a',path:null}];
test('exact Binding and slash-delimited subtree partition; nested files included, sibling prefixes excluded',()=>{
 assert.deepEqual(policy.partition(rows,scope,'downloads'),rows.slice(0,2));
 assert.deepEqual(policy.partition(rows,scope,'library'),rows.slice(2));
 assert.equal(policy.within({binding_id:'a',path:'/云下载'},scope),false);
 assert.equal(policy.within({binding_id:'a',path:'/%E4%BA%91%E4%B8%8B%E8%BD%BD/a'},scope),false);
});
test('missing or malformed cloud target never empties the library or falls back to all files on downloads',()=>{
 for(const bad of [null,{}, {...scope,path:'/'},{...scope,path:'/云下载/'},{...scope,path:'/a/../云下载'},{...scope,path:'/a//b'},{...scope,path:'relative'}]){
  assert.equal(policy.validScope(bad),false);assert.deepEqual(policy.partition(rows,bad,'library'),rows);assert.deepEqual(policy.partition(rows,bad,'downloads'),[]);
 }
 assert.throws(()=>policy.partition(rows,scope,'unknown'));
});
test('every HTML surface gets one shared component and no duplicated action implementation',()=>{
 const web=path.join(__dirname,'../internal/producthttp/web'),fragment=fs.readFileSync(path.join(web,'file-browser.html'),'utf8');
 for(const page of ['library.html','downloads.html']){
  const raw=fs.readFileSync(path.join(web,page),'utf8');assert.equal(raw.split('<!--FILE_BROWSER-->').length,2);
  const rendered=raw.replace('<!--FILE_BROWSER-->',fragment),ids=[...rendered.matchAll(/id="([^"]+)"/g)].map(m=>m[1]);assert.equal(ids.length,new Set(ids).size,page);
  for(const id of ['file-grid','file-download','file-share','file-preview','share-form','file-search'])assert.ok(ids.includes(id));
  assert.match(raw,/assets\/library.js/);assert.match(raw,/assets\/library-sharing.js/);
 }
 const downloads=fs.readFileSync(path.join(web,'downloads.js'),'utf8');assert.match(downloads,/link.href='#download-files'/);assert.equal(downloads.includes('回到文件库查看'),false);assert.equal(downloads.includes('function previewMedia'),false);
});
