<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  HostRequestError,
  TrButton,
  TrCard,
  TrEmpty,
  TrIcon,
  TrPagination,
  TrSelect,
  TrSkeleton,
  useHost,
  useMessages,
} from '@tokenflux/tokenrouter-plugin-sdk'
import CheckinCalendar from '../components/CheckinCalendar.vue'
import HistoryTable from '../components/HistoryTable.vue'
import RewardAmount from '../components/RewardAmount.vue'
import { messages } from '../messages'
import { monthLabel, requestFailure } from '../presentation'
import type { CheckinStatus, HistoryPage } from '../types'

// HomePage.vue implements the approved daily-action layout with personal history
// and a host-calendar month view. The left column owns today's reward, totals,
// and an optional site notice; the right column owns calendar inspection and rewards.
// Native SDK components supply controls, tables, amounts, and feedback so this
// page follows the gateway's theme and responsive behavior.
//
// Status and history reads have separate cancellation owners. Changing a month
// cancels its predecessor, while confirmed credits remain visible if later reads
// or the account refresh fail. Server midnight schedules a read-only refresh;
// browser timezone and local clock do not create a new financial identity.

// The mounted page owns all pending reads and the claim request. Completion
// flags remain local to this mount and are reconciled with authoritative responses.
const host = useHost()
const t = useMessages(messages)
const status = ref<CheckinStatus | null>(null)
const history = ref<HistoryPage | null>(null)
const selectedMonth = ref('')
const historyScope = ref('all')
const page = ref(1)
const expanded = ref(false)
const statusLoading = ref(true)
const historyLoading = ref(false)
const claiming = ref(false)
const refreshing = ref(false)
const statusFailure = ref('')
const historyFailure = ref('')
const claimFailure = ref('')
const refreshFailed = ref(false)
const unknownReward = ref(false)
const lifetime = new AbortController()
let statusRequest: AbortController | undefined
let historyRequest: AbortController | undefined
let resetTimer: ReturnType<typeof setTimeout> | undefined

// Presentation derives only from accepted records. Month labels follow the host
// language, while the server's date determines current-month navigation and scope.
const visibleRows = computed(() =>
  expanded.value ? (history.value?.rows ?? []) : (history.value?.rows.slice(0, 3) ?? []),
)
const monthCount = computed(() =>
  history.value?.month === selectedMonth.value ? history.value.month_summary.count : null,
)
const monthOptions = computed(() => {
  const months = new Set([
    ...(history.value?.months ?? []),
    selectedMonth.value,
    status.value?.day.slice(0, 7) ?? '',
  ])
  return [
    { value: 'all', label: t('allMonths') },
    ...[...months]
      .filter(Boolean)
      .sort()
      .reverse()
      .map((month) => ({
        value: month,
        label: monthLabel(month, host.locale.value),
      })),
  ]
})
const selectedFilter = computed(() => (historyScope.value === 'all' ? 'all' : selectedMonth.value))
const monthRecords = computed(() =>
  history.value?.month === selectedMonth.value ? history.value.calendar : [],
)
const claimLabel = computed(() =>
  t(claiming.value ? 'claiming' : status.value?.state === 'pending' ? 'retryClaim' : 'claim'),
)

/** loadStatus accepts only the latest mounted read and schedules server midnight.
 * It preserves an already confirmed claim on transport failure. A new server day
 * follows the current calendar automatically only when the user was viewing the
 * previous current month, preserving deliberately selected historical months.
 *
 * Cancellation prevents an older read from replacing newer claim feedback. The
 * timer uses the difference between server timestamps, avoiding browser clock
 * drift; waking a suspended tab also requests an authoritative refresh.
 */
