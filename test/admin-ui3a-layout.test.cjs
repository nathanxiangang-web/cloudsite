'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
test('command forms belong to authenticated main, not the hidden login shell',()=>{
 const html=fs.readFileSync(path.join(__dirname,'../internal/producthttp/web/index.html'),'utf8');
 const start=html.indexOf('id="workspace-view"'),main=html.indexOf('id="main-content"'),end=html.indexOf('</main>',main);
 for(const id of ['user-commands-panel','account-probe-panel','admin-action-panel']){const pos=html.indexOf('id="'+id+'"');assert.ok(pos>start&&pos>main&&pos<end,id+' must be in authenticated main');}
});
