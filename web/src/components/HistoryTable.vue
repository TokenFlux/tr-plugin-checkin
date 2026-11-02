<script setup lang="ts">
import { computed } from 'vue'
import { TrBadge, TrTable, useHost, useMessages } from '@tokenflux/tokenrouter-plugin-sdk'
import RewardAmount from './RewardAmount.vue'
import { messages } from '../messages'
import { recordState } from '../presentation'
import type { CheckinRecord } from '../types'

// HistoryTable.vue displays recorded dates, captured rewards, and confirmed
// completion using the host's responsive table and amount components. Personal
// views omit recipient identifiers, while administrators can identify recipients.
// Zero rewards are labeled as records rather than successful wallet credits.

// Table props select authorized rows and presentation only; callers own fetching,
// pagination, and filtering, and no user identity is inferred in this component.
const props = withDefaults(
  defineProps<{
    records: CheckinRecord[] // Authorized current-page daily records.
    admin?: boolean // Include the recipient identifier column.
    loading?: boolean // Current table request pending.
  }>(),
  { admin: false, loading: false },
)
const host = useHost()
const t = useMessages(messages)
const rows = computed(() =>
  props.records.map((record) => ({ ...record, state: recordState(record) })),
)
const columns = computed(() => [
  ...(props.admin ? [{ key: 'user_id', title: t('user') }] : []),
  { key: 'day', title: t('day') },
  { key: 'amount', title: t('amount') },
  { key: 'state', title: t('status') },
])

/** timestamp formats recorded Unix seconds in the host's named timezone.
 * Legacy records without a positive timestamp show no time instead of displaying
 * the Unix epoch. Formatting never changes the separate server-calendar date.
 */
function timestamp(value: unknown): string {
  if (typeof value !== 'number' || value <= 0) return ''
  // SDK timestamps use seconds; browser dates require milliseconds.
  return new Intl.DateTimeFormat(host.locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: host.site.timezone,
  }).format(new Date(value * 1000))
}
</script>

<template>
  <TrTable :columns="columns" :rows="rows" :loading="loading">
    <template #cell-day="{ value, row }"
      ><span class="checkin-record-date"
        >{{ value }}<small>{{ timestamp(row.at) }}</small></span
      ></template
    >
    <template #cell-amount="{ value }"
      ><span class="checkin-record-amount"><RewardAmount :value="String(value)" /></span
    ></template>
    <template #cell-state="{ row }"
      ><TrBadge
        :tone="
          row.state === 'pending' ? 'warning' : row.state === 'received' ? 'success' : 'neutral'
        "
        >{{ t(String(row.state)) }}</TrBadge
      ></template
    >
  </TrTable>
</template>
