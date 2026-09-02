import { registerSlot } from '../framework'
import { SandboxSettingsCard } from './SandboxSettings'

/** sandbox slot registrations (side-effect module). */
function registerSandboxSlots(): void {
  registerSlot('settings.plugin.item', () => <SandboxSettingsCard />, { order: 2 })
}

registerSandboxSlots()