async function loadStatus(): Promise<boolean> {
  statusRequest?.abort()
  const request = new AbortController()
  statusRequest = request
  statusLoading.value = true
  statusFailure.value = ''
  try {
    const result = await host.request<CheckinStatus>('GET', '/status', {
      signal: request.signal,
    })
    if (request.signal.aborted || lifetime.signal.aborted) return false
    const previousMonth = status.value?.day.slice(0, 7)
    if (!selectedMonth.value || selectedMonth.value === previousMonth)
      selectedMonth.value = result.day.slice(0, 7)
    status.value = result
    unknownReward.value = false
    clearTimeout(resetTimer)
    // A server-relative delay survives a misconfigured client clock. The extra
    // second avoids requesting the previous day at a rounded timestamp boundary.
    const delay = Math.max(1, result.next_reset_at - result.server_time + 1) * 1000
    resetTimer = setTimeout(() => {
      void refresh()
    }, delay)
    return true
  } catch (error) {
    if (!request.signal.aborted && !lifetime.signal.aborted)
      statusFailure.value = t(requestFailure(error, 'loadFailed'))
    return false
  } finally {
    if (statusRequest === request) statusLoading.value = false
  }
}

/** loadHistory replaces one table and calendar read using the selected month.
 * Cancelled or superseded responses cannot change rows, totals, or pagination.
 * Failed reads retain filter inputs and offer retry instead of displaying an
 * empty calendar that could incorrectly imply the user never signed in.
 * The same accepted response supplies rows, calendar entries, and totals, keeping
 * those views in one date scope while independent status refreshes continue.
 */
async function loadHistory(): Promise<void> {
  if (!selectedMonth.value || lifetime.signal.aborted) return
  historyRequest?.abort()
  const request = new AbortController()
  historyRequest = request
  historyLoading.value = true
  historyFailure.value = ''
  try {
    const result = await host.request<HistoryPage>('GET', '/history', {
      query: {
        month: selectedMonth.value,
        scope: historyScope.value,
        page: String(page.value),
      },
      signal: request.signal,
    })
    if (request.signal.aborted || lifetime.signal.aborted) return
    history.value = result
    page.value = result.page
  } catch (error) {
    if (!request.signal.aborted && !lifetime.signal.aborted)
      historyFailure.value = t(requestFailure(error, 'historyLoadFailed'))
  } finally {
    if (historyRequest === request) historyLoading.value = false
  }
}

/** refresh reconciles the host day before requesting its associated personal calendar.
 * Claims and retired pages skip this read sequence. A failed status read keeps
 * the existing calendar and selected filters available for a later retry.
 */
async function refresh(): Promise<void> {
  if (lifetime.signal.aborted || claiming.value) return
  if (await loadStatus()) await loadHistory()
}

/** selectMonth synchronizes calendar navigation and table filtering without losing the claim.
 * Calendar controls supply an ISO month. Applying it resets pagination to the
 * first page and preserves the independently owned daily reward state.
 */
function selectMonth(month: string): void {
  selectedMonth.value = month
  historyScope.value = 'month'
  page.value = 1
  void loadHistory()
}

/** selectFilter applies an all-month or specific-month history scope and resets pagination.
 * The all-month choice preserves the visible calendar month. A specific month
 * updates both views so their displayed date scopes remain consistent.
 */
function selectFilter(value: string | number): void {
  if (String(value) === 'all') {
    historyScope.value = 'all'
    page.value = 1
    void loadHistory()
  } else {
    selectMonth(String(value))
  }
}

/** selectPage loads another server page without changing the selected calendar month.
 * The host pagination control supplies a positive page number. The accepted
 * response can resolve an out-of-range request to the last available page.
 */
function selectPage(value: number): void {
  page.value = value
  void loadHistory()
}

/** refreshBalance retries the host account display without creating another reward.
 * Completion stays visible during this read. Page teardown suppresses feedback,
 * and a failed read keeps a separate retry action available for recovery.
 */
async function refreshBalance(): Promise<void> {
  if (refreshing.value || lifetime.signal.aborted) return
  refreshing.value = true
  try {
    await host.refreshAccount()
    if (!lifetime.signal.aborted) refreshFailed.value = false
  } catch {
    if (!lifetime.signal.aborted) refreshFailed.value = true
  } finally {
    refreshing.value = false
  }
}

/** claim records the returned financial result before refreshing presentation data.
 * Pending claims reuse their server-owned identity. Duplicate responses suppress
 * another claim immediately, then read the captured amount instead of presenting
 * the currently configured reward as proof of a particular wallet credit.
 *
 * Failed status, history, or balance refreshes do not undo confirmed completion.
 * The backend's day in a successful response identifies the credited claim even
 * if midnight occurs while it runs. Read-only reconciliation then determines
 * whether another host-calendar day has become available.
 */
