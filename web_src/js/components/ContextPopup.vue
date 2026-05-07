<script lang="ts" setup>
import {SvgIcon} from '../svg.ts';
import {getIssueColorClass, getIssueIcon} from '../features/issue.ts';
import {computed} from 'vue';
import type {Issue} from '../types.ts';

const props = defineProps<{
  issue?: Issue | null,
  renderedLabels?: string,
  errorMessage?: string,
}>();

const createdAt = computed(() => {
  if (!props.issue) return '';
  return new Date(props.issue.created_at).toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'});
});

const body = computed(() => {
  if (!props.issue) return '';
  const body = props.issue.body.replace(/\n+/g, ' ');
  return body.length > 85 ? `${body.substring(0, 85)}…` : body;
});
</script>

<template>
  <div class="tw-p-4">
    <div v-if="issue" class="tw-flex tw-flex-col tw-gap-2">
      <div class="tw-text-12">
        <a :href="issue.repository.html_url" class="muted">{{ issue.repository.full_name }}</a>
        on {{ createdAt }}
      </div>
      <div class="flex-text-block">
        <svg-icon :name="getIssueIcon(issue)" :class="getIssueColorClass(issue)"/>
        <a :href="issue.html_url" class="issue-title tw-font-semibold tw-break-anywhere muted">
          {{ issue.title }}
          <span class="index">#{{ issue.number }}</span>
        </a>
      </div>
      <div v-if="body">{{ body }}</div>
      <div>
        <a :href="`${issue.user.html_url}?tab=activity`" :title="issue.user.username" class="author text black tw-font-semibold muted" target="_blank">{{ issue.user.full_name }}</a>&nbsp;
        <!-- eslint-disable-next-line vue/no-v-html -->
        <span v-if="issue.labels.length" v-html="renderedLabels"/>
      </div>
    </div>
    <div v-else>
      {{ errorMessage }}
    </div>
  </div>
</template>
