<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
defineProps<{
  modelValue: boolean
  balance: string
  walletOnly: boolean
  deduction: string
  online: string
  insufficient: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>
<template>
  <div class="wallet-option">
    <div class="wallet-option-row">
      <span class="wallet-caption">{{ t('payment.walletBalanceLabel') }} <strong>{{ balance }}</strong></span>
      <label class="wallet-choice">
        <input type="checkbox" :checked="modelValue" :disabled="walletOnly"
          @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)" />
        <span>{{ t('payment.useBalance') }}</span>
      </label>
    </div>
    <div v-if="modelValue || walletOnly" class="wallet-details">
      <span v-if="walletOnly">{{ t('payment.walletOnlyHint') }}</span>
      <span v-if="modelValue">{{ t('payment.walletDeductLabel') }}：{{ deduction }}</span>
      <span v-if="modelValue && !walletOnly">{{ t('payment.onlinePayLabel') }}：{{ online }}</span>
      <span v-if="walletOnly && insufficient" class="wallet-warning">{{ t('payment.walletInsufficientHint') }}</span>
    </div>
  </div>
</template>
<style scoped>
.wallet-option { margin-bottom: 10px; padding: 0 10px; border: 1px solid var(--ui-border); border-radius: 10px; background: var(--ui-bg-elevated); color: var(--ui-text-primary); }
.wallet-option-row { min-height: 40px; display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.wallet-caption { color: var(--ui-text-secondary); font-size: 11px; line-height: 1.5; }
.wallet-caption strong { display: inline-block; margin-left: 5px; color: var(--ui-text-primary); font-size: 13px; font-weight: 700; font-variant-numeric: tabular-nums; }
.wallet-choice { min-height: 40px; display: inline-flex; align-items: center; gap: 5px; cursor: pointer; font-size: 11px; color: var(--ui-text-secondary); white-space: nowrap; }
.wallet-choice input { width: 13px; height: 13px; accent-color: var(--ui-accent); }
.wallet-choice:has(input:focus-visible) { outline: 2px solid var(--ui-accent); outline-offset: 2px; border-radius: 4px; }
.wallet-details { display: flex; flex-wrap: wrap; gap: 3px 10px; padding-bottom: 8px; color: var(--ui-text-secondary); font-size: 11px; line-height: 1.5; }
.wallet-warning { color: var(--ui-warning); }
@media (max-width: 360px) { .wallet-option-row { flex-wrap: wrap; gap: 0; } }
</style>