async function claim(): Promise<void> {
  if (!status.value || claiming.value || status.value.checked_in || status.value.state === 'paused')
    return
  statusRequest?.abort()
  historyRequest?.abort()
  claiming.value = true
  claimFailure.value = ''
  let completed = false
  try {
    const result = await host.request<{ reward: string; day: string }>('POST', '/checkin', {
      signal: lifetime.signal,
    })
    if (lifetime.signal.aborted) return
    status.value = {
      ...status.value,
      checked_in: true,
      state: 'completed',
      reward: result.reward,
      day: result.day,
    }
    unknownReward.value = false
    completed = true
    host.toast.success(t(Number(result.reward) === 0 ? 'recordedSuccess' : 'claimSuccess'))
  } catch (error) {
    if (lifetime.signal.aborted) return
    if (error instanceof HostRequestError && error.code === 'ALREADY_CHECKED_IN') {
      status.value = { ...status.value, checked_in: true, state: 'completed' }
      unknownReward.value = true
      completed = true
    } else {
      claimFailure.value = t(requestFailure(error, 'claimFailed'))
    }
  } finally {
    claiming.value = false
  }
  if (lifetime.signal.aborted) return
  if (completed) await refreshBalance()
  await refresh()
}

/** onVisible refreshes stale server-day information after the browser resumes the page.
 * Hidden documents do not start reads. The owning component removes both wake
 * listeners during teardown, and refresh coordinates them with any active claim.
 */
function onVisible(): void {
  if (document.visibilityState === 'visible') void refresh()
}

onMounted(() => {
  void refresh()
  document.addEventListener('visibilitychange', onVisible)
  window.addEventListener('focus', onVisible)
})
onBeforeUnmount(() => {
  // All page-owned reads, the midnight timer, and wake listeners retire together;
  // backend claims remain protected by their existing durable wallet replay key.
  lifetime.abort()
  statusRequest?.abort()
  historyRequest?.abort()
  clearTimeout(resetTimer)
  document.removeEventListener('visibilitychange', onVisible)
  window.removeEventListener('focus', onVisible)
})
</script>

