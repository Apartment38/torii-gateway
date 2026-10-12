const form = document.querySelector('#session-login');
// A configured gateway redirects unauthenticated root visits here. Offer setup
// only while it is actually available, rather than linking back into that loop.
fetch('/gateway/v1/setup', {credentials: 'same-origin'})
  .then(response => response.ok ? response.json() : null)
  .then(setup => {
    if (setup?.setup_required) {
      document.querySelector('#login-help').innerHTML = 'Setting up a new gateway? <a href="/">Create your administrator</a>';
    }
  })
  .catch(() => {});
form.addEventListener('submit', async event => {
  event.preventDefault();
  const button = form.querySelector('button');
  const label = button.innerHTML;
  const errorMessage = document.querySelector('#error');
  button.disabled = true;
  button.textContent = 'Signing in…';
  errorMessage.textContent = '';
  const input = form.elements.password;
  const body = JSON.stringify({username: form.elements.username.value.trim(), password: input.value});
  input.value = '';
  try {
    const response = await fetch('/gateway/v1/session', {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json'}, body});
    if (!response.ok) throw new Error('Sign-in failed. Check your credentials; wait one minute if rate limited.');
    location.replace('/');
  } catch (error) {
    errorMessage.textContent = error.message;
    input.focus();
  } finally {
    button.disabled = false;
    button.innerHTML = label;
  }
});

if (new URLSearchParams(location.search).has('updated')) {
 document.querySelector('#error').textContent = 'Administrator updated. Sign in again.';
}
const resetForm = document.querySelector('#password-reset-request');
resetForm.addEventListener('submit', async event => {
 event.preventDefault();
 const button = resetForm.querySelector('button');
 const status = document.querySelector('#reset-message');
 button.disabled = true;
 try {
  const response = await fetch('/gateway/v1/password-reset', {method:'POST', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:JSON.stringify({email:resetForm.elements.email.value.trim()})});
  if (!response.ok) throw new Error('Unable to request a reset link. Please try again.');
  status.textContent = (await response.json()).message;
 } catch (error) { status.textContent = error.message; }
 finally { button.disabled = false; }
});
