import {createApp} from 'vue'
import App from './App.vue'
import './style.css'

const colorScheme = window.matchMedia('(prefers-color-scheme: dark)')

function applyTheme(): void {
  const savedTheme = localStorage.getItem('theme')
  const useDark = savedTheme === 'dark' || (!savedTheme && colorScheme.matches)
  document.documentElement.classList.toggle('dark', useDark)
  document.documentElement.style.colorScheme = useDark ? 'dark' : 'light'
}

// Match the main Sub2API frontend before Vue mounts to avoid a light-theme flash.
applyTheme()
window.addEventListener('storage', (event) => {
  if (event.key === 'theme') applyTheme()
})
window.addEventListener('focus', applyTheme)
document.addEventListener('visibilitychange', () => {
  if (!document.hidden) applyTheme()
})
colorScheme.addEventListener('change', applyTheme)

createApp(App).mount('#app')
