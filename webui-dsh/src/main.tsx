/**
 * Application entry point — thin bootstrap over the app.
 */
import './styles/components.css'
import { AppShell } from './app/App.tsx'
import { createRoot } from 'react-dom/client'

const el = document.getElementById('root')
if (el === null) throw new Error('dsh-shell: missing #root')

createRoot(el).render(<AppShell />)
