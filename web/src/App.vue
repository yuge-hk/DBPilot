<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NMessageProvider,
  NDialogProvider,
  NNotificationProvider,
  NGlobalStyle,
  NLoadingBarProvider,
  darkTheme,
  zhCN,
  dateZhCN,
} from 'naive-ui'
import { useThemeStore } from '@/stores/theme'
import { useAuthStore } from '@/stores/auth'
import { setUnauthorizedHandler } from '@/api/client'

const themeStore = useThemeStore()
const authStore = useAuthStore()
const router = useRouter()

const theme = computed(() => (themeStore.dark ? darkTheme : null))

setUnauthorizedHandler(() => {
  authStore.logout()
  router.push('/login')
})

onMounted(async () => {
  await authStore.checkStatus()
  if (authStore.initialized === false) {
    router.push('/login')
    return
  }
  const ok = await authStore.fetchMe()
  if (!ok) {
    router.push('/login')
  }
})
</script>

<template>
  <n-config-provider
    :theme="theme"
    :locale="zhCN"
    :date-locale="dateZhCN"
  >
    <n-loading-bar-provider>
      <n-message-provider>
        <n-dialog-provider>
          <n-notification-provider>
            <n-global-style />
            <router-view />
          </n-notification-provider>
        </n-dialog-provider>
      </n-message-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
