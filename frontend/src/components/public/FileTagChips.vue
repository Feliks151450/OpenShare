<script setup lang="ts">
import type { PublicFileTag } from "../../lib/publicFileTags";
import { readableTextColorForPreset } from "../../lib/publicFileTags";

const props = withDefaults(
  defineProps<{
    tags: PublicFileTag[];
    size?: "sm" | "md";
    wrap?: boolean;
  }>(),
  {
    size: "sm",
    wrap: true,
  },
);

function chipClass() {
  return props.size === "md"
    ? "rounded-lg px-2.5 py-1 text-xs font-medium"
    : "rounded-md px-2 py-0.5 text-[13px] font-medium leading-snug";
}
</script>

<template>
  <div v-if="tags.length > 0" class="flex min-w-0 gap-1.5" :class="wrap ? 'flex-wrap' : 'flex-nowrap overflow-x-auto'">
    <span
      v-for="t in tags"
      :key="t.id"
      class="max-w-full shrink-0 truncate ring-1 ring-black/10"
      :class="chipClass()"
      :style="{
        backgroundColor: t.color,
        color: readableTextColorForPreset(t.color),
      }"
      :title="t.name"
    >
      {{ t.name }}
    </span>
  </div>
</template>
