// Async markdown rendering via Web Worker.
// Falls back to synchronous renderSimpleMarkdown if the worker fails to load.
import DOMPurify from "dompurify";
import { renderSimpleMarkdown } from "./markdown";

type PendingEntry = {
	resolve: (html: string) => void;
	source: string;
};

type WorkerInstance = {
	worker: Worker;
	nextId: number;
	pending: Map<number, PendingEntry>;
};

let shared: WorkerInstance | null = null;

const sanitizeConfig = {
	ADD_ATTR: ["target", "rel", "loading", "decoding", "align", "start", "open", "style"],
	ADD_TAGS: ["input", "details", "summary", "section", "header"],
} as DOMPurify.Config;

function sanitizeWorkerHtml(html: string): string {
	return DOMPurify.sanitize(html, sanitizeConfig as any) as unknown as string;
}

function getWorker(): WorkerInstance | null {
	if (shared) return shared;
	try {
		const worker = new Worker(
			new URL("./markdown.worker.ts", import.meta.url),
			{ type: "module" },
		);
		const instance: WorkerInstance = {
			worker,
			nextId: 1,
			pending: new Map(),
		};
		worker.onmessage = (e) => {
			const { id, html, error } = e.data as { id: number; html: string; error?: string };
			const entry = instance.pending.get(id);
			if (!entry) return;
			instance.pending.delete(id);
			if (error) {
				// worker 解析失败：回退到主线程同步渲染，正文不再静默消失
				entry.resolve(renderSimpleMarkdown(entry.source));
				return;
			}
			entry.resolve(sanitizeWorkerHtml(html));
		};
		worker.onerror = () => {
			shared = null;
			worker.terminate();
			// worker 整体崩溃：所有 pending 回退主线程渲染（原实现 resolve("")
			// 会让正文静默消失，且失败风暴下每次调用都新建注定失败的 Worker）
			instance.pending.forEach((entry) => entry.resolve(renderSimpleMarkdown(entry.source)));
			instance.pending.clear();
		};
		shared = instance;
		return instance;
	} catch {
		return null;
	}
}

/** Render markdown asynchronously via a shared Web Worker. DOMPurify is applied
 *  on the main thread. Falls back to synchronous renderSimpleMarkdown if the
 *  worker cannot be created or fails mid-render. */
export function renderMarkdownAsync(source: string): Promise<string> {
	const instance = getWorker();
	if (!instance) {
		return Promise.resolve(renderSimpleMarkdown(source));
	}
	return new Promise((resolve) => {
		const id = instance.nextId++;
		instance.pending.set(id, { resolve, source });
		instance.worker.postMessage({ id, source });
	});
}
