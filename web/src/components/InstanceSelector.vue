<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { NSelect } from 'naive-ui'
import { useI18n } from 'vue-i18n'
import { useInstanceStore } from '@/stores/instance'

const { t } = useI18n()
const store = useInstanceStore()

const options = computed(() =>
  store.instances.map((i) => ({ label: i.name, value: i.id })),
)

onMounted(() => store.load())
</script>

<template>
  <n-select
    v-model:value="store.selectedId"
    :options="options"
    style="width: 240px"
    :placeholder="t('instances.selectPlaceholder')"
  />
</template>
