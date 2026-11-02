<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  TrBadge,
  TrButton,
  TrCard,
  TrEmpty,
  TrInput,
  TrPagination,
  TrSelect,
  TrSkeleton,
  useHost,
  useMessages,
} from '@tokenflux/tokenrouter-plugin-sdk'
import HistoryTable from '../components/HistoryTable.vue'
import RewardAmount from '../components/RewardAmount.vue'
import { messages } from '../messages'
import { monthLabel, requestFailure } from '../presentation'
import type { HistoryPage, RewardSettings } from '../types'

// AdminPage.vue groups check-in record queries and reward-rule inspection in one
// administration entry. Filters are applied together, and table pagination uses
// their accepted values rather than unsaved inputs. Settings remain host-owned;
// contextual links open the existing configuration and capability-limit editors
// instead of introducing another writable source of reward configuration.
//
// Each resource has its own abort controller. A newer query cannot be replaced
// by an older response, and navigation retires both readers and the focus listener.
// Returning from the host editor reloads rule values without discarding record
// filters. Failed reads preserve the query and expose a retry with the same scope.

// Page state distinguishes editable filters from the accepted server query so
// changing a draft while paging cannot silently change the visible result scope.
const host = useHost()
const t = useMessages(messages)
const tab = ref('records')
const history = ref<HistoryPage | null>(null)
const settings = ref<RewardSettings | null>(null)
const month = ref('')
const userID = ref('')
const completion = ref('')
const applied = ref({ month: '', user_id: '', state: '' })
const page = ref(1)
const loading = ref(false)
const settingsLoading = ref(false)
const failure = ref('')
const filterFailure = ref('')
const settingsFailure = ref('')
let historyRequest: AbortController | undefined
let settingsRequest: AbortController | undefined
let disposed = false

// Host-owned URLs are derived from the authenticated plugin identity. These
// links preserve the host's authentication and configuration authorization flow.
const configurationURL = computed(
  () => `/admin/plugins/${encodeURIComponent(host.pluginId)}?tab=config`,
)
const limitsURL = computed(() => `/admin/plugins/${encodeURIComponent(host.pluginId)}?tab=limits`)
const monthOptions = computed(() => [
  { value: '', label: t('allMonths') },
  ...[...new Set([...(history.value?.months ?? []), month.value])]
    .filter(Boolean)
    .sort()
    .reverse()
    .map((value) => ({ value, label: monthLabel(value, host.locale.value) })),
])
const stateOptions = computed(() => [
  { value: '', label: t('allStates') },
  { value: 'completed', label: t('done') },
  { value: 'pending', label: t('pending') },
])

/** loadHistory reads one accepted query and suppresses obsolete server responses.
 * Request failure keeps filters and the requested page available for retry.
 * The accepted response supplies its resolved page after an out-of-range request.
 *
 * Each new reader retires its predecessor before dispatch. Only the active
 * controller can publish results or clear its loading indicator, so a late
 * completion cannot replace newer filters with a previously requested scope.
 */
async function loadHistory(): Promise<void> {
  historyRequest?.abort()
  const request = new AbortController()
  historyRequest = request
  loading.value = true
  failure.value = ''
  try {
    const result = await host.request<HistoryPage>('GET', '/admin/history', {
      query: { ...applied.value, page: String(page.value) },
      signal: request.signal,
    })
    if (request.signal.aborted || disposed) return
    history.value = result
    page.value = result.page
  } catch (error) {
    if (!request.signal.aborted && !disposed)
      failure.value = t(requestFailure(error, 'historyLoadFailed'))
  } finally {
    if (historyRequest === request) loading.value = false
  }
}

/** applyFilters validates the recipient identifier before accepting the whole query.
 * Empty recipient input means all users. Invalid input leaves the previous
 * accepted query untouched and exposes a local message without contacting storage.
 */
