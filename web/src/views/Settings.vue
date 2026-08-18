<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NInput, NForm, NFormItem, NCard, useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const message = useMessage()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const loading = ref(false)

async function changePassword() {
  if (newPassword.value !== confirmPassword.value) {
    message.error(t('settings.passwordMismatch'))
    return
  }
  loading.value = true
  try {
    const { api } = await import('@/api/client')
    await api.post('/auth/change-password', {
      current_password: currentPassword.value,
      new_password: newPassword.value,
    })
    message.success(t('settings.changeSuccess'))
    await auth.logout()
    router.push('/login')
  } catch (e) {
    if (e instanceof ApiRequestError) message.error(t(`errors.${e.code}`))
    else message.error(t('common.failed'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div>
    <n-card :title="t('settings.changePassword')" style="max-width: 480px">
      <n-form label-placement="left" label-width="100">
        <n-form-item :label="t('settings.currentPassword')">
          <n-input v-model:value="currentPassword" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item :label="t('settings.newPassword')">
          <n-input v-model:value="newPassword" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item :label="t('settings.confirmPassword')">
          <n-input v-model:value="confirmPassword" type="password" show-password-on="click" />
        </n-form-item>
      </n-form>
      <n-button type="primary" :loading="loading" @click="changePassword">
        {{ t('settings.changePassword') }}
      </n-button>
    </n-card>

    <n-card :title="t('settings.theme')" style="max-width: 480px; margin-top: 16px">
      <n-button @click="theme.toggle">
        {{ theme.dark ? t('settings.light') : t('settings.dark') }}
      </n-button>
    </n-card>
  </div>
</template>
