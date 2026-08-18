<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { NForm, NFormItem, NInput, NButton, NCard, useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { ApiRequestError } from '@/api/client'

const { t } = useI18n()
const auth = useAuthStore()
const theme = useThemeStore()
const router = useRouter()
const message = useMessage()

const isSetup = computed(() => auth.initialized === false)
const username = ref('')
const password = ref('')
const confirmPassword = ref('')
const loading = ref(false)

async function submit() {
  if (isSetup.value && password.value !== confirmPassword.value) {
    message.error(t('auth.passwordMismatch'))
    return
  }
  loading.value = true
  try {
    if (isSetup.value) {
      await auth.setup(username.value, password.value)
      message.success(t('common.success'))
    } else {
      await auth.login(username.value, password.value)
    }
    router.push('/')
  } catch (e) {
    if (e instanceof ApiRequestError && e.code === 'invalid_request') {
      message.error(t('auth.invalidCredentials'))
    } else if (e instanceof ApiRequestError) {
      message.error(t(`errors.${e.code}`))
    } else {
      message.error(t('common.failed'))
    }
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <n-card class="login-card" :title="isSetup ? t('auth.setupTitle') : t('auth.loginTitle')">
      <div class="brand">
        <div class="brand-name">{{ t('app.name') }}</div>
        <div class="brand-sub">{{ t('app.subtitle') }}</div>
      </div>
      <n-form @submit.prevent="submit">
        <n-form-item :label="t('auth.username')">
          <n-input
            v-model:value="username"
            :placeholder="t('auth.username')"
            autocomplete="username"
            @keydown.enter="submit"
          />
        </n-form-item>
        <n-form-item :label="t('auth.password')">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            :placeholder="t('auth.password')"
            autocomplete="current-password"
            @keydown.enter="submit"
          />
        </n-form-item>
        <n-form-item v-if="isSetup" :label="t('auth.confirmPassword')">
          <n-input
            v-model:value="confirmPassword"
            type="password"
            show-password-on="click"
            :placeholder="t('auth.confirmPassword')"
            @keydown.enter="submit"
          />
        </n-form-item>
        <n-button
          type="primary"
          block
          attr-type="submit"
          :loading="loading"
        >
          {{ isSetup ? t('common.confirm') : t('auth.login') }}
        </n-button>
      </n-form>
      <div v-if="isSetup" class="hint">{{ t('auth.setupHint') }}</div>
    </n-card>
    <div class="theme-toggle">
      <n-button quaternary size="small" @click="theme.toggle">
        {{ theme.dark ? 'Light' : 'Dark' }}
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 16px;
}
.login-card {
  width: 360px;
}
.brand {
  text-align: center;
  margin-bottom: 20px;
}
.brand-name {
  font-size: 24px;
  font-weight: 600;
}
.brand-sub {
  font-size: 13px;
  opacity: 0.6;
  margin-top: 4px;
}
.hint {
  margin-top: 12px;
  font-size: 12px;
  opacity: 0.6;
  text-align: center;
}
</style>
