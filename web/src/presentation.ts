import { HostRequestError } from '@tokenflux/tokenrouter-plugin-sdk'
import type { CheckinRecord } from './types'

// presentation.ts formats server-calendar dates and maps stable API failures to
// localized recovery messages. Date-only calculations use UTC solely as a calendar
// arithmetic container; the browser's timezone never decides which day the host
// considers signed, missed, current, or still in the future.
//
// Recorded rewards can carry eight fractional digits, while the host amount
// component defaults to two. Exact labels preserve fractional text and use
// integer grouping separately, avoiding a display-only float conversion that
// could hide small rewards. These helpers never derive wallet inputs from their
// formatted strings; the backend remains responsible for amounts and identities.

/** CalendarDay pairs one calendar position with its persisted check-in record. */
export interface CalendarDay {
  day: string // Host-calendar date or empty padding.
  number: number // One-based calendar day number.
  record?: CheckinRecord // Optional persisted daily reward intent.
}

/** monthLabel formats an ISO month without shifting it through local timezone.
 * Callers supply a validated server month and supported host locale. UTC is
 * used only to preserve the date-only value during localized formatting.
 */
export function monthLabel(month: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  }).format(new Date(`${month}-01T12:00:00Z`))
}

/** shiftMonth advances whole calendar months and returns their ISO identifier.
 * Callers supply a valid month and signed month offset. Starting on the first
 * day prevents shorter months from overflowing into an unintended following month.
 */
export function shiftMonth(month: string, offset: number): string {
  const date = new Date(`${month}-01T12:00:00Z`)
  date.setUTCMonth(date.getUTCMonth() + offset)
  return date.toISOString().slice(0, 7)
}

/** calendarDays fills Monday-first weeks around the supplied host-calendar month.
 * Records are matched by their complete date, preventing previous-month values
 * from appearing when a late response reaches a newly selected calendar.
 *
 * Leading and trailing positions complete whole Monday-first weeks. Month length
 * comes from calendar arithmetic, including leap years, and full ISO dates keep
 * records attached to their original month rather than a reused day number.
 */
export function calendarDays(month: string, records: CheckinRecord[]): CalendarDay[] {
  const first = new Date(`${month}-01T12:00:00Z`)
  const end = new Date(first)
  end.setUTCMonth(end.getUTCMonth() + 1, 0)
  // Monday-first positioning follows the host date-picker convention without
  // converting these date-only values into the browser's local calendar day.
  const offset = (first.getUTCDay() + 6) % 7
  const count = end.getUTCDate()
  const length = Math.ceil((offset + count) / 7) * 7
  const byDay = new Map(records.map((record) => [record.day, record]))
  return Array.from({ length }, (_, index) => {
    const number = index - offset + 1
    if (number < 1 || number > count) return { day: '', number: 0 }
    const day = `${month}-${String(number).padStart(2, '0')}`
    return { day, number, record: byDay.get(day) }
  })
}

/** weekdayLabels returns localized weekday names in the calendar's Monday-first order. */
export function weekdayLabels(locale: string): string[] {
  const formatter = new Intl.DateTimeFormat(locale, { weekday: 'short', timeZone: 'UTC' })
  return Array.from({ length: 7 }, (_, index) =>
    formatter.format(new Date(Date.UTC(2025, 10, 3 + index))),
  )
}

/** recordState distinguishes credited rewards, zero-value records, and unconfirmed intent.
 * Only a completed positive reward is labeled received. Zero-value completion
 * represents attendance without claiming the wallet balance was increased.
 */
export function recordState(record: CheckinRecord): 'received' | 'recorded' | 'pending' {
  if (!record.completed) return 'pending'
  return Number(record.amount) === 0 ? 'recorded' : 'received'
}

/** needsExactAmount identifies rewards the host's two-decimal component would round.
 * Trailing zeros do not require a different renderer. Any remaining fractional
 * digit beyond the second must stay visible in recorded financial amounts.
 */
export function needsExactAmount(value: string): boolean {
  return (value.split('.')[1] ?? '').replace(/0+$/, '').length > 2
}

/** exactAmountLabel localizes grouping without converting a decimal reward to float.
 * Callers provide the backend's validated nonnegative decimal string. Integer
 * grouping and the decimal separator follow locale, while fractional digits remain
 * the captured value after insignificant trailing zeros are removed.
 *
 * Separating those representations also retains small fractions beside large
 * integer totals. The result contains no currency symbol, leaving the rendering
 * component to identify the host's configured balance unit without another conversion.
 */
export function exactAmountLabel(value: string, locale: string): string {
  const [integer, fraction = ''] = value.split('.')
  const formatter = new Intl.NumberFormat(locale)
  const grouped = formatter.format(BigInt(integer))
  const digits = fraction.replace(/0+$/, '')
  // Browsers normally include a decimal token for this sample. A missing token
  // uses a period so the exact fractional digits remain distinguishable.
  const separator =
    formatter.formatToParts(1.1).find((part) => part.type === 'decimal')?.value ?? '.'
  return digits ? `${grouped}${separator}${digits}` : grouped
}

/** requestFailure maps actionable server failures while preserving a caller-specific fallback.
 * Known capability and configuration failures explain the required administrator action.
 * Unrecognized transport failures keep the calling view's own retry message.
 */
export function requestFailure(error: unknown, fallback: string): string {
  if (!(error instanceof HostRequestError)) return fallback
  switch (error.code) {
    case 'CHECKIN_PAUSED':
      return 'pausedFailure'
    case 'INVALID_REWARD':
      return 'rewardInvalid'
    case 'LIMIT_EXCEEDED':
      return 'limitFailure'
    case 'PERMISSION_NOT_GRANTED':
      return 'permissionFailure'
    case 'HISTORY_TOO_LARGE':
      return 'historyTooLarge'
    case 'INVALID_ARGUMENT':
      return 'invalidFilters'
    default:
      return fallback
  }
}
