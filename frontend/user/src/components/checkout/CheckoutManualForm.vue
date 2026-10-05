<template>
  <div
    v-if="manualFormProducts.length"
    class="delivery-form"
  >
    <div class="delivery-heading">
      <ClipboardPenLine :size="22" aria-hidden="true" />
      <div>
        <h2>{{ t('checkout.manualFormTitle') }}</h2>
        <p>{{ t('checkout.manualFormTip') }}</p>
      </div>
    </div>
    <div class="space-y-5">
      <div
        v-for="manualItem in manualFormProducts"
        :key="manualItem.itemKey"
        class="delivery-product"
      >
        <h3>{{ manualItemTitle(manualItem) }}</h3>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div v-for="field in manualItem.fields" :key="`${manualItem.itemKey}-${field.key}`" class="delivery-field"
            :class="{ 'md:col-span-2': field.type === 'textarea', 'delivery-field-invalid': submitAttempted && manualFieldError(manualItem.itemKey, field.key) }">
            <label :for="fieldID(manualItem.itemKey, field.key)">
              {{ getManualFieldLabel(field) }}
              <span v-if="field.required" class="ml-1 text-destructive">*</span>
            </label>

            <Textarea
              v-if="field.type === 'textarea'"
              :id="fieldID(manualItem.itemKey, field.key)"
              class="delivery-input delivery-textarea"
              :aria-required="field.required"
              :aria-invalid="Boolean(submitAttempted && manualFieldError(manualItem.itemKey, field.key))"
              :aria-describedby="fieldID(manualItem.itemKey, field.key) + '-error'"
              :model-value="getFieldValue(manualItem.itemKey, field.key)"
              @update:model-value="updateFieldValue(manualItem.itemKey, field.key, $event)"
              rows="3"
              :placeholder="getManualFieldPlaceholder(field)"
            />

            <select
              v-else-if="field.type === 'select'"
              :id="fieldID(manualItem.itemKey, field.key)"
              :aria-required="field.required"
              :aria-invalid="Boolean(submitAttempted && manualFieldError(manualItem.itemKey, field.key))"
              :aria-describedby="fieldID(manualItem.itemKey, field.key) + '-error'"
              :value="getFieldValue(manualItem.itemKey, field.key)"
              @change="updateFieldValue(manualItem.itemKey, field.key, ($event.target as HTMLSelectElement).value)"
              :class="selectClass"
            >
              <option value="">{{ t('checkout.manualFormSelectPlaceholder') }}</option>
              <option v-for="option in field.options" :key="option" :value="option">{{ option }}</option>
            </select>

            <div v-else-if="field.type === 'radio'" :id="fieldID(manualItem.itemKey, field.key)" class="delivery-options" role="radiogroup" :aria-label="getManualFieldLabel(field)">
              <label v-for="option in field.options" :key="option" class="flex items-center gap-2 text-sm text-muted-foreground">
                <input
                  :checked="getFieldValue(manualItem.itemKey, field.key) === option"
                  @change="updateFieldValue(manualItem.itemKey, field.key, option)"
                  type="radio"
                  :name="`manual-radio-${manualItem.itemKey}-${field.key}`"
                  :value="option"
                  class="h-4 w-4 accent-primary"
                />
                <span>{{ option }}</span>
              </label>
            </div>

            <div v-else-if="field.type === 'checkbox'" :id="fieldID(manualItem.itemKey, field.key)" class="delivery-options" role="group" :aria-label="getManualFieldLabel(field)">
              <label v-for="option in field.options" :key="option" class="flex items-center gap-2 text-sm text-muted-foreground">
                <input
                  :checked="isCheckboxChecked(manualItem.itemKey, field.key, option)"
                  @change="toggleCheckboxValue(manualItem.itemKey, field.key, option, ($event.target as HTMLInputElement).checked)"
                  type="checkbox"
                  :value="option"
                  class="h-4 w-4 accent-primary"
                />
                <span>{{ option }}</span>
              </label>
            </div>

            <Input
              v-else
              :id="fieldID(manualItem.itemKey, field.key)"
              class="delivery-input"
              :aria-required="field.required"
              :aria-invalid="Boolean(submitAttempted && manualFieldError(manualItem.itemKey, field.key))"
              :aria-describedby="fieldID(manualItem.itemKey, field.key) + '-error'"
              :model-value="getFieldValue(manualItem.itemKey, field.key)"
              @update:model-value="updateFieldValue(manualItem.itemKey, field.key, $event)"
              :type="field.type === 'number' ? 'number' : field.type === 'email' ? 'email' : field.type === 'phone' ? 'tel' : 'text'"
              :placeholder="getManualFieldPlaceholder(field)"
            />

            <p
              :id="fieldID(manualItem.itemKey, field.key) + '-error'"
              class="delivery-error" aria-live="polite"
            >
              {{ submitAttempted ? manualFieldError(manualItem.itemKey, field.key) : '' }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useLocalized } from '../../composables/useProduct'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { ClipboardPenLine } from 'lucide-vue-next'