function applyFilters(): void {
  const id = userID.value.trim()
  if (id && !/^[1-9]\d*$/.test(id)) {
    filterFailure.value = t('invalidUserID')
    return
  }
  filterFailure.value = ''
  applied.value = { month: month.value, user_id: id, state: completion.value }
  page.value = 1
  void loadHistory()
}

/** selectPage loads another page using the last explicitly applied filter values.
 * Draft filter edits remain unapplied until the administrator submits them. Page
 * changes therefore cannot silently replace the scope of an existing result set.
 */
function selectPage(value: number): void {
  page.value = value
  void loadHistory()
}

/** loadSettings refreshes the nonsecret rule overview after host configuration edits.
 * It preserves existing rules on failure but hides them behind the recovery
 * message so stale values are not presented as a successful refreshed read.
 *
 * Switching views and browser focus can request the same resource independently.
 * Replacing the controller retires the previous owner, and teardown prevents any
 * remaining response from publishing rules into an abandoned administration view.
 */
async function loadSettings(): Promise<void> {
  settingsRequest?.abort()
  const request = new AbortController()
  settingsRequest = request
  settingsLoading.value = true
  settingsFailure.value = ''
  try {
    const result = await host.request<RewardSettings>('GET', '/admin/settings', {
      signal: request.signal,
    })
    if (!request.signal.aborted && !disposed) settings.value = result
  } catch (error) {
    if (!request.signal.aborted && !disposed)
      settingsFailure.value = t(requestFailure(error, 'settingsLoadFailed'))
  } finally {
    if (settingsRequest === request) settingsLoading.value = false
  }
}

/** selectTab opens the requested operational view and refreshes rule inspection on entry.
 * Record queries retain their accepted filters and current page. Settings always
 * reread host configuration so returning from its editor shows current values.
 */
function selectTab(value: string): void {
  tab.value = value
  if (value === 'settings') void loadSettings()
}

/** onFocus rereads rule changes when returning from the host's settings editor.
 * Only the mounted settings view performs this read. Record pagination remains
 * unchanged when the browser regains focus after inspecting another page.
 */
function onFocus(): void {
  if (tab.value === 'settings' && !disposed) void loadSettings()
}

onMounted(() => {
  void loadHistory()
  window.addEventListener('focus', onFocus)
})
onBeforeUnmount(() => {
  // Cancelling both independent readers and removing the listener prevents the
  // retired administration page from republishing rows or accepting refreshed rules.
  disposed = true
  historyRequest?.abort()
  settingsRequest?.abort()
  window.removeEventListener('focus', onFocus)
})
</script>

