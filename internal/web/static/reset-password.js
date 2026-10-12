let token = '';
const form = document.querySelector('#reset-password');
const status = document.querySelector('#reset-status');
const button = form.querySelector('button');
const instructions = document.querySelector('#reset-instructions');
const originalInstructions = instructions.textContent;
function readResetLink() {
 token = location.hash.slice(1);
 // Remove the secret from the visible URL and current history entry immediately.
 history.replaceState(null, '', location.pathname);
 form.hidden = false;
 form.elements.password.value = ''; form.elements.confirm.value = '';
 status.textContent = ''; instructions.textContent = originalInstructions;
 if (!/^[a-f0-9]{64}$/.test(token)) {
  token = '';
  status.textContent = 'This reset link is invalid. Request a new link from the login page.';
 }
 button.disabled = !token;
}
readResetLink();
window.addEventListener('hashchange', readResetLink);
form.addEventListener('submit', async event => {
 event.preventDefault();
 if (!token) return;
 if (form.elements.password.value !== form.elements.confirm.value) {
  status.textContent = 'Passwords do not match.'; return;
 }
 button.disabled = true;
 const password = form.elements.password.value;
 form.elements.password.value = ''; form.elements.confirm.value = '';
 try {
  const response = await fetch('/gateway/v1/password-reset/confirm', {method:'POST', credentials:'omit', headers:{'Content-Type':'application/json'}, body:JSON.stringify({token,password})});
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || 'Unable to reset password. Request a new link from the login page.');
  token = ''; status.textContent = result.message; form.hidden = true;
  document.querySelector('#reset-instructions').textContent = result.message;
 } catch (error) { status.textContent = error.message; }
 finally { button.disabled = !token; }
});
