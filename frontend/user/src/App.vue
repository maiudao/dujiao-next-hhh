<template>
  <div id="app" class="min-h-screen bg-background text-foreground flex flex-col">
    <!-- vault 模板：自带顶栏/页脚的外壳包裹页面（控制台仍走下方分支） -->
    <VaultLayout v-if="isVault && !isResellerConsole">
      <ErrorBoundary>
        <RouterView v-slot="{ Component }">
          <component :is="Component" />
        </RouterView>
      </ErrorBoundary>
    </VaultLayout>

    <!-- classic 模板 / 分销控制台（保持原有结构不变） -->
    <template v-else>
      <Navbar v-if="!isResellerConsole" />
      <main class="flex-1" :class="isResellerConsole ? '' : 'pb-14 lg:pb-0'">
        <ErrorBoundary>
          <RouterView v-slot="{ Component }">
            <component :is="Component" />
          </RouterView>
        </ErrorBoundary>
      </main>
      <Footer v-if="!isResellerConsole" />
      <BackToTop v-if="!isResellerConsole" />
      <MobileBottomNav v-if="!isResellerConsole" />
    </template>

    <SupportCharacter />
    <Toast />
    <ConfirmDialog />
  </div>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent, watch } from 'vue'
import { useRoute } from 'vue-router'
import { getActiveTemplate, loadVaultLayout } from './templates/registry'
import Navbar from './components/Navbar.vue'
import Footer from './components/Footer.vue'
import Toast from './components/Toast.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import ErrorBoundary from './components/ErrorBoundary.vue'
import BackToTop from './components/BackToTop.vue'
import MobileBottomNav from './components/MobileBottomNav.vue'
import SupportCharacter from './components/SupportCharacter.vue'
import { useStorefrontRefresh } from './composables/useStorefrontRefresh'

// vault 外壳按需加载，classic 用户不会拉取其 chunk/样式
const VaultLayout = defineAsyncComponent(loadVaultLayout)

// config 由 router.beforeEach 统一加载，无需在此重复调用
useStorefrontRefresh()
const route = useRoute()
const isResellerConsole = computed(() => route.meta.resellerConsole === true)
// 启动时确定模板，外壳与路由始终使用同一份选择。
const isVault = computed(() => getActiveTemplate() === 'vault')
// 首次渲染前给 body 的弹窗/提示同步配色，不等异步外壳的 mounted。
watch([isVault, isResellerConsole], ([vault, consolePage]) => {
  document.body.classList.toggle('vault-tokens', vault && !consolePage)
}, { immediate: true, flush: 'sync' })
</script>

<style>
.storefront-payment-closed:disabled { background: var(--ui-bg-secondary, #eef0f4); color: var(--ui-text-secondary, #646b7b); border: 1px solid var(--ui-border, #d9dce5); opacity: 1; cursor: not-allowed; }
</style>
