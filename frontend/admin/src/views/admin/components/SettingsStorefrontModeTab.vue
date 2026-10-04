<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { PauseCircle, Store } from 'lucide-vue-next'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { notifyError, notifySuccess } from '@/utils/notify'

type SupportedLanguage = 'zh-CN' | 'zh-TW' | 'en-US'
type SamplingLabel = Record<SupportedLanguage, string>

const props = defineProps<{
  mode: 'open' | 'sampling'
  modelValue: SamplingLabel[]
  currentLang: SupportedLanguage
}>()

const emit = defineEmits<{
  'update:modelValue': [value: SamplingLabel[]]
  'mode-changed': [mode: 'open' | 'sampling']
}>()

const { t } = useI18n()
const submitting = ref(false)

const updateLabel = (index: number, value: string | number) => {
  const labels = Array.from({ length: 3 }, (_, itemIndex) => ({
    ...(props.modelValue[itemIndex] || { 'zh-CN': '', 'zh-TW': '', 'en-US': '' }),
  })) as SamplingLabel[]
  const label = labels[index]
  if (!label) return
  label[props.currentLang] = String(value)
  emit('update:modelValue', labels)
}

const setMode = async (mode: 'open' | 'sampling') => {
  if (submitting.value || props.mode === mode) return
  submitting.value = true
  try {
    const response = await adminAPI.getSettings({ key: 'site_config' })
    const raw = response.data?.data
    const current = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}
    await adminAPI.updateSettings({
      key: 'site_config',
      value: { ...current, storefront_mode: mode },
    })
    emit('mode-changed', mode)
    notifySuccess(t(mode === 'sampling' ? 'admin.settings.storefrontMode.samplingEnabled' : 'admin.settings.storefrontMode.storeOpened'))
  } catch {
    notifyError(t('admin.settings.alerts.saveFailed'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="space-y-6">
    <section class="rounded-xl border border-border bg-card">
      <div class="border-b border-border bg-muted/40 px-6 py-4">
        <h2 class="text-lg font-semibold">{{ t('admin.settings.storefrontMode.title') }}</h2>
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.storefrontMode.subtitle') }}</p>
      </div>
      <div class="space-y-6 p-6">
        <div class="flex flex-col gap-4 rounded-lg border px-4 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex items-start gap-3">
            <PauseCircle v-if="mode === 'sampling'" class="mt-0.5 h-5 w-5 text-muted-foreground" />
            <Store v-else class="mt-0.5 h-5 w-5 text-emerald-600" />
            <div>
              <p class="font-semibold">{{ t(mode === 'sampling' ? 'admin.settings.storefrontMode.samplingActive' : 'admin.settings.storefrontMode.openActive') }}</p>
              <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.settings.storefrontMode.impact') }}</p>
            </div>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button type="button" variant="outline" :disabled="submitting || mode === 'sampling'" @click="setMode('sampling')">
              <PauseCircle class="mr-2 h-4 w-4" />{{ t('admin.settings.storefrontMode.startSampling') }}
            </Button>
            <Button type="button" :disabled="submitting || mode === 'open'" @click="setMode('open')">
              <Store class="mr-2 h-4 w-4" />{{ t('admin.settings.storefrontMode.openStore') }}
            </Button>
          </div>
        </div>

        <div class="space-y-3">
          <div>
            <h3 class="text-sm font-semibold">{{ t('admin.settings.storefrontMode.labelsTitle') }}</h3>
            <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.storefrontMode.labelsHint') }}</p>
          </div>
          <div v-for="(label, index) in modelValue" :key="index" class="space-y-2">
            <Label :for="`storefront-sampling-label-${index}`">{{ t('admin.settings.storefrontMode.label', { index: index + 1 }) }}</Label>
            <Input
              :id="`storefront-sampling-label-${index}`"
              :model-value="label[currentLang]"
              :maxlength="100"
              @update:model-value="updateLabel(index, $event)"
            />
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
