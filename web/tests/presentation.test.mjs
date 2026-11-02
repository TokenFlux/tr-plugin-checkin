import assert from 'node:assert/strict'
import test from 'node:test'
import { HostRequestError } from '@tokenflux/tokenrouter-plugin-sdk'
import {
  calendarDays,
  exactAmountLabel,
  monthLabel,
  needsExactAmount,
  recordState,
  requestFailure,
  shiftMonth,
} from '../src/presentation.ts'

// presentation.test.mjs protects date-only calendar behavior and financial status
// labels without requiring a browser or a live host. Tests run under an unrelated
// machine timezone so calendar dates cannot accidentally depend on the developer's
// local day when rendering the host's selected month.

// November begins on Sunday and ends in a sixth visible week. The calendar must
// retain Monday-first alignment and exactly one cell for each real date.
test('calendar uses host dates across complete Monday-first weeks', () => {
  const cells = calendarDays('2026-11', [])
  assert.equal(cells.length, 42)
  assert.equal(cells[6].day, '2026-11-01')
  assert.equal(cells[7].day, '2026-11-02')
  assert.equal(cells[35].day, '2026-11-30')
  assert.equal(cells.filter((cell) => cell.day).length, 30)
})

// Leap years contain an extra February date, and December navigation changes the
// year. These boundaries must remain valid without inheriting a local UTC offset.
test('month navigation and leap-day placement preserve ISO calendar boundaries', () => {
  assert.equal(calendarDays('2024-02', []).filter((cell) => cell.day).length, 29)
  assert.equal(calendarDays('2025-02', []).filter((cell) => cell.day).length, 28)
  assert.equal(shiftMonth('2026-12', 1), '2027-01')
  assert.equal(shiftMonth('2026-01', -1), '2025-12')
  assert.equal(monthLabel('2026-11', 'en'), 'November 2026')
})

// A record for another month can share the same day number. Only the full ISO
// date may attach its completion and reward to a cell in the selected month.
test('calendar does not attach stale month records by day number', () => {
  const record = { user_id: 1, day: '2026-10-01', amount: '0.1', completed: true, at: 0 }
  const cells = calendarDays('2026-11', [record])
  assert.equal(cells.find((cell) => cell.day === '2026-11-01').record, undefined)
})

// A zero-value completion records attendance without crediting a wallet, while
// an unfinished intent cannot prove payment succeeded even when its amount is positive.
test('reward states distinguish credits, zero rewards, and unconfirmed intent', () => {
  const base = { user_id: 1, day: '2026-11-01', amount: '0.1', completed: true, at: 0 }
  assert.equal(recordState(base), 'received')
  assert.equal(recordState({ ...base, amount: '0.00000000' }), 'recorded')
  assert.equal(recordState({ ...base, completed: false }), 'pending')
})

// Host quota failures require administrator action rather than an unconditional
// retry prompt. Unrecognized failures retain the caller's contextual fallback message.
test('known host failures select actionable messages', () => {
  assert.equal(
    requestFailure(new HostRequestError('Limit', 429, 'LIMIT_EXCEEDED'), 'claimFailed'),
    'limitFailure',
  )
  assert.equal(requestFailure(new Error('Network'), 'historyLoadFailed'), 'historyLoadFailed')
})

// The wallet accepts eight decimal places, including values a two-decimal amount
// component would display as zero. Exact labels must retain those digits and grouping.
test('recorded rewards retain precision beyond the native two-decimal amount', () => {
  assert.equal(needsExactAmount('0.00000001'), true)
  assert.equal(needsExactAmount('0.10000000'), false)
  assert.equal(exactAmountLabel('1000000.00000001', 'en'), '1,000,000.00000001')
  assert.equal(exactAmountLabel('1.23450000', 'zh-Hans'), '1.2345')
})
