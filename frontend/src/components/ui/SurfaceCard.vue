<script setup lang="ts">
withDefaults(
  defineProps<{
    padded?: boolean;
    subtle?: boolean;
  }>(),
  {
    padded: true,
    subtle: false,
  },
);
</script>

<template>
  <section
    class="rounded-none px-2 py-2 border shadow-sm sm:rounded-xl sm:px-5 sm:py-5"
    :class="[
      subtle ? 'border-slate-100 bg-slate-50/70 shadow-none' : 'border-slate-200 bg-white shadow-slate-950/[0.03]',
      padded ? 'p-6' : '',
    ]"
  >
    <slot />
  </section>
</template>
