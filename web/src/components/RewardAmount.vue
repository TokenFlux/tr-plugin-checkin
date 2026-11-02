<script setup lang="ts">
import { computed } from 'vue'
import { TrAmount, useHost } from '@tokenflux/tokenrouter-plugin-sdk'
import { exactAmountLabel, needsExactAmount } from '../presentation'

// RewardAmount.vue preserves the precision of recorded rewards while reusing the
// native host amount component for ordinary two-decimal values. The SDK exposes
// that component without a precision option, so a valid eight-decimal reward could
// otherwise appear as zero or a different amount. This component chooses the
// exact text representation only when the stored fraction needs additional digits.
//
// Exact values use localized integer grouping and the host's balance unit name.
// Fractional digits never pass through floating-point conversion, and trailing
// zeros do not change the selected renderer. These display decisions do not feed
// values into a claim, alter the wallet amount, or define another currency unit.

// Props contains a validated backend decimal in internal balance units; callers
// retain ownership of the value and use the same component for records and totals.
const props = defineProps<{
  value: string // Validated internal balance amount.
}>()
const host = useHost()
const precise = computed(() => needsExactAmount(props.value))
const label = computed(() => exactAmountLabel(props.value, host.locale.value))
</script>

<template>
  <span v-if="precise" class="checkin-precise-amount"
    >{{ label }}<small>{{ host.site.balanceUnitName }}</small></span
  >
  <TrAmount v-else :value="value" />
</template>
