<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { PauseCircle, Store } from 'lucide-vue-next'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { notifyError, notifySuccess } from '@/utils/notify'
import MediaPicker from '@/components/admin/MediaPicker.vue'
import { Textarea } from '@/components/ui/textarea'
import type { StorefrontSupport, SupportMode, StorefrontContacts, ContactKey } from '@/utils/storefrontSupport'
import { Switch } from '@/components/ui/switch'

type SupportedLanguage = 'zh-CN' | 'zh-TW' | 'en-US'
type SamplingLabel = Record<SupportedLanguage, string>

const props = defineProps<{
  mode: 'open' | 'sampling'
  modelValue: SamplingLabel[]
  currentLang: SupportedLanguage
  support: StorefrontSupport
  contacts: StorefrontContacts
}>()

const emit = defineEmits<{
  'update:modelValue': [value: SamplingLabel[]]
  'mode-changed': [mode: 'open' | 'sampling']
  'update:support': [value: StorefrontSupport]
  'update:contacts': [value: StorefrontContacts]
}>()

const { t } = useI18n()
const submitting = ref(false)
const supportModes: SupportMode[] = ['open', 'sampling']
const contactTypes: { key: ContactKey; label: string; placeholder: string }[] = [
  { key: 'qq', label: 'QQ', placeholder: '填写 QQ 号' },
  { key: 'wechat', label: '微信', placeholder: '填写微信号' },
  { key: 'telegram', label: 'Telegram', placeholder: '填写 Telegram 链接' },
]
function updateContact(key: ContactKey, field: 'value' | 'enabled', value: string | boolean | number) {
  emit('update:contacts', { ...props.contacts, [key]: { ...props.contacts[key], [field]: value } })
}

const updateSupport = (mode: SupportMode, field: 'images' | 'messages', values: string[]) => {
  emit('update:support', { ...props.support, [mode]: { ...props.support[mode], [field]: values } })
}
const updateImages = (mode: SupportMode, values: string | string[]) => {
  updateSupport(mode, 'images', (Array.isArray(values) ? values : [values]).filter(Boolean).slice(0, 20))
}
const editMessage = (mode: SupportMode, index: number, text: string | number) => {
  const messages = [...props.support[mode].messages]
  messages[index] = String(text)
  updateSupport(mode, 'messages', messages)
}
const removeMessage = (mode: SupportMode, index: number) => {
  updateSupport(mode, 'messages', props.support[mode].messages.filter((_, i) => i !== index))
}

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
    <section class="rounded-xl border border-border bg-card p-6">
      <h2 class="text-lg font-semibold">联系方式</h2>
      <p class="mt-2 text-sm text-muted-foreground">勾选“公开展示”后，访客可在顶部“联系方式”弹窗查看并复制。关闭展示会同时隐藏号码和链接；填写后记得保存更改。</p>
      <div class="mt-5 grid gap-4">
        <div v-for="item in contactTypes" :key="item.key" class="flex items-center gap-5">
          <div class="w-24 shrink-0"><Label :for="`store-contact-${item.key}`">{{ item.label }}</Label></div>
          <Input :id="`store-contact-${item.key}`" class="max-w-lg" :model-value="contacts[item.key].value" :placeholder="item.placeholder" :maxlength="180" @update:model-value="updateContact(item.key, 'value', $event)" />
          <div class="flex items-center gap-2 whitespace-nowrap">
            <Switch :id="`store-contact-visible-${item.key}`" :model-value="contacts[item.key].enabled" @update:model-value="updateContact(item.key, 'enabled', $event)" />
            <Label :for="`store-contact-visible-${item.key}`">公开展示 {{ item.label }}</Label>
          </div>
        </div>
      </div>
    </section>
    <section class="rounded-xl border border-border bg-card p-6">
      <h2 class="text-lg font-semibold">客服角色与提示语</h2>
      <p class="mt-2 text-sm text-muted-foreground">分别配置两个营业状态。刷新随机展示，点击角色或气泡切换提示；标题固定为“营业中”或“打烊中”。编辑完成后点击页面的保存按钮。</p>
      <div class="mt-6 grid gap-8 xl:grid-cols-2">
        <div v-for="stateMode in supportModes" :key="stateMode" class="space-y-5">
          <div class="border-b pb-3">
            <h3 class="font-bold">{{ stateMode === 'open' ? '营业中' : '打烊中' }}</h3>
          </div>
          <div>
            <Label>角色图片（最多 20 张）</Label>
            <p class="mb-3 mt-1 text-xs text-muted-foreground">建议使用透明 PNG / WebP。可以上传、从图库添加或移除；移除仅取消当前配置，不删除图库原文件。</p>
            <MediaPicker :model-value="support[stateMode].images" multiple @update:model-value="updateImages(stateMode, $event)" />
          </div>
          <div class="space-y-3">
            <Label>对话正文（最多 30 条）</Label>
            <div v-for="(message, index) in support[stateMode].messages" :key="index" class="flex items-start gap-2">
              <Textarea :model-value="message" :maxlength="180" :rows="3" :aria-label="`${stateMode === 'open' ? '营业' : '打烊'}提示 ${index + 1}`"
                class="flex-1" @update:model-value="editMessage(stateMode, index, $event)" />
              <Button type="button" variant="outline" :disabled="support[stateMode].messages.length <= 1" @click="removeMessage(stateMode, index)">删除</Button>
            </div>
            <Button type="button" variant="outline" :disabled="support[stateMode].messages.length >= 30"
              @click="updateSupport(stateMode, 'messages', [...support[stateMode].messages, ''])">添加提示语</Button>
            <p class="text-xs text-muted-foreground">正文为纯文字，不执行 HTML。至少保留一条；空白内容保存时会略过。</p>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
