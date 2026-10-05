<script setup lang="ts">
import { computed } from 'vue'
import { DialogRoot, DialogPortal, DialogOverlay, DialogContent, DialogTitle, DialogDescription, DialogClose } from 'reka-ui'
import { X } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { toast } from '../composables/useToast'
import { copyText } from '../utils/clipboard'
import { visibleStorefrontContacts } from '../utils/storefrontContacts'

defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [value: boolean] }>()
const app = useAppStore()
const contacts = computed(() => visibleStorefrontContacts(app.config?.storefront_contacts, app.config?.contact?.telegram || ''))
async function copy(value: string, label: string) {
  try { await copyText(value); toast.success(`${label}已复制`) }
  catch { toast.error('复制未成功，请长按联系方式手动复制') }
}
</script>

<template>
  <DialogRoot :open="open" @update:open="emit('update:open', $event)">
    <DialogPortal>
      <DialogOverlay class="contact-overlay" />
      <DialogContent class="contact-dialog">
        <DialogTitle class="contact-heading">联系店主</DialogTitle>
        <DialogDescription class="contact-description">购买前后有疑问，都可以联系我。点击联系方式即可复制。</DialogDescription>
        <div v-if="contacts.length" class="contact-list">
          <button v-for="item in contacts" :key="item.key" type="button" class="contact-row"
            :aria-label="`复制${item.copyLabel}`" @click="copy(item.value, item.copyLabel)">
            <span class="contact-mark"><i class="contact-icon" :style="{ maskImage: `url(/storefront/contact-${item.key}.svg)` }" aria-hidden="true"></i></span>
            <span class="contact-details"><span class="contact-label">{{ item.label }}</span><span class="contact-value">{{ item.value }}</span></span>
            <i class="contact-copy" aria-hidden="true"></i>
          </button>
        </div>
        <p v-else class="contact-empty">店主暂未公开联系方式，请稍后再来查看。</p>
        <DialogClose class="contact-close" aria-label="关闭联系方式"><X :size="18" /></DialogClose>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<style scoped>
.contact-overlay { position: fixed; inset: 0; z-index: 80; background: rgb(10 17 33 / 45%); }
.contact-dialog { position: fixed; z-index: 81; left: 50%; top: 50%; transform: translate(-50%, -50%); width: min(440px, calc(100vw - 32px)); max-height: calc(100dvh - 48px); overflow-y: auto; padding: 28px; border-radius: 16px; background: var(--ui-bg-elevated); color: var(--ui-text-primary); box-shadow: 0 20px 65px rgb(0 0 0 / 22%); }
.contact-heading { margin: 0 32px 8px 0; font-size: 22px; font-weight: 800; }
.contact-description { margin: 0 0 22px; color: var(--ui-text-secondary); font-size: 13px; line-height: 1.7; }
.contact-list { display: grid; gap: 10px; }
.contact-row { display: flex; align-items: center; gap: 14px; width: 100%; min-height: 74px; padding: 12px 14px; border: 1px solid var(--ui-border); border-radius: 12px; background: var(--ui-bg-page); color: inherit; text-align: left; cursor: pointer; transition: border-color 140ms, background-color 140ms; }
.contact-row:hover { border-color: var(--ui-accent); background: var(--ui-bg-soft); }
.contact-mark { display: grid; place-items: center; flex: 0 0 30px; color: var(--ui-accent); }
.contact-icon, .contact-copy { display: block; width: 23px; height: 23px; background: currentColor; mask-size: contain; mask-position: center; mask-repeat: no-repeat; }
.contact-details { display: grid; gap: 3px; min-width: 0; flex: 1; }
.contact-label { color: var(--ui-text-secondary); font-size: 11px; font-weight: 600; }
.contact-value { font-size: 14px; font-weight: 700; line-height: 1.5; overflow-wrap: anywhere; user-select: text; font-variant-numeric: tabular-nums; }
.contact-copy { flex: 0 0 16px; width: 16px; height: 16px; color: var(--ui-accent); mask-image: url('/storefront/contact-copy.svg'); }
.contact-close { position: absolute; right: 16px; top: 16px; display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 50%; background: var(--ui-bg-page); color: var(--ui-text-secondary); cursor: pointer; }
.contact-close:hover { color: var(--ui-text-primary); background: var(--ui-bg-soft); }
.contact-empty { padding: 18px 0; font-size: 14px; color: var(--ui-text-secondary); }
.contact-dialog button:focus-visible { outline: 2px solid var(--ui-accent); outline-offset: 3px; }
.contact-dialog[data-state='open'] { animation: contact-appear 200ms cubic-bezier(.16,1,.3,1); }
@keyframes contact-appear { from { opacity: 0; transform: translate(-50%, calc(-50% + 8px)) scale(.97); } }
@media (max-width: 600px) { .contact-dialog { padding: 24px 20px; } .contact-row { gap: 10px; padding: 12px; } }
@media (prefers-reduced-motion: reduce) { .contact-dialog[data-state='open'] { animation: none; } }
</style>
