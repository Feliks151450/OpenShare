<script setup lang="ts">
import { onMounted, ref } from "vue";

import EmptyState from "../../components/ui/EmptyState.vue";
import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { HttpError, httpClient } from "../../lib/http/client";
import { clearStoredReceiptCode, ensureSessionReceiptCode, readStoredReceiptCode } from "../../lib/receiptCode";

interface SubmissionLookupResponse {
  receipt_code: string;
  items: Array<{
    title: string;
    relative_path: string;
    status: string;
    uploaded_at: string;
    download_count: number;
    reject_reason?: string;
  }>;
}

interface FeedbackLookupResponse {
  receipt_code: string;
  items: Array<{
    target_name: string;
    target_type: string;
    reason: string;
    reason_label: string;
    description: string;
    status: string;
    review_reason: string;
    created_at: string;
    reviewed_at: string | null;
  }>;
}

const receiptCode = ref("");
const lookupLoading = ref(false);
const lookupError = ref("");
const submissionLookupResult = ref<SubmissionLookupResponse | null>(null);
const feedbackLookupResult = ref<FeedbackLookupResponse | null>(null);

onMounted(() => {
  void syncSessionReceiptCode();
  localStorage.removeItem("openshare_feedback_receipt_code");
});

async function lookupReceipt() {
  const code = receiptCode.value.trim();
  if (!code) {
    lookupError.value = "请输入回执码。";
    submissionLookupResult.value = null;
    feedbackLookupResult.value = null;
    return;
  }

  lookupLoading.value = true;
  lookupError.value = "";
  submissionLookupResult.value = null;
  feedbackLookupResult.value = null;

  const [submissionResult, feedbackResult] = await Promise.allSettled([
    httpClient.get<SubmissionLookupResponse>(`/public/submissions/${encodeURIComponent(code)}`),
    httpClient.get<FeedbackLookupResponse>(`/public/reports/${encodeURIComponent(code)}`),
  ]);

  const submissionError = submissionResult.status === "rejected" ? submissionResult.reason : null;
  const feedbackError = feedbackResult.status === "rejected" ? feedbackResult.reason : null;
  const fatalSubmissionError =
    submissionError instanceof HttpError ? submissionError.status !== 404 : Boolean(submissionError);
  const fatalFeedbackError =
    feedbackError instanceof HttpError ? feedbackError.status !== 404 : Boolean(feedbackError);

  if (fatalSubmissionError || fatalFeedbackError) {
    lookupError.value = "查询回执失败。";
    lookupLoading.value = false;
    return;
  }

  if (submissionResult.status === "fulfilled") {
    submissionLookupResult.value = submissionResult.value;
    sessionStorage.setItem("openshare_receipt_code", submissionResult.value.receipt_code);
  }
  if (feedbackResult.status === "fulfilled") {
    feedbackLookupResult.value = feedbackResult.value;
    sessionStorage.setItem("openshare_receipt_code", feedbackResult.value.receipt_code);
  }

  if (!submissionLookupResult.value && !feedbackLookupResult.value) {
    lookupError.value = "未找到对应回执码。";
  }
  lookupLoading.value = false;
}

function clearReceipt() {
  clearStoredReceiptCode();
  submissionLookupResult.value = null;
  feedbackLookupResult.value = null;
  lookupError.value = "";
  localStorage.removeItem("openshare_feedback_receipt_code");
  void syncSessionReceiptCode();
}

