<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  TrButton,
  TrCard,
  TrIcon,
  TrSkeleton,
  useHost,
  useMessages,
} from '@tokenflux/tokenrouter-plugin-sdk'
import { calendarDays, monthLabel, recordState, shiftMonth, weekdayLabels } from '../presentation'
import RewardAmount from './RewardAmount.vue'
import { messages } from '../messages'
import type { CheckinRecord } from '../types'

// CheckinCalendar.vue displays complete host-calendar months with explicit signed,
// pending, missed, and future states. Month changes belong to the parent request
// owner; this component only selects dates and emits navigation. Date selection
// never creates a claim, and future days remain unavailable for inspection.

// Props keeps the server's today and records together; loading or failure prevents
// stale rows from being presented as a successful empty calendar for a new month.
const props = defineProps<{
  month: string // Currently selected ISO month.
  today: string // Current server-calendar ISO date.
  records: CheckinRecord[] // Complete selected-month daily records.
  loading: boolean // Month read is in progress.
  failure: string // Localized read failure or empty.
}>()
const emit = defineEmits<{
  month: [value: string] // Requested calendar month identifier.
  retry: [] // Retry the current month request.
}>()

// Local selection belongs to the visible calendar month. Host locale controls
// labels while all date arithmetic stays in the server-provided calendar scope.
const host = useHost()
const t = useMessages(messages)
const selected = ref('')
const days = computed(() => calendarDays(props.month, props.records))
const weekdays = computed(() => weekdayLabels(host.locale.value))
const title = computed(() => monthLabel(props.month, host.locale.value))
const selectedRecord = computed(() => props.records.find((record) => record.day === selected.value))
const currentMonth = computed(() => props.today.slice(0, 7))

/** dayDescription explains a calendar date without relying on its state color.
 * A stored record takes precedence over date-based absence. Future dates stay
 * distinct from missed days, and zero rewards retain their recorded-only label.
 */
function dayDescription(day: string, record?: CheckinRecord): string {
  if (record) return t(recordState(record))
  return t(day === props.today ? 'todayAvailable' : day < props.today ? 'missed' : 'future')
}

watch(
  () => props.month,
  () => {
    selected.value = ''
  },
)
</script>

<template>
  <TrCard class="checkin-calendar">
    <header class="checkin-card-heading checkin-calendar-heading">
      <h2>{{ t('calendar') }}</h2>
      <div class="checkin-month-controls">
        <TrButton
          variant="ghost"
          size="sm"
          :disabled="loading"
          :aria-label="t('previousMonth')"
          @click="emit('month', shiftMonth(month, -1))"
        >
          <svg
            class="checkin-chevron"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.75"
            aria-hidden="true"
          >
            <path d="m15 18-6-6 6-6" />
          </svg>
        </TrButton>
        <span class="checkin-month-title">{{ title }}</span>
        <TrButton
          variant="ghost"
          size="sm"
          :disabled="loading || month >= currentMonth"
          :aria-label="t('nextMonth')"
          @click="emit('month', shiftMonth(month, 1))"
        >
          <svg
            class="checkin-chevron"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.75"
            aria-hidden="true"
          >
            <path d="m9 18 6-6-6-6" />
          </svg>
        </TrButton>
      </div>
    </header>
    <div v-if="loading" class="checkin-calendar-loading" role="status"
      ><TrSkeleton :rows="5" /><span class="checkin-sr-only">{{ t('loadingHistory') }}</span></div
    >
    <div v-else-if="failure" class="checkin-feedback"
      ><p class="checkin-error" role="alert">{{ failure }}</p
      ><TrButton variant="secondary" @click="emit('retry')">{{ t('retry') }}</TrButton></div
    >
    <div v-else class="checkin-calendar-body">
      <div class="checkin-weekdays"
        ><span v-for="label in weekdays" :key="label">{{ label }}</span></div
      >
      <div class="checkin-days">
        <template v-for="(cell, index) in days" :key="cell.day || index">
          <span v-if="!cell.day" aria-hidden="true"></span>
          <button
            v-else
            type="button"
            class="checkin-day"
            :class="{
              'is-today': cell.day === today,
              'is-completed': cell.record?.completed,
              'is-pending': cell.record && !cell.record.completed,
              'is-future': cell.day > today,
              'is-selected': selected === cell.day,
            }"
            :disabled="cell.day > today"
            :aria-label="`${cell.day}, ${dayDescription(cell.day, cell.record)}`"
            :aria-pressed="selected === cell.day"
            :aria-current="cell.day === today ? 'date' : undefined"
            @click="selected = cell.day"
          >
            <span>{{ cell.number }}</span
            ><TrIcon v-if="cell.record?.completed" name="check" /><span
              v-else-if="cell.record"
              class="checkin-pending-mark"
              aria-hidden="true"
              >!</span
            >
          </button>
        </template>
      </div>
      <p v-if="selected" class="checkin-day-detail" role="status"
        ><span>{{ selected }}</span
        ><span>{{ dayDescription(selected, selectedRecord) }}</span
        ><RewardAmount v-if="selectedRecord" :value="selectedRecord.amount"
      /></p>
    </div>
    <footer class="checkin-calendar-footer"
      ><span class="checkin-legend"><TrIcon name="check" />{{ t('signed') }}</span
      ><span class="checkin-legend"><i class="checkin-today-mark"></i>{{ t('today') }}</span
      ><span class="checkin-legend">! {{ t('pending') }}</span
      ><button
        v-if="month !== currentMonth"
        class="checkin-text-button"
        type="button"
        @click="emit('month', currentMonth)"
        >{{ t('currentMonth') }}</button
      ></footer
    >
  </TrCard>
</template>
