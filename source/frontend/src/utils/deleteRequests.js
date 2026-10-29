// HW-SUBMIT: student deletion requests reviewed by the teacher.
import { reactive } from "vue";
import { getApiPath } from "@/utils/url.js";
import { state } from "@/store";
import { isStudentUser } from "@/utils/submitOnly.js";

/** key "source\0path" -> { status, note } for the current student's latest requests */
export const deleteRequestStatus = reactive({});

const key = (source, path) => `${source}\u0000${path}`;

export function requestStatusFor(source, path) {
  return deleteRequestStatus[key(source, path)] || null;
}

export async function refreshDeleteRequests() {
  if (!isStudentUser(state.user)) return;
  try {
    const res = await fetch(getApiPath("hwsubmit/delete-requests"), {
      credentials: "same-origin",
      headers: { sessionId: state.sessionId },
    });
    if (!res.ok) return;
    const list = await res.json();
    for (const k of Object.keys(deleteRequestStatus)) delete deleteRequestStatus[k];
    for (const q of list) {
      deleteRequestStatus[key(q.source, q.path)] = { status: q.status, note: q.note || "" };
    }
  } catch {
    /* keep the last known state */
  }
}

/** Throws an Error with .status on failure (409 = already pending). */
export async function submitDeleteRequest(source, path, reason) {
  const res = await fetch(getApiPath("hwsubmit/delete-requests"), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", sessionId: state.sessionId },
    body: JSON.stringify({ source, path, reason }),
  });
  if (!res.ok) {
    const err = new Error((await res.text()) || `HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
  const q = await res.json();
  deleteRequestStatus[key(q.source, q.path)] = { status: q.status, note: "" };
  return q;
}

export const STATUS_LABELS = {
  pending: "删除审核中",
  rejected: "删除申请被拒绝",
  failed: "删除未完成",
};
