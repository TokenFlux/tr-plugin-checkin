// types.ts describes the plugin's daily status and paginated history responses.
// Amounts remain decimal strings in internal balance units until TrAmount formats
// them. Calendar dates use the server's timezone, while timestamps use Unix
// seconds and are converted only for display.

/** CheckinRecord preserves the original reward intent for one recipient and day. */
export interface CheckinRecord {
  user_id: number // Verified recipient account identifier.
  day: string // Host-calendar ISO date string.
  amount: string // Captured internal balance units.
  completed: boolean // Confirmed record completion flag.
  at: number // Initial claim Unix seconds.
}

/** CheckinStatus carries current-day availability, captured amounts, and reset timing. */
export interface CheckinStatus {
  checked_in: boolean // Confirmed current-day completion flag.
  state: 'available' | 'pending' | 'completed' | 'paused' // Current claim state.
  reward: string // Existing intent or configured amount.
  day: string // Current host-calendar ISO date.
  timezone: string // Host IANA timezone name.
  server_time: number // Response generation Unix seconds.
  next_reset_at: number // Next host midnight Unix seconds.
  paused: boolean // New daily claims paused.
  notice: string // Administrator-authored check-in instructions.
}

/** RewardSummary counts completed records and their exact stored reward total. */
export interface RewardSummary {
  count: number // Confirmed daily record count.
  amount: string // Captured rewards in balance units.
}

/** HistoryPage carries a table page and an independently complete selected-month calendar. */
export interface HistoryPage {
  rows: CheckinRecord[] // Requested table page records.
  calendar: CheckinRecord[] // Complete personal selected-month records.
  months: string[] // Recorded months, newest first.
  month: string // Selected personal calendar month.
  total: number // Matching table record count.
  page: number // Resolved one-based page number.
  page_size: number // Maximum table page size.
  summary: RewardSummary // All completed authorized records.
  month_summary: RewardSummary // Completed selected-month record totals.
}

/** RewardSettings contains the nonsecret rules shown in the administration overview. */
export interface RewardSettings {
  reward: string // Configured daily balance reward.
  paused: boolean // New daily claims paused.
  timezone: string // Host IANA timezone name.
  notice: string // Administrator-authored check-in instructions.
}
