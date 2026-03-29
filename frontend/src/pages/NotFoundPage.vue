<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>

<template>
  <div class="page-shell not-found-shell min-h-screen p-2 sm:p-3 transition-colors duration-300">
    <div class="page-card not-found-card relative flex min-h-full w-full overflow-hidden rounded-2xl">
      <div class="not-found-glow not-found-glow--primary" aria-hidden="true" />
      <div class="not-found-glow not-found-glow--secondary" aria-hidden="true" />

      <main class="not-found-content relative z-10 flex min-h-[calc(100vh-1rem)] w-full items-center justify-center px-6 py-12 sm:min-h-[calc(100vh-1.5rem)] sm:px-10">
        <div class="not-found-copy text-center">
          <h1 class="not-found-title">
            {{ t('notFoundTitle') }}
          </h1>
          <p class="not-found-subtitle">
            {{ t('notFoundSubtitle') }}
          </p>
        </div>
      </main>
    </div>
  </div>
</template>

<style>
@import './not-found-page.css';
</style>
