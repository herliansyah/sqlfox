import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';

// Hide static server-rendered remote-lock-modal shell so Svelte's RemoteLockModal takes over cleanly without blur occlusion
const serverShell = document.getElementById('remote-lock-modal');
if (serverShell) {
  serverShell.classList.add('hidden');
}

const app = mount(App, {
  target: document.getElementById('app')!,
});

export default app;
