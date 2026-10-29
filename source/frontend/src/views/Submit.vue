<template>
  <div
    class="sub-page"
    :class="{ 'dark-mode': isDarkMode }"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent="dragOver = !busy"
    @dragleave.prevent="onDragLeave"
    @drop.prevent="onDrop"
  >
    <div class="sub-bar">
      <div class="sub-bar-inner">
        <span class="sub-brand">{{ siteName }}</span>
        <span class="sub-user">
          <span class="sub-user-name">{{ studentName }}</span>
          <span class="sub-user-id">{{ studentId }}</span>
        </span>
        <button type="button" class="sub-bar-btn" :disabled="busy" @click="goFiles">已交文件</button>
        <button type="button" class="sub-bar-btn" :disabled="busy" @click="signOut">退出</button>
      </div>
    </div>

    <div class="sub-main">
      <h1 class="sub-title">提交作业</h1>

      <p v-if="courseName" class="sub-course-warning">注意：现在正在提交「{{ courseName }}」，请注意核对！</p>
      <section class="sub-panel" :class="{ dragging: dragOver }">
        <div class="sub-pick">
          <input ref="picker" class="sub-file-input" type="file" multiple :accept="acceptList" @change="onPick" />
          <button type="button" class="sub-btn sub-btn-primary" :disabled="busy" @click="$refs.picker.click()">
            <i class="material-symbols">file_upload</i>选择文件
          </button>
          <span class="sub-pick-hint">{{ dragOver ? "松开即可加入列表" : "也可以把文件拖到这里" }}</span>
        </div>

        <p v-if="rejected.length" class="sub-alert">
          <i class="material-symbols">error</i>
          <span>{{ fileTypeErrorText }}</span>
        </p>

        <ul v-if="items.length" class="sub-list">
          <li v-for="item in items" :key="item.id" class="sub-row" :class="item.status">
            <div class="sub-row-icon">
              <Icon :filename="item.file.name" :mimetype="item.file.type || 'blob'" />
            </div>
            <div class="sub-row-main">
              <div class="sub-row-name">{{ item.file.name }}</div>
              <div class="sub-row-meta">
                <span>{{ formatSize(item.file.size) }}</span>
                <span v-if="item.status === 'done' && item.savedAs !== item.file.name">已存为 {{ item.savedAs }}</span>
                <span v-if="item.status === 'failed'" class="sub-err">{{ item.error }}</span>
              </div>
              <div v-if="item.status === 'uploading'" class="sub-progress">
                <span :style="{ width: item.percent + '%' }"></span>
              </div>
            </div>
            <div class="sub-row-state">
              <button
                v-if="item.status === 'ready' && !busy"
                type="button"
                class="sub-icon-btn"
                aria-label="移除"
                title="移除"
                @click="remove(item)"
              ><i class="material-symbols">close</i></button>
              <span v-else-if="item.status === 'uploading'" class="sub-percent">{{ item.percent }}%</span>
              <span v-else-if="item.status === 'done'" class="sub-ok"><i class="material-symbols">check_circle</i>已交</span>
              <span v-else-if="item.status === 'failed'" class="sub-err"><i class="material-symbols">error</i>失败</span>
            </div>
          </li>
        </ul>

        <div v-if="items.length" class="sub-foot">
          <button v-if="!busy && finishedCount" type="button" class="sub-btn sub-btn-text" @click="clearFinished">清空列表</button>
          <span class="sub-foot-space"></span>
          <button
            v-if="pendingCount || busy"
            type="button"
            class="sub-btn sub-btn-primary"
            :disabled="busy"
            @click="submitAll"
          >{{ busy ? "正在提交…" : `提交 ${pendingCount} 个文件` }}</button>
        </div>
        <details class="sub-types">
          <summary>支持哪些格式？</summary>
          <p>{{ documentTypesText }}</p>
        </details>
      </section>
    </div>
  </div>
</template>

