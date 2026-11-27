<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="not-found-shell min-h-screen flex items-center justify-center px-6 py-12">
    <main class="not-found-copy text-center">
      <h1 class="not-found-title">
        {{ t('notFoundTitle') }}
      </h1>
      <p class="not-found-subtitle">
        {{ t('notFoundSubtitle') }}
      </p>
    </main>
  </div>
</template>

<style>
@import './not-found-page.css';
</style>
