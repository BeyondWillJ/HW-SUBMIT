// HW-SUBMIT: course info and student helpers.
// The course name and allowed extensions come from the course config (courses.toml) through
// globalVars.hwsubmitCourse; the server enforces the same list.
import { globalVars } from "@/utils/constants";

const FALLBACK_EXTENSIONS = [
  ".doc", ".docx", ".docm", ".dot", ".dotx", ".dotm",
  ".rtf", ".odt", ".ott", ".wps", ".wpt", ".pages",
  ".txt", ".md", ".markdown", ".tex",
  ".xls", ".xlsx", ".xlsm", ".xlsb", ".xlt", ".xltx", ".xltm",
  ".csv", ".ods", ".ots", ".et", ".ett", ".numbers",
  ".ppt", ".pptx", ".pptm", ".pps", ".ppsx", ".pot", ".potx", ".potm",
  ".odp", ".otp", ".dps", ".dpt", ".key",
  ".pdf", ".ofd", ".xps", ".oxps", ".caj", ".djvu", ".epub", ".mobi",
];

const courseVars = globalVars.hwsubmitCourse || {};

/** Course display name, e.g. "数值模拟" ("" when not configured). */
export const COURSE_NAME = courseVars.name || "";

export const DOCUMENT_EXTENSIONS =
  Array.isArray(courseVars.extensions) && courseVars.extensions.length ? courseVars.extensions : FALLBACK_EXTENSIONS;

/** "允许的文件类型包括：*.doc, *.docx, ..." */
export const DOCUMENT_TYPES_TEXT = "允许的文件类型包括：" + DOCUMENT_EXTENSIONS.map((e) => `*${e}`).join(", ");

export const UNSUPPORTED_FILE_MESSAGE = "只能提交选定格式的文件。允许的文件类型见上传界面。";

export function fileExtension(name) {
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot).toLowerCase() : "";
}

export function isDocumentFile(name) {
  return DOCUMENT_EXTENSIONS.includes(fileExtension(name));
}

/** A logged-in non-admin account: may list its own folder and upload documents only. */
export function isStudentUser(user) {
  if (!user || !user.username || user.username === "anonymous") {
    return false;
  }
  return !user.permissions?.admin;
}

/** Student's real name, from their folder "学号_姓名" ("" for non-students). */
export function studentNameOf(user) {
  if (!isStudentUser(user)) return "";
  const folder = ((user.scopes || [])[0]?.scope || "").replace(/^\/+|\/+$/g, "");
  const cut = folder.indexOf("_");
  return cut > 0 ? folder.slice(cut + 1) : "";
}

/** "作业.docx" -> "作业(1).docx" */
export function indexedName(name, n) {
  if (n === 0) return name;
  const ext = fileExtension(name);
  const stem = ext ? name.slice(0, name.length - ext.length) : name;
  return `${stem}(${n})${name.slice(name.length - ext.length)}`;
}

export const STUDENT_OPEN_BLOCKED = "已提交的文件不能打开或下载";
