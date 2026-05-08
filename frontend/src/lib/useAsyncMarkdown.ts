// Async markdown rendering via Web Worker.
// Falls back to synchronous renderSimpleMarkdown if the worker fails to load.
import DOMPurify from "dompurify";
import { renderSimpleMarkdown } from "./markdown";

type WorkerInstance = {
	worker: Worker;
	nextId: number;
	pending: Map<number, (html: string) => void>;
};

let shared: WorkerInstance | null = null;

const sanitizeConfig: DOMPurify.Config = {
	ADD_ATTR: ["target", "rel", "loading", "decoding", "align", "start", "open"],
	ADD_TAGS: ["input", "details", "summary", "section", "header"],
};

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
			const { id, html } = e.data;
			const resolve = instance.pending.get(id);
			if (resolve) {
				instance.pending.delete(id);
				resolve(DOMPurify.sanitize(html, sanitizeConfig));
			}
		};
		worker.onerror = () => {
			shared = null;
			worker.terminate();
			instance.pending.forEach((resolve) => resolve(""));
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
 *  worker cannot be created. */
export function renderMarkdownAsync(source: string): Promise<string> {
	const instance = getWorker();
	if (!instance) {
		return Promise.resolve(renderSimpleMarkdown(source));
	}
	return new Promise((resolve) => {
		const id = instance.nextId++;
		instance.pending.set(id, resolve);
		instance.worker.postMessage({ id, source });
	});
}
