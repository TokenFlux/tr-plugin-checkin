import { configureMock } from '@tokenflux/tokenrouter-plugin-sdk/dev'
import { fixtureRequest, resetFixtures, setHistoryFailure, settings } from './preview/fixtures'

// dev.ts starts the SDK preview with the plugin's current endpoint fixtures.
// Its small author toolbar changes only in-memory test scenarios. Production
// library builds enter src/index.ts directly and therefore do not bundle these
// fixture controls or their simulated records.

configureMock(fixtureRequest)
await import('./src/index')

// Author-only controls exercise recoverable errors and paused states through the
// page's normal refresh handlers, without adding test-state routes to production.
const controls = document.createElement('div')
controls.style.cssText = 'display:flex;gap:8px;padding:12px;flex-wrap:wrap;font:13px system-ui;'
for (const [label, action] of [
  ['History failure once', setHistoryFailure],
  [
    'Pause / resume new check-ins',
    () => {
      settings.paused = !settings.paused
    },
  ],
  ['Reset preview records', resetFixtures],
] as const) {
  const button = document.createElement('button')
  button.textContent = label
  button.addEventListener('click', () => {
    action()
    window.dispatchEvent(new Event('focus'))
  })
  controls.append(button)
}
document.body.prepend(controls)
