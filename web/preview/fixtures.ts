import { HostRequestError } from '@tokenflux/tokenrouter-plugin-sdk'
import type { RequestOptions } from '@tokenflux/tokenrouter-plugin-sdk'
import type { CheckinRecord, HistoryPage, RewardSettings } from '../src/types'

// fixtures.ts provides explicit browser-development responses for the check-in
// pages. Records and failure controls exist only in the current preview process;
// production bundles never import this module. These fixtures support visual and
// interaction checks without calling a live wallet or changing host configuration.
//
// The fixture's fixed date keeps screenshots repeatable and makes the previous
// month contain both completed and pending examples. Requests return detached
// history objects so Vue state cannot mutate the fixture through a response.
// This model exercises presentation contracts only: its in-memory completion and
// cent totals do not establish persistence, concurrent credit safety, or actual
// wallet behavior in a running TokenRouter installation.

// The fixed preview day and mutable records make calendar comparisons repeatable.
// Changing these values affects only the preview and never persists host records.
export const today = '2026-11-02'
export let records: CheckinRecord[] = []
export let failHistory = false
export const settings: RewardSettings = {
  reward: '0.1',
  paused: false,
  notice: '',
  timezone: 'UTC',
}

/** resetFixtures restores sample records and an available daily reward for development. */
export function resetFixtures(): void {
  records = []
  settings.paused = false
  failHistory = false
  for (let day = 0; day < 20; day++) {
    const date = new Date('2026-11-01T10:00:00Z')
    date.setUTCDate(date.getUTCDate() - day)
    records.push({
      user_id: 1,
      day: date.toISOString().slice(0, 10),
      amount: day === 5 ? '0' : day > 10 ? '0.2' : '0.1',
      completed: day !== 2,
      at: date.getTime() / 1000,
    })
  }
  records.push({ user_id: 2, day: '2026-11-01', amount: '0.1', completed: true, at: 1793527200 })
}

/** setHistoryFailure makes the next history request fail without discarding its data. */
export function setHistoryFailure(): void {
  failHistory = true
}

/** fixtureRequest models declared plugin endpoints using isolated in-memory examples.
 * The development host handles cancellation around this callback. Unsupported
 * endpoints fail explicitly, and duplicate claims retain the completed daily record.
 *
 * Personal requests always use the fixture user, while administrator filters can
 * inspect the second sample recipient. Table pages and calendar rows are selected
 * independently from the same records, allowing month changes and pending states
 * to be exercised without implying that a real host operation was performed.
 */
export async function fixtureRequest(
  method: string,
  path: string,
  options?: RequestOptions,
): Promise<unknown> {
  if (method === 'GET' && path === '/status') {
    const record = records.find((row) => row.user_id === 1 && row.day === today)
    return {
      checked_in: !!record?.completed,
      state: record
        ? record.completed
          ? 'completed'
          : 'pending'
        : settings.paused
          ? 'paused'
          : 'available',
      reward: record?.amount ?? settings.reward,
      day: today,
      timezone: settings.timezone,
      server_time: 1793620800,
      next_reset_at: 1793664000,
      paused: settings.paused,
      notice: settings.notice,
    }
  }
  if (method === 'POST' && path === '/checkin') {
    const record = records.find((row) => row.user_id === 1 && row.day === today)
    if (record?.completed)
      throw new HostRequestError('Already checked in', 409, 'ALREADY_CHECKED_IN')
    if (!record && settings.paused) throw new HostRequestError('Paused', 503, 'CHECKIN_PAUSED')
    if (record) record.completed = true
    else
      records.push({
        user_id: 1,
        day: today,
        amount: settings.reward,
        completed: true,
        at: 1793620800,
      })
    return { day: today, reward: record?.amount ?? settings.reward }
  }
  if (method === 'GET' && path === '/admin/settings') return { ...settings }
  if (method !== 'GET' || !['/history', '/admin/history'].includes(path)) {
    throw new HostRequestError('Unknown fixture endpoint', 501, 'MOCK_NOT_CONFIGURED')
  }
  if (failHistory) {
    failHistory = false
    throw new HostRequestError('Preview history unavailable', 503, 'HOST_UNAVAILABLE')
  }
  const query = options?.query ?? {}
  const personal = path === '/history'
  const user = personal ? 1 : Number(query.user_id || 0)
  const month = query.month || (personal ? today.slice(0, 7) : '')
  const owned = records
    .filter((row) => !user || row.user_id === user)
    .sort((a, b) => b.day.localeCompare(a.day) || b.at - a.at)
  const calendar = personal ? owned.filter((row) => row.day.startsWith(month)) : []
  const matching = owned.filter(
    (row) =>
      (!(personal && query.scope === 'all') && month ? row.day.startsWith(month) : true) &&
      (query.state ? row.completed === (query.state === 'completed') : true),
  )
  const page = Math.min(Number(query.page || 1), Math.max(1, Math.ceil(matching.length / 10)))
  const result: HistoryPage = {
    rows: matching.slice((page - 1) * 10, page * 10),
    calendar,
    months: [...new Set(owned.map((row) => row.day.slice(0, 7)))],
    month: personal ? month : '',
    total: matching.length,
    page,
    page_size: 10,
    summary: summarize(owned),
    month_summary: summarize(calendar),
  }
  return structuredClone(result)
}

/** summarize totals preview rewards in integer cents for these two-decimal fixtures.
 * Pending records are excluded from both the completed count and the displayed
 * reward total. Zero rewards still count as a completed check-in without adding
 * money, matching the distinction the page must show to users.
 * This calculation only supports the preview's sample amounts; exact production
 * totals remain the responsibility of the backend history response.
 */
function summarize(rows: CheckinRecord[]): { count: number; amount: string } {
  const completed = rows.filter((row) => row.completed)
  // Preview amounts have at most two decimals; integer cents keep their displayed
  // totals consistent without representing this fixture as the production ledger.
  const cents = completed.reduce((sum, row) => sum + Math.round(Number(row.amount) * 100), 0)
  return { count: completed.length, amount: (cents / 100).toFixed(2) }
}

resetFixtures()