<template>
  <section class="checkin-page" :data-checkin-theme="host.theme.value">
    <header class="checkin-page-heading">
      <h1>{{ t('title') }}</h1>
      <p class="checkin-timezone">
        {{ t('resetAt', { timezone: status?.timezone ?? host.site.timezone }) }}
      </p>
    </header>
    <div v-if="!status" class="checkin-initial">
      <TrCard v-if="statusLoading" class="checkin-panel" role="status"
        ><TrSkeleton :rows="5" /><span class="checkin-sr-only">{{
          t('loadingStatus')
        }}</span></TrCard
      >
      <TrCard v-else class="checkin-panel checkin-feedback"
        ><p class="checkin-error" role="alert">{{ statusFailure }}</p>
        <TrButton variant="secondary" @click="refresh">{{ t('retry') }}</TrButton></TrCard
      >
    </div>
    <div v-else class="checkin-layout">
      <aside class="checkin-aside">
        <TrCard class="checkin-panel checkin-claim-card" :aria-busy="claiming">
          <div class="checkin-inline-heading">
            <h2>{{ t('todayCheckin') }}</h2>
            <span class="checkin-muted checkin-small">{{ status.day }}</span>
          </div>
          <div class="checkin-reward-value">
            <span v-if="unknownReward" class="checkin-small checkin-muted">{{
              t('amountAwaitingRefresh')
            }}</span
            ><RewardAmount v-else :value="status.reward" />
          </div>
          <p v-if="status.state === 'pending'" class="checkin-claim-hint">
            {{ t('pendingHint') }}
          </p>
          <div v-if="status.checked_in" class="checkin-complete" role="status">
            <TrIcon name="check" /><span>{{
              t(Number(status.reward) === 0 ? 'recordedSuccess' : 'completed')
            }}</span>
          </div>
          <div v-else-if="status.state === 'paused'" class="checkin-paused" role="status">
            <strong>{{ t('paused') }}</strong>
            <p>{{ t('pausedHint') }}</p>
          </div>
          <TrButton
            v-else
            class="checkin-claim-button"
            :loading="claiming"
            :disabled="claiming"
            @click="claim"
            ><TrIcon v-if="!claiming" :name="status.state === 'pending' ? 'refresh' : 'check'" />{{
              claimLabel
            }}</TrButton
          >
          <p
            v-if="status.state !== 'pending' && status.state !== 'paused'"
            class="checkin-claim-hint"
          >
            {{ t(Number(status.reward) === 0 ? 'zeroRewardRule' : 'claimHint') }}
          </p>
          <p v-if="claimFailure" class="checkin-error" role="alert">
            {{ claimFailure }}
          </p>
          <div v-if="statusFailure" class="checkin-feedback-inline">
            <p class="checkin-error" role="alert">{{ statusFailure }}</p>
            <TrButton variant="secondary" size="sm" @click="refresh">{{
              t('refreshStatus')
            }}</TrButton>
          </div>
          <div v-if="refreshFailed" class="checkin-feedback-inline">
            <p class="checkin-muted checkin-small" role="alert">
              {{ t('refreshFailed') }}
            </p>
            <TrButton variant="secondary" size="sm" :loading="refreshing" @click="refreshBalance">{{
              t('refreshBalance')
            }}</TrButton>
          </div>
        </TrCard>
        <TrCard class="checkin-summary" :aria-busy="historyLoading"
          ><dl>
            <div>
              <dt>
                {{
                  t(selectedMonth === status.day.slice(0, 7) ? 'monthCount' : 'selectedMonthCount')
                }}
              </dt>
              <dd>
                {{ monthCount ?? '—' }}<small v-if="monthCount !== null">{{ t('days') }}</small>
              </dd>
            </div>
            <div>
              <dt>{{ t('totalCount') }}</dt>
              <dd>
                {{ history?.summary.count ?? '—' }}<small v-if="history">{{ t('days') }}</small>
              </dd>
            </div>
            <div>
              <dt>{{ t('totalReward') }}</dt>
              <dd>
                <RewardAmount v-if="history" :value="history.summary.amount" /><span v-else>—</span>
              </dd>
            </div>
          </dl></TrCard
        >
        <TrCard v-if="status.notice" class="checkin-panel checkin-notice"
          ><h2>{{ t('noticeTitle') }}</h2> <p>{{ status.notice }}</p></TrCard
        >
      </aside>
      <div class="checkin-main">
        <CheckinCalendar
          :month="selectedMonth"
          :today="status.day"
          :records="monthRecords"
          :loading="historyLoading"
          :failure="historyFailure"
          @month="selectMonth"
          @retry="loadHistory"
        />
        <TrCard class="checkin-history">
          <header class="checkin-card-heading">
            <h2>{{ t(expanded ? 'history' : 'recentHistory') }}</h2>
            <button
              v-if="!expanded"
              type="button"
              class="checkin-text-button"
              @click="expanded = true"
            >
              {{ t('allRecords') }}</button
            ><TrSelect
              v-else
              :model-value="selectedFilter"
              :options="monthOptions"
              :aria-label="t('filterMonth')"
              @update:model-value="selectFilter"
            />
          </header>
          <div v-if="historyFailure" class="checkin-feedback">
            <p class="checkin-error" role="alert">{{ historyFailure }}</p>
            <TrButton variant="secondary" @click="loadHistory">{{ t('retry') }}</TrButton>
          </div>
          <HistoryTable
            v-else-if="visibleRows.length || historyLoading"
            :records="historyLoading ? [] : visibleRows"
            :loading="historyLoading"
          />
          <div v-else class="checkin-empty">
            <TrEmpty :title="t('empty')" />
          </div>
          <TrPagination
            v-if="history && !historyFailure && expanded"
            :page="history.page"
            :page-size="history.page_size"
            :total="history.total"
            @update:page="selectPage"
          />
        </TrCard>
      </div>
    </div>
  </section>
</template>
