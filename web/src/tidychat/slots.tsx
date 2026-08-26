import { registerSlot } from '../framework'
import { TidySettingsCard } from './TidySettings'

/** tidychat slot registrations (side-effect module, mirrors traffic/slots).
 * The settings card binds into the settings panel via `settings.plugin.item`. */
function registerTidychatSlots(): void {
  registerSlot('settings.plugin.item', () => <TidySettingsCard />, { order: 0 })
}

registerTidychatSlots()