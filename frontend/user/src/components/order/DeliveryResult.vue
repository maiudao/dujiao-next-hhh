<script setup lang="ts">
import { PackageCheck } from 'lucide-vue-next'
defineProps<{ payload?: string; lines?: string[]; truncated?: boolean }>()
</script>

<template>
  <section class="order-delivery-result" aria-label="交付结果">
    <h4><PackageCheck :size="18" aria-hidden="true" />交付结果</h4>
    <div class="order-delivery-content" :class="{ 'order-delivery-truncated': truncated }">
      <template v-if="lines?.length"><p v-for="(line, index) in lines" :key="index">{{ line }}</p></template>
      <p v-else>{{ payload || '交付内容暂未填写，请联系店主确认。' }}</p>
    </div>
  </section>
</template>

<style scoped>
.order-delivery-result { margin-top: 14px; padding: 18px 20px; border: 1px solid #87bdaa; border-radius: 8px; background: #eaf7f1; color: #163e31; }
.order-delivery-result h4 { display: flex; align-items: center; gap: 8px; margin: 0 0 12px; color: #163e31; font-size: 15px; font-weight: 800; }
.order-delivery-content { font-size: 16px; font-weight: 600; line-height: 1.75; overflow-wrap: anywhere; white-space: pre-wrap; user-select: text; }
.order-delivery-content p { margin: 0; }
.order-delivery-truncated { max-height: 280px; overflow-y: auto; }
:global(.dark .order-delivery-result) { background: #223b32; border-color: #527e6d; color: #e0f6eb; }
:global(.dark .order-delivery-result h4) { color: #a9e6c8; }
@media (max-width: 640px) { .order-delivery-result { padding: 16px; } .order-delivery-content { font-size: 15px; } }
</style>
