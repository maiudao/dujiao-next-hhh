<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

type SupportedLanguage = 'zh-CN' | 'zh-TW' | 'en-US'
type LocalizedText = Record<SupportedLanguage, string>

const props = defineProps<{
  modelValue: Record<string, any>
  currentLang: SupportedLanguage
}>()

const emit = defineEmits<{
  'update:modelValue': [value: Record<string, any>]
}>()

const { t } = useI18n()

const updateLocalized = (field: string, value: string | number) => {
  emit('update:modelValue', {
    ...props.modelValue,
    [field]: {
      ...(props.modelValue[field] || {}),
      [props.currentLang]: String(value),
    },
  })
}

const updateURL = (field: string, value: string | number) => {
  emit('update:modelValue', { ...props.modelValue, [field]: String(value) })
}

const updateHighlight = (index: number, value: string | number) => {
  const highlights = Array.from({ length: 3 }, (_, itemIndex) => ({
    ...((props.modelValue.highlights?.[itemIndex] || {}) as LocalizedText),
  }))
  const highlight = highlights[index]
  if (!highlight) return
  highlight[props.currentLang] = String(value)
  emit('update:modelValue', { ...props.modelValue, highlights })
}
</script>

<template>
  <div class="space-y-6">
    <section class="rounded-xl border border-border bg-card">
      <div class="border-b border-border bg-muted/40 px-6 py-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-lg font-semibold">{{ t('admin.settings.homeLanding.title') }}</h2>
            <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.homeLanding.subtitle') }}</p>
          </div>
          <span class="rounded bg-muted px-2 py-1 text-xs text-muted-foreground">{{ currentLang }}</span>
        </div>
      </div>

      <div class="space-y-6 p-6">
        <div class="grid grid-cols-1 gap-5 md:grid-cols-2">
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.badgePrimary') }}</Label>
            <Input :model-value="modelValue.badge_primary?.[currentLang] || ''" @update:model-value="updateLocalized('badge_primary', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.badgeSecondary') }}</Label>
            <Input :model-value="modelValue.badge_secondary?.[currentLang] || ''" @update:model-value="updateLocalized('badge_secondary', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.headline') }}</Label>
            <Input :model-value="modelValue.title?.[currentLang] || ''" @update:model-value="updateLocalized('title', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.accentHeadline') }}</Label>
            <Input :model-value="modelValue.accent_title?.[currentLang] || ''" @update:model-value="updateLocalized('accent_title', $event)" />
          </div>
          <div class="space-y-2 md:col-span-2">
            <Label>{{ t('admin.settings.homeLanding.description') }}</Label>
            <Textarea
              :model-value="modelValue.description?.[currentLang] || ''"
              rows="4"
              @update:model-value="updateLocalized('description', $event)"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-5 border-t border-border pt-6 md:grid-cols-2">
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.primaryLabel') }}</Label>
            <Input :model-value="modelValue.primary_label?.[currentLang] || ''" @update:model-value="updateLocalized('primary_label', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.primaryURL') }}</Label>
            <Input :model-value="modelValue.primary_url || ''" placeholder="#products" @update:model-value="updateURL('primary_url', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.secondaryLabel') }}</Label>
            <Input :model-value="modelValue.secondary_label?.[currentLang] || ''" @update:model-value="updateLocalized('secondary_label', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.secondaryURL') }}</Label>
            <Input :model-value="modelValue.secondary_url || ''" placeholder="/login" @update:model-value="updateURL('secondary_url', $event)" />
          </div>
        </div>

        <div class="space-y-4 border-t border-border pt-6">
          <div v-for="index in 3" :key="index" class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.highlight', { index }) }}</Label>
            <Input
              :model-value="modelValue.highlights?.[index - 1]?.[currentLang] || ''"
              @update:model-value="updateHighlight(index - 1, $event)"
            />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-5 border-t border-border pt-6 md:grid-cols-2">
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.supportLabel') }}</Label>
            <Input :model-value="modelValue.support_label?.[currentLang] || ''" @update:model-value="updateLocalized('support_label', $event)" />
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.homeLanding.supportURL') }}</Label>
            <Input :model-value="modelValue.support_url || ''" placeholder="https://t.me/..." @update:model-value="updateURL('support_url', $event)" />
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
