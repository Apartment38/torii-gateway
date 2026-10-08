const form=document.querySelector('#session-login');
form.addEventListener('submit',async event=>{
 event.preventDefault();const button=form.querySelector('button');button.disabled=true;
 const input=form.elements.password;const body=JSON.stringify({username:form.elements.username.value,password:input.value});input.value='';
 try{const response=await fetch('/gateway/v1/session',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body});if(!response.ok)throw new Error('Sign-in failed. Check your credentials; wait one minute if rate limited.');location.replace('/');}
 catch(error){document.querySelector('#error').textContent=error.message;}finally{button.disabled=false;}
});
