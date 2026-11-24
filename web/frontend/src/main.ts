import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { appState } from './lib/stores/appState.svelte';

const app = mount(App, {
  target: document.getElementById('app')!,
});

// Hide static server-rendered remote-lock-modal shell after Svelte mounts so RemoteLockModal takes over seamlessly
const serverShell = document.getElementById('remote-lock-modal');
if (serverShell) {
  serverShell.classList.add('hidden');
}

if (typeof window !== 'undefined' && (import.meta as any).env?.DEV) {
  (window as any).appState = appState;
}

export default app;

