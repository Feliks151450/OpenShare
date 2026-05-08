// Web Worker: offloads CPU-intensive marked.parse() from the main thread.
// DOMPurify stays on the main thread (requires DOM API).
import { marked } from "marked";

// Reuse the same marked config across invocations
marked.use({
	gfm: true,
	breaks: true,
});

interface WorkerRequest {
	id: number;
	source: string;
}

interface WorkerResponse {
	id: number;
	html: string;
	error?: string;
}

self.onmessage = (e: MessageEvent<WorkerRequest>) => {
	const { id, source } = e.data;
	const normalized = source.replace(/\r\n/g, "\n");
	if (!normalized.trim()) {
		self.postMessage({ id, html: "" } satisfies WorkerResponse);
		return;
	}
	try {
		const html = marked.parse(normalized, { async: false }) as string;
		self.postMessage({ id, html } satisfies WorkerResponse);
	} catch (err: unknown) {
		const message = err instanceof Error ? err.message : "markdown parse error";
		self.postMessage({ id, html: "", error: message } satisfies WorkerResponse);
	}
};
