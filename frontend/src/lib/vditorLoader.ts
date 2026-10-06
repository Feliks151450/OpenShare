/** 懒加载自托管的 vscode-vditor@4.0.0 UMD 产物（4.7MB，绝不能进主 bundle）。 */

export const VDITOR_ASSET_ROOT = "/vendor/vditor";
/** 编辑器与纯预览共用同一个 Lute（fork 定制版）；两处显式传路径，只下载一次。 */
export const VDITOR_LUTE_URL = `${VDITOR_ASSET_ROOT}/dist/lutePro.min.js`;

const STYLE_URL = `${VDITOR_ASSET_ROOT}/dist/index.css`;
const SCRIPT_URL = `${VDITOR_ASSET_ROOT}/dist/index.min.js`;
const STYLE_ID = "openshare-vditor-style";
const SCRIPT_ID = "openshare-vditor-script";

export interface VditorInstance {
  getValue(): string;
  setValue(markdown: string, clearStack?: boolean): void;
  insertMarkdown(markdown: string): void;
  getHTML(): string;
  focus(): void;
  blur(): void;
  tip(text: string, time?: number): void;
  destroy(): void;
  getCurrentMode(): string;
  switchEditMode(mode: "wysiwyg" | "ir"): void;
  setEditorTheme(theme: string): void;
  setEditorSettings(partial: Record<string, unknown>): void;
  markSaved(): void;
  isDirty(): boolean;
}

export interface VditorStatic {
  preview(element: HTMLElement, markdown: string, options?: Record<string, unknown>): Promise<void>;
  previewImage(imgElement: HTMLElement, lang?: string, theme?: string): void;
}

export type VditorGlobal = VditorStatic & (new (elementId: string, options?: Record<string, unknown>) => VditorInstance);

declare global {
  interface Window {
    Vditor?: VditorGlobal;
    /** fork 内部面板（设置/AI）依赖全局取公开方法，见 INTEGRATION.md 16 节第 7 条 */
    vditor?: VditorInstance;
  }
}

let pending: Promise<VditorGlobal> | null = null;

/** 编辑器必须以字符串 id 构造（fork 会据此补 cache.id），故每个实例要一个唯一 id */
let mountSeq = 0;
export function nextVditorMountId(): string {
  mountSeq += 1;
  return `openshare-vditor-${mountSeq}`;
}

function appendStyle(): void {
  if (document.getElementById(STYLE_ID)) return;
  const link = document.createElement("link");
  link.id = STYLE_ID;
  link.rel = "stylesheet";
  link.href = STYLE_URL;
  document.head.appendChild(link);
}

function appendScript(): Promise<VditorGlobal> {
  const script = document.createElement("script");
  script.id = SCRIPT_ID;
  script.src = SCRIPT_URL;
  const settled = new Promise<VditorGlobal>((resolve, reject) => {
    script.addEventListener("load", () => {
      if (window.Vditor) {
        resolve(window.Vditor);
      } else {
        reject(new Error("vditor 已加载但全局缺失"));
      }
    });
    // fork 内部的 addScript 没有 onerror，加载失败只会静默卡住；我们这层必须有。
    script.addEventListener("error", () => reject(new Error("vditor 脚本加载失败")));
  });
  document.head.appendChild(script);
  return settled;
}

export function loadVditor(): Promise<VditorGlobal> {
  if (window.Vditor) return Promise.resolve(window.Vditor);
  if (!pending) {
    appendStyle();
    pending = appendScript().catch((err: unknown) => {
      // 允许调用方在下次使用时重试（例如从离线切回在线）
      pending = null;
      throw err;
    });
  }
  return pending;
}
