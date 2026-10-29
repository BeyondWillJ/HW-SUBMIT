<template>
  <div class="card-content">
    <p class="card-message">
      申请删除以下{{ items.length > 1 ? ` ${items.length} 个` : "" }}文件，审核通过后才会删除：
    </p>
    <ul class="delete-request-files">
      <li v-for="item in items" :key="item.path">{{ item.name }}</li>
    </ul>
    <label class="delete-request-label" for="deleteRequestReason">删除理由</label>
    <textarea
      id="deleteRequestReason"
      ref="reason"
      v-model="reason"
      v-focus
      class="input delete-request-reason"
      rows="4"
      maxlength="200"
      placeholder="例如：我是XXX，交错了文件“文件1.pdf”，正确的应该是新上传的“文件2.pdf”"
    ></textarea>
    <p class="delete-request-count">{{ reason.trim().length }}/200</p>
  </div>

  <div class="card-actions">
    <button type="button" class="button button--flat button--grey" @click="cancel">取消</button>
    <button type="button" class="button button--flat" :disabled="reason.trim() === '' || sending" @click="submit">
      {{ sending ? "提交中…" : "提交" }}
    </button>
  </div>
</template>

<script>
import { mutations, state } from "@/store";
import { notify } from "@/notify";
import { submitDeleteRequest } from "@/utils/deleteRequests.js";

// HW-SUBMIT: student deletion requests reviewed by the teacher.
export default {
  name: "deleteRequest",
  props: {
    items: {
      type: Array,
      required: true,
    },
  },
  data: () => ({
    reason: "",
    sending: false,
  }),
  methods: {
    cancel() {
      mutations.closeTopPrompt();
    },
    async submit() {
      const reason = this.reason.trim();
      if (reason === "" || this.sending) return;
      this.sending = true;
      const failed = [];
      for (const item of this.items) {
        try {
          await submitDeleteRequest(item.source || state.req.source, item.path, reason);
        } catch (err) {
          failed.push(err.status === 409 ? `${item.name}（已在审核中）` : item.name);
        }
      }
      this.sending = false;
      if (failed.length === this.items.length) {
        notify.showErrorToast(`提交失败：${failed.join("、")}`);
        return;
      }
      mutations.closeTopPrompt();
      if (failed.length) {
        notify.showErrorToast(`部分未提交：${failed.join("、")}`);
      }
      notify.showSuccessToast("删除申请已提交，正在审核中");
    },
  },
};
</script>

<style scoped>
.delete-request-files {
  margin: 0 0 1em;
  padding-left: 1.2em;
  word-break: break-all;
}

.delete-request-label {
  display: block;
  margin-bottom: 0.3em;
  font-weight: 500;
}

.delete-request-reason {
  width: 100%;
  resize: vertical;
  font: inherit;
  line-height: 1.5;
}

.delete-request-count {
  margin: 0.3em 0 0;
  text-align: right;
  font-size: 0.85em;
  opacity: 0.7;
}
</style>