<script>
import { postUploadChunk } from "@/utils/uploadTransport";
import { getters, state } from "@/store";
import Icon from "@/components/files/Icon.vue";
import { globalVars } from "@/utils/constants";
import { getApiPath } from "@/utils/url.js";
import { logout, renew } from "@/utils/auth";
import { COURSE_NAME, DOCUMENT_EXTENSIONS, DOCUMENT_TYPES_TEXT, UNSUPPORTED_FILE_MESSAGE, indexedName, isDocumentFile } from "@/utils/submitOnly.js";

const CHUNK_SIZE = 16 * 1024 * 1024;
const MAX_RENAME_TRIES = 200;
let nextId = 1;

function randomSession() {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

function errorText(status) {
  if (status === 0) return "网络中断，请检查网络后重试";
  if (status === 401) return "登录已过期，请退出后重新登录";
  if (status === 403) return "没有提交权限";
  if (status === 413) return "文件太大";
  if (status === 415) return UNSUPPORTED_FILE_MESSAGE;
  if (status >= 500) return "服务器出错，请稍后重试";
  return `上传失败（${status}）`;
}

export default {
  name: "submit",
  components: { Icon },
  data: () => ({
    items: [],
    rejected: [],
    busy: false,
    dragOver: false,
    dragDepth: 0,
  }),
  computed: {
    siteName: () => globalVars.name || "作业提交",
    documentTypesText: () => DOCUMENT_TYPES_TEXT,
    fileTypeErrorText: () => UNSUPPORTED_FILE_MESSAGE,
    courseName: () => COURSE_NAME,
    isDarkMode: () => getters.isDarkMode(),
    acceptList: () => DOCUMENT_EXTENSIONS.join(","),
    scope() {
      return (state.user?.scopes || [])[0] || null;
    },
    studentId() {
      return state.user?.username || "";
    },
    studentName() {
      const folder = (this.scope?.scope || "").replace(/^\/+|\/+$/g, "");
      const cut = folder.indexOf("_");
      return cut > 0 ? folder.slice(cut + 1) : "";
    },
    pendingCount() {
      return this.items.filter((i) => i.status === "ready" || i.status === "failed").length;
    },
    finishedCount() {
      return this.items.filter((i) => i.status === "done").length;
    },
  },
  mounted() {
    document.title = this.siteName;
    window.addEventListener("beforeunload", this.guardLeave);
  },
  beforeUnmount() {
    window.removeEventListener("beforeunload", this.guardLeave);
  },
  methods: {
    guardLeave(e) {
      if (this.busy) {
        e.preventDefault();
        e.returnValue = "";
      }
    },
    formatSize(n) {
      if (n < 1024) return `${n} B`;
      if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
      if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
      return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
    },
    addFiles(list) {
      const rejected = [];
      for (const file of list) {
        if (!isDocumentFile(file.name)) {
          rejected.push(file.name);
          continue;
        }
        if (file.size === 0) {
          rejected.push(`${file.name}（空文件）`);
          continue;
        }
        this.items.push({ id: nextId++, file, status: "ready", percent: 0, savedAs: "", error: "" });
      }
      this.rejected = rejected;
    },
    onPick(e) {
      this.addFiles(Array.from(e.target.files || []));
      e.target.value = "";
    },
    onDragEnter() {
      this.dragDepth++;
      this.dragOver = !this.busy;
    },
    onDragLeave() {
      this.dragDepth = Math.max(0, this.dragDepth - 1);
      if (this.dragDepth === 0) this.dragOver = false;
    },
    onDrop(e) {
      this.dragDepth = 0;
      this.dragOver = false;
      if (this.busy) return;
      this.addFiles(Array.from(e.dataTransfer?.files || []));
    },
    remove(item) {
      this.items = this.items.filter((i) => i !== item);
    },
    clearFinished() {
      this.items = this.items.filter((i) => i.status !== "done");
      this.rejected = [];
    },
    goFiles() {
      if (this.busy) return;
      this.$router.push("/files/");
    },
    async signOut() {
      await logout();
    },
    async submitAll() {
      if (this.busy || !this.scope) return;
      this.busy = true;
      try {
        for (const item of this.items) {
          if (item.status !== "ready" && item.status !== "failed") continue;
          await this.uploadItem(item);
        }
      } finally {
        this.busy = false;
      }
    },
    async uploadItem(item) {
      item.status = "uploading";
      item.percent = 0;
      item.error = "";
      try {
        for (let n = 0; n < MAX_RENAME_TRIES; n++) {
          const name = indexedName(item.file.name, n);
          const result = await this.sendFile(item, name);
          if (result === "ok") {
            item.savedAs = name;
            item.percent = 100;
            item.status = "done";
            return;
          }
        }
        throw Object.assign(new Error("同名文件太多，请改名后再交"), { friendly: true });
      } catch (err) {
        item.status = "failed";
        item.error = err.friendly ? err.message : err.message || "上传失败";
      }
    },
    // Uploads one file in chunks. Returns "ok", or "conflict" when the name is taken.
    async sendFile(item, name) {
      const file = item.file;
      const session = randomSession();
      const url = getApiPath("resources", { source: this.scope.name, path: `/${name}` });
      for (let offset = 0; offset < file.size; offset += CHUNK_SIZE) {
        const chunk = file.slice(offset, Math.min(offset + CHUNK_SIZE, file.size));
        const headers = {
          "Content-Type": "application/octet-stream",
          "X-File-Chunk-Offset": String(offset),
          "X-File-Total-Size": String(file.size),
          "X-File-Upload-Session": session,
        };
        let res = await this.postChunk(url, chunk, headers, offset, file.size, item);
        if (res.status === 401) {
          try {
            await renew();
          } catch {
            /* reported below */
          }
          res = await this.postChunk(url, chunk, headers, offset, file.size, item);
        }
        if (res.status === 409 && offset === 0) return "conflict";
        if (res.status < 200 || res.status > 299) {
          throw Object.assign(new Error(errorText(res.status)), { friendly: true });
        }
      }
      return "ok";
    },
    async postChunk(url, body, headers, offset, total, item) {
      const target = new URL(url, window.location.href);
      try {
        const response = await postUploadChunk({
          source:target.searchParams.get("source"), path:target.searchParams.get("path"), body, headers,
          onprogress:percent => { item.percent = Math.min(99,Math.floor((offset + percent*body.size/100)/total*100)); }
        });
        return {status:200,body:response};
      } catch (error) {
        return {status:error.response?.status || 0,body:error.message};
      }
    },
  },
};
</script>

<style scoped>
.sub-page {
  position: fixed;
  inset: 0;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  background: var(--background);
  color: var(--textPrimary);
  font-size: 15px;
  line-height: 1.55;
}

.sub-page .material-symbols {
  font-size: 1.25em;
  vertical-align: -0.22em;
}

/* Top bar on the same surface as the file manager panels */
.sub-bar {
  background: var(--surfacePrimary);
  padding-top: var(--safe-area-top);
  box-shadow: 0 1px 0 var(--divider);
}

.sub-bar-inner {
  max-width: 760px;
  margin: 0 auto;
  height: 3.6em;
  padding: 0 16px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.sub-brand {
  font-weight: 600;
  margin-right: auto;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sub-user {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.25;
  margin-right: 8px;
}

.sub-user-name {
  font-weight: 600;
}

.sub-user-id {
  font-size: 0.8em;
  color: var(--textSecondary);
  font-variant-numeric: tabular-nums;
}

.sub-bar-btn {
  font: inherit;
  font-size: 0.93em;
  border: 0;
  background: none;
  color: var(--primaryColor);
  padding: 8px 10px;
  border-radius: 0.6em;
  cursor: pointer;
}

.sub-bar-btn:hover {
  background: var(--surfaceSecondary);
}

.sub-main {
  max-width: 760px;
  margin: 0 auto;
  padding: 28px 16px calc(40px + var(--safe-area-bottom));
}

.sub-title {
  font-size: 1.6em;
  font-weight: 600;
  margin: 0 0 20px;
}

.sub-course-warning {
  margin: 0 0 16px;
  padding: 10px 14px;
  border-radius: 0.6em;
  background: rgba(244, 67, 54, 0.12);
  color: #c62828;
  font-weight: 700;
}

.dark-mode .sub-course-warning {
  color: #ff8a80;
}

.sub-panel {
  background: var(--surfacePrimary);
  border-radius: var(--borderRadius);
  box-shadow: var(--surfaceElevationShadow);
  border: 2px solid transparent;
  transition: border-color 0.15s;
}

.sub-panel.dragging {
  border-color: var(--primaryColor);
}

.sub-pick {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px 16px;
  padding: 20px 20px 8px;
}

.sub-file-input {
  display: none;
}

.sub-pick-hint {
  color: var(--textSecondary);
}

.sub-types {
  padding: 0 20px 16px;
  font-size: 0.9em;
  color: var(--textSecondary);
}

.sub-types summary {
  cursor: pointer;
  width: max-content;
  color: var(--primaryColor);
}

.sub-types p {
  margin: 6px 0 0;
  word-break: break-word;
}

.sub-btn {
  font: inherit;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: 0.8em;
  padding: 10px 18px;
  cursor: pointer;
}

.sub-btn:disabled {
  opacity: 0.6;
  cursor: default;
}

.sub-btn-primary {
  background: var(--primaryColor);
  color: #fff;
  font-weight: 500;
}

.sub-btn-primary:not(:disabled):hover {
  background: var(--dark-blue);
}

.sub-btn-text {
  background: none;
  color: var(--textSecondary);
  padding-left: 8px;
  padding-right: 8px;
}

.sub-alert {
  display: flex;
  gap: 8px;
  margin: 0 20px 16px;
  padding: 10px 12px;
  border-radius: 0.6em;
  background: rgba(244, 67, 54, 0.1);
  color: var(--dark-red);
  font-size: 0.9em;
  word-break: break-word;
}

.dark-mode .sub-alert {
  color: #ff8a80;
}

.sub-list {
  list-style: none;
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--divider);
}

.sub-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 20px;
  border-bottom: 1px solid var(--divider);
}

.sub-row-icon {
  flex: none;
  width: 2.6em;
  height: 2.6em;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.6em;
  background: var(--iconBackground);
  overflow: hidden;
}

.sub-row-main {
  flex: 1;
  min-width: 0;
}

.sub-row-name {
  word-break: break-all;
}

.sub-row-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 12px;
  font-size: 0.85em;
  color: var(--textSecondary);
}