const selectClass =
  'delivery-input'
const fieldID = (itemKey: string, fieldKey: string) => 'delivery-' + encodeURIComponent(itemKey + '-' + fieldKey)

interface ManualFormField {
  key: string
  type: string
  required: boolean
  label?: Record<string, string>
  placeholder?: Record<string, string>
  regex?: string
  min?: number
  max?: number
  max_len?: number
  options: string[]
}

interface ManualFormProduct {
  itemKey: string
  productId: number
  title: any
  fields: ManualFormField[]
  skuCount: number
}

const props = defineProps<{
  manualFormProducts: ManualFormProduct[]
  modelValue: Record<string, Record<string, any>>
  submitAttempted: boolean
  getManualFieldLabel: (field: ManualFormField) => string
  getManualFieldPlaceholder: (field: ManualFormField) => string
  manualFieldError: (itemKey: string, fieldKey: string) => string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: Record<string, Record<string, any>>): void
}>()

const { t } = useI18n()
const { getLocalizedText } = useLocalized()

const manualItemTitle = (manualItem: ManualFormProduct) => {
  const productTitle = getLocalizedText(manualItem.title)
  if (manualItem.skuCount <= 1) return productTitle
  return `${productTitle} (${t('checkout.manualFormAppliesToSkuCount', { count: manualItem.skuCount })})`
}

const getFieldValue = (itemKey: string, fieldKey: string) => {
  return props.modelValue[itemKey]?.[fieldKey] ?? ''
}

const updateFieldValue = (itemKey: string, fieldKey: string, value: any) => {
  const updated = { ...props.modelValue }
  if (!updated[itemKey]) {
    updated[itemKey] = {}
  }
  updated[itemKey] = { ...updated[itemKey], [fieldKey]: value }
  emit('update:modelValue', updated)
}

const isCheckboxChecked = (itemKey: string, fieldKey: string, option: string) => {
  const value = props.modelValue[itemKey]?.[fieldKey]
  return Array.isArray(value) && value.includes(option)
}

const toggleCheckboxValue = (itemKey: string, fieldKey: string, option: string, checked: boolean) => {
  const current = props.modelValue[itemKey]?.[fieldKey]
  const list = Array.isArray(current) ? [...current] : []
  if (checked) {
    if (!list.includes(option)) list.push(option)
  } else {
    const idx = list.indexOf(option)
    if (idx !== -1) list.splice(idx, 1)
  }
  updateFieldValue(itemKey, fieldKey, list)
}
</script>
<style scoped>
.delivery-form { padding: 24px; border: 1px solid var(--ui-border); border-radius: 16px; background: var(--ui-bg-elevated); color: var(--ui-text-primary); }
.delivery-heading { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 22px; }
.delivery-heading svg { margin-top: 3px; flex: none; color: var(--ui-accent); }
.delivery-heading h2 { font-size: 18px; font-weight: 750; line-height: 1.5; }
.delivery-heading p { margin-top: 6px; color: var(--ui-text-secondary); font-size: 13px; line-height: 1.6; }
.delivery-product { padding-top: 18px; border-top: 1px solid var(--ui-border); }
.delivery-product h3 { margin-bottom: 16px; font-size: 14px; font-weight: 700; }
.delivery-field { min-width: 0; }
.delivery-field > label { display: block; margin-bottom: 8px; color: var(--ui-text-primary); font-size: 13px; font-weight: 600; }
.delivery-input { display: block; width: 100%; min-height: 44px; border: 1px solid var(--ui-border-strong); border-radius: 10px; background: var(--ui-bg-page); color: var(--ui-text-primary); padding: 11px 13px; font-size: 14px; line-height: 1.6; box-shadow: none; transition: border-color 120ms; }
.delivery-input::placeholder { color: var(--ui-text-secondary); opacity: 1; }
.delivery-input:hover { border-color: color-mix(in srgb, var(--ui-text-primary) 35%, var(--ui-border)); }
.delivery-input:focus-visible { outline: 2px solid var(--ui-accent); outline-offset: 2px; }
.delivery-textarea { min-height: 112px; resize: vertical; }
.delivery-options { display: grid; gap: 9px; padding: 12px; border: 1px solid var(--ui-border); border-radius: 10px; background: var(--ui-bg-page); }
.delivery-options label { min-height: 32px; cursor: pointer; }
.delivery-field-invalid .delivery-input { border-color: var(--ui-danger); }
.delivery-error { color: var(--ui-danger); font-size: 12px; margin-top: 5px; }
.delivery-error:empty { display: none; }
@media (max-width: 640px) {
  .delivery-form { padding: 18px 16px; }
  .delivery-input { font-size: 16px; }
}
@media (prefers-reduced-motion: reduce) { .delivery-input { transition: none; } }
</style>