async function syncSessionReceiptCode() {
  try {
    receiptCode.value = await ensureSessionReceiptCode();
  } catch {
    receiptCode.value = readStoredReceiptCode();
  }
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function submissionStatusLabel(status: string) {
  const labels: Record<string, string> = {
    pending: "待审核",
    approved: "已通过",
    rejected: "已驳回",
  };
  return labels[status] ?? status;
}

function feedbackStatusLabel(status: string) {
  const labels: Record<string, string> = {
    pending: "待处理",
    approved: "已处理",
    rejected: "已驳回",
  };
  return labels[status] ?? status;
}

function feedbackReasonLabel(item: FeedbackLookupResponse["items"][number]) {
  return item.reason_label || item.reason || "-";
}
</script>

<template>
  <div class="app-container py-8 sm:py-10">
    <div class="mx-auto max-w-[66%] min-w-[720px]">
      <SurfaceCard>
        <PageHeader
          eyebrow="Receipt"
          title="回执查询"
        />

        <div class="mt-6 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm leading-7 text-slate-600">
          本会话回执码为：<span class="font-semibold text-slate-900">{{ receiptCode || "暂未同步" }}</span>。请妥善保存该回执码，若清除浏览器缓存或更换浏览器/设备，该回执码将会改变。
        </div>

        <div class="mt-6 flex gap-3">
          <input
            v-model="receiptCode"
            class="field flex-1"
            placeholder="输入回执码"
            readonly
          />
          <button class="btn-secondary" :disabled="lookupLoading" @click="lookupReceipt">
            {{ lookupLoading ? "查询中…" : "查询" }}
          </button>
        </div>

        <div class="mt-4 flex gap-3">
          <button class="text-sm text-slate-500 transition hover:text-slate-900" @click="clearReceipt">
            清除本地回执码
          </button>
        </div>

        <p v-if="lookupError" class="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
          {{ lookupError }}
        </p>
        <p v-else-if="lookupLoading" class="mt-4 text-sm text-slate-500">正在查询…</p>

        <div v-if="submissionLookupResult" class="mt-6 space-y-3">
          <div class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
            当前类型：<span class="font-medium text-slate-900">上传记录</span>
          </div>
          <article
            v-for="item in submissionLookupResult.items"
            :key="`${item.title}-${item.uploaded_at}`"
            class="rounded-xl border border-slate-200 p-4"
          >
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-slate-900">{{ item.title }}</h3>
                <p class="mt-1 text-sm text-slate-500">{{ formatDate(item.uploaded_at) }}</p>
                <p v-if="item.relative_path" class="mt-1 text-sm text-slate-500">目录结构：{{ item.relative_path }}</p>
              </div>
              <span class="rounded-md bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-700">
                {{ submissionStatusLabel(item.status) }}
              </span>
            </div>
            <div class="mt-3 flex flex-wrap gap-4 text-sm text-slate-500">
              <span>下载 {{ item.download_count }}</span>
              <span v-if="item.reject_reason">驳回原因：{{ item.reject_reason }}</span>
            </div>
          </article>
        </div>

        <div v-if="feedbackLookupResult" class="mt-6 space-y-3">
          <div class="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
            当前类型：<span class="font-medium text-slate-900">反馈记录</span>
          </div>
          <article
            v-for="item in feedbackLookupResult.items"
            :key="`${item.target_type}-${item.target_name}-${item.created_at}`"
            class="rounded-xl border border-slate-200 p-4 text-sm text-slate-600"
          >
            <div class="grid gap-3">
              <p><span class="font-medium text-slate-900">回执码：</span>{{ feedbackLookupResult.receipt_code }}</p>
              <p><span class="font-medium text-slate-900">目标：</span>{{ item.target_name || "-" }}</p>
              <p><span class="font-medium text-slate-900">类型：</span>{{ item.target_type === "folder" ? "文件夹" : "文件" }}</p>
              <p><span class="font-medium text-slate-900">原因：</span>{{ feedbackReasonLabel(item) }}</p>
              <p><span class="font-medium text-slate-900">状态：</span>{{ feedbackStatusLabel(item.status) }}</p>
              <p><span class="font-medium text-slate-900">提交时间：</span>{{ formatDate(item.created_at) }}</p>
              <p v-if="item.description"><span class="font-medium text-slate-900">说明：</span>{{ item.description }}</p>
              <p v-if="item.reviewed_at"><span class="font-medium text-slate-900">处理时间：</span>{{ formatDate(item.reviewed_at) }}</p>
              <p v-if="item.review_reason"><span class="font-medium text-slate-900">{{ item.status === 'rejected' ? '驳回说明：' : '处理意见：' }}</span>{{ item.review_reason }}</p>
            </div>
          </article>
        </div>

        <div v-if="!submissionLookupResult && !feedbackLookupResult" class="mt-6">
          <EmptyState title="输入回执码后查看记录" />
        </div>
      </SurfaceCard>
    </div>
  </div>
</template>