.sub-progress {
  height: 4px;
  margin-top: 6px;
  border-radius: 2px;
  background: var(--surfaceSecondary);
  overflow: hidden;
}

.sub-progress span {
  display: block;
  height: 100%;
  background: var(--primaryColor);
  transition: width 0.2s linear;
}

.sub-row-state {
  flex: none;
  font-size: 0.9em;
  white-space: nowrap;
}

.sub-percent {
  color: var(--primaryColor);
  font-variant-numeric: tabular-nums;
}

.sub-ok {
  color: #2e7d32;
}

.dark-mode .sub-ok {
  color: #81c995;
}

.sub-err {
  color: var(--dark-red);
}

.sub-ok .material-symbols,
.sub-err .material-symbols {
  margin-right: 3px;
}

.sub-icon-btn {
  border: 0;
  background: none;
  color: var(--textSecondary);
  padding: 6px;
  border-radius: 50%;
  cursor: pointer;
  line-height: 0;
}

.sub-icon-btn:hover {
  background: var(--surfaceSecondary);
}

.sub-foot {
  display: flex;
  align-items: center;
  padding: 14px 20px;
}

.sub-foot-space {
  flex: 1;
}

@media (hover: none) {
  .sub-pick-hint {
    display: none;
  }
}

@media (max-width: 480px) {
  .sub-main {
    padding-top: 20px;
  }
  .sub-brand {
    font-size: 0.93em;
  }
  .sub-bar-btn {
    padding: 8px 6px;
  }
  .sub-pick .sub-btn-primary {
    flex: 1;
    justify-content: center;
  }
  .sub-pick,
  .sub-types,
  .sub-row,
  .sub-foot {
    padding-left: 14px;
    padding-right: 14px;
  }
  .sub-alert {
    margin-left: 14px;
    margin-right: 14px;
  }
}
</style>
