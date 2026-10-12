import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';

function fixture(script, token='a'.repeat(64)) {
 const nodes=new Map();
 const node=id=>{
  if(!nodes.has(id))nodes.set(id,{value:'',textContent:'',innerHTML:'',disabled:false,hidden:false,listeners:{},elements:{password:{value:'new-password-123',focus(){}},confirm:{value:'new-password-123'},email:{value:'admin@example.com'},username:{value:'admin'}},addEventListener(event,callback){this.listeners[event]=callback;},querySelector(){return node(id+'button');}});
  return nodes.get(id);
 };
 const calls=[],history=[];
 let result={ok:true,message:'Password changed. Sign in with your new password.'};
 const context=vm.createContext({document:{querySelector:node},location:{hash:'#'+token,pathname:'/reset-password',search:'',replace(){}},history:{replaceState(...args){history.push(args);}},window:{listeners:{},addEventListener(event,callback){this.listeners[event]=callback;}},URLSearchParams,Error,fetch:async(url,options)=>{calls.push({url,options});return {ok:result.ok,json:async()=>result};}});
 vm.runInContext(readFileSync(new URL(`./static/${script}`,import.meta.url),'utf8'),context);
 return {node,calls,history,run:source=>vm.runInContext(source,context),reply:next=>result=next};
}

test('reset page removes fragment and sends secrets only in a POST body without credentials',async()=>{
 const app=fixture('reset-password.js');
 assert.deepEqual(app.history,[[null,'','/reset-password']]);
 app.node('#reset-password').elements.password.value='new-password-123';app.node('#reset-password').elements.confirm.value='new-password-123';
 await app.node('#reset-password').listeners.submit({preventDefault(){}});
 assert.equal(app.calls.length,1);const call=app.calls[0];
 assert.equal(call.url,'/gateway/v1/password-reset/confirm');assert.equal(call.options.method,'POST');assert.equal(call.options.credentials,'omit');
 assert.deepEqual(JSON.parse(call.options.body),{token:'a'.repeat(64),password:'new-password-123'});
 assert.equal(app.node('#reset-password').elements.password.value,'');assert.equal(app.node('#reset-password').elements.confirm.value,'');
 assert.equal(app.node('#reset-instructions').textContent,'Password changed. Sign in with your new password.');
 await app.node('#reset-password').listeners.submit({preventDefault(){}});assert.equal(app.calls.length,1);
});
test('invalid links and mismatched passwords are blocked before submission',async()=>{
 const invalid=fixture('reset-password.js','invalid');assert.equal(invalid.node('#reset-passwordbutton').disabled,true);
 await invalid.node('#reset-password').listeners.submit({preventDefault(){}});assert.equal(invalid.calls.length,0);
 const app=fixture('reset-password.js');app.node('#reset-password').elements.confirm.value='different';
 await app.node('#reset-password').listeners.submit({preventDefault(){}});assert.equal(app.calls.length,0);assert.match(app.node('#reset-status').textContent,/do not match/);
});
test('expired links and rate limits display errors and allow retry without retaining passwords',async()=>{
 for(const error of ['invalid or expired reset link','wait before retrying']){
  const app=fixture('reset-password.js');app.reply({ok:false,error});app.node('#reset-password').elements.password.value='new-password-123';app.node('#reset-password').elements.confirm.value='new-password-123';await app.node('#reset-password').listeners.submit({preventDefault(){}});
  assert.equal(app.node('#reset-status').textContent,error);assert.equal(app.node('#reset-passwordbutton').disabled,false);assert.equal(app.node('#reset-password').elements.password.value,'');
 }
});
test('login recovery sends an email address and displays the generic server response',async()=>{
 const app=fixture('login.js');app.reply({ok:true,message:'If this email matches, a reset link will be sent.'});
 await app.node('#password-reset-request').listeners.submit({preventDefault(){}});
 const call=app.calls.find(c=>c.url==='/gateway/v1/password-reset');assert.ok(call);assert.equal(call.options.method,'POST');assert.deepEqual(JSON.parse(call.options.body),{email:'admin@example.com'});
 assert.equal(app.node('#reset-message').textContent,'If this email matches, a reset link will be sent.');assert.equal(app.node('#password-reset-requestbutton').disabled,false);
});

test('opening a replacement link in the same tab captures and removes the new token',async()=>{
 const app=fixture('reset-password.js','invalid');
 app.run('location.hash="#"+"b".repeat(64);window.listeners.hashchange()');
 assert.equal(app.node('#reset-passwordbutton').disabled,false);
 app.node('#reset-password').elements.password.value='new-password-123';app.node('#reset-password').elements.confirm.value='new-password-123';
 await app.node('#reset-password').listeners.submit({preventDefault(){}});
 assert.equal(JSON.parse(app.calls[0].options.body).token,'b'.repeat(64));
 assert.equal(app.history.length,2);
});