<template>
  <section class="checkin-page" :data-checkin-theme="host.theme.value">
    <header class="checkin-page-heading"
      ><div
        ><h1>{{ t('adminTitle') }}</h1
        ><p>{{ t('adminDescription') }}</p></div
      ></header
    >
    <nav class="checkin-tabs" :aria-label="t('adminTitle')"
      ><button
        type="button"
        :class="{ 'is-active': tab === 'records' }"
        :aria-pressed="tab === 'records'"
        @click="selectTab('records')"
        >{{ t('adminHistory') }}</button
      ><button
        type="button"
        :class="{ 'is-active': tab === 'settings' }"
        :aria-pressed="tab === 'settings'"
        @click="selectTab('settings')"
        >{{ t('rewardSettings') }}</button
      ></nav
    >
    <template v-if="tab === 'records'">
      <form class="checkin-filters" @submit.prevent="applyFilters"
        ><label
          ><span>{{ t('filterMonth') }}</span
          ><TrSelect
            v-model="month"
            :options="monthOptions"
            :aria-label="t('filterMonth')" /></label
        ><label
          ><span>{{ t('user') }}</span
          ><TrInput v-model="userID" :placeholder="t('userIDPlaceholder')" /></label
        ><label
          ><span>{{ t('status') }}</span
          ><TrSelect
            v-model="completion"
            :options="stateOptions"
            :aria-label="t('status')" /></label
        ><TrButton :disabled="loading" @click="applyFilters">{{ t('applyFilters') }}</TrButton
        ><TrButton variant="secondary" :loading="loading" @click="loadHistory">{{
          t('refresh')
        }}</TrButton></form
      >
      <p v-if="filterFailure" class="checkin-error checkin-filter-error" role="alert">{{
        filterFailure
      }}</p>
      <TrCard class="checkin-history">
        <div v-if="failure" class="checkin-feedback"
          ><p class="checkin-error" role="alert">{{ failure }}</p
          ><TrButton variant="secondary" @click="loadHistory">{{ t('retry') }}</TrButton></div
        >
        <HistoryTable
          v-else-if="history?.rows.length || loading"
          :records="loading ? [] : (history?.rows ?? [])"
          :loading="loading"
          admin
        />
        <div v-else class="checkin-empty"
          ><TrEmpty :title="t('adminEmpty')" :description="t('adminEmptyDescription')"
        /></div>
        <TrPagination
          v-if="history && !failure"
          :page="history.page"
          :page-size="history.page_size"
          :total="history.total"
          @update:page="selectPage"
        />
      </TrCard>
      <p class="checkin-muted checkin-small checkin-admin-note">{{ t('pendingAdminHint') }}</p>
    </template>
    <template v-else>
      <TrCard v-if="settingsLoading" class="checkin-panel"><TrSkeleton :rows="5" /></TrCard>
      <TrCard v-else-if="settingsFailure" class="checkin-panel checkin-feedback"
        ><p class="checkin-error" role="alert">{{ settingsFailure }}</p
        ><TrButton variant="secondary" @click="loadSettings">{{ t('retry') }}</TrButton></TrCard
      >
      <div v-else-if="settings" class="checkin-settings-layout">
        <TrCard class="checkin-settings-card">
          <header class="checkin-card-heading"
            ><h2>{{ t('rewardSettings') }}</h2
            ><div class="checkin-settings-actions">
              <a class="checkin-text-button" :href="limitsURL">{{ t('editLimits') }}</a>
              <a class="checkin-link-button" :href="configurationURL">{{ t('editSettings') }}</a>
            </div></header
          >
          <dl class="checkin-settings-list"
            ><div
              ><dt
                ><strong>{{ t('availability') }}</strong
                ><p>{{ t('pauseRule') }}</p></dt
              ><dd
                ><TrBadge :tone="settings.paused ? 'warning' : 'success'">{{
                  t(settings.paused ? 'paused' : 'active')
                }}</TrBadge></dd
              ></div
            ><div
              ><dt
                ><strong>{{ t('dailyReward') }}</strong
                ><p>{{ t('rewardRule') }}</p></dt
              ><dd
                ><RewardAmount :value="settings.reward" /><small>{{
                  host.site.balanceUnitName
                }}</small></dd
              ></div
            ><div
              ><dt
                ><strong>{{ t('resetPeriod') }}</strong
                ><p>{{ t('timezoneRule') }}</p></dt
              ><dd>{{ t('resetAt', { timezone: settings.timezone }) }}</dd></div
            ><div class="checkin-setting-notice"
              ><dt
                ><strong>{{ t('noticeTitle') }}</strong
                ><p>{{ t('noticeRule') }}</p></dt
              ><dd>{{ settings.notice || t('noNotice') }}</dd></div
            ></dl
          >
        </TrCard>
        <aside class="checkin-settings-aside"
          ><TrCard class="checkin-panel"
            ><h2>{{ t('rewardPreview') }}</h2
            ><div class="checkin-preview-reward"
              ><span class="checkin-muted checkin-small">{{ t('dailyReward') }}</span
              ><strong><RewardAmount :value="settings.reward" /></strong
              ><p class="checkin-muted checkin-small">{{
                t(Number(settings.reward) === 0 ? 'zeroRewardRule' : 'claimHint')
              }}</p></div
            ><p v-if="settings.paused" class="checkin-muted">{{ t('pausedHint') }}</p></TrCard
          ></aside
        >
      </div>
    </template>
  </section>
</template>
