<script setup lang="ts">
import { onMounted, computed } from 'vue'
import { NSelect } from 'naive-ui'
import { useInstanceStore } from '@/stores/instance'

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
    placeholder="选择实例"
  />
</template>
