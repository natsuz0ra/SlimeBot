<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { mdiGithub } from '@mdi/js'

import MdiIcon from '@/components/ui/MdiIcon.vue'

const { t } = useI18n()

const githubUrl = 'https://github.com/natsuz0ra/SlimeBot'
const version = `v${__APP_VERSION__}`
</script>

<template>
  <div>
    <p class="section-label">{{ t('aboutSettings') }}</p>

    <div class="settings-card about-card rounded-xl px-5 py-6">
      <div class="about-hero">
        <div class="about-logo-wrap">
          <img src="/slime-icon.svg" alt="SlimeBot" class="about-logo" />
        </div>
        <h2 class="about-title">SlimeBot</h2>
      </div>

      <div class="about-info">
        <div class="about-row">
          <span class="text-sm settings-field-label">{{ t('appVersion') }}</span>
          <span class="text-sm settings-item-sub">{{ version }}</span>
        </div>

        <div class="about-row">
          <span class="text-sm settings-field-label">{{ t('githubRepository') }}</span>
          <a
            class="about-link"
            :href="githubUrl"
            target="_blank"
            rel="noreferrer"
          >
            <MdiIcon :path="mdiGithub" :size="16" />
            <span>natsuz0ra/SlimeBot</span>
          </a>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.about-card {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.about-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 8px 0 2px;
}

.about-logo-wrap {
  width: 88px;
  height: 88px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 22px;
  background: var(--primary-alpha-08);
  border: 1px solid var(--primary-alpha-15);
}

.about-logo {
  width: 68px;
  height: 68px;
  display: block;
}

.about-title {
  color: var(--text-primary);
  font-size: 22px;
  line-height: 1.25;
  font-weight: 700;
  margin: 0;
}

.about-info {
  display: flex;
  flex-direction: column;
  border-top: 1px solid var(--card-border);
}

.about-row {
  min-height: 48px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border-bottom: 1px solid var(--card-border);
}

.about-row:last-child {
  border-bottom: none;
}

.about-link {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--sb-brand);
  font-size: 14px;
  line-height: 1.3;
  text-decoration: none;
  transition: color 0.15s;
}

.about-link:hover {
  color: var(--sb-brand-hover);
}

@media (max-width: 640px) {
  .about-card {
    padding: 20px 16px;
  }

  .about-row {
    align-items: flex-start;
    flex-direction: column;
    justify-content: center;
    gap: 6px;
    padding: 10px 0;
  }
}
</style>